/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\idgen.go
 * @Description: ID 生成工具函数 | ID generation helpers
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package models

import (
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/go-toolbox/pkg/safe"
)

// 为什么放在 models 而不是 service | Why this lives in models, not service
//
// 分配主键的时机横跨三层：service 创建实体时要、repository 批量插入时要、
// model 的 BeforeCreate 钩子里也可能要。依赖方向是 service → repository → models，
// 所以它只能待在最底层，放在 service 会让下面两层用不了（import 循环）。
//
// [EN] Assigning a primary key happens across all three layers: in service when creating an
// entity, in repository on bulk insert, and possibly in a model's BeforeCreate hook. The
// dependency direction is service → repository → models, so this must sit at the bottom;
// in service the two lower layers could not use it without an import cycle.

// 三个 ID 来源，分别用在什么地方 | The three id sources and where each belongs
//
//	NewID()             业务实体的主键。装配了中心分发器就走号段，否则退回 Snowflake。
//	                    新建实体一律用这个，不要直接用 NewSnowflakeID。
//	NewSnowflakeID()    本地生成，不依赖外部。NewID 的降级目标，也用于那些
//	                    明确不需要跨实例唯一性保证的场合。
//	NewDeterministicID() 由业务键推导，同 key 恒同值，用于需要幂等的内置数据。
//
// 为什么默认不直接用 Snowflake：见 models/idsource.go 开头，Deployment 下
// worker id 是哈希取模出来的，槽位只有 32 个，副本一多就会撞。
//
// [EN] NewID is the primary key for business entities and is what entity creation should call;
// it uses the dispenser when one is assembled and falls back to Snowflake otherwise.
// NewSnowflakeID is local and dependency-free, serving as that fallback and as the choice where
// cross-instance uniqueness is explicitly not required. NewDeterministicID derives a stable id
// from a business key for idempotent seeding. The reason Snowflake is not the default is at the
// top of models/idsource.go: a Deployment hashes its worker id into only 32 slots.

// NewSnowflakeID 生成一个新的短 ID 字符串
//
// 委托给网关的全局生成器，不自建：网关启动时已按 osx.GetWorkerIdForSnowflake() 与
// osx.GetDatacenterId() 初始化过一个 Snowflake 生成器（启动日志里的「正在初始化:
// Snowflake」）。再建一个参数相同的生成器，两者各有独立的序列计数器，
// 同一毫秒内会发出相同的 ID。
//
// [EN] Delegates to the gateway's global generator instead of building one. The gateway
// already initializes a Snowflake generator from the same worker and datacenter IDs at
// startup; a second generator with identical parameters keeps its own sequence counter and
// can emit the same ID within the same millisecond.
func NewSnowflakeID() string {
	return gwglobal.NewSnowflakeID()
}

// NewDeterministicID 由业务键生成确定性 ID，相同 key 恒得相同结果
//
// 用于需要幂等的场景：初始化内置数据、按「模块:资源:类型」生成固定节点 ID 等。
// 这类记录每次部署都要落成同一个 ID，不能用 Snowflake（它每次都不同）。
//
// [EN] For idempotent cases such as seeding built-in rows or deriving a stable node ID from
// "module:resource:type". Those records must land on the same ID on every deployment, so
// Snowflake (which differs every call) is not usable there.
func NewDeterministicID(key string) string {
	return safe.ShortHash(key)
}
