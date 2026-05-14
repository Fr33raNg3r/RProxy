#!/bin/bash
# ============================================================================
# RProxy 服务端 安装/升级/卸载脚本
# 适用：Debian 13 x86_64 VPS
# 协议：VMess + WebSocket + TLS + 真网站
# 用法：
#   wget -O- https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/server/install.sh | bash
#   或：bash install.sh [install|upgrade|uninstall|status]
# ============================================================================

set -e

# ---------- 加载公共函数 ----------
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo /tmp)"

if [[ -f "${SELF_DIR}/scripts/common.sh" ]]; then
    source "${SELF_DIR}/scripts/common.sh"
    LOCAL_MODE=1
else
    TMP_COMMON=$(mktemp)
    if ! curl -fsSL "https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/server/scripts/common.sh" -o "${TMP_COMMON}"; then
        echo "[错误] 无法下载 common.sh，请检查网络" >&2
        exit 1
    fi
    source "${TMP_COMMON}"
    rm -f "${TMP_COMMON}"
    LOCAL_MODE=0
fi

# ============================================================================
# 主菜单
# ============================================================================

show_menu() {
    while true; do
        clear
        cat <<EOF
${CYAN}╔═══════════════════════════════════════════════════════════════╗
║          RProxy 服务端 安装/管理脚本                          ║
║          Debian 13 · VMess + WebSocket + TLS + 真网站         ║
╚═══════════════════════════════════════════════════════════════╝${NC}

EOF

        # 远程版本（GitHub）—— 同步显示，超时 5 秒
        local remote_ver
        remote_ver=$(get_remote_version)
        echo -e "  最新版本（GitHub）：${CYAN}${remote_ver}${NC}"

        if is_installed; then
            local local_ver
            local_ver=$(get_installed_version)
            if [[ "$remote_ver" != "获取失败" && "$local_ver" != "$remote_ver" ]]; then
                echo -e "  当前已安装版本：  ${YELLOW}${local_ver}${NC}  ${YELLOW}（有新版可升级）${NC}"
            else
                echo -e "  当前已安装版本：  ${GREEN}${local_ver}${NC}"
            fi
            if is_service_active xray; then
                echo -e "  Xray 状态：       ${GREEN}运行中${NC}"
            else
                echo -e "  Xray 状态：       ${RED}未运行${NC}"
            fi
            if is_service_active nginx; then
                echo -e "  Nginx 状态：      ${GREEN}运行中${NC}"
            else
                echo -e "  Nginx 状态：      ${RED}未运行${NC}"
            fi
        else
            echo -e "  当前状态：        ${YELLOW}未安装${NC}"
        fi
        echo ""
        echo "  请选择操作："
        echo "    1) 全新安装"
        echo "    2) 升级安装"
        echo "    3) 卸载"
        echo "    4) 查看状态"
        echo "    5) 系统优化（内核切换、网络调优）"
        echo "    0) 退出"
        echo ""
        local choice
        choice=$(ask "  请输入选项 [0-5]: ")
        case "$choice" in
            # 终止性操作
            1) action_install_fresh; exit 0 ;;
            2) action_upgrade; exit 0 ;;
            3) action_uninstall; exit 0 ;;
            # 查看/工具类
            4) action_status; press_enter ;;
            5) action_system_optimize ;;
            0) exit 0 ;;
            *) log_error "无效选项"; sleep 1 ;;
        esac
    done
}

press_enter() {
    echo ""
    read -rp "  按回车键返回主菜单..." _ </dev/tty || true
}

action_system_optimize() {
    local opt_script="${INSTALL_DIR}/scripts/system-optimize.sh"
    if [[ -f "$opt_script" ]]; then
        # shellcheck disable=SC1090
        source "$opt_script"
    elif [[ -f "${SELF_DIR}/scripts/system-optimize.sh" ]]; then
        # shellcheck disable=SC1091
        source "${SELF_DIR}/scripts/system-optimize.sh"
    else
        die "找不到 system-optimize.sh"
    fi
    system_optimize_menu
}

# ============================================================================
# 准备工作
# ============================================================================

install_base_packages() {
    log_step "安装基础软件包"
    apt-get update -y
    apt-get install -y --no-install-recommends \
        wget curl ca-certificates \
        nginx jq qrencode socat \
        php-fpm php-cli php-curl \
        unattended-upgrades cron \
        unzip openssl
    log_done "基础包安装完成"
}

