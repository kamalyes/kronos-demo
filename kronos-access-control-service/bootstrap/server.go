/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 00:00:00
 * @FilePath: \kronos-access-control-service\bootstrap\server.go
 * @Description: 服务启动引导
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	"fmt"

	gateway "github.com/kamalyes/go-rpc-gateway"
	goconfig "github.com/kamalyes/go-rpc-gateway/config"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-access-control-service/constants"
	authsvc "github.com/kamalyes/kronos-access-control-service/service/auth"
	"google.golang.org/grpc"
)

// AccessControlServiceGateway 服务启动器
type AccessControlServiceGateway struct {
	gateway *gateway.Gateway
	authSvc *authsvc.AuthService
}

// NewAccessControlServiceGateway 创建服务启动器实例
func NewAccessControlServiceGateway() *AccessControlServiceGateway {
	return &AccessControlServiceGateway{}
}

// Run 启动服务
func (s *AccessControlServiceGateway) Run() error {
	gw, err := gateway.NewGateway().
		WithSearchPath("resources").
		WithEnvironment(goconfig.GetEnvironment()).
		WithPrefix(constants.ServiceName).
		WithHotReload(nil).
		Build()
	if err != nil {
		return fmt.Errorf("create gateway failed: %v", err)
	}
	s.gateway = gw

	configureLoggerContext()

	// ========================================================================
	// 桩实现禁止在生产级别环境启动
	// ========================================================================
	//
	// service/auth 里的实现不验签、不查吊销、不看过期，任何字符串都算有效 token。
	// 它上生产等于把整套鉴权拿掉，而且现象是**一切正常**——所有接口都通，
	// 所有身份都"合法"，监控上看不出任何异常。
	//
	// 所以这里是硬失败，不是警告。警告会被忽略：它只在启动时打一次，
	// 之后几个月的日志里都没有任何痕迹，而发现问题时已经跑了很久。
	//
	// 判断用 IsProductionLevel（生产级别及以上）而不是 IsProduction（精确匹配
	// production 这一个名字）：新增 release / live 之类的别名时，
	// 精确匹配会漏掉它，于是桩在一个真实的生产环境里悄悄跑起来。
	//
	// 真实实现替换掉 service/auth 之后，把这一段删掉。
	//
	// [EN] The implementation in service/auth verifies no signature, no revocation and no
	// expiry, so any string is a valid token. In production that removes authorization
	// altogether while everything appears normal: every endpoint works, every identity is
	// valid and no monitor shows anything. This is therefore a hard failure rather than a
	// warning, which would be ignored — a warning prints once at startup and leaves no trace in
	// months of logs, by which time it has been running a long time. IsProductionLevel rather
	// than IsProduction, because a new alias such as release or live slips past an exact match
	// and quietly runs the stub in a real production environment. Delete this block once
	// service/auth is replaced.
	if goconfig.IsProductionLevel() {
		return fmt.Errorf(
			"拒绝启动：service/auth 是不做任何校验的桩实现，不能用于生产级别环境"+
				"（当前环境 %s）。替换成真实实现后删除 bootstrap/server.go 里的这段检查"+
				" | refusing to start: service/auth is a stub that validates nothing and must not"+
				" run at production level (current environment %s)",
			goconfig.GetCurrentEnvironment().String(), goconfig.GetCurrentEnvironment().String())
	}

	gwglobal.LOGGER.Warn("⚠️  鉴权为桩实现：不验签、不查吊销、任何 token 都放行" +
		" | authorization is a stub: no signature, no revocation, every token passes")

	gwglobal.LOGGER.Info("Initializing services...")
	s.authSvc = authsvc.NewAuthService()

	// 只注册 gRPC，不注册 HTTP handler
	//
	// Authorities 在协议里刻意没有 google.api.http 注解（见协议中心的说明）：
	// 它是网关专用的内部 RPC。给它 HTTP 路由等于提供一个「拿 token 换身份」
	// 的公开端点，且绕过所有业务鉴权。
	//
	// 所以这个服务连 RebuildHTTPGateway 都不需要——它没有任何 HTTP 面。
	//
	// [EN] gRPC only. Authorities deliberately carries no http annotation: it is the gateway's
	// internal RPC, and a route would be a public token-for-identity endpoint that bypasses
	// every business check. This service therefore needs no HTTP gateway rebuild at all.
	gwglobal.LOGGER.Info("Registering gRPC services...")
	s.gateway.RegisterService(func(server *grpc.Server) {
		pbRegister(server, s.authSvc)
	})

	gwglobal.LOGGER.Info("AccessControlService Server is ready (gRPC only)")
	return s.gateway.Run()
}
