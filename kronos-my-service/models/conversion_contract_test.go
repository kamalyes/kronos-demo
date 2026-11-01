/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 18:10:33
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:30:27
 * @FilePath: \kronos-my-service\models\conversion_contract_test.go
 * @Description: Model-PB 转换契约测试，守住字段映射不随协议演进而断裂
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	pbmo "github.com/kamalyes/go-pbmo"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	"google.golang.org/protobuf/runtime/protoimpl"
	"gorm.io/gorm/schema"
)

// 为什么需要这组测试 | Why these tests exist
//
// pbmo 按字段名自动配对，配不上就静默跳过——这是它好用的原因，也是它最危险的地方：
// 协议中心把某个字段改了名，或者本地 model 改了名，转换不会报错，
// 那个字段只是从此恒为零值。接口照常返回 200，字段静悄悄变成空。
//
// 所以这里把每个注册过的类型对都填满非零值转一遍，断言 PB 一侧没有残留零值。
// 协议新增字段时这组测试会失败，提醒你在 model 上补同名字段——
// 这正是我们想要的：宁可现在红，不要线上静默丢字段。
//
// 不连数据库：转换是纯内存操作，没有理由让这组测试依赖外部环境。
//
// [EN] pbmo pairs fields by name and silently skips what it cannot pair. That is what makes it
// convenient and also what makes it dangerous: rename a field on either side and the conversion
// still succeeds while that field is zero from then on. The endpoint keeps returning 200 and the
// value quietly disappears. These tests therefore fill every registered pair with non-zero
// values and assert nothing on the PB side stayed zero. A new field in the contract makes them
// fail, which is the point: better red now than silently dropped in production. No database is
// involved, since conversion is pure in-memory work.

func init() {
	// 测试里也要注册：注册表是进程级的，而测试进程不会跑 bootstrap
	// [EN] Registration is process-wide and the test binary never runs bootstrap
	pbmo.Register[ProductModel, commonpb.ProductInfo]()
	pbmo.Register[WarehouseModel, commonpb.WarehouseInfo]()
	pbmo.Register[TenantModel, commonpb.TenantInfo]()
	pbmo.Register[TenantMemberModel, commonpb.TenantMemberInfo]()
	pbmo.Register[TenantRegionAssignmentModel, commonpb.TenantRegionAssignment]().
		WithFieldMappings(map[string]string{"IsPrimary": "Primary"})
}

// zeroFieldsOf 列出 PB 消息里仍为零值的导出字段
//
// 跳过 protobuf 自己的内部字段（state / sizeCache / unknownFields），
// 它们由 protoimpl 维护，与业务字段无关
// [EN] protobuf's internal bookkeeping fields are maintained by protoimpl and skipped
func zeroFieldsOf(msg any, allowed map[string]bool) []string {
	v := reflect.ValueOf(msg).Elem()
	t := v.Type()
	var zero []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Type == reflect.TypeOf(protoimpl.MessageState{}) {
			continue
		}
		if allowed[f.Name] {
			continue
		}
		if v.Field(i).IsZero() {
			zero = append(zero, f.Name)
		}
	}
	return zero
}

