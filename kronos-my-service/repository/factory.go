/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\repository\factory.go
 * @Description: 仓库工厂，统一管理所有仓库实例
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package repository

import (
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/go-sqlbuilder/db"
	sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
	"sync"
)

// RepositoryFactory 仓库工厂结构体
type RepositoryFactory struct {
	Product      *ProductRepository
	Warehouse    *WarehouseRepository
	Tenant       *TenantRepository
	TenantMember *TenantMemberRepository
}

var (
	factoryInstance *RepositoryFactory
	factoryOnce     sync.Once
)

// NewRepositoryFactory 创建仓库工厂单例
func NewRepositoryFactory() *RepositoryFactory {
	factoryOnce.Do(func() {
		factoryInstance = &RepositoryFactory{
			Product:      NewProductRepository(),
			Warehouse:    NewWarehouseRepository(),
			Tenant:       NewTenantRepository(),
			TenantMember: NewTenantMemberRepository(),
		}
	})
	return factoryInstance
}

// newBaseRepository 构造通用 BaseRepository 实例
func newBaseRepository[T any](tableName string) *sqlrepo.BaseRepository[T] {
	dbHandler := db.MustNewGormHandler(gwglobal.DB)
	return sqlrepo.NewBaseRepository[T](
		dbHandler,
		gwglobal.GetLogger(),
		tableName,
		sqlrepo.WithAutoFields[T](),
	)
}
