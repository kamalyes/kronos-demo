#!/usr/bin/env bash
# 一键打通：解析协议中心依赖 → go mod tidy → 格式化 → build → test
#
# 为什么需要这个脚本：go.mod 里协议中心的版本是占位符
# （v0.0.0-00010101000000-000000000000），直接 go mod tidy 解析不到。
# 协议契约不在本仓库，而在协议中心（kronos-share-proto-template 生成的仓库），
# 它的版本得先解析出来写进 go.mod。
#
# 协议中心还不存在时脚本会说清两条出路，而不是抛一串 Go 的 "Repository not found"。
#
# 幂等：重复执行只是重新解析并再 tidy 一遍，不会累积副作用。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

PROTO_MODULE="github.com/kamalyes/kronos-share-proto-template"
PROTO_VERSION="build"

if [[ -t 1 ]]; then
    C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'; C_CYAN=$'\033[36m'; C_OFF=$'\033[0m'
else
    C_GREEN=""; C_YELLOW=""; C_RED=""; C_CYAN=""; C_OFF=""
fi
log_info()    { echo "${C_CYAN}ℹ️  $1${C_OFF}"; }
log_success() { echo "${C_GREEN}✅ $1${C_OFF}"; }
log_warn()    { echo "${C_YELLOW}⚠️  $1${C_OFF}"; }
log_error()   { echo "${C_RED}❌ $1${C_OFF}" >&2; }

SKIP_TEST=0
SKIP_PROTO=0
SKIP_FMT=0
for arg in "$@"; do
    case "$arg" in
        --skip-test)  SKIP_TEST=1 ;;
        --skip-proto) SKIP_PROTO=1 ;;
        --skip-fmt)   SKIP_FMT=1 ;;
        --help|-h)
            echo "用法: bash scripts/init.sh [--skip-proto] [--skip-fmt] [--skip-test]"
            echo "  --skip-proto  跳过协议中心依赖解析（已手工指定版本或已打开本地 replace 时用）"
            echo "  --skip-fmt    跳过 go fmt（不建议：生成产物开箱就过不了 gofmt 检查）"
            echo "  --skip-test   跳过 go test 自检"
            exit 0
            ;;
        *) log_error "未知选项: $arg"; exit 1 ;;
    esac
done

echo "========================================"
echo "🚀 初始化 MyService"
echo "========================================"
echo ""

if ! command -v go >/dev/null 2>&1; then
    log_error "Go 未安装，请先安装 Go 1.25 或更高版本"
    exit 1
fi
log_success "Go: $(go version | awk '{print $3}')"
echo ""

# ── 1/5 协议中心依赖 ────────────────────────────────────────────────────────
# go.mod 里已打开本地 replace 时不必解析远端版本，占位符也不会被用到
if grep -qE '^replace[[:space:]]+github.com/kamalyes/kronos-share-proto-template' go.mod 2>/dev/null; then
    log_warn "[1/5] go.mod 里已打开本地 replace，跳过远端版本解析"
elif [[ "$SKIP_PROTO" == 1 ]]; then
    log_warn "[1/5] 按 --skip-proto 跳过协议中心依赖解析"
else
    log_info "[1/5] 解析协议中心依赖 ${PROTO_MODULE}@${PROTO_VERSION}"
    # 先摘掉已有的 require 再 go get：go get 会先解析 go.mod 里现存的版本约束，
    # 若那里是个占位或写坏的版本，会直接报 invalid version 而拿不到 latest
    # 模块本就不在 require 时 droprequire 是无副作用的空操作
    #
    # [EN] Drop any existing requirement first: go get resolves the version already in
    # go.mod, so a placeholder or broken one makes it fail instead of fetching latest
    go mod edit -droprequire="${PROTO_MODULE}" 2>/dev/null || true
    if ! go get "${PROTO_MODULE}@${PROTO_VERSION}"; then
        echo ""
        log_error "解析 ${PROTO_MODULE}@${PROTO_VERSION} 失败"
        cat >&2 <<HINT

  本服务的 proto 契约来自协议中心，不在本仓库。上面的失败通常是下面两种情况之一：

  一、协议中心仓库还没创建
      用 kronos-share-proto-template 生成并推上去，再回来重跑本脚本：
        kopy new https://github.com/kamalyes/kronos-share-proto-template -o ../kronos-share-proto

  二、想先本地联调，暂不推远端
      打开 go.mod 末尾那条 replace（去掉行首的 //），指向同级目录的协议仓库，
      然后重跑：bash scripts/init.sh

  只想先看工程骨架、暂时不接协议中心：bash scripts/init.sh --skip-proto
  （此时 models/base.go 会编译不过，因为它用了协议中心的 pb/common）
