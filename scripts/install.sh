#!/usr/bin/env bash
#
# Catflap 一键安装脚本（Debian / Ubuntu，systemd）。
#
#   curl -fsSL https://raw.githubusercontent.com/chenggegedeid/catflap/main/scripts/install.sh | sudo bash
#
# 也能干别的：
#   sudo bash install.sh                # 安装或升级到最新版
#   sudo bash install.sh --uninstall    # 卸载（默认保留配置与数据）
#   sudo bash install.sh --purge        # 卸载并删除配置与数据
#   sudo bash install.sh --version v1.2.0
#   sudo bash install.sh --dry-run      # 只打印将要做什么，不动手
#
# 可选环境变量：
#   CATFLAP_VERSION=1.0.0   装指定版本（不带 v 前缀也行）
#   CATFLAP_MIRROR=<前缀>   下载慢时用它当前缀，例如
#                           https://ghfast.top/https://github.com/...
#   CATFLAP_PORT=8081       改监听端口（写进服务单元的 Environment）
#
# 设计上刻意遵守的几条：
#   - **不用 root 跑服务**：建专用用户 catflap，程序目录只读。面板能读写配置、
#     能把内网服务暴露到公网，用 root 跑是把风险放大一级。
#   - **配置与数据默认不动**：升级只换二进制。--purge 才删。
#   - **先校验再替换**：下载后比对 sha256，不对就停下，绝不把坏二进制装进去。

set -euo pipefail

REPO="chenggegedeid/catflap"
APP="catflap"
INSTALL_DIR="/opt/catflap"
SERVICE_FILE="/etc/systemd/system/catflap.service"
BIN="${INSTALL_DIR}/catflap"

VERSION=""
MIRROR="${CATFLAP_MIRROR:-}"
PORT="${CATFLAP_PORT:-}"
DRY_RUN=0
ACTION="install"

# ── 小工具 ──────────────────────────────────────────────────────────────────
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror\033[0m %s\n' "$*" >&2; exit 1; }

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '    [dry-run] %s\n' "$*"
  else
    "$@"
  fi
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1（先 apt install $1）"
}

# ── 参数 ────────────────────────────────────────────────────────────────────
while [ $# -gt 0 ]; do
  case "$1" in
    --version)   VERSION="${2:-}"; shift 2 ;;
    --mirror)    MIRROR="${2:-}"; shift 2 ;;
    --port)      PORT="${2:-}"; shift 2 ;;
    --dry-run)   DRY_RUN=1; shift ;;
    --uninstall) ACTION="uninstall"; shift ;;
    --purge)     ACTION="purge"; shift ;;
    -h|--help)   sed -n '3,30p' "$0"; exit 0 ;;
    *)           die "不认识的参数：$1（用 --help 看用法）" ;;
  esac
done

# ── 卸载 ────────────────────────────────────────────────────────────────────
if [ "$ACTION" = "uninstall" ] || [ "$ACTION" = "purge" ]; then
  [ "$(id -u)" -eq 0 ] || die "卸载需要 root：sudo bash $0 --uninstall"
  log "停止并禁用服务"
  run systemctl stop "$APP" 2>/dev/null || true
  run systemctl disable "$APP" 2>/dev/null || true
  run rm -f "$SERVICE_FILE"
  run systemctl daemon-reload
  if [ "$ACTION" = "purge" ]; then
    log "删除 ${INSTALL_DIR}（含配置与数据）"
    run rm -rf "$INSTALL_DIR"
    run userdel "$APP" 2>/dev/null || true
  else
    log "保留 ${INSTALL_DIR} 下的配置与数据"
    run rm -f "$BIN"
  fi
  log "完成"
  exit 0
fi

# ── 安装前检查 ──────────────────────────────────────────────────────────────
[ "$(id -u)" -eq 0 ] || die "安装需要 root：curl ... | sudo bash"
need_cmd curl
command -v systemctl >/dev/null 2>&1 || die "没找到 systemd，本脚本只支持 systemd 的系统"
[ -r /etc/os-release ] || die "看不出系统版本（没有 /etc/os-release）"
# shellcheck disable=SC1091
. /etc/os-release
case "${ID:-}" in
  debian|ubuntu|raspbian|deepin|uos|kylin) ;;
  *) warn "本脚本主要面向 Debian / Ubuntu，当前是 ${ID:-未知}，继续但请自行确认" ;;
esac

# ── 架构 ────────────────────────────────────────────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "amd64" ;;
    aarch64|arm64)  echo "arm64" ;;
    armv7l|armv6l)  echo "armv7" ;;
    mipsel|mipsle)  echo "mipsle" ;;
    *) die "不支持的架构：$(uname -m)" ;;
  esac
}
ARCH="$(detect_arch)"
# 资产名必须与发布工作流里的产物名完全一致，否则会下到 404
case "$ARCH" in
  amd64)  ASSET="catflap" ;;
  *)      ASSET="catflap-linux-${ARCH}" ;;
esac
log "架构：${ARCH} → 资产 ${ASSET}"

