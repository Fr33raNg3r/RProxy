#!/bin/bash
# ============================================================================
# RProxy 客户端 健康检查 / 故障转移守护脚本
# 由 systemd timer 触发：rproxy-gw-watchdog.timer（上一轮结束 60 秒后再运行）
# 任务：
#   1. 本机检查（每轮都做，不产生出网流量）：Xray / mosdns / WebUI 是否运行、mosdns 能否应答、
#      nftables 规则表、策略路由、cn_ips（每 10 分钟）、resolv.conf
#   2. 代理探测（经当前节点出网）：间隔随机，白天 200～300 秒、0～9 点 300～600 秒；
#      每次从几个常用站点里随机挑一个，失败后隔 10～20 秒再测两次，3 次都失败才算故障
#   3. 确认故障：第一次重新生成配置并重启 Xray；之后再确认故障就切到下一个节点
#   4. 代理正常时检查是否可以回切到更优先的节点
#   5. 状态写入 /run/rproxy-gw/health.json 供 WebUI 读取
# ============================================================================

# 不用 set -e：守护脚本里任何一步失败（如 systemctl restart 报错、webui.json 损坏导致 jq 失败）
# 都不能中断后续的自愈检查和 state / health.json 的写入，否则计数错乱、WebUI 显示过期状态

source /opt/rproxy-gw/scripts/common.sh

# 本单元设了 LogLevelMax=notice（滤掉 systemd 每轮的启动 / 完成记录）：脚本自己的日志以 notice 级别写入 journal，
# 否则同样被滤掉，WebUI 日志页「健康检查」就看不到内容
LOG_STDOUT_PREFIX="<5>"

# 每轮都写的运行时状态放 /run（内存文件系统）：不磨损闪存，重启后从头开始也没有影响
RUN_DIR="/run/rproxy-gw"
HEALTH_FILE="${RUN_DIR}/health.json"
STATE_FILE="${RUN_DIR}/watchdog.state"
mkdir -p "${RUN_DIR}"

LOCK_FILE="/var/run/rproxy-gw-watchdog.lock"
exec 201>"$LOCK_FILE"
flock -n 201 || exit 0

# 代理探测地址：每次随机挑一个，访问目标不固定、与日常上网流量混在一起
# （与 webui-backend/services/failback.go 的 probeTargets 保持一致）
PROBE_URLS=(
    "https://www.google.com/generate_204"
    "https://www.youtube.com/generate_204"
    "https://www.facebook.com/favicon.ico"
    "https://www.instagram.com/favicon.ico"
    "https://x.com/favicon.ico"
    "https://www.netflix.com/favicon.ico"
)

# ---------- 状态文件 ----------
# restart_count：确认故障后是否已重启过 Xray（下一次确认故障就切换节点）
# next_probe_at：下一次代理探测的时间（epoch 秒）；proxy_ok / fail_count：最近一次探测的结果
# last_cn_check：上次检查 cn_ips 的时间
load_state() {
    if [[ -f "$STATE_FILE" ]]; then
        # shellcheck disable=SC1090
        source "$STATE_FILE"
    fi
    : "${restart_count:=0}"
    : "${next_probe_at:=0}"
    : "${proxy_ok:=0}"
    : "${fail_count:=0}"
    : "${last_cn_check:=0}"
}

save_state() {
    cat > "${STATE_FILE}.tmp" <<EOF
restart_count=${restart_count}
next_probe_at=${next_probe_at}
proxy_ok=${proxy_ok}
fail_count=${fail_count}
last_cn_check=${last_cn_check}
EOF
    mv -f "${STATE_FILE}.tmp" "$STATE_FILE"
}

# 下一次代理探测前的随机间隔（秒）：0～9 点 300～600，其余时间 200～300（按北京时间）
next_probe_delay() {
    local hour
    hour=$(TZ=Asia/Shanghai date +%-H)
    if (( hour < 9 )); then
        echo $(( 300 + RANDOM % 301 ))
    else
        echo $(( 200 + RANDOM % 101 ))
    fi
}

