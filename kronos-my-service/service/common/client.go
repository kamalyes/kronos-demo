/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\common\client.go
 * @Description: 服务间 gRPC 客户端持有与取用
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package common

// 本文件整体不受 GenerateExample 门控
//
// 原先 Clients 连同 import 一起被圈进 `if eq .GenerateExample "true"`，
// 但 bootstrap/idgen.go 是无条件生成的，而它要用 IdBlockRemote 与
// constants.RemoteServiceKey——于是 GenerateExample=false 时渲染出的项目
// 直接报 "undefined: common.IdBlockRemote"
//
// 为什么是去掉门而不是把 idgen.go 也关掉：中心发号是正确性设施而非示例。
// 本地 Snowflake 的 worker id 只有 32 个槽位且由 hostname 哈希得出，
// 16 副本时碰撞概率约 99%，表现为偶发主键冲突（详见 bootstrap/idgen.go 开头）。
// 把它随示例一起删掉，等于让骨架项目默默退回这条有缺陷的路径
//
// 保留门控的是「装配」而不是「声明」：bootstrap/clients.go 在
// GenerateExample=false 时不调用 SetClients，于是单例停在零值，
// IdBlockRemote 返回 false，idgen 打一行「下游不可用」后跳过——
// 骨架项目因此拿到的是一条看得见的降级提示，而不是一个被悄悄删掉的能力
//
// [EN] This file is intentionally not gated on GenerateExample. Clients used to sit inside
// `if eq .GenerateExample "true"` together with the imports, but bootstrap/idgen.go is
// generated unconditionally and needs IdBlockRemote and constants.RemoteServiceKey, so
// GenerateExample=false produced "undefined: common.IdBlockRemote". The gate is removed rather
// than also dropping idgen.go because central id dispensing is a correctness facility, not a
// sample: a local Snowflake worker id has 32 slots derived from a hostname hash and collides
// with about 99% probability at sixteen replicas, surfacing as intermittent primary-key
// conflicts. What stays gated is the assembly rather than the declaration: clients.go does not
// call SetClients when GenerateExample is false, so the singleton stays zero-valued,
// IdBlockRemote reports false and idgen logs an unavailable downstream and skips. A skeleton
// project therefore gets a visible degradation notice instead of a silently deleted capability.
import (
	"context"

	gwconfig "github.com/kamalyes/go-config/pkg/gateway"
	gwconstants "github.com/kamalyes/go-rpc-gateway/constants"
	grpcpool "github.com/kamalyes/go-rpc-gateway/cpool/grpc"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-my-service/constants"
	secondpb "github.com/kamalyes/kronos-share-proto-template/pb/inventory"
	keypb "github.com/kamalyes/kronos-share-proto-template/pb/key"
	"google.golang.org/grpc/metadata"
)

// Clients 本服务要调用的下游客户端集合
//
// 为什么集中放一个结构体而不是各模块自己拨号：客户端是进程级资源，
// 连接要复用（连接池按服务名索引，重复 InitClient 会复用同一条 *grpc.ClientConn），
// 健康状态要共享。各模块各建一份的话，连接数翻倍，而且健康检查各记一套。
//
// [EN] Clients are process-level resources: the connection is reused (the pool is indexed by
// service name and a repeat InitClient returns the same ClientConn) and the health state is
// shared. One holder avoids doubling connections and keeping several health views.
type Clients struct {
	health        *grpcpool.HealthChecker
	IdBlockClient keypb.IdBlockServiceClient
	IdBlockReady  bool

	// 第二下游：混合模块的同步 worker 用它做服务间调用示例
	// [EN] The second downstream, used by the mixed module's sync worker
	WarehouseClient secondpb.WarehouseServiceClient
	WarehouseReady  bool
}

// clients 进程内单例，由 bootstrap 在启动阶段装配一次
// [EN] A process-wide singleton assembled once by bootstrap at startup
var clients = &Clients{}

// SetClients 由 bootstrap 注入，业务代码不要调用
// [EN] Injected by bootstrap; not for business code
func SetClients(c *Clients) {
	if c != nil {
		clients = c
	}
}

