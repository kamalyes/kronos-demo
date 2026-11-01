/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 00:00:00
 * @FilePath: \kronos-access-control-service\service\auth\auth_service.go
 * @Description: 鉴权服务 —— 可替换的桩实现
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package auth

import (
	"context"
	"strings"

	"github.com/kamalyes/kronos-access-control-service/constants"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/access_control"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	goconfig "github.com/kamalyes/go-config"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	gwmiddleware "github.com/kamalyes/go-rpc-gateway/middleware"
)

// ============================================================================
// 这是一个桩，不是鉴权实现
// ============================================================================
//
// 它存在的目的是让「网关 → 鉴权 → 下游」这条链路能跑通，以及把真实实现
// 该在哪几个位置接什么写清楚。它做的事只有：
//
//   1. 看请求路径是否命中免鉴权白名单 → 放行，不带身份
//   2. 看 metadata 里有没有 authorization → 没有就 401
//   3. 有就当它合法，拿它当 user_id 造一个 Payload → 放行
//
// 第 3 步是整个桩里唯一不能带进生产的一行：它不验签、不查吊销、不看过期，
// 任何字符串都算有效 token。所以非开发环境会直接拒绝启动，见 bootstrap。
//
// 真实实现要替换的位置，代码里用 TODO 标出来了，共三处：
//   解析并校验 token（签名、过期、吊销）
//   查用户与角色，填齐 Payload
//   用策略引擎判断 (user, path, method) 是否放行
//
// [EN] A stub rather than an authorization implementation. It exists to make the gateway,
// authorization and downstream path work end to end and to mark where a real implementation
// plugs in. It only checks the path against an allow list, checks that an authorization header
// exists, and otherwise trusts that header as the user id. That last step is the one line that
// must never reach production: it verifies no signature, no revocation and no expiry, so any
// string counts as a valid token, which is why the service refuses to start outside
// development. The three places a real implementation replaces are marked TODO: parsing and
// verifying the token, loading the user and roles into the payload, and asking a policy engine
// whether the request is allowed.

// AuthService 实现协议中心的 AuthServiceServer
type AuthService struct {
	pb.UnimplementedAuthServiceServer
}

// NewAuthService 构造服务
func NewAuthService() *AuthService { return &AuthService{} }

