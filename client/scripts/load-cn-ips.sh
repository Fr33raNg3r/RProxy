#!/bin/bash
# ============================================================================
# RProxy 客户端 cn_ips 预加载脚本
# 把 geoip-cn.txt 里的国内 CIDR 一次性灌进 nftables inet tp cn_ips set。
# 用途：解决"裸 IP 访问国内站点被错送代理"的问题——nftables 不再依赖
#       mosdns 懒填充，启动时就拥有完整的国内 IP 视图。
# 触发：tproxy-gw-load-cn-ips.service（开机），update-daemon.sh（每日更新后）
# ============================================================================

set -euo pipefail

GEO_FILE="/opt/tproxy-gw/data/geo/geoip-cn.txt"

if [[ ! -f "$GEO_FILE" ]]; then
    echo "[load-cn-ips] geoip-cn.txt 不存在: $GEO_FILE" >&2
    exit 1
fi

# 确认 cn_ips set 存在（nftables 必须已加载）
if ! nft list set inet tp cn_ips >/dev/null 2>&1; then
    echo "[load-cn-ips] cn_ips set 不存在，请确认 nftables.service 已启动" >&2
    exit 1
fi

TMP=$(mktemp --suffix=.nft)
trap 'rm -f "$TMP"' EXIT

# 生成 nft 片段：只保留 IPv4 CIDR（cn_ips set 是 ipv4_addr，
# 上游 geoip-cn.txt 是 v4+v6 混排，混入 IPv6 段会让整个 batch 加载失败）
{
    echo 'add element inet tp cn_ips {'
    awk 'NF && !/^[[:space:]]*#/' "$GEO_FILE" \
        | sed 's/[[:space:]]//g' \
        | grep -E '^([0-9]{1,3}\.){3}[0-9]{1,3}(/[0-9]+)?$' \
        | paste -sd, -
    echo '}'
} > "$TMP"

# auto-merge 模式下重复/相邻区间会自动合并，重跑安全
if ! nft -f "$TMP"; then
    echo "[load-cn-ips] nft 加载失败" >&2
    exit 1
fi

COUNT=$(awk 'NF && !/^[[:space:]]*#/' "$GEO_FILE" \
    | sed 's/[[:space:]]//g' \
    | grep -cE '^([0-9]{1,3}\.){3}[0-9]{1,3}(/[0-9]+)?$')
echo "[load-cn-ips] 已加载 $COUNT 条 CIDR 到 cn_ips"
