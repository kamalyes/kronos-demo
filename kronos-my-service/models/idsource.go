/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\idsource.go
 * @Description: 号段式 ID 分发 —— 中心分发器 + 本地缓冲 + 降级
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
)

// ============================================================================
// 为什么需要一个中心分发器：本地 Snowflake 在 Deployment 下会撞 worker id
// ============================================================================
//
// 网关的全局 Snowflake 用 osx.GetWorkerIdForSnowflake() 取 worker id，它等于
// GetWorkerId() % 32 —— 只有 32 个槽位。而 GetWorkerId() 的取值顺序是：
//
//   1. POD_NAME 里的序号（StatefulSet 的 myapp-0 / myapp-1，确定且唯一）
//   2. HOSTNAME 里的序号
//   3. WORKER_ID / NODE_ID / POD_ORDINAL 环境变量
//   4. 以上都没有 → sha256(hostname) % 1024
//
// Deployment 的 Pod 名字是 myapp-7d9f8b-x4k2p 这种随机后缀，提取不出序号，
// 于是落到第 4 条：哈希取模。再 % 32 之后，槽位只有 32 个，于是生日碰撞：
//
//    2 个副本  3.1%
//    4 个副本 17.7%
//    8 个副本 61.4%
//   16 个副本 99.0%
//
// 两个 Pod 拿到同一个 worker id，它们各有独立的序列计数器，同一毫秒内就会发出
// 完全相同的 ID。症状是插入时偶发主键冲突——偶发、与流量相关、重启后碰撞组合
// 还会变，是最难查的一类问题。
//
// 两条解法：
//   A. 把 worker id 变成确定且唯一的：改用 StatefulSet，或在 Deployment 里用
//      Downward API 注入一个唯一的 WORKER_ID。能做到的话这是最简单的办法。
//   B. 不再让每个实例自己决定 ID，改由一个中心分发器发号。本文件做的是这个。
//
// [EN] The gateway's Snowflake takes its worker id from GetWorkerId() modulo 32, so there are
// only 32 slots. GetWorkerId() prefers a pod ordinal, then a worker-id env var, and otherwise
// falls back to a hash of the hostname. A Deployment pod's name carries a random suffix with no
// ordinal to extract, so the hash path is used, and with 32 slots birthday collisions follow:
// about 3% at two replicas, 18% at four, 61% at eight and 99% at sixteen. Two pods sharing a
// worker id keep independent sequence counters and emit identical ids within the same
// millisecond, which surfaces as intermittent primary-key conflicts that correlate with traffic
// and change shape on every restart. Either make the worker id unique and deterministic, with a
// StatefulSet or an injected WORKER_ID, or stop letting each instance decide and have a central
// dispenser hand out ids, which is what this file does.

// ============================================================================
// 号段模式：为什么不是每次要 ID 都问分发器
// ============================================================================
//
// 每条记录一次远程调用有三个问题，任何一个都足以否掉这个做法：
//
//   1. 每次插入都多一个网络往返。批量导入一万条就是一万次 RTT。
//   2. 分发器变成写路径上的硬依赖：它挂了，所有写接口跟着挂。
//   3. 分发器成为全局单点热点，所有服务所有实例的写流量都压在它身上。
//
// 所以一次取一整段（block），之后从本地缓冲里发，段用掉大半时提前异步补下一段。
// 稳态下每 blockSize 条记录才有一次远程调用，分发器的压力降到 1/blockSize。
//
// 代价是 ID 会有空洞：实例重启时缓冲里没用掉的号段直接丢弃。这对代理主键没有
// 任何影响（它只要唯一，不需要连续），但不要把这种 ID 直接当成「订单号」给用户看——
// 用户会从空洞里推断出业务量，也会因为号跳了来问。
//
// [EN] One remote call per row is unacceptable three times over: it adds a round trip to every
// insert, so a ten-thousand-row import pays ten thousand of them; it makes the dispenser a hard
// dependency of the write path, so its outage takes every write down; and it concentrates the
// write traffic of every instance of every service onto one hot spot. A whole block is fetched
// instead and served from a local buffer, with the next block pre-fetched asynchronously once
// the current one is mostly spent, so the steady state is one remote call per blockSize rows and
// the dispenser sees a fraction of the load. The cost is gaps: a restart discards whatever is
// left in the buffer. That is irrelevant for a surrogate key, which needs only uniqueness, but
// such an id should not be shown to users as an order number, since they will infer volume from
// the gaps and ask why the numbers jump.

