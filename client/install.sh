#!/bin/bash
# ============================================================================
# RProxy 客户端 安装/升级/卸载脚本
# 适用：Debian 13 x86_64
# 用法：
#   wget -O- https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/client/install.sh | bash
#   或下载后：bash install.sh [install|upgrade|uninstall]
# ============================================================================

set -e

# ---------- 加载公共函数 ----------
# 如果是通过 wget pipe 执行，需要先下载 common.sh
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo /tmp)"

if [[ -f "${SELF_DIR}/scripts/common.sh" ]]; then
    # 本地执行模式
    source "${SELF_DIR}/scripts/common.sh"
    LOCAL_MODE=1
else
    # 远程执行模式（管道安装）
    TMP_COMMON=$(mktemp)
    if ! curl -fsSL "https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/client/scripts/common.sh" -o "${TMP_COMMON}"; then
        echo "[错误] 无法下载 common.sh，请检查网络" >&2
        exit 1
    fi
    source "${TMP_COMMON}"
    rm -f "${TMP_COMMON}"
    LOCAL_MODE=0
fi

# ---------- 主菜单 ----------
show_menu() {
    while true; do
        clear
        cat <<EOF
${CYAN}╔═══════════════════════════════════════════════════════════════╗
║              RProxy 客户端（旁路由） 安装/管理脚本                     ║
║              适用：Debian 13 x86_64 旁路由                     ║
╚═══════════════════════════════════════════════════════════════╝${NC}

EOF

        # 远程版本（GitHub）—— 同步显示，超时 5 秒
        local remote_ver
        remote_ver=$(get_remote_version)
        if [[ "$remote_ver" == "获取失败" ]]; then
            echo -e "  最新版本（GitHub）：${YELLOW}获取失败${NC}"
            echo -e "  ${YELLOW}⚠ 无法访问 GitHub，国内用户请挂代理后再安装：${NC}"
            echo -e "    ${YELLOW}export https_proxy=http://你的代理:port${NC}"
            echo -e "    ${YELLOW}export http_proxy=http://你的代理:port${NC}"
        else
            echo -e "  最新版本（GitHub）：${CYAN}${remote_ver}${NC}"
        fi

        if is_installed; then
            local local_ver
            local_ver=$(get_installed_version)
            if [[ "$remote_ver" != "获取失败" && "$local_ver" != "$remote_ver" ]]; then
                echo -e "  当前已安装版本：  ${YELLOW}${local_ver}${NC}  ${YELLOW}（有新版可升级）${NC}"
            else
                echo -e "  当前已安装版本：  ${GREEN}${local_ver}${NC}"
            fi
        else
            echo -e "  当前状态：        ${YELLOW}未安装${NC}"
        fi
        echo ""
        echo "  请选择操作："
        echo "    1) 全新安装（清理已有配置）"
        echo "    2) 升级安装（保留配置）"
        echo "    3) 卸载"
        echo "    4) 查看状态"
        echo "    5) 紧急停止透明代理"
        echo "    6) 系统优化（内核切换、网络调优）"
        echo "    0) 退出"
        echo ""
        local choice
        choice=$(ask "  请输入选项 [0-6]: ")
        case "$choice" in
            # 终止性操作：执行后退出 shell（用户预期一次性完成）
            1) action_install_fresh; exit 0 ;;
            2) action_upgrade; exit 0 ;;
            3) action_uninstall; exit 0 ;;
            # 查看/工具类操作：执行完返回菜单继续
            4) action_status; press_enter ;;
            5) action_emergency_stop; press_enter ;;
            6) action_system_optimize ;;
            0) exit 0 ;;
            *) log_error "无效选项"; sleep 1 ;;
        esac
    done
}

# 等用户按回车后再回主菜单（避免输出被 clear 冲掉）
press_enter() {
    echo ""
    read -rp "  按回车键返回主菜单..." _ </dev/tty || true
}

# 加载并执行系统优化菜单
action_system_optimize() {
    local opt_script="${INSTALL_DIR}/scripts/system-optimize.sh"
    # 已安装：用部署的脚本
    if [[ -f "$opt_script" ]]; then
        # shellcheck disable=SC1090
        source "$opt_script"
    # 未安装：从源码目录用
    elif [[ -f "${SELF_DIR}/scripts/system-optimize.sh" ]]; then
        # shellcheck disable=SC1091
        source "${SELF_DIR}/scripts/system-optimize.sh"
    else
        die "找不到 system-optimize.sh，请确认安装完整"
    fi
    system_optimize_menu
}

