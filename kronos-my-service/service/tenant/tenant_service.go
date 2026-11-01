/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\tenant\tenant_service.go
 * @Description: Tenant 聚合根业务逻辑，实现协议中心的 TenantServiceServer
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package tenant

import (
	"context"

	bizerror "github.com/kamalyes/go-bizerror"
	pbmo "github.com/kamalyes/go-pbmo"
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

// TenantService 实现协议中心 pb.TenantServiceServer
//
// 这个 service 覆盖了协议中心规范里讲的三类接口形态，前两个示例只有第一类：
//
//	标准 CRUD        Create / Get / List / Update / Delete
//	动作型           RetrySync —— 触发一次行为，不是改字段，结果用枚举而非 bool
//	子资源           RegionAssign / RegionAssignmentUpdate / RegionAssignmentRemove
//	                有独立 ID、单独增改删，父 ID 只放路径不进 body
//
// 它与 TenantMemberService 共用 proto 包与 Go 包（同一个 tenant 模块），
// 但两者不互相 import——共享的类型在 common/ 里，否则两个 service 又被焊回一起。
//
// [EN] This service covers the three endpoint shapes the hub's conventions describe, where the
// first two examples only cover CRUD: an action endpoint (RetrySync triggers a behaviour rather
// than editing fields, and its outcome is an enum, not a bool) and a sub-resource with its own
// id and its own create/update/delete. It shares the proto and Go package with the member
// service but the two never import each other; shared types live in common/.
type TenantService struct {
	pb.UnimplementedTenantServiceServer
	repo *repository.TenantRepository
}

// NewTenantService 创建服务实例
func NewTenantService(repo *repository.TenantRepository) *TenantService {
	return &TenantService{repo: repo}
}

// TenantCreate 创建聚合根
//
// 聚合根与初始子资源同一个事务：创建成功但地区没分配上，会留下一个
// 不符合业务约束的聚合根，而调用方拿到的是成功
// [EN] Root and initial sub-resources share a transaction: a created root whose regions failed
// to attach violates the business invariant while the caller was told it succeeded
func (s *TenantService) TenantCreate(
	ctx context.Context, req *pb.TenantCreateRequest,
) (*pb.TenantCreateResponse, error) {
	exists, err := s.repo.CodeExists(ctx, req.GetCode(), "")
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询编码是否重复失败", "code", req.GetCode(), "error", err)
		return nil, common.DBError(ctx)
	}
	if exists {
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeTenantAlreadyExists,
			map[string]any{"reason": req.GetCode()})
	}

	model := &models.TenantModel{
		Id:         models.NewID(),
		Code:       req.GetCode(),
		Name:       req.GetName(),
		PlatformId: req.GetPlatformId(),
		Status:     enumspb.TenantStatus_TENANT_STATUS_PENDING,
		SyncState:  enumspb.TenantSyncState_TENANT_SYNC_STATE_PENDING,
	}
	if ids := req.GetRegionIds(); len(ids) > 0 {
		model.RegionId = ids[0]
	}

	if _, err := s.repo.Create(ctx, model); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "创建失败", "code", req.GetCode(), "error", err)
		return nil, common.DBError(ctx)
	}

	// 初始地区分配：第一个为主地区，其余为非主
	for i, regionID := range req.GetRegionIds() {
		assignment := &models.TenantRegionAssignmentModel{
			Id:        models.NewID(),
			TenantId:  model.Id,
			RegionId:  regionID,
			IsPrimary: i == 0,
		}
		if _, err := s.repo.Regions.Create(ctx, assignment); err != nil {
			gwglobal.LOGGER.ErrorContextKV(ctx, "初始地区分配失败",
				"id", model.Id, "region_id", regionID, "error", err)
			return nil, common.DBError(ctx)
		}
	}

	info, err := model.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	// 新建的记录立刻进字段级缓存，不等下一次全量刷新
	// [EN] Put the new row into the field cache now rather than waiting for the next refresh
	common.RefreshKV(ctx, model)

	gwglobal.LOGGER.InfoContextKV(ctx, "创建成功",
		"id", model.Id, "code", model.Code, "regions", len(req.GetRegionIds()))
	return &pb.TenantCreateResponse{Info: info}, nil
}

