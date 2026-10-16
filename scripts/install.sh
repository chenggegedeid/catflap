#!/usr/bin/env bash
#
# Catflap 一键安装 / 更新 / 卸载脚本（Debian / Ubuntu，systemd）。
#
# 推荐写法（先下载成文件再跑，之后 `sudo bash /tmp/install.sh` 就是管理菜单）：
#
#   curl -fsSL https://raw.githubusercontent.com/chenggegedeid/catflap/main/scripts/install.sh \
#     -o /tmp/install.sh && sudo bash /tmp/install.sh
#
# ⚠️ 别把「脚本的来源地址」当作参数传回来 —— Release 资产不在
# raw.githubusercontent.com 上，传了会被当成镜像前缀拼进下载链接，拼出
# `.../main/https://github.com/...` 这种畸形 URL（curl 报 404 且看不出原因）。
# 要换镜像就传镜像域名：sudo bash /tmp/install.sh https://ghfast.top
#
# 管道写法也能用（一样会弹菜单，会去读 /dev/tty）：
#
#   curl -fsSL https://raw.githubusercontent.com/chenggegedeid/catflap/main/scripts/install.sh | sudo bash
#
# 直接跑会先看你装没装：
#   · 没装过 → 问装哪个版本
#   · 装过   → 问「更新 / 重装 / 卸载 / 完全卸载」，不会闷头就开始下载
#
# 自动化（给了任何参数就不弹菜单，保证结果确定）：
#   sudo bash install.sh --status          只看装没装、装的哪版，不动手
#   sudo bash install.sh --yes             非交互安装 / 升级到最新版
#   sudo bash install.sh --version v2.0.0  装指定版本
#   sudo bash install.sh --uninstall       卸载（保留配置与数据）
#   sudo bash install.sh --purge           卸载并删除配置与数据（**删之前自动备份**）
#   sudo bash install.sh --dir /etc/catflap 指定安装目录（卸载定位非默认目录时用）
#   sudo bash install.sh --dry-run         只打印将要做什么，不动手
#   sudo bash install.sh --run-as-root     让服务进程用 root 跑（需要绑 80/443 时）
#   sudo bash install.sh --mirror https://ghfast.top   走镜像下载
#   sudo bash install.sh https://ghfast.top            同上（位置参数写法）
#
# 可选环境变量：
#   CATFLAP_VERSION=2.0.0   装指定版本（不带 v 前缀也行）
#   CATFLAP_MIRROR=<前缀>   下载慢时用它当前缀，例如 https://ghfast.top
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
#   - **配置与数据默认不动**：升级只换二进制。--purge 才删，且**删之前先打包备份**。
#   - **先校验再替换**：下载后比对 sha256，不对就停下，绝不把坏二进制装进去。
#   - **装过就不闷头下载**：先问你要更新还是卸载，别让人以为只是"查一下"结果被升了级。

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
ASSUME_YES=0
# 有没有显式给过参数。给了就完全按参数走（适合自动化），一个都没给且能交互
# 时才进菜单。
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
info() { printf '    %s\n' "$*"; }

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
    --dir)       INSTALL_DIR="${2:-}"; BIN="${INSTALL_DIR}/${APP}"; ARGS_GIVEN=1; shift 2 ;;
    --dry-run)   DRY_RUN=1; ARGS_GIVEN=1; shift ;;
    --yes|-y)    ASSUME_YES=1; ARGS_GIVEN=1; shift ;;
    --status)    ACTION="status"; ARGS_GIVEN=1; shift ;;
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
    # 位置参数：镜像前缀（和 Lucky 的 `sh install.sh "$URL"` 一个意思）。
    # 这样换镜像不用记 --mirror，把源直接写在后面就行。
    #
    # ⚠️ 但**脚本的来源地址不是下载镜像**：
    # `.../install.sh` 从 raw.githubusercontent.com 来，而 Release 资产只能从
    # github.com（或镜像）下。照着「把脚本的 URL 原样传回来」的直觉写，
    # 会拼出 `https://raw.githubusercontent.com/.../main/https://github.com/...`
    # 这种畸形 URL，curl 报 "The requested URL returned error: 404"，
    # 光看报错完全想不到是参数被当成前缀了。
    # 所以这里认出是脚本源地址就明确忽略并说清楚，不悄悄拼坏。
    http://*|https://*)
      case "$1" in
        *raw.githubusercontent.com*|*gist.githubusercontent.com*|*jsdelivr*|*fastgit.org*)
          warn "忽略参数 $1：它是脚本自己的来源地址，不能拿来当下载镜像（Release 资产不在那儿）。"
          warn "要换镜像请用 --mirror https://ghfast.top，或直接不带参数。"
          ;;
        *) MIRROR="${1%/}" ;;
      esac
      ARGS_GIVEN=1; shift ;;
    *)           die "不认识的参数：$1（用 --help 看用法）" ;;
  esac
