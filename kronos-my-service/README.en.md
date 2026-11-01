# kronos MyService

A gRPC microservice for the my domain. Its contract comes from a shared contract hub, and the HTTP surface is exposed automatically from the proto annotations through grpc-gateway, so **one service implementation serves both gRPC and HTTP**.

[中文](./README.md)

## Stack

| Component | Purpose |
|-----------|---------|
| [go-rpc-gateway](https://github.com/kamalyes/go-rpc-gateway) | Framework: gRPC + HTTP, interceptor chain (rate limit / shedder / breaker / i18n / validation), cpool connections |
| [go-config](https://github.com/kamalyes/go-config) | Configuration, env prefix `KRONOS` |
| [go-pbmo](https://github.com/kamalyes/go-pbmo) | Model ↔ Protobuf conversion by reflection, no hand-written DTOs |
| [go-sqlbuilder](https://github.com/kamalyes/go-sqlbuilder) | Generic repository base plus enforced parameterised queries |
| [go-bizerror](https://github.com/kamalyes/go-bizerror) | Business error codes → gRPC/gateway status codes, message resolved per request language |
| [go-i18n](https://github.com/kamalyes/go-i18n) | i18n runtime |
| github.com/kamalyes/kronos-share-proto-template | Contract hub: proto definitions and generated pb code |

## Layout

```
kronos-my-service/
├── main.go                  Entry point, bootstrap only
├── bootstrap/               Startup order: config → error codes → database → services → gRPC/HTTP
│   ├── server.go            The full Run() sequence
│   ├── errcodes.go          Business error-code registration, must precede any request
│   ├── services.go          Dependency injection plus gRPC and HTTP handler registration
│   ├── pbmo.go              Model ↔ PB converter registration
│   ├── database.go          Migration, indexes, table comments, seed data
│   └── kvcache.go           KV and model caches
├── models/                  One file per entity
│   └── table_names.go       Table-name constants, taken from TableName()
├── repository/              Data access
│   └── factory.go           Repository factory singleton
├── service/                 Business logic, one package per contract-hub module
│   ├── catalog/            Product, full CRUD
│   ├── inventory/          Warehouse
│   ├── tenant/             Tenant + TenantMember (several services in one module)
│   └── common/              Shared base: paging, conversion, clients, cache
├── errcodes/codes.go        Error-code constants and the status-code mapping table
├── migrations/              Auto migration: model list, indexes, table comments
├── locales/                 Translations; an error code IS its i18n key
├── resources/               Per-environment configuration
└── scripts/init.sh          One-shot initialisation
```

## Getting started

```bash
bash scripts/init.sh        # Linux / macOS
scripts\init.bat            # Windows
```

Five stages, and the order matters:

| Stage | What it does | Why here |
|-------|--------------|----------|
| 1/5 | Resolve the contract-hub dependency | The proto contract is the prerequisite for everything |
| 2/5 | `go mod tidy` | — |
| 3/5 | `go fmt ./...` | tidy may rewrite go.mod, so formatting must finish before the build |
| 4/5 | `go build ./...` | — |
| 5/5 | `go test ./...` | — |

Flags: `--skip-proto` (version pinned by hand, or a local replace is open), `--skip-fmt`, `--skip-test`.

Then run it:

```bash
go run .                    # Reads resources/kronos-my-service-dev.yaml when APP_ENV is unset
```

| Port | Purpose |
|------|---------|
| 9190 | gRPC |
| 8190 | HTTP, routes registered automatically by grpc-gateway |
| 8191 | Health checks and metrics |

## Adding an entity

**Define the proto in the contract hub first** — the contract is the single source of truth and a service cannot invent an entity locally.

One entity then cuts through these layers:

```
models/<entity>_model.go          new: fields + TableName() + TableComment()
models/table_names.go             add the table-name constant
repository/<entity>_repository.go new: extend BaseRepository / CachedRepository
repository/factory.go             add a field AND an initialiser (two places)
service/<module>/<entity>_service.go  new: implement the proto ServiceServer
errcodes/codes.go                 add the constants AND register them in Mappings()
locales/*.json                    every language file; the key sets must stay equal
migrations/auto_migrate.go        model list, indexes, table comments (three places)
bootstrap/pbmo.go                 register the Model ↔ PB converter
bootstrap/server.go               add the field to the Gateway struct
bootstrap/services.go             init + gRPC registration + HTTP handler (three places)
```

**There is a tool for this, do not do it by hand**: [kronos-codegen](https://github.com/kamalyes/kronos-codegen) creates the files and inserts every wiring point above idempotently in one command.

```bash
kronos-codegen new entity <Name> --dry-run    # preview the changes
kronos-codegen new entity <Name>              # write after confirming
kronos-codegen check --entity <Name>          # drift check, exit code 3 when something is missing
```

Doing it by hand is risky because **the cost of missing one place is asymmetric**: miss `bootstrap/pbmo.go` and it compiles but fails to convert at runtime; miss `bootstrap/server.go` and it fails to compile; miss one language file and that language silently falls back.

Step-by-step patterns live in `.trae/skills/my-service-dev/SKILL.md`.

## Error codes and i18n

Business code is one line:

```go
return nil, bizerror.ToGRPCError(ctx, errcodes.ProductNotFound)
```

The framework handles the rest: error code → message in the request language → gRPC status mapping → HTTP JSON shape.

**The error-code string IS the i18n key**, so there is only one name to maintain:

```go
ProductNotFound = "error.product_not_found"
```

⚠️ **Every code that does not end in `_not_found` MUST be registered explicitly in `Mappings()`.** go-bizerror only auto-maps the `_not_found` suffix to `NotFound`; everything else falls back to `Internal`. An unregistered `_already_exists` therefore returns **500 instead of 409**, with no warning anywhere.

When adding a code, change both places in `errcodes/codes.go` (the constant and `Mappings()`) and **all** files under `locales/`.

## Example modules

Three example groups are generated by default, covering three shapes:

| Module | Entity | Shape |
|--------|--------|-------|
| `catalog` | Product | One service, full CRUD |
| `inventory` | Warehouse | One service whose update request uses wrapper optionals |
| `tenant` | Tenant + TenantMember | Several services sharing one proto and Go package |

A real service usually implements only its own module, so **delete the groups you do not need as a whole** (model, repository, service, error codes, locale keys, and every wiring point).

Pass `--var GenerateExample=false` at generation time to get the scaffolding without examples.

## Conventions

```bash
go fmt ./...          # always before committing
go vet ./...
golangci-lint run     # configured in .golangci.yml
go test ./...
```

- **All SQL goes through sqlbuilder**; order-by fields need the whitelist (`AddSafeOrderFromSort`), never string concatenation
- **Model field names must line up with the proto field names**, otherwise pbmo cannot convert
- IDs use snowflake: `models.NewSnowflakeID()`
- You do not write validation: rules injected via `@inject_tag: validate:"..."` in the proto are applied by the framework's validation interceptor
