#!/bin/bash
# ============================================================================
# RProxy 服务端 - 系统优化脚本
# 路径: /opt/tproxy-gw/scripts/system-optimize.sh
# 由 install.sh 通过 source 调用
# ============================================================================

# 这个脚本依赖 common.sh 提供的 log_info / log_done / die / ask 等函数
# install.sh 在 source 本脚本前应已 source common.sh

# 服务端 sysctl 配置（VPS 场景，不需要 conntrack 调大）
RPROXY_SYSCTL_FILE="/etc/sysctl.d/99-rproxy-server.conf"

# ---------------- 工具函数 ----------------

current_kernel() {
    uname -r
}

kernel_source() {
    if uname -r | grep -q "zabbly"; then
        echo "Zabbly 主线内核"
    else
        echo "Debian 默认"
    fi
}

# ---------------- Zabbly 内核切换 ----------------

preflight_check() {
    # OS 检查
    if ! grep -q "VERSION_ID=\"13\"" /etc/os-release; then
        log_error "Zabbly 内核切换当前仅支持 Debian 13 (Trixie)"
        return 1
    fi
    # 架构
    if [[ "$(dpkg --print-architecture)" != "amd64" ]]; then
        log_error "仅支持 amd64 架构"
        return 1
    fi
    # /boot 空间
    local boot_avail
    boot_avail=$(df -m /boot 2>/dev/null | awk 'NR==2 {print $4}')
    if [[ -z "$boot_avail" || $boot_avail -lt 500 ]]; then
        log_error "/boot 分区可用空间不足 500MB（当前 ${boot_avail:-?}MB）"
        return 1
    fi
    # 当前已经是 Zabbly?
    if uname -r | grep -q "zabbly"; then
        log_warn "当前已经是 Zabbly 内核：$(uname -r)"
        return 1
    fi
    # GRUB 存在
    if [[ ! -d /boot/grub ]]; then
        log_warn "未检测到 GRUB（可能用了 systemd-boot 或其他引导器）"
        log_warn "强烈建议在能物理访问或有 Console 的环境下进行"
    fi
    return 0
}

install_zabbly_kernel() {
    log_step "添加 Zabbly 仓库（apt key + sources）"
    mkdir -p /etc/apt/keyrings/
    if ! curl -fsSL --max-time 15 https://pkgs.zabbly.com/key.asc -o /etc/apt/keyrings/zabbly.asc; then
        die "无法下载 Zabbly GPG 公钥（pkgs.zabbly.com 不可达？）"
    fi

    local codename
    codename=$(. /etc/os-release && echo "$VERSION_CODENAME")

    cat > /etc/apt/sources.list.d/zabbly-kernel-stable.sources <<EOF
Enabled: yes
Types: deb
URIs: https://pkgs.zabbly.com/kernel/stable
Suites: ${codename}
Components: main
Architectures: amd64
Signed-By: /etc/apt/keyrings/zabbly.asc
EOF

    log_step "刷新软件包列表"
    apt-get update -y || die "apt-get update 失败"

    log_step "安装 Zabbly 主线内核（约 100MB 下载）"
    apt-get install -y linux-zabbly || die "Zabbly 内核安装失败"

    log_step "调整 GRUB 配置（5 秒倒计时 + 显示菜单）"
    sed -i 's/^GRUB_TIMEOUT=.*/GRUB_TIMEOUT=5/' /etc/default/grub
    sed -i 's/^GRUB_TIMEOUT_STYLE=.*/GRUB_TIMEOUT_STYLE=menu/' /etc/default/grub
    update-grub
    log_done "Zabbly 内核已安装"
}

