/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\constants\cache.go
 * @Description: 缓存命名空间与时长常量
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package constants

import "time"

// 缓存命名空间 | Cache namespaces
//
// 命名空间会拼进 Redis 键，用途是在同一个 Redis 实例上隔离键空间。
// 带上服务名是必须的：多个服务共用一套 Redis 时，只写 "tenant" 这种模块名
// 会让两个服务的同名表互相覆盖，而且覆盖的现象是「读到别的服务的数据」，
// 不是报错，排查起来极其费时。
//
// [EN] The namespace is part of the Redis key and isolates key space on a shared Redis
// instance. Including the service name is not optional: with several services on one Redis, a
// bare module name lets two services with a same-named table overwrite each other, and the
// symptom is reading another service's data rather than an error.
const (
	KVNamespaceCatalog   = "kronos-my-service:catalog"
	KVNamespaceInventory = "kronos-my-service:inventory"
	KVNamespaceTenant    = "kronos-my-service:tenant"
)

// KVCacheTenantIDByCode code → id 反向索引的注册名
//
// 自定义 RegisterKV 必须自己起名，而 ModelKV 的名字是「表名:字段名」自动派生的。
// 这个名字要避开所有自动派生出来的名字，否则注册时 panic。
// 带上 by_code 这种方向标识，比叫 tenants 清楚得多。
//
// [EN] A custom RegisterKV names itself, while ModelKV derives its name from table and field.
// This name must avoid every derived one or registration panics; spelling out the direction is
// clearer than reusing the table name.
const KVCacheTenantIDByCode = "tenants:by_code"

// 缓存时长 | Cache durations
const (
	// TTLModelCache 整实例缓存的 TTL
	//
	// 写路径会主动失效，TTL 只是漏删时的兜底，所以不必很短；但也不能很长，
	// 否则一次漏删的影响会持续很久。半小时量级是常见取值。
	//
	// [EN] The write path invalidates explicitly, so the TTL is only a backstop for a missed
	// deletion. It need not be short, but a long one makes one missed deletion linger.
	TTLModelCache = 30 * time.Minute

	// KVCacheRefreshInterval 字段级 KV 缓存的全量刷新间隔
	//
	// 注意这个刷新是「全表 SELECT」：go-cachex 的 ModelKVCache 为每个缓存字段
	// 注册一个全量 loader，到点就把整张表扫一遍重建本地映射。
	// 所以只给有界的字典表注册 ModelKV（租户、仓库、分类这种几千行以内的），
	// 不要给会无限增长的业务表注册，否则每个间隔都在扫大表。
	//
	// [EN] This refresh is a full table SELECT: ModelKVCache registers a full loader per cached
	// field and rebuilds the local map on each tick. Register ModelKV only for bounded reference
	// tables, never for a business table that grows without bound.
	KVCacheRefreshInterval = 5 * time.Minute

	// ModelKVLoadTTL 同一次刷新内多个字段共用一次全表查询的去重窗口
	// [EN] The window in which several fields of one refresh share a single full-table query
	ModelKVLoadTTL = 30 * time.Second

	// CacheWarmupTimeout 启动预热的总超时
	//
	// 预热要把每张注册过 ModelKV 的表扫一遍，所以这个值随表的数量和行数增长。
	// 超时只会让预热不完整（缓存退化为按需加载），不影响启动。
	// 定得太小就等于没预热，而且不会有人注意到。
	//
	// [EN] Warmup scans every table with a registered ModelKV, so this grows with their number
	// and size. Exceeding it leaves the warmup incomplete and the caches on-demand rather than
	// failing startup. Too small a value means no warmup at all, and nobody notices.
	CacheWarmupTimeout = 2 * time.Minute

	// IDSequenceLength 分发器序列号的零填充位数
	//
	// 位数不够不会出错（Go 的 %0*d 超了就自然变长），只是 ID 长度会跳变，
	// 对按长度做校验或截断的下游是个隐患。按单个时间片内的峰值发号量取值。
	//
	// [EN] Too few digits is not an error, since the format simply widens, but the id length
	// then jumps, which trips any downstream that validates or truncates by length.
	IDSequenceLength = 8

	// IDSequenceExpire 序列号键的存活时长
	//
	// 必须大于一个时间片（默认按小时发号），否则当前时间片的键会在用着的时候过期，
	// 序列从头开始，于是同一小时内发出重复 ID。留两倍以上余量。
	//
	// [EN] Must exceed one time bucket, hourly by default, or the key in active use expires, the
	// sequence restarts and duplicate ids follow within the same hour. Leave at least double.
	IDSequenceExpire = 3 * time.Hour

	// IDBlockSize 一次向分发器申领多少个 ID
	// [EN] How many ids are claimed from the dispenser at once
	IDBlockSize = 200

	// RemoteHealthCheckInterval 下游 gRPC 服务的健康探测间隔
	//
	// 这个值决定「下游挂了多久之后调用方才知道」。定得太长，调用方会在一整个
	// 间隔里持续把请求送进一个已经死掉的下游，每次都付一次连接超时；
	// 定得太短，几十个下游就是一片心跳噪声。十秒量级是常见折中。
	//
	// [EN] This sets how long it takes a caller to learn the downstream is gone. Too long and
	// callers keep feeding requests into a dead service, paying a connection timeout each time;
	// too short and a few dozen downstreams become a wall of heartbeats.
	RemoteHealthCheckInterval = 10 * time.Second
)