func TestConvertProductModelToPB(t *testing.T) {
	ctx := context.Background()
	m := &ProductModel{
		Id: "id-1", TenantId: "t-1", PlatformId: "p-1", RegionId: "r-1",
		Name: "name", Code: "code",
		Type:      enumspb.ProductType_PRODUCT_TYPE_PERSONAL,
		Level:     enumspb.ProductLevel_PRODUCT_LEVEL_SILVER,
		Status:    enumspb.CommonStatus_COMMON_STATUS_NORMAL,
		Tags:      []string{"a"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.SetContact(&commonpb.ProductContact{Email: "a@b.c"})
	m.SetExtra(map[string]string{"k": "v"})

	info, err := m.ToPB(ctx)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if zero := zeroFieldsOf(info, nil); len(zero) > 0 {
		t.Errorf("以下字段未被转换填充（协议改名或 model 字段名不一致）: %v", zero)
	}
}

func TestConvertWarehouseModelToPB(t *testing.T) {
	ctx := context.Background()
	m := &WarehouseModel{
		Id: "id-1", TenantId: "t-1", Name: "name", Code: "code",
		IconUrl: "https://example.com/i.png", SortOrder: 3,
		Category:  enumspb.WarehouseCategory_WAREHOUSE_CATEGORY_PREMIUM,
		Status:    enumspb.CommonStatus_COMMON_STATUS_NORMAL,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	info, err := m.ToPB(ctx)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if zero := zeroFieldsOf(info, nil); len(zero) > 0 {
		t.Errorf("以下字段未被转换填充: %v", zero)
	}
}

func TestConvertTenantModelToPB(t *testing.T) {
	ctx := context.Background()
	m := &TenantModel{
		Id: "id-1", Code: "code", Name: "name",
		Status:     enumspb.TenantStatus_TENANT_STATUS_NORMAL,
		SyncState:  enumspb.TenantSyncState_TENANT_SYNC_STATE_SYNCED,
		PlatformId: "p-1", RegionId: "r-1", MemberCount: 5,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	info, err := m.ToPB(ctx)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	// Regions 由调用方按需填充（详情接口才查子资源），不在转换范围内
	// [EN] Regions is filled on demand by the detail endpoint, not by the conversion
	if zero := zeroFieldsOf(info, map[string]bool{"Regions": true}); len(zero) > 0 {
		t.Errorf("以下字段未被转换填充: %v", zero)
	}
}

func TestConvertTenantMemberModelToPB(t *testing.T) {
	ctx := context.Background()
	m := &TenantMemberModel{
		Id: "id-1", TenantId: "t-1", UserId: "u-1",
		DisplayName: "someone", Email: "a@b.c",
		Status: enumspb.CommonStatus_COMMON_STATUS_NORMAL,
		Roles: TenantMemberRoles{
			enumspb.TenantMemberRole_TENANT_MEMBER_ROLE_OWNER,
		},
		JoinedAt: time.Now(),
	}
	info, err := m.ToPB(ctx)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if zero := zeroFieldsOf(info, nil); len(zero) > 0 {
		t.Errorf("以下字段未被转换填充: %v", zero)
	}
}

func TestConvertTenantRegionAssignmentToPB(t *testing.T) {
	ctx := context.Background()
	m := &TenantRegionAssignmentModel{
		Id: "id-1", TenantId: "t-1", RegionId: "r-1",
		IsPrimary: true, AssignedAt: time.Now(),
	}
	pb, err := m.ToPB(ctx)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	// 这里顺带守住 IsPrimary → Primary 的字段映射：名字对不上，
	// 靠 WithFieldMapping 或显式赋值补，漏掉就恒为 false
	// [EN] Guards the IsPrimary to Primary mapping, which is false forever if missed
	if !pb.GetPrimary() {
		t.Error("IsPrimary 未映射到 PB 的 primary 字段")
	}
	if zero := zeroFieldsOf(pb, nil); len(zero) > 0 {
		t.Errorf("以下字段未被转换填充: %v", zero)
	}
}

// TestTableNamesDistinct 表名不得重复
//
// 四个实体的表名由各自的 TableName() 返回，复制实体时最容易忘改这个方法——
// 两张表同名会让两个实体读写同一张表，而且不会有任何报错
// [EN] Table names come from each entity's TableName(), the method most often forgotten when an
// entity is copied. Two entities sharing a table read and write each other's rows with no error
func TestTableNamesDistinct(t *testing.T) {
	names := []string{
		ProductModel{}.TableName(),
		WarehouseModel{}.TableName(),
		TenantModel{}.TableName(),
		TenantMemberModel{}.TableName(),
		TenantRegionAssignmentModel{}.TableName(),
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if n == "" {
			t.Error("存在空表名")
			continue
		}
		if seen[n] {
			t.Errorf("表名重复: %s", n)
		}
		seen[n] = true
	}
}

// TestGormSchemaParses 每个实体都能被 GORM 解析成表结构
//
// 为什么要在测试里解析一遍：枚举字段用的是 enums.XxxType 这类具名类型，
// GORM 要靠 Kind 推断列类型。哪天某个字段换成了 GORM 认不出的类型
// （指针包装、没实现 Scanner/Valuer 的结构体），现象是服务启动时
// AutoMigrate 报 "unsupported data type"——那已经是部署之后了。
// 这里提前把这件事挪到测试阶段，而且不需要数据库。
//
// 顺带断言枚举列确实被映射出来了：字段被 GORM 忽略时不会报错，只是那一列不存在，
// 建表时静默少一列，写入时那个值无处可去。
//
// [EN] Enum fields use named types such as enums.XxxType and GORM infers the column type from
// their kind. If a field is changed to something GORM cannot map, the symptom is AutoMigrate
// failing with "unsupported data type" at service start, which is already after deployment.
// This moves that failure into the test phase and needs no database. It also asserts the enum
// columns are actually mapped, because an ignored field raises no error: the column simply does
// not exist, so the table is built one column short and the value has nowhere to go.
func TestGormSchemaParses(t *testing.T) {
	cases := []struct {
		entity  any
		columns []string
	}{
		{&ProductModel{}, []string{"type", "level", "status", "tags", "extra", "contact"}},
		{&WarehouseModel{}, []string{"category", "status"}},
		{&TenantModel{}, []string{"status", "sync_state"}},
		{&TenantMemberModel{}, []string{"status", "roles"}},
		{&TenantRegionAssignmentModel{}, []string{"is_primary"}},
	}

	cache := &sync.Map{}
	namer := schema.NamingStrategy{}
	for _, tc := range cases {
		sch, err := schema.Parse(tc.entity, cache, namer)
		if err != nil {
			t.Errorf("%T 无法被 GORM 解析: %v", tc.entity, err)
			continue
		}
		for _, col := range tc.columns {
			field := sch.LookUpField(col)
			if field == nil {
				t.Errorf("%T: 列 %q 没有被映射（字段被 GORM 忽略了）", tc.entity, col)
				continue
			}
			if field.DataType == "" {
				t.Errorf("%T: 列 %q 推断不出数据类型", tc.entity, col)
			}
		}
	}
}