// BlockAllocator 向中心分发器申领 n 个 ID
//
// 签名刻意只有「要 n 个，给我一批」这一种语义，不暴露底层是什么：
// 模板默认接的是 go-cachex 的 Redis IDGenerator（一条 Lua 脚本 INCRBY n，
// 一次往返拿一整段，天然原子）。要换成独立的发号服务，
// 只需把这个函数改成一次 gRPC 调用，上层一行都不用动
// （客户端怎么接见 service/common/client.go）。
//
// [EN] The signature deliberately exposes only "give me n ids" and says nothing about the
// implementation. The default is go-cachex's Redis IDGenerator, whose Lua INCRBY fetches a whole
// block atomically in one round trip. Swapping in a dedicated dispenser service means changing
// this one function to a gRPC call, with nothing above it touched.
type BlockAllocator func(ctx context.Context, n int) ([]string, error)

const (
	// defaultBlockSize 一次申领多少个
	//
	// 这个值是「远程调用频率」和「重启丢号量」的权衡：
	// 调大 → 远程调用更少，但重启时丢掉的号更多，空洞更大；
	// 调小 → 空洞小，但分发器压力上升，极端情况下退化成每条一次调用。
	// 按单实例写入峰值的几秒用量取值比较稳妥。
	//
	// [EN] A trade-off between how often the allocator is called and how many ids a restart
	// discards. A few seconds of peak single-instance write volume is a sound starting point.
	defaultBlockSize = 200

	// watermarkRatio 水位 = 号段大小 / 这个比例
	//
	// 写成比例而不是绝对值：绝对值在号段调小之后会变成病态配置——
	// 比如水位 50 配号段 60，意味着刚取 10 个就开始补，补段和消费几乎同频，
	// 失去了批量的意义。按比例派生则任何号段大小下行为都一致。
	//
	// [EN] A ratio rather than an absolute value, because an absolute watermark becomes
	// pathological once the block shrinks: a watermark of 50 with a block of 60 starts refilling
	// after ten ids, so refills and consumption run at nearly the same rate and batching buys
	// nothing. A ratio behaves consistently at any block size.
	watermarkRatio = 4

	// waitForRefill 缓冲见底、且已有补段在飞时，最多等多久
	//
	// 为什么要等而不是立刻降级：降级目标是本地 Snowflake，而那正是引入分发器
	// 想避开的东西（worker id 碰撞）。见底多半只是补段还在路上——等几十毫秒
	// 就能拿到号段里的 ID，比换一种 ID 形态划算。
	//
	// 但必须有上限：分发器真的挂了时，无上限等待会把写接口全部挂死，
	// 那比用 Snowflake 严重得多。等不到就降级，立刻返回。
	//
	// [EN] Waiting beats degrading immediately, because the fallback is the local Snowflake that
	// the dispenser exists to avoid. An empty buffer usually just means the refill is still in
	// flight, and tens of milliseconds buys a proper block id instead of a different id shape.
	// The bound is mandatory, though: if the allocator is genuinely down, an unbounded wait
	// hangs every write endpoint, which is far worse than using Snowflake.
	waitForRefill = 100 * time.Millisecond

	// refillWatermark 水位的下限，号段很小时兜底
	//
	// 不能等用光才补：用光那一刻的请求要同步等一次远程调用，
	// 表现为每 blockSize 条就有一条延迟尖刺，在 p99 上看得很清楚。
	// 提前补的话补段和消费并行，正常流量下调用方永远等不到。
	//
	// 水位要留足「补一段的耗时 × 峰值消费速率」。留太少等于没留。
	//
	// [EN] Refilling only on exhaustion makes the unlucky request wait for a round trip, which
	// shows up as a latency spike every blockSize rows and is plainly visible at p99. Refilling
	// early overlaps the fetch with consumption so callers never wait. The watermark must cover
	// the fetch latency multiplied by the peak consumption rate; too small and it buys nothing.
	refillWatermark = 1

	// allocTimeout 单次申领的超时
	// [EN] Timeout for one allocation
	allocTimeout = 3 * time.Second
)

