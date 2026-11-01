/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\common\cache.go
 * @Description: 跨模块字段级缓存读取（列表回显与存在性校验）
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package common

import (
	"context"

	cachex "github.com/kamalyes/go-cachex"
	"github.com/kamalyes/kronos-my-service/constants"
	"github.com/kamalyes/kronos-my-service/models"
)

// 为什么这些 helper 放在 common 而不是各模块自己写
//
// 「拿一批 ID 换一批名字」是跨模块的需求：catalog 模块的列表要显示
// tenant 的名字，inventory 模块也要。如果各模块自己去读缓存，
// 字段名字符串（"Name"）就散落在各处，哪天协议改名要改一圈，而且改漏的地方
// 不会报错——GetFieldMany 对不存在的字段名直接返回 nil，静默退化成空值。
// 集中在这里，字段名只写一次。
//
// 全部用非 panic 的 GetModelKV：缓存是可选的（Redis 没配时 bootstrap 跳过注册），
// 拿不到就返回 nil，调用方按「查不到」处理。GetFieldMany 本身也是 nil 安全的。
//
// [EN] Turning a batch of ids into a batch of names is a cross-module need, so the field-name
// strings live here once instead of being scattered across modules: GetFieldMany returns nil
// for an unknown field name rather than failing, so a missed rename degrades silently to empty
// values. All lookups use the non-panicking GetModelKV because the cache is optional, and
// GetFieldMany is nil-safe, so an unregistered cache simply yields no values.

// TenantIDByCode 按 code 反查 id，第二个返回值表示答案是否可信
//
// 和 TenantNameLookup 一样的三态语义：cached 为 false 表示缓存不可用，
// 不是「这个 code 不存在」。把两者混为一谈会让无缓存环境下所有按 code 的查询
// 都判为不存在。
//
// 这个索引走的是自定义 RegisterKV（见 bootstrap/kvcache.go），
// 不是 ModelKV——方向是「字段 → 主键」，ModelKV 只能「主键 → 字段」。
//
// [EN] The same three-state contract as the name lookup: a cached value of false means the
// cache is unavailable, not that the code is absent, and conflating the two makes every lookup
// by code fail where no cache is configured. This index comes from a custom RegisterKV rather
// than ModelKV, because the direction is field to primary key and ModelKV only does the reverse.
func TenantIDByCode(ctx context.Context, code string) (string, bool) {
	if code == "" {
		return "", true
	}
	kv, err := cachex.GetKV[string, string](constants.KVCacheTenantIDByCode)
	if err != nil {
		return "", false
	}
	id, found, err := kv.Get(ctx, code)
	if err != nil {
		return "", false
	}
	if !found {
		return "", true
	}
	return id, true
}

// RefreshKV 写后刷新字段级缓存，泛型覆盖所有注册过 ModelKV 的实体
//
// 为什么写路径必须显式维护它：ModelKV 的自动刷新是「到点全量扫表」，
// 间隔内的改动它看不见。只靠刷新间隔兜底，意味着改完名字最长要等一个间隔
// 才在列表里生效——而用户改完名字立刻刷新列表是最常见的操作。
//
// 整实例缓存（ModelCache）不需要在这里管：CachedRepository 的
// UpdateScoped / DeleteScoped 已经负责失效，见 repository/base_cached.go。
// 两者是两套缓存，别指望维护了一个另一个就跟着对。
//
// 错误一律忽略：缓存维护失败不该让已经成功的写操作返回失败。
// 代价是这一条在下次全量刷新前可能是旧值，比「库里写成功了但接口报错」轻得多。
//
// [EN] The write path must maintain this explicitly because ModelKV refreshes by scanning the
// whole table on a timer and cannot see changes within an interval: relying on the timer alone
// means a renamed row takes up to one interval to appear in lists, while refreshing the list
// right after renaming is the most common thing a user does. The whole-instance cache is not
// handled here, since UpdateScoped and DeleteScoped already invalidate it; the two are separate
// caches and maintaining one does not fix the other. Errors are ignored on purpose: a cache
// maintenance failure must not turn a successful write into a failed response.
func RefreshKV[M any](ctx context.Context, m *M) {
	kv, err := cachex.GetModelKV[M]()
	if err != nil {
		return
	}
	_ = kv.Set(ctx, m)
}

