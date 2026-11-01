---
name: "kronos-service"
description: "Scaffolds Go gRPC microservice components (model/repo/service/migration/locale). Invoke when adding new business entities, CRUD APIs, or gRPC endpoints to an kronos-service project."
---

# kronos Service - Go gRPC Microservice Development

You are an expert in the kronos microservice architecture. When the user asks to add a new feature, entity, or API to an kronos-service project, follow these conventions precisely.

## Project Architecture

```
project/
├── bootstrap/          # Server init, DI, gRPC/HTTP registration
│   ├── server.go           # Gateway struct, Run(), lifecycle
│   ├── services.go         # initServices(), registerGRPCServices(), registerHTTPHandlers()
│   ├── database.go         # initDatabase(), migration trigger
│   ├── errcodes.go         # registerErrorCodes(), called first in Run()
│   └── subscriber.go       # Event subscribers (optional)
├── constants/          # Service name constants
├── errcodes/           # THIS service's own business error codes
│   └── codes.go            # BizErrCode<Entity><Semantics> constants + Mappings()
├── locales/            # i18n JSON files, 28 of them (zh.json, en.json, ...)
├── migrations/         # Database auto-migration
│   ├── auto_migrate.go     # GetAllModels(), GetAllIndexes(), GetAllComments()
│   └── migrate.go          # SeedData()
├── middleware/          # gRPC interceptors
├── models/             # GORM models + base types
│   ├── base.go             # BuildPagination(), PBConvertible, LocalizedTextList
│   └── table_names.go      # TableName constants
├── repository/         # Data access layer
│   ├── factory.go          # RepositoryFactory singleton
│   └── *_repository.go     # Per-entity repositories
├── service/            # Business logic layer
│   ├── idgen.go            # NewSnowflakeID()
│   ├── *_service.go        # Main service impl (holds RepositoryFactory)
│   └── *_service.go        # Per-entity service methods
└── resources/          # YAML configs (gateway-{env}.yaml)
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/kamalyes/go-rpc-gateway` | Gateway framework (gRPC + HTTP) |
| `github.com/kamalyes/go-sqlbuilder` | ORM base repository, migrator |
| `github.com/kamalyes/go-pbmo` | Proto ↔ Model bidirectional conversion |
| `github.com/kamalyes/go-config` | Environment config |
| `github.com/kamalyes/go-toolbox` | ID generation, utilities |
| `github.com/kamalyes/go-bizerror` | Generic business error codes, i18n errors, status mapping |
| `{org}/{prefix}-share-proto/pb/*` | Generated proto Go code |

## Adding a New Entity - Complete Checklist

When the user says "add a XXX entity/feature", perform ALL of the following steps:

### Step 1: Model (`models/xxx_model.go`)

```go
package models

import (
    "time"
    commonpb "{share-proto-module}/pb/common"
    "github.com/kamalyes/go-pbmo"
)

// XxxModel - field names must match proto for PBMO auto-conversion
type XxxModel struct {
    XxxId      string                    `gorm:"column:xxx_id;type:varchar(36);primaryKey;comment:XXX ID" json:"xxx_id"`
    Name       string                    `gorm:"column:name;type:varchar(128);not null;comment:名称" json:"name"`
    Status     int32                     `gorm:"column:status;type:tinyint;default:1;not null;comment:状态(1:正常,2:禁用)" json:"status"`
    I18nNames  models.LocalizedTextList  `gorm:"column:i18n_names;type:json;comment:多语言名称" json:"i18n_names"`
    CreatedAt  time.Time                 `gorm:"column:created_at;autoCreateTime;comment:创建时间" json:"created_at"`
    UpdatedAt  time.Time                 `gorm:"column:updated_at;autoUpdateTime;comment:更新时间" json:"updated_at"`
}

func (XxxModel) TableName() string { return "xxxs" }
func (XxxModel) TableComment() string { return "XXX表" }
```

**Rules:**
- Field names must match proto field names (PascalCase) for PBMO compatibility
- Use `varchar(36)` for string PKs (Snowflake short hash)
- Use `tinyint` for status/enums
- Use `models.LocalizedTextList` + `gorm:"type:json"` for i18n fields
- Always include `CreatedAt`/`UpdatedAt` with auto time tags

