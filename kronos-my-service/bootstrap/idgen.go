/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\idgen.go
 * @Description: ID 中心分发器装配 —— 服务间 gRPC 调用的真实用例
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	"context"
	"fmt"

	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-my-service/constants"
	"github.com/kamalyes/kronos-my-service/models"
	"github.com/kamalyes/kronos-my-service/service/common"
	keypb "github.com/kamalyes/kronos-share-proto-template/pb/key"
)

// initIDDispenser 把 ID 号段分发接到中心发号服务
//
// ============================================================================
// 为什么需要它
// ============================================================================
// 本地 Snowflake 要求实例有确定且唯一的 worker id，而网关用的
// osx.GetWorkerIdForSnowflake() 等于 GetWorkerId() % 32——只有 32 个槽位。
// Deployment 的 Pod 名是 myapp-7d9f8b-x4k2p 这种随机后缀，提取不出序号，
// 于是落到 sha256(hostname) % 1024 再 % 32，副本一多就是生日碰撞：
//
//	2 个副本  3.1%     8 个副本 61.4%
//	4 个副本 17.7%    16 个副本 99.0%
//
// 两个 Pod 拿到同一个 worker id，各有独立序列计数器，同一毫秒发出相同 ID，
// 表现为偶发主键冲突——与流量相关、重启后碰撞组合还会变，极难排查。
//
// 由一处集中发号就没有这个问题：唯一性只取决于中心序列，与实例数、Pod 名、
// worker id 全都无关。详见 models/idsource.go 开头。
//
// [EN] A local Snowflake needs a unique deterministic worker id, but the gateway derives it
// modulo 32, and a Deployment pod name carries a random suffix with no ordinal, so the id comes
// from a hostname hash into 32 slots and birthday collisions follow: about 3% at two replicas,
// 18% at four, 61% at eight and 99% at sixteen. Two pods sharing a worker id keep independent
// counters and emit identical ids within a millisecond, surfacing as intermittent primary-key
// conflicts. Issuing from one place removes it entirely.
//
// ============================================================================
// 这就是服务间 gRPC 调用的落点
// ============================================================================
// 下面那个闭包是 models.BlockAllocator，它一次向 kronos-key-service 申领
// 一整段 ID，models 侧在本地缓冲后逐个发放。链路透传（trace_id / request_id /
// tenant_id）由网关连接池的客户端拦截器自动完成，这里没有手工拼任何 metadata，
// 见 service/common/client.go 的说明。
//
// 注意 ctx：这个闭包的 ctx 由 models 侧自己造（见 idsource.go 的 fetch），
// 不是请求的 ctx——号段是进程级资源，不属于某一次请求。所以透传出去的
// tenant_id 是空的，下游 IdBlockService 会因为 TenantOf 失败而拒绝。
// 这正是下面要显式注入租户的原因。
//
// [EN] The closure below is the BlockAllocator: it claims a whole block from the issuer and
// models buffers it locally. Trace, request and tenant propagation come from the pool's client
// interceptor with no hand-assembled metadata. Note the context: models builds its own, since a
// block is a process-level resource rather than part of one request, so the propagated tenant
// is empty and the downstream would refuse on its tenant check — which is why the tenant is
// injected explicitly below.
func (s *MyServiceGateway) initIDDispenser() {
	client, ok := common.IdBlockRemote()
	if !ok {
		gwglobal.LOGGER.Warn("中心发号服务 %s 不可用，ID 使用本地 Snowflake"+
			" | the id issuer is unavailable, ids come from local snowflake",
			constants.RemoteServiceKey)
		return
	}

	// 命名空间用「服务名:实体」而不是只用服务名
	//
	// 命名空间决定用哪个序列。所有实体共用一个的话，号会交错——不会重复
	// （是同一个序列），但一张表的写入量会推高另一张表的号，看监控时
	// 无法按实体区分发号速率。
	//
	// [EN] The namespace selects the sequence. Sharing one across entities interleaves the
	// numbers: no duplicates, since it is one sequence, but one table's volume pushes another's
	// along and the issue rate cannot be attributed per entity in monitoring.
	namespace := constants.ServiceName + ":entity"

	models.SetIDAllocator(func(ctx context.Context, n int) ([]string, error) {
		// 显式带上租户：这个调用来自后台（号段预取），ctx 里没有
		// RequestCommonMeta，连接池的透传拦截器注入出去的租户是空的。
		// 下游 IdBlockService 用租户给命名空间加前缀并做隔离，
		// 拿不到租户会直接拒绝。
		//
		// 用服务自身的身份而不是某个业务租户：号段是本服务的基础设施资源，
		// 不归任何一个业务租户。拿请求里的租户去取号，会让不同租户的
		// 写入各自占一个序列，号段利用率被切碎，而且第一个请求的租户
		// 会决定整个进程的序列归属。
		//
		// [EN] The tenant is attached explicitly: this call comes from the background prefetch,
		// whose context holds no request metadata, so the interceptor propagates an empty tenant
		// and the downstream, which prefixes and isolates the namespace by tenant, refuses. The
		// service's own identity is used rather than a business tenant, because a block is this
		// service's infrastructure resource and belongs to no single tenant; drawing on the
		// requesting tenant would fragment the blocks per tenant and let the first request
		// decide the whole process's sequence.
		callCtx := common.WithServiceIdentity(ctx)

		resp, err := client.IdBlockAllocate(callCtx, &keypb.IdBlockAllocateRequest{
			Namespace: namespace,
			Count:     int32(n),
		})
		if err != nil {
			return nil, fmt.Errorf("向 %s 申领号段失败 | allocate from %s failed: %w",
				constants.RemoteServiceKey, constants.RemoteServiceKey, err)
		}
		return resp.GetIds(), nil
	}, constants.IDBlockSize)

	gwglobal.LOGGER.Info("ID dispenser initialized via %s (namespace %s, block size %d)",
		constants.RemoteServiceKey, namespace, constants.IDBlockSize)
}
