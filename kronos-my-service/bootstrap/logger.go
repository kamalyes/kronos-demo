/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\logger.go
 * @Description: 日志上下文字段配置
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	logger "github.com/kamalyes/go-logger"
	gwconstants "github.com/kamalyes/go-rpc-gateway/constants"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
)

// configureLoggerContext 声明每条 Context 日志自动带出哪些字段
//
// 配一次，之后所有 *ContextKV / *Context 日志都自动带上这些字段，业务代码里
// 一个字都不用写——不要再在调用处手工拼 request_id / tenant_id。
//
// go-logger 的取值顺序（见 extractKeyValue）：
//  1. trace_id 固定从 OTel span 取，是单一真相源，不受这里的配置影响；
//     网关的 gRPC server 挂了 otelgrpc 的 StatsHandler，所以每个 RPC 的 ctx 都在 span 内。
//  2. 其余 key 先查 ctx.Value(key)——网关的 request_context 中间件正是用
//     constants.Metadata* 这些字符串做键存进 ctx 的，所以这里直接用同一批常量，
//     不要写字面量 "request_id"：那个键没人往里存，查出来恒为空。
//  3. ctx 里没有则回落 gRPC incoming metadata 的同名 header。于是即便某个请求
//     没走中间件（比如服务间直连），只要上游传了 header 就照样有值。
//
// 字段为空值时 go-logger 直接跳过，不会打出一堆空串；调用处用 KV 传了同名键则以
// KV 为准，不会重复。
//
// 注意日志里的字段名就是这里的 key，即 "x-tenant-id" 而不是 "tenant_id"：
// 查找键和输出键是同一个，没有改名的余地。好处是日志字段名与 HTTP header、
// gRPC metadata 完全一致，查问题时三处可以用同一个名字对；
// 代价是 grep 日志要记得带 x- 前缀。
//
// [EN] Declare once which fields every context log carries, and no call site needs to pass them
// again. go-logger resolves trace_id from the OTel span as the single source of truth, and for
// the remaining keys it reads ctx.Value(key) first and then falls back to the gRPC incoming
// metadata header of the same name. The gateway's request_context middleware stores its values
// under exactly these constants.Metadata* strings, which is why the constants are used here
// rather than literals like "request_id": nothing is ever stored under that name. Empty values
// are skipped, and a key passed explicitly as KV at a call site wins over the extracted one.
func configureLoggerContext() {
	// WithContextKeys 只在具体类型 *logger.Logger 上，不在 ILogger 接口上
	// [EN] WithContextKeys lives on the concrete *logger.Logger, not on the ILogger interface
	l, ok := gwglobal.LOGGER.(*logger.Logger)
	if !ok {
		return
	}
	l.WithContextKeys(
		logger.ContextKeyTraceID,
		gwconstants.MetadataRequestID,
		gwconstants.MetadataTenantID,
		gwconstants.MetadataUserID,
		gwconstants.MetadataPlatformID,
		gwconstants.MetadataRegionID,
	)
}
