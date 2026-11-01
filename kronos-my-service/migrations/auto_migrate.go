/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\migrations\auto_migrate.go
 * @Description: 数据库自动迁移 基于 go-sqlbuilder Migrator
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package migrations

import (
	"github.com/kamalyes/go-sqlbuilder/db"
	"github.com/kamalyes/kronos-my-service/models"
	"gorm.io/gorm"
)

// GetAllModels 获取所有需要初始化的数据库模型
func GetAllModels() []interface{} {
	return []interface{}{
		&models.ProductModel{},
		&models.WarehouseModel{},
		&models.TenantModel{},
		&models.TenantRegionAssignmentModel{},
		&models.TenantMemberModel{},
	}
}

// GetAllIndexes 获取所有需要创建的复合索引
func GetAllIndexes() []db.IndexDefinition {
	return []db.IndexDefinition{
		{
			Table: models.TableNameProduct,
			Name:  "idx_products_tenant_code",
			// Columns 在 go-sqlbuilder v0.7 起是「列定义字符串」而非 []string：
			// 这样才能表达 "name DESC"、"(tenant_id, name)" 这类形式。
			// [EN] Since go-sqlbuilder v0.7 Columns is a column DEFINITION string rather than
			// a []string, so forms like "name DESC" or "(tenant_id, name)" can be expressed.
			// 编码在租户内唯一：协议中心的 ProductCreateRequest 里
			// code 是必填且带 min/max 约束，业务语义上是租户内的业务主键
			// [EN] Code is unique per tenant: the hub's create request marks it required
			Columns: "(tenant_id, code)",
			Unique:  true,
		},
	}
}

// GetAllComments 获取所有表注释
func GetAllComments() []db.TableComment {
	return []db.TableComment{
		{
			Table:   models.TableNameProduct,
			Comment: models.ProductModel{}.TableComment(),
		},
	}
}

// GetMigratorConfig 获取迁移器配置
func GetMigratorConfig() *db.MigratorConfig {
	return &db.MigratorConfig{
		Models:             GetAllModels(),
		Indexes:            GetAllIndexes(),
		Comments:           GetAllComments(),
		SkipIndexOnError:   true,
		SkipCommentOnError: true,
	}
}

// AutoMigrate 自动迁移所有数据表
func AutoMigrate(gormDB *gorm.DB) error {
	migrator := db.NewMigrator(gormDB, GetMigratorConfig())
	return migrator.AutoMigrate()
}

// CreateIndexes 创建额外的索引
func CreateIndexes(gormDB *gorm.DB) error {
	migrator := db.NewMigrator(gormDB, &db.MigratorConfig{
		Indexes:          GetAllIndexes(),
		SkipIndexOnError: true,
	})
	return migrator.CreateIndexes()
}

// AddTableComments 添加表注释
func AddTableComments(gormDB *gorm.DB) error {
	migrator := db.NewMigrator(gormDB, &db.MigratorConfig{
		Comments:           GetAllComments(),
		SkipCommentOnError: true,
	})
	return migrator.AddComments()
}