setup_unattended_upgrades() {
    log_step "配置自动安全更新"
    cat > /etc/apt/apt.conf.d/50unattended-upgrades <<'EOF'
Unattended-Upgrade::Origins-Pattern {
    "origin=Debian,codename=${distro_codename}-security,label=Debian-Security";
    "origin=Debian,codename=${distro_codename}-updates";
};
Unattended-Upgrade::AutoFixInterruptedDpkg "true";
Unattended-Upgrade::MinimalSteps "true";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
Unattended-Upgrade::Automatic-Reboot "false";
EOF
    cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
EOF
    systemctl enable --now unattended-upgrades
    log_done "自动安全更新已启用（不会自动重启）"
}

# ============================================================================
# SSH 端口与端口占用检查
# ============================================================================

check_ssh_port() {
    local ssh_port
    ssh_port=$(get_ssh_port)
    if [[ "$ssh_port" == "22" ]]; then
        echo ""
        log_warn "检测到 SSH 仍在使用默认端口 22"
        echo "    强烈建议修改 SSH 端口以提升安全性。修改方法："
        echo "      1. 编辑 /etc/ssh/sshd_config，将 #Port 22 改为 Port <新端口>（如 22222）"
        echo "      2. systemctl restart ssh"
        echo "      3. 用新端口重新登录验证"
        echo ""
        local yn
        yn=$(ask "    是否继续安装（保持当前 SSH 端口 22）？(y/N): " "n")
        if [[ ! "$yn" =~ ^[Yy]$ ]]; then
            log_info "已取消，请先修改 SSH 端口后再运行安装"
            exit 0
        fi
    else
        log_info "检测到 SSH 端口为 $ssh_port（非默认，安全性较好）"
    fi
}

check_443_and_80_ports() {
    if is_port_in_use 443; then
        log_error "443 端口已被占用："
        ss -tlnp 2>/dev/null | grep -E ":443\b" | sed 's/^/      /'
        echo "    请先停掉占用 443 的服务（如 nginx/apache/caddy）再安装"
        exit 1
    fi
    if is_port_in_use 80; then
        log_error "80 端口已被占用："
        ss -tlnp 2>/dev/null | grep -E ":80\b" | sed 's/^/      /'
        echo "    请先停掉占用 80 的服务（acme.sh 申请证书需要）"
        exit 1
    fi
    log_info "443、80 端口可用"
}

# ============================================================================
# 域名输入
# ============================================================================

interactive_get_domain() {
    echo ""
    log_step "请输入你的域名"
    echo "    要求："
    echo "    1. 此域名已经解析到本机公网 IP（DNS A 记录指向 $(get_public_ip)）"
    echo "    2. 用于 Let's Encrypt 证书申请和 TLS 伪装"
    echo "    3. 例如：vps.example.com"
    echo ""
    local input_domain=""
    while [[ -z "$input_domain" ]]; do
        input_domain=$(ask "    请输入域名: ")
        # 简单格式检查
        if [[ ! "$input_domain" =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?\.[a-zA-Z]{2,}$ ]]; then
            log_warn "域名格式错误，请重新输入"
            input_domain=""
        fi
    done
    DOMAIN="$input_domain"
    log_info "已设置域名: ${DOMAIN}"
}

# ============================================================================
# 防火墙交互式配置
# ============================================================================

interactive_firewall_setup() {
    echo ""
    log_step "防火墙配置"
    echo "    检测当前监听端口的服务："
    ss -tlnp 2>/dev/null | awk 'NR>1 {print "      " $4 "    " $7}' | sort -u | head -20
    echo ""
    echo "    脚本可以为你部署一套 nftables 防火墙规则。请选择："
    echo "      1) 不动防火墙（推荐：使用云厂商安全组管理）"
    echo "      2) 启用基础防火墙：仅放行 443、80 和 SSH 端口"
    echo "      3) 启用基础防火墙：放行 443、80、SSH 和当前所有监听端口"
    echo ""
    local fw_choice
    fw_choice=$(ask "    请选择 [1-3，默认 1]: " "1")

    case "$fw_choice" in
        2) deploy_firewall_minimal ;;
        3) deploy_firewall_open_existing ;;
        *) log_info "跳过防火墙配置" ;;
    esac
}