# 经 Xray SOCKS 访问一个随机挑选的站点：收到 2xx/3xx/4xx 说明整条链路是通的；5xx 与无响应算失败
probe_once() {
    local url code
    url="${PROBE_URLS[RANDOM % ${#PROBE_URLS[@]}]}"
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 \
        --socks5-hostname 127.0.0.1:10808 "$url" 2>/dev/null)
    [[ "$code" =~ ^[234][0-9][0-9]$ ]]
}

# 一轮探测：失败后隔 10～20 秒再测，最多 3 次，3 次都失败才算故障（单次抖动不触发动作）
# fail_count 记录本轮失败次数
probe_round() {
    local i
    fail_count=0
    for i in 1 2 3; do
        if probe_once; then
            return 0
        fi
        fail_count=$((fail_count + 1))
        if (( i < 3 )); then
            sleep $(( 10 + RANDOM % 11 ))
        fi
    done
    return 1
}

# ---------- 当前状态 ----------
load_state

# 紧急停止期间不做任何自愈（否则会把刚停掉的服务又拉起来），只更新 health.json
emergency=0
[[ -f "$EMERGENCY_FLAG" ]] && emergency=1

xray_active=0
mosdns_active=0
webui_active=0
current_node=""
last_action=""

is_service_active xray             && xray_active=1
is_service_active rproxy-gw-mosdns && mosdns_active=1
is_service_active rproxy-gw-webui  && webui_active=1

# 取得当前节点 ID
if [[ -f "${CONFIG_DIR}/webui.json" ]]; then
    current_node=$(jq -r '.current_node_id // ""' "${CONFIG_DIR}/webui.json")
fi

if [[ $emergency -eq 1 ]]; then
    last_action="emergency_stop"
    proxy_ok=0
else
# ==================== 本机检查（每轮都做，不出网） ====================

# ---------- Xray 未运行 ----------
# 下面的探测与故障转移都要求 Xray 在运行；Xray 因配置错误退出时 systemd 不会自动重启（退出码 23），
# 这里先直接重启，仍起不来再重新生成配置后重启
if [[ $xray_active -eq 0 ]]; then
    log_to_file "Xray 未运行，尝试重启"
    systemctl reset-failed xray 2>/dev/null || true
    systemctl restart xray || true
    sleep 2
    if ! is_service_active xray; then
        log_to_file "Xray 重启后仍未运行，重新生成配置后再试"
        "${BIN_DIR}/webui" render-xray >/dev/null 2>&1 || log_to_file "重新生成 Xray 配置失败"
        systemctl reset-failed xray 2>/dev/null || true
        systemctl restart xray || true
        sleep 2
    fi
    is_service_active xray && xray_active=1
    last_action="restart_xray"
fi

# ---------- nftables 规则表 ----------
# 规则表整个丢失（被误清空、nftables 重载失败）时国外流量全部不通。紧急停止已在上面排除，
# 这里表不存在就是故障：重启 nftables，策略路由、cn_ips、黑白名单静态 IP 都是 PartOf=nftables，会随之重新加载
if ! nft list table inet tp >/dev/null 2>&1; then
    log_to_file "nftables 规则表 inet tp 不存在，重新加载规则"
    systemctl restart nftables || true
    sleep 2
    last_action="reload_nftables"
fi

# ---------- mosdns ----------
if [[ $mosdns_active -eq 0 ]]; then
    log_to_file "mosdns 未运行，尝试重启"
    systemctl restart rproxy-gw-mosdns || true
elif ! "${BIN_DIR}/webui" dns-check >/dev/null 2>&1; then
    # 进程还在但不应答：只看进程是否在运行发现不了，局域网会全部解析失败
    log_to_file "mosdns 在运行但不应答 DNS 查询，重启"
    systemctl restart rproxy-gw-mosdns || true
    last_action="restart_mosdns"
fi

if [[ $webui_active -eq 0 ]]; then
    log_to_file "webui 未运行，尝试重启"
    systemctl restart rproxy-gw-webui || true
fi

# 策略路由丢失时 TPROXY 流量到不了 Xray（局域网和本机都不走代理），
# 而代理探测走 socks 端口、不经过策略路由，发现不了这种情况，需单独检查
if ! ip rule | grep -q 'fwmark 0x1 lookup 100' || ! ip route show table 100 | grep -q 'local default dev lo'; then
    log_to_file "策略路由（fwmark 1 → table 100）缺失，尝试恢复"
    systemctl restart rproxy-gw-iproute || true
