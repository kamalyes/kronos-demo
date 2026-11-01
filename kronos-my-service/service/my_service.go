/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\my_service.go
 * @Description: MyService 服务实现
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package service

import (
	"github.com/kamalyes/kronos-my-service/repository"
)

// MyServiceServiceImpl 服务实现
type MyServiceServiceImpl struct {
	// UnimplementedMyServiceServiceServer
	repo *repository.RepositoryFactory
}

// NewMyServiceService 创建服务实例
func NewMyServiceService(repo *repository.RepositoryFactory) *MyServiceServiceImpl {
	return &MyServiceServiceImpl{
		repo: repo,
	}
}
