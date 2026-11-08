/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 00:00:00
 * @FilePath: \kronos-access-control-service\constants\service_name.go
 * @Description: 服务名与桩实现常量
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package constants

import "strings"

// ServiceName 服务名称
//
// 同时是配置文件前缀：go-rpc-gateway/config 按 <前缀>-<环境别名>.yaml 匹配，
// bootstrap 用 WithPrefix(ServiceName) 取它，resources/ 下的文件名由同一个
// kopy 变量渲染，所以两者不可能分叉。
//
// 也必须与网关 grpc.clients 里的 key 一致——网关靠服务名从连接池取连接
// 来调 Authorities。对不上的现象是网关每个请求都报
// "auth service connection is nil"，而本服务明明活着。
//
// [EN] Also the config prefix and, critically, the key under the gateway's grpc.clients: the
// gateway looks the connection up by service name to call Authorities, and a mismatch makes
// every gateway request report a nil auth connection while this service is plainly alive.
const ServiceName = "kronos-access-control-service"

// 桩实现用的固定身份
//
// 真实实现会从用户表查出这些值。桩里写死是为了让下游服务的租户隔离
// 有一个非空的租户可用——下游拿 GetTenantID() 拿到空串时，
// 多数实现等于「不过滤」，那会让演示看起来"能跑"但实际绕过了隔离。
//
// [EN] A real implementation loads these from a user table. The stub hardcodes them so
// downstream tenant checks have a non-empty tenant to work with: an empty tenant usually means
// no filtering at all, which would make a demo look like it works while bypassing isolation.
const (
	StubTenantID = "demo-tenant"
	StubRoleCode = "demo-role"
)

// publicPaths 免鉴权路径前缀
//
// 只放真正不该要凭据的：健康检查、文档站、指标。
//
// 不要往这里加业务路径。白名单是绕过鉴权的后门，加一条就多一个永远
// 不会被鉴权覆盖的入口，而且因为它工作正常，没人会回头检查它该不该在这儿。
// 业务接口的免鉴权需求应该用策略引擎里的「匿名角色」表达，而不是加白名单。
//
// [EN] Only paths that genuinely need no credentials: health, docs and metrics. Business paths
// do not belong here. The allow list is a bypass, and each entry is one more entrance that
// authorization never covers; because it works, nobody revisits whether it should be there. An
// anonymous business endpoint belongs in the policy engine as an anonymous role instead.
var publicPaths = []string{
	"/health",
	"/healthz",
	"/ready",
	"/metrics",
	"/swagger",
	"/openapi",
}

// IsPublicPath 判断路径是否免鉴权
//
// 用前缀匹配：文档站是一组路径（/swagger/*），健康检查也可能带后缀。
//
// 空前缀会匹配一切，所以显式跳过——publicPaths 里混进一个空串
// （比如配置解析出错、或者有人加了一行逗号）就等于整个服务免鉴权，
// 而现象是"一切正常"。这种失败必须在代码里挡住，不能靠维护者小心。
//
// [EN] Prefix matching, since the docs site is a group of paths and a health check may carry a
// suffix. An empty prefix matches everything and is skipped explicitly: one empty string in the
// list, from a parsing slip or a stray comma, would make the whole service credential-free with
// everything apparently working. That failure is blocked in code rather than left to care.
func IsPublicPath(path string) bool {
	if path == "" {
		return false
	}
	for _, prefix := range publicPaths {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
