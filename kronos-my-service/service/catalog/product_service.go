/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\service\catalog\product_service.go
 * @Description: Product 业务逻辑层，实现协议中心的 ProductServiceServer
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package catalog

import (
	"context"

	bizerror "github.com/kamalyes/go-bizerror"
	pbmo "github.com/kamalyes/go-pbmo"
	gwglobal "github.com/kamalyes/go-rpc-gateway/global"
	gwmiddleware "github.com/kamalyes/go-rpc-gateway/middleware"
	sqltypes "github.com/kamalyes/go-sqlbuilder/types"
	"github.com/kamalyes/kronos-my-service/errcodes"
	"github.com/kamalyes/kronos-my-service/models"
	"github.com/kamalyes/kronos-my-service/repository"
	"github.com/kamalyes/kronos-my-service/service/common"
	pb "github.com/kamalyes/kronos-share-proto-template/pb/catalog"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
	enumspb "github.com/kamalyes/kronos-share-proto-template/pb/enums"
)

// ProductService 实现协议中心 pb.ProductServiceServer
//
// 这是一份可直接改的真实示例，不是占位：它实现的就是协议中心
// proto/catalog/product_service.proto 里定义的那个 service，
// 换成自己的业务时照着替换 model 与 RPC 体即可。
//
// 嵌入 Unimplemented 是 protoc-gen-go-grpc 的要求，也给了向前兼容：
// 协议中心新增 RPC 后本服务仍能编译，新方法返回 Unimplemented 而不是编译失败。
//
// [EN] A real, editable example rather than a placeholder: it implements the very service the
// contract hub defines. Embedding Unimplemented is required by protoc-gen-go-grpc and gives
// forward compatibility: a new RPC in the hub still compiles here and returns Unimplemented.
type ProductService struct {
	pb.UnimplementedProductServiceServer
	repo *repository.ProductRepository
}

// NewProductService 创建服务实例
func NewProductService(repo *repository.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

// ProductCreate 创建
//
// 参数校验不在这里写：协议中心的 proto 字段上注入了 @inject_tag: validate:"..."，
// go-rpc-gateway 已把基于 go-argus 的 struct tag 校验拦截器挂在链上，
// 走到这里时 req 的格式约束已经满足。这里只做格式之外的业务规则（唯一性之类）。
//
// [EN] Field validation is not written here: the hub injects validate tags into the proto
// fields and go-rpc-gateway runs the go-argus struct-tag interceptor, so format constraints
// already hold. Only business rules beyond format belong here.
func (s *ProductService) ProductCreate(
	ctx context.Context, req *pb.ProductCreateRequest,
) (*pb.ProductCreateResponse, error) {
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
		// 用实体专属码而不是通用的 BizErrCodeConflict：通用码只说「冲突了」，
		// 调用方还得猜是哪个实体的哪个字段撞了，而它要据此决定是换个编码重试
		// 还是提示用户改输入带 {reason} 模板把重复的那个值一并带回，
		// 文案由 go-bizerror 按请求语言渲染，这里不拼人类可读的句子
		// [EN] The entity's own code rather than the generic conflict: the generic one says only
		// that something collided and leaves the caller guessing which entity and field, when
		// that is exactly what it needs in order to choose between retrying with another code
		// and asking the user to change the input. The templated code carries the duplicate
		// value back, and go-bizerror renders the message in the request language
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeProductAlreadyExists,
			map[string]any{"reason": req.GetCode()})
	}

	model := &models.ProductModel{
		Id:       models.NewID(),
		TenantId: tenantID,
		Name:     req.GetName(),
		Code:     req.GetCode(),
		Type:     req.GetType(),
		Level:    req.GetLevel(),
		Status:   enumspb.CommonStatus_COMMON_STATUS_NORMAL,
		Tags:     req.GetTags(),
	}
	model.SetContact(req.GetContact())

	if _, err := s.repo.Create(ctx, model); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "创建失败", "code", req.GetCode(), "error", err)
		return nil, common.DBError(ctx)
	}

	info, err := model.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "创建成功", "id", model.Id, "code", model.Code)
	return &pb.ProductCreateResponse{Info: info}, nil
}

