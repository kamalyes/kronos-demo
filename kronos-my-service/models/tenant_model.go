/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\tenant_model.go
 * @Description: Tenant 聚合根与子资源模型
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

// TenantModel 对应协议中心 common.TenantInfo
//
// 这个实体和前两个有本质区别，值得对照着看：
//
//  1. 它自己就是租户，没有 tenant_id 列，code 是**全局唯一**而非租户内唯一。
//     所以它不能套用 CachedRepository 那套「按 tenant_id 过滤」的方法——
//     那前提在这里不成立，硬套会写出 WHERE tenant_id = ” 这种恒不命中的条件。
//  2. member_count 是聚合统计，不落列：成员数会随成员表变化，落成列就要在每次
//     增删成员时同步维护，漏一处就长期对不上。改为查询时用子查询算（见仓库的
//     AddComputedField），代价是多一次子查询，换来的是不会出现脏计数。
//  3. regions 是子资源，有独立 ID、要单独增改删，所以是另一张表
//     （TenantRegionAssignmentModel），不是 JSON 列。
//     判断标准：需要单独寻址的就建表，只读的附属数据才用 JSON。
//
// [EN] This entity differs fundamentally from the previous two. It IS the tenant, so it has no
// tenant_id column and its code is globally unique; the tenant-scoped helpers do not apply.
// member_count is an aggregate and is not stored as a column, because a stored counter has to
// be maintained on every membership change and drifts the moment one path forgets; it is
// computed with a subquery instead. regions is a sub-resource with its own id and its own
// create/update/delete, so it is a separate table rather than a JSON column: anything that
// needs addressing individually gets a table, read-only attached data gets JSON.
type TenantModel struct {
	Id         string                  `gorm:"column:id;type:varchar(36);primaryKey;comment:主键ID" json:"id"`
	Code       string                  `gorm:"column:code;type:varchar(32);not null;uniqueIndex:uk_tenants_code;comment:编码（全局唯一）" json:"code"`
	Name       string                  `gorm:"column:name;type:varchar(64);not null;comment:名称" json:"name"`
	Status     enumspb.TenantStatus    `gorm:"column:status;type:int;default:0;not null;index:idx_tenants_status;comment:状态" json:"status"`
	SyncState  enumspb.TenantSyncState `gorm:"column:sync_state;type:int;default:0;not null;comment:下游同步状态" json:"sync_state"`
	PlatformId string                  `gorm:"column:platform_id;type:varchar(36);not null;comment:平台ID" json:"platform_id"`
	RegionId   string                  `gorm:"column:region_id;type:varchar(36);comment:主地区ID" json:"region_id"`

	// MemberCount 不落列，由查询时的子查询填充（gorm:"-" 排除出表结构）
	// [EN] Not a column; filled by a subquery at query time
	MemberCount int32 `gorm:"->;column:member_count" json:"member_count"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;comment:更新时间" json:"updated_at"`
}

// TableName 指定表名
func (TenantModel) TableName() string {
	return "tenants"
}

// TableComment 指定表注释
func (TenantModel) TableComment() string {
	return "Tenant表"
}

// ToPB 转换为协议中心的 TenantInfo
//
// regions 由调用方按需填充：列表接口不查子资源（N+1），详情接口才查
// [EN] regions is filled by the caller on demand: the list endpoint does not fetch the
// sub-resource (N+1), only the detail endpoint does
func (m *TenantModel) ToPB(ctx context.Context) (*commonpb.TenantInfo, error) {
	return bizerror.ToPB[TenantModel, commonpb.TenantInfo](ctx, m)
}

// ToPBsTenant 批量转换
func ToPBsTenant(ctx context.Context, items []*TenantModel) ([]*commonpb.TenantInfo, error) {
	return bizerror.ToPBs[TenantModel, commonpb.TenantInfo](ctx, items)
}

// TenantRegionAssignmentModel 地区分配，Tenant 的子资源
//
// 字段名与 common.TenantRegionAssignment 对齐；
// Primary 在 SQL 里是保留字，列名加后缀避开，靠 pbmo 的字段映射对回 PB 的 primary
// [EN] Primary is a reserved word in SQL, so the column is suffixed and mapped back to the
// PB field by pbmo
type TenantRegionAssignmentModel struct {
	Id         string    `gorm:"column:id;type:varchar(36);primaryKey;comment:分配ID" json:"id"`
	TenantId   string    `gorm:"column:tenant_id;type:varchar(36);not null;index:idx_tenant_regions_tenant;comment:所属TenantID" json:"tenant_id"`
	RegionId   string    `gorm:"column:region_id;type:varchar(36);not null;comment:地区ID" json:"region_id"`
	IsPrimary  bool      `gorm:"column:is_primary;default:false;not null;comment:是否主地区" json:"is_primary"`
	AssignedAt time.Time `gorm:"column:assigned_at;autoCreateTime;comment:分配时间" json:"assigned_at"`
}

// TableName 指定表名
func (TenantRegionAssignmentModel) TableName() string {
	return "tenant_region_assignments"
}

// TableComment 指定表注释
func (TenantRegionAssignmentModel) TableComment() string {
	return "Tenant地区分配表"
}

// GetID 实现 repository.TenantScoped
func (m *TenantRegionAssignmentModel) GetID() string { return m.Id }

// GetTenantID 实现 repository.TenantScoped
func (m *TenantRegionAssignmentModel) GetTenantID() string { return m.TenantId }

// ToPB 转换为协议中心的地区分配消息
//
// IsPrimary → primary 的名字对不上，pbmo 的自动配对接不上，这里显式赋值；
// 也可以在 Register 时用 WithFieldMapping("IsPrimary", "Primary") 声明，
// 两种都行，字段少时直接赋值更直白
// [EN] IsPrimary and primary do not match by name, so the value is assigned explicitly here;
// declaring WithFieldMapping at registration works equally well
func (m *TenantRegionAssignmentModel) ToPB(ctx context.Context) (*commonpb.TenantRegionAssignment, error) {
	pb, err := bizerror.ToPB[TenantRegionAssignmentModel, commonpb.TenantRegionAssignment](ctx, m)
	if err != nil {
		return nil, err
	}
	pb.Primary = m.IsPrimary
	return pb, nil
}

// ToPBsTenantRegionAssignment 批量转换
func ToPBsTenantRegionAssignment(
	ctx context.Context, items []*TenantRegionAssignmentModel,
) ([]*commonpb.TenantRegionAssignment, error) {
	out := make([]*commonpb.TenantRegionAssignment, 0, len(items))
	for _, it := range items {
		pb, err := it.ToPB(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, pb)
	}
	return out, nil
}
