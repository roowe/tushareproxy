#!/usr/bin/env bash
#
# 发布脚本：交叉编译出 Ubuntu 24 (linux) 可执行文件，
# 并组装一个自包含的发布目录（二进制 + 配置 + 启动脚本）。手动启动，不装 systemd。
#
# 用法：
#   ./scripts/release.sh              # 默认 linux/amd64
#   ./scripts/release.sh arm64        # linux/arm64
#
set -euo pipefail

# --- 定位项目根目录 ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

# --- 参数 ---
ARCH="${1:-amd64}"                       # amd64 | arm64
APP_NAME="tushareproxy"

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

PKG_NAME="${APP_NAME}-${VERSION}-linux-${ARCH}"
OUT_ROOT="${ROOT_DIR}/release"
OUT_DIR="${OUT_ROOT}/${PKG_NAME}"

echo "==> 发布 ${APP_NAME}"
echo "    version : ${VERSION}"
echo "    target  : linux/${ARCH}"
echo "    outdir  : ${OUT_DIR}"

# --- 清理并创建发布目录 ---
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR" "$OUT_DIR/data" "$OUT_DIR/logs"

# --- 交叉编译 ---
echo "==> 编译中..."
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
  go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
  -o "$OUT_DIR/${APP_NAME}" .

# --- 配置文件 ---
echo "==> 生成配置文件 proxy.toml"
cp "${ROOT_DIR}/proxy.toml.example" "$OUT_DIR/proxy.toml"

# --- 启动脚本（手动启动，后台运行）---
echo "==> 生成 start.sh"
cat > "$OUT_DIR/start.sh" <<EOF
#!/usr/bin/env bash
#
# 手动启动 ${APP_NAME}（后台运行，日志写到 logs/proxy.log）。
#
set -euo pipefail

APP_NAME="${APP_NAME}"
EOF
cat >> "$OUT_DIR/start.sh" <<'EOF'
cd "$(dirname "${BASH_SOURCE[0]}")"

nohup "./$APP_NAME" ./proxy.toml >> logs/run.out 2>&1 &
echo $! > "$APP_NAME.pid"
echo "==> 已启动，PID=$(cat "$APP_NAME.pid")"
echo "    日志： tail -f logs/run.out  或  logs/proxy.log"
echo "    停止： kill \$(cat $APP_NAME.pid)"
EOF
chmod +x "$OUT_DIR/start.sh"

# --- 部署说明 ---
echo "==> 生成 README"
cat > "$OUT_DIR/README.md" <<EOF
# ${PKG_NAME}

tushareproxy 发布包（目标环境：Ubuntu 24 / linux-${ARCH}）。

- version: \`${VERSION}\`
- build:   \`${BUILD_TIME}\`

## 内容

- \`${APP_NAME}\`     可执行文件
- \`proxy.toml\`    配置文件（先按需修改）
- \`start.sh\`      手动启动脚本（后台运行）
- \`data/\` \`logs/\` 运行时数据与日志目录

## 部署步骤

1. 上传并解压：

   \`\`\`bash
   scp ${PKG_NAME}.tar.gz user@server:/tmp/
   ssh user@server 'cd /tmp && tar -xzf ${PKG_NAME}.tar.gz'
   \`\`\`

2. 按需修改 \`proxy.toml\`，然后启动：

   \`\`\`bash
   cd /tmp/${PKG_NAME}
   ./start.sh
   \`\`\`

   也可直接前台运行： \`./${APP_NAME} ./proxy.toml\`

3. 停止：

   \`\`\`bash
   kill \$(cat ${APP_NAME}.pid)
   \`\`\`
EOF

# --- 打包 ---
echo "==> 打包 tar.gz"
tar -czf "${OUT_ROOT}/${PKG_NAME}.tar.gz" -C "$OUT_ROOT" "$PKG_NAME"

echo ""
echo "==> 发布完成"
echo "    目录: ${OUT_DIR}"
echo "    压缩包: ${OUT_ROOT}/${PKG_NAME}.tar.gz"