HINT
        exit 1
    fi
    log_success "协议中心依赖已写入 go.mod"
fi
echo ""

# ── 2/5 依赖整理 ────────────────────────────────────────────────────────────
log_info "[2/5] go mod tidy"
go mod tidy
log_success "依赖整理完成"
echo ""

# ── 3/5 格式化 ──────────────────────────────────────────────────────────────
# 为什么模板需要这一步，而普通 Go 工程不需要：
#
# 模板里的标识符是占位符，渲染后长度会变，而 gofmt 的列对齐是按**实际**标识符长度
# 算的模板源码里写成对齐的那几处（models/table_names.go 的 var 块、
# repository/factory.go 的 struct 字段与构造函数、bootstrap/pbmo.go 的 import 等），
# 渲染成 Product / Warehouse / Tenant 这些长度不同的名字之后列就错开了
# 这不是模板写错了，而是「对齐」这件事在渲染前根本算不出来——同一份模板按不同变量
# 渲染，正确的对齐结果就是不同的
# 同理，import 块的排序取决于 OrgName / ProjectName 渲染成什么，模板里也定不下来
#
# 不做这一步的后果：每个新生成的项目开箱就过不了 gofmt -l，而 .golangci.yml 里
# gofmt 与 goimports 两个 linter 都是开的，第一次推送 CI 就是红的
#
# 放在 tidy 之后：tidy 会改 go.mod/go.sum，但更要紧的是放在 build 之前——
# 编译的必须是最终形态，否则「能编译」这个结论是对格式化前的代码说的
#
# 用 go fmt ./... 而不是 gofmt -w .：go fmt 由 go 命令自己调起 gofmt，不必去猜
# gofmt 二进制在不在 PATH 上（Windows 上常见只有 go 在 PATH）；它同时带 -l，
# 会把实际改写过的文件列出来，于是这一步做了什么是看得见的
#
# [EN] A template needs this step where an ordinary Go project does not. Identifiers here are
# placeholders whose length changes on render, while gofmt computes column alignment from the
# *actual* identifier lengths, so the places written aligned in the template source come out
# misaligned once the names become Product, Warehouse, Tenant and so on. That is not a mistake
# in the template: alignment simply cannot be computed before rendering, because the correct
# result differs per variable set, and the same holds for import ordering, which depends on what
# OrgName and ProjectName render to. Without this step every generated project fails gofmt -l
# out of the box, and .golangci.yml enables both the gofmt and goimports linters, so the first
# push is red. It runs after tidy and, more importantly, before build, so that "it compiles" is
# a statement about the final form of the code. go fmt ./... is used rather than gofmt -w .
# because the go command locates gofmt itself, which matters where only go is on PATH, and
# because it passes -l and therefore shows which files it actually rewrote.
if [[ "$SKIP_FMT" == 0 ]]; then
    log_info "[3/5] go fmt ./..."
    FMT_CHANGED="$(go fmt ./...)"
    if [[ -n "$FMT_CHANGED" ]]; then
        echo "$FMT_CHANGED" | sed 's/^/    /'
        log_success "格式化完成（上列文件已重写）"
    else
        log_success "格式化完成（无需改动）"
    fi
else
    log_warn "[3/5] 按 --skip-fmt 跳过格式化"
fi
echo ""

# ── 4/5 编译 ────────────────────────────────────────────────────────────────
log_info "[4/5] go build ./..."
go build ./...
log_success "编译通过"
echo ""

# ── 5/5 自检 ────────────────────────────────────────────────────────────────
if [[ "$SKIP_TEST" == 0 ]]; then
    log_info "[5/5] go test ./..."
    go test ./...
    log_success "测试通过"
else
    log_warn "[5/5] 按 --skip-test 跳过测试"
fi

echo ""
echo "========================================"
log_success "初始化完成"
echo "========================================"
echo ""
echo "下一步："
echo "  go run .                               # 启动服务（APP_ENV 未设置时读 resources/kronos-my-service-dev.yaml）"
echo "  git add -A && git commit -m \"chore: bootstrap my-service\""
echo ""
echo "接入 proto 定义的 service：在 bootstrap/services.go 的两个 TODO 处注册 gRPC 与 HTTP handler。"
echo "参数校验不需要自己写：go-rpc-gateway 已把基于 go-argus 的 struct tag 校验拦截器挂在链上，"
echo "proto 里 @inject_tag: validate:\"...\" 注入的规则自动生效。"