// NewClients 构造客户端集合
//
// 这里不自己 grpc.NewClient，一律走 grpcpool.InitClientTo。原因是连接池在拨号时
// 挂了一整条客户端拦截器链，自己拨号这些全都拿不到：
//
//	UnaryClientRequestContextInterceptor —— 把本次请求的 trace_id / request_id /
//	  tenant_id / authorization / accept-language 等三十余个字段注入出向 metadata。
//	  少了它，下游拿不到租户、拿不到语言、链路断在这一跳，下游日志里这些字段全空。
//	UnaryClientLoggingInterceptor —— 记录每次调用的目标、耗时、结果。
//	UnaryClientHealthInterceptor —— 下游不健康时直接短路，不耗一次超时。
//	otelgrpc.NewClientHandler —— 注入 grpc-trace-bin，和 HTTP 侧的 span 串起来。
//
// 换句话说：服务间调用的链路透传不是业务代码的事，用对了工具就是免费的；
// 用错了（自己拨号）则要手写三十多个字段的 metadata 注入，且必然漏。
//
// [EN] Always go through grpcpool.InitClientTo rather than dialing directly, because the pool
// attaches a full client interceptor chain at dial time that a hand-rolled dial does not get:
// request-context propagation injecting trace id, request id, tenant id, authorization,
// accept-language and some thirty more fields into the outgoing metadata, without which the
// downstream has no tenant, no language and a trace broken at this hop; call logging; health
// short-circuiting that avoids spending a timeout on a known-down service; and the otelgrpc
// client handler that ties the span to the HTTP side. Propagation is free when the right tool
// is used, and requires hand-writing thirty-odd metadata fields when it is not.
func NewClients(gw GatewayConfigProvider) *Clients {
	c := &Clients{health: grpcpool.NewHealthChecker()}

	cfg := gw.GetConfig()
	if cfg == nil || cfg.GRPC == nil || cfg.GRPC.Clients == nil {
		gwglobal.LOGGER.Warn("未配置任何 gRPC 客户端 | no gRPC clients configured")
		return c
	}
	grpcClients := cfg.GRPC.Clients

	c.IdBlockReady = grpcpool.InitClientTo(
		c.health, grpcClients,
		constants.RemoteServiceKey, "IdBlock",
		keypb.NewIdBlockServiceClient,
		&c.IdBlockClient,
	)

	c.WarehouseReady = grpcpool.InitClientTo(
		c.health, grpcClients,
		constants.SecondRemoteServiceKey, "Warehouse",
		secondpb.NewWarehouseServiceClient,
		&c.WarehouseClient,
	)

	// 周期性健康检查：拨号本身不等就绪（grpc.NewClient 是惰性的），
	// 所以「连接建好了」不等于「下游活着」。健康检查把这件事变成可查询的状态，
	// 调用方据此决定是跳过还是硬调。
	// [EN] Dialing does not wait for readiness, so a created connection does not mean a live
	// downstream. The periodic check turns that into queryable state for callers to act on.
	if endpoints := grpcpool.BuildEndpointMap(grpcClients); len(endpoints) > 0 {
		c.health.StartPeriodicCheck(constants.RemoteHealthCheckInterval, endpoints)
	}
	return c
}

// GatewayConfigProvider 只要求能拿到网关配置
//
// 用接口而不是直接依赖 *gateway.Gateway：service 包不该为了读一段配置
// 就把整个网关类型拖进来，那会让 service 反向依赖 bootstrap 的装配细节。
//
// [EN] An interface rather than a dependency on *gateway.Gateway, so the service package does
// not pull the whole gateway type in just to read a config section.
type GatewayConfigProvider interface {
	GetConfig() *gwconfig.Gateway
}

// IdBlockRemote 取中心发号服务的客户端，第二个返回值表示现在能不能用
//
// 两种「不能用」合并成一个 false：没配置（endpoints 为空）和配了但不健康。
// 调用方对这两种的处置是一样的——跳过这次远程调用，走降级逻辑。
// 想区分的话查日志：未配置在启动时 Warn 一次，不健康由健康检查持续记录。
//
// 注意不要在这里返回 error：远程下游不可用在多数场景下不是本次请求的错误，
// 返回 error 会诱导调用方把它当失败往上抛，于是下游抖一下上游就跟着 500。
//
// [EN] Unconfigured and unhealthy both collapse to false because callers handle them the same
// way: skip the remote call and degrade. No error is returned on purpose, since an unavailable
// downstream is usually not this request's failure, and an error would invite callers to
// propagate it so that a downstream blip turns into a 500 upstream.
func IdBlockRemote() (keypb.IdBlockServiceClient, bool) {
	if !clients.IdBlockReady || clients.IdBlockClient == nil {
		return nil, false
	}
	if clients.health != nil && !clients.health.IsHealthy(constants.RemoteServiceKey) {
		return nil, false
	}
	return clients.IdBlockClient, true
}

// WarehouseRemote 取第二下游的客户端，语义与 IdBlockRemote 完全一致
//
// 未配置与不健康都归为 false，调用方一律跳过本次远程调用；这里同样不返回 error
// 理由见上面 IdBlockRemote 的说明
//
// [EN] The second downstream's client, with exactly the same semantics as IdBlockRemote:
// unconfigured and unhealthy both collapse to false so callers skip the call, and no error is
// returned, for the reasons given on IdBlockRemote above
func WarehouseRemote() (secondpb.WarehouseServiceClient, bool) {
	if !clients.WarehouseReady || clients.WarehouseClient == nil {
		return nil, false
	}
	if clients.health != nil && !clients.health.IsHealthy(constants.SecondRemoteServiceKey) {
		return nil, false
	}
	return clients.WarehouseClient, true
}

// WithServiceIdentity 给后台调用注入本服务自己的身份
//
// 后台任务（号段预取、同步 worker）的 ctx 不是从请求来的，里面没有
// RequestCommonMeta，所以连接池的透传拦截器注入出去的租户、请求 ID 全是空的。
// 下游若按租户隔离就会直接拒绝。
//
// 注入的是「本服务」而不是某个业务租户：基础设施资源（号段）不归任何一个
// 业务租户。用请求里的租户去取，会让每个租户各占一个序列把号段切碎，
// 而且第一个请求的租户会决定整个进程的序列归属。
//
// 用 AppendToOutgoingContext 而不是 NewOutgoingContext：后者整个替换出向
// metadata，会把拦截器稍后注入的字段一起冲掉；前者是追加。
//
// [EN] A background context carries no request metadata, so the pool's interceptor propagates
// an empty tenant and request id and a tenant-isolating downstream refuses. The service's own
// identity is injected rather than a business tenant, because infrastructure resources belong
// to none; drawing on the requesting tenant would fragment them per tenant and let the first
// request decide the whole process's sequence. AppendToOutgoingContext is used because
// NewOutgoingContext replaces the outgoing metadata wholesale and discards what the interceptor
// adds later.
func WithServiceIdentity(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		gwconstants.MetadataTenantID, constants.ServiceName,
		gwconstants.MetadataRequestID, "svc-"+constants.ServiceName,
	)
}
