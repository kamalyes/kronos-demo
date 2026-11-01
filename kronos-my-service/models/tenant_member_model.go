/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\tenant_member_model.go
 * @Description: TenantMember 成员模型
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	"context"
	"time"

	bizerror "github.com/kamalyes/go-bizerror"
	sqlbuilder "github.com/kamalyes/go-sqlbuilder"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
)

// TenantMemberRoles 角色集合，落库为 JSON 数组
//
// 角色是「一个成员多个角色」的重复枚举，有三种落库方式：
//
//	另开一张 member_roles 关联表 —— 需要按角色反查成员时才值得
//	每个角色一列的布尔位 —— 加角色就要改表结构，不可取
//	JSON 数组 —— 本例选它：角色集合总是跟着成员整体读写，没有单独寻址的需求
//
// [EN] Roles are a repeated enum. A join table is worth it only when members must be looked up
// by role; a boolean column per role means a schema change per new role. A JSON array is used
// here because the set is always read and written together with the member.
type TenantMemberRoles = sqlbuilder.Slice[enumspb.TenantMemberRole]

// TenantMemberModel 对应协议中心 common.TenantMemberInfo
type TenantMemberModel struct {
	Id          string               `gorm:"column:id;type:varchar(36);primaryKey;comment:成员ID" json:"id"`
	TenantId    string               `gorm:"column:tenant_id;type:varchar(36);not null;index:idx_tenant_members_tenant;comment:所属TenantID" json:"tenant_id"`
	UserId      string               `gorm:"column:user_id;type:varchar(36);not null;comment:用户ID" json:"user_id"`
	DisplayName string               `gorm:"column:display_name;type:varchar(64);comment:显示名" json:"display_name"`
	Email       string               `gorm:"column:email;type:varchar(128);comment:邮箱" json:"email"`
	Roles       TenantMemberRoles    `gorm:"column:roles;type:json;comment:角色列表" json:"roles"`
	Status      enumspb.CommonStatus `gorm:"column:status;type:int;default:0;not null;comment:状态" json:"status"`
	JoinedAt    time.Time            `gorm:"column:joined_at;autoCreateTime;comment:加入时间" json:"joined_at"`
}

// TableName 指定表名
func (TenantMemberModel) TableName() string {
	return "tenant_members"
}

// TableComment 指定表注释
func (TenantMemberModel) TableComment() string {
	return "Tenant成员表"
}

// GetID 实现 repository.TenantScoped
func (m *TenantMemberModel) GetID() string { return m.Id }

// GetTenantID 实现 repository.TenantScoped
func (m *TenantMemberModel) GetTenantID() string { return m.TenantId }

// SetRoles 写入角色集合，入参直接是 PB 的枚举切片
//
// 落库类型就是 Slice[enums.TenantMemberRole]，与 PB 同构，
// 所以这里是一次类型转换而不是逐元素搬运——不要把枚举退化成 int32 存，
// 那样读出来还得转回去，而且落库的 JSON 里只剩数字，看日志时无从对照
//
// [EN] The stored type is Slice of the same enum as the PB, so this is one conversion rather
// than an element-by-element copy. Storing the enum as int32 would force a conversion back on
// read and leave only bare numbers in the stored JSON
func (m *TenantMemberModel) SetRoles(roles []enumspb.TenantMemberRole) {
	m.Roles = TenantMemberRoles(roles)
}

// ToPB 转换为协议中心的成员消息
//
// Roles 两侧同构（Slice[T] 的底层类型就是 []T，元素都是同一个枚举），pbmo 自动搬运，
// 不需要在这里手工转——conversion_contract_test.go 里断言了这一点，
// 哪天协议把角色枚举换了名字，那个测试会先红
//
// [EN] Roles is structurally identical on both sides, since Slice[T] is defined as []T over the
// same enum, so pbmo copies it automatically. The contract test asserts this, and it is what
// turns a renamed enum in the contract into a test failure instead of a silently empty field
func (m *TenantMemberModel) ToPB(ctx context.Context) (*commonpb.TenantMemberInfo, error) {
	return bizerror.ToPB[TenantMemberModel, commonpb.TenantMemberInfo](ctx, m)
}

// ToPBsTenantMember 批量转换
func ToPBsTenantMember(
	ctx context.Context, items []*TenantMemberModel,
) ([]*commonpb.TenantMemberInfo, error) {
	out := make([]*commonpb.TenantMemberInfo, 0, len(items))
	for _, it := range items {
		info, err := it.ToPB(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}