deploy_firewall_minimal() {
    local ssh_port
    ssh_port=$(get_ssh_port)
    log_step "部署最小防火墙规则（443、80、SSH:${ssh_port}）"
    apt-get install -y --no-install-recommends nftables
    cat > /etc/nftables.conf <<EOF
#!/usr/sbin/nft -f
flush ruleset
table inet filter {
    chain input {
        type filter hook input priority filter; policy drop;
        iif lo accept
        ct state established,related accept
        ct state invalid drop
        ip protocol icmp accept
        tcp dport 443 accept comment "HTTPS"
        tcp dport 80 accept comment "HTTP/ACME"
        tcp dport ${ssh_port} accept comment "SSH"
    }
    chain forward {
        type filter hook forward priority filter; policy drop;
    }
    chain output {
        type filter hook output priority filter; policy accept;
    }
}
EOF
    systemctl enable --now nftables
    nft -f /etc/nftables.conf
    log_done "防火墙规则已加载"
}

deploy_firewall_open_existing() {
    local ssh_port
    ssh_port=$(get_ssh_port)
    log_step "部署防火墙（含当前所有监听端口）"
    apt-get install -y --no-install-recommends nftables
    local tcp_ports udp_ports
    tcp_ports=$(ss -tlnH 2>/dev/null | awk '{print $4}' | sed 's/.*://' | sort -u | grep -E '^[0-9]+$' | tr '\n' ',' | sed 's/,$//')
    udp_ports=$(ss -ulnH 2>/dev/null | awk '{print $4}' | sed 's/.*://' | sort -u | grep -E '^[0-9]+$' | tr '\n' ',' | sed 's/,$//')
    cat > /etc/nftables.conf <<EOF
#!/usr/sbin/nft -f
flush ruleset
table inet filter {
    chain input {
        type filter hook input priority filter; policy drop;
        iif lo accept
        ct state established,related accept
        ct state invalid drop
        ip protocol icmp accept
        tcp dport 443 accept comment "HTTPS"
        tcp dport 80 accept comment "HTTP/ACME"
        tcp dport ${ssh_port} accept comment "SSH"
EOF
    if [[ -n "$tcp_ports" ]]; then
        echo "        tcp dport { ${tcp_ports} } accept comment \"existing TCP\"" >> /etc/nftables.conf
    fi
    if [[ -n "$udp_ports" ]]; then
        echo "        udp dport { ${udp_ports} } accept comment \"existing UDP\"" >> /etc/nftables.conf
    fi
    cat >> /etc/nftables.conf <<EOF
    }
    chain forward {
        type filter hook forward priority filter; policy drop;
    }
    chain output {
        type filter hook output priority filter; policy accept;
    }
}
EOF
    systemctl enable --now nftables
    nft -f /etc/nftables.conf
    log_done "防火墙规则已加载"
}

# ============================================================================
# 拉取源码 & 安装组件
# ============================================================================

fetch_source() {
    log_step "拉取源码到 ${BUILD_DIR}"
    rm -rf "${BUILD_DIR}"
    if [[ "${LOCAL_MODE}" == "1" && -d "${SELF_DIR}/scripts" && -d "${SELF_DIR}/configs" ]]; then
        log_info "本地模式：从 ${SELF_DIR} 复制源码"
        cp -a "${SELF_DIR}" "${BUILD_DIR}"
    else
        log_info "远程模式：从 GitHub 下载（仅 server 子目录）"
        mkdir -p "${BUILD_DIR}"
        # --strip-components=2 去掉 RProxy-main/server/ 两层
        # 后面的过滤参数让 tar 只解压 server/ 目录下的文件
        if ! curl -fsSL "https://github.com/Fr33raNg3r/RProxy/archive/refs/heads/main.tar.gz" \
                | tar xz -C "${BUILD_DIR}" --strip-components=2 'RProxy-main/server'; then
            die "源码下载失败"
        fi
    fi
    log_done "源码已就绪"
}

install_xray() {
    log_step "通过官方一键脚本安装 Xray-core"
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
    log_done "Xray 安装完成：$(xray version | head -n 1)"
}

# ============================================================================
# 部署文件
# ============================================================================

