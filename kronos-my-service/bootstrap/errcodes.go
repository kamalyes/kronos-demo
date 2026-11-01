/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\errcodes.go
 * @Description: 业务错误码状态映射注册
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	bizerror "github.com/kamalyes/go-bizerror"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"

	"github.com/kamalyes/kronos-my-service/errcodes"
)

// registerErrorCodes 把本服务专属错误码的状态映射注入 go-bizerror 的注册中心
//
// 为什么是启动期注册而不是在 errcodes 包里 init()：注册中心在 go-bizerror 里，
// 把注册写成本服务 errcodes 包的 init() 就得让每个用到码的地方都 import 那个包
// 才能保证 init 跑过——而只用码字符串、不 import 包的写法（比如从配置里读一个码）
// 会静默绕过注册于是同一个码在不同调用路径上给出不同状态码
// 显式在启动流程里调一次，注册这件事就只有一个入口、只有一个时点
//
// 为什么放在 Run() 的最前面（见 server.go）：这张表是纯静态数据，不依赖数据库、
// 缓存、下游客户端中的任何一个，没有任何理由排在它们之后；而排在后面意味着
// 在它之前出错的那些阶段若返回业务错误码，拿到的是兜底的 500 而不是正确的状态码
//
// 注册是幂等的（后注册覆盖先注册），重复调用不会累积副作用
//
// [EN] Registered during startup rather than from an init() in the errcodes package: the
// registry lives in go-bizerror, so an init() there would only run for call sites that import
// the package, and any path that uses a code as a bare string — reading one from configuration,
// say — would silently bypass registration and hand out a different status code for the same
// code. One explicit call gives registration a single entry point at a single moment. It sits
// at the very top of Run() because the table is static data that depends on neither the
// database, the cache nor any downstream client, and because anything failing before it would
// otherwise fall back to 500 instead of its real status. Registration is idempotent.
func registerErrorCodes() {
	mappings := errcodes.Mappings()
	bizerror.RegisterMany(mappings)
	gwglobal.LOGGER.Info("business error codes registered: %d", len(mappings))
}
