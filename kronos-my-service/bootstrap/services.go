/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\services.go
 * @Description: 服务初始化和依赖注入
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	"context"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-my-service/repository"
	"github.com/kamalyes/kronos-my-service/service"
	catalogsvc "github.com/kamalyes/kronos-my-service/service/catalog"
	inventorysvc "github.com/kamalyes/kronos-my-service/service/inventory"
	tenantsvc "github.com/kamalyes/kronos-my-service/service/tenant"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/catalog"
	secondpb "github.com/kamalyes/kronos-share-proto-template/pb/inventory"
	mixedpb "github.com/kamalyes/kronos-share-proto-template/pb/tenant"
	"google.golang.org/grpc"
)

// initServices 初始化所有服务
func (s *MyServiceGateway) initServices() {
	gwglobal.LOGGER.Info("Initializing services...")

	// 转换器先注册：registerConverters 会按反射构建字段计划并缓存，
	// 放到开始服务之后注册，第一个请求就要替所有人付这笔反射开销
	// [EN] Register converters first; doing it after serving starts makes the first request
	// pay the reflection cost for everyone
	registerConverters()

	s.repo = repository.NewRepositoryFactory()
	s.svc = service.NewMyServiceService(s.repo)
	s.exampleSvc = catalogsvc.NewProductService(s.repo.Product)
	s.secondSvc = inventorysvc.NewWarehouseService(s.repo.Warehouse)
	s.mixedSvc = tenantsvc.NewTenantService(s.repo.Tenant)
	s.mixedMemberSvc = tenantsvc.NewTenantMemberService(s.repo.TenantMember)

	gwglobal.LOGGER.Info("Services initialized")
}

// registerGRPCServices 注册gRPC服务
func (s *MyServiceGateway) registerGRPCServices() {
	s.gateway.RegisterService(func(srv *grpc.Server) {
		pb.RegisterProductServiceServer(srv, s.exampleSvc)
		secondpb.RegisterWarehouseServiceServer(srv, s.secondSvc)
		mixedpb.RegisterTenantServiceServer(srv, s.mixedSvc)
		mixedpb.RegisterTenantMemberServiceServer(srv, s.mixedMemberSvc)
	})
	gwglobal.LOGGER.Info("gRPC services registered")
}

// registerHTTPHandlers 注册HTTP处理器
func (s *MyServiceGateway) registerHTTPHandlers() {
	// 用 HandlerServer 而不是 HandlerFromEndpoint：前者把 HTTP 请求直接交给同进程的
	// 实现，不绕回 gRPC 端口，省掉一次本机网络往返与一次序列化
	// [EN] HandlerServer dispatches HTTP straight to the in-process implementation instead of
	// dialing back into the gRPC port, saving a loopback round trip and one serialization
	if err := s.gateway.RegisterGatewayHandler(func(ctx context.Context, mux *runtime.ServeMux) error {
		if err := pb.RegisterProductServiceHandlerServer(ctx, mux, s.exampleSvc); err != nil {
			return err
		}
		if err := secondpb.RegisterWarehouseServiceHandlerServer(ctx, mux, s.secondSvc); err != nil {
			return err
		}
		if err := mixedpb.RegisterTenantServiceHandlerServer(ctx, mux, s.mixedSvc); err != nil {
			return err
		}
		return mixedpb.RegisterTenantMemberServiceHandlerServer(ctx, mux, s.mixedMemberSvc)
	}); err != nil {
		gwglobal.LOGGER.Fatal("注册 HTTP handler 失败 | failed to register HTTP handler: %v", err)
	}

	gwglobal.LOGGER.Info("HTTP handlers registered")
}
