#!/bin/bash
# ============================================================================
# RProxy 客户端 静态 IP 加载脚本
# 把用户在 WebUI 黑白名单里填的 IP/CIDR（域名走 mosdns，IP 走这里）灌进
# nftables 的 whitelist_ips（强制直连）/ blacklist_ips（强制走代理）set。
# 触发：tproxy-gw-load-static-ips.service（开机）、tproxy-gw-flush-ipsets.service
#       （每日 flush 后）、WebUI 保存黑白名单后（dns.go 调用）。
# 设计：始终 exit 0，不阻塞调用它的 oneshot 服务（文件缺/空、set 未加载、
#       nft 报错都只告警不失败）。
# ============================================================================

set -uo pipefail

DNS_DIR="/opt/tproxy-gw/config/dns"

load_set() {
    local set_name="$1" file="$2"
    [[ -f "$file" ]] || return 0

    # 仅保留 IPv4 地址/CIDR 行（域名等其它行不在此处理）
    local elems
    elems=$(awk 'NF && !/^[[:space:]]*#/' "$file" \
        | sed 's/[[:space:]]//g' \
        | grep -E '^([0-9]{1,3}\.){3}[0-9]{1,3}(/[0-9]+)?$' \
        | paste -sd, -)
    [[ -n "$elems" ]] || return 0

    # set 必须已存在（nftables 已加载）
    if ! nft list set inet tp "$set_name" >/dev/null 2>&1; then
        echo "[load-static-ips] set $set_name 不存在，跳过（nftables 未加载？）" >&2
        return 0
    fi

    # auto-merge 模式下重复/相邻区间自动合并，重跑安全
    if printf 'add element inet tp %s { %s }\n' "$set_name" "$elems" | nft -f -; then
        echo "[load-static-ips] 已加载静态 IP 到 $set_name"
    else
        echo "[load-static-ips] 加载到 $set_name 失败（忽略）" >&2
    fi
}

load_set whitelist_ips "${DNS_DIR}/whitelist_ips.txt"
load_set blacklist_ips "${DNS_DIR}/blacklist_ips.txt"
exit 0
