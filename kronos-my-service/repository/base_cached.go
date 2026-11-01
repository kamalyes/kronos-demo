/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\repository\base_cached.go
 * @Description: 带缓存与事务的泛型仓储基座，所有实体仓库共用
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package repository

import (
	"context"

	cachex "github.com/kamalyes/go-cachex"
	sqlconstants "github.com/kamalyes/go-sqlbuilder/constants"
	sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
)

// TenantScoped 要求实体能报出自己的主键与所属租户
//
// 泛型基座靠它定位缓存键与校验归属，所有落库实体都应实现这两个方法
// [EN] The generic base uses this to build the cache key and verify ownership
type TenantScoped interface {
	GetID() string
	GetTenantID() string
}

// CachedRepository 在 go-sqlbuilder 的 BaseRepository 之上加缓存与事务批量操作
//
// 为什么做成泛型基座而不是每个实体各写一份：缓存键怎么拼、什么时候失效、
// 批量删除要不要事务，这些决定一旦在四个仓库里各写一遍，就会在第五个实体上走样。
// 收在一处后，新增实体只需声明 entity 名与表名。
//
// [EN] A generic base rather than one copy per entity: how the cache key is built, when it is
// invalidated and whether batch deletes are transactional are decisions that drift once copied
// into four repositories. Centralized, a new entity only declares its name and table.
type CachedRepository[T any] struct {
	*sqlrepo.BaseRepository[T]
	// entity 缓存键的命名空间，取实体名的蛇形形式
	entity string
}

// NewCachedRepository 构造泛型仓储基座
func NewCachedRepository[T any](entity, tableName string) *CachedRepository[T] {
	return &CachedRepository[T]{
		BaseRepository: newBaseRepository[T](tableName),
		entity:         entity,
	}
}

// modelCache 取本实体已注册的整实例缓存
//
// 用非 panic 的 GetModelCache：缓存是可选的。Redis 没配时 bootstrap 会跳过注册
// （见 bootstrap/kvcache.go），这里拿不到就退回直查数据库，功能照常。
// 用 MustGetModelCache 会让「没开 Redis」变成启动即崩。
//
// 缓存键由注册时声明的 KeyFields 决定，本基座统一按 (TenantId, Id) 注册——
// 键里必须带租户：缓存键是另一个越权读的口子，A 租户读过的记录会被 B 租户命中。
//
// [EN] GetModelCache rather than MustGetModelCache, because the cache is optional: bootstrap
// skips registration when Redis is absent and this falls back to a direct query. The key is
// whatever KeyFields was registered with, and this base always registers (TenantId, Id): the
// tenant must be in the key, or the cache becomes another cross-tenant read hole.
func (r *CachedRepository[T]) modelCache() *cachex.ModelCache[T] {
	c, err := cachex.GetModelCache[T]()
	if err != nil {
		return nil
	}
	return c
}

// GetByIDScoped 按主键查询并校验租户归属
//
// 所有单条查询都带 tenantID 过滤：多租户服务里只按主键查是越权读的经典口子，
// 调用方只要猜到别的租户的 ID 就能读到数据。tenantID 由拦截器从请求上下文解析，
// 不从请求体取（请求体是调用方可控的）。
//
// [EN] Every single-row read is tenant-scoped: looking up by primary key alone is the classic
// cross-tenant read hole. The tenant comes from the request context, never from the request
// body, which the caller controls.
func (r *CachedRepository[T]) GetByIDScoped(ctx context.Context, tenantID, id string) (*T, error) {
	return r.GetByFilters(ctx,
		sqlrepo.NewEqFilter("id", id),
		sqlrepo.NewEqFilter("tenant_id", tenantID),
	)
}

// GetByIDCached 带缓存的单条查询
//
// 三层兜底由 go-cachex 的 ModelCache 提供：本地分片缓存 → Redis Hash →
// 按键精准回源数据库。不自己写「先 Get 再 Set」那套，原因有三个，
// 每一个都是手写版本一定会漏的：
//
//  1. 并发未命中会同时穿到数据库（缓存击穿），要自己接 singleflight；
//  2. 本地缓存改了之后其他副本不知道，要自己接失效广播——单副本测不出来，
//     多副本上线后表现为「改完有的节点生效有的不生效」；
//  3. 回源查询要按缺失的键精准查，不是整表捞。
//
// 回源仍然带租户条件：ModelCache 注册时的 KeyFields 是 (TenantId, Id)，
// 它生成的 WHERE 两个条件都带上，所以缓存路径和直查路径的租户隔离是一致的。
//
// [EN] ModelCache provides local, Redis and precise per-key database fallback. A hand-rolled
// get-then-set misses three things every time: concurrent misses all reach the database unless
// singleflight is wired in; other replicas keep a stale local copy unless invalidation is
// broadcast, which one replica cannot reveal and production shows as an update landing on some
// nodes only; and the reload must query the missing keys precisely rather than scan. The
// reload stays tenant-scoped because KeyFields is (TenantId, Id), so both conditions appear in
// its WHERE and the cached path isolates tenants exactly as the direct path does.
func (r *CachedRepository[T]) GetByIDCached(ctx context.Context, tenantID, id string) (*T, error) {
	c := r.modelCache()
	if c == nil {
		return r.GetByIDScoped(ctx, tenantID, id)
	}

	entity, found, err := c.Get(ctx, tenantID, id)
	if err != nil {
		// 缓存层出错不该让查询失败：退回直查数据库
		// [EN] A cache failure must not fail the query; fall back to a direct read
		return r.GetByIDScoped(ctx, tenantID, id)
	}
	if !found {
		return nil, nil
	}
	return &entity, nil
}