// ProductGet 查询详情
func (s *ProductService) ProductGet(
	ctx context.Context, req *pb.ProductGetRequest,
) (*pb.ProductGetResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	model, err := s.repo.GetByIDCached(ctx, tenantID, req.GetProductId())
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "查询失败", "id", req.GetProductId(), "error", err)
		return nil, common.DBError(ctx)
	}
	if model == nil {
		// 实体专属 not_found：码值以 _not_found 结尾，go-bizerror 的后缀规则
		// 本就能映射成 NotFound，errcodes.Mappings 里仍然逐条登记，理由见那里
		// [EN] The entity's own not-found. Its value ends in _not_found, which go-bizerror's
		// suffix rule already maps to NotFound; Mappings lists it anyway and explains why
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeProductNotFound)
	}

	info, err := model.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.ProductGetResponse{Info: info}, nil
}

// ProductList 分页列表
func (s *ProductService) ProductList(
	ctx context.Context, req *pb.ProductListRequest,
) (*pb.ProductListResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	items, paging, err := s.repo.ListWithPagination(ctx, tenantID, req)
	if err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "分页查询失败", "error", err)
		return nil, common.DBError(ctx)
	}

	infos, err := models.ToPBsProduct(ctx, items)
	if err != nil {
		return nil, err
	}

	return &pb.ProductListResponse{
		// Total 在 PaginationT 里固定是 int64（为支持大数据量），而 common.Paging.total
		// 是 int32，这里显式收窄；真到 21 亿条时分页本身已不是合适的读取方式
		// [EN] Total is int64 in PaginationT while common.Paging.total is int32
		PageResponse: models.BuildPagination(paging.Page, paging.PageSize, int32(paging.Total)),
		Infos:        infos,
	}, nil
}

// ProductUpdate 更新
//
// 用 pbmo.NewUpdates() 拼部分更新：请求里的可选字段是 *wrappers.StringValue，
// SetIfNotNil 只在调用方真的传了该字段时才写进 updates。
// 换成「取 GetValue() 再判空串」会把「传了空串」和「没传」混为一谈，
// 于是清空某个字段永远做不到。
//
// [EN] Partial updates are assembled with pbmo.NewUpdates(): optional request fields are
// wrapper types, and SetIfNotNil only writes when the caller actually sent the field. Reading
// GetValue() and testing for "" would conflate "sent empty" with "not sent", making it
// impossible to clear a field.
func (s *ProductService) ProductUpdate(
	ctx context.Context, req *pb.ProductUpdateRequest,
) (*pb.ProductUpdateResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	id := req.GetProductId()
	model, err := s.repo.GetByIDCached(ctx, tenantID, id)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if model == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeProductNotFound)
	}

	builder := pbmo.NewUpdates().
		SetIfNotNil("name", req.GetName()).
		SetIfNotZero("level", req.GetLevel())

	// tags 用一个独立的布尔开关控制是否覆盖：空数组既可能是「清空」也可能是「没传」，
	// 协议中心为此在请求里专门加了 update_tags
	// [EN] An empty array means either "clear" or "not sent", so the hub added update_tags
	if req.GetUpdateTags().GetValue() {
		builder.SetJSONSlice("tags", req.GetTags())
	}
	if req.GetContact() != nil {
		// SetJSON 的入参是「已序列化好的 JSON 字符串」，不是对象；
		// 这里直接 Set 一个 ProtoJSON 包装体，由它的 driver.Valuer 负责序列化，
		// 和 model 落库走的是同一条路径，不会出现两种 JSON 形状
		// [EN] SetJSON takes an already-serialized JSON string, not an object. Setting the
		// ProtoJSON wrapper instead reuses its driver.Valuer, so the stored shape matches
		// exactly what the model writes
		builder.Set("contact", sqltypes.ProtoJSON[*commonpb.ProductContact]{Data: req.GetContact()})
	}

	updates := builder.Build()
	if len(updates) > 0 {
		if err := s.repo.UpdateScoped(ctx, tenantID, id, updates); err != nil {
			gwglobal.LOGGER.ErrorContextKV(ctx, "更新失败", "id", id, "error", err)
			return nil, common.DBError(ctx)
		}
		gwglobal.LOGGER.InfoContextKV(ctx, "更新成功", "id", id, "fields", len(updates))
	}

	// 读回失败与「行不见了」分开判：并成一条的话，被并发删掉的记录会报成
	// 数据库错误（500），调用方于是去重试一个永远不会成功的请求
	// [EN] Split from the error branch: folded together, a row deleted concurrently reports as
	// a database error (500) and the caller retries a request that can never succeed
	fresh, err := s.repo.GetByIDCached(ctx, tenantID, id)
	if err != nil {
		return nil, common.DBError(ctx)
	}
	if fresh == nil {
		return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeProductNotFound)
	}
	info, err := fresh.ToPB(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.ProductUpdateResponse{Info: info}, nil
}

