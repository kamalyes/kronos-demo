/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\pbmo.go
 * @Description: Model-PB 转换器注册
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	pbmo "github.com/kamalyes/go-pbmo"
	"github.com/kamalyes/kronos-my-service/models"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
)

// registerConverters 注册所有 Model-PB 转换器
//
// 必须在服务开始处理请求前注册：pbmo 在 Register 时按反射构建字段搬运计划并缓存，
// 之后每次 ToPB 走的是预编译好的闭包。没注册过的类型对在首次转换时才构建，
// 那一次会落在请求路径上。
//
// 字段名两侧一致即自动配对（不分大小写）。名字对不上时显式声明：
//
//	单条 .WithFieldMapping("ID", "XxxId")
//	多条 .WithFieldMappings(map[string]string{"ID": "XxxId", "CreatedBy": "CreatorId"})
//
// 值本身需要换算（枚举口径不同、单位不同）用 .RegisterTransformer(field, fn)，
// 那是按字段挂一个转换函数，和改名是两件事。
//
// [EN] Register before the service starts serving: pbmo builds and caches a field-copy plan
// via reflection at registration time, and later conversions run the precompiled closure. An
// unregistered pair is built on first use, which lands that cost on a request. Fields pair
// automatically by case-insensitive name; use WithFieldMapping when the names differ.
func registerConverters() {
	pbmo.Register[models.ProductModel, commonpb.ProductInfo]()
	pbmo.Register[models.WarehouseModel, commonpb.WarehouseInfo]()
	pbmo.Register[models.TenantModel, commonpb.TenantInfo]()
	pbmo.Register[models.TenantMemberModel, commonpb.TenantMemberInfo]()
	// 名字对不上的字段要显式声明，否则 pbmo 静默跳过、该字段恒为零值。
	// 两种写法：单条用 WithFieldMapping(model, pb)，多条用 WithFieldMappings(map)——
	// 后者一次把整张映射表交出去，加一条就是加一行，不会在链式调用里越接越长。
	// 两者可以叠加，后者不会清掉前面已声明的映射。
	//
	// [EN] A field whose names differ must be declared, or pbmo silently skips it and the field
	// stays zero forever. WithFieldMapping takes one pair; WithFieldMappings takes the whole
	// table at once, so adding a rename is one more line rather than one more chained call.
	// The two combine, and the plural form does not clear mappings already declared.
	pbmo.Register[models.TenantRegionAssignmentModel, commonpb.TenantRegionAssignment]().
		WithFieldMappings(map[string]string{
			"IsPrimary": "Primary",
			// "ModelFieldName": "PBFieldName",
		})
}