done

# ── 交互终端在哪 ────────────────────────────────────────────────────────────
#
# 关键改动：`curl ... | sudo bash` 时 stdin 是管道，`[ -t 0 ]` 为假，
# 老版本因此**整个跳过菜单**直接开始下载 —— 装过的机器也会被闷头覆盖掉。
#
# 但管道执行不代表不能交互：进程的**控制终端** /dev/tty 还在（只要你是坐在
# 终端前敲的这条命令）。所以这里除了 stdin，再试一次 /dev/tty。
# 真的都拿不到（CI / 非交互 SSH）才认命走非交互分支。
TTY_DEV=""
# 注意重定向顺序：`: < /dev/tty 2>/dev/null` 是不行的 ——
# bash 从左到右处理重定向，< /dev/tty 先失败就把错误打出来了，
# 那时 2>/dev/null 还没生效（脚本里会冒一行 "/dev/tty: 没有那个设备或地址"）。
# 丢进子 shell 再整体重定向才压得住。
if [ -t 0 ]; then
  TTY_DEV="/dev/stdin"
elif ( : < /dev/tty ) 2>/dev/null; then
  TTY_DEV="/dev/tty"
fi

# ── 探测已安装 ──────────────────────────────────────────────────────────────
#
# 不能只看 /opt/catflap 在不在：安装目录当初是可以改的（菜单里能选，--dir 也能给），
# 硬认默认目录会导致"明明装了却说没装"，以及更糟的 —— 卸载时删错地方。
#
# 所以优先从服务单元里把真实安装目录**反解出来**：ExecStart= 那行去掉参数取目录。
detect_install_dir() {
  local exec_line d
  [ -f "$SERVICE_FILE" ] || return 0
  exec_line="$(sed -n 's/^ExecStart=//p' "$SERVICE_FILE" | head -n1)"
  [ -n "$exec_line" ] || return 0
  # 去掉命令行参数，只留可执行文件路径
  d="$(dirname "${exec_line%% *}")"
  [ -d "$d" ] || return 0
  INSTALL_DIR="$d"
  BIN="${INSTALL_DIR}/${APP}"
}

is_installed() {
  [ -x "$BIN" ] || [ -f "$SERVICE_FILE" ]
}

# 已装版本。三个来源按可靠度排：安装时写下的 VERSION 文件 → 直接问二进制 → 问接口。
# 之所以要 VERSION 文件兜底：老版本装的机器上没有它，而服务可能没在跑（问不了接口）。
installed_version() {
  local v=""
  if [ -r "${INSTALL_DIR}/VERSION" ]; then
    v="$(head -n1 "${INSTALL_DIR}/VERSION" 2>/dev/null || true)"
  fi
  if [ -z "$v" ] && [ -x "$BIN" ] && command -v timeout >/dev/null 2>&1; then
    # ⚠️ 必须加 timeout：2.0.0 之前的二进制**不认 --version**，
    # 它会把参数直接忽略、把后端跑起来（进程里是 select{}，永不退出）。
    # 不加超时的话，在装了老版本的机器上跑这个脚本会**卡死在这里**，
    # 而且看不出为什么 —— 只看到服务莫名其妙被启动了一次。
    v="$(timeout 5 "$BIN" --version 2>/dev/null | head -n1 || true)"
  fi
  if [ -z "$v" ] && command -v curl >/dev/null 2>&1; then
    v="$(curl -fsS "http://127.0.0.1:${PORT:-3791}/api/version" 2>/dev/null \
         | sed -n 's/.*"version" *: *"\([^"]*\)".*/\1/p' | head -n1 || true)"
  fi
  # 归一成带 v 的形式再显示：三个来源给的写法不一致 ——
  # VERSION 文件存的是发布标签（v2.0.0），而二进制 --version 报的是注入值
  # （发布工作流注入的是去掉 v 的 "2.0.0"）。不归一的话菜单里会出现
  # 「已装版本：2.0.0 → 将更新到 v2.0.0」这种看着像降级的一行。
  case "$v" in
    v*)            ;;
    [0-9]*[0-9])   v="v${v}" ;;
  esac
  printf '%s' "${v:-未知}"
}

