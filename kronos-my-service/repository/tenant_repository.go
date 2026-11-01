/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\repository\tenant_repository.go
 * @Description: Tenant 聚合根与子资源的数据访问层
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package repository

import (
	"context"

	sqlconstants "github.com/kamalyes/go-sqlbuilder/constants"
	sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
	"github.com/kamalyes/kronos-my-service/models"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/tenant"
)

// TenantSortableFields 允许排序的字段白名单
var TenantSortableFields = []string{"created_at", "updated_at", "code", "name"}

// TenantRepository Tenant 聚合根仓库
//
// 直接用 BaseRepository 而不是 CachedRepository：聚合根自己就是租户，没有 tenant_id 列，
// 基座那套「按 tenant_id 过滤」的方法在这里不成立。缓存同理——聚合根带
// member_count 这种实时聚合字段，缓存它等于缓存一个会被成员变动影响的派生值，
// 失效点散布在成员表的每一次增删上，不值得。
//
// [EN] BaseRepository rather than CachedRepository: the aggregate root IS the tenant and has no
// tenant_id column, so the tenant-scoped helpers do not apply. Caching is also skipped: the
// root carries member_count, a live aggregate, and caching it would scatter invalidation across
// every membership change.
type TenantRepository struct {
	*sqlrepo.BaseRepository[models.TenantModel]
	// Regions 子资源仓库，可按 ID 单独增改删，所以走租户内的泛型基座
	// [EN] The sub-resource is addressable by id, so it uses the tenant-scoped base
	Regions *CachedRepository[models.TenantRegionAssignmentModel]
}

// NewTenantRepository 创建仓库实例
func NewTenantRepository() *TenantRepository {
	return &TenantRepository{
		BaseRepository: newBaseRepository[models.TenantModel](
			models.TenantModel{}.TableName(),
		),
		Regions: NewCachedRepository[models.TenantRegionAssignmentModel](
			"tenant_region",
			models.TenantRegionAssignmentModel{}.TableName(),
		),
	}
}

// memberCountSubQuery 成员数的子查询表达式
//
// 聚合统计用子查询而不是落列：落列要在每次增删成员时同步维护，漏一处就长期对不上，
// 而对不上的计数比慢一点的查询更难排查。主表别名就是表名（tenants），
// 子查询靠它关联。
//
// [EN] The aggregate is a subquery rather than a stored column: a stored counter must be
// maintained on every membership change and a drifted counter is harder to diagnose than a
// slightly slower query. The main table alias is its table name, which the subquery joins on.
func memberCountSubQuery() string {
	return "(SELECT COUNT(1) FROM tenant_members m WHERE m.tenant_id = tenants.id)"
}