// ProductUpdateStatus 更新状态（局部更新）
func (s *ProductService) ProductUpdateStatus(
	ctx context.Context, req *pb.ProductUpdateStatusRequest,
) (*pb.ProductUpdateStatusResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	// 枚举的允许值由业务方法校验，不用 oneof 系列规则：
	// go-argus 会把 protobuf 枚举格式化成名称再比较，数字候选值会误拒合法请求
	// （协议中心的 proto 规范第 7 节写了这条）
	// [EN] Allowed enum values are checked here rather than with oneof-style tags
	if req.GetStatus() == enumspb.CommonStatus_COMMON_STATUS_UNSPECIFIED {
		return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeProductInvalidStatus,
			map[string]any{"reason": req.GetStatus().String()})
	}

	id := req.GetProductId()
	// SetInt32Val 收的是 *wrapperspb.Int32Value（给可选字段用）；
	// status 这里是必填枚举，已是确定值，直接 Set
	// [EN] SetInt32Val takes a wrapper for optional fields; status is a required enum here
	updates := pbmo.NewUpdates().Set("status", req.GetStatus()).Build()
	if err := s.repo.UpdateScoped(ctx, tenantID, id, updates); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "更新状态失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "更新状态成功", "id", id, "status", req.GetStatus().String())
	return &pb.ProductUpdateStatusResponse{}, nil
}

// ProductDelete 删除
func (s *ProductService) ProductDelete(
	ctx context.Context, req *pb.ProductDeleteRequest,
) (*pb.ProductDeleteResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	id := req.GetProductId()
	if err := s.repo.DeleteScoped(ctx, tenantID, id); err != nil {
		gwglobal.LOGGER.ErrorContextKV(ctx, "删除失败", "id", id, "error", err)
		return nil, common.DBError(ctx)
	}
	gwglobal.LOGGER.InfoContextKV(ctx, "删除成功", "id", id)
	return &pb.ProductDeleteResponse{}, nil
}

// ProductBatchDelete 批量删除
//
// 逐条返回结果而不是整体成败：调用方据此只重试失败的那几条。
// 条数上限由 proto 的 validate:"required,max=100" 保证，这里不必再查一遍。
//
// [EN] Per-item results rather than all-or-nothing, so the caller retries only what failed.
// The count cap is enforced by the proto's validate tag, so it is not re-checked here.
func (s *ProductService) ProductBatchDelete(
	ctx context.Context, req *pb.ProductBatchDeleteRequest,
) (*pb.ProductBatchDeleteResponse, error) {
	tenantID, err := common.TenantOf(ctx)
	if err != nil {
		return nil, err
	}

	ids := req.GetProductIds()
	results := make([]*commonpb.BatchResult, 0, len(ids))
	var failed int
	for _, id := range ids {
		if err := s.repo.DeleteScoped(ctx, tenantID, id); err != nil {
			failed++
			results = append(results, &commonpb.BatchResult{
				Id:           id,
				Success:      false,
				ErrorMessage: gwmiddleware.GetMsgByKey(ctx, bizerror.BizErrCodeDatabaseOperationFailed),
			})
			continue
		}
		results = append(results, &commonpb.BatchResult{Id: id, Success: true})
	}

	gwglobal.LOGGER.InfoContextKV(ctx, "批量删除完成",
		"total", len(ids), "failed", failed)
	return &pb.ProductBatchDeleteResponse{Results: results}, nil
}