service_state() {
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$APP" 2>/dev/null; then
    printf '运行中'
  elif [ -f "$SERVICE_FILE" ]; then
    printf '已安装未运行'
  else
    printf '无服务'
  fi
}

detect_install_dir

# ── --status：只看不动手 ────────────────────────────────────────────────────
if [ "$ACTION" = "status" ]; then
  if is_installed; then
    echo "已安装：Catflap $(installed_version)"
    echo "  目录：${INSTALL_DIR}"
    echo "  服务：$(service_state)"
  else
    echo "未安装 Catflap"
  fi
  exit 0
fi

# ── 交互菜单 ────────────────────────────────────────────────────────────────
# 菜单只负责把变量问出来，后面走的还是那条安装路径，
# 不存在「菜单装的和命令装的不一样」这种情况。
#
# 输错数字**回到菜单**而不是退出 —— 手滑按错一个键就得重跑一遍脚本太烦人
# （老版本是 die 直接退出）。

ask() {
  # $1 提示，$2 默认值
  #
  # 提示语一律写 stderr：这个函数是用 $(ask ...) 取返回值的，
  # 提示语走 stdout 的话会和用户输入粘在一起被一起捕获 ——
  # 第一版就是这个 bug，菜单里选什么都被判成「没有这个选项」。
  printf '  %s' "$1" >&2
  [ -n "${2:-}" ] && printf '（默认 %s）' "$2" >&2
  printf ' > ' >&2
  local ans=""
  if [ -n "$TTY_DEV" ]; then
    read -r ans < "$TTY_DEV" || ans=""
  fi
  ans="$(printf '%s' "$ans" | tr -d '[:space:]')"
  if [ -z "$ans" ]; then printf '%s' "${2:-}"; else printf '%s' "$ans"; fi
}

banner() {
  echo
  echo "*************************************************"
  echo "**                 欢迎使用                    **"
  echo "**                Catflap 管理脚本             **"
  echo "*************************************************"
  echo
}

# 已安装时的菜单：把「更新」和「卸载」摆在一起，默认项是更新但不自动动手。
menu_installed() {
  while :; do
    banner
    echo "  检测到已安装 Catflap $(installed_version)"
    echo "    目录：${INSTALL_DIR}"
    echo "    服务：$(service_state)"
    echo
    echo "  1 更新到最新版（保留配置与数据）"
    echo "  2 重装 / 安装指定版本"
    echo "  3 卸载（保留配置与数据）"
    echo "  4 完全卸载（连配置与数据一起删，删前自动备份）"
    echo "  0 退出"
    echo
    case "$(ask '请输入对应数字' '1')" in
      1) ACTION="install"; break ;;
      2) ACTION="install"; ask_version; break ;;
      3) ACTION="uninstall"; break ;;
      4) ACTION="purge"; break ;;
      0) echo "已退出"; exit 0 ;;
      *) echo "  没有这个选项，请重新输入" ;;
    esac
  done
}

menu_fresh() {
  while :; do
    banner
    echo "  1 安装最新版"
    echo "  2 安装指定版本"
    echo "  0 退出"
    echo
    case "$(ask '请输入对应数字' '1')" in
      1) ACTION="install"; break ;;
      2) ACTION="install"; ask_version; break ;;
      0) echo "已退出"; exit 0 ;;
      *) echo "  没有这个选项，请重新输入" ;;
    esac
  done
  ask_options
}

# 装新的时候才问目录 / 账号 / 端口。更新时不问 —— 那些值已经落在服务单元里了，
# 再问一遍等于给用户一个"不小心改掉运行方式"的机会。
ask_version() {
  echo
  echo "--- 版本 ---"
  echo "  直接回车 = 最新版；也可以填指定版本，例如 2.0.0"
  VERSION="$(ask '版本号')"
}

