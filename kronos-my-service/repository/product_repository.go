/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\repository\product_repository.go
 * @Description: Product 数据访问层
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package repository

import (
	"context"

	sqlconstants "github.com/kamalyes/go-sqlbuilder/constants"
	sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
	"github.com/kamalyes/kronos-my-service/models"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/catalog"
)

// ProductSortableFields 允许排序的字段白名单
//
// 排序字段来自请求（common.Sorting.field），直接拼进 SQL 等于把 ORDER BY 交给调用方。
// 白名单之外的字段由 AddSafeOrderFromSort 丢弃并退回默认排序，而不是报错——
// 调用方传了个不存在的字段时，返回一页数据比返回 500 更合理
//
// [EN] The sort field comes from the request, so splicing it into SQL would hand ORDER BY to
// the caller. AddSafeOrderFromSort drops anything outside this list and falls back to the
// default order rather than erroring: a bogus field is better answered with a page of data
// than a 500
var ProductSortableFields = []string{"created_at", "updated_at", "name", "code"}

// ProductRepository Product 仓库
//
// 缓存、事务批量删除、租户归属校验都在 CachedRepository 里，这里只放
// 本实体特有的查询（分页条件、唯一性字段）
// [EN] Caching, transactional batch delete and tenant scoping live in CachedRepository; only
// entity-specific queries belong here
type ProductRepository struct {
	*CachedRepository[models.ProductModel]
}

// NewProductRepository 创建仓库实例
func NewProductRepository() *ProductRepository {
	return &ProductRepository{
		CachedRepository: NewCachedRepository[models.ProductModel](
			"product",
			models.ProductModel{}.TableName(),
		),
	}
}

// ListWithPagination 按请求条件分页查询
//
// 过滤条件直接由协议中心的 ProductListRequest 驱动：
//   - keyword 走 name / code 的 LIKE 组（NewKeywordFilterGroup）
//   - type / status 为零值时不参与过滤（AddEqFilterIfNotEmpty），
//     这与 proto3 的语义一致：枚举零值是 UNSPECIFIED，表示「不限」
//   - 排序过白名单，默认 created_at DESC
//
// [EN] Filters are driven straight by the hub's list request. A zero enum means UNSPECIFIED,
// i.e. "any", so those fields are only applied when non-zero, matching proto3 semantics
func (r *ProductRepository) ListWithPagination(
	ctx context.Context,
	tenantID string,
	req *pb.ProductListRequest,
) ([]*models.ProductModel, *sqlrepo.Pagination32, error) {
	keywordGroup := sqlrepo.NewKeywordFilterGroup(req.GetKeyword().GetValue(), nil, []string{"name", "code"})

	mainGroup := sqlrepo.NewFilterGroup(sqlconstants.LOGIC_AND).
		AddFilter(sqlrepo.NewEqFilter("tenant_id", tenantID)).
		AddEqFilterIfNotEmpty("type", req.GetType()).
		AddEqFilterIfNotEmpty("status", req.GetStatus()).
		AddGroupIfNotEmpty(keywordGroup)

	q := sqlrepo.NewQuery().
		WithPaging(int(req.GetPageRequest().GetPage()), int(req.GetPageRequest().GetSize())).
		WithFilterGroup(mainGroup)

	q.AddSafeOrderFromSort(req.GetSort(), "created_at", "DESC", ProductSortableFields)

	return r.ListWithPagination32(ctx, q)
}
