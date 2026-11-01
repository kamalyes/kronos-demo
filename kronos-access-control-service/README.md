# kronos-access-control-service

**简体中文** | [English](./README.en.md)

网关鉴权服务：实现协议中心 `access_control` 模块的 `AuthService.Authorities`。

> ⚠️ **`service/auth` 是桩实现**：不验签、不查吊销、不看过期,任何字符串都算有效 token。它在生产级别环境会**拒绝启动**。替换成真实实现的位置见下。

## 它在架构里的位置

网关是唯一讲 HTTP 的进程,它在把请求转发给下游之前先问这里:

```
浏览器 ──HTTP──▶ 网关 ──Authorities(path, method)──▶ 本服务
                  │                                    │
                  │◀────── allowed + Payload ──────────┘
                  │
                  └──gRPC（Payload 注入 header）──▶ 业务服务 / key-service
```

三个服务模板的分工:

| 模板 | 角色 | gRPC | 自己的 HTTP |
|---|---|---|---|
| `kronos-service-template` | 业务服务（有库有缓存） | ✅ | ✅ |
| `kronos-key-service-template` | 基础设施（密钥与发号） | ✅ | ❌ |
| **本模板** | 鉴权 | ✅ | ❌（刻意） |
| `kronos-gateway-template` | HTTP 面 | ❌ | ✅ |

## 两条关键约束

### Authorities 刻意没有 HTTP 路由

协议里它**没有** `google.api.http` 注解。给它一个 HTTP 路由意味着任何能打到网关的人都能直接调鉴权接口,而它的返回值里带着 `Payload`（用户身份）—— 那等于提供一个「拿 token 换身份信息」的公开端点,且绕过所有业务鉴权。

所以本服务连 `RebuildHTTPGateway` 都不需要,它没有任何 HTTP 面。

### 下游服务不重复验 token

验一次就够。验两次意味着两处逻辑要永远一致,而它们迟早不一致 —— 不一致时「网关放行、下游拒绝」是好结果,「网关拒绝、下游放行」才是事故。

**这条约束的前提是下游服务不能直接暴露给外网。** 它们信任 header 里的 `Payload`,谁能直连它们谁就能伪造任意用户。部署时靠网络策略保证,不是靠代码 —— 这一点必须写明,否则「下游不验 token」会被当成疏漏而不是设计。

## 替换成真实实现

`service/auth/auth_service.go` 里有三个 `TODO`,按顺序:

| # | 要做什么 | 做错的后果 |
|---|---|---|
| 1 | 解析并校验 token（签名、过期、吊销） | 失败时必须返回 `TokenValid=false`（401）。返回 `Allowed=false` + `TokenValid=true` 会让前端以为是权限不足而不跳登录 |
| 2 | 查用户与角色,填齐 `Payload` | 漏填 `tenant_id` 会让下游的租户隔离**全部失效** —— 下游拿到空串,而空租户在多数实现里等于「不过滤」 |
| 3 | 用策略引擎判断 `(user, path, method)` | 拒绝时 `TokenValid` 要保持 `true`,网关据此返回 403 而不是 401 |

`AuthoritiesPolicy` 的 `v1`/`v2` 就是为了直接喂 Casbin 的 enforcer 而设计的,所以第 3 步通常是一行 `enforcer.Enforce(roleCode, v1, v2)`。

替换完之后:

1. 删掉 `bootstrap/server.go` 里那段生产环境拒绝启动的检查
2. 把 `resources/` 里的 `cache` 段加回来 —— `Authorities` 是整条链路上**最热的 RPC**（网关每个请求都调它）,token 吊销名单、用户角色、策略表都必须走缓存而不是每次查库

## 免鉴权白名单

`constants/service_name.go` 里的 `publicPaths`,只放真正不该要凭据的:健康检查、文档站、指标。

**不要往这里加业务路径。** 白名单是绕过鉴权的后门,加一条就多一个永远不会被鉴权覆盖的入口,而且因为它工作正常,没人会回头检查它该不该在这儿。业务接口的免鉴权需求应该用策略引擎里的「匿名角色」表达。

前缀匹配有个已知副作用:`/health` 会命中 `/healthcheck-admin`。测试里把这条也断言了 —— 不是因为它对,而是因为它是当前行为,改白名单实现时这条测试会提醒你顺手修掉它。

`IsPublicPath` 显式跳过空前缀。这是白名单最危险的失效模式:一个空串让 `strings.HasPrefix` 对任何路径都返回 `true`,于是整个服务免鉴权,而所有接口都「正常工作」,没有任何报错。`TestIsPublicPathRejectsEmptyPrefix` 守住那一行。

## 快速开始

```bash
kopy new https://github.com/kamalyes/kronos-access-control-template -o ./kronos-access-control-service
cd kronos-access-control-service
go run .
```

桩实现没有任何外部依赖 —— 不需要数据库,也不需要 Redis。

```bash
# 验证 reflection（网关靠它发现本服务）
grpcurl -plaintext localhost:9390 list

# 免鉴权路径：不带 token 也放行
grpcurl -plaintext \
  -d '{"policy":{"v1":"/health","v2":"GET"}}' \
  localhost:9390 kronos.api.access_control.AuthService/Authorities

# 业务路径不带 token：allowed=false, token_valid=false → 网关返回 401
grpcurl -plaintext \
  -d '{"policy":{"v1":"/v1/products","v2":"GET"}}' \
  localhost:9390 kronos.api.access_control.AuthService/Authorities

# 带上任意 token：桩一律放行,并返回固定身份
grpcurl -plaintext \
  -H 'authorization: Bearer whatever' \
  -d '{"policy":{"v1":"/v1/products","v2":"GET"}}' \
  localhost:9390 kronos.api.access_control.AuthService/Authorities
```

## 一个已知的权衡

`service/auth` 嵌入了 `pb.UnimplementedAuthServiceServer`,所以协议中心给 `AuthService` 加新 RPC 时**不会**编译失败,而是运行时返回 `codes.Unimplemented` —— 那个错误在网关侧看起来像「AC 服务挂了」。

去掉嵌入就能在编译期拦住,但那样协议每加一个 RPC 都会立刻让这个模板构建失败,对一个桩来说太脆。选了后者,代价写在这里。

## 服务名必须与网关配置一致

`constants.ServiceName` 同时是三样东西:配置文件前缀、本服务的注册名、以及**网关 `grpc.clients` 里的 key**。网关靠服务名从连接池取连接来调 `Authorities`,对不上的现象是网关每个请求都报 `auth service connection is nil`,而本服务明明活着。