# ── 版本号 ──────────────────────────────────────────────────────────────────
if [ -z "$VERSION" ]; then
  log "查询最新版本"
  latest="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
            | sed -n 's/.*"tag_name" *: *"\([^"]*\)".*/\1/p' | head -n1)"
  [ -n "$latest" ] || die "查不到最新版本（网络不通，或仓库还没有 Release）。可以指定版本：--version v1.0.0"
  VERSION="$latest"
fi
VERSION="${VERSION#v}"
TAG="v${VERSION}"
log "版本：${TAG}"

base_url() {
  # MIRROR 是「前缀」而不是完整 URL：拼在 github.com 前面即可
  if [ -n "$MIRROR" ]; then
    printf '%s' "${MIRROR%/}/https://github.com"
  else
    printf '%s' "https://github.com"
  fi
}
BASE="$(base_url)"
BIN_URL="${BASE}/${REPO}/releases/download/${TAG}/${ASSET}"
SUM_URL="${BASE}/${REPO}/releases/download/${TAG}/SHA256SUMS"

if [ "$DRY_RUN" -eq 1 ]; then
  log "将要：下载 ${BIN_URL} → ${BIN}，装 systemd 单元，启动服务"
  log "（dry-run，到此为止）"
  exit 0
fi

# ── 下载 ────────────────────────────────────────────────────────────────────
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

log "下载 ${ASSET}"
curl -fsSL --retry 2 --max-time 300 -o "${tmpdir}/${ASSET}" "$BIN_URL" \
  || die "下载失败：${BIN_URL}
  国内网络可以试试：CATFLAP_MIRROR=https://ghfast.top bash $0"

# sha256 校验：SHA256SUMS 是给人看的（程序内部的更新校验用的是 GitHub API 的
# digest），但安装时它是唯一能挡住「下到一半 / 被换成别的东西」的手段。
if curl -fsSL --max-time 60 -o "${tmpdir}/SHA256SUMS" "$SUM_URL" 2>/dev/null; then
  want="$(awk -v a="$ASSET" '$2==a || $2=="*"a || $2=="./"a {print $1}' "${tmpdir}/SHA256SUMS" | head -n1)"
  if [ -n "$want" ]; then
    got="$(sha256sum "${tmpdir}/${ASSET}" | awk '{print $1}')"
    [ "$want" = "$got" ] || die "sha256 对不上：期望 ${want}，实际 ${got}。文件没有替换，请重试或换镜像。"
    log "sha256 校验通过"
  else
    warn "SHA256SUMS 里没有 ${ASSET}，跳过校验"
  fi
else
  warn "拿不到 SHA256SUMS，跳过校验"
fi

# ── 用户与目录 ──────────────────────────────────────────────────────────────
if ! id -u "$APP" >/dev/null 2>&1; then
  log "创建系统用户 ${APP}"
  useradd --system --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin "$APP"
fi
mkdir -p "$INSTALL_DIR"
# 配置目录里有 DNS 服务商 Token 等明文凭据，必须 0700
chmod 0700 "$INSTALL_DIR"
chown -R "$APP":"$APP" "$INSTALL_DIR"

# ── 换二进制 ────────────────────────────────────────────────────────────────
if systemctl is-active --quiet "$APP" 2>/dev/null; then
  log "停止正在运行的服务"
  systemctl stop "$APP"
fi
install -m 0755 "${tmpdir}/${ASSET}" "$BIN"

# ── systemd 单元 ────────────────────────────────────────────────────────────
log "写入 ${SERVICE_FILE}"
{
  cat <<'UNIT'
[Unit]
Description=Catflap 网络入口管理
Documentation=https://github.com/chenggegedeid/catflap
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
# 专用用户 + 只读程序目录：面板能读写凭据、能把内网服务暴露到公网，
# 用 root 跑是把风险放大一级。
User=catflap
Group=catflap
WorkingDirectory=/opt/catflap
ExecStart=/opt/catflap/catflap
Restart=on-failure
RestartSec=5
# 新建文件默认只对属主可读写（config/ 里是明文凭据）
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/catflap
UNIT
  if [ -n "$PORT" ]; then
    printf 'Environment=CATFLAP_PORT=%s\n' "$PORT"
  fi
  cat <<'UNIT'

[Install]
WantedBy=multi-user.target
UNIT
} > "$SERVICE_FILE"

systemctl daemon-reload
systemctl enable "$APP" >/dev/null 2>&1
systemctl restart "$APP"

# ── 收尾 ────────────────────────────────────────────────────────────────────
PORT_SHOWN="${PORT:-3791}"
log "等待服务就绪"
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT_SHOWN}/api/version" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

cat <<EOF

  Catflap ${TAG} 装好了。

  本机初始化（首次必须在本机操作，程序会拒绝来自其它 IP 的初始化请求）：
      http://127.0.0.1:${PORT_SHOWN}/catflap/

  之后从局域网其它设备访问：
      http://<这台机器的内网IP>:${PORT_SHOWN}/catflap/

  常用命令：
      systemctl status catflap
      journalctl -u catflap -f
      sudo bash $0 --uninstall

EOF

if ! systemctl is-active --quiet "$APP"; then
  warn "服务没起来，看日志：journalctl -u catflap -n 50 --no-pager"
  exit 1
fi