deploy_files() {
    log_step "部署脚本和配置文件"
    mkdir -p "${INSTALL_DIR}" "${SCRIPTS_DIR}" "${CONFIG_DIR}" "${DATA_DIR}" "${LOG_DIR}"

    cp "${BUILD_DIR}/scripts/common.sh"            "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/update-daemon.sh"     "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/system-optimize.sh"   "${SCRIPTS_DIR}/"
    # install.sh 本身也部署
    cp "${BUILD_DIR}/install.sh" "${INSTALL_DIR}/install.sh"
    chmod +x "${SCRIPTS_DIR}"/*.sh "${INSTALL_DIR}/install.sh"

    cp "${BUILD_DIR}/scripts/tproxy-server" /usr/local/bin/tproxy-server
    chmod +x /usr/local/bin/tproxy-server

    # 复制 nginx 模板到 scripts 目录，方便 tproxy-server change-domain 调用
    cp "${BUILD_DIR}/configs/nginx/tproxy-server.conf.tpl" "${SCRIPTS_DIR}/nginx-template.conf"

    cp "${BUILD_DIR}/configs/systemd/"*.service /etc/systemd/system/ 2>/dev/null || true
    cp "${BUILD_DIR}/configs/systemd/"*.timer   /etc/systemd/system/ 2>/dev/null || true
    systemctl daemon-reload

    cp "${BUILD_DIR}/VERSION" "${INSTALL_DIR}/VERSION"

    log_done "文件部署完成"
}

# ============================================================================
# 部署 LibreSpeed 静态站
# ============================================================================

deploy_librespeed() {
    log_step "部署 LibreSpeed 测速网站"
    mkdir -p "${WEB_ROOT}"
    cp -a "${BUILD_DIR}/configs/librespeed/"* "${WEB_ROOT}/"
    chown -R www-data:www-data "${WEB_ROOT}"
    log_done "LibreSpeed 已部署到 ${WEB_ROOT}"
}

# ============================================================================
# 申请 Let's Encrypt 证书（acme.sh + HTTP-01 webroot）
# ============================================================================

deploy_temp_nginx_for_acme() {
    # 临时部署一个最小 nginx 配置，让 acme.sh 用 webroot 验证
    log_step "部署临时 Nginx 配置以申请证书"
    rm -f /etc/nginx/sites-enabled/default
    cat > "${NGINX_CONF}" <<EOF
server {
    listen 80;
    server_name ${DOMAIN};
    root ${WEB_ROOT};
    index index.html;

    # acme.sh webroot 验证用
    location /.well-known/acme-challenge/ {
        root ${WEB_ROOT};
    }
}
EOF
    nginx -t && systemctl restart nginx
    log_done "临时 Nginx 已启动"
}

acquire_ssl_cert() {
    log_step "申请 Let's Encrypt 证书"
    mkdir -p "${SSL_DIR}"
    if [[ ! -d "${ACME_HOME}" ]]; then
        log_info "安装 acme.sh"
        curl -fsSL https://get.acme.sh | sh -s email=admin@${DOMAIN}
    fi

    "${ACME_HOME}/acme.sh" --upgrade --auto-upgrade
    "${ACME_HOME}/acme.sh" --set-default-ca --server letsencrypt

    log_info "通过 webroot 验证申请 ECC 证书"
    if ! "${ACME_HOME}/acme.sh" --issue \
            -d "${DOMAIN}" \
            -w "${WEB_ROOT}" \
            -k ec-256 \
            --force; then
        log_error "证书申请失败。可能原因："
        echo "      1. 域名未正确解析到本机 IP（${PUBLIC_IP}）"
        echo "      2. 80 端口被占用或防火墙阻挡"
        echo "      3. Let's Encrypt 速率限制"
        exit 1
    fi

    "${ACME_HOME}/acme.sh" --installcert -d "${DOMAIN}" \
        --ecc \
        --key-file       "${SSL_DIR}/server.key" \
        --fullchain-file "${SSL_DIR}/server.crt" \
        --reloadcmd      "systemctl reload nginx"

    chmod 600 "${SSL_DIR}/server.key"
    log_done "证书申请并部署成功"
}

# ============================================================================
# 渲染最终 Nginx 配置（含 HTTPS + WS 反代 + 真网站）
# ============================================================================

deploy_final_nginx() {
    log_step "部署最终 Nginx 配置"
    local ws_path
    ws_path=$(json_get "${SERVER_CFG}" '.ws_path')

    # 用 sed 替换占位符
    sed -e "s|{{DOMAIN}}|${DOMAIN}|g" \
        -e "s|{{WS_PATH}}|${ws_path}|g" \
        -e "s|{{WEB_ROOT}}|${WEB_ROOT}|g" \
        -e "s|{{SSL_DIR}}|${SSL_DIR}|g" \
        -e "s|{{XRAY_PORT}}|${XRAY_LOCAL_PORT}|g" \
        "${BUILD_DIR}/configs/nginx/tproxy-server.conf.tpl" \
        > "${NGINX_CONF}"

    # PHP 版本检测，调整 socket 路径
    local php_sock
    php_sock=$(ls /run/php/php*-fpm.sock 2>/dev/null | head -1)
    if [[ -n "$php_sock" ]]; then
        sed -i "s|{{PHP_SOCK}}|${php_sock}|g" "${NGINX_CONF}"
    fi

    if ! nginx -t; then
        log_error "Nginx 配置语法错误"
        nginx -t
        exit 1
    fi
    systemctl reload nginx
    log_done "Nginx 已加载新配置"
}

# ============================================================================
# 生成服务端核心配置 + Xray 配置
# ============================================================================

generate_server_config() {
    log_step "生成服务端配置"
    local uuid ws_path
    uuid=$(xray uuid)
    ws_path="/$(openssl rand -hex 8)"

    cat > "${SERVER_CFG}" <<EOF
{
  "domain":     "${DOMAIN}",
  "uuid":       "${uuid}",
  "ws_path":    "${ws_path}",
  "alter_id":   0,
  "security":   "auto"
}
EOF
    chmod 600 "${SERVER_CFG}"
    log_done "服务端配置已生成"
}

render_xray_config() {
    log_step "渲染 Xray 配置"
    /usr/local/bin/tproxy-server _render-xray
    log_done "Xray 配置已生成"
}

start_services() {
    log_step "启动服务"
    systemctl enable xray
    # 必须用 restart 而不是 start——Xray 安装脚本可能已经启动了 Xray
    # 用空的默认配置，需要 restart 才能加载我们渲染的配置
    systemctl restart xray
    systemctl reload nginx
    sleep 2
    if ! is_service_active xray; then
        log_error "Xray 启动失败，查看日志:"
        journalctl -u xray -n 30 --no-pager
        exit 1
    fi
    if ! is_service_active nginx; then
        log_error "Nginx 启动失败，查看日志:"
        journalctl -u nginx -n 30 --no-pager
        exit 1
    fi
    log_done "服务已启动"
}

enable_auto_update() {
    log_step "启用每日自动更新（脚本+Xray+证书续期）"
    systemctl enable --now tproxy-server-update.timer
    log_done "自动更新已启用（每日 04:05 检查）"
}

# ============================================================================
# 安装 / 升级 / 卸载主流程
# ============================================================================

action_install_fresh() {
    require_root
    check_arch
    check_debian13

    if is_installed; then
        echo ""
        log_warn "检测到已有安装，全新安装会清除所有现有配置！"
        local confirm
        confirm=$(ask "确认继续？(yes/no): " "no")
        [[ "$confirm" == "yes" ]] || { log_info "已取消"; exit 0; }
        do_uninstall_silent
    fi

    PUBLIC_IP=$(get_public_ip)
    log_info "本机公网 IP：${PUBLIC_IP:-未知}"

    check_443_and_80_ports
    check_ssh_port
    interactive_get_domain
    interactive_firewall_setup

    install_base_packages
    setup_unattended_upgrades

    fetch_source
    install_xray

    deploy_files
    deploy_librespeed
    generate_server_config

    deploy_temp_nginx_for_acme
    acquire_ssl_cert

    render_xray_config
    deploy_final_nginx
    start_services

    enable_auto_update

    show_completion_info
}

action_upgrade() {
    require_root
    check_arch
    check_debian13
    is_installed || die "未检测到已有安装，请先全新安装"

    log_info "开始升级"
    fetch_source

    local old_ver new_ver
    old_ver=$(cat "${INSTALL_DIR}/VERSION")
    new_ver=$(cat "${BUILD_DIR}/VERSION")
    log_info "本地版本：${old_ver}    最新版本：${new_ver}"

    # 升级 Xray
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install || true

    # 部署新脚本（保留配置）
    cp "${BUILD_DIR}/scripts/"*.sh "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/tproxy-server" /usr/local/bin/tproxy-server
    chmod +x "${SCRIPTS_DIR}"/*.sh /usr/local/bin/tproxy-server
    cp "${BUILD_DIR}/configs/systemd/"*.service /etc/systemd/system/ 2>/dev/null || true
    cp "${BUILD_DIR}/configs/systemd/"*.timer   /etc/systemd/system/ 2>/dev/null || true
    systemctl daemon-reload
    cp "${BUILD_DIR}/VERSION" "${INSTALL_DIR}/VERSION"

    # 重新部署 LibreSpeed（PHP 版本可能变了）
    deploy_librespeed

    # 重新渲染 nginx 和 xray 配置（防止模板有更新）
    DOMAIN=$(json_get "${SERVER_CFG}" '.domain')
    deploy_final_nginx
    render_xray_config

    restart_service xray
    systemctl reload nginx

    log_done "升级完成（${old_ver} → ${new_ver}）"
}

action_uninstall() {
    require_root
    if ! is_installed; then
        log_warn "未检测到 RProxy 服务端，无需卸载"
        exit 0
    fi
    echo ""
    log_warn "卸载会删除所有配置和证书！"
    local confirm
    confirm=$(ask "确认卸载？(yes/no): " "no")
    [[ "$confirm" == "yes" ]] || { log_info "已取消"; exit 0; }
    do_uninstall_silent
    log_done "卸载完成"
}

do_uninstall_silent() {
    log_step "停止并禁用服务"
    for svc in tproxy-server-update.timer tproxy-server-update.service xray; do
        systemctl disable --now "$svc" 2>/dev/null || true
    done

    log_step "卸载 Xray"
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ remove --purge 2>/dev/null || true

    log_step "卸载 acme.sh"
    if [[ -d "${ACME_HOME}" ]]; then
        "${ACME_HOME}/acme.sh" --uninstall 2>/dev/null || true
        rm -rf "${ACME_HOME}"
    fi

    log_step "删除 Nginx 站点配置"
    rm -f "${NGINX_CONF}"
    systemctl reload nginx 2>/dev/null || true

    log_step "删除文件"
    rm -rf "${INSTALL_DIR}" "${SSL_DIR}" "${WEB_ROOT}" \
           /usr/local/bin/tproxy-server \
           /etc/systemd/system/tproxy-server-*.service \
           /etc/systemd/system/tproxy-server-*.timer

    systemctl daemon-reload

    log_info "Nginx 与 PHP-FPM 等基础包未自动卸载（可能其他用途在使用）"
    log_info "防火墙规则未清除（保留 SSH 安全）。如需清除：nft flush ruleset"
}

action_status() {
    if command -v tproxy-server &>/dev/null; then
        tproxy-server status
    else
        log_warn "tproxy-server 命令不可用，可能未完成安装"
    fi
}

# ============================================================================
# 完成提示
# ============================================================================

show_completion_info() {
    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║              RProxy 服务端 安装完成！                         ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    /usr/local/bin/tproxy-server show
    echo ""
    echo -e "  ${CYAN}测试网站访问：${NC}https://${DOMAIN}"
    echo -e "  ${CYAN}访客看到的是 LibreSpeed 测速网站${NC}"
    echo ""
    echo -e "  ${CYAN}常用命令：${NC}"
    echo "    tproxy-server                显示菜单"
    echo "    tproxy-server status         查看状态"
    echo "    tproxy-server show           显示客户端配置"
    echo "    tproxy-server help           完整帮助"
    echo ""
}

# ============================================================================
# 入口
# ============================================================================

main() {
    require_root
    case "${1:-}" in
        install)   action_install_fresh ;;
        upgrade)   action_upgrade ;;
        uninstall) action_uninstall ;;
        status)    action_status ;;
        "")        show_menu ;;
        *)         echo "用法: $0 [install|upgrade|uninstall|status]"; exit 1 ;;
    esac
}

main "$@"
