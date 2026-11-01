/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\tenant\tenant_sync_worker.go
 * @Description: 下游同步 worker —— 服务间 gRPC 调用示例
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package tenant

import (
	"context"
	"time"

	pbmo "github.com/kamalyes/go-pbmo"
	gwconstants "github.com/kamalyes/go-rpc-gateway/constants"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-my-service/models"
	"github.com/kamalyes/kronos-my-service/repository"
	"github.com/kamalyes/kronos-my-service/service/common"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	secondpb "github.com/kamalyes/kronos-share-proto-template/pb/inventory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// 服务间 gRPC 调用示例 | Service-to-service gRPC call example
//
// ============================================================================
// 为什么远程调用放在 worker 里，而不是放在 TenantRetrySync 这个 RPC 里
// ============================================================================
// 在 RPC 里同步等下游，等于把本接口的延迟和可用性绑在下游身上：下游慢 2 秒，
// 本接口就慢 2 秒；下游挂了，本接口跟着超时。更糟的是重试语义——调用方超时重试，
// 而下游可能已经处理过了。
//
// 所以 RPC 只做一件事：把状态置为 PENDING 然后立刻返回（见 RetrySync）。
// 真正的远程调用由这个后台 worker 拉取 PENDING 项来做，失败就留在 FAILED 等下一轮，
// 调用方的延迟与下游彻底解耦。
//
// [EN] Awaiting a downstream inside an RPC ties this endpoint's latency and availability to it:
// two seconds slower downstream means two seconds slower here, and a downstream outage becomes
// a timeout here. Worse is the retry semantics, where the caller times out and retries while
// the downstream may already have processed the request. The RPC therefore only marks the row
// PENDING and returns, and this worker performs the actual call, leaving failures to the next
// round and decoupling caller latency from the downstream entirely.
//
// ============================================================================
// 链路透传是免费的，前提是客户端来自连接池
// ============================================================================
// 下面调用下游时没有手工拼任何 metadata，但下游依然能拿到 trace_id / request_id /
// tenant_id / accept-language 等字段——连接池在拨号时挂了
// UnaryClientRequestContextInterceptor，它从 ctx 里取 RequestCommonMeta 注入出向 metadata。
// 详见 service/common/client.go 的说明。
//
// 但后台 worker 有个特殊之处：它的 ctx 不是从请求来的，里面没有 RequestCommonMeta，
// 所以透传出去的全是空值，下游日志里这条调用没有租户也没有链路。
// 这不是 bug，是 worker 的固有属性——没有「发起这次调用的请求」。
// 需要可追溯时得自己造一个标识，下面 withWorkerMeta 就是干这个的。
//
// [EN] No metadata is assembled by hand below, yet the downstream still receives trace id,
// request id, tenant id and language, because the pool attaches the request-context interceptor
// at dial time. A background worker is special, though: its context does not come from a
// request and holds no request metadata, so the propagated values are empty and the downstream
// log has neither tenant nor trace for this call. That is inherent to a worker rather than a
// bug, since there is no originating request; withWorkerMeta below manufactures an identity so
// the call remains traceable.

const (
	// syncBatchSize 每轮处理多少条
	// [EN] Rows handled per round
	syncBatchSize = 50

	// syncInterval 两轮之间的间隔
	//
	// 这个值和 syncBatchSize 一起决定吞吐上限（50 条 / 30 秒）。积压追不上时
	// 应该先调大批次，而不是缩短间隔——缩短间隔会在下游已经不堪重负时
	// 继续加压，把下游彻底压死。
	//
	// [EN] Together with the batch size this caps throughput. When a backlog cannot be worked
	// off, raise the batch size rather than shortening the interval: a shorter interval keeps
	// adding load to a downstream that is already struggling.
	syncInterval = 30 * time.Second

	// syncCallTimeout 单次下游调用的超时
	//
	// 必须有：worker 的 ctx 生命周期跟进程一样长，不给单次调用设超时的话，
	// 一个卡住的调用会把整个 worker 永久挂住，后面的全部积压，
	// 而现象只是「同步不动了」，日志里什么都没有。
	//
	// [EN] Mandatory: the worker's context lives as long as the process, so without a per-call
	// timeout one stuck call blocks the worker forever and everything behind it backs up, with
	// nothing in the log beyond syncing having stopped.
	syncCallTimeout = 5 * time.Second
)

// TenantSyncWorker 把 PENDING 的聚合根同步到下游
type TenantSyncWorker struct {
	repo *repository.TenantRepository
}

// NewTenantSyncWorker 创建 worker
func NewTenantSyncWorker(repo *repository.TenantRepository) *TenantSyncWorker {
	return &TenantSyncWorker{repo: repo}
}

// Start 启动 worker，随 ctx 取消退出
//
// ctx 来自网关（gateway.Context()），所以服务收到 SIGTERM 时这里会一起退——
// 不要用 context.Background() 起 goroutine，那样进程关不干净，
// 容器里表现为 SIGTERM 之后要等到 grace period 结束才被 KILL。
//
// [EN] The context comes from the gateway so a SIGTERM stops this too. Do not start the
// goroutine with context.Background(): the process then fails to shut down cleanly and the
// container waits out its grace period before being killed.
func (w *TenantSyncWorker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				gwglobal.LOGGER.InfoContext(ctx, "Tenant 同步 worker 退出")
				return
			case <-ticker.C:
				w.runOnce(ctx)
			}
		}
	}()
	gwglobal.LOGGER.Info("Tenant sync worker started")
}