### Step 2: Table Name (`models/table_names.go`)

Add a new variable:
```go
var TableNameXxx = XxxModel{}.TableName()
```

### Step 3: Repository (`repository/xxx_repository.go`)

```go
package repository

import (
    "context"
    "{module}/models"
    sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
)

type XxxRepository struct {
    *sqlrepo.BaseRepository[models.XxxModel]
}

func NewXxxRepository() *XxxRepository {
    return &XxxRepository{
        BaseRepository: newBaseRepository[models.XxxModel](models.XxxModel{}.TableName()),
    }
}

func (r *XxxRepository) GetByID(ctx context.Context, id string) (*models.XxxModel, error) {
    return r.GetByFilter(ctx, sqlrepo.NewEqFilter("xxx_id", id))
}

func (r *XxxRepository) ListWithPagination(ctx context.Context, query *sqlrepo.Query, pagination *sqlrepo.Pagination32) ([]*models.XxxModel, *sqlrepo.Pagination32, error) {
    return r.ListWithPagination32(ctx, query, pagination)
}

func (r *XxxRepository) UpdateByID(ctx context.Context, id string, updates map[string]interface{}) error {
    return r.UpdateFieldsByFilters(ctx, updates, sqlrepo.NewEqFilter("xxx_id", id))
}

func (r *XxxRepository) DeleteByID(ctx context.Context, id string) error {
    return r.DeleteByFilters(ctx, sqlrepo.NewEqFilter("xxx_id", id))
}
```

**Available BaseRepository methods:** `Create`, `GetByFilter`, `Count`, `ListWithPagination32`, `UpdateFieldsByFilters`, `DeleteByFilters`, `ExistsByFilter`

### Step 4: Register Repository in Factory (`repository/factory.go`)

Add field to `RepositoryFactory` struct and initialize in `NewRepositoryFactory()`:
```go
type RepositoryFactory struct {
    Xxx *XxxRepository  // add this
}

factoryOnce.Do(func() {
    factoryInstance = &RepositoryFactory{
        Xxx: NewXxxRepository(),  // add this
    }
})
```

### Step 5: Service (`service/xxx_service.go`)

```go
package service

import (
    "context"
    "{module}/models"
    "{module}/repository"
    sqlrepo "github.com/kamalyes/go-sqlbuilder/repository"
)

type XxxService struct {
    repo *repository.XxxRepository
}

func NewXxxService(repo *repository.XxxRepository) *XxxService {
    return &XxxService{repo: repo}
}

func (s *XxxService) GetByID(ctx context.Context, id string) (*models.XxxModel, error) {
    return s.repo.GetByID(ctx, id)
}

func (s *XxxService) List(ctx context.Context, query *sqlrepo.Query, pagination *sqlrepo.Pagination32) ([]*models.XxxModel, *sqlrepo.Pagination32, error) {
    return s.repo.ListWithPagination(ctx, query, pagination)
}

func (s *XxxService) Create(ctx context.Context, model *models.XxxModel) error {
    _, err := s.repo.Create(ctx, model)
    return err
}

func (s *XxxService) Update(ctx context.Context, id string, updates map[string]interface{}) error {
    return s.repo.UpdateByID(ctx, id, updates)
}

func (s *XxxService) Delete(ctx context.Context, id string) error {
    return s.repo.DeleteByID(ctx, id)
}
```

### Step 6: Register Service in Main Service (`service/{service_name}_service.go`)

Add the sub-service field and initialize it:
```go
type XxxServiceImpl struct {
    repo *repository.RepositoryFactory
    xxx  *XxxService  // add this
}

func NewXxxService(repo *repository.RepositoryFactory) *XxxServiceImpl {
    return &XxxServiceImpl{
        repo: repo,
        xxx:  NewXxxService(repo.Xxx),  // add this
    }
}
```

### Step 7: Migration (`migrations/auto_migrate.go`)

Add to `GetAllModels()`:
```go
&models.XxxModel{},
```

Add to `GetAllIndexes()` if needed:
```go
{
    Table:   models.TableNameXxx,
    Name:    "idx_xxxs_name",
    Columns: []string{"name"},
    Unique:  true,
},
```

