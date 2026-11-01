/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\errcodes\codes.go
 * @Description: 本服务专属业务错误码，以及它们到 gRPC / 网关状态码的映射
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package errcodes 声明本服务专属的业务错误码，并给出它们的状态码映射
//
// 为什么需要这一层：go-bizerror 只持有跨服务通用的码（error.not_found、error.conflict
// 这类），实体专属语义必须由各服务自己定义通用码只能回答「冲突了」，回答不了
// 「哪个实体的哪个字段冲突了」，而调用方正是靠后者决定换个编码重试还是提示用户改输入
//
// 为什么叫 errcodes 而不是 errors：包名 errors 会在本包内遮蔽标准库的 errors，
// 而本包只放「码 + 映射」、不提供任何错误构造函数（构造在 go-bizerror），
// 叫 errcodes 名实相符，也不会让人误以为这里有 New / Wrap 之类
//
// 三条约定
//
// 一、码值本身就是 i18n key go-bizerror 的内置码就是这么定的
// （BizErrCodeNotFound = "error.not_found"），所以这里的每个码值都要在
// locales/*.json 的 error 对象下有同名键，28 个语言文件都要有
//
// 二、常量名统一 BizErrCode<实体><语义>，与 go-bizerror 的命名对齐，
// 让通用码与专属码在调用处读起来是同一套东西
//
// 三、新增码必须同时做三件事：加常量、在 Mappings 里登记映射、补 28 个语言文件
// 漏掉第二件事的后果见 Mappings 的注释
//
// [EN] This package declares the service's own business error codes and their status mappings.
// go-bizerror holds only the cross-service generic codes, so entity-specific semantics have to
// live here: a generic code can say "there was a conflict" but not which entity and which field
// conflicted, which is exactly what the caller needs in order to decide between retrying with
// another code and asking the user to change the input. The package is named errcodes rather
// than errors because the latter would shadow the standard library inside this package, and
// because this package holds codes and mappings only, with no error constructors. Three
// conventions: a code's value IS its i18n key, matching how go-bizerror defines its builtin
// codes, so every value here needs the same key under the error object of all 28 locale files;
// constant names follow BizErrCode<Entity><Semantics> so generic and specific codes read as one
// system at the call site; and adding a code means adding the constant, registering its mapping
// in Mappings and filling in all 28 locale files.
package errcodes

import (
	bizerror "github.com/kamalyes/go-bizerror"
	gwerrors "github.com/kamalyes/go-rpc-gateway/errors"
	"google.golang.org/grpc/codes"
)

// ============================================================================
// Product（catalog 模块）| [EN] Product, the catalog module
// ============================================================================

const (
	// BizErrCodeProductNotFound Product 不存在
	//
	// 用在按 ID 查不到记录的地方（Get / Update 读回时）仓储查不到返回的是
	// (nil, nil) 而不是 error，所以「查询失败」与「查到了但没有」是两条路，
	// 前者归并成数据库错误，后者才是这个码
	//
	// [EN] Used where a lookup by id finds no row. The repository returns (nil, nil) rather
	// than an error for a miss, so "the query failed" and "the query found nothing" are two
	// different paths: the first becomes a database error, only the second becomes this code
	BizErrCodeProductNotFound = "error.product_not_found"

	// BizErrCodeProductAlreadyExists Product 编码已存在
	//
	// 唯一性冲突模型上 code 列在租户内唯一，创建前先查一次，
	// 用 {{reason}} 把重复的那个编码带回去，调用方不必再猜是哪个字段撞了
	//
	// [EN] A uniqueness conflict: the code column is unique per tenant, checked before insert.
	// The offending value travels back through {{reason}} so the caller need not guess which
	// field collided
	BizErrCodeProductAlreadyExists = "error.product_already_exists"

	// BizErrCodeProductInvalidStatus 请求里的 Product 状态枚举不合法
	//
	// 这是**入参**不合法，不是资源状态不合法，所以映射成 InvalidArgument
	// 枚举的允许值由业务方法校验而不是 go-argus 的 oneof 规则，原因见 service 层注释
	//
	// [EN] The *request* is invalid rather than the resource's state, hence InvalidArgument.
	// Allowed enum values are checked in the business method rather than with a oneof tag; the
	// service layer explains why
	BizErrCodeProductInvalidStatus = "error.product_invalid_status"
)

// ============================================================================
// Warehouse（inventory 模块）| [EN] Warehouse, the inventory module
// ============================================================================

