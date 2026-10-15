#!/usr/bin/env bash
#
# Catflap 一键安装脚本（Debian / Ubuntu，systemd）。
#
#   curl -fsSL https://raw.githubusercontent.com/chenggegedeid/catflap/main/scripts/install.sh | sudo bash
#
# 直接下载下来再跑，会进入**交互式菜单**（可选版本、安装目录、运行账号、端口）：
#
#   curl -fsSL .../install.sh -o install.sh && sudo bash install.sh
#
# 也能干别的：
#   sudo bash install.sh                # 安装或升级到最新版
#   sudo bash install.sh --uninstall    # 卸载（默认保留配置与数据）
#   sudo bash install.sh --purge        # 卸载并删除配置与数据
#   sudo bash install.sh --version v1.2.0
#   sudo bash install.sh --dry-run      # 只打印将要做什么，不动手
#   sudo bash install.sh --run-as-root  # 让服务进程用 root 跑（需要绑 80/443 时）
#
# 可选环境变量：
#   CATFLAP_VERSION=1.0.0   装指定版本（不带 v 前缀也行）
#   CATFLAP_MIRROR=<前缀>   下载慢时用它当前缀，例如
#                           https://ghfast.top/https://github.com/...
#   CATFLAP_PORT=8081       改监听端口（写进服务单元的 Environment）
#   CATFLAP_RUN_AS=root     同上，让服务进程用 root 跑
#
# 关于「服务用什么账号跑」：
#   默认建专用用户 catflap，不用 root —— 这个程序握着 DNS 服务商 Token、
#   能把内网服务暴露到公网，用 root 跑的话任何一个漏洞都等于整机 root。
#   **但这是默认值不是限制**：要绑 80/443、要动 iptables，就加 --run-as-root
#   （那时候你清楚自己在放弃什么）。真正需要特权端口但不想给 root 的话，
#   还有第三条路：保持非 root，在服务单元里放开 CAP_NET_BIND_SERVICE。
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
# 服务进程的运行账号。默认专用用户 catflap；给 root 则用 root 跑（见 --help 的说明）。
RUN_AS="${CATFLAP_RUN_AS:-catflap}"
DRY_RUN=0
ACTION="install"
# 有没有显式给过参数。给了就完全按参数走（适合自动化），一个都没给且终端可用
# 时才进交互菜单。
ARGS_GIVEN=0

# 重跑自己时该写什么命令。直接写 $0 在 `curl ... | sudo bash` 这种管道执行下会
# 变成尴尬的 "bash bash"（管道里 $0 就是 bash，前面再加一个 bash 就成了两个）。
# 管道执行时没法照着原样重放，那就给最稳的写法。
case "${0##*/}" in
  bash|sh|-bash|dash) SELF="bash ${APP}-install.sh" ;;
  *)                  SELF="bash $0" ;;
esac

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
    --version)   VERSION="${2:-}"; ARGS_GIVEN=1; shift 2 ;;
    --mirror)    MIRROR="${2:-}"; ARGS_GIVEN=1; shift 2 ;;
    --port)      PORT="${2:-}"; ARGS_GIVEN=1; shift 2 ;;
    --dry-run)   DRY_RUN=1; ARGS_GIVEN=1; shift ;;
    --uninstall) ACTION="uninstall"; ARGS_GIVEN=1; shift ;;
    --purge)     ACTION="purge"; ARGS_GIVEN=1; shift ;;
    --run-as-root) RUN_AS="root"; ARGS_GIVEN=1; shift ;;
    -h|--help)
      # 只打印文件开头那段连续的注释，并且去掉行首的 "# "。
      # 之前写的是 sed -n '3,30p'，注释块结束之后就把代码也打出来了（REPO=… 之类），
      # 行数一改还会错位。改成「遇到第一行不是注释就停」，跟着内容走不跟着行号走。
      awk 'NR>=3 { if ($0 !~ /^#/) exit; sub(/^# ?/, ""); print }' "$0"
      exit 0
      ;;
    *)           die "不认识的参数：$1（用 --help 看用法）" ;;
  esac
done

# ── 交互菜单 ────────────────────────────────────────────────────────────────
#
# 只在「没给任何参数」且「标准输入是终端」时才进菜单。
#
# 为什么要判断 -t 0：curl ... | sudo bash 这种写法里，stdin 被脚本自己占着，
# 菜单读不到东西（读到的可能是脚本剩下还没被 shell 读走的内容），会莫名其妙
# 地选错或者直接卡住。给过任何参数也一律不进菜单 —— 自动化场景要的是确定性。
#
# 菜单只负责把这几个变量问出来，后面走的还是原来那条安装路径，
# 不存在「菜单装的和命令装的不一样」这种情况。

