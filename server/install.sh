#!/bin/bash
# ============================================================================
# RProxy 服务端 安装/升级/卸载脚本
# 适用：Debian 13 x86_64 VPS
# 协议：VLESS + XHTTP + TLS + 真网站
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
# 清理 1.4.7 / 1.4.8 改名迁移留下的兼容软链接和迁移前备份包
# ============================================================================
cleanup_legacy_tproxy() {
    # 1.5.0 起不再提供迁移：仍是未迁移的旧布局时拒绝升级，避免装出半新半旧的系统
    if [[ -d /opt/tproxy-server && ! -L /opt/tproxy-server ]]; then
        die "检测到未迁移的旧版安装（/opt/tproxy-server），请先用 1.4.8 的脚本完成迁移后再升级：
    bash <(curl -fsSL https://raw.githubusercontent.com/${REPO_SLUG}/v1.4.8/server/install.sh) upgrade 1.4.8"
    fi
    # 删证书目录软链接前再确认一次 acme.sh 续期路径已指向新目录，否则续期会写到不存在的路径
    grep -rIl 'tproxy-server' "${ACME_HOME}" --include='*.conf' 2>/dev/null \
        | xargs -r sed -i -e "s#/var/www/tproxy-server#${WEB_ROOT}#g" -e "s#/etc/ssl/tproxy-server#${SSL_DIR}#g"
    local p
    for p in /opt/tproxy-server /var/www/tproxy-server /etc/ssl/tproxy-server /usr/local/bin/tproxy-server; do
        if [[ -L "$p" ]]; then rm -f "$p"; fi
    done
    rm -f /var/backups/tproxy-server-pre-rproxy-*.tar.gz
    # 旧定时器（Persistent=true）留下的 stamp 记录，以及旧版升级用过的临时构建目录
    rm -f /var/lib/systemd/timers/stamp-tproxy-server-*.timer
    rm -rf /tmp/tproxy-server-build
}

# ============================================================================
# 主菜单
# ============================================================================

show_menu() {
    while true; do
        clear
        cat <<EOF
${CYAN}╔═══════════════════════════════════════════════════════════════╗
║          RProxy 服务端 安装/管理脚本                          ║
║          Debian 13 · VLESS + XHTTP + TLS + 真网站             ║
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
    log_info "升级系统已有软件包（可能需要几分钟）"
    DEBIAN_FRONTEND=noninteractive apt-get -y \
        -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold \
        full-upgrade || die "系统升级失败"
    # 内核有更新时当前仍运行旧内核，完成页提示重启
    local newest_kernel
    newest_kernel=$(ls -v /boot/vmlinuz-* 2>/dev/null | tail -1 || true)
    if [[ -n "$newest_kernel" && "$newest_kernel" != "/boot/vmlinuz-$(uname -r)" ]]; then
        NEED_REBOOT=1
    fi
    apt-get install -y --no-install-recommends \
        wget curl ca-certificates \
        nginx jq qrencode socat \
        php-fpm php-cli php-curl \
        unattended-upgrades cron \
        unzip openssl
    log_done "基础包安装完成"
}

enable_ntp() {
    log_step "启用 NTP（chrony）"
    # chrony 与 systemd-timesyncd 互斥，安装时 apt 会自动卸载后者
    systemctl disable --now ntpsec 2>/dev/null || true
    # 升级流程也会走到这里：系统自动更新（unattended-upgrades 等）可能正占着 dpkg 锁，最多等 5 分钟
    apt-get install -y --no-install-recommends -o DPkg::Lock::Timeout=300 chrony
    # 海外时间源；注释掉 Debian 默认 pool，避免混入其他源
    mkdir -p /etc/chrony/sources.d
    cat > /etc/chrony/sources.d/rproxy.sources <<'EOF'
server pool.ntp.org iburst
server time.cloudflare.com iburst
server time.nist.gov iburst
EOF
    sed -i 's/^pool /# pool /' /etc/chrony/chrony.conf
    systemctl enable chrony 2>/dev/null || true
    systemctl restart chrony
    log_done "NTP 已启用"
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
    log_step "准备服务端 build 目录 ${BUILD_DIR}"
    rm -rf "${BUILD_DIR}"
    mkdir -p "${BUILD_DIR}"

    # 本地开发模式：SELF_DIR 自带 scripts + configs（仓库工作区）
    if [[ "${LOCAL_MODE}" == "1" && -d "${SELF_DIR}/scripts" && -d "${SELF_DIR}/configs" ]]; then
        log_info "本地模式：从 ${SELF_DIR} 复制"
        cp -a "${SELF_DIR}/." "${BUILD_DIR}/"
        log_done "Build 目录已就绪"
        return
    fi

    # 远程模式：下载 release asset
    local tag
    if [[ -n "${RPROXY_TAG:-}" ]]; then
        tag="${RPROXY_TAG}"
        log_info "远程模式：使用指定 tag ${tag}"
    else
        tag=$(get_latest_release_tag || true)
        [[ -n "$tag" ]] || die "未能查询到 release —— 请检查网络或在 GitHub 上确认已存在 vX.Y.Z 发布"
        log_info "远程模式：使用最新 release ${tag}"
    fi
    local version="${tag#v}"
    local asset="server-v${version}.tar.gz"
    local url="https://github.com/${REPO_SLUG}/releases/download/${tag}/${asset}"

    log_info "下载 release 产物：${url}"
    if ! curl -fsSL "$url" | tar xz -C "${BUILD_DIR}"; then
        die "release 产物下载失败: ${url}
（若该 tag 是从旧版 install.sh 创建的、不含 server 产物，请改用更新的 tag）"
    fi
    [[ -f "${BUILD_DIR}/install.sh" ]] || die "下载的产物缺 install.sh，包可能损坏"
    log_done "release 产物已展开到 ${BUILD_DIR}"
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

    cp "${BUILD_DIR}/scripts/rproxy-server" /usr/local/bin/rproxy-server
    chmod +x /usr/local/bin/rproxy-server
    # 本机快照、回滚与自检工具（升级时会先用它打快照）
    install -m 755 "${BUILD_DIR}/scripts/rproxy-rollback" /usr/local/sbin/rproxy-rollback

    # 复制 nginx 模板到 scripts 目录，方便 rproxy-server change-domain 调用
    cp "${BUILD_DIR}/configs/nginx/rproxy-server.conf.tpl" "${SCRIPTS_DIR}/nginx-template.conf"

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
    # 目录有内容但缺少测速后端文件：视为用户自定义站点，跳过部署避免覆盖
    # 目录为空或含 empty.php + garbage.php（LibreSpeed）则照常部署/更新
    if [[ -n "$(ls -A "${WEB_ROOT}" 2>/dev/null)" ]]; then
        if [[ ! -f "${WEB_ROOT}/empty.php" || ! -f "${WEB_ROOT}/garbage.php" ]]; then
            log_info "检测到自定义站点（${WEB_ROOT}），跳过 LibreSpeed 部署"
            return 0
        fi
    fi
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
# 渲染最终 Nginx 配置（含 HTTPS + XHTTP 反代 + 真网站）
# ============================================================================

deploy_final_nginx() {
    log_step "部署最终 Nginx 配置"
    local ws_path
    ws_path=$(json_get "${SERVER_CFG}" '.ws_path')

    # 先备份：nginx -t 不通过时要恢复原配置，不能把坏配置留在磁盘上
    # （运行中的 nginx 当下不受影响，但下次 reload / 证书续期 / 重启服务器就起不来）
    # .bak 后缀不匹配 conf.d/*.conf，不会被 nginx 加载
    local bak="${NGINX_CONF}.bak"
    rm -f "$bak"
    [[ -f "${NGINX_CONF}" ]] && cp -f "${NGINX_CONF}" "$bak"

    # 用 sed 替换占位符
    sed -e "s|{{DOMAIN}}|${DOMAIN}|g" \
        -e "s|{{WS_PATH}}|${ws_path}|g" \
        -e "s|{{WEB_ROOT}}|${WEB_ROOT}|g" \
        -e "s|{{SSL_DIR}}|${SSL_DIR}|g" \
        "${BUILD_DIR}/configs/nginx/rproxy-server.conf.tpl" \
        > "${NGINX_CONF}"

    # PHP 版本检测，调整 socket 路径
    local php_sock
    php_sock=$(ls /run/php/php*-fpm.sock 2>/dev/null | head -1)
    if [[ -n "$php_sock" ]]; then
        sed -i "s|{{PHP_SOCK}}|${php_sock}|g" "${NGINX_CONF}"
    fi

    if ! nginx -t; then
        log_error "Nginx 配置语法错误"
        if [[ -f "$bak" ]]; then
            mv -f "$bak" "${NGINX_CONF}"
            log_warn "已恢复为原 Nginx 配置"
        fi
        exit 1
    fi
    rm -f "$bak"
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
  "ws_path":    "${ws_path}"
}
EOF
    chmod 600 "${SERVER_CFG}"
    log_done "服务端配置已生成"
}

render_xray_config() {
    log_step "渲染 Xray 配置"
    /usr/local/bin/rproxy-server _render-xray
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
    systemctl enable --now rproxy-server-update.timer
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
    enable_ntp

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

# ---------- 升级保护：本机快照 + 自检 + 出问题自动回滚（1.7.0）----------
# 这些函数放在 install.sh 而不是 common.sh：rproxy-server upgrade 会 source 本机旧版 common.sh。
readonly ROLLBACK_BIN="/usr/local/sbin/rproxy-rollback"
readonly HOLD_RPROXY_FILE="/var/lib/rproxy-server/hold-rproxy-version"
UPGRADE_SNAPSHOT=""   # 升级前快照名；非空表示"中途失败要自动回滚"

# 升级后本机自检（含 XHTTP）：服务刚重启需要几秒，最多试 3 次
upgrade_check_ok() {
    local i
    for i in 1 2 3; do
        "$ROLLBACK_BIN" check --xhttp && return 0
        sleep 5
    done
    return 1
}

# upgrade_auto_rollback <原因> <快照名>
upgrade_auto_rollback() {
    log_error "$1，自动回滚到升级前快照 $2"
    "$ROLLBACK_BIN" --yes --reason "$1" "$2" || log_error "自动回滚失败，请执行 rproxy-rollback 手动恢复"
}

# 升级中途任何失败（die / set -e）都会走到这里
upgrade_exit_trap() {
    local rc=$?
    [[ $rc -ne 0 && -n "$UPGRADE_SNAPSHOT" ]] || return 0
    local snap="$UPGRADE_SNAPSHOT"
    UPGRADE_SNAPSHOT=""
    upgrade_auto_rollback "升级过程出错（退出码 ${rc}）" "$snap"
}

action_upgrade() {
    require_root
    check_arch
    check_debian13
    cleanup_legacy_tproxy
    is_installed || die "未检测到已有安装，请先全新安装"

    log_info "开始升级"
    # 升级时也确保 NTP 启用（旧版安装可能未开启）
    enable_ntp
    fetch_source

    local old_ver new_ver
    old_ver=$(cat "${INSTALL_DIR}/VERSION")
    new_ver=$(cat "${BUILD_DIR}/VERSION")
    log_info "本地版本：${old_ver}    最新版本：${new_ver}"

    # 部署回滚工具并打本机快照：从这里起，任何失败都能回到升级前
    install -m 755 "${BUILD_DIR}/scripts/rproxy-rollback" "$ROLLBACK_BIN"
    local pre_ok=0
    "$ROLLBACK_BIN" check && pre_ok=1
    log_step "创建升级前快照（出问题时可执行 rproxy-rollback 恢复）"
    UPGRADE_SNAPSHOT=$("$ROLLBACK_BIN" create "升级前 ${old_ver} → ${new_ver}") \
        || die "快照创建失败（多半是磁盘空间不足），为安全起见取消升级，系统未做改动"
    log_done "快照：${UPGRADE_SNAPSHOT}"
    trap upgrade_exit_trap EXIT

    # 升级 Xray
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install || true

    # 部署新脚本（保留配置）
    cp "${BUILD_DIR}/scripts/"*.sh "${SCRIPTS_DIR}/"
    cp "${BUILD_DIR}/scripts/rproxy-server" /usr/local/bin/rproxy-server
    # install.sh 本身也更新：rproxy-server upgrade 优先运行本机这份，不更新就会一直停在首次安装时的版本
    cp "${BUILD_DIR}/install.sh" "${INSTALL_DIR}/install.sh"
    chmod +x "${SCRIPTS_DIR}"/*.sh /usr/local/bin/rproxy-server "${INSTALL_DIR}/install.sh"
    cp "${BUILD_DIR}/configs/systemd/"*.service /etc/systemd/system/ 2>/dev/null || true
    cp "${BUILD_DIR}/configs/systemd/"*.timer   /etc/systemd/system/ 2>/dev/null || true
    systemctl daemon-reload
    # nginx 模板也要更新：rproxy-server change-domain / regenerate 用的是这份副本
    cp "${BUILD_DIR}/configs/nginx/rproxy-server.conf.tpl" "${SCRIPTS_DIR}/nginx-template.conf"
    cp "${BUILD_DIR}/VERSION" "${INSTALL_DIR}/VERSION"

    # 重新部署 LibreSpeed（PHP 版本可能变了）
    deploy_librespeed

    # 重新渲染 nginx 和 xray 配置（防止模板有更新）
    DOMAIN=$(json_get "${SERVER_CFG}" '.domain')
    deploy_final_nginx
    render_xray_config

    restart_service xray
    systemctl reload nginx

    # 升级前自检通过、升级后（含 XHTTP）不通过 → 整体回滚到升级前快照
    local snap="$UPGRADE_SNAPSHOT"
    UPGRADE_SNAPSHOT=""   # 后续不再由退出陷阱回滚
    if [[ $pre_ok -eq 1 ]] && ! upgrade_check_ok; then
        upgrade_auto_rollback "升级到 ${new_ver} 后本机自检未通过" "$snap"
        exit 1
    fi
    rm -f "$HOLD_RPROXY_FILE"

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
    for svc in rproxy-server-update.timer rproxy-server-update.service xray; do
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
           /usr/local/bin/rproxy-server \
           /usr/local/sbin/rproxy-rollback /var/lib/rproxy-server \
           /etc/systemd/system/rproxy-server-*.service \
           /etc/systemd/system/rproxy-server-*.timer

    systemctl daemon-reload

    log_info "Nginx 与 PHP-FPM 等基础包未自动卸载（可能其他用途在使用）"
    log_info "防火墙规则未清除（保留 SSH 安全）。如需清除：nft flush ruleset"
}

action_status() {
    if command -v rproxy-server &>/dev/null; then
        rproxy-server status
    else
        log_warn "rproxy-server 命令不可用，可能未完成安装"
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
    /usr/local/bin/rproxy-server show
    echo ""
    echo -e "  ${CYAN}测试网站访问：${NC}https://${DOMAIN}"
    echo -e "  ${CYAN}访客看到的是 LibreSpeed 测速网站${NC}"
    echo ""
    echo -e "  ${CYAN}常用命令：${NC}"
    echo "    rproxy-server                显示菜单"
    echo "    rproxy-server status         查看状态"
    echo "    rproxy-server show           显示客户端配置"
    echo "    rproxy-server help           完整帮助"
    echo ""
    if [[ "${NEED_REBOOT:-0}" == "1" ]]; then
        echo -e "  ${YELLOW}系统内核已升级，建议执行 reboot 重启后生效。${NC}"
        echo ""
    fi
}

# ============================================================================
# 入口
# ============================================================================

main() {
    require_root
    # 第二参数（可选）= 版本号。形式：1.1.4 / v1.1.4
    # 留空则装最新 release。也可用环境变量 RPROXY_VERSION 传入。
    local ver_arg="${2:-${RPROXY_VERSION:-}}"
    if [[ -n "$ver_arg" ]]; then
        normalize_release_tag "$ver_arg"
        log_info "目标版本：${RPROXY_TAG}"
    fi

    case "${1:-}" in
        install)   action_install_fresh ;;
        upgrade)   action_upgrade ;;
        uninstall) action_uninstall ;;
        status)    action_status ;;
        "")        show_menu ;;
        *)         echo "用法: $0 [install|upgrade|uninstall|status] [version]"; exit 1 ;;
    esac
}

main "$@"
