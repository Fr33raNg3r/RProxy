#!/bin/bash
# ============================================================================
# RProxy 客户端 公共函数库
# 被 install.sh / update-daemon.sh / watchdog.sh 共同使用
# ============================================================================

# ---------- 全局常量 ----------
readonly REPO_URL="https://github.com/Fr33raNg3r/RProxy.git"
readonly REPO_SLUG="Fr33raNg3r/RProxy"
readonly RAW_URL="https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/client"
readonly RELEASES_API="https://api.github.com/repos/${REPO_SLUG}/releases?per_page=30"
readonly INSTALL_DIR="/opt/tproxy-gw"
readonly WWW_DIR="/var/www/tproxy-gw"
readonly XRAY_CONFIG_DIR="/usr/local/etc/xray"
readonly BUILD_DIR="/tmp/tproxy-build"

readonly WG_DEFAULT_SUBNET="172.16.7.0/24"
readonly WG_DEFAULT_PORT="51820"
readonly DEFAULT_WEBUI_PORT="80"

readonly LOG_DIR="${INSTALL_DIR}/logs"
readonly DATA_DIR="${INSTALL_DIR}/data"
readonly CONFIG_DIR="${INSTALL_DIR}/config"
readonly BIN_DIR="${INSTALL_DIR}/bin"
readonly SCRIPTS_DIR="${INSTALL_DIR}/scripts"
readonly BACKUP_DIR="${INSTALL_DIR}/backup"

# 颜色
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly CYAN=$'\033[0;36m'
readonly NC=$'\033[0m'

# ---------- 日志函数 ----------
log_info()  { echo -e "${GREEN}[信息]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[警告]${NC} $*"; }
log_error() { echo -e "${RED}[错误]${NC} $*" >&2; }
log_step()  { echo -e "${CYAN}[步骤]${NC} $*"; }
log_done()  { echo -e "${GREEN}[完成]${NC} $*"; }

# 同时打印到 stdout 和日志文件
log_to_file() {
    local logfile="${LOG_DIR}/$(date +%Y%m%d).log"
    mkdir -p "${LOG_DIR}"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "${logfile}"
}

# ---------- 错误处理 ----------
die() {
    log_error "$*"
    exit 1
}

# ---------- 交互输入（兼容 wget|bash 管道执行）----------
# 用法：
#   ans=$(ask "提示文字: ")
#   ans=$(ask "提示文字: " "默认值")
ask() {
    local prompt="$1" default="${2:-}" answer=""
    if [[ -t 0 ]]; then
        read -rp "$prompt" answer
    else
        # stdin 是管道时，从 /dev/tty 读取真实终端输入
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

require_root() {
    [[ $EUID -eq 0 ]] || die "必须以 root 用户运行该脚本"
}

# ---------- 系统检测 ----------
check_debian13() {
    [[ -f /etc/os-release ]] || die "无法识别操作系统"
    local id version_id
    id=$(. /etc/os-release && echo "$ID")
    version_id=$(. /etc/os-release && echo "$VERSION_ID")
    if [[ "$id" != "debian" ]]; then
        die "本脚本仅支持 Debian，当前系统：$id"
    fi
    if [[ "$version_id" != "13" ]]; then
        die "本脚本仅支持 Debian 13 (Trixie)，当前版本：$version_id"
    fi
    log_info "系统检测通过：Debian $version_id"
}

check_arch() {
    local arch
    arch=$(uname -m)
    if [[ "$arch" != "x86_64" ]]; then
        die "本脚本仅支持 x86_64 架构，当前架构：$arch"
    fi
    log_info "架构检测通过：$arch"
}

# ---------- 已安装检测 ----------
is_installed() {
    [[ -d "${INSTALL_DIR}" && -f "${INSTALL_DIR}/VERSION" ]]
}

get_installed_version() {
    if [[ -f "${INSTALL_DIR}/VERSION" ]]; then
        cat "${INSTALL_DIR}/VERSION"
    else
        echo "未安装"
    fi
}

# 查询 GitHub 上指定组件 (client / server) 的最新 release tag。
# 成功输出 "client-v1.1.3" 这种 tag 字符串；失败返回空。
# 用 grep+sed 解析 JSON 避免依赖 jq（install.sh 早期阶段 jq 可能没装）。
get_latest_release_tag() {
    local prefix="${1:?用法: get_latest_release_tag <client|server>}-v"
    local tag
    tag=$(curl -fsSL --max-time 5 "${RELEASES_API}" 2>/dev/null \
        | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' \
        | sed -E 's/.*"([^"]+)"$/\1/' \
        | grep -E "^${prefix}[0-9]+\.[0-9]+\.[0-9]+$" \
        | head -n 1)
    echo "$tag"
}

