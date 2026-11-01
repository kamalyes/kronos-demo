/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\tenant\tenant_member_service.go
 * @Description: TenantMember 业务逻辑，实现协议中心的 TenantMemberServiceServer
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package tenant

import (
	"context"

	bizerror "github.com/kamalyes/go-bizerror"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	gwmiddleware "github.com/kamalyes/go-rpc-gateway/middleware"
	"github.com/kamalyes/kronos-my-service/errcodes"
	"github.com/kamalyes/kronos-my-service/models"
	"github.com/kamalyes/kronos-my-service/repository"
	"github.com/kamalyes/kronos-my-service/service/common"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/tenant"
)

// TenantMemberService 实现协议中心 pb.TenantMemberServiceServer
//
// 与 TenantService 同处 tenant 模块、共用 proto 包与 Go 包，
// 但两者不互相 import：共享的类型在 common/ 里。协议中心的规范第 8.5 节写明
// 「拆开后两个 service 不要互相 import，否则又被焊回一起」。
//
// 这里的父 ID 来自请求字段（tenant_id）而不是 gwmiddleware：
// 成员接口操作的是「某个聚合根下的成员」，父 ID 是路径参数，属于寻址信息；
// 真实系统里还应校验调用者对该聚合根有权限，那是鉴权层的事，不在这里。
//
// [EN] It shares the module and Go package with the aggregate service but never imports it;
// shared types live in common/. The parent id comes from the request field rather than the
// context because it is addressing information, a path parameter. A real system would also
// verify the caller's permission on that root, which belongs to the authorization layer.
type TenantMemberService struct {
	pb.UnimplementedTenantMemberServiceServer
	repo *repository.TenantMemberRepository
}

// NewTenantMemberService 创建服务实例
func NewTenantMemberService(repo *repository.TenantMemberRepository) *TenantMemberService {
	return &TenantMemberService{repo: repo}
}

// validateMemberRoles 校验角色枚举的允许值
//
// 为什么不交给 proto 的 struct tag：协议中心在 roles 上注入的是
// validate:"required,max=8"，它只保证「传了、不超过 8 个」，保证不了每个值都是
// 合法角色——UNSPECIFIED 是枚举的零值，照样过校验
// 而 go-argus 的 oneof 系列规则在这里也用不上：它把 protobuf 枚举格式化成名称再比较，
// 写数字候选值会误拒合法请求（协议中心的 proto 规范第 7 节写了这条）
//
// 三个接口（Add / RoleReplace / BatchInvite）都收 roles，所以抽成一处，
// 不然加一条规则要改三个地方，漏一个就只有那条路径能塞进非法角色
//
// [EN] The hub's validate tag guarantees only that roles were sent and that there are at most
// eight; it cannot guarantee each value is a real role, since UNSPECIFIED is the enum's zero
// value and passes. go-argus's oneof-style rules do not help either, because it formats a
// protobuf enum to its name before comparing, so numeric candidates reject valid requests. All
// three endpoints take roles, so the check lives in one place: otherwise a new rule means three
// edits and the one that is missed becomes the path that accepts an illegal role
func validateMemberRoles(ctx context.Context, roles []enumspb.TenantMemberRole) error {
	unspecified := enumspb.TenantMemberRole_TENANT_MEMBER_ROLE_UNSPECIFIED
	for _, role := range roles {
		if role == unspecified {
			return bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeTenantMemberInvalidRole,
				map[string]any{"reason": role.String()})
		}
	}
	return nil
}

// TenantMemberList 分页列出成员
//
// 子资源列表同样要分页：它会随父资源增长，不分页的「列出全部成员」
// 在大租户上就是一次全表扫描
// [EN] A sub-resource list is paged too: it grows with the parent, and listing all members
// unpaged becomes a full scan on a large tenant
func (s *TenantMemberService) TenantMemberList(
	ctx context.Context, req *pb.TenantMemberListRequest,
) (*pb.TenantMemberListResponse, error) {
	tenantID := req.GetTenantId()

	items, paging, err := s.repo.ListWithPagination(ctx, tenantID, req)
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "分页查询成员失败", "id", tenantID, "error", err)
		return nil, common.DBError(ctx)
	}

	infos, err := models.ToPBsTenantMember(ctx, items)
	if err != nil {
		return nil, err
	}

	return &pb.TenantMemberListResponse{
		PageResponse: models.BuildPagination(paging.Page, paging.PageSize, int32(paging.Total)),
		Infos:        infos,
	}, nil
}