const (
	// BizErrCodeWarehouseNotFound Warehouse 不存在
	// [EN] Warehouse does not exist
	BizErrCodeWarehouseNotFound = "error.warehouse_not_found"

	// BizErrCodeWarehouseAlreadyExists Warehouse 编码已存在（租户内唯一性冲突）
	// [EN] A uniqueness conflict on the per-tenant code column
	BizErrCodeWarehouseAlreadyExists = "error.warehouse_already_exists"

	// BizErrCodeWarehouseInvalidStatus 请求里的 Warehouse 状态枚举不合法
	// [EN] The status enum in the request is not an allowed value
	BizErrCodeWarehouseInvalidStatus = "error.warehouse_invalid_status"
)

// ============================================================================
// Tenant 与 TenantMember（tenant 模块）
// [EN] Tenant and TenantMember, the tenant module
// ============================================================================

const (
	// BizErrCodeTenantNotFound Tenant 不存在
	//
	// 聚合根不存在成员接口把成员加到不存在的聚合根上也用这个码，
	// 而不是成员的 not_found——缺的是父资源，报成员不存在会把调用方引到错误的地方
	//
	// [EN] The aggregate root is missing. The member endpoints report this code too when the
	// owning root is absent, rather than the member's own not-found: what is missing is the
	// parent, and blaming the member sends the caller looking in the wrong place
	BizErrCodeTenantNotFound = "error.tenant_not_found"

	// BizErrCodeTenantAlreadyExists Tenant 编码已存在
	//
	// 与前两个实体不同，Tenant 自己就是租户，code 是**全局唯一**而非租户内唯一
	// [EN] Unlike the other two entities this one IS the tenant, so its code is globally unique
	BizErrCodeTenantAlreadyExists = "error.tenant_already_exists"

	// BizErrCodeTenantInvalidStatus Tenant 当前状态不允许该操作
	//
	// 注意它与前两个实体的同名码**语义不同**，映射也因此不同：
	// 前两个是「请求里的状态枚举不合法」（入参问题，InvalidArgument），
	// 这一个是「资源当前的状态不满足操作的前置条件」（状态问题，FailedPrecondition）
	// 比如冻结的 Tenant 不该再触发下游同步——请求本身没错，是资源不在可操作的状态
	//
	// [EN] The same-named codes of the other two entities mean something different, and map
	// differently as a result: there the status enum in the request is invalid, an argument
	// problem; here the resource's current state does not satisfy the operation's precondition,
	// so it is FailedPrecondition. A frozen root must not trigger a downstream sync: nothing is
	// wrong with the request, the resource is simply not in an operable state
	BizErrCodeTenantInvalidStatus = "error.tenant_invalid_status"

	// BizErrCodeTenantRegionAssignmentNotFound 地区分配不存在
	//
	// 子资源有自己的 not_found：父存在而子不存在时报父的 not_found，
	// 调用方会以为自己拿错了父 ID，于是去查一个其实没问题的东西
	//
	// [EN] The sub-resource has its own not-found. Reporting the parent's when the parent
	// exists but the child does not makes the caller believe it holds the wrong parent id and
	// go debugging something that is in fact correct
	BizErrCodeTenantRegionAssignmentNotFound = "error.tenant_region_assignment_not_found"
)

const (
	// BizErrCodeTenantMemberNotFound Tenant 成员不存在
	// [EN] The member does not exist under this root
	BizErrCodeTenantMemberNotFound = "error.tenant_member_not_found"

	// BizErrCodeTenantMemberAlreadyExists Tenant 成员已存在
	//
	// 两个来源：同一个 user_id 重复加入，以及批量邀请里重复或已存在的邮箱
	// {{reason}} 带回具体的 user_id 或邮箱，批量接口逐条返回时正好用得上
	//
	// [EN] Two sources: the same user id added twice, and an email that repeats within a batch
	// invite or already exists. {{reason}} carries the offending user id or email, which is
	// exactly what the batch endpoint's per-item results need
	BizErrCodeTenantMemberAlreadyExists = "error.tenant_member_already_exists"

	// BizErrCodeTenantMemberInvalidRole 角色枚举不合法
	//
	// proto 上的 validate:"required,max=8" 只保证「传了、不超过 8 个」，
	// 保证不了每个值都是合法枚举——UNSPECIFIED 能过校验，所以这一条在业务方法里拦
	//
	// [EN] The proto's validate tag guarantees only that roles were sent and that there are at
	// most eight; it cannot guarantee each value is a real role, since UNSPECIFIED passes. The
	// business method therefore rejects it
	BizErrCodeTenantMemberInvalidRole = "error.tenant_member_invalid_role"
)

