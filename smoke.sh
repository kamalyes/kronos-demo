#!/usr/bin/env bash
# 冒烟：起 access-control + gateway，验网关靠 gRPC reflection 发现下游并建出 HTTP 路由
#
# 只起这两个，因为它们不需要任何基础设施 key-service 要 Redis、my-service 还要 MySQL，
# 两者都是刻意不降级的（见 README）
#
# 先 go build 再执行产出的二进制，而不是 go run . ——
# go run 会 fork 一个子进程去跑编译结果，脚本拿到的是 go run 自己的 pid，
# 收尾时杀掉它并不会杀掉真正在监听端口的那个子进程 现象是「下一次运行探活
# 1 秒就通过、uptime 却是几百秒」，读到的是上一轮的残留进程，而本轮因端口被占
# 根本没起来 build 之后 pid 就是服务本身，trap 才真的收得掉
#
# 探活打的是 HTTP 端口而不是配置里的 health.port —— 后者没有被监听，/health 挂在 HTTP 端口上
set -uo pipefail

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/kronos-demo-smoke"
mkdir -p "$WORK_DIR/bin"

PIDS=()
cleanup() {
  for p in "${PIDS[@]:-}"; do [ -n "${p:-}" ] && kill "$p" 2>/dev/null; done
  wait 2>/dev/null
}
trap cleanup EXIT

# 端口预检：被占着就直接停，否则探活会打到残留进程上，报一个假的成功
for port in 8390 9390 8490 9490; do
  holder=$(lsof -nP -iTCP:$port -sTCP:LISTEN 2>/dev/null | awk 'NR==2{print $2" ("$1")"}')
  [ -z "$holder" ] && continue
  echo "❌ 端口 $port 已被占用: pid $holder"
  echo "   先收掉它再跑，否则探活会读到它的响应而不是本轮启动的服务:"
  echo "   lsof -nP -iTCP:8390 -iTCP:9390 -iTCP:8490 -iTCP:9490 -sTCP:LISTEN"
  exit 1
done

start() { # 产物目录 HTTP端口
  local dir=$1 port=$2
  go build -o "$WORK_DIR/bin/$dir" "$DEMO_DIR/$dir" || { echo "  ❌ $dir 编译失败"; return 1; }
  ( cd "$DEMO_DIR/$dir" && exec "$WORK_DIR/bin/$dir" ) >"$WORK_DIR/$dir.log" 2>&1 &
  PIDS+=($!)
  for i in $(seq 1 60); do
    curl -fsS -m 2 "http://127.0.0.1:$port/health" >/dev/null 2>&1 && { echo "  ✅ $dir 健康检查通过 (${i}s)"; return 0; }
    kill -0 "${PIDS[-1]}" 2>/dev/null || { echo "  ❌ $dir 进程已退出"; tail -15 "$WORK_DIR/$dir.log"; return 1; }
    sleep 1
  done
  echo "  ❌ $dir 60s 内没起来，日志: $WORK_DIR/$dir.log"
  tail -20 "$WORK_DIR/$dir.log"
  return 1
}

echo "[1] kronos-access-control-service  (http 8390 / grpc 9390)"
start kronos-access-control-service 8390 || exit 1

echo "[2] kronos-gateway-service         (http 8490 / grpc 9490)"
start kronos-gateway-service 8490 || exit 1

echo
echo "[3] 网关 /health"
curl -s -m 3 http://127.0.0.1:8490/health; echo

echo
echo "[4] 下游发现结果（未启动的两个应当如实报 Unavailable，网关不该因此挂掉）"
grep -hoE '✅ kronos-[a-z-]+ 连接已建立 -> [0-9.:]+|reflection 发现 [0-9]+ 个服务: \[[^]]*\]|reflection: 发现服务 kronos-[a-z-]+ 失败|AutoRegister: [0-9]+/[0-9]+ clients, [0-9]+/[0-9]+ handlers initialized' \
  "$WORK_DIR/kronos-gateway-service.log" | sed 's/^/  /'

echo
echo "完整日志: $WORK_DIR/"
