/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\repository\warehouse_repository.go
 * @Description: Warehouse 数据访问层
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package repository

import (
	"context"

	sqlconstants "github.com/kamalyes/go-sqlbuilder/constants"
	sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
	"github.com/kamalyes/kronos-my-service/models"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/inventory"
)

// WarehouseSortableFields 允许排序的字段白名单
//
// 这个实体带 sort_order，是业务上真正的默认排序字段，所以默认排序用它而不是 created_at
// [EN] This entity has sort_order, the real business default, so it orders by that
var WarehouseSortableFields = []string{"sort_order", "created_at", "updated_at", "name", "code"}

// WarehouseRepository Warehouse 仓库
type WarehouseRepository struct {
	*CachedRepository[models.WarehouseModel]
}

// NewWarehouseRepository 创建仓库实例
func NewWarehouseRepository() *WarehouseRepository {
	return &WarehouseRepository{
		CachedRepository: NewCachedRepository[models.WarehouseModel](
			"warehouse",
			models.WarehouseModel{}.TableName(),
		),
	}
}

// ListWithPagination 按请求条件分页查询
func (r *WarehouseRepository) ListWithPagination(
	ctx context.Context, tenantID string, req *pb.WarehouseListRequest,
) ([]*models.WarehouseModel, *sqlrepo.Pagination32, error) {
	keywordGroup := sqlrepo.NewKeywordFilterGroup(req.GetKeyword().GetValue(), nil, []string{"name", "code"})

	mainGroup := sqlrepo.NewFilterGroup(sqlconstants.LOGIC_AND).
		AddFilter(sqlrepo.NewEqFilter("tenant_id", tenantID)).
		AddEqFilterIfNotEmpty("category", req.GetCategory()).
		AddEqFilterIfNotEmpty("status", req.GetStatus()).
		AddGroupIfNotEmpty(keywordGroup)

	q := sqlrepo.NewQuery().
		WithPaging(int(req.GetPageRequest().GetPage()), int(req.GetPageRequest().GetSize())).
		WithFilterGroup(mainGroup)

	q.AddSafeOrderFromSort(req.GetSort(), "sort_order", "ASC", WarehouseSortableFields)

	return r.ListWithPagination32(ctx, q)
}
