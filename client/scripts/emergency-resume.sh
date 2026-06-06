#!/bin/bash
# ============================================================================
# RProxy 客户端 紧急恢复脚本
# 用途：紧急停止后一键恢复透明代理
# 用法：sudo /opt/tproxy-gw/scripts/emergency-resume.sh
# ============================================================================

set -e

[[ $EUID -eq 0 ]] || { echo "[错误] 请用 root 运行" >&2; exit 1; }

echo "[恢复] 启动 mosdns..."
systemctl start tproxy-gw-mosdns

echo "[恢复] 启动 Xray..."
systemctl start xray

echo "[恢复] 重载 nftables 规则..."
nft -f /etc/nftables.conf
# nft -f 含 flush ruleset，cn_ips 集合被清空，必须重灌
systemctl restart tproxy-gw-load-cn-ips.service 2>/dev/null || true

echo "[恢复] 重置 resolv.conf 指向本机 mosdns..."
chattr -i /etc/resolv.conf 2>/dev/null || true
echo "nameserver 127.0.0.1" > /etc/resolv.conf

echo ""
echo "✅ 透明代理已恢复。"
echo ""