# 兼容旧调用：返回最新 client release 的纯版本号（去掉 client-v 前缀）。
# 失败返回 "获取失败"，调用方据此显示提示。
get_remote_version() {
    local tag
    tag=$(get_latest_release_tag client)
    if [[ -z "$tag" ]]; then
        echo "获取失败"
    else
        echo "${tag#client-v}"
    fi
}

# 把用户输入的版本号 (1.1.3 / v1.1.3 / client-v1.1.3) 归一化为完整 tag，
# 并校验形如 <component>-vX.Y.Z。校验失败直接 die。
# 结果赋值给全局 RPROXY_TAG，供 fetch_source 等使用。
normalize_release_tag() {
    local raw="$1" component="${2:?用法: normalize_release_tag <ver> <client|server>}"
    local tag
    case "$raw" in
        ${component}-v*) tag="$raw" ;;
        v*)              tag="${component}-${raw}" ;;
        *)               tag="${component}-v${raw}" ;;
    esac
    if ! [[ "$tag" =~ ^${component}-v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        die "非法版本号格式: '$raw'（期望: X.Y.Z / vX.Y.Z / ${component}-vX.Y.Z）"
    fi
    RPROXY_TAG="$tag"
}

# ---------- 下载工具 ----------

# download_or_die: 下载文件到指定路径，失败则中止脚本
# 用法: download_or_die <url> <output_path> [<description>]
# 例: download_or_die "https://..." "/opt/.../geoip.dat" "GeoIP 数据"
download_or_die() {
    local url="$1" out="$2" desc="${3:-文件}"
    if ! curl -fL --max-time 120 -o "$out" "$url"; then
        die "${desc}下载失败: $url"
    fi
}

# ---------- 服务管理 ----------
restart_service() {
    local service="$1"
    if systemctl is-enabled "$service" &>/dev/null; then
        log_info "重启服务：$service"
        systemctl restart "$service"
    fi
}

is_service_active() {
    systemctl is-active --quiet "$1"
}

# ---------- JSON 操作（依赖 jq） ----------
json_get() {
    # json_get <file> <jq_expr>
    jq -r "$2" "$1" 2>/dev/null
}

json_set() {
    # json_set <file> <jq_expr>
    local file="$1"
    local expr="$2"
    local tmp
    tmp=$(mktemp)
    jq "$expr" "$file" > "$tmp" && mv "$tmp" "$file"
}

# ---------- 网络辅助 ----------
get_default_iface() {
    ip route | awk '/default/ {print $5; exit}'
}

# 通过 DoH 解析域名（curl + cloudflare/google）
resolve_doh() {
    local domain="$1"
    local result
    # 先尝试 cloudflare
    result=$(curl -s --max-time 5 \
        -H 'accept: application/dns-json' \
        "https://1.1.1.1/dns-query?name=${domain}&type=A" \
        | jq -r '.Answer[]? | select(.type==1) | .data' 2>/dev/null \
        | head -n 1)
    if [[ -z "$result" ]]; then
        # 失败则尝试 google
        result=$(curl -s --max-time 5 \
            -H 'accept: application/dns-json' \
            "https://8.8.8.8/dns-query?name=${domain}&type=A" \
            | jq -r '.Answer[]? | select(.type==1) | .data' 2>/dev/null \
            | head -n 1)
    fi
    echo "$result"
}

# ---------- 备份/恢复 ----------
make_backup() {
    local ts
    ts=$(date +%Y%m%d-%H%M%S)
    local backup_path="${BACKUP_DIR}/${ts}"
    mkdir -p "$backup_path"
    if [[ -d "${CONFIG_DIR}" ]]; then
        cp -a "${CONFIG_DIR}" "${backup_path}/config"
    fi
    if [[ -d "${XRAY_CONFIG_DIR}" ]]; then
        cp -a "${XRAY_CONFIG_DIR}" "${backup_path}/xray-etc"
    fi
    echo "$backup_path"
}

# 仅保留最近 N 个备份
prune_backups() {
    local keep="${1:-5}"
    if [[ -d "${BACKUP_DIR}" ]]; then
        ls -1t "${BACKUP_DIR}" 2>/dev/null | tail -n "+$((keep+1))" | while read -r d; do
            rm -rf "${BACKUP_DIR}/${d}"
        done
    fi
}

# ---------- 健康检查 ----------
# 通过 Xray 出站测试连通性，目标：Google generate_204（大陆无法直连）
proxy_health_check() {
    # SOCKS 端口由 Xray 配置预留，默认 10808
    local socks_port="${1:-10808}"
    local code
    code=$(curl -s -o /dev/null -w '%{http_code}' \
        --max-time 8 \
        --socks5-hostname "127.0.0.1:${socks_port}" \
        "https://www.google.com/generate_204" 2>/dev/null)
    [[ "$code" == "204" ]]
}

# ---------- 工具检查 ----------
require_cmd() {
    for c in "$@"; do
        command -v "$c" &>/dev/null || die "缺少命令：$c"
    done
}