ask() {
  # $1 提示，$2 默认值
  #
  # 提示语一律写 stderr：这个函数是用 $(ask ...) 取返回值的，
  # 提示语走 stdout 的话会和用户输入粘在一起被一起捕获 ——
  # 第一版就是这个 bug，菜单里选什么都被判成「没有这个选项」。
  printf '  %s' "$1" >&2
  [ -n "${2:-}" ] && printf '（默认 %s）' "$2" >&2
  printf ' > ' >&2
  read -r ans || ans=""
  ans="$(printf '%s' "$ans" | tr -d '[:space:]')"
  if [ -z "$ans" ]; then printf '%s' "${2:-}"; else printf '%s' "$ans"; fi
}

menu() {
  echo
  echo "*************************************************"
  echo "**                 欢迎使用                    **"
  echo "**                Catflap 管理脚本             **"
  echo "*************************************************"
  echo
  echo "  1 安装 / 升级 Catflap"
  echo "  2 卸载（保留配置与数据）"
  echo "  3 完全卸载（连配置与数据一起删）"
  echo "  0 退出"
  echo
  choice="$(ask '请输入对应数字' '1')"
  case "$choice" in
    1) ACTION="install" ;;
    2) ACTION="uninstall" ;;
    3) ACTION="purge" ;;
    0) echo "已退出"; exit 0 ;;
    *) die "没有这个选项：$choice" ;;
  esac

  [ "$ACTION" = "install" ] || return 0

  echo
  echo "--- 版本 ---"
  echo "  直接回车 = 装最新版；也可以填指定版本，例如 1.0.0"
  VERSION="$(ask '版本号')"

  echo
  echo "--- 安装目录 ---"
  echo "  常用：/opt/catflap（默认）、/etc/catflap、/usr/share/catflap"
  INSTALL_DIR="$(ask '安装目录' '/opt/catflap')"
  BIN="${INSTALL_DIR}/catflap"

  echo
  echo "--- 服务用什么账号跑 ---"
  echo "  1 catflap（专用用户，默认，更安全）"
  echo "  2 root（需要用 80/443 或动 iptables 时选这个）"
  acct="$(ask '请选择' '1')"
  case "$acct" in
    1) RUN_AS="catflap" ;;
    2) RUN_AS="root" ;;
    *) die "没有这个选项：$acct" ;;
  esac

  echo
  echo "--- 监听端口 ---"
  PORT="$(ask '端口' '3791')"

  echo
  printf '  即将：安装到 %s，用 %s 跑，端口 %s\n' "$INSTALL_DIR" "$RUN_AS" "$PORT"
  confirm="$(ask '确认开始？[y/N]' 'y')"
  case "$confirm" in
    y|Y|yes|YES) ;;
    *) echo "已取消"; exit 0 ;;
  esac
}

if [ "$ARGS_GIVEN" -eq 0 ] && [ -t 0 ]; then
  menu
fi

# ── 卸载 ────────────────────────────────────────────────────────────────────
if [ "$ACTION" = "uninstall" ] || [ "$ACTION" = "purge" ]; then
  [ "$(id -u)" -eq 0 ] || die "卸载需要 root：sudo ${SELF} --uninstall"
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
# ⚠️ 必须放在子 shell 里读，不能直接 `. /etc/os-release`。
#
# 那个文件里有一行 VERSION="13 (trixie)"（Debian 自己的版本号），
# source 进来就会把本脚本的 VERSION 变量覆盖掉 —— 于是脚本拿 Debian 的版本号
# 去拼下载链接： .../releases/download/v13 (trixie)/catflap，
# curl 报 "Malformed input to a URL function"，光看报错完全想不到是这里。
# （Debian 13 trixie 上实测踩到。）
OS_ID="$(. /etc/os-release && printf '%s' "${ID:-}")"
case "${OS_ID}" in
  debian|ubuntu|raspbian|deepin|uos|kylin) ;;
  *) warn "本脚本主要面向 Debian / Ubuntu，当前是 ${OS_ID:-未知}，继续但请自行确认" ;;
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
  # api.github.com 和 github.com（下载）是两个域名，被墙 / 被代理规则挡住的情况
  # 经常只中一个。所以查询失败不必让用户去猜：直接告诉他怎么跳过这一步。
  # 也能 -f 到 curl 的 stderr（比如代理返回 403）照原样打出来 —— 那是最有用的线索。
  api_err="$(mktemp)"
  latest="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>"$api_err" \
            | sed -n 's/.*"tag_name" *: *"\([^"]*\)".*/\1/p' | head -n1)"
  if [ -z "$latest" ]; then
    sed 's/^/    /' "$api_err" >&2
    rm -f "$api_err"
    die "查不到最新版本。
  可以直接指定版本号跳过这一步（版本号见 https://github.com/${REPO}/releases）：
    ${SELF} --version v1.0.0
  若是网络问题，加代理或镜像前缀：
    sudo env http_proxy=http://<代理>:<端口> ${SELF} --version v1.0.0
    sudo ${SELF} --version v1.0.0 --mirror https://ghfast.top"
  fi
  rm -f "$api_err"
  # 拿到什么就信什么是危险的：API 偶尔返回空串、带换行或者别的东西，
  # 拼进 URL 之后 curl 只会说 "Malformed input to a URL function"，
  # 看不出真正原因。这里先把关，不合格就说清楚。
  case "$latest" in
    v[0-9]*[0-9]) ;;
    *) die "从 GitHub 拿到的版本号不像样（${latest}）。请手动指定：--version v1.0.0" ;;
  esac
  VERSION="$latest"