// idDispenser 本地号段缓冲
type idDispenser struct {
	mu    sync.Mutex
	buf   []string
	alloc BlockAllocator
	size  int
	mark  int

	// waiters 补段完成的广播通道：补完就 close 再换一个新的。
	// 用 close 广播而不是逐个通知——等待者数量不定，close 一次全部唤醒。
	// [EN] A broadcast channel closed and replaced on each refill: the number of waiters is
	// unknown, and one close wakes them all
	waiters chan struct{}

	// refilling 保证同一时刻只有一个补段在飞
	//
	// 没有这个的话：水位一破，这一瞬间所有并发请求各自触发一次补段，
	// 一下申领几十个段，绝大部分还没用就随进程退出丢掉——
	// 既浪费号，又把分发器打成尖峰。
	//
	// [EN] Without it, the moment the watermark breaks every concurrent request triggers its own
	// refill, allocating dozens of blocks at once, most of which are discarded unused at exit:
	// it both wastes ids and turns the allocator's load into a spike.
	refilling atomic.Bool

	// 统计：降级次数，用来回答「分发器到底在不在工作」
	// [EN] Counts fallbacks, to answer whether the dispenser is working at all
	fallbacks atomic.Int64
}

var dispenser atomic.Pointer[idDispenser]

// SetIDAllocator 装配中心分发器，由 bootstrap 在启动阶段调用一次
//
// 不装配时 NewID 等价于 NewSnowflakeID，模板开箱即用不需要 Redis。
// [EN] Without it NewID behaves as NewSnowflakeID, so the template runs without Redis
func SetIDAllocator(alloc BlockAllocator, blockSize int) {
	if alloc == nil {
		return
	}
	if blockSize <= 0 {
		blockSize = defaultBlockSize
	}
	mark := blockSize / watermarkRatio
	if mark < refillWatermark {
		mark = refillWatermark
	}
	dispenser.Store(&idDispenser{
		alloc:   alloc,
		size:    blockSize,
		mark:    mark,
		waiters: make(chan struct{}),
	})
}

// NewID 取一个主键
//
// 装配了中心分发器就从号段缓冲取，否则退回本地 Snowflake。
//
// 关键点：这个函数永不失败。ID 分配在写路径的最前面，让它能返回 error 的话，
// 分发器一抖整条写链路就全线 500；而退回 Snowflake 的后果仅仅是
// 「这段时间的 ID 换了一种形态，且重新面对 worker id 碰撞的概率」，
// 远轻于直接拒绝写入。降级会记日志和计数，不是静默的。
//
// [EN] This function never fails. Id allocation sits at the very front of the write path, so an
// error return would turn any allocator blip into a wave of 500s, whereas falling back merely
// changes the id's shape for a while and reaccepts the worker-id collision risk, which is far
// milder than refusing writes. The fallback is logged and counted rather than silent.
func NewID() string {
	d := dispenser.Load()
	if d == nil {
		return NewSnowflakeID()
	}
	if id, ok := d.next(); ok {
		return id
	}
	if n := d.fallbacks.Add(1); n == 1 || n%1000 == 0 {
		// 不是每次都打：分发器挂掉时这里会被打爆，日志量本身就成了故障
		//
		// LOGGER 要判空：models 是最底层，除了服务进程，测试、迁移工具、
		// 离线脚本都会用到它，而那些场景没跑过网关的启动流程，全局日志器是 nil。
		// 不判空的话这条降级路径一走就 panic——而它恰恰是出故障时才走的那条，
		// 等于在故障现场再加一个 panic。
		//
		// [EN] Not every time: during an outage this would flood the log and become the
		// incident. The nil check matters because models is the bottom layer and is used by
		// tests, migration tools and offline scripts that never run the gateway's startup, so
		// the global logger is nil there. Without it this path panics, and it is precisely the
		// path taken when something is already wrong.
		if gwglobal.LOGGER != nil {
			gwglobal.LOGGER.Warn("ID 分发器不可用，已降级为本地 Snowflake（累计 %d 次）"+
				" | id dispenser unavailable, fell back to local snowflake (%d times)", n, n)
		}
	}
	return NewSnowflakeID()
}