// InvalidateKV 删后清除字段级缓存
//
// key 是注册时 KeyField 对应的值，缺省是 gorm 主键，所以通常就是记录 ID。
// [EN] key is the value of the registered KeyField, by default the gorm primary key
func InvalidateKV[M any](ctx context.Context, key string) {
	kv, err := cachex.GetModelKV[M]()
	if err != nil {
		return
	}
	_ = kv.DeleteByKey(ctx, key)
}

// TenantNamesByIDs 批量取 Tenant 名称，返回 map[id]name
//
// 一次 HMGet 换掉列表接口里的 N 次查询。未命中的 id 不出现在返回值里，
// 调用方不能假设 len(结果) == len(入参)。
//
// [EN] One HMGet replaces N queries on a list endpoint. Ids that miss do not appear in the
// result, so callers must not assume the lengths match.
func TenantNamesByIDs(ctx context.Context, ids []string) map[string]string {
	kv, err := cachex.GetModelKV[models.TenantModel]()
	if err != nil {
		return nil
	}
	return kv.GetFieldMany(ctx, "Name", ids)
}

// TenantDisplayByIDs 一次拿回 Name 与 Code 两个字段
//
// 用 GetFieldsMany 而不是调两次 GetFieldMany：前者把 N 个字段的 HMGet 合进一次
// Redis Pipeline，是一次网络往返而不是 N 次。
//
// 代价要知道：GetFieldsMany 跳过本地缓存和回源，只读 Redis，所以未命中的键
// 不会被补上。回显场景可以接受（显示空白比多查一次库划算），
// 需要「一定要有值」的场景请用单字段的 TenantNamesByIDs。
//
// [EN] GetFieldsMany merges the per-field HMGets into one Redis pipeline, one round trip
// instead of N. The trade-off: it reads Redis only, skipping the local layer and the database
// reload, so keys that miss stay missing. That is fine for display, where a blank beats an
// extra query, but a caller that must have a value should use the single-field helper.
func TenantDisplayByIDs(ctx context.Context, ids []string) map[string]map[string]string {
	kv, err := cachex.GetModelKV[models.TenantModel]()
	if err != nil {
		return nil
	}
	return kv.GetFieldsMany(ctx, []string{"Name", "Code"}, ids)
}

// TenantNameLookup 取单个名称，并告知这个答案是否可信
//
// 返回值 cached 为 false 表示「缓存不可用」，不是「记录不存在」——
// 这两件事必须分开，否则就有一个很隐蔽的事故：Redis 没配或没注册时
// 名称恒为空，调用方若直接把空当成不存在，所有引用校验都会判为失败，
// 整个写接口在无缓存环境下全线 404，而日志里什么错都没有。
//
// 所以存在性校验的正确写法是「只在缓存可信且确实查不到时才否决」：
//
//	if name, cached := common.TenantNameLookup(ctx, id); cached && name == "" {
//	    return nil, common.NotFound(ctx)
//	}
//
// 为什么值得用缓存做这件事：外键校验是写接口里最高频的一次查询，
// 而被校验的对象是有界字典表、改动极少，正是字段级缓存最划算的场景。
//
// 新建的记录不会有时间窗问题：创建成功后服务层会立刻 RefreshKV 把它放进缓存
// （见各模块 Create 的收尾），不必等下一次全量刷新。
//
// [EN] A cached value of false means the cache is unavailable, not that the row is absent, and
// conflating the two causes a subtle outage: with Redis unconfigured the name is always empty,
// so a caller treating empty as absent rejects every reference check and the whole write API
// returns not-found with nothing in the logs. An existence check must therefore only reject
// when the cache is trustworthy and the row is genuinely missing. It is worth caching because
// a foreign-key check is the most frequent read on a write path over a small, rarely-changed
// table. Newly created rows are not subject to a window, since the service refreshes the cache
// immediately after a successful create rather than waiting for the next full reload.
func TenantNameLookup(ctx context.Context, id string) (string, bool) {
	kv, err := cachex.GetModelKV[models.TenantModel]()
	if err != nil {
		return "", false
	}
	names := kv.GetFieldMany(ctx, "Name", []string{id})
	if names == nil {
		return "", false
	}
	return names[id], true
}
