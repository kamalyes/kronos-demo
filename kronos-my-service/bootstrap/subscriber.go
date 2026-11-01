/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\subscriber.go
 * @Description: 后台任务与事件订阅启动
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	tenantsvc "github.com/kamalyes/kronos-my-service/service/tenant"
)

// startSubscribers 启动后台任务与事件订阅
//
// 所有后台 goroutine 都用 s.gateway.Context() 作为父 ctx，不要用
// context.Background()：前者在收到 SIGTERM 时会被取消，goroutine 跟着退出，
// 进程能干净关闭；后者会让 goroutine 一直活着，容器要等到 grace period
// 结束才被 SIGKILL，滚动发布时每个 Pod 都多等几十秒。
//
// [EN] Every background goroutine takes s.gateway.Context() as its parent rather than
// context.Background(): the former is cancelled on SIGTERM so the goroutines exit and the
// process shuts down cleanly, while the latter keeps them alive until the container's grace
// period expires, adding that delay to every pod of a rolling release.
func (s *MyServiceGateway) startSubscribers() {
	// 下游同步 worker：服务间 gRPC 调用示例
	// 为什么同步放在后台而不是 RPC 里，见 worker 文件开头的说明
	// [EN] The downstream sync worker, an example of a service-to-service gRPC call; the file
	// header explains why the call belongs in the background rather than in the RPC
	tenantsvc.NewTenantSyncWorker(s.repo.Tenant).
		Start(s.gateway.Context())
	gwglobal.LOGGER.Info("All subscribers started")
}
