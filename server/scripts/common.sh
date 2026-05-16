#!/bin/bash
# ============================================================================
# RProxy 服务端 公共函数库
# 被 install.sh / tproxy-server / update-daemon.sh 共同使用
# ============================================================================

# ---------- 全局常量 ----------
readonly REPO_URL="https://github.com/Fr33raNg3r/RProxy.git"
readonly REPO_SLUG="Fr33raNg3r/RProxy"
readonly RAW_URL="https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/server"
readonly RELEASES_API="https://api.github.com/repos/${REPO_SLUG}/releases?per_page=30"
readonly INSTALL_DIR="/opt/tproxy-server"
readonly XRAY_CONFIG_FILE="/usr/local/etc/xray/config.json"
readonly BUILD_DIR="/tmp/tproxy-server-build"

readonly CONFIG_DIR="${INSTALL_DIR}/config"
readonly LOG_DIR="${INSTALL_DIR}/logs"
readonly DATA_DIR="${INSTALL_DIR}/data"
readonly SCRIPTS_DIR="${INSTALL_DIR}/scripts"

readonly SERVER_CFG="${CONFIG_DIR}/server.json"           # 服务端核心配置
readonly NGINX_CONF="/etc/nginx/conf.d/tproxy-server.conf"
readonly WEB_ROOT="/var/www/tproxy-server"
readonly SSL_DIR="/etc/ssl/tproxy-server"
readonly ACME_HOME="/root/.acme.sh"

# Xray 监听本地端口
readonly XRAY_LOCAL_PORT="9890"

# 颜色
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly BLUE=$'\033[0;34m'
readonly CYAN=$'\033[0;36m'
readonly NC=$'\033[0m'

# ---------- 日志 ----------
log_info()  { echo -e "${GREEN}[信息]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[警告]${NC} $*"; }
log_error() { echo -e "${RED}[错误]${NC} $*" >&2; }
log_step()  { echo -e "${CYAN}[步骤]${NC} $*"; }
log_done()  { echo -e "${GREEN}[完成]${NC} $*"; }

log_to_file() {
    local logfile="${LOG_DIR}/$(date +%Y%m%d).log"
    mkdir -p "${LOG_DIR}"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "${logfile}"
}

die() { log_error "$*"; exit 1; }

# ---------- 交互输入（兼容 wget|bash 管道执行） ----------
# 用法：
#   ans=$(ask "提示: ")
#   ans=$(ask "提示: " "默认值")
ask() {
    local prompt="$1" default="${2:-}" answer=""
    if [[ -t 0 ]]; then
        read -rp "$prompt" answer
    else
        { exec 3</dev/tty; } 2>/dev/null
        if [[ -e /proc/self/fd/3 ]]; then
            read -rp "$prompt" -u 3 answer
            exec 3<&-
        else
            answer="$default"
        fi
    fi
    echo "${answer:-$default}"
}

# ---------- 系统检测 ----------
require_root() {
    [[ $EUID -eq 0 ]] || die "必须以 root 用户运行该脚本"
}

check_debian13() {
    [[ -f /etc/os-release ]] || die "无法识别操作系统"
    local id version_id
    id=$(. /etc/os-release && echo "$ID")
    version_id=$(. /etc/os-release && echo "$VERSION_ID")
    [[ "$id" == "debian" ]] || die "本脚本仅支持 Debian，当前系统：$id"
    [[ "$version_id" == "13" ]] || die "本脚本仅支持 Debian 13，当前版本：$version_id"
    log_info "系统检测通过：Debian $version_id"
}

check_arch() {
    local arch=$(uname -m)
    [[ "$arch" == "x86_64" ]] || die "本脚本仅支持 x86_64，当前架构：$arch"
}

is_installed() {
    [[ -d "${INSTALL_DIR}" && -f "${INSTALL_DIR}/VERSION" ]]
}

get_installed_version() {
    [[ -f "${INSTALL_DIR}/VERSION" ]] && cat "${INSTALL_DIR}/VERSION" || echo "未安装"
}

# 查询 GitHub 上最新 release tag（vX.Y.Z，client/server 同步发布）。
# 成功输出 "v1.1.4" 这种 tag 字符串；失败返回空。
get_latest_release_tag() {
    local tag
    tag=$(curl -fsSL --max-time 5 "${RELEASES_API}" 2>/dev/null \
        | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' \
        | sed -E 's/.*"([^"]+)"$/\1/' \
        | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
        | head -n 1)
    echo "$tag"
}

# 兼容旧调用：返回最新 release 的纯版本号。失败返回 "获取失败"。
get_remote_version() {
    local tag
    tag=$(get_latest_release_tag)
    if [[ -z "$tag" ]]; then
        echo "获取失败"
    else
        echo "${tag#v}"
    fi
}

# 把用户输入的版本号 (1.1.4 / v1.1.4) 归一化为完整 tag vX.Y.Z。
# 校验失败直接 die。结果赋值给全局 RPROXY_TAG。
normalize_release_tag() {
    local raw="$1"
    local tag
    case "$raw" in
        v*) tag="$raw" ;;
        *)  tag="v${raw}" ;;
    esac
    if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        die "非法版本号格式: '$raw'（期望: X.Y.Z 或 vX.Y.Z）"
    fi
    RPROXY_TAG="$tag"
}

# ---------- SSH 端口检测 ----------
get_ssh_port() {
    local port
    if [[ -f /etc/ssh/sshd_config ]]; then
        port=$(grep -E '^\s*Port\s+' /etc/ssh/sshd_config 2>/dev/null | awk '{print $2}' | head -1)
    fi
    if [[ -z "$port" ]]; then
        port=$(ss -tlnp 2>/dev/null | grep sshd | awk '{print $4}' | sed 's/.*://' | head -1)
    fi
    echo "${port:-22}"
}

# ---------- 端口占用检查 ----------
is_port_in_use() {
    local port="$1"
    ss -tlnH 2>/dev/null | awk '{print $4}' | grep -qE ":${port}$"
}

# ---------- 服务管理 ----------
is_service_active() {
    systemctl is-active --quiet "$1"
}

restart_service() {
    systemctl restart "$1"
}

# ---------- JSON 操作 ----------
json_get() { jq -r "$2" "$1" 2>/dev/null; }

json_set() {
    local file="$1" expr="$2" tmp
    tmp=$(mktemp)
    jq "$expr" "$file" > "$tmp" && mv "$tmp" "$file"
}

# ---------- VPS 公网 IP 探测 ----------
get_public_ip() {
    local ip
    for candidate in $(hostname -I 2>/dev/null); do
        if [[ ! "$candidate" =~ ^(10\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|192\.168\.|127\.|169\.254\.) ]]; then
            ip="$candidate"
            break
        fi
    done
    if [[ -z "$ip" ]]; then
        ip=$(curl -fsSL --max-time 5 https://api.ipify.org 2>/dev/null)
    fi
    if [[ -z "$ip" ]]; then
        ip=$(curl -fsSL --max-time 5 https://ifconfig.me 2>/dev/null)
    fi
    echo "$ip"
}