// Invalidate 删除单条缓存
//
// 写路径一律「改完就删键」，不是「改完就写新值」：写新值要求写缓存与写库原子，
// 否则并发更新下两者会错位（A 写库 → B 写库 → B 写缓存 → A 写缓存，缓存留下旧值）。
// 删键的失败模式只是下一次读多查一次库。
//
// ModelCache 的 Delete 会同时清本地、清 Redis 并广播给其他副本，
// 这正是自己维护 map 做不到的那一步。
//
// [EN] The write path deletes the key rather than writing the new value: writing through would
// need cache and database updated atomically, or concurrent updates leave a stale value
// (A writes db, B writes db, B writes cache, A writes cache). A failed delete merely costs one
// extra query on the next read. ModelCache's Delete clears the local layer and Redis and
// broadcasts to the other replicas, which is the step a hand-maintained map cannot do.
func (r *CachedRepository[T]) Invalidate(ctx context.Context, tenantID, id string) {
	c := r.modelCache()
	if c == nil {
		return
	}
	_ = c.DeleteByKey(ctx, tenantID, id)
}

// InvalidateMany 批量删键
//
// 批量写之后要删的是每条记录各自的键，不是一个「列表键」：列表结果没有进缓存，
// 进缓存的是单条详情。漏删一条的后果是那一条在 TTL 内读到旧值。
//
// [EN] After a batch write the keys to delete are the per-row keys, not some list key: list
// results are not cached, individual rows are. A missed key means that one row reads stale
// until its TTL expires.
func (r *CachedRepository[T]) InvalidateMany(ctx context.Context, tenantID string, ids []string) {
	c := r.modelCache()
	if c == nil {
		return
	}
	for _, id := range ids {
		_ = c.DeleteByKey(ctx, tenantID, id)
	}
}

// UpdateScoped 按主键更新并失效缓存
func (r *CachedRepository[T]) UpdateScoped(
	ctx context.Context, tenantID, id string, updates map[string]interface{},
) error {
	if len(updates) == 0 {
		return nil
	}
	if err := r.UpdateFieldsByFilters(ctx, updates,
		sqlrepo.NewEqFilter("id", id),
		sqlrepo.NewEqFilter("tenant_id", tenantID),
	); err != nil {
		return err
	}
	r.Invalidate(ctx, tenantID, id)
	return nil
}

// DeleteScoped 按主键删除并失效缓存
func (r *CachedRepository[T]) DeleteScoped(ctx context.Context, tenantID, id string) error {
	if err := r.DeleteByFilters(ctx,
		sqlrepo.NewEqFilter("id", id),
		sqlrepo.NewEqFilter("tenant_id", tenantID),
	); err != nil {
		return err
	}
	r.Invalidate(ctx, tenantID, id)
	return nil
}

// ExistsByField 判断某字段在租户内是否已存在，excludeID 非空时排除自身（供更新时校验）
func (r *CachedRepository[T]) ExistsByField(
	ctx context.Context, tenantID, field, value, excludeID string,
) (bool, error) {
	filters := []*sqlrepo.Filter{
		sqlrepo.NewEqFilter("tenant_id", tenantID),
		sqlrepo.NewEqFilter(field, value),
	}
	if excludeID != "" {
		filters = append(filters, sqlrepo.NewFilter("id", sqlconstants.OP_NEQ, excludeID))
	}
	count, err := r.Count(ctx, filters...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// BatchDeleteByIDs 事务内批量删除，返回实际删除的 ID 列表
//
// 为什么要事务：逐条删除是 N 次往返，且中途失败会留下删一半的状态——
// 调用方拿到「部分成功」后无法判断该重试哪些，重试又可能重复删。
// 放进一个事务后要么全删要么全不删，调用方的重试语义才是确定的。
//
// 先查出存在的记录再删：批量接口的调用方常常混进不属于本租户的 ID，
// 直接按 ID 删会让「删了 0 行」和「删成功」无法区分，也顺手拿到了要失效的键。
//
// [EN] Why a transaction: deleting one by one is N round trips and a mid-way failure leaves a
// half-deleted state, after which the caller cannot tell what to retry. Inside one transaction
// it is all or nothing, so retry semantics are well defined. Existing rows are fetched first
// because batch callers routinely include ids from other tenants, which also yields the keys to
// invalidate.
func (r *CachedRepository[T]) BatchDeleteByIDs(
	ctx context.Context, tenantID string, ids []string,
) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	q := sqlrepo.NewQuery().
		AddFilter(sqlrepo.NewEqFilter("tenant_id", tenantID)).
		AddFilter(sqlrepo.NewFilter("id", sqlconstants.OP_IN, ids))
	existing, err := r.List(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, nil
	}

	if err := r.Transaction(ctx, func(tx sqlrepo.Transaction[T]) error {
		return tx.DeleteBatch(ctx, existing...)
	}); err != nil {
		return nil, err
	}

	deleted := make([]string, 0, len(existing))
	for _, item := range existing {
		if scoped, ok := any(item).(TenantScoped); ok {
			deleted = append(deleted, scoped.GetID())
			r.Invalidate(ctx, tenantID, scoped.GetID())
		}
	}
	return deleted, nil
}
