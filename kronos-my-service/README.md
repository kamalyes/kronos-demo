# kronos MyService

my 领域的 gRPC 微服务 契约来自协议中心,HTTP 接口由 proto 注解经 grpc-gateway 自动暴露,**同一套 service 实现同时服务 gRPC 与 HTTP**

[English](./README.en.md)

## 技术栈

| 组件 | 用途 |
|------|------|
| [go-rpc-gateway](https://github.com/kamalyes/go-rpc-gateway) | 框架:gRPC + HTTP 双协议、拦截器链(限流/降载/熔断/i18n/校验)、cpool 连接池 |
| [go-config](https://github.com/kamalyes/go-config) | 配置加载(环境变量前缀 `KRONOS`) |
| [go-pbmo](https://github.com/kamalyes/go-pbmo) | Model ↔ Protobuf 反射转换,消灭手写 DTO |
| [go-sqlbuilder](https://github.com/kamalyes/go-sqlbuilder) | 泛型 Repository 基类 + 强制参数化查询 |
| [go-bizerror](https://github.com/kamalyes/go-bizerror) | 业务错误码 → gRPC/网关状态码映射 + 按请求语言取文案 |
| [go-i18n](https://github.com/kamalyes/go-i18n) | 多语言运行时 |
| github.com/kamalyes/kronos-share-proto-template | 协议中心:proto 契约与生成的 pb 代码 |

## 项目结构

```
kronos-my-service/
├── main.go                  入口,只做 bootstrap
├── bootstrap/               启动编排:配置 → 错误码注册 → 数据库 → 服务装配 → gRPC/HTTP 注册
│   ├── server.go            Run() 的完整时序
│   ├── errcodes.go          业务错误码注册(必须早于任何请求进来)
│   ├── services.go          依赖注入 + gRPC 与 HTTP handler 注册
│   ├── pbmo.go              Model ↔ PB 转换器注册
│   ├── database.go          迁移 / 索引 / 表注释 / 种子数据
│   └── kvcache.go           KV 缓存与模型缓存
├── models/                  数据模型,一个实体一个文件
│   └── table_names.go       表名常量(由 TableName() 动态取得)
├── repository/              数据访问层
│   └── factory.go           仓库工厂单例
├── service/                 业务实现,按协议中心的模块分包
│   ├── catalog/            Product 完整 CRUD
│   ├── inventory/          Warehouse
│   ├── tenant/             Tenant + TenantMember(一个模块多个 service)
│   └── common/              公共基座:分页、转换、客户端、缓存
├── errcodes/codes.go        错误码常量与状态码映射表
├── migrations/              自动迁移:模型清单 / 索引 / 表注释
├── locales/                 多语言文案,错误码即 i18n 键
├── resources/               分环境配置
└── scripts/init.sh          一键初始化
```

## 快速开始

```bash
bash scripts/init.sh        # Linux / macOS
scripts\init.bat            # Windows
```

五个阶段,顺序是有讲究的:

| 阶段 | 做什么 | 为什么在这个位置 |
|------|--------|----------------|
| 1/5 | 解析协议中心依赖 | proto 契约是一切的前提 |
| 2/5 | `go mod tidy` | — |
| 3/5 | `go fmt ./...` | tidy 可能改 go.mod,格式化要在 build 前完成 |
| 4/5 | `go build ./...` | — |
| 5/5 | `go test ./...` | — |

可用开关:`--skip-proto`(已手工指定版本或打开本地 replace 时)`--skip-fmt` `--skip-test`

然后启动:

```bash
go run .                    # APP_ENV 未设置时读 resources/kronos-my-service-dev.yaml
```

| 端口 | 用途 |
|------|------|
| 9190 | gRPC |
| 8190 | HTTP(grpc-gateway 自动注册的路由) |
| 8191 | 健康检查与指标 |

## 添加一个业务实体

**先在协议中心定义 proto** —— 契约是单一来源,服务侧不能凭空造实体

然后一个实体要穿透这些层:

```
models/<entity>_model.go          新建:字段 + TableName() + TableComment()
models/table_names.go             加表名常量
repository/<entity>_repository.go 新建:继承 BaseRepository / CachedRepository
repository/factory.go             加字段 + 加初始化项(两处)
service/<module>/<entity>_service.go  新建:实现 proto 的 ServiceServer
errcodes/codes.go                 加错误码常量 + 加进 Mappings()
locales/*.json                    全部语言文件都要加键,键集合必须相等
migrations/auto_migrate.go        模型清单 / 索引 / 表注释(三处)
bootstrap/pbmo.go                 注册 Model ↔ PB 转换器
bootstrap/server.go               Gateway struct 加字段
bootstrap/services.go             初始化 + gRPC 注册 + HTTP handler(三处)
```

**这件事有工具,不要手工做**:[kronos-codegen](https://github.com/kamalyes/kronos-codegen) 能一条命令建文件并把上面全部接线点幂等插入

```bash
kronos-codegen new entity <Name> --dry-run    # 先看会改什么
kronos-codegen new entity <Name>              # 确认后写盘
kronos-codegen check --entity <Name>          # 校验是否都已落地,漂移时退出码 3
```

手工做的风险在于**漏一处的后果不对称**:漏 `bootstrap/pbmo.go` 编译能过但运行时转换失败,漏 `bootstrap/server.go` 直接编译失败,漏某个语言文件则该语言静默回退

详细步骤与代码范式见 `.trae/skills/my-service-dev/SKILL.md`

## 错误码与多语言

业务代码只写一行:

```go
return nil, bizerror.ToGRPCError(ctx, errcodes.ProductNotFound)
```

框架完成:错误码 → 按请求语言取文案 → gRPC status 映射 → HTTP JSON 结构

**错误码字符串本身就是 i18n 键**,所以只有一处命名:

```go
ProductNotFound = "error.product_not_found"
```

⚠️ **除 `_not_found` 结尾的码之外,必须在 `Mappings()` 里显式登记** go-bizerror 只对 `_not_found` 后缀自动映射成 `NotFound`,其余一律兜底 `Internal` —— 也就是说漏登记的 `_already_exists` 会变成 **500 而不是 409**,而且没有任何报错提示

新增错误码时同步改两处:`errcodes/codes.go` 的常量与 `Mappings()`,以及 `locales/` 下**全部**语言文件

## 示例模块

默认生成三组示例,覆盖三种形态:

| 模块 | 实体 | 形态 |
|------|------|------|
| `catalog` | Product | 单 service 完整 CRUD |
| `inventory` | Warehouse | 单 service,更新请求用 wrapper 可选字段 |
| `tenant` | Tenant + TenantMember | 一个模块多个 service 共用 proto 包与 Go 包 |

真实服务通常只实现自己那一个模块,**把不需要的整组删掉即可**(model / repository / service / 错误码 / locale 键 / 各处接线)

生成时用 `--var GenerateExample=false` 可以不生成示例,只要工程骨架

## 开发规范

```bash
go fmt ./...          # 提交前必跑
go vet ./...
golangci-lint run     # 配置在 .golangci.yml
go test ./...
```

- **SQL 一律走 sqlbuilder**,排序字段要用白名单(`AddSafeOrderFromSort`),不要手工拼接
- **model 字段名要与 proto 字段名对应**,否则 pbmo 转换不上
- ID 用雪花算法:`models.NewSnowflakeID()`
- 参数校验不用自己写:proto 里 `@inject_tag: validate:"..."` 注入的规则由框架的校验拦截器自动生效