action_install_zabbly() {
    log_step "切换到 Zabbly 主线内核"
    echo ""
    echo -e "${YELLOW}========================================================================${NC}"
    echo -e "${YELLOW}                    ⚠️  Zabbly 内核切换警告${NC}"
    echo -e "${YELLOW}========================================================================${NC}"
    cat <<'EOF'

  即将安装 Zabbly 主线内核仓库提供的最新稳定内核。

  好处：
    ✓ 内核更新（通常 6.11+，比 Debian 13 默认 6.1 新）
    ✓ 内置 BBRv3 拥塞控制算法
    ✓ MPTCP / 更新的 TLS / TCP 优化

  风险：
    ✗ 新内核可能与某些硬件驱动不兼容，启动失败
    ✗ Zabbly 是第三方仓库，长期维护性依赖上游
    ✗ 部分 VPS 厂商不允许自定义内核

  保护措施：
    → 安装过程不删除现有 Debian 内核（GRUB 中保留 fallback）
    → GRUB 倒计时改为 5 秒 + menu 模式（启动失败可手动选旧内核）

  必须自行确认：
    □ 我能物理访问这台机器或有 IPMI/Console
    □ 我理解切换失败时如何用 GRUB 启动旧内核

EOF
    echo -e "${YELLOW}========================================================================${NC}"
    echo ""
    local ans
    ans=$(ask "  确认继续？(yes/no): ")
    [[ "$ans" != "yes" ]] && { log_info "已取消"; return 0; }

    preflight_check || { log_error "前置检查失败"; return 1; }
    install_zabbly_kernel

    echo ""
    echo -e "${GREEN}========================================================================${NC}"
    echo -e "${GREEN}                    ✅ Zabbly 内核安装完成${NC}"
    echo -e "${GREEN}========================================================================${NC}"
    echo ""
    echo "  下一步：重启系统以加载新内核"
    echo ""
    echo "    sudo reboot"
    echo ""
    echo "  重启后验证："
    echo "    uname -r            # 应该看到带 'zabbly' 的版本号"
    echo "    sysctl net.ipv4.tcp_congestion_control"
    echo ""
    echo -e "${YELLOW}  如果启动失败：${NC}"
    echo "    1) 开机时按 [Esc] 或 [Shift]，进入 GRUB 菜单"
    echo "    2) 选 'Advanced options for Debian'"
    echo "    3) 选不带 'zabbly' 的旧内核启动"
    echo "    4) 进系统后在「系统优化」菜单选「回滚到 Debian 默认内核」"
    echo ""
    echo -e "${GREEN}========================================================================${NC}"
    echo ""
    ans=$(ask "  现在重启？(yes/no): ")
    if [[ "$ans" == "yes" ]]; then
        log_info "5 秒后重启..."
        sleep 5
        reboot
    else
        log_info "稍后请手动 reboot"
    fi
}

# ---------------- 回滚到 Debian 默认内核 ----------------

action_rollback_kernel() {
    log_step "回滚到 Debian 默认内核"

    if ! ls /etc/apt/sources.list.d/zabbly-*.sources 2>/dev/null | grep -q .; then
        log_warn "未检测到 Zabbly 仓库，可能已经回滚或从未安装"
        return 0
    fi

    echo ""
    echo "  即将进行：删除 Zabbly 仓库 + 卸载 Zabbly 内核包 + 切换 GRUB 默认入口"
    echo ""
    local ans
    ans=$(ask "  确认继续？(yes/no): ")
    [[ "$ans" != "yes" ]] && { log_info "已取消"; return 0; }

    log_step "查找 Debian 默认内核包"
    local debian_kernel
    debian_kernel=$(dpkg -l 'linux-image-*-amd64' 2>/dev/null | awk '/^ii/{print $2}' | grep -v zabbly | tail -1)
    if [[ -z "$debian_kernel" ]]; then
        die "未找到 Debian 默认内核包，请先安装：apt install linux-image-amd64"
    fi
    log_info "找到 Debian 内核：$debian_kernel"

    log_step "卸载 Zabbly 内核包"
    apt-get purge -y 'linux-image-*zabbly*' linux-zabbly 2>/dev/null || true

    log_step "删除 Zabbly 仓库"
    rm -f /etc/apt/sources.list.d/zabbly-kernel-stable.sources
    rm -f /etc/apt/keyrings/zabbly.asc

    log_step "更新 GRUB"
    update-grub

    log_step "刷新 apt 列表"
    apt-get update -y

    log_done "回滚完成"
    echo ""
    echo "  下一步：sudo reboot 重启"
    echo ""
    ans=$(ask "  现在重启？(yes/no): ")
    if [[ "$ans" == "yes" ]]; then
        log_info "5 秒后重启..."
        sleep 5
        reboot
    fi
}

# ---------------- 网络参数调优 ----------------