ask_options() {
  echo
  echo "--- 安装目录 ---"
  echo "  常用：/opt/catflap（默认）、/etc/catflap、/usr/share/catflap"
  INSTALL_DIR="$(ask '安装目录' '/opt/catflap')"
  BIN="${INSTALL_DIR}/${APP}"

  echo
  echo "--- 服务用什么账号跑 ---"
  echo "  1 catflap（专用用户，默认，更安全）"
  echo "  2 root（需要用 80/443 或动 iptables 时选这个）"
  case "$(ask '请选择' '1')" in
    1) RUN_AS="catflap" ;;
    2) RUN_AS="root" ;;
    *) echo "  没有这个选项，按默认（catflap）"; RUN_AS="catflap" ;;
  esac

  echo
  echo "--- 监听端口 ---"
  PORT="$(ask '端口' '3791')"
}

confirm_or_exit() {
  # $1 要干什么的一句话
  echo
  printf '  即将：%s\n' "$1"
  case "$(ask '确认开始？[Y/n]' 'y')" in
    y|Y|yes|YES|'') ;;
    *) echo "已取消"; exit 0 ;;
  esac
}

if [ "$ARGS_GIVEN" -eq 0 ] && [ -n "$TTY_DEV" ]; then
  if is_installed; then menu_installed; else menu_fresh; fi
  case "$ACTION" in
    install) confirm_or_exit "安装 / 更新到 ${INSTALL_DIR}（账号 ${RUN_AS}，端口 ${PORT:-3791}）" ;;
    uninstall) confirm_or_exit "卸载但保留 ${INSTALL_DIR} 下的配置与数据" ;;
    purge) confirm_or_exit "完全卸载并删除 ${INSTALL_DIR}（含配置与数据，删前先备份）" ;;
  esac
fi

# ── 非交互但已经装过：不动手，把选项说清楚 ────────────────────────────────
#
# 管道执行又抓不到终端（CI、非交互 SSH）时，装没装是两回事：
#   · 没装过 → 照常装，不然 CI 里"一条命令装好"这件事就废了；
#   · 装过   → **什么也别做**。用户很可能只是想看看，闷头升级等于替他做了决定，
#             而升级是有风险的（配置格式会变、服务会重启）。
if [ "$ARGS_GIVEN" -eq 0 ] && [ -z "$TTY_DEV" ] && is_installed; then
  echo
  echo "  检测到已安装 Catflap $(installed_version)（目录 ${INSTALL_DIR}，服务$(service_state)）"
  echo "  当前不是交互终端，为避免动到你现有的配置与数据，**未做任何改动**。"
  echo "  你的配置与数据都保留在 ${INSTALL_DIR}。"
  echo
  echo "  要更新：  sudo ${SELF} --yes"
  echo "  要升级到指定版本：  sudo ${SELF} --yes --version v2.0.0"
  echo "  要卸载（保留配置）：  sudo ${SELF} --uninstall"
  echo "  要完全卸载：  sudo ${SELF} --purge"
  exit 0
fi

