/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\inventory\warehouse_service.go
 * @Description: Warehouse 业务逻辑层，实现协议中心的 WarehouseServiceServer
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package inventory

import (
	"context"

	bizerror "github.com/kamalyes/go-bizerror"
	pbmo "github.com/kamalyes/go-pbmo"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	"github.com/kamalyes/kronos-my-service/errcodes"
	"github.com/kamalyes/kronos-my-service/models"
	"github.com/kamalyes/kronos-my-service/repository"
	"github.com/kamalyes/kronos-my-service/service/common"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/inventory"
)

// WarehouseService 实现协议中心 pb.WarehouseServiceServer
//
// 与 ProductService 的差别值得对照着看：
//   - 更新请求的可选字段全是 wrapper 类型（StringValue / Int32Value），
//     所以部分更新整条交给 pbmo 的 SetIfNotNil / SetInt32Val，一个 if 都不用写
//   - UpdateStatus 与 Delete 返回 common.GeneralEmptyResponse 而不是专属空响应，
//     协议中心对「无返回内容」的两种写法都有示例
//
// [EN] Worth comparing against the other service: here every optional update field is a
// wrapper type, so partial updates defer entirely to pbmo's SetIfNotNil / SetInt32Val with no
// hand-written conditionals; and two RPCs return common.GeneralEmptyResponse rather than
// dedicated empty messages, which the hub demonstrates both ways.
type WarehouseService struct {
	pb.UnimplementedWarehouseServiceServer
	repo *repository.WarehouseRepository
}

// NewWarehouseService 创建服务实例
func NewWarehouseService(repo *repository.WarehouseRepository) *WarehouseService {
	return &WarehouseService{repo: repo}
}

// WarehouseCreate 创建
func (s *WarehouseService) WarehouseCreate(
	ctx context.Context, req *pb.WarehouseCreateRequest,
) (*pb.WarehouseCreateResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	exists, err := s.repo.ExistsByField(ctx, tenantID, "code", req.GetCode(), "")
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询编码是否重复失败", "code", req.GetCode(), "error", err)
		return nil, common.DBError(ctx)
	}
	if exists {
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeWarehouseAlreadyExists,
			map[string]any{"reason": req.GetCode()})
	}

	model := &models.WarehouseModel{
		Id:        models.NewID(),
		TenantId:  tenantID,
		Name:      req.GetName(),
		Code:      req.GetCode(),
		IconUrl:   req.GetIconUrl(),
		Category:  req.GetCategory(),
		Status:    enumspb.CommonStatus_COMMON_STATUS_NORMAL,
		SortOrder: req.GetSortOrder(),
	}

	if _, err := s.repo.Create(ctx, model); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "创建失败", "code", req.GetCode(), "error", err)
		return nil, common.DBError(ctx)
	}

	info, err := model.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	common.RefreshKV(ctx, model)

	gwglobal.LOGGER.InfoContextKV(ctx, "创建成功", "id", model.Id, "code", model.Code)
	return &pb.WarehouseCreateResponse{Info: info}, nil
}

// WarehouseGet 查询详情
func (s *WarehouseService) WarehouseGet(
	ctx context.Context, req *pb.WarehouseGetRequest,
) (*pb.WarehouseGetResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	model, err := s.repo.GetByIDCached(ctx, tenantID, req.GetWarehouseId())
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询失败", "id", req.GetWarehouseId(), "error", err)
		return nil, common.DBError(ctx)
	}
	if model == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeWarehouseNotFound)
	}

	info, err := model.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.WarehouseGetResponse{Info: info}, nil
}

// WarehouseList 分页列表
func (s *WarehouseService) WarehouseList(
	ctx context.Context, req *pb.WarehouseListRequest,
) (*pb.WarehouseListResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	items, paging, err := s.repo.ListWithPagination(ctx, tenantID, req)
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "分页查询失败", "error", err)
		return nil, common.DBError(ctx)
	}

	infos, err := models.ToPBsWarehouse(ctx, items)
	if err != nil {
		return nil, err
	}

	return &pb.WarehouseListResponse{
		PageResponse: models.BuildPagination(paging.Page, paging.PageSize, int32(paging.Total)),
		Infos:        infos,
	}, nil
}

// WarehouseUpdate 更新
//
// 整条更新由 pbmo 的 SetIfNotNil / SetInt32Val 组装：请求里的可选字段都是 wrapper，
// 「没传」与「传了零值」在类型层面就分得开，不需要任何额外判断。
// SetInt32Val 专收 *wrapperspb.Int32Value，正是 sort_order 的类型。
//
// [EN] Assembled entirely by pbmo: every optional field is a wrapper, so "not sent" and "sent
// zero" are distinguishable at the type level with no extra checks. SetInt32Val takes a
// *wrapperspb.Int32Value, exactly sort_order's type.
func (s *WarehouseService) WarehouseUpdate(
	ctx context.Context, req *pb.WarehouseUpdateRequest,
) (*pb.WarehouseUpdateResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	id := req.GetWarehouseId()
	updates := pbmo.NewUpdates().
		SetIfNotNil("name", req.GetName()).
		SetIfNotNil("icon_url", req.GetIconUrl()).
		SetInt32Val("sort_order", req.GetSortOrder()).
		SetIfNotZero("category", req.GetCategory()).
		Build()

	if err := s.repo.UpdateScoped(ctx, tenantID, id, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "更新失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}
	if len(updates) > 0 {
		gwglobal.LOGGER.InfoContextKV(ctx, "更新成功", "id", id, "fields", len(updates))
	}

	fresh, err := s.repo.GetByIDScoped(ctx, tenantID, id)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if fresh == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeWarehouseNotFound)
	}
	common.RefreshKV(ctx, fresh)

	info, err := fresh.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.WarehouseUpdateResponse{Info: info}, nil
}

// WarehouseUpdateStatus 更新状态
func (s *WarehouseService) WarehouseUpdateStatus(
	ctx context.Context, req *pb.WarehouseUpdateStatusRequest,
) (*commonpb.GeneralEmptyResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetStatus() == enumspb.CommonStatus_COMMON_STATUS_UNSPECIFIED {
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeWarehouseInvalidStatus,
			map[string]any{"reason": req.GetStatus().String()})
	}

	id := req.GetWarehouseId()
	updates := pbmo.NewUpdates().Set("status", req.GetStatus()).Build()
	if err := s.repo.UpdateScoped(ctx, tenantID, id, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "更新状态失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "更新状态成功", "id", id, "status", req.GetStatus().String())
	return &commonpb.GeneralEmptyResponse{}, nil
}

// WarehouseDelete 删除
func (s *WarehouseService) WarehouseDelete(
	ctx context.Context, req *pb.WarehouseDeleteRequest,
) (*commonpb.GeneralEmptyResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	id := req.GetWarehouseId()
	if err := s.repo.DeleteScoped(ctx, tenantID, id); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "删除失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}
	// 整实例缓存由 DeleteScoped 内部失效，字段级缓存要在这里单独清
	// [EN] DeleteScoped invalidates the whole-instance cache; the field cache is cleared here
	common.InvalidateKV[models.WarehouseModel](ctx, id)

	gwglobal.LOGGER.InfoContextKV(ctx, "删除成功", "id", id)
	return &commonpb.GeneralEmptyResponse{}, nil
}
