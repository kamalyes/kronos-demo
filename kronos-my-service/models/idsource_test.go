/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 18:10:33
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:30:27
 * @FilePath: \kronos-my-service\models\idsource_test.go
 * @Description: 号段分发的缓冲、降级与并发行为
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// 这组测试为什么不需要 Redis
//
// 分发器的接口只有 BlockAllocator 一个函数，所以把它换成一个计数的假实现，
// 就能把「缓冲、补段、降级、并发」这些真正容易写错的分支全测到，
// 而不必起一个 Redis。反过来说，如果这些逻辑耦合在 Redis 调用里，
// 它们就只能靠线上来验证——这也是把 allocator 抽成一个函数类型的理由之一。
//
// [EN] The dispenser's only seam is the BlockAllocator function, so a counting fake covers
// buffering, refill, fallback and concurrency without a Redis. Had that logic been entangled
// with the Redis call it could only be verified in production, which is part of why the
// allocator is a function type.

// resetDispenser 每个用例独立起点
// [EN] A fresh starting point per test
func resetDispenser(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { dispenser.Store(nil) })
	dispenser.Store(nil)
}

// countingAllocator 记录被调用次数与申领总量
func countingAllocator() (BlockAllocator, *atomic.Int64, *atomic.Int64) {
	var calls, issued atomic.Int64
	var mu sync.Mutex
	seq := 0
	return func(ctx context.Context, n int) ([]string, error) {
		calls.Add(1)
		issued.Add(int64(n))
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, 0, n)
		for i := 0; i < n; i++ {
			seq++
			out = append(out, fmt.Sprintf("blk-%06d", seq))
		}
		return out, nil
	}, &calls, &issued
}

func TestNewIDWithoutDispenserFallsBackToSnowflake(t *testing.T) {
	resetDispenser(t)
	id := NewID()
	if id == "" {
		t.Fatal("未装配分发器时 NewID 不该返回空")
	}
	// 没装配时不应该有任何号段痕迹
	// [EN] No block-allocated id should appear when nothing is assembled
	if len(id) >= 4 && id[:4] == "blk-" {
		t.Errorf("未装配分发器却返回了号段 ID: %s", id)
	}
}

func TestNewIDServesFromBlockBuffer(t *testing.T) {
	resetDispenser(t)
	alloc, calls, _ := countingAllocator()
	const block = 100
	SetIDAllocator(alloc, block)

	// 取远少于一个号段的量，应该只触发一次申领——
	// 这正是号段模式的全部意义：N 条记录 1 次远程调用，而不是 N 次
	// [EN] Far fewer ids than one block must trigger a single allocation, which is the whole
	// point of the block pattern: one remote call per N rows rather than N calls
	for i := 0; i < block/2; i++ {
		if NewID() == "" {
			t.Fatal("NewID 返回空")
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("取 %d 个 ID 期望 1 次申领，实际 %d 次", block/2, got)
	}
}

func TestNewIDUniqueUnderConcurrency(t *testing.T) {
	resetDispenser(t)
	alloc, _, _ := countingAllocator()
	SetIDAllocator(alloc, 64)

	const goroutines, each = 32, 100
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*each)

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			local := make([]string, 0, each)
			for i := 0; i < each; i++ {
				local = append(local, NewID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()

	// 唯一性是这个组件唯一不能妥协的性质：重复主键会让插入失败，
	// 而且是偶发的、与并发度相关的，线上最难查的那一类
	// [EN] Uniqueness is the one property that cannot be compromised: a duplicate key fails the
	// insert intermittently and in proportion to concurrency, the hardest kind to diagnose
	if len(seen) != goroutines*each {
		t.Errorf("并发取号出现重复：期望 %d 个互不相同，实际 %d 个",
			goroutines*each, len(seen))
	}
}

func TestNewIDFallsBackWhenAllocatorFails(t *testing.T) {
	resetDispenser(t)
	var calls atomic.Int64
	SetIDAllocator(func(ctx context.Context, n int) ([]string, error) {
		calls.Add(1)
		return nil, errors.New("分发器不可用")
	}, 50)

	// 分发器全程失败时 NewID 仍必须返回可用 ID：
	// ID 分配在写路径最前面，让它失败等于整条写链路跟着挂
	// [EN] NewID must still return a usable id while the allocator fails throughout: id
	// allocation sits at the front of the write path and its failure would take every write down
	for i := 0; i < 10; i++ {
		if NewID() == "" {
			t.Fatal("分发器失败时 NewID 返回了空")
		}
	}
	if calls.Load() == 0 {
		t.Error("期望至少尝试过申领")
	}
	stats := IDDispenserStats()
	if fb, _ := stats["fallbacks"].(int64); fb == 0 {
		t.Error("降级次数未被计数——降级必须可观测，否则没人知道分发器没在工作")
	}
}

func TestRefillDoesNotStampede(t *testing.T) {
	resetDispenser(t)
	// 慢 allocator 放大并发补段的窗口
	// [EN] A slow allocator widens the window for concurrent refills
	var inFlight, maxInFlight atomic.Int64
	alloc, calls, _ := countingAllocator()
	SetIDAllocator(func(ctx context.Context, n int) ([]string, error) {
		cur := inFlight.Add(1)
		for {
			old := maxInFlight.Load()
			if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
				break
			}
		}
		defer inFlight.Add(-1)
		return alloc(ctx, n)
	}, 60)

	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				NewID()
			}
		}()
	}
	wg.Wait()

	// 同一时刻只能有一个补段在飞。没有这个保证的话，水位一破
	// 所有并发请求各自申领一段，一瞬间几十段，绝大部分还没用就随进程退出丢掉，
	// 既浪费号也把分发器打成尖峰
	// [EN] Only one refill may be in flight. Without that guarantee, the moment the watermark
	// breaks every concurrent request allocates its own block, dozens at once, most discarded
	// unused at exit: it wastes ids and spikes the allocator's load
	if got := maxInFlight.Load(); got > 1 {
		t.Errorf("出现并发补段：最高同时 %d 个申领在飞，期望 1 个", got)
	}

	// 降级比例要低。缓冲见底时如果立刻降级，这里会有很大比例的 ID
	// 来自本地 Snowflake——而 Snowflake 的 worker id 碰撞正是引入分发器
	// 要避开的东西，降级率高等于分发器白装。
	// 见底多半只是补段还在路上，短暂等一下就能拿到号段里的 ID。
	//
	// [EN] The fallback ratio must stay low. Degrading the moment the buffer empties would send
	// a large share of ids to the local Snowflake, whose worker-id collisions are exactly what
	// the dispenser exists to avoid, so a high ratio means it was installed for nothing. An
	// empty buffer usually just means the refill is still in flight.
	const total = 50 * 20
	stats := IDDispenserStats()
	fallbacks, _ := stats["fallbacks"].(int64)
	if fallbacks*10 > int64(total) {
		t.Errorf("降级比例过高：%d/%d 走了本地 Snowflake，期望低于 10%%", fallbacks, total)
	}
	t.Logf("%d 次取号：申领 %d 次（号段 60），降级 %d 次", total, calls.Load(), fallbacks)
}
