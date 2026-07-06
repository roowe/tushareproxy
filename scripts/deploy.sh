#!/usr/bin/env bash
#
# 部署脚本：先本地打包（调用 release.sh），再把发布包传到指定主机并解压安装。
# 目标环境 Ubuntu 24。默认不覆盖远端已存在的 proxy.toml，也不自动启动。
#
# 用法：
#   ./scripts/deploy.sh user@host
#   ./scripts/deploy.sh user@host arm64
#
# 可用环境变量：
#   REMOTE_DIR   远端安装目录（默认 ~/tushareproxy）
#   SSH_PORT     ssh 端口（默认 22）
#   START=1      部署完成后自动执行远端 start.sh
#   SKIP_BUILD=1 跳过本地打包，直接用 release/ 里已有的包
#
set -euo pipefail

# --- 参数 ---
HOST="${1:-}"
ARCH="${2:-amd64}"
if [[ -z "$HOST" ]]; then
  echo "用法: ./scripts/deploy.sh user@host [arch]"
  echo "例如: ./scripts/deploy.sh ubuntu@1.2.3.4"
  exit 1
fi

REMOTE_DIR="${REMOTE_DIR:-tushareproxy}"   # 相对 => 远端 HOME 下；也可写绝对路径
SSH_PORT="${SSH_PORT:-22}"

# --- 定位项目根目录 ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME="tushareproxy"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
PKG_NAME="${APP_NAME}-${VERSION}-linux-${ARCH}"
TARBALL="${ROOT_DIR}/release/${PKG_NAME}.tar.gz"

# --- 本地打包 ---
if [[ "${SKIP_BUILD:-}" != "1" ]]; then
  echo "==> 本地打包"
  "$SCRIPT_DIR/release.sh" "$ARCH"
fi

if [[ ! -f "$TARBALL" ]]; then
  echo "找不到发布包: $TARBALL"
  echo "请先运行 ./scripts/release.sh $ARCH，或去掉 SKIP_BUILD。"
  exit 1
fi

SSH="ssh -p ${SSH_PORT}"
SCP="scp -P ${SSH_PORT}"

echo "==> 上传到 ${HOST}:/tmp/${PKG_NAME}.tar.gz"
$SCP "$TARBALL" "${HOST}:/tmp/${PKG_NAME}.tar.gz"

echo "==> 在远端解压安装到 ${REMOTE_DIR}"
$SSH "$HOST" \
  APP_NAME="$APP_NAME" \
  PKG_NAME="$PKG_NAME" \
  REMOTE_DIR="$REMOTE_DIR" \
  START="${START:-}" \
  'bash -s' <<'REMOTE'
set -euo pipefail

TMP="/tmp/${PKG_NAME}"
rm -rf "$TMP"
mkdir -p "$TMP"
tar -xzf "/tmp/${PKG_NAME}.tar.gz" -C /tmp

mkdir -p "$REMOTE_DIR" "$REMOTE_DIR/data" "$REMOTE_DIR/logs"

# 二进制与脚本：直接覆盖
install -m 0755 "$TMP/$APP_NAME" "$REMOTE_DIR/$APP_NAME"
install -m 0755 "$TMP/start.sh" "$REMOTE_DIR/start.sh"
cp "$TMP/README.md" "$REMOTE_DIR/README.md" 2>/dev/null || true

# 配置：已存在则保留旧的，新配置写为 proxy.toml.new
if [[ -f "$REMOTE_DIR/proxy.toml" ]]; then
  echo "    已存在 proxy.toml，保留旧配置，新配置写为 proxy.toml.new"
  cp "$TMP/proxy.toml" "$REMOTE_DIR/proxy.toml.new"
else
  cp "$TMP/proxy.toml" "$REMOTE_DIR/proxy.toml"
fi

rm -rf "$TMP" "/tmp/${PKG_NAME}.tar.gz"

echo "    已部署到 $(cd "$REMOTE_DIR" && pwd)"

if [[ "$START" == "1" ]]; then
  echo "==> 远端启动"
  # 若已在运行则先停
  if [[ -f "$REMOTE_DIR/$APP_NAME.pid" ]] && kill -0 "$(cat "$REMOTE_DIR/$APP_NAME.pid")" 2>/dev/null; then
    kill "$(cat "$REMOTE_DIR/$APP_NAME.pid")"
    sleep 1
  fi
  ( cd "$REMOTE_DIR" && ./start.sh )
fi
REMOTE

echo ""
echo "==> 部署完成 -> ${HOST}:${REMOTE_DIR}"
if [[ "${START:-}" != "1" ]]; then
  echo "    远端启动： ssh ${HOST} 'cd ${REMOTE_DIR} && ./start.sh'"
fi
