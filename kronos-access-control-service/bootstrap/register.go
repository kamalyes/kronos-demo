/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 00:00:00
 * @FilePath: \kronos-access-control-service\bootstrap\register.go
 * @Description: gRPC 服务注册
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	authsvc "github.com/kamalyes/kronos-access-control-service/service/auth"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/access_control"
	"google.golang.org/grpc"
)

// pbRegister 把服务挂到 gRPC server 上
//
// 单独拆一个函数只为把协议包的 import 收在一处：server.go 里就不必同时
// 引入 pb 和 service 两个包，替换真实实现时改动面更小。
//
// [EN] A separate function keeps the contract package's import in one place, so server.go does
// not import both the pb and the service package and swapping in a real implementation touches
// less.
func pbRegister(server *grpc.Server, svc *authsvc.AuthService) {
	pb.RegisterAuthServiceServer(server, svc)
}