// TenantGet 查询详情
//
// 详情接口才查子资源，列表接口不查：列表里逐行查地区就是 N+1
// [EN] Only the detail endpoint fetches sub-resources; doing it per row in a list is N+1
func (s *TenantService) TenantGet(
	ctx context.Context, req *pb.TenantGetRequest,
) (*pb.TenantGetResponse, error) {
	model, err := s.repo.GetByID(ctx, req.GetTenantId())
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询失败", "id", req.GetTenantId(), "error", err)
		return nil, common.DBError(ctx)
	}
	if model == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantNotFound)
	}

	info, err := model.ToPB(ctx)
	if err != nil {
		return nil, err
	}

	assignments, err := s.repo.ListRegions(ctx, model.Id)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	regions, err := models.ToPBsTenantRegionAssignment(ctx, assignments)
	if err != nil {
		return nil, err
	}
	info.Regions = regions

	return &pb.TenantGetResponse{Info: info}, nil
}

// TenantList 分页列表
func (s *TenantService) TenantList(
	ctx context.Context, req *pb.TenantListRequest,
) (*pb.TenantListResponse, error) {
	items, paging, err := s.repo.ListWithPagination(ctx, req)
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "分页查询失败", "error", err)
		return nil, common.DBError(ctx)
	}

	infos, err := models.ToPBsTenant(ctx, items)
	if err != nil {
		return nil, err
	}

	return &pb.TenantListResponse{
		PageResponse: models.BuildPagination(paging.Page, paging.PageSize, int32(paging.Total)),
		Infos:        infos,
	}, nil
}

// TenantUpdate 更新
func (s *TenantService) TenantUpdate(
	ctx context.Context, req *pb.TenantUpdateRequest,
) (*pb.TenantUpdateResponse, error) {
	id := req.GetTenantId()
	updates := pbmo.NewUpdates().
		SetIfNotNil("name", req.GetName()).
		SetIfNotZero("status", req.GetStatus()).
		Build()

	if err := s.repo.UpdateByID(ctx, id, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "更新失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}

	fresh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if fresh == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantNotFound)
	}
	info, err := fresh.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	// 用刚读回来的那条刷缓存，而不是用 updates 拼一个半成品
	// [EN] Refresh from the row just read back, not from a partial built out of updates
	common.RefreshKV(ctx, fresh)

	gwglobal.LOGGER.InfoContextKV(ctx, "更新成功", "id", id, "fields", len(updates))
	return &pb.TenantUpdateResponse{Info: info}, nil
}

// TenantDelete 删除
func (s *TenantService) TenantDelete(
	ctx context.Context, req *pb.TenantDeleteRequest,
) (*commonpb.GeneralEmptyResponse, error) {
	id := req.GetTenantId()
	if err := s.repo.DeleteWithRegions(ctx, id); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "删除失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}
	common.InvalidateKV[models.TenantModel](ctx, id)

	gwglobal.LOGGER.InfoContextKV(ctx, "删除成功", "id", id)
	return &commonpb.GeneralEmptyResponse{}, nil
}