# ── 卸载 ────────────────────────────────────────────────────────────────────
if [ "$ACTION" = "uninstall" ] || [ "$ACTION" = "purge" ]; then
  [ "$(id -u)" -eq 0 ] || die "卸载需要 root：sudo ${SELF} --uninstall"
  is_installed || die "没找到已安装的 Catflap（${INSTALL_DIR}）。装在别的地方就用 --dir 指一下：sudo ${SELF} --uninstall --dir /etc/catflap"

  log "停止并禁用服务"
  run systemctl stop "$APP" 2>/dev/null || true
  run systemctl disable "$APP" 2>/dev/null || true
  # reset-failed 必须做：服务失败过的话 systemd 会留一个 failed 状态，
  # 不清掉下次 install 同一个名字会被报成"已存在且失败"，看着像装坏了。
  run systemctl reset-failed "$APP" 2>/dev/null || true

  log "删除服务单元"
  run rm -f "$SERVICE_FILE"
  run systemctl daemon-reload

  # 兜底：进程可能不是 systemd 拉起来的（比如手动跑过一次），停服务管不到它
  if command -v pkill >/dev/null 2>&1 && [ "$DRY_RUN" -eq 0 ]; then
    pkill -f "${BIN}" 2>/dev/null || true
  fi

  if [ "$ACTION" = "purge" ]; then
    # 删数据之前**先打包备份**。用户可能几年后才想起"那条 DDNS 配置"，
    # 也可能只是手滑选了 4 —— 备份一份，删错了还能救回来。
    # tar 失败就停手：备份没做成还删数据，是最坏的组合。
    bak="${INSTALL_DIR}.bak-$(date +%Y%m%d-%H%M%S).tar.gz"
    log "备份 ${INSTALL_DIR} → ${bak}"
    if [ "$DRY_RUN" -eq 0 ]; then
      tar czf "$bak" -C "$(dirname "$INSTALL_DIR")" "$(basename "$INSTALL_DIR")" \
        || die "备份失败，已停止卸载（数据都还在）。空间不够就先清一下再试。"
    fi
    log "删除 ${INSTALL_DIR}（含配置与数据）"
    run rm -rf "$INSTALL_DIR"
    if id -u "$APP" >/dev/null 2>&1; then
      log "删除系统用户 ${APP}"
      run userdel "$APP" 2>/dev/null || true
    fi
    log "完全卸载完成"
    echo
    info "配置与数据的备份在：${bak}"
    info "确认不需要了再删： rm -f ${bak}"
  else
    log "保留 ${INSTALL_DIR} 下的配置与数据"
    run rm -f "$BIN"
    run rm -f "${INSTALL_DIR}/VERSION"
    log "卸载完成"
    echo
    info "配置与数据还在 ${INSTALL_DIR}，随时可以重装回来。"
    info "要连它们一起删： sudo ${SELF} --purge"
  fi
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
    ${SELF} --version v2.0.0
  若是网络问题，加代理或镜像前缀：
    sudo env http_proxy=http://<代理>:<端口> ${SELF} --version v2.0.0
    sudo ${SELF} --version v2.0.0 --mirror https://ghfast.top"
  fi
  rm -f "$api_err"
  # 拿到什么就信什么是危险的：API 偶尔返回空串、带换行或者别的东西，
  # 拼进 URL 之后 curl 只会说 "Malformed input to a URL function"，
  # 看不出真正原因。这里先把关，不合格就说清楚。
  case "$latest" in
    v[0-9]*[0-9]) ;;
    *) die "从 GitHub 拿到的版本号不像样（${latest}）。请手动指定：--version v2.0.0" ;;
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

# 装过的机器先说一声：让用户知道这只是在换二进制，配置不会动
if is_installed; then
  log "已装版本：$(installed_version) → 将更新到 ${TAG}（配置与数据保留）"
fi

base_url() {
  # MIRROR 是「前缀」而不是完整 URL：拼在 github.com 前面即可
  if [ -z "$MIRROR" ]; then
    printf '%s' "https://github.com"
    return 0
  fi
  m="$MIRROR"
  # 没写 scheme 就补上：用户常写成 --mirror ghfast.top，
  # 拼出来是 "ghfast.top/https://github.com"（没有 scheme，curl 直接失败）。
  case "$m" in
    http://*|https://*) ;;
    *) m="https://${m}" ;;
  esac
  # 前缀里不该再出现 github —— 那是拼坏的迹象（多半是把完整 URL 当前缀传了）。
  #
  # ⚠️ 只写 `*github*` 不要写 `*github.com*`：
  # raw.githubusercontent.com 里是 "githubusercontent"，**不包含**
  # "github.com" 这个子串 —— 用后者这个守卫形同虚设（实测漏过）。
  case "$m" in
    *github*) die "镜像前缀里不该带 github（拿到的是 ${MIRROR}）。前缀只填镜像域名，例如 https://ghfast.top" ;;
  esac
  printf '%s' "${m%/}/https://github.com"
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
# 记下这一版：卸载/更新时要告诉用户"你装的是哪一版"，
# 光靠二进制问不总是行（服务没起、老版本还没有 --version）。
printf '%s\n' "$TAG" > "${INSTALL_DIR}/VERSION"

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
  # 安装目录也不能写死：菜单和 --dir 都能改它，写死的话装到别处就启不来。
  printf 'WorkingDirectory=%s\nExecStart=%s\n' "$INSTALL_DIR" "$BIN"
  cat <<'UNIT'
Restart=on-failure
RestartSec=5
# 新建文件默认只对属主可读写（config/ 里是明文凭据）
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
UNIT
  printf 'ReadWritePaths=%s\n' "$INSTALL_DIR"
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
      catflap --version
      sudo ${SELF} --status
      sudo ${SELF} --uninstall

EOF

if ! systemctl is-active --quiet "$APP"; then
  warn "服务没起来，看日志：journalctl -u catflap -n 50 --no-pager"
  exit 1
fi
