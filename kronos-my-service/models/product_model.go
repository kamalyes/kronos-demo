/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\product_model.go
 * @Description: Product 数据模型，字段与协议中心的 ProductInfo 对齐
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	"context"
	"time"

	bizerror "github.com/kamalyes/go-bizerror"
	sqlbuilder "github.com/kamalyes/go-sqlbuilder"
	sqltypes "github.com/kamalyes/go-sqlbuilder/types"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
)

// ProductModel 对应协议中心 common.ProductInfo 的落库形态
//
// 字段名与 PB 一侧保持一致（pbmo 按名称不分大小写自动配对），这样新增字段时
// 两边同名即可，不必手写赋值。类型两侧同构的字段由 pbmo 直接搬运，包括
// time.Time ↔ *timestamppb.Timestamp（pbmo 内建该方向的转换）。
//
// 三个 JSON 列的类型两侧不同构，pbmo 的字段计划配不上，由 ToPB / ApplyPB 显式处理：
//
//	Contact  PB 是 *commonpb.ProductContact，落库用 types.ProtoJSON 包一层
//	         （ProtoJSON 只在 go-sqlbuilder/types 子包，根包未重导出，故单独 import）
//	Tags     PB 是 []string，落库用 StringSlice（命名切片，自带 Valuer/Scanner）
//	Extra    PB 是 map[string]string，落库用 JSON[map[string]string]
//
// [EN] Field names mirror the PB side so pbmo pairs them automatically (case-insensitive);
// adding a field means naming it the same on both sides, with no hand-written assignment.
// Three JSON columns are not structurally identical across the boundary, so ToPB / ApplyPB
// handle them explicitly.
type ProductModel struct {
	Id         string `gorm:"column:id;type:varchar(36);primaryKey;comment:主键ID" json:"id"`
	TenantId   string `gorm:"column:tenant_id;type:varchar(36);not null;index:idx_products_tenant;comment:租户ID" json:"tenant_id"`
	PlatformId string `gorm:"column:platform_id;type:varchar(36);comment:平台ID" json:"platform_id"`
	RegionId   string `gorm:"column:region_id;type:varchar(36);comment:地区ID" json:"region_id"`
	Name       string `gorm:"column:name;type:varchar(64);not null;comment:名称" json:"name"`
	Code       string `gorm:"column:code;type:varchar(32);not null;comment:编码" json:"code"`

	// 枚举落库存数值：枚举名会随 proto 调整，数值由 proto 的字段号固定下来
	// [EN] Enums are stored numerically: names change with the proto, numbers are pinned by it
	Type   enumspb.ProductType  `gorm:"column:type;type:int;default:0;not null;comment:类型" json:"type"`
	Level  enumspb.ProductLevel `gorm:"column:level;type:int;default:0;not null;comment:等级" json:"level"`
	Status enumspb.CommonStatus `gorm:"column:status;type:int;default:0;not null;index:idx_products_status;comment:状态" json:"status"`

	Contact sqltypes.ProtoJSON[*commonpb.ProductContact] `gorm:"column:contact;type:json;comment:联系方式" json:"contact"`
	Tags    sqlbuilder.StringSlice                       `gorm:"column:tags;type:json;comment:标签列表" json:"tags"`
	Extra   sqlbuilder.JSON[map[string]string]           `gorm:"column:extra;type:json;comment:扩展字段" json:"extra"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;comment:更新时间" json:"updated_at"`
}

// TableName 指定表名
func (ProductModel) TableName() string {
	return "products"
}

// GetID 实现 repository.TenantScoped，供泛型仓储定位缓存键
func (m *ProductModel) GetID() string { return m.Id }

// GetTenantID 实现 repository.TenantScoped，供泛型仓储校验归属
func (m *ProductModel) GetTenantID() string { return m.TenantId }

// TableComment 指定表注释
func (ProductModel) TableComment() string {
	return "Product表"
}

// ToPB 转换为协议中心的 ProductInfo
//
// 走 bizerror.ToPB 而不是 pbmo.ToPB：转换失败会被归并成
// error.conversion_failed 这个业务错误码，带 i18n 文案并映射到 gRPC Internal，
// 不必在每个调用点自己包一层。
//
// [EN] Uses bizerror.ToPB rather than pbmo.ToPB so a failure becomes the
// error.conversion_failed business code, already localized and mapped to gRPC Internal.
func (m *ProductModel) ToPB(ctx context.Context) (*commonpb.ProductInfo, error) {
	info, err := bizerror.ToPB[ProductModel, commonpb.ProductInfo](ctx, m)
	if err != nil {
		return nil, err
	}
	info.Contact = m.Contact.Data
	info.Tags = m.Tags
	info.Extra = m.Extra.Data
	return info, nil
}

// ToPBs 批量转换
func ToPBsProduct(ctx context.Context, items []*ProductModel) ([]*commonpb.ProductInfo, error) {
	out := make([]*commonpb.ProductInfo, 0, len(items))
	for _, it := range items {
		info, err := it.ToPB(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

// SetContact 写入联系方式（nil 表示清空）
func (m *ProductModel) SetContact(c *commonpb.ProductContact) {
	m.Contact = sqltypes.ProtoJSON[*commonpb.ProductContact]{Data: c}
}

// SetExtra 写入扩展字段
func (m *ProductModel) SetExtra(extra map[string]string) {
	m.Extra = sqlbuilder.JSON[map[string]string]{Data: extra}
}