Add to `GetAllComments()`:
```go
{
    Table:   models.TableNameXxx,
    Comment: models.XxxModel{}.TableComment(),
},
```

### Step 8: Error Codes (`errcodes/codes.go`)

Two edits, and **both are mandatory**. Only the first one compiles on its own, which is exactly
why it gets forgotten.

**8a. Declare the constants.** A code's value IS its i18n key. Generic codes live in
`go-bizerror`; only entity-specific semantics belong here.

```go
const (
    BizErrCodeXxxNotFound      = "error.xxx_not_found"
    BizErrCodeXxxAlreadyExists = "error.xxx_already_exists"
    BizErrCodeXxxInvalidStatus = "error.xxx_invalid_status"
)
```

**8b. Register each code's mapping in `Mappings()`.**

```go
BizErrCodeXxxNotFound:      {GRPC: codes.NotFound, Gateway: gwerrors.ErrCodeNotFound},
BizErrCodeXxxAlreadyExists: {GRPC: codes.AlreadyExists, Gateway: gwerrors.ErrCodeConflict},
BizErrCodeXxxInvalidStatus: {GRPC: codes.InvalidArgument, Gateway: gwerrors.ErrCodeBadRequest},
```

`bizerror.Mapping()` resolves a code in exactly three steps: an explicitly registered mapping
wins; otherwise a code ending in `_not_found` maps to `NotFound`; otherwise **everything falls
back to `Internal`, i.e. 500**. So a declared-but-unregistered `error.xxx_already_exists`
answers a uniqueness conflict with a 500, which tells the caller the service is broken and
invites it to retry a request that can never succeed.

Pick the gRPC code by what is actually wrong:

| Situation | GRPC | Gateway |
|---|---|---|
| Resource does not exist | `codes.NotFound` | `ErrCodeNotFound` |
| Uniqueness conflict | `codes.AlreadyExists` | `ErrCodeConflict` |
| Request field or enum is invalid | `codes.InvalidArgument` | `ErrCodeBadRequest` |
| Resource's current state forbids the operation | `codes.FailedPrecondition` | `ErrCodeBadRequest` |

The last two are easy to confuse: an invalid enum **in the request** is `InvalidArgument`, while
"this resource is frozen so the action is not allowed" is `FailedPrecondition` — nothing is
wrong with the request there.

Always write the field names (`GRPC:` / `Gateway:`): `go vet`'s composites check rejects unkeyed
literals of a type from another package, and `go vet ./...` is its own CI step.

Nothing needs wiring: `bootstrap/errcodes.go` calls
`bizerror.RegisterMany(errcodes.Mappings())` once at the top of `Run()`, before any request can
arrive, so a new entry in `Mappings()` takes effect on its own.

### Step 9: Locale Files (`locales/*.json`)

Add every new key to **all 28** locale files, and keep the key sets exactly equal across them: a
key missing from one language falls back for that language only, which is the kind of bug that
shows up solely under a particular `Accept-Language`.

Keys go **under the `error` object without the `error.` prefix**. go-i18n flattens nested
objects with dots at load time, so `error` + `xxx_not_found` is the code value
`error.xxx_not_found`. Do not write the dotted key at the top level; it would work, but it would
no longer match the layout the other 28 files use.

`en.json`:
```json
"error": {
  "xxx_not_found": "Xxx not found",
  "xxx_already_exists": "Xxx already exists: {{reason}}",
  "xxx_invalid_status": "Invalid Xxx status: {{reason}}"
}
```

`zh.json`:
```json
"error": {
  "xxx_not_found": "Xxx 不存在",
  "xxx_already_exists": "Xxx 已存在：{{reason}}",
  "xxx_invalid_status": "Xxx 状态无效：{{reason}}"
}
```

Template variables use double braces (`{{reason}}`) and pair with
`ToGRPCErrorWithTemplate(ctx, code, map[string]any{"reason": ...})`. A message that carries a
placeholder must be resolved through a template-aware function: `GetMsgByKey` does not apply
templates and hands the literal `{{reason}}` to the caller, so a batch endpoint building
per-item messages needs `GetMsgWithMap`.

See `locales/CONVENTIONS.md` for the full language list and translation rules.

