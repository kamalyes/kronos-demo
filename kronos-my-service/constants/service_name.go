/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\constants\service_name.go
 * @Description: 服务名称常量
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package constants

// ServiceName 服务名称
const ServiceName = "kronos-my-service"

// 远程服务名 | Remote service names
//
// 这里的字符串必须与 resources/kronos-my-service-dev.yaml 里 grpc.clients
// 的 key 完全一致——连接池是按服务名索引的，对不上就是「配了也拿不到连接」，
// 而且 InitClient 只返回 ok=false，没有任何报错，表现为那条调用静默退化成
// 「下游未配置」。所以两边都引用同一个常量，别在任何一侧写字面量。
//
// [EN] These strings must match the keys under grpc.clients in the resources file exactly: the
// connection pool is indexed by service name, and a mismatch means the client is configured yet
// never resolves. InitClient merely returns false with no error, so the call silently degrades
// to "downstream not configured". Reference this constant on both sides rather than writing the
// literal anywhere.
const (
	// RemoteServiceKey 中心发号 / 发密钥服务
	//
	// 由 kronos-key-service-template 生成，实现协议中心 key 模块的
	// IdBlockService（ID 号段）与 TempKeyService（临时 RSA 密钥）。
	// 本服务目前只用到前者，见 bootstrap/idgen.go。
	//
	// [EN] Generated from kronos-key-service-template and implementing the contract hub's key
	// module: IdBlockService for id blocks and TempKeyService for temporary RSA keys. Only the
	// former is used here, in bootstrap/idgen.go.
	RemoteServiceKey = "kronos-key-service"

	// SecondRemoteServiceKey 第二下游，实现协议中心 inventory 模块的
	// WarehouseService
	//
	// 由混合模块的同步 worker 调用，用来演示服务间 gRPC 调用，
	// 见 service/tenant/tenant_sync_worker.go
	//
	// [EN] The second downstream implementing the contract hub's WarehouseService in
	// the inventory module, called by the mixed module's sync worker as a
	// service-to-service example
	SecondRemoteServiceKey = "kronos-inventory-service"
)