// TenantMemberAdd 添加成员
func (s *TenantMemberService) TenantMemberAdd(
	ctx context.Context, req *pb.TenantMemberAddRequest,
) (*pb.TenantMemberAddResponse, error) {
	tenantID := req.GetTenantId()

	// 先校验所属 Tenant 存在，再查成员是否重复：
	// 顺序反了的话，往一个不存在的 Tenant 里加成员会「成功」，
	// 留下一条永远查不到所属主体的孤儿记录。
	//
	// 只在缓存可信且确实查不到时才否决——见 common.TenantNameLookup 的说明，
	// 把「缓存没开」当成「不存在」会让无缓存环境下所有添加成员都失败。
	//
	// [EN] Validate the owning aggregate before checking for a duplicate member: the other
	// order lets a member be added to a nonexistent owner and leaves an orphan row. The
	// rejection only happens when the cache is trustworthy, since treating an unavailable cache
	// as a missing row would fail every add in an environment without one.
	if name, cached := common.TenantNameLookup(ctx, tenantID); cached && name == "" {
		gwglobal.LOGGER.WarnContextKV(ctx, "所属 Tenant 不存在", "id", tenantID)
		// 报父资源的 not_found 而不是成员的：缺的是 Tenant，
		// 报成员不存在会让调用方以为是 user_id 的问题，去查一个还没创建的东西
		// [EN] The root's not-found rather than the member's: what is missing is the root, and
		// blaming the member points the caller at a user id that was never the problem
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantNotFound)
	}

	if err := validateMemberRoles(ctx, req.GetRoles()); err != nil {
		return nil, err
	}

	exists, err := s.repo.UserExists(ctx, tenantID, req.GetUserId())
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询成员是否已存在失败",
			"id", tenantID, "user_id", req.GetUserId(), "error", err)
		return nil, common.DBError(ctx)
	}
	if exists {
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeTenantMemberAlreadyExists,
			map[string]any{"reason": req.GetUserId()})
	}

	member := &models.TenantMemberModel{
		Id:       models.NewID(),
		TenantId: tenantID,
		UserId:   req.GetUserId(),
		Email:    req.GetEmail(),
		Status:   enumspb.CommonStatus_COMMON_STATUS_NORMAL,
	}
	member.SetRoles(req.GetRoles())

	if _, err := s.repo.Create(ctx, member); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "添加成员失败", "id", tenantID, "error", err)
		return nil, common.DBError(ctx)
	}

	info, err := member.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "添加成员成功",
		"id", tenantID, "member_id", member.Id, "roles", len(req.GetRoles()))
	return &pb.TenantMemberAddResponse{Info: info}, nil
}

// TenantMemberRemove 移除成员
func (s *TenantMemberService) TenantMemberRemove(
	ctx context.Context, req *pb.TenantMemberRemoveRequest,
) (*commonpb.GeneralEmptyResponse, error) {
	tenantID := req.GetTenantId()
	memberID := req.GetMemberId()

	if err := s.repo.DeleteScoped(ctx, tenantID, memberID); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "移除成员失败",
			"id", tenantID, "member_id", memberID, "error", err)
		return nil, common.DBError(ctx)
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "移除成员成功",
		"id", tenantID, "member_id", memberID)
	return &commonpb.GeneralEmptyResponse{}, nil
}

// TenantMemberRoleReplace 覆盖成员角色
//
// 语义是覆盖而不是追加，协议中心的字段注释写的是「覆盖后的角色集合」。
// 覆盖型接口不能用「空集合视为不变」那套：调用方传空集合就是要清空角色，
// 而「不想改角色」就根本不该调这个接口
//
// [EN] The semantics are replace, not append, per the hub's field comment. A replace endpoint
// must not treat an empty set as "unchanged": an empty set means clear the roles, and a caller
// who does not want to change them simply does not call this endpoint
func (s *TenantMemberService) TenantMemberRoleReplace(
	ctx context.Context, req *pb.TenantMemberRoleReplaceRequest,
) (*pb.TenantMemberRoleReplaceResponse, error) {
	tenantID := req.GetTenantId()
	memberID := req.GetMemberId()

	if err := validateMemberRoles(ctx, req.GetRoles()); err != nil {
		return nil, err
	}

	member, err := s.repo.GetByIDScoped(ctx, tenantID, memberID)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if member == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantMemberNotFound)
	}

	member.SetRoles(req.GetRoles())
	if err := s.repo.UpdateScoped(ctx, tenantID, memberID,
		map[string]interface{}{"roles": member.Roles}); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "覆盖成员角色失败",
			"member_id", memberID, "error", err)
		return nil, common.DBError(ctx)
	}

	info, err := member.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "覆盖成员角色成功",
		"member_id", memberID, "roles", len(req.GetRoles()))
	return &pb.TenantMemberRoleReplaceResponse{Info: info}, nil
}