// TenantRetrySync 重试下游同步（动作型接口）
//
// 动作型接口的三条约定（协议中心规范第 8.2 节）：
//
//	走 POST + 动词短横线路径，不塞进 PUT/PATCH——它不改字段，重复调用也不是幂等覆盖
//	有自己的 Request/Response，不复用 GetRequest，否则以后加参数没地方放
//	结果用枚举而不是 bool：动作的结局常常超过两种（待同步/已同步/失败）
//
// 这里只演示状态流转与返回形态，真实实现应该把同步任务投递到队列，
// 而不是在 RPC 里同步等外部系统——那会让这个调用的耗时取决于下游
//
// [EN] The three conventions for action endpoints: POST with a verb path rather than PUT/PATCH
// since it edits no fields and repeating it is not an idempotent overwrite; its own request and
// response so future parameters have somewhere to live; and an enum outcome because actions
// usually have more than two results. A real implementation would enqueue the sync rather than
// waiting on the downstream inside the RPC, which would tie this call's latency to it.
func (s *TenantService) TenantRetrySync(
	ctx context.Context, req *pb.TenantRetrySyncRequest,
) (*pb.TenantRetrySyncResponse, error) {
	id := req.GetTenantId()
	model, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if model == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantNotFound)
	}

	// 冻结的 Tenant 不触发下游同步：协议中心的状态机是 待激活 → 正常 → 冻结，
	// 冻结表示这个 Tenant 已停止对外活动，把它推给下游只会在对端留下脏数据
	//
	// 这里用的是「资源当前状态不满足前置条件」而不是「入参不合法」，所以错误码映射到
	// FailedPrecondition 而不是 InvalidArgument——请求本身没有任何问题
	// 它也正是那种不显式 Register 就会变成 500 的码：码值不以 _not_found 结尾，
	// go-bizerror 的后缀规则接不上，兜底是 Internal
	//
	// [EN] A frozen root does not trigger a downstream sync: the hub's state machine runs
	// pending to normal to frozen, and frozen means this root has stopped operating, so pushing
	// it downstream only leaves dirty data at the far end. The condition is the resource's
	// current state rather than a malformed request, so the code maps to FailedPrecondition
	// rather than InvalidArgument. It is also exactly the kind of code that becomes a 500
	// without an explicit Register: the value does not end in _not_found, so go-bizerror's
	// suffix rule does not apply and the fallback is Internal
	frozen := enumspb.TenantStatus_TENANT_STATUS_FROZEN
	if model.Status == frozen {
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeTenantInvalidStatus,
			map[string]any{"reason": model.Status.String()})
	}

	// 已同步成功且未要求强制时直接返回当前状态，不重复触发
	// [EN] Already synced and force not requested: return the current state without retriggering
	synced := enumspb.TenantSyncState_TENANT_SYNC_STATE_SYNCED
	if model.SyncState == synced && !req.GetForce().GetValue() {
		return &pb.TenantRetrySyncResponse{
			SyncState: synced,
			Message:   gwmiddleware.GetMsgByKey(ctx, "tenant.sync.already_synced"),
		}, nil
	}

	// 枚举里没有 SYNCING：协议中心只定义了 UNSPECIFIED / PENDING / SYNCED / FAILED，
	// 「已触发、等下游回执」对应 PENDING。要表达「同步中」须先在协议中心加枚举值，
	// 服务侧不能凭空造一个数值——那会在下游按名称解析时变成未知值
	// [EN] There is no SYNCING: the hub defines UNSPECIFIED / PENDING / SYNCED / FAILED, and
	// "triggered, awaiting the downstream" maps to PENDING. Adding a state requires changing
	// the hub; inventing a numeric value here would decode as unknown downstream
	next := enumspb.TenantSyncState_TENANT_SYNC_STATE_PENDING
	updates := pbmo.NewUpdates().Set("sync_state", next).Build()
	if err := s.repo.UpdateByID(ctx, id, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "更新同步状态失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}

	gwglobal.LOGGER.InfoContextKV(ctx, "已触发下游同步",
		"id", id, "force", req.GetForce().GetValue())
	return &pb.TenantRetrySyncResponse{
		SyncState: next,
		Message:   gwmiddleware.GetMsgByKey(ctx, "tenant.sync.triggered"),
	}, nil
}