// runOnce 处理一批
func (w *TenantSyncWorker) runOnce(ctx context.Context) {
	// 下游不可用时直接不拉取：拉出来也只能原样写回 PENDING，
	// 白查一次库，还把 updated_at 搅乱导致排序失真
	// [EN] Skip the fetch when the downstream is unavailable: the rows could only be written
	// back as PENDING, wasting a query and disturbing updated_at so the ordering degrades
	client, ok := common.WarehouseRemote()
	if !ok {
		gwglobal.LOGGER.DebugContext(ctx, "下游 Warehouse 不可用，跳过本轮同步")
		return
	}

	pending := enumspb.TenantSyncState_TENANT_SYNC_STATE_PENDING
	items, err := w.repo.ListBySyncState(ctx, pending, syncBatchSize)
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "拉取待同步项失败", "error", err)
		return
	}
	if len(items) == 0 {
		return
	}

	var succeeded, failed int
	for _, item := range items {
		if ctx.Err() != nil {
			// 进程正在退出：剩下的留在 PENDING，下次启动继续，不要硬着头皮跑完
			// [EN] Shutting down: leave the rest PENDING for the next start rather than pushing on
			return
		}
		if w.syncOne(ctx, client, item) {
			succeeded++
		} else {
			failed++
		}
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "同步批次完成",
		"total", len(items), "succeeded", succeeded, "failed", failed)
}

// syncOne 同步单条，返回是否成功
func (w *TenantSyncWorker) syncOne(
	ctx context.Context,
	client secondpb.WarehouseServiceClient,
	item *models.TenantModel,
) bool {
	callCtx, cancel := context.WithTimeout(withWorkerMeta(ctx, item.Id), syncCallTimeout)
	defer cancel()

	// 这就是那次远程调用。用 List 做探测而不是写接口：探测要幂等，
	// 重试不能产生副作用——worker 一定会重试（失败留 FAILED，下一轮再来），
	// 用非幂等的写接口做探测等于每轮都在下游造一条脏数据。
	//
	// [EN] This is the remote call. A read is used as the probe rather than a write because the
	// probe must be idempotent: the worker always retries, and probing with a non-idempotent
	// write would create a dirty row downstream on every round.
	resp, err := client.WarehouseList(callCtx, &secondpb.WarehouseListRequest{
		PageRequest: &commonpb.Paging{Page: 1, Size: 1},
	})

	next := enumspb.TenantSyncState_TENANT_SYNC_STATE_SYNCED
	if err != nil {
		// 区分「下游明确拒绝」和「下游没答上来」：
		//   NotFound / InvalidArgument 这类是下游给出的确定答案，重试结果一样，置 FAILED;
		//   Unavailable / DeadlineExceeded 是没答上来，重试可能成功，留在 PENDING。
		// 不区分的话，一次网络抖动会把一批记录永久钉在 FAILED 上，
		// 要人工介入才能恢复。
		//
		// [EN] Distinguish a definite refusal from no answer at all: NotFound or
		// InvalidArgument are answers that a retry will repeat, so the row goes to FAILED,
		// while Unavailable or DeadlineExceeded may succeed on retry and stay PENDING. Without
		// that split, one network blip pins a batch to FAILED until someone intervenes.
		if retryable(err) {
			gwglobal.LOGGER.WarnContextKV(ctx, "下游暂时不可用，保持待同步",
				"id", item.Id, "error", err)
			return false
		}
		next = enumspb.TenantSyncState_TENANT_SYNC_STATE_FAILED
		gwglobal.LOGGER.ErrorContextKV(ctx, "下游明确拒绝，置为同步失败",
			"id", item.Id, "code", status.Code(err).String(), "error", err)
	} else {
		gwglobal.LOGGER.DebugContextKV(ctx, "下游探测成功",
			"id", item.Id, "downstream_total", resp.GetPageResponse().GetTotal())
	}

	updates := pbmo.NewUpdates().Set("sync_state", next).Build()
	if err := w.repo.UpdateByID(ctx, item.Id, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "回写同步状态失败", "id", item.Id, "error", err)
		return false
	}
	return next == enumspb.TenantSyncState_TENANT_SYNC_STATE_SYNCED
}

// retryable 判断这个错误值不值得下一轮再试
//
// 只认传输层和超时类错误。Unknown 不算：它多半是下游 panic 或返回了非 status 错误，
// 重试同样会 panic，留在 PENDING 只会让它每轮都去撞一次。
//
// [EN] Only transport and deadline errors count. Unknown does not: it usually means the
// downstream panicked or returned a non-status error, which a retry reproduces, so leaving the
// row PENDING would only make it collide every round.
func retryable(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// withWorkerMeta 给后台调用造一个可追溯的标识
//
// worker 的 ctx 里没有 RequestCommonMeta（没有发起它的请求），所以连接池的
// 透传拦截器注入出去的是一堆空值。这里手工补一个 request_id，让下游日志里
// 这次调用能和本服务的日志对上——否则下游只看到一串无主的调用。
//
// 用 AppendToOutgoingContext 而不是 NewOutgoingContext：后者会整个替换掉
// 出向 metadata，把拦截器稍后注入的字段也一起冲掉；前者是追加。
//
// 真实项目里这里还应该带上一个标识「这是后台任务不是用户请求」的字段，
// 下游据此区分限流策略——用户请求不该和批量任务抢同一个配额。
//
// [EN] The worker's context has no request metadata, so the pool's interceptor propagates empty
// values. A request id is added here so the downstream log can be matched against this
// service's. AppendToOutgoingContext is used rather than NewOutgoingContext, which would
// replace the outgoing metadata wholesale and discard what the interceptor adds later. A real
// project would also mark the call as a background task so the downstream can rate-limit it
// separately from user requests, which should not compete with batch work for one quota.
func withWorkerMeta(ctx context.Context, itemID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		gwconstants.MetadataRequestID, "worker-tenant-sync-"+itemID,
	)
}