# ============================================================================
# 准备工作：apt 源、基础包、内核参数等
# ============================================================================

setup_apt_source() {
    log_step "替换 apt 源为清华镜像"
    local sources_file="/etc/apt/sources.list.d/debian.sources"
    if [[ -f "$sources_file" ]]; then
        cp "$sources_file" "${sources_file}.tproxy.bak"
    fi
    cat > "$sources_file" <<EOF
Types: deb deb-src
URIs: https://mirrors.tuna.tsinghua.edu.cn/debian
Suites: trixie trixie-updates trixie-backports
Components: main contrib non-free non-free-firmware
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg

Types: deb deb-src
URIs: https://mirrors.tuna.tsinghua.edu.cn/debian-security
Suites: trixie-security
Components: main contrib non-free non-free-firmware
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
EOF
    # 移除旧的 sources.list（避免重复）
    if [[ -f /etc/apt/sources.list ]]; then
        mv /etc/apt/sources.list /etc/apt/sources.list.tproxy.bak
    fi
    log_done "apt 源已切换"
}

install_base_packages() {
    log_step "更新 apt 索引并安装基础软件包"
    apt-get update -y
    apt-get install -y --no-install-recommends \
        wget curl ca-certificates \
        nftables wireguard-tools qrencode jq \
        unzip tar xz-utils \
        cron systemd
    log_done "基础软件包安装完成"
}

install_build_toolchain() {
    log_step "安装编译工具链（Go + Node.js + npm）"
    apt-get install -y --no-install-recommends golang-go nodejs npm
    # 配置国内镜像
    npm config set registry https://registry.npmmirror.com
    export GOPROXY=https://goproxy.cn,direct
    export GO111MODULE=on
    log_done "编译工具链安装完成"
}

remove_build_toolchain() {
    log_step "卸载编译工具链以释放磁盘"
    apt-get remove -y --purge golang-go nodejs npm 2>/dev/null || true
    apt-get autoremove -y --purge 2>/dev/null || true
    rm -rf /root/.npm /root/.cache/go-build /root/go 2>/dev/null || true
    apt-get clean
    log_done "编译工具链已清理"
}

disable_ipv6() {
    log_step "禁用 IPv6"
    cat > /etc/sysctl.d/99-tproxy-disable-ipv6.conf <<EOF
net.ipv6.conf.all.disable_ipv6 = 1
net.ipv6.conf.default.disable_ipv6 = 1
net.ipv6.conf.lo.disable_ipv6 = 1
EOF
    sysctl --system >/dev/null
    log_done "IPv6 已禁用"
}

enable_ip_forward() {
    log_step "启用 IP 转发"
    cat > /etc/sysctl.d/99-tproxy-forward.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
EOF
    sysctl --system >/dev/null
    log_done "IP 转发已启用"
}

disable_systemd_resolved() {
    log_step "停用 systemd-resolved（让 mosdns 接管 53 端口）"
    if systemctl is-enabled systemd-resolved &>/dev/null; then
        systemctl disable --now systemd-resolved
    fi
    # 替换 resolv.conf
    rm -f /etc/resolv.conf
    cat > /etc/resolv.conf <<EOF
# Managed by RProxy - DNS handled by mosdns on 127.0.0.1:53
nameserver 127.0.0.1
EOF
    # 防止被覆盖
    chattr +i /etc/resolv.conf 2>/dev/null || true
    log_done "systemd-resolved 已停用，resolv.conf 指向 mosdns"
}

# ============================================================================
# 下载、编译、部署
# ============================================================================