fi

# 去掉可能存在的空格与 v 前缀再重新拼 TAG：菜单里手敲版本号很容易带上空格，
# 而带空格的 URL 同样会让 curl 报那个看不懂的 malformed。
# 先去空格、再去 v 前缀，最后重新拼上 v —— 顺序不能反。
# 反了的话 " v1.0.0 " 会先被去掉 v（去不掉，前面有空格），
# 再去掉空格变成 v1.0.0，最后拼成 vv1.0.0。
VERSION="$(printf '%s' "$VERSION" | tr -d '[:space:]')"
VERSION="${VERSION#v}"
[ -n "$VERSION" ] || die "版本号是空的"
case "$VERSION" in
  *[!0-9A-Za-z._-]*) die "版本号里有非法字符：${VERSION}" ;;
esac
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
  国内网络可以试试：
    ${SELF} --version ${TAG} --mirror https://ghfast.top
  或走代理：
    sudo env http_proxy=http://<代理>:<端口> ${SELF} --version ${TAG}"

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
if [ "$RUN_AS" = "root" ]; then
  log "按你的选择：服务进程用 root 跑（跳过建专用用户）"
else
  if ! id -u "$APP" >/dev/null 2>&1; then
    log "创建系统用户 ${APP}"
    useradd --system --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin "$APP"
  fi
fi
mkdir -p "$INSTALL_DIR"
# 配置目录里有 DNS 服务商 Token 等明文凭据，必须 0700
chmod 0700 "$INSTALL_DIR"
chown -R "$RUN_AS":"$RUN_AS" "$INSTALL_DIR" 2>/dev/null || true

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
# 运行账号由 --run-as-root / CATFLAP_RUN_AS 决定（默认专用用户 catflap）。
# 用 root 时下面几条加固仍然生效，只是进程本身有完整权限 —— 这是你有意的取舍，
# 不是默认值。
UNIT
  # User/Group 不能用 heredoc 直接写：那段是 <<'UNIT'（不展开变量），
  # 写进去就变成字面量 ${RUN_AS}。和下面的 Environment 一样用 printf 输出。
  printf 'User=%s\nGroup=%s\n' "$RUN_AS" "$RUN_AS"
  cat <<'UNIT'
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

# 尽力取一个内网 IP 出来，省得用户自己去 ip a 里翻。
# 用 ip route get：它走的是内核路由表，比解析 hostname -I 稳（后者多网卡 /
# IPv6 优先时可能给出一个访问不到的地址）。取不到就退回占位符。
lan_ip="$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -n1)"
[ -n "$lan_ip" ] || lan_ip="<这台机器的内网IP>"

cat <<EOF

  Catflap ${TAG} 装好了。

  ⚠️ 请立刻打开下面的地址设置管理员密码。

      http://${lan_ip}:${PORT_SHOWN}/catflap/

  这台机器上没浏览器也没关系：首次初始化**不限制来源**，局域网里任何设备的
  浏览器都能打开。这是有意的取舍，理由见
  docs/decisions/0005-初始化接口放开来源限制.md。

  代价要知道：**在设置好密码之前，能访问到这个地址的人都可以抢先设置密码**，
  而改密码需要旧密码 —— 也就是机器会被接管。所以别把它晾在那儿。

  常用命令：
      systemctl status catflap
      journalctl -u catflap -f
      sudo ${SELF} --uninstall

EOF

if ! systemctl is-active --quiet "$APP"; then
  warn "服务没起来，看日志：journalctl -u catflap -n 50 --no-pager"
  exit 1
fi