### Step 10: gRPC/HTTP Registration (`bootstrap/services.go`)

Uncomment and fill in the gRPC and HTTP handler registration:
```go
// In registerGRPCServices():
pb.RegisterXxxServiceServer(srv, s.svc)

// In registerHTTPHandlers():
s.gateway.RegisterGatewayHandler(func(ctx context.Context, mux *runtime.ServeMux) error {
    return pb.RegisterXxxServiceHandlerServer(ctx, mux, s.svc)
})
```

## Error Handling Pattern

A repository miss returns `(nil, nil)`, not an error, so "the query failed" and "the query found
nothing" are two different branches and must map to two different codes. Folding them together
reports a concurrently deleted row as a 500 and sends the caller into a retry loop.

```go
func (s *XxxService) XxxGet(ctx context.Context, req *pb.XxxGetRequest) (*pb.XxxGetResponse, error) {
    model, err := s.repo.GetByIDCached(ctx, tenantID, req.GetXxxId())
    if err != nil {
        // 查询本身失败：日志记全文，响应只给错误码
        gwglobal.LOGGER.ErrorContextKV(ctx, "查询失败", "id", req.GetXxxId(), "error", err)
        return nil, common.DBError(ctx)
    }
    if model == nil {
        // 查到了但没有：实体专属 not_found
        return nil, bizerror.ToGRPCError(ctx, errcodes.BizErrCodeXxxNotFound)
    }
    ...
}
```

Uniqueness conflicts carry the offending value back through the template:

```go
if exists {
    return nil, bizerror.ToGRPCErrorWithTemplate(ctx, errcodes.BizErrCodeXxxAlreadyExists,
        map[string]any{"reason": req.GetCode()})
}
```

Which code to reach for:

- **`errcodes.BizErrCode<Entity>*`** whenever an entity can be named. The response then carries
  `error.<entity>_not_found` and the caller can see which resource is missing.
- **`go-bizerror` builtins** only when nothing entity-specific is involved:
  `BizErrCodeDatabaseOperationFailed` for an infrastructure failure (via `common.DBError`),
  `BizErrCodeUnauthenticated` for a missing tenant in the context (via `common.TenantOf`).
- **Never** the raw underlying error: it can carry table names, SQL fragments or connection
  strings. The full text goes to the log, the response carries only the code and its localized
  message.

Enum allowed-values are checked in the business method, not with a `oneof` struct tag: go-argus
formats a protobuf enum to its name before comparing, so numeric candidates reject valid
requests. A `validate:"required,max=8"` tag on a repeated enum guarantees only that values were
sent and how many — `UNSPECIFIED` passes it.

## ID Generation Pattern

Use Snowflake short IDs for new entities:
```go
model.XxxId = NewSnowflakeID()
```

## Pagination Pattern

```go
pagination := models.BuildPagination(req.Page, req.Size, total)
```

## i18n Field Pattern

For entities with multilingual names:
```go
// Model side:
I18nNames models.LocalizedTextList `gorm:"column:i18n_names;type:json"`

// Proto → Model:
model.I18nNames = models.ConvertI18NNamesToModel(req.I18nNames)

// Model → Proto:
pbNames := models.ConvertI18NNamesToPB(model.I18nNames)
```

## Important Rules

1. **NEVER** skip any step in the checklist - all layers must be consistent
2. **ALWAYS** add locale entries for new error codes in ALL 28 language files, keeping the key
   sets equal
3. **ALWAYS** register every new error code in `errcodes.Mappings()`; a code that is declared
   but not registered answers 500 unless it ends in `_not_found`
4. **ALWAYS** use `bizerror.ToGRPCError()` / `ToGRPCErrorWithTemplate()` for returning errors
   from service methods, with an `errcodes` code whenever an entity can be named
5. **ALWAYS** use `NewSnowflakeID()` for generating entity IDs
6. **ALWAYS** match model field names with proto field names for PBMO compatibility
7. **NEVER** use raw `status.Errorf()` in service code - use the i18n error system
8. **ALWAYS** add table name constants to `models/table_names.go`
9. **ALWAYS** register new repositories in `RepositoryFactory`
10. **ALWAYS** add models to `GetAllModels()` in migrations