fetch_source() {
    log_step "拉取 RProxy 客户端源码到 ${BUILD_DIR}"
    rm -rf "${BUILD_DIR}"
    if [[ "${LOCAL_MODE}" == "1" && -d "${SELF_DIR}/webui-backend" ]]; then
        # 本地模式：直接复制
        log_info "本地模式：从 ${SELF_DIR} 复制源码"
        cp -a "${SELF_DIR}" "${BUILD_DIR}"
    else
        mkdir -p "${BUILD_DIR}"
        # 选择 ref：用户指定的 tag > 自动查询最新 release > main 兜底
        local ref tarball_prefix
        if [[ -n "${RPROXY_TAG:-}" ]]; then
            ref="${RPROXY_TAG}"
            log_info "远程模式：拉取 tag ${ref}"
        else
            local latest
            latest=$(get_latest_release_tag client || true)
            if [[ -n "$latest" ]]; then
                ref="$latest"
                log_info "远程模式：未指定版本，使用最新 release ${ref}"
            else
                ref="main"
                log_warn "未能查询到 release，回退到 main 分支（不稳定）"
            fi
        fi
        # tarball 顶层目录名 = "<repo>-<ref-without-leading-v-if-any-prefix-version>"
        # GitHub 行为：tags/X 的 tarball 解压顶层是 RProxy-X（去掉前缀 v 也保留——见下方）
        # tag "client-v1.1.3" → 顶层 "RProxy-client-v1.1.3"
        # branch "main"     → 顶层 "RProxy-main"
        tarball_prefix="RProxy-${ref#v}"
        local url
        if [[ "$ref" == "main" ]]; then
            url="https://github.com/Fr33raNg3r/RProxy/archive/refs/heads/main.tar.gz"
        else
            url="https://github.com/Fr33raNg3r/RProxy/archive/refs/tags/${ref}.tar.gz"
        fi
        # --strip-components=2 去掉 <prefix>/client/ 两层，只解压 client 子目录
        if ! curl -fsSL "$url" \
                | tar xz -C "${BUILD_DIR}" --strip-components=2 "${tarball_prefix}/client"; then
            die "源码下载失败: $url"
        fi
    fi
    log_done "源码已就绪"
}

install_xray() {
    log_step "通过官方一键脚本安装 Xray-core"
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
    # 确保 Xray 日志目录存在并有正确权限（保险措施）
    mkdir -p /var/log/xray
    chown nobody:nogroup /var/log/xray 2>/dev/null || chown nobody /var/log/xray
    chmod 755 /var/log/xray

    # 关键修复：让 Xray 以 root 运行
    # 原因：透明代理需要 Xray 用 SO_MARK 给出包打 mark=0xff（防 nftables 环路），
    #       这个 socket 选项需要 CAP_NET_ADMIN 权限。
    #       Xray 官方 systemd unit 默认 User=nobody + NoNewPrivileges=true，
    #       即使有 AmbientCapabilities=CAP_NET_ADMIN，SO_MARK 仍然被内核拒绝。
    #       让它以 root 运行能彻底解决这个问题。
    # 安全性：旁路由是内网设备，不暴露公网，root 运行可接受。
    log_step "让 Xray 以 root 运行（透明代理需要）"
    write_xray_override
    # 清理可能残留的 access.log（之前 root 跑过留下的，nobody 写不了）
    rm -f /var/log/xray/access.log /var/log/xray/error.log
    systemctl daemon-reload
    log_done "Xray 安装完成：$(xray version | head -n 1)"
}

# Xray systemd override 配置——单独提取出来供升级流程也能调用
# 这是透明代理工作的关键：必须以 root 运行 + 必须有 CAP_NET_ADMIN
# NoNewPrivileges 必须显式 false，不能用空值（systemd 会忽略空值，保留 true）
write_xray_override() {
    mkdir -p /etc/systemd/system/xray.service.d
    cat > /etc/systemd/system/xray.service.d/override.conf <<'EOF'
[Service]
# 清空原 systemd unit 中的 User/Group 配置（systemd 允许用空值清空字符串字段）
User=
Group=

# NoNewPrivileges / CapabilityBoundingSet / AmbientCapabilities 不能用空值！
# 必须显式设置正确的值，否则原 unit 中的 NoNewPrivileges=true 仍生效，
# 导致 Xray 即使以 root 跑，TPROXY socket 也收不到包（缺 CAP_NET_ADMIN）
NoNewPrivileges=false
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_SYS_RESOURCE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE

# 显式以 root 运行
User=root
Group=root
EOF
}

install_mosdns() {
    log_step "下载并安装 mosdns v5"
    local arch="amd64"
    local tmp_dir
    tmp_dir=$(mktemp -d)
    cd "$tmp_dir"
    # 获取最新版下载链接
    local download_url
    download_url=$(curl -fsSL https://api.github.com/repos/IrineSistiana/mosdns/releases/latest \
        | jq -r ".assets[] | select(.name | endswith(\"linux-${arch}.zip\")) | .browser_download_url" \
        | head -n 1)
    if [[ -z "$download_url" ]]; then
        die "无法获取 mosdns 下载链接"
    fi
    log_info "下载：${download_url}"
    curl -fL -o mosdns.zip "$download_url"
    unzip -o mosdns.zip
    install -m 0755 mosdns "${BIN_DIR}/mosdns"
    cd /
    rm -rf "$tmp_dir"
    log_done "mosdns 已安装：$(${BIN_DIR}/mosdns version 2>&1 | head -n 1)"
}