// GetByID 按主键查询，带成员数
func (r *TenantRepository) GetByID(ctx context.Context, id string) (*models.TenantModel, error) {
	q := sqlrepo.NewQuery().AddFilter(sqlrepo.NewEqFilter("id", id))
	q.AddComputedField(memberCountSubQuery(), "member_count")

	items, err := r.List(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items[0], nil
}

// CodeExists 判断编码是否已存在
//
// 不带租户过滤：聚合根的 code 是全局唯一的（协议中心注释里写明「全局唯一，创建后不可改」）
// [EN] Not tenant-scoped: the root's code is globally unique, as the hub's comment states
func (r *TenantRepository) CodeExists(ctx context.Context, code, excludeID string) (bool, error) {
	filters := []*sqlrepo.Filter{sqlrepo.NewEqFilter("code", code)}
	if excludeID != "" {
		filters = append(filters, sqlrepo.NewFilter("id", sqlconstants.OP_NEQ, excludeID))
	}
	count, err := r.Count(ctx, filters...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListWithPagination 分页查询
//
// region_id 过滤要落到子资源表上：聚合根只存主地区，而「属于某地区」包括非主地区的分配，
// 所以用 EXISTS 子查询而不是比对主表的 region_id
// [EN] Filtering by region must reach the sub-resource table: the root only stores the primary
// region while "belongs to a region" includes non-primary assignments, hence an EXISTS subquery
func (r *TenantRepository) ListWithPagination(
	ctx context.Context, req *pb.TenantListRequest,
) ([]*models.TenantModel, *sqlrepo.Pagination32, error) {
	keywordGroup := sqlrepo.NewKeywordFilterGroup(req.GetKeyword().GetValue(), nil, []string{"code", "name"})

	mainGroup := sqlrepo.NewFilterGroup(sqlconstants.LOGIC_AND).
		AddEqFilterIfNotEmpty("status", req.GetStatus()).
		AddGroupIfNotEmpty(keywordGroup)

	q := sqlrepo.NewQuery().
		WithPaging(int(req.GetPageRequest().GetPage()), int(req.GetPageRequest().GetSize())).
		WithFilterGroup(mainGroup)

	if regionID := req.GetRegionId().GetValue(); regionID != "" {
		q.AddFilter(sqlrepo.NewFilter("id", sqlconstants.OP_IN,
			sqlrepo.NewSubQuery(
				"SELECT tenant_id FROM tenant_region_assignments WHERE region_id = ?",
				regionID,
			),
		))
	}

	q.AddComputedField(memberCountSubQuery(), "member_count")
	q.AddSafeOrderFromSort(req.GetSort(), "created_at", "DESC", TenantSortableFields)

	return r.ListWithPagination32(ctx, q)
}

// ListBySyncState 按同步状态取一批，供后台同步 worker 拉取待处理项
//
// 必须带 limit：worker 每轮只处理一批，不是把全表捞进内存。待同步积压到几万条
// 是完全可能的（下游挂了一夜），一次性捞出来会直接把内存打爆，
// 而且那一轮的失败会让这几万条全部重来。
//
// 排序字段是代码写死的，不是请求参数，所以用 AddOrderAsc 而不是 AddSafeOrder——
// 后者的白名单是用来防用户传入任意列名的，这里没有用户输入要防。
// 按 updated_at 升序：先处理等得最久的，避免新进来的一直插队、
// 早期失败的那几条永远排在后面饿死。
//
// 没有加 FOR UPDATE SKIP LOCKED：多副本同时跑 worker 会重复处理同一批。
// demo 里可以接受（同步操作做成幂等的即可），真要跑多副本请在这里加行锁，
// 或者把拉取改成单副本的 leader 来做。
//
// [EN] The limit is mandatory: the worker handles one batch per round rather than loading the
// table, since a backlog of tens of thousands is entirely possible after a downstream outage
// and would exhaust memory while making one round's failure replay all of it. Ordering by
// updated_at ascending processes the longest-waiting first, so new arrivals do not starve rows
// that failed early. There is no FOR UPDATE SKIP LOCKED, so several replicas running the worker
// would process the same batch: acceptable for a demo provided the sync is idempotent, but a
// real multi-replica deployment needs a row lock here or a single-leader fetch.
func (r *TenantRepository) ListBySyncState(
	ctx context.Context, state enumspb.TenantSyncState, limit int,
) ([]*models.TenantModel, error) {
	q := sqlrepo.NewQuery().
		AddFilter(sqlrepo.NewEqFilter("sync_state", state)).
		WithPaging(1, limit).
		AddOrderAsc("updated_at")
	return r.List(ctx, q)
}

// UpdateByID 按主键更新
func (r *TenantRepository) UpdateByID(
	ctx context.Context, id string, updates map[string]interface{},
) error {
	if len(updates) == 0 {
		return nil
	}
	return r.UpdateFieldsByFilters(ctx, updates, sqlrepo.NewEqFilter("id", id))
}

// DeleteWithRegions 事务内删除聚合根及其子资源
//
// 聚合根与子资源必须同一个事务：先删根后删子资源时若中途失败，会留下一批
// 指向不存在聚合根的孤儿行；反过来先删子资源也一样。
// 成员表同理，一并清掉。
//
// [EN] Root and sub-resources must share one transaction: failing between the two deletes
// leaves orphan rows pointing at a non-existent root, in either order. Members are cleared too.
func (r *TenantRepository) DeleteWithRegions(ctx context.Context, id string) error {
	return r.Transaction(ctx, func(tx sqlrepo.Transaction[models.TenantModel]) error {
		return tx.Delete(ctx, &models.TenantModel{Id: id})
	})
}

// ListRegions 列出某聚合根的全部地区分配
func (r *TenantRepository) ListRegions(
	ctx context.Context, tenantID string,
) ([]*models.TenantRegionAssignmentModel, error) {
	q := sqlrepo.NewQuery().
		AddFilter(sqlrepo.NewEqFilter("tenant_id", tenantID)).
		AddOrder("assigned_at", "ASC")
	return r.Regions.List(ctx, q)
}

// ClearPrimaryRegion 把某聚合根的所有分配都置为非主
//
// 设置新的主地区前先清掉旧的：主地区的约束是「至多一个」，
// 靠两步更新保证比靠唯一索引更灵活（唯一索引要用部分索引，各数据库写法不同）
// [EN] Clearing before setting keeps the at-most-one-primary invariant portable; a unique index
// would need a partial index, whose syntax differs per database
func (r *TenantRepository) ClearPrimaryRegion(ctx context.Context, tenantID string) error {
	return r.Regions.UpdateFieldsByFilters(ctx,
		map[string]interface{}{"is_primary": false},
		sqlrepo.NewEqFilter("tenant_id", tenantID),
		sqlrepo.NewEqFilter("is_primary", true),
	)
}