fi

# cn_ips 被清空（只剩 nftables.conf 预置的几条 DNS）时，国内流量会全部绕道走代理；正常加载后有数千条。
# 列出整个集合开销较大且很少出问题，每 10 分钟查一次
now=$(date +%s)
if (( now - last_cn_check >= 600 )); then
    last_cn_check=$now
    if nft list set inet tp cn_ips >/dev/null 2>&1 && \
       [[ $(nft list set inet tp cn_ips | grep -oE '[0-9]+(\.[0-9]+){3}(/[0-9]+|-[0-9.]+)?' | wc -l) -lt 100 ]]; then
        log_to_file "cn_ips 集合几乎为空，尝试重新加载国内 IP"
        systemctl restart rproxy-gw-load-cn-ips || true
    fi
fi

# 本机 DNS 应指向 mosdns：紧急停止会改成公共 DNS，之后重启机器（标记随之清除）不会自动改回
if is_service_active rproxy-gw-mosdns && ! grep -q '^nameserver 127\.0\.0\.1' /etc/resolv.conf 2>/dev/null; then
    log_to_file "resolv.conf 未指向本机 mosdns，恢复"
    chattr -i /etc/resolv.conf 2>/dev/null || true
    printf '# Managed by RProxy - DNS handled by mosdns on 127.0.0.1:53\nnameserver 127.0.0.1\n' > /etc/resolv.conf
    chattr +i /etc/resolv.conf 2>/dev/null || true
fi

# ==================== 代理探测（按随机时间表） ====================
# 定时器每轮间隔约 60 秒，探测时间落在本轮之后 70 秒内时，在本轮里等到点再测，探测时间才是真正随机的
if [[ $xray_active -eq 1 && -n "$current_node" ]]; then
    now=$(date +%s)
    if (( next_probe_at <= now + 70 )); then
        if (( next_probe_at > now )); then
            sleep $(( next_probe_at - now ))
        fi
        if probe_round; then
            proxy_ok=1
            restart_count=0
            # 自动故障转移后，排在前面的节点恢复了就切回去（是否需要检查、探测、切换都在 failback-check 里判断）
            "${BIN_DIR}/webui" failback-check 2>&1 | tee -a "${LOG_DIR}/$(date +%Y%m%d).log" | sed 's/^/<5>/' || true
        else
            proxy_ok=0
            if [[ $restart_count -lt 1 ]]; then
                # 第一次：先重新生成配置再重启 Xray
                # 节点域名在生成配置时才预解析成 IP，VPS 换了 IP 后只重启会一直连旧 IP
                log_to_file "代理探测连续 3 次失败，重新生成配置并重启 Xray"
                "${BIN_DIR}/webui" render-xray >/dev/null 2>&1 || log_to_file "重新生成 Xray 配置失败，沿用旧配置重启"
                systemctl restart xray
                last_action="restart_xray"
                restart_count=1
            else
                # 第二次：切换节点
                log_to_file "重启 Xray 后代理仍不通，切换到下一个节点"
                "${BIN_DIR}/webui" switch-next-node 2>&1 | tee -a "${LOG_DIR}/$(date +%Y%m%d).log" | sed 's/^/<5>/'
                last_action="switch_node"
                restart_count=0
            fi
        fi
        next_probe_at=$(( $(date +%s) + $(next_probe_delay) ))
    fi
else
    proxy_ok=0
fi
fi

save_state

# ---------- 写入 health.json ----------
# 先写临时文件再 mv（同目录 rename 原子替换），WebUI 不会读到写了一半的 JSON
last_check_time=$(date '+%Y-%m-%d %H:%M:%S')
cat > "${HEALTH_FILE}.tmp" <<EOF
{
  "last_check": "${last_check_time}",
  "xray_active": ${xray_active},
  "mosdns_active": ${mosdns_active},
  "webui_active": ${webui_active},
  "proxy_ok": ${proxy_ok},
  "current_node_id": "${current_node}",
  "fail_count": ${fail_count},
  "restart_count": ${restart_count},
  "last_action": "${last_action}"
}
EOF
mv -f "${HEALTH_FILE}.tmp" "$HEALTH_FILE"