// TenantRegionAssign 分配地区（子资源创建）
func (s *TenantService) TenantRegionAssign(
	ctx context.Context, req *pb.TenantRegionAssignRequest,
) (*pb.TenantRegionAssignResponse, error) {
	tenantID := req.GetTenantId()

	// 父资源必须存在：子资源挂在不存在的父上是脏数据，而外键不是所有部署都开
	// [EN] The parent must exist: a sub-resource on a missing parent is dirty data, and foreign
	// keys are not enabled in every deployment
	parent, err := s.repo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if parent == nil {
		// 缺的是父资源，所以报 Tenant 的 not_found，而不是地区分配的
		// [EN] The parent is what is missing, so this is the root's not-found, not the
		// assignment's
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantNotFound)
	}

	if req.GetPrimary() {
		if err := s.repo.ClearPrimaryRegion(ctx, tenantID); err != nil {
			gwglobal.LOGGER.ErrorContextKV(ctx, "清除原主地区失败", "id", tenantID, "error", err)
			return nil, common.DBError(ctx)
		}
	}

	assignment := &models.TenantRegionAssignmentModel{
		Id:        models.NewID(),
		TenantId:  tenantID,
		RegionId:  req.GetRegionId(),
		IsPrimary: req.GetPrimary(),
	}
	if _, err := s.repo.Regions.Create(ctx, assignment); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "分配地区失败",
			"id", tenantID, "region_id", req.GetRegionId(), "error", err)
		return nil, common.DBError(ctx)
	}

	pbAssignment, err := assignment.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "分配地区成功",
		"id", tenantID, "assignment_id", assignment.Id, "primary", req.GetPrimary())
	return &pb.TenantRegionAssignResponse{Assignment: pbAssignment}, nil
}

// TenantRegionAssignmentUpdate 更新地区分配（子资源更新）
func (s *TenantService) TenantRegionAssignmentUpdate(
	ctx context.Context, req *pb.TenantRegionAssignmentUpdateRequest,
) (*pb.TenantRegionAssignmentUpdateResponse, error) {
	tenantID := req.GetTenantId()
	assignmentID := req.GetAssignmentId()

	// 子资源的归属要显式校验：父 ID 在路径里，但不校验就等于让调用方改别人的子资源
	// [EN] Ownership is verified explicitly: the parent id is in the path, but without checking
	// it the caller could edit someone else's sub-resource
	current, err := s.repo.Regions.GetByIDScoped(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if current == nil {
		// 子资源专属 not_found：父存在而子不存在时报父的 not_found，
		// 调用方会以为自己拿错了父 ID，去查一个其实没问题的东西
		// [EN] The sub-resource's own not-found: reporting the parent's would make the caller
		// believe it holds the wrong parent id and debug something that is in fact correct
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantRegionAssignmentNotFound)
	}

	if req.GetPrimary() != nil && req.GetPrimary().GetValue() {
		if err := s.repo.ClearPrimaryRegion(ctx, tenantID); err != nil {
			return nil, common.DBError(ctx)
		}
	}

	updates := pbmo.NewUpdates().SetIfNotNil("is_primary", req.GetPrimary()).Build()
	if err := s.repo.Regions.UpdateScoped(ctx, tenantID, assignmentID, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "更新地区分配失败",
			"assignment_id", assignmentID, "error", err)
		return nil, common.DBError(ctx)
	}

	fresh, err := s.repo.Regions.GetByIDScoped(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if fresh == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeTenantRegionAssignmentNotFound)
	}
	pbAssignment, err := fresh.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.TenantRegionAssignmentUpdateResponse{Assignment: pbAssignment}, nil
}

// TenantRegionAssignmentRemove 移除地区分配（子资源删除）
func (s *TenantService) TenantRegionAssignmentRemove(
	ctx context.Context, req *pb.TenantRegionAssignmentRemoveRequest,
) (*commonpb.GeneralEmptyResponse, error) {
	tenantID := req.GetTenantId()
	assignmentID := req.GetAssignmentId()

	if err := s.repo.Regions.DeleteScoped(ctx, tenantID, assignmentID); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "移除地区分配失败",
			"assignment_id", assignmentID, "error", err)
		return nil, common.DBError(ctx)
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "移除地区分配成功",
		"id", tenantID, "assignment_id", assignmentID)
	return &commonpb.GeneralEmptyResponse{}, nil
}
