/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\bootstrap\kvcache.go
 * @Description: 注册 go-cachex 缓存（字段级 KV + 整实例）
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package bootstrap

import (
	"context"
	cachex "github.com/kamalyes/go-cachex"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"

	"github.com/kamalyes/kronos-my-service/constants"
	"github.com/kamalyes/kronos-my-service/models"
)

// registerKvCache 注册缓存
//
// ============================================================================
// 两个组件，解决两类不同的问题，不要混用
// ============================================================================
//
// ModelKVCache（字段级，NewModelKV）：缓存「主键 → 某个字段」，如 id→name、id→code。
//
//	专治列表接口的 N+1 回显：拿到一页 100 条记录里的 100 个 tenant_id，
//	要显示租户名，不缓存就是 100 次查询或一次 IN 查询加一轮拼装。
//	读用 GetFieldMany(ctx, "Name", ids) 一次拿回 map[id]name。
//
//	代价：每个缓存字段都注册了一个全量 loader，到 KVCacheRefreshInterval 就
//	SELECT 整张表重建本地映射。所以只给「有界的字典表」注册——租户、仓库、
//	分类这种总量可控的。给会无限增长的业务表注册，等于每隔几分钟扫一次大表。
//
// ModelCache（整实例，NewModelCache）：缓存「复合业务键 → 整条记录」。
//
//	读走本地 → Redis Hash → 按键精准回源 DB 三层兜底，不做全表加载，
//	所以对大表也安全。适合「按 (tenant_id, id) 取整条配置/实体」这类热点单条读。
//
//	代价：本地那层只有 TTL 淘汰，没有条数上限。键空间极大的表会把内存吃掉，
//	这种场景要么别缓存整实例，要么把 TTL 压短。
//
// 两者都带写后失效广播：Set/Delete 会通过 PubSub 通知其他副本删本地缓存。
// 这是自己用 map 做本地缓存最容易漏的一环——单副本测不出来，
// 多副本上线后表现为「改完有的节点生效有的不生效」。
//
// [EN] Two components for two different problems. ModelKVCache caches primary-key-to-field
// pairs such as id-to-name and exists to kill the N+1 lookup on list endpoints, read in bulk
// with GetFieldMany; its cost is a full-table SELECT per cached field on every refresh tick, so
// it suits bounded reference tables only. ModelCache caches whole rows by a composite business
// key with local, Redis and precise per-key database fallback and no full load, so it is safe
// for large tables; its cost is a local layer bounded only by TTL, not by entry count. Both
// broadcast invalidation over PubSub on write, which is the part a hand-rolled local map
// always misses: it cannot be reproduced on one replica and shows up in production as an update
// taking effect on some nodes but not others.
//
// ============================================================================
// 注册名冲突
// ============================================================================
// 同一张表不能注册两套 ModelKV：注册名只按 表名 + 字段名 生成，不受 namespace 隔离。
// 需要为同一张表建另一套索引（比如用 code 反查而不是用主键），
// 要用 cachex.RegisterKV 自己起一个唯一名字，别再 NewModelKV 一次。
//
// [EN] One table cannot carry two ModelKV registrations: the registration name is derived from
// table plus field and is not isolated by namespace. A second index over the same table, such
// as a lookup by code rather than by primary key, needs cachex.RegisterKV under a name of its
// own.
func (s *MyServiceGateway) registerKvCache() {
	// Redis 没配就直接返回：RegisterKV 在全局客户端为 nil 时会 panic，
	// 而「本地起服务不开 Redis」是很常见的开发姿势，不该因此起不来。
	// 读路径用的是非 panic 的 GetModelKV/GetModelCache，拿不到就退回直查数据库，
	// 所以跳过注册只是失去缓存，功能不受影响。
	//
	// [EN] Return early when Redis is absent: RegisterKV panics on a nil global client, and
	// running locally without Redis is common enough that it must not prevent startup. The read
	// paths use the non-panicking GetModelKV/GetModelCache and fall back to a direct query, so
	// skipping registration costs the cache and nothing else.
	if gwglobal.REDIS == nil {
		gwglobal.LOGGER.Warn("Redis 未配置，跳过缓存注册（将全部直查数据库）| Redis not configured, cache registration skipped")
		return
	}

	cachex.SetGlobalRedisClient(gwglobal.REDIS)
	cachex.SetLogger(gwglobal.LOGGER)

	// ------------------------------------------------------------------
	// tenant 模块：聚合根是有界的（租户总量可控），既做字段级又做整实例
	// [EN] The tenant aggregate root is bounded, so both cache kinds apply
	// ------------------------------------------------------------------
	tenantBase := cachex.NewModelKVBase().
		Namespace(constants.KVNamespaceTenant).
		TTL(constants.TTLModelCache).
		RefreshInterval(constants.KVCacheRefreshInterval)

	// id → name / code：跨模块回显与存在性校验都走这里，省掉一次查询
	// 主键由 gorm schema 自动识别，无需 KeyField
	// [EN] id to name and code, used for cross-module display and existence checks; the primary
	// key is taken from the gorm schema
	cachex.NewModelKV[models.TenantModel](tenantBase, gwglobal.DB).
		Fields("Name", "Code").
		Register()

	// (tenant_id, id) → 整条成员记录
	cachex.NewModelCache[models.TenantMemberModel](tenantBase, gwglobal.DB).
		KeyFields("TenantId", "Id").
		Register()

	// ------------------------------------------------------------------
	// inventory 模块：按租户分组的字典表，总量有界
	// [EN] A per-tenant reference table, bounded in size
	// ------------------------------------------------------------------
	inventoryBase := cachex.NewModelKVBase().
		Namespace(constants.KVNamespaceInventory).
		TTL(constants.TTLModelCache).
		RefreshInterval(constants.KVCacheRefreshInterval)

	cachex.NewModelKV[models.WarehouseModel](inventoryBase, gwglobal.DB).
		Fields("Name", "Code").
		Register()

	cachex.NewModelCache[models.WarehouseModel](inventoryBase, gwglobal.DB).
		KeyFields("TenantId", "Id").
		Register()

	// ------------------------------------------------------------------
	// catalog 模块：业务主表，会随业务无限增长
	//
	// 这里故意只注册 ModelCache，不注册 ModelKV：
	// ModelKV 的全量刷新会每隔 KVCacheRefreshInterval 扫一次整张表，
	// 对一张持续增长的表来说这是纯粹的浪费，而且随数据量恶化。
	// 需要按 id 批量取名字时，老老实实走一次 IN 查询——
	// 一次有界的查询比定期全表扫描便宜得多。
	//
	// [EN] Deliberately ModelCache only, no ModelKV: the full refresh would scan this growing
	// table on every tick, which is pure waste and gets worse with size. When names are needed
	// in bulk here, one bounded IN query is far cheaper than a periodic full scan.
	// ------------------------------------------------------------------
	catalogBase := cachex.NewModelKVBase().
		Namespace(constants.KVNamespaceCatalog).
		TTL(constants.TTLModelCache)

	cachex.NewModelCache[models.ProductModel](catalogBase, gwglobal.DB).
		KeyFields("TenantId", "Id").
		Register()

	// ------------------------------------------------------------------
	// 反向索引：code → id
	//
	// 为什么不能再 NewModelKV[TenantModel] 一次：ModelKV 的注册名是
	// 「表名:字段名」派生的，不含 namespace。上面已经注册过
	// tenants:Name 和 :Code，再注册一次同表同字段会直接 panic
	// （「already registered」）。而且方向也不对——ModelKV 是「主键 → 字段」，
	// 这里要的是「字段 → 主键」，是反过来的。
	//
	// 所以用 cachex.RegisterKV 自己起一个唯一名字，配一个全量 loader。
	//
	// 这里的键是 code，前提是 code 全局唯一（协议中心里它就是这么定义的）。
	// 千万不要拿「租户内唯一」的列做这种全局索引：一张表一个 Redis Hash，
	// 不区分租户，两个租户用了同一个 code 就会互相覆盖，
	// 现象是 A 租户按 code 查出了 B 租户的 ID——越权读，而且不报错。
	//
	// [EN] A second NewModelKV on the same table would panic, since the registration name is
	// derived from table plus field and carries no namespace, and the direction is wrong anyway:
	// ModelKV maps primary key to field, while a reverse index maps field to primary key. Hence
	// RegisterKV under a name of its own with a full loader. Keying by code assumes code is
	// globally unique, as the contract defines it. Never build such a global index on a column
	// that is only unique per tenant: one table is one Redis hash with no tenant dimension, so
	// two tenants sharing a code overwrite each other and one tenant resolves the other's id,
	// which is a cross-tenant read that raises no error.
	// ------------------------------------------------------------------
	cachex.RegisterKV[string, string](
		constants.KVCacheTenantIDByCode,
		loadTenantIDByCode,
		cachex.WithKVNamespace(constants.KVNamespaceTenant),
		cachex.WithKVTTL(constants.TTLModelCache),
		cachex.WithKVRefreshInterval(constants.KVCacheRefreshInterval),
	)

	// 启动预热：从数据库重新加载并原子替换所有带全量 loader 的缓存
	//
	// 不预热会怎样：Redis 里可能还留着上一个版本留下的数据。本地缓存是空的，
	// 第一次读会从 Redis 拿——拿到的是旧值。而自动刷新要等一个
	// KVCacheRefreshInterval 才发生，于是发布后的几分钟里接口在拿旧数据，
	// 期间一切「正常」，没有任何报错。数据迁移之后这个窗口尤其致命。
	//
	// 为什么是 WarmupAllKV 而不是自己 Clear 再 Refresh：Clear+Refresh 在
	// Redis DEL 之后、HSET 之前有一段空窗，这期间所有节点全部 cache miss，
	// 一起回源 → DB 雪崩。WarmupAllKV 走的是 RefreshAndSwap：先把新数据从库里
	// 读出来，再用一条 Lua 脚本原子替换（DEL+HSET+EXPIRE），Redis 从旧值直接
	// 变成新值，没有空窗，最后广播让其他副本清本地缓存重读。
	//
	// 只影响带全量 loader 的缓存（ModelKV 的字段缓存、上面那个反向索引）。
	// ModelCache 注册时 loader 是 nil，是按需回源的，会被安静跳过。
	//
	// 超时要留足：预热会把每张注册过的表扫一遍。超时了只是预热不完整
	// （缓存退化成按需加载），不影响启动，所以这里不处理返回值。
	//
	// [EN] Without warmup, Redis may still hold data from the previous release while the local
	// cache is empty, so the first read fetches a stale value from Redis and the automatic
	// refresh is a whole interval away: for minutes after a deploy the endpoints serve old data
	// with everything apparently fine and nothing logged, which is especially damaging right
	// after a data migration. WarmupAllKV rather than a manual Clear plus Refresh, because the
	// latter leaves a window between the Redis DEL and the HSET during which every node misses
	// and reloads at once, stampeding the database. WarmupAllKV uses refresh-and-swap: the new
	// data is read first and then installed by one Lua script performing DEL, HSET and EXPIRE,
	// so Redis moves from old value to new with no window, after which other replicas are told
	// to drop their local copies. Only caches with a full loader are affected; ModelCache
	// registers a nil loader and is skipped. The timeout must be generous since every
	// registered table is scanned, and exceeding it merely leaves the warmup incomplete, with
	// the caches degrading to on-demand loading, so the result is deliberately not fatal.
	warmupCtx, cancel := context.WithTimeout(context.Background(), constants.CacheWarmupTimeout)
	defer cancel()
	_ = cachex.WarmupAllKV(warmupCtx)

	gwglobal.LOGGER.Info("Cache registered")
}

// loadTenantIDByCode 全量构建 code → id 映射
//
// 只 Select 用到的两列，不要 SELECT *：这张表会被整表扫，多取的每一列都要
// 过一遍网络和反序列化。字典表的行数不大，但列可能很宽（JSON 扩展字段之类）。
//
// 过滤掉空 code：空串做键没有意义，而且会和「查不到」混在一起分不清。
//
// [EN] Only the two columns in use are selected rather than everything, because the table is
// scanned in full and each extra column pays network and decoding cost; a reference table has
// few rows but can have wide ones. Empty codes are filtered out, since an empty key is
// meaningless and would be indistinguishable from a miss.
func loadTenantIDByCode(ctx context.Context) (map[string]string, error) {
	var rows []struct {
		Id   string
		Code string
	}
	if err := gwglobal.DB.WithContext(ctx).
		Model(&models.TenantModel{}).
		Select("id", "code").
		Where("code <> ?", "").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Code] = r.Id
	}
	return out, nil
}