// Mappings 返回本服务专属错误码到 gRPC 状态码与网关错误码的映射
//
// 为什么每个码都要在这里登记：go-bizerror 的 Mapping 只认三件事——
// 显式注册过的映射、以 _not_found 结尾的码（自动映射成 NotFound），其余一律兜底
// Internal 也就是 500所以 error.xxx_already_exists 这种码不登记就是
// 「唯一性冲突返回 500」，而 500 让调用方以为是服务挂了、于是去重试
//
// 以 _not_found 结尾的码靠后缀规则也能对，但这里仍然逐条登记，两个理由：
// 一是整张状态码契约在一个地方读完，不必先知道有条后缀规则才能看懂；
// 二是哪天有人把码重命名成 error.xxx_missing，显式登记只会让这里报编译错误，
// 而靠后缀规则的话状态码会从 404 静默变成 500
//
// [EN] Every code must be registered here because go-bizerror's Mapping recognizes only three
// things: an explicitly registered mapping, a code ending in _not_found (mapped to NotFound),
// and everything else falling back to Internal, i.e. 500. An unregistered
// error.xxx_already_exists therefore answers a uniqueness conflict with a 500, which tells the
// caller the service is broken and invites a retry. Codes ending in _not_found would be covered
// by the suffix rule, yet they are listed anyway for two reasons: the whole status contract can
// then be read in one place without first knowing the rule exists, and renaming a code to
// something like error.xxx_missing becomes a compile error here instead of silently turning a
// 404 into a 500.
//
// 复合字面量写字段名（GRPC: / Gateway:）不是啰嗦：go vet 的 composites 检查专门拦
// 跨包类型的无字段名字面量，而 CI 里 go vet ./... 是独立一步，省掉字段名直接红
// 字段名同时也是这张表的列头，不必回去翻 CodeMapping 的定义才知道哪个是哪个
//
// [EN] The field names are not noise: go vet's composites check rejects unkeyed literals of a
// type from another package, and go vet ./... is a CI step of its own, so omitting them turns
// it red. They also act as this table's column headers, so reading it does not require looking
// up CodeMapping's definition.
func Mappings() map[string]bizerror.CodeMapping {
	return map[string]bizerror.CodeMapping{
		// Product
		BizErrCodeProductNotFound:      {GRPC: codes.NotFound, Gateway: gwerrors.ErrCodeNotFound},
		BizErrCodeProductAlreadyExists: {GRPC: codes.AlreadyExists, Gateway: gwerrors.ErrCodeConflict},
		BizErrCodeProductInvalidStatus: {GRPC: codes.InvalidArgument, Gateway: gwerrors.ErrCodeBadRequest},

		// Warehouse
		BizErrCodeWarehouseNotFound:      {GRPC: codes.NotFound, Gateway: gwerrors.ErrCodeNotFound},
		BizErrCodeWarehouseAlreadyExists: {GRPC: codes.AlreadyExists, Gateway: gwerrors.ErrCodeConflict},
		BizErrCodeWarehouseInvalidStatus: {GRPC: codes.InvalidArgument, Gateway: gwerrors.ErrCodeBadRequest},

		// Tenant
		BizErrCodeTenantNotFound:                 {GRPC: codes.NotFound, Gateway: gwerrors.ErrCodeNotFound},
		BizErrCodeTenantAlreadyExists:            {GRPC: codes.AlreadyExists, Gateway: gwerrors.ErrCodeConflict},
		BizErrCodeTenantInvalidStatus:            {GRPC: codes.FailedPrecondition, Gateway: gwerrors.ErrCodeBadRequest},
		BizErrCodeTenantRegionAssignmentNotFound: {GRPC: codes.NotFound, Gateway: gwerrors.ErrCodeNotFound},

		// TenantMember
		BizErrCodeTenantMemberNotFound:      {GRPC: codes.NotFound, Gateway: gwerrors.ErrCodeNotFound},
		BizErrCodeTenantMemberAlreadyExists: {GRPC: codes.AlreadyExists, Gateway: gwerrors.ErrCodeConflict},
		BizErrCodeTenantMemberInvalidRole:   {GRPC: codes.InvalidArgument, Gateway: gwerrors.ErrCodeBadRequest},
	}
}