// next 从缓冲取一个
func (d *idDispenser) next() (string, bool) {
	// 最多三轮，每轮要么取到、要么推进一步（自己补一段 / 等别人补完）。
	// 有上限是关键：没有上限的话，分发器变慢时所有请求都会卡在这个循环里，
	// 把「ID 分配慢」放大成「写接口全部超时」。
	//
	// [EN] At most three passes, each either taking an id or making progress by refilling or by
	// waiting for an in-flight refill. The bound is the point: without it a slowing allocator
	// pins every request in this loop and turns slow id allocation into blanket write timeouts.
	for attempt := 0; attempt < 3; attempt++ {
		d.mu.Lock()
		if n := len(d.buf); n > 0 {
			id := d.buf[n-1]
			d.buf = d.buf[:n-1]
			remaining := n - 1
			d.mu.Unlock()
			if remaining <= d.mark {
				d.refillAsync()
			}
			return id, true
		}
		// 在锁内取走当前的广播通道：锁外再取可能拿到补段完成后换上的新通道，
		// 那就会错过这一次唤醒，白等一个超时
		// [EN] Take the broadcast channel under the lock: fetching it outside could yield the
		// channel installed after the refill completed, missing the wakeup and waiting out the
		// timeout for nothing
		wait := d.waiters
		d.mu.Unlock()

		// 没人在补就自己补，同步等这一次
		// [EN] Refill synchronously when nobody else is
		if d.refilling.CompareAndSwap(false, true) {
			ok := d.fetch()
			d.refilling.Store(false)
			if !ok {
				// 分发器真的失败了，不必再试：立刻降级
				// [EN] The allocator genuinely failed; degrade at once rather than retry
				return "", false
			}
			continue
		}

		// 已有补段在飞，等它，但不无限等
		// [EN] A refill is in flight: wait for it, bounded
		timer := time.NewTimer(waitForRefill)
		select {
		case <-wait:
			timer.Stop()
		case <-timer.C:
			return "", false
		}
	}
	return "", false
}

// refillAsync 异步补段，正常流量下走的是这条
func (d *idDispenser) refillAsync() {
	if !d.refilling.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer d.refilling.Store(false)
		d.fetch()
	}()
}

// fetch 真正去申领一段
func (d *idDispenser) fetch() bool {
	// 自己造 ctx 而不是用请求的：号段是进程级资源，不属于某一次请求。
	// 用请求 ctx 的话，那个请求一取消，正在补的段就废了，
	// 而这一段本来是要给后面成百上千个请求用的。
	//
	// [EN] A context of its own rather than the request's: a block is a process-level resource,
	// not part of one request. With the request's context, its cancellation would discard a
	// block meant for hundreds of later requests.
	ctx, cancel := context.WithTimeout(context.Background(), allocTimeout)
	defer cancel()

	ids, err := d.alloc(ctx, d.size)
	if err != nil || len(ids) == 0 {
		return false
	}

	d.mu.Lock()
	d.buf = append(d.buf, ids...)
	// 唤醒所有等待者并换上新通道，供下一轮等待使用
	// [EN] Wake every waiter and install a fresh channel for the next round
	close(d.waiters)
	d.waiters = make(chan struct{})
	d.mu.Unlock()
	return true
}

// IDDispenserStats 号段缓冲的运行状态，挂在运维接口上
//
// buffered 长期贴近 0 且 fallbacks 持续涨，说明分发器实际没在工作，
// 服务一直在用本地 Snowflake——功能正常，所以不看这两个数就发现不了。
//
// [EN] A buffered count that stays near zero while fallbacks keep climbing means the dispenser
// is not actually working and the service has been on local Snowflake all along. Nothing else
// reveals it, because everything keeps functioning.
func IDDispenserStats() map[string]any {
	d := dispenser.Load()
	if d == nil {
		return map[string]any{"enabled": false}
	}
	d.mu.Lock()
	buffered := len(d.buf)
	d.mu.Unlock()
	return map[string]any{
		"enabled":    true,
		"buffered":   buffered,
		"block_size": d.size,
		"fallbacks":  d.fallbacks.Load(),
	}
}