build_webui() {
    log_step "编译 WebUI 后端（Go）"
    cd "${BUILD_DIR}/webui-backend"
    # 多镜像 fallback：goproxy.cn 优先，失败时自动尝试 goproxy.io 和官方
    export GOPROXY=https://goproxy.cn,https://goproxy.io,https://proxy.golang.org,direct
    export GOSUMDB=sum.golang.google.cn  # 使用国内镜像的校验源
    
    # go mod tidy 带重试（应对 CDN 抖动 / HTTP/2 协议错误）
    local retry=0
    local max_retry=3
    while [[ $retry -lt $max_retry ]]; do
        if go mod tidy; then
            break
        fi
        retry=$((retry + 1))
        if [[ $retry -lt $max_retry ]]; then
            log_warn "go mod tidy 失败，等待 5 秒后重试（$retry/$max_retry）..."
            sleep 5
            # 切换到下一个 GOPROXY，跳过当前可能出问题的镜像
            case $retry in
                1) export GOPROXY=https://goproxy.io,https://proxy.golang.org,direct ;;
                2) export GOPROXY=https://proxy.golang.org,direct ;;
            esac
        fi
    done
    
    if [[ $retry -eq $max_retry ]]; then
        die "go mod tidy 经过 $max_retry 次重试仍失败，请检查网络后重新运行 install.sh"
    fi
    
    go build -trimpath -ldflags='-s -w' -o "${BIN_DIR}/webui" .
    log_done "Go 后端编译完成"

    log_step "编译 WebUI 前端（Vue）"
    cd "${BUILD_DIR}/webui-frontend"
    npm install --no-audit --no-fund
    npm run build
    rm -rf "${WWW_DIR}"
    mkdir -p "${WWW_DIR}"
    cp -a dist/* "${WWW_DIR}/"
    log_done "Vue 前端编译并部署完成"
    cd /
}

download_geo_data() {
    log_step "下载 Xray 用的 GeoIP / GeoSite .dat 文件"
    local geo_dir="${INSTALL_DIR}/data/geo"
    local rules_url="https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download"
    mkdir -p "$geo_dir"

    # Xray-core 用 .dat 二进制格式做内部分流
    download_or_die "${rules_url}/geoip.dat"   "${geo_dir}/geoip.dat"   "geoip.dat"
    download_or_die "${rules_url}/geosite.dat" "${geo_dir}/geosite.dat" "geosite.dat"
    # 复制到 Xray 默认查找路径
    mkdir -p /usr/local/share/xray
    cp -f "${geo_dir}/geoip.dat"   /usr/local/share/xray/
    cp -f "${geo_dir}/geosite.dat" /usr/local/share/xray/
    log_done "GeoIP/GeoSite .dat 已下载"

    log_step "下载 mosdns 用的纯文本规则文件"
    # 直接下载 Loyalsoldier release 分支预解压的 .txt 列表，无需 v2dat 工具
    download_or_die "${rules_url}/direct-list.txt" "${geo_dir}/geosite-cn.txt" "geosite-cn"
    download_or_die "${rules_url}/proxy-list.txt"  "${geo_dir}/proxy-list.txt" "proxy-list"
    download_or_die "${rules_url}/gfw.txt"         "${geo_dir}/gfw.txt"        "gfw.txt"
    download_or_die \
        "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt" \
        "${geo_dir}/geoip-cn.txt" "geoip-cn"

    # 合并 proxy-list + gfw → geosite-no-cn.txt（mosdns 配置里引用的文件名）
    cat "${geo_dir}/proxy-list.txt" "${geo_dir}/gfw.txt" \
        | sort -u > "${geo_dir}/geosite-no-cn.txt"

    log_done "mosdns 规则文件已就绪"
}

deploy_scripts_and_configs() {
    log_step "部署脚本和配置文件"
    mkdir -p "${INSTALL_DIR}" "${SCRIPTS_DIR}" "${CONFIG_DIR}" \
             "${DATA_DIR}" "${LOG_DIR}" "${BACKUP_DIR}" "${BIN_DIR}" \
             "${CONFIG_DIR}/xray" "${CONFIG_DIR}/mosdns" \
             "${CONFIG_DIR}/wireguard" "${CONFIG_DIR}/dns"

    # 部署 shell 脚本
    cp "${BUILD_DIR}/scripts/common.sh" "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/update-daemon.sh" "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/watchdog.sh" "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/emergency-stop.sh" "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/system-optimize.sh" "${SCRIPTS_DIR}/"
    # install.sh 本身也部署到 INSTALL_DIR（升级时需要重新运行）
    cp "${BUILD_DIR}/install.sh" "${INSTALL_DIR}/install.sh"
    chmod +x "${SCRIPTS_DIR}"/*.sh "${INSTALL_DIR}/install.sh"

    # 部署 nftables 规则
    cp "${BUILD_DIR}/configs/nftables-tproxy.nft" /etc/nftables.conf

    # 部署 systemd 服务
    cp "${BUILD_DIR}/configs/systemd/"*.service /etc/systemd/system/
    cp "${BUILD_DIR}/configs/systemd/"*.timer   /etc/systemd/system/
    systemctl daemon-reload

    # 部署版本号
    cp "${BUILD_DIR}/VERSION" "${INSTALL_DIR}/VERSION"

    log_done "脚本和配置文件已部署"
}

generate_default_configs() {
    log_step "生成默认配置文件"

    # ----- WebUI 配置（端口、密码哈希等） -----
    if [[ ! -f "${CONFIG_DIR}/webui.json" ]]; then
        local random_pass
        random_pass=$(tr -dc 'A-Za-z0-9' </dev/urandom | head -c 12)
        # 用 Go 程序生成 bcrypt 哈希
        local pass_hash
        pass_hash=$("${BIN_DIR}/webui" hashpass "${random_pass}")
        cat > "${CONFIG_DIR}/webui.json" <<EOF
{
  "listen_port": ${DEFAULT_WEBUI_PORT},
  "username": "admin",
  "password_hash": "${pass_hash}",
  "session_secret": "$(tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32)",
  "wg_enabled": false,
  "wg_listen_port": ${WG_DEFAULT_PORT},
  "wg_subnet": "${WG_DEFAULT_SUBNET}",
  "update_hour": 4,
  "update_minute": 0,
  "current_node_id": ""
}
EOF
        chmod 600 "${CONFIG_DIR}/webui.json"
        # 把初始密码记下来给安装脚本最后打印
        echo "${random_pass}" > "${CONFIG_DIR}/.initial_password"
        chmod 600 "${CONFIG_DIR}/.initial_password"
    fi

    # ----- 节点池（空） -----
    if [[ ! -f "${CONFIG_DIR}/xray/nodes.json" ]]; then
        cat > "${CONFIG_DIR}/xray/nodes.json" <<'EOF'
{
  "nodes": []
}
EOF
    fi

    # ----- mosdns 配置 -----
    if [[ ! -f "${CONFIG_DIR}/mosdns/config.yaml" ]]; then
        cp "${BUILD_DIR}/configs/mosdns-config.yaml.tpl" "${CONFIG_DIR}/mosdns/config.yaml"
    fi

    # ----- DNS 黑白名单（用户可在 WebUI 里加） -----
    [[ -f "${CONFIG_DIR}/dns/whitelist.txt" ]] || touch "${CONFIG_DIR}/dns/whitelist.txt"
    [[ -f "${CONFIG_DIR}/dns/blacklist.txt" ]] || touch "${CONFIG_DIR}/dns/blacklist.txt"
    [[ -f "${CONFIG_DIR}/dns/hosts.txt"     ]] || touch "${CONFIG_DIR}/dns/hosts.txt"

    # ----- WireGuard 配置（私钥首次生成） -----
    if [[ ! -f "${CONFIG_DIR}/wireguard/server_privatekey" ]]; then
        umask 077
        wg genkey > "${CONFIG_DIR}/wireguard/server_privatekey"
        wg pubkey < "${CONFIG_DIR}/wireguard/server_privatekey" > "${CONFIG_DIR}/wireguard/server_publickey"
    fi
    if [[ ! -f "${CONFIG_DIR}/wireguard/peers.json" ]]; then
        cat > "${CONFIG_DIR}/wireguard/peers.json" <<'EOF'
{
  "peers": []
}
EOF
    fi

    # ----- Xray 默认配置（无节点时仅监听 SOCKS，不做 TPROXY 直到选定节点） -----
    if [[ ! -f "${XRAY_CONFIG_DIR}/config.json" ]]; then
        # 调用 webui 程序根据 nodes.json + current_node 渲染配置
        # 此时 nodes 为空，会生成一个仅 SOCKS、无 outbound 代理的配置
        "${BIN_DIR}/webui" render-xray || {
            # 兜底：写一个最小安全配置
            cat > "${XRAY_CONFIG_DIR}/config.json" <<EOF
{
  "log": { "loglevel": "warning" },
  "inbounds": [
    {
      "tag": "socks-in",
      "port": 10808,
      "protocol": "socks",
      "settings": { "auth": "noauth", "udp": true },
      "listen": "127.0.0.1"
    }
  ],
  "outbounds": [
    { "tag": "direct", "protocol": "freedom" }
  ]
}
EOF
        }
    fi

    log_done "默认配置已生成"
}

generate_wg_config_file() {
    # 根据 peers.json 生成 /etc/wireguard/wg0.conf
    "${BIN_DIR}/webui" render-wg || true
}

# ============================================================================
# nftables / systemd 启用
# ============================================================================

enable_nftables() {
    log_step "启用并加载 nftables 规则"
    # 加载前先做语法预检，避免规则错误时 systemctl 报含糊错误
    if ! nft -c -f /etc/nftables.conf; then
        log_error "nftables 配置语法错误，安装中止"
        exit 1
    fi
    systemctl enable --now nftables.service
    nft -f /etc/nftables.conf
    log_done "nftables 规则已加载"
}

enable_services() {
    log_step "启用 systemd 服务"
    systemctl enable --now tproxy-gw-iproute.service
    systemctl enable --now xray.service || log_warn "Xray 服务启动失败（可能因为没有节点，待添加节点后会自动正常）"
    systemctl enable --now tproxy-gw-mosdns.service
    systemctl enable --now tproxy-gw-webui.service
    systemctl enable --now tproxy-gw-watchdog.timer
    systemctl enable --now tproxy-gw-update.timer
    systemctl enable --now tproxy-gw-flush-ipsets.timer
    # WG 服务在添加 peer 后由 webui 启用
    log_done "服务已启用"
}

# ============================================================================
# 安装/升级/卸载主流程
# ============================================================================

action_install_fresh() {
    require_root
    check_arch
    check_debian13

    if is_installed; then
        echo ""
        log_warn "检测到已有安装，全新安装会清除所有现有配置！"
        confirm=$(ask "确认继续？(yes/no): ")
        [[ "$confirm" == "yes" ]] || { log_info "已取消"; exit 0; }
        # 先卸载
        do_uninstall_silent
    fi

    setup_apt_source
    install_base_packages
    disable_ipv6
    enable_ip_forward

    fetch_source
    install_xray

    # 创建目录
    mkdir -p "${INSTALL_DIR}" "${BIN_DIR}"

    install_build_toolchain
    build_webui
    # 保存源码哈希——升级时若哈希一致就跳过重新编译
    compute_webui_hash "${BUILD_DIR}" > "${INSTALL_DIR}/.webui-hash"
    install_mosdns
    download_geo_data

    deploy_scripts_and_configs
    generate_default_configs
    generate_wg_config_file

    enable_nftables
    disable_systemd_resolved      # mosdns 启动前才停掉 systemd-resolved，避免安装中 DNS 中断
    enable_services

    remove_build_toolchain

    show_completion_info
}

action_upgrade() {
    require_root
    check_arch
    check_debian13

    if ! is_installed; then
        log_error "未检测到已有安装，请先全新安装"
        exit 1
    fi

    log_info "开始升级，备份当前配置..."
    local backup_path
    backup_path=$(make_backup)
    log_info "备份路径：${backup_path}"

    fetch_source

    # 检查 VERSION 是否有变化
    local old_ver new_ver
    old_ver=$(cat "${INSTALL_DIR}/VERSION")
    new_ver=$(cat "${BUILD_DIR}/VERSION")
    log_info "本地版本：${old_ver}    最新版本：${new_ver}"

    # ===================== 按需重做：避免无谓的下载/编译 =====================
    # 1) WebUI 哈希比对：源码不变就不装编译工具，不重新编译
    local old_webui_hash new_webui_hash
    old_webui_hash=$(cat "${INSTALL_DIR}/.webui-hash" 2>/dev/null || echo "")
    new_webui_hash=$(compute_webui_hash "${BUILD_DIR}")
    if [[ -n "$new_webui_hash" && "$old_webui_hash" == "$new_webui_hash" ]]; then
        log_info "WebUI 源码无变化（hash 一致），跳过编译"
    else
        log_info "WebUI 源码有变化，开始重新编译"
        install_build_toolchain
        build_webui
        # 编译成功后记录新哈希
        echo "$new_webui_hash" > "${INSTALL_DIR}/.webui-hash"
    fi

    # 2) mosdns：已存在二进制就跳过下载（首次升级或文件丢失才下）
    if [[ -x "${BIN_DIR}/mosdns" ]]; then
        log_info "mosdns 已存在，跳过下载（如需手动升级 mosdns 请直接 rm ${BIN_DIR}/mosdns 后再升级）"
    else
        install_mosdns
    fi

    # 3) Geo 数据：mtime < 7 天就跳过（避免每次升级都下 ~50MB）
    local geo_file="${INSTALL_DIR}/data/geo/geoip.dat"
    if [[ -f "$geo_file" ]] && find "$geo_file" -mtime -7 -print 2>/dev/null | grep -q .; then
        log_info "Geo 数据存在且 7 天内更新过，跳过下载"
    else
        download_geo_data
    fi

    # 4) Xray：官方脚本自带版本检查（看到新版才下载）
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install || true
    # 重写 Xray override.conf（关键：修复 NoNewPrivileges 等配置）
    # 即使 Xray 没升级，老版本的 override.conf 可能用了错误的空值语法导致 TPROXY 失效
    write_xray_override
    # =====================================================================

    # 部署新脚本（保留配置文件）
    cp "${BUILD_DIR}/scripts/"*.sh "${SCRIPTS_DIR}/"
    chmod +x "${SCRIPTS_DIR}"/*.sh
    # install.sh 本身也部署到 INSTALL_DIR
    cp "${BUILD_DIR}/install.sh" "${INSTALL_DIR}/install.sh"
    chmod +x "${INSTALL_DIR}/install.sh"
    cp "${BUILD_DIR}/configs/nftables-tproxy.nft" /etc/nftables.conf
    cp "${BUILD_DIR}/configs/systemd/"*.service /etc/systemd/system/
    cp "${BUILD_DIR}/configs/systemd/"*.timer   /etc/systemd/system/
    systemctl daemon-reload

    # 启用新版本可能引入的 timer（已 enable 的不会重复 enable）
    systemctl enable --now tproxy-gw-flush-ipsets.timer 2>/dev/null || true

    cp "${BUILD_DIR}/VERSION" "${INSTALL_DIR}/VERSION"

    # 重新生成 Xray 和 WG 配置（基于现有 nodes.json / peers.json）
    "${BIN_DIR}/webui" render-xray || true
    "${BIN_DIR}/webui" render-wg || true

    nft -f /etc/nftables.conf
    # 重启顺序很关键：mosdns 必须先于 Xray 启动
    # 否则 Xray 启动时需要解析 VPS 域名，但 mosdns 还没起来 → Xray 解析失败 → 退出
    # 即使 Xray 配置已经预解析了 IP，mosdns 先起也是更安全的顺序
    restart_service tproxy-gw-mosdns
    sleep 2
    restart_service xray
    restart_service tproxy-gw-webui

    # 只在装过编译工具时才卸载（如果跳过了编译就不需要卸载）
    if [[ -n "$new_webui_hash" && "$old_webui_hash" != "$new_webui_hash" ]]; then
        remove_build_toolchain
    fi
    prune_backups 5

    log_done "升级完成（${old_ver} → ${new_ver}）"
}

action_uninstall() {
    require_root
    if ! is_installed; then
        log_warn "未检测到 RProxy 客户端，无需卸载"
        exit 0
    fi
    echo ""
    log_warn "卸载会删除所有配置和数据！"
    confirm=$(ask "确认卸载？(yes/no): ")
    [[ "$confirm" == "yes" ]] || { log_info "已取消"; exit 0; }
    do_uninstall_silent
    log_done "卸载完成"
}

do_uninstall_silent() {
    log_step "停止并禁用服务"
    for svc in tproxy-gw-update.timer tproxy-gw-watchdog.timer tproxy-gw-flush-ipsets.timer \
               tproxy-gw-update.service tproxy-gw-watchdog.service tproxy-gw-flush-ipsets.service \
               tproxy-gw-webui tproxy-gw-mosdns tproxy-gw-iproute \
               wg-quick@wg0 xray nftables; do
        systemctl disable --now "$svc" 2>/dev/null || true
    done

    log_step "清空 nftables 规则"
    nft flush ruleset 2>/dev/null || true

    log_step "卸载 Xray"
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ remove --purge 2>/dev/null || true

    log_step "删除文件"
    rm -rf "${INSTALL_DIR}" "${WWW_DIR}" "${XRAY_CONFIG_DIR}" \
           /etc/wireguard/wg0.conf \
           /etc/systemd/system/tproxy-gw-*.service \
           /etc/systemd/system/tproxy-gw-*.timer \
           /etc/sysctl.d/99-tproxy-*.conf

    systemctl daemon-reload

    # 恢复 resolv.conf
    chattr -i /etc/resolv.conf 2>/dev/null || true
    rm -f /etc/resolv.conf
    if [[ -f /etc/resolv.conf.bak ]]; then
        mv /etc/resolv.conf.bak /etc/resolv.conf
    else
        echo -e "nameserver 223.5.5.5\nnameserver 119.29.29.29" > /etc/resolv.conf
    fi

    # 重启 systemd-resolved（如果还想用）
    # systemctl enable --now systemd-resolved 2>/dev/null || true
}

action_status() {
    echo ""
    if is_installed; then
        echo -e "  RProxy 客户端版本：${GREEN}$(get_installed_version)${NC}"
    else
        echo -e "  ${YELLOW}RProxy 客户端 未安装${NC}"
        return
    fi
    echo ""
    echo "  组件状态："
    for svc in xray tproxy-gw-mosdns tproxy-gw-webui wg-quick@wg0; do
        if systemctl is-active --quiet "$svc"; then
            echo -e "    ${svc}: ${GREEN}运行中${NC}"
        else
            echo -e "    ${svc}: ${RED}未运行${NC}"
        fi
    done
    echo ""
    if [[ -f "${CONFIG_DIR}/webui.json" ]]; then
        local port
        port=$(jq -r '.listen_port' "${CONFIG_DIR}/webui.json")
        local ip
        ip=$(hostname -I | awk '{print $1}')
        echo -e "  WebUI 地址：${CYAN}http://${ip}:${port}${NC}"
    fi
    echo ""
}

action_emergency_stop() {
    require_root
    if [[ -x "${SCRIPTS_DIR}/emergency-stop.sh" ]]; then
        "${SCRIPTS_DIR}/emergency-stop.sh"
    else
        log_error "找不到 emergency-stop.sh"
    fi
}

show_completion_info() {
    local ip
    ip=$(hostname -I | awk '{print $1}')
    local port
    port=$(jq -r '.listen_port' "${CONFIG_DIR}/webui.json")
    local pass=""
    if [[ -f "${CONFIG_DIR}/.initial_password" ]]; then
        pass=$(cat "${CONFIG_DIR}/.initial_password")
    fi

    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                   RProxy 客户端 安装完成！                           ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  WebUI 访问地址：${CYAN}http://${ip}:${port}${NC}"
    echo -e "  用户名：${CYAN}admin${NC}"
    if [[ -n "$pass" ]]; then
        echo -e "  初始密码：${YELLOW}${pass}${NC}"
        echo -e "  ${RED}请登录后立即修改密码！${NC}"
    fi
    echo ""
    echo -e "  下一步："
    echo -e "    1) 在 WebUI 的【节点管理】页面添加你的 VPS 节点"
    echo -e "    2) 选择一个节点作为活动节点"
    echo -e "    3) 把局域网客户端的网关和 DNS 指向 ${CYAN}${ip}${NC}"
    echo ""
    echo -e "  紧急救援：在旁路由 SSH 执行 ${CYAN}${SCRIPTS_DIR}/emergency-stop.sh${NC}"
    echo -e "  可清空所有透明代理规则恢复直连。"
    echo ""
}

# ============================================================================
# 入口
# ============================================================================

main() {
    require_root
    # 第二参数（可选）= 版本号。形式：1.1.3 / v1.1.3 / client-v1.1.3
    # 留空则装最新 release。也可用环境变量 RPROXY_VERSION 传入。
    # 优先级：CLI 参数 > 环境变量
    local ver_arg="${2:-${RPROXY_VERSION:-}}"
    if [[ -n "$ver_arg" ]]; then
        # 由 common.sh 提供的归一化函数 → 设置 RPROXY_TAG 全局变量
        normalize_release_tag "$ver_arg" "client"
        log_info "目标版本：${RPROXY_TAG}"
    fi

    case "${1:-}" in
        install)        action_install_fresh ;;
        upgrade)        action_upgrade ;;
        uninstall)      action_uninstall ;;
        status)         action_status ;;
        emergency-stop) action_emergency_stop ;;
        "")             show_menu ;;
        *)              echo "用法: $0 [install|upgrade|uninstall|status|emergency-stop] [version]"; exit 1 ;;
    esac
}

main "$@"