action_apply_sysctl() {
    log_step "应用网络参数调优"

    cat > "${RPROXY_SYSCTL_FILE}" <<'EOF'
# ============================================================================
# RProxy 服务端（VPS）网络参数调优
# 由系统优化菜单生成，可手动 rm 删除恢复默认
# ============================================================================

# ---- TCP 拥塞控制：BBR ----
# fq qdisc 是 BBR 的最佳搭档（按流公平队列）
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr

# ---- TCP Fast Open ----
# 3 = 同时支持作为客户端和服务端使用 TFO
net.ipv4.tcp_fastopen = 3

# ---- 内核缓冲区 ----
# 提升大带宽/高延迟链路的吞吐
net.core.rmem_max = 67108864
net.core.wmem_max = 67108864
net.core.rmem_default = 1048576
net.core.wmem_default = 1048576
net.core.netdev_max_backlog = 30000

# TCP 缓冲区动态调整
net.ipv4.tcp_rmem = 4096 87380 67108864
net.ipv4.tcp_wmem = 4096 65536 67108864
net.ipv4.tcp_mem = 786432 1048576 26777216

# ---- TCP 优化 ----
net.ipv4.tcp_window_scaling = 1
net.ipv4.tcp_sack = 1
net.ipv4.tcp_fack = 1
net.ipv4.tcp_timestamps = 1
net.ipv4.tcp_no_metrics_save = 1
net.ipv4.tcp_slow_start_after_idle = 0

# Keepalive（透明代理场景需要较短探测）
net.ipv4.tcp_keepalive_time = 600
net.ipv4.tcp_keepalive_probes = 3
net.ipv4.tcp_keepalive_intvl = 15

# ---- UDP 缓冲区基本调整 ----
net.core.netdev_budget = 600
EOF

    log_step "立即生效（sysctl -p）"
    sysctl --system >/dev/null 2>&1 || sysctl -p "${RPROXY_SYSCTL_FILE}" >/dev/null 2>&1 || true

    log_done "网络参数调优已应用"
    echo ""
    echo "  当前关键参数："
    echo "    拥塞控制：$(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null)"
    echo "    qdisc：$(sysctl -n net.core.default_qdisc 2>/dev/null)"
    echo "    TCP Fast Open：$(sysctl -n net.ipv4.tcp_fastopen 2>/dev/null)"

    echo ""
    echo "  配置文件路径：${RPROXY_SYSCTL_FILE}"
    echo ""
}

# ---------------- 还原默认网络参数 ----------------

action_restore_default_sysctl() {
    if [[ ! -f "${RPROXY_SYSCTL_FILE}" ]]; then
        log_info "未应用过 RProxy 调优，无需还原"
        return 0
    fi

    local ans
    ans=$(ask "  将删除 ${RPROXY_SYSCTL_FILE}，确认还原？(yes/no): ")
    [[ "$ans" != "yes" ]] && { log_info "已取消"; return 0; }

    log_step "删除调优配置文件"
    rm -f "${RPROXY_SYSCTL_FILE}"

    log_step "重新加载所有 sysctl 配置"
    sysctl --system >/dev/null 2>&1 || true

    log_done "已还原（可能需要重启才能完全恢复 Debian 默认值）"
}

# ---------------- 显示已安装内核 ----------------

action_list_kernels() {
    echo ""
    echo "  当前运行内核：${GREEN}$(uname -r)${NC}"
    echo ""
    echo "  已安装的所有内核包："
    dpkg -l 'linux-image-*' 2>/dev/null | awk '/^ii/{print "    " $2 "  " $3}'
    echo ""
    echo "  GRUB 默认启动项："
    grep "^GRUB_DEFAULT" /etc/default/grub
    echo ""
}

# ---------------- 主菜单 ----------------

system_optimize_menu() {
    while true; do
        clear
        echo -e "${CYAN}========================================================================${NC}"
        echo -e "${CYAN}                       RProxy 服务端 - 系统优化${NC}"
        echo -e "${CYAN}========================================================================${NC}"
        echo ""
        echo -e "  当前内核：     ${GREEN}$(current_kernel)${NC}"
        echo -e "  内核来源：     $(kernel_source)"
        if [[ -f "${RPROXY_SYSCTL_FILE}" ]]; then
            echo -e "  网络调优：     ${GREEN}已应用${NC}"
        else
            echo -e "  网络调优：     ${YELLOW}未应用${NC}"
        fi
        echo ""
        echo "  ───────────────────────────────────────────────────────"
        echo "    1) 切换到 Zabbly 主线内核（需重启）"
        echo "    2) 回滚到 Debian 默认内核（需重启）"
        echo "    3) 应用网络参数调优（BBR + sysctl）"
        echo "    4) 显示已安装的所有内核"
        echo "    5) 还原默认网络参数"
        echo "    0) 返回主菜单"
        echo ""
        local choice
        choice=$(ask "  请输入选项 [0-5]: ")
        case "$choice" in
            1) action_install_zabbly ;;
            2) action_rollback_kernel ;;
            3) action_apply_sysctl ;;
            4) action_list_kernels ;;
            5) action_restore_default_sysctl ;;
            0|"") break ;;
            *) log_error "无效选项"; sleep 1 ;;
        esac
        echo ""
        read -rp "  按回车键继续..." _ </dev/tty || true
    done
}
