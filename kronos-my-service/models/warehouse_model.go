/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\warehouse_model.go
 * @Description: Warehouse 数据模型，字段与协议中心的 WarehouseInfo 对齐
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	"context"
	"time"

	bizerror "github.com/kamalyes/go-bizerror"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
)

// WarehouseModel 对应协议中心 common.WarehouseInfo
//
// 与 ProductModel 的区别：这个实体没有嵌套消息与 JSON 列，
// 两侧字段全部同构，所以 ToPB 整条交给 pbmo，不需要任何显式补字段——
// 这是大多数实体的常态，Product 那种才是需要额外处理的少数。
//
// [EN] Unlike ProductModel this entity has no nested messages or JSON columns, so
// every field is structurally identical across the boundary and ToPB defers entirely to pbmo
// with nothing to fill in by hand. That is the common case; the other model is the exception.
type WarehouseModel struct {
	Id        string                    `gorm:"column:id;type:varchar(36);primaryKey;comment:主键ID" json:"id"`
	TenantId  string                    `gorm:"column:tenant_id;type:varchar(36);not null;index:idx_warehouses_tenant;comment:租户ID" json:"tenant_id"`
	Name      string                    `gorm:"column:name;type:varchar(64);not null;comment:名称" json:"name"`
	Code      string                    `gorm:"column:code;type:varchar(32);not null;comment:编码" json:"code"`
	IconUrl   string                    `gorm:"column:icon_url;type:varchar(512);comment:图标地址" json:"icon_url"`
	Category  enumspb.WarehouseCategory `gorm:"column:category;type:int;default:0;not null;comment:分类" json:"category"`
	Status    enumspb.CommonStatus      `gorm:"column:status;type:int;default:0;not null;index:idx_warehouses_status;comment:状态" json:"status"`
	SortOrder int32                     `gorm:"column:sort_order;type:int;default:0;not null;comment:排序序号" json:"sort_order"`
	CreatedAt time.Time                 `gorm:"column:created_at;autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt time.Time                 `gorm:"column:updated_at;autoUpdateTime;comment:更新时间" json:"updated_at"`
}

// TableName 指定表名
func (WarehouseModel) TableName() string {
	return "warehouses"
}

// TableComment 指定表注释
func (WarehouseModel) TableComment() string {
	return "Warehouse表"
}

// GetID 实现 repository.TenantScoped
func (m *WarehouseModel) GetID() string { return m.Id }

// GetTenantID 实现 repository.TenantScoped
func (m *WarehouseModel) GetTenantID() string { return m.TenantId }

// ToPB 转换为协议中心的 WarehouseInfo
func (m *WarehouseModel) ToPB(ctx context.Context) (*commonpb.WarehouseInfo, error) {
	return bizerror.ToPB[WarehouseModel, commonpb.WarehouseInfo](ctx, m)
}

// ToPBsWarehouse 批量转换
//
// 直接用 bizerror.ToPBs：整个切片一次转换，内部复用同一份字段计划，
// 不像逐条调用那样每次都走一遍类型断言
// [EN] bizerror.ToPBs converts the whole slice reusing one field plan
func ToPBsWarehouse(ctx context.Context, items []*WarehouseModel) ([]*commonpb.WarehouseInfo, error) {
	return bizerror.ToPBs[WarehouseModel, commonpb.WarehouseInfo](ctx, items)
}
