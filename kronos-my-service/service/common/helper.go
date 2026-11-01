/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\common\helper.go
 * @Description: 服务层跨模块公共辅助：租户解析与错误归并
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package common

import (
	"context"

	bizerror "github.com/kamalyes/go-bizerror"
	gwmiddleware "github.com/kamalyes/go-rpc-gateway/middleware"
)

// TenantOf 从请求上下文取租户 ID
//
// 租户只认上下文，不认请求体：请求体里的 tenant_id 是调用方可控的，拿它过滤等于没过滤。
// 上下文里的值由网关的 request_context 中间件按 resources 配置的
// authorization-sources 解析出来（见 resources/kronos-my-service-dev.yaml）。
//
// [EN] The tenant comes from the context only, never the request body, which the caller
// controls. The gateway's request_context middleware resolves it per the configured sources.
func TenantOf(ctx context.Context) (string, error) {
	tenantID := gwmiddleware.GetTenantID(ctx)
	if tenantID == "" {
		return "", bizerror.ToGRPCError(ctx, bizerror.BizErrCodeUnauthenticated)
	}
	return tenantID, nil
}

// DBError 把仓储层的错误统一归并成业务错误码
//
// 不把 err 原样返回给调用方：底层错误里可能带表名、SQL 片段、连接串，
// 那是内部信息。日志里记全文，响应里只给错误码与本地化文案。
//
// [EN] The underlying error is not returned to the caller: it can carry table names, SQL
// fragments or connection strings. The full text goes to the log; the response carries only
// the code and its localized message.
func DBError(ctx context.Context) error {
	return bizerror.ToGRPCError(ctx, bizerror.BizErrCodeDatabaseOperationFailed)
}

// NotFound 资源不存在（通用码）
//
// 优先用实体专属码：errcodes 里每个实体都有自己的 not_found，
// 响应里带的是 error.<实体>_not_found，调用方一眼能看出缺的是哪个资源；
// 用这个通用码只会得到 error.not_found，排查时还要回去翻日志对调用栈
//
// 保留它是给「确实没有实体可指」的场合：比如按某个外部标识查不到任何东西、
// 或者还没为新实体声明专属码时的过渡写法
//
// [EN] Prefer the entity's own code: errcodes gives every entity a not-found of its own, so the
// response carries error.<entity>_not_found and the caller can see which resource is missing,
// where this generic one yields only error.not_found and sends whoever is debugging back to the
// logs to match up the call stack. It is kept for the cases where no entity can be named, such
// as a lookup by an external identifier that matches nothing, and as a stopgap before a new
// entity gets its own code
func NotFound(ctx context.Context) error {
	return bizerror.ToGRPCError(ctx, bizerror.BizErrCodeNotFound)
}
