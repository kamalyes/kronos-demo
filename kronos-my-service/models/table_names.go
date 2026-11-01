/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\table_names.go
 * @Description: 数据库表名常量定义
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

// 数据库表名常量 - 通过调用模型的 TableName() 方法动态获取
var (
	TableNameProduct                = ProductModel{}.TableName()
	TableNameWarehouse              = WarehouseModel{}.TableName()
	TableNameTenant                 = TenantModel{}.TableName()
	TableNameTenantRegionAssignment = TenantRegionAssignmentModel{}.TableName()
	TableNameTenantMember           = TenantMemberModel{}.TableName()
)
