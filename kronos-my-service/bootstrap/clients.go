/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\clients.go
 * @Description: 服务间 gRPC 客户端装配
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-my-service/service/common"
)

// initClients 装配下游 gRPC 客户端
//
// 放在 initServices 之前：service 层通过 common.XxxRemote() 取客户端，
// 取的是装配好的单例。顺序反了的话，service 构造时拿到的是空壳——
// 不过因为取用是在调用时而不是构造时，顺序错了也不会立刻崩，
// 而是表现为「启动后一段时间内所有远程调用都认为下游未配置」，更难查。
//
// 客户端装配失败不阻断启动：下游不可用不该让本服务起不来，
// 否则一个边缘下游的故障会沿调用链反向放大成一片服务起不来。
//
// [EN] Before initServices, because the service layer fetches the assembled singleton through
// common.XxxRemote(). The wrong order does not crash, since the fetch happens at call time
// rather than construction time; it shows up as every remote call believing the downstream is
// unconfigured, which is harder to diagnose. A failure here never blocks startup: an
// unavailable downstream must not keep this service down, or one peripheral failure amplifies
// backwards along the call chain into a set of services that cannot start.
func (s *MyServiceGateway) initClients() {
	common.SetClients(common.NewClients(s.gateway))
	gwglobal.LOGGER.Info("gRPC clients initialized")
}
