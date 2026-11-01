#!/usr/bin/env bash
# 重建 demo：按全默认变量重新渲染四个模板，跑各自的 init.sh，补上两个模板缺的 locales
#
# 只动下面 SERVICES 里列出的四个目录 本目录下的其它内容一概不碰
# —— 包括 kronos-share-proto/，那不是这个脚本生成的
set -euo pipefail

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE="$(cd "$DEMO_DIR/.." && pwd)"

# <产物目录>:<模板目录>
SERVICES=(
  "kronos-access-control-service:kronos-access-control-template"
  "kronos-key-service:kronos-key-service-template"
  "kronos-my-service:kronos-service-template"
  "kronos-gateway-service:kronos-gateway-template"
)

# 这两个模板不带 locales/，却在配置里开着 i18n —— 渲染产物起不来，要在这里补
NEEDS_LOCALES=(kronos-access-control-service kronos-key-service)

command -v kopy >/dev/null || { echo "❌ 没装 kopy: go install github.com/kamalyes/kopy@latest"; exit 1; }
command -v go   >/dev/null || { echo "❌ 没装 Go"; exit 1; }

for entry in "${SERVICES[@]}"; do
  out="${entry%%:*}"; tpl="${entry##*:}"
  [ -d "$WORKSPACE/$tpl" ] || { echo "❌ 模板不在: $WORKSPACE/$tpl"; exit 1; }

  echo "=============================================="
  echo "🎨 $tpl → $out"
  echo "=============================================="
  rm -rf "$DEMO_DIR/$out"
  kopy new "$WORKSPACE/$tpl" -o "$DEMO_DIR/$out" -y --hooks never >/dev/null

  # 和模板 CI 同一道闸门：kopy 对未定义的变量渲染成 <no value> 且不报错
  if grep -rIl -e '<no value>' -e '<%' "$DEMO_DIR/$out" 2>/dev/null | head -1 | grep -q .; then
    echo "❌ 渲染产物里有未解析的占位符"
    grep -rIn -e '<no value>' -e '<%' "$DEMO_DIR/$out" | head -10
    exit 1
  fi

  for n in "${NEEDS_LOCALES[@]}"; do
    [ "$out" = "$n" ] || continue
    mkdir -p "$DEMO_DIR/$out/locales"
    cat > "$DEMO_DIR/$out/locales/en.json" <<'JSON'
{
  "welcome": "Welcome",
  "success": "Success",
  "hello": "Hello",
  "goodbye": "Goodbye"
}
JSON
    cat > "$DEMO_DIR/$out/locales/zh.json" <<'JSON'
{
  "welcome": "欢迎",
  "success": "成功",
  "hello": "你好",
  "goodbye": "再见"
}
JSON
    echo "🩹 补了 locales/{en,zh}.json —— 模板缺这个目录，不补起不来"
  done

  ( cd "$DEMO_DIR/$out" && bash scripts/init.sh ) | tail -4
  echo
done

echo "✅ 四个服务都重建完了，看 README.md 的端口表与启动约束"