// TenantMemberBatchInvite 批量邀请
//
// 批量接口的三条约定（协议中心规范第 8.4 节）：
//
//	走 POST 而不是带 body 的 DELETE/PUT——网关与代理对带 body 的 DELETE 行为不一致
//	必须有条数上限，由 proto 的 validate:"required,max=50" 保证，这里不重复校验
//	逐条返回结果（common.BatchResult），让调用方精确重试，而不是整体成功/失败
//
// 已存在的邮箱一次查完再比对，不是逐个查：50 条逐个查就是 50 次往返。
// 真正要插入的那批放进一个事务——部分成功后调用方重试会把已建的再建一遍，
// 而成员表上「同租户同邮箱」没有唯一约束（邮箱可为空），重试就产生重复行。
//
// [EN] The three conventions for batch endpoints: POST rather than a body-bearing DELETE, since
// gateways and proxies treat those inconsistently; a mandatory count cap, enforced by the
// proto's validate tag and not re-checked here; and per-item results so the caller retries
// precisely. Existing emails are fetched in one query rather than one per item, and the rows to
// insert share a transaction because a partial success plus a retry would duplicate rows: there
// is no unique constraint on tenant plus email, as email may be empty.
func (s *TenantMemberService) TenantMemberBatchInvite(
	ctx context.Context, req *pb.TenantMemberBatchInviteRequest,
) (*pb.TenantMemberBatchInviteResponse, error) {
	tenantID := req.GetTenantId()
	emails := req.GetEmails()

	// 角色校验放在查库之前：非法入参不该先让数据库跑一趟
	// [EN] Validated before the query: a malformed request should not cost a database round trip
	if err := validateMemberRoles(ctx, req.GetRoles()); err != nil {
		return nil, err
	}

	existing, err := s.repo.EmailsExist(ctx, tenantID, emails)
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询已存在邮箱失败", "id", tenantID, "error", err)
		return nil, common.DBError(ctx)
	}

	results := make([]*commonpb.BatchResult, 0, len(emails))
	toCreate := make([]*models.TenantMemberModel, 0, len(emails))
	seen := make(map[string]bool, len(emails))

	for _, email := range emails {
		// 入参自身可能重复，去重后再判断，否则同一邮箱会被插两次
		// [EN] The input itself may repeat; dedupe first or the same email is inserted twice
		if seen[email] {
			results = append(results, &commonpb.BatchResult{
				Id:           email,
				Success:      false,
				ErrorMessage: memberExistsMsg(ctx, email),
			})
			continue
		}
		seen[email] = true

		if existing[email] {
			results = append(results, &commonpb.BatchResult{
				Id:           email,
				Success:      false,
				ErrorMessage: memberExistsMsg(ctx, email),
			})
			continue
		}

		member := &models.TenantMemberModel{
			Id:       models.NewID(),
			TenantId: tenantID,
			Email:    email,
			// CommonStatus 只有 UNSPECIFIED / NORMAL / DISABLED，没有 PENDING：
			// 被邀请但未接受的成员先置 DISABLED，接受后再转 NORMAL
			// [EN] CommonStatus has no PENDING, so an invited-but-unaccepted member starts
			// DISABLED and moves to NORMAL on acceptance
			Status: enumspb.CommonStatus_COMMON_STATUS_DISABLED,
		}
		member.SetRoles(req.GetRoles())
		toCreate = append(toCreate, member)
		results = append(results, &commonpb.BatchResult{Id: email, Success: true})
	}

	if err := s.repo.CreateBatchInTx(ctx, toCreate); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "批量邀请失败",
			"id", tenantID, "count", len(toCreate), "error", err)
		return nil, common.DBError(ctx)
	}

	gwglobal.LOGGER.InfoContextKV(ctx, "批量邀请完成",
		"id", tenantID, "total", len(emails), "created", len(toCreate))
	return &pb.TenantMemberBatchInviteResponse{Results: results}, nil
}

// memberExistsMsg 批量接口逐条结果里的「成员已存在」文案
//
// 必须用 GetMsgWithMap 而不是 GetMsgByKey：这个码的文案带 {{reason}} 占位符，
// GetMsgByKey 不套模板，调用方会收到字面量 {{reason}}
// 批量接口返回的是文案而不是错误码（common.BatchResult 只有 error_message 一个字段），
// 所以这里不经过 ToGRPCError，自己取一次文案
//
// [EN] GetMsgWithMap rather than GetMsgByKey: this code's message carries a {{reason}}
// placeholder and GetMsgByKey does not apply templates, so the caller would receive the literal
// placeholder. A batch endpoint returns a message rather than a code, since BatchResult has
// only error_message, which is why the text is resolved here instead of going through
// ToGRPCError
func memberExistsMsg(ctx context.Context, email string) string {
	return gwmiddleware.GetMsgWithMap(ctx, errcodes.BizErrCodeTenantMemberAlreadyExists,
		map[string]any{"reason": email})
}
