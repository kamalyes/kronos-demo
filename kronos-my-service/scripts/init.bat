@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul

rem 一键打通：解析协议中心依赖 -> go mod tidy -> 格式化 -> build -> test
rem
rem go.mod 里协议中心的版本是占位符，直接 go mod tidy 解析不到：
rem 协议契约不在本仓库，而在协议中心（kronos-share-proto-template 生成的仓库）。
rem 协议中心还不存在时脚本会说清出路，而不是抛一串 Go 的 Repository not found。

set "PROTO_MODULE=github.com/kamalyes/kronos-share-proto-template"
set "PROTO_VERSION=build"
set "SKIP_TEST=false"
set "SKIP_PROTO=false"
set "SKIP_FMT=false"

:parse_args
if "%~1"=="" goto after_parse_args
if /i "%~1"=="--skip-test" set "SKIP_TEST=true" && shift && goto parse_args
if /i "%~1"=="--skip-proto" set "SKIP_PROTO=true" && shift && goto parse_args
if /i "%~1"=="--skip-fmt" set "SKIP_FMT=true" && shift && goto parse_args
if /i "%~1"=="--help" goto show_help
if /i "%~1"=="-h" goto show_help
echo ❌ 未知选项: %~1
exit /b 1

:show_help
echo 用法: scripts\init.bat [--skip-proto] [--skip-fmt] [--skip-test]
echo   --skip-proto  跳过协议中心依赖解析
echo   --skip-fmt    跳过 go fmt（不建议：生成产物开箱就过不了 gofmt 检查）
echo   --skip-test   跳过 go test 自检
exit /b 0

:after_parse_args

set "SCRIPT_DIR=%~dp0"
for %%i in ("%SCRIPT_DIR%..") do set "PROJECT_ROOT=%%~fi"
cd /d "%PROJECT_ROOT%"

echo ========================================
echo 🚀 初始化 MyService
echo ========================================
echo.

where go >nul 2>&1
if errorlevel 1 (
    echo ❌ Go 未安装，请先安装 Go 1.25 或更高版本
    exit /b 1
)
for /f "tokens=3" %%v in ('go version') do echo ✅ Go: %%v
echo.

if "%SKIP_PROTO%"=="true" (
    echo ⚠️  [1/5] 按 --skip-proto 跳过协议中心依赖解析
) else (
    findstr /r /c:"^replace[ 	]*%PROTO_MODULE%" go.mod >nul 2>&1
    if not errorlevel 1 (
        echo ⚠️  [1/5] go.mod 里已打开本地 replace，跳过远端版本解析
    ) else (
        echo ℹ️  [1/5] 解析协议中心依赖 %PROTO_MODULE%@%PROTO_VERSION%
        go get %PROTO_MODULE%@%PROTO_VERSION%
        if errorlevel 1 (
            echo.
            echo ❌ 解析 %PROTO_MODULE%@%PROTO_VERSION% 失败
            echo.
            echo   本服务的 proto 契约来自协议中心，不在本仓库。通常是下面两种情况：
            echo.
            echo   一、协议中心仓库还没创建
            echo       用 kronos-share-proto-template 生成并推上去，再回来重跑：
            echo         kopy new https://github.com/kamalyes/kronos-share-proto-template -o ..\kronos-share-proto
            echo.
            echo   二、想先本地联调，暂不推远端
            echo       打开 go.mod 末尾那条 replace（去掉行首的 //），指向同级目录的协议仓库，
            echo       然后重跑：scripts\init.bat
            echo.
            echo   只想先看工程骨架：scripts\init.bat --skip-proto
            exit /b 1
        )
        echo ✅ 协议中心依赖已写入 go.mod
    )
)
echo.

echo ℹ️  [2/5] go mod tidy
go mod tidy
if errorlevel 1 (
    echo ❌ go mod tidy 失败
    exit /b 1
)
echo ✅ 依赖整理完成
echo.

rem 为什么模板需要格式化这一步，而普通 Go 工程不需要：
rem 模板里的标识符是占位符，渲染后长度会变，而 gofmt 的列对齐按实际标识符长度算，
rem 所以模板源码里对齐的那几处渲染成实际名字后列就错开了——「对齐」在渲染前算不出来
rem import 块的排序同理，取决于 OrgName / ProjectName 渲染成什么
rem 不做这一步，每个新生成的项目开箱就过不了 gofmt -l，而 .golangci.yml 里
rem gofmt 与 goimports 两个 linter 都是开的，第一次推送 CI 就是红的
rem 放在 build 之前：编译的必须是最终形态
if "%SKIP_FMT%"=="true" (
    echo ⚠️  [3/5] 按 --skip-fmt 跳过格式化
) else (
    echo ℹ️  [3/5] go fmt ./...
    go fmt ./...
    if errorlevel 1 (
        echo ❌ 格式化失败
        exit /b 1
    )
    echo ✅ 格式化完成
)
echo.

echo ℹ️  [4/5] go build ./...
go build ./...
if errorlevel 1 (
    echo ❌ 编译失败
    exit /b 1
)
echo ✅ 编译通过
echo.

if "%SKIP_TEST%"=="true" (
    echo ⚠️  [5/5] 按 --skip-test 跳过测试
) else (
    echo ℹ️  [5/5] go test ./...
    go test ./...
    if errorlevel 1 (
        echo ❌ 测试失败
        exit /b 1
    )
    echo ✅ 测试通过
)

echo.
echo ========================================
echo ✅ 初始化完成
echo ========================================
echo.
echo 下一步：
echo   go run .                               启动服务（读 resources\kronos-my-service-dev.yaml）
echo   git add -A ^&^& git commit -m "chore: bootstrap my-service"
echo.
echo 接入 proto 定义的 service：在 bootstrap\services.go 的两个 TODO 处注册 gRPC 与 HTTP handler。
echo 参数校验不需要自己写：go-rpc-gateway 已把基于 go-argus 的 struct tag 校验拦截器挂在链上。
exit /b 0