// Authorities 判断请求是否放行，并返回调用者身份
//
// 网关在**每个** HTTP 请求前都会调它，所以这个方法在整条链路上是最热的一个 RPC，
// 它的延迟直接加在所有接口上。真实实现里策略判断和用户查询都必须走缓存，
// 不要每次查库——这也是为什么协议把它设计成一次调用返回「是否放行 + 完整身份」，
// 而不是让网关先查身份再查权限两次往返。
//
// [EN] The gateway calls this before every HTTP request, making it the hottest RPC on the path
// with its latency added to every endpoint. A real implementation must serve policy decisions
// and user lookups from cache rather than querying a database each time, which is also why the
// contract returns the decision and the full identity in one call instead of making the gateway
// fetch identity and permissions in two round trips.
func (s *AuthService) Authorities(
	ctx context.Context, req *pb.AuthoritiesRequest,
) (*pb.AuthoritiesResponse, error) {
	path := req.GetPolicy().GetV1()
	method := req.GetPolicy().GetV2()

	// 免鉴权白名单：健康检查、文档站这类不该要 token 的路径
	//
	// 白名单用前缀匹配而不是全等：文档站是一组路径（/swagger/*），
	// 健康检查也可能带后缀。但前缀匹配要小心——"/" 会匹配一切，
	// 所以 constants 里对空前缀做了拒绝，见那边的说明。
	//
	// [EN] An allow list for paths that should not require a token, such as health checks and
	// the docs site. Prefix matching rather than equality, since the docs site is a group of
	// paths, but prefix matching needs care: "/" would match everything, so an empty prefix is
	// rejected in constants.
	if constants.IsPublicPath(path) {
		gwglobal.LOGGER.DebugContextKV(ctx, "免鉴权路径放行", "path", path, "method", method)
		return &pb.AuthoritiesResponse{Allowed: true, TokenValid: true}, nil
	}

	// token 从 metadata 取，不从请求体取——协议里刻意没有 token 字段，
	// 原因见协议中心 auth_service.proto 的说明（放请求体会进访问日志和链路 span）
	// [EN] The token comes from metadata, not the body: the contract deliberately has no token
	// field, because a body field lands in access logs and trace spans
	token := strings.TrimSpace(gwmiddleware.GetAuthorization(ctx))
	if token == "" {
		// token_valid=false → 网关返回 401，客户端该去登录
		// [EN] token_valid false makes the gateway answer 401 and the client log in
		return &pb.AuthoritiesResponse{
			Allowed:    false,
			TokenValid: false,
			Reason:     "missing credentials",
		}, nil
	}

	// TODO(真实实现 1/3)：解析并校验 token
	//
	// 要做的事：验签名、查过期、查吊销（jti 黑名单 / family_id 失效）。
	// 校验失败返回 TokenValid=false（401），不要返回 Allowed=false+TokenValid=true，
	// 那会让前端以为是权限不足而不跳登录。
	//
	// 桩的行为：不验，直接把 token 当 user_id 用。
	//
	// [EN] Verify the signature, the expiry and revocation through a jti blacklist or an
	// invalidated family id. A failure returns TokenValid false, which is a 401; returning
	// Allowed false with TokenValid true would tell the frontend it lacks permission and keep
	// it from the login page. The stub skips all of it and uses the token as the user id.
	userID := stubUserIDFromToken(token)

	// TODO(真实实现 2/3)：查用户与角色，填齐 Payload
	//
	// Payload 是下游服务唯一的身份来源（它们不重复验 token），所以这里填什么
	// 下游就信什么。漏填 tenant_id 的后果是下游的租户隔离全部失效——
	// 那些服务拿 GetTenantID() 拿到空串，而空租户在多数实现里等于「不过滤」。
	//
	// [EN] The payload is the only identity source downstream, since those services do not
	// re-verify the token, so whatever is filled here is believed. Omitting tenant_id voids
	// every downstream tenant check, because GetTenantID returns an empty string and an empty
	// tenant usually means no filtering at all.
	payload := &commonpb.Payload{
		UserId:   userID,
		Nickname: userID,
		TenantId: constants.StubTenantID,
		RoleCode: constants.StubRoleCode,
		Domain:   enumspb.DomainType_DOMAIN_TYPE_TENANT,
		IsOps:    false,
	}

	// TODO(真实实现 3/3)：用策略引擎判断 (user, path, method) 是否放行
	//
	// AuthoritiesPolicy 的 v1/v2 就是为了直接喂 Casbin 的 enforcer 而设计的
	// （见协议里的说明），所以这里通常是一行 enforcer.Enforce(roleCode, v1, v2)。
	// 拒绝时 TokenValid 要保持 true——token 是有效的，只是权限不够，
	// 网关据此返回 403 而不是 401。
	//
	// 桩的行为：一律放行。
	//
	// [EN] The policy's v1 and v2 exist to feed a Casbin enforcer directly, so this is usually
	// one Enforce call. On a denial TokenValid stays true, because the token is valid and only
	// the permission is missing, which makes the gateway answer 403 rather than 401. The stub
	// allows everything.
	gwglobal.LOGGER.InfoContextKV(ctx, "鉴权放行（桩实现，未做任何校验）",
		"path", path, "method", method, "user_id", userID)

	return &pb.AuthoritiesResponse{
		Allowed:    true,
		TokenValid: true,
		Payload:    payload,
	}, nil
}

// stubUserIDFromToken 把 token 原样当成 user_id
//
// 只在非生产环境可用——bootstrap 在生产级别环境会拒绝启动，所以走到这里
// 一定是开发/测试环境。即便如此也不要在日志里打完整 token：
// 开发环境的日志同样会被复制、贴进工单、进 CI 产物。
//
// [EN] Available only outside production, since bootstrap refuses to start at production
// level, so reaching here means a development or test environment. Even then the full token
// stays out of the log: development logs are copied, pasted into tickets and kept as CI
// artifacts just the same.
func stubUserIDFromToken(token string) string {
	const maxLen = 32
	id := strings.TrimPrefix(token, "Bearer ")
	id = strings.TrimSpace(id)
	if len(id) > maxLen {
		id = id[:maxLen]
	}
	if id == "" {
		return "stub-user"
	}
	return id
}

// 编译期断言：桩必须始终满足协议接口
//
// 协议中心给 AuthService 加了新 RPC 时，这一行会让编译失败而不是运行时
// 返回 Unimplemented——后者的现象是网关收到 codes.Unimplemented，
// 而那个错误在网关侧看起来像「AC 服务挂了」。
//
// 注意它只在没有嵌入 Unimplemented 时才拦得住；这里嵌入了，所以这行断言
// 实际只保证类型存在。要真正拦住漏实现，得去掉嵌入——但那样协议每加一个
// RPC 都会立刻编译失败，对桩来说太脆。两者取后者，并在 README 里写明。
//
// [EN] A compile-time assertion that the stub still satisfies the contract. Because the
// Unimplemented struct is embedded, it only guarantees the type exists rather than catching a
// missing method; dropping the embed would catch it but make every new RPC in the contract
// break the build immediately, which is too brittle for a stub. The trade-off is documented in
// the README.
var _ pb.AuthServiceServer = (*AuthService)(nil)

var _ = goconfig.IsProductionLevel
