#!/bin/bash
# ============================================================================
# RProxy 客户端 紧急停止脚本
# 用途：分流出问题导致全网断网时，一键清空 nftables 规则恢复直连
# 用法：sudo /opt/tproxy-gw/scripts/emergency-stop.sh
# ============================================================================

set -e

[[ $EUID -eq 0 ]] || { echo "[错误] 请用 root 运行" >&2; exit 1; }

echo "[紧急停止] 清空 nftables 规则..."
nft flush ruleset 2>/dev/null || true

echo "[紧急停止] 停止 Xray..."
systemctl stop xray 2>/dev/null || true

echo "[紧急停止] 停止 mosdns（恢复正常 DNS）..."
systemctl stop tproxy-gw-mosdns 2>/dev/null || true

echo "[紧急停止] 重置 resolv.conf 为公共 DNS..."
chattr -i /etc/resolv.conf 2>/dev/null || true
cat > /etc/resolv.conf <<EOF
nameserver 223.5.5.5
nameserver 119.29.29.29
EOF

echo ""
echo "✅ 透明代理已停止，旁路由现在为直连模式。"
echo ""
echo "如需恢复，执行："
echo "  systemctl start tproxy-gw-mosdns"
echo "  systemctl start xray"
echo "  nft -f /etc/nftables.conf"
echo "  echo 'nameserver 127.0.0.1' > /etc/resolv.conf"
echo ""
