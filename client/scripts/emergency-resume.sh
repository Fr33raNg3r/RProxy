#!/bin/bash
# ============================================================================
# RProxy 客户端 紧急恢复脚本
# 用途：紧急停止后一键恢复透明代理
# 用法：sudo /opt/rproxy-gw/scripts/emergency-resume.sh
# ============================================================================

set -e

[[ $EUID -eq 0 ]] || { echo "[错误] 请用 root 运行" >&2; exit 1; }

# 任一步失败就停掉已启动的服务、保持完整的紧急停止状态（规则、DNS、标记都不动），
# 避免半恢复：mosdns 起了但规则没加载、resolv.conf 仍是公共 DNS、标记还在
abort() {
    echo "[错误] $1，已撤回，仍处于紧急停止状态（直连）。可执行 journalctl -u $2 -n 50 查看原因" >&2
    nft flush ruleset 2>/dev/null || true
    systemctl stop xray rproxy-gw-mosdns 2>/dev/null || true
    exit 1
}

# start_checked <服务>：启动并等 2 秒确认仍在运行（配置错误时进程会在启动后很快退出，systemctl start 照样返回成功）
start_checked() {
    systemctl reset-failed "$1" 2>/dev/null || true
    systemctl start "$1" || return 1
    sleep 2
    systemctl is-active --quiet "$1"
}

echo "[恢复] 启动 mosdns..."
start_checked rproxy-gw-mosdns || abort "mosdns 启动失败" rproxy-gw-mosdns

echo "[恢复] 启动 Xray..."
start_checked xray || abort "Xray 启动失败" xray

echo "[恢复] 重载 nftables 规则..."
nft -f /etc/nftables.conf || abort "nftables 规则加载失败" nftables
# nft -f 含 flush ruleset，cn_ips 与黑白名单集合都被清空，必须重灌
systemctl restart rproxy-gw-load-cn-ips.service 2>/dev/null || true
systemctl restart rproxy-gw-load-static-ips.service 2>/dev/null || true

echo "[恢复] 重置 resolv.conf 指向本机 mosdns..."
chattr -i /etc/resolv.conf 2>/dev/null || true
printf '# Managed by RProxy - DNS handled by mosdns on 127.0.0.1:53\nnameserver 127.0.0.1\n' > /etc/resolv.conf
# 与安装时一致：防止被 DHCP 等覆盖
chattr +i /etc/resolv.conf 2>/dev/null || true

rm -f /run/rproxy-gw-emergency-stop

echo ""
echo "✅ 透明代理已恢复。"
echo ""
