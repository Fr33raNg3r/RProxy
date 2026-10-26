#!/bin/bash
# ============================================================================
# RProxy 服务端 Xray 看门狗
# 由 systemd timer 触发：rproxy-server-watchdog.timer（上一轮结束 60 秒后再运行）
#   - Xray 未运行：直接重启（配置错误退出时 systemd 不会自动重启）
#   - Xray 在运行但不响应：经本机 Nginx 访问 XHTTP 路径没有正常响应、而首页正常（说明 Nginx 没问题），
#     连续 2 轮就重启 Xray
# 只访问本机 127.0.0.1，不出公网。Nginx 本身的故障不在这里处理（只在开始与恢复时各记一条日志）。
# ============================================================================

source /opt/rproxy-server/scripts/common.sh

exec 201>/var/run/rproxy-server-watchdog.lock
flock -n 201 || exit 0

# 每日更新 / 升级进行中（持有更新锁）时跳过本轮，不和升级流程抢着重启 Xray；
# 只试探一下锁就立即释放，不能一直占着，否则每日更新会以为"已有任务在运行"而跳过
exec 202>/var/run/rproxy-server-update.lock
if ! flock -n 202; then
    exit 0
fi
flock -u 202
exec 202>&-

# xray_fail：XHTTP 连续无正常响应（且 Nginx 正常）的轮数；nginx_down：上一轮 Nginx 是否无响应
STATE_FILE="/run/rproxy-server-watchdog.state"
xray_fail=0
nginx_down=0
# shellcheck disable=SC1090
[[ -f "$STATE_FILE" ]] && source "$STATE_FILE"

save_state() {
    printf 'xray_fail=%s\nnginx_down=%s\n' "$xray_fail" "$nginx_down" > "$STATE_FILE"
}

if ! systemctl is-active --quiet xray; then
    log_to_file "[watchdog] Xray 未运行，重启"
    systemctl reset-failed xray 2>/dev/null || true
    systemctl restart xray || log_to_file "[watchdog] Xray 重启失败，请执行 journalctl -u xray 查看"
    xray_fail=0
    save_state
    exit 0
fi

domain=$(json_get "${SERVER_CFG}" '.domain')
path=$(json_get "${SERVER_CFG}" '.ws_path')
if [[ -z "$domain" || -z "$path" ]]; then
    exit 0
fi

# local_code <路径>：经本机 Nginx 访问，返回 HTTP 状态码（无响应为 000）
local_code() {
    curl -sk -o /dev/null -w '%{http_code}' --max-time 8 \
        --resolve "${domain}:443:127.0.0.1" "https://${domain}$1" 2>/dev/null
}

# Xray 正常时 XHTTP 路径对普通 GET 返回 404（Xray 原本回 400，被 nginx 改写成站点 404 页面）。Xray 进程没了时 Nginx 返回 502；
# Xray 卡死（进程在、端口还能连上但不回应）时 Nginx 一直等后端（grpc_read_timeout 315 秒），curl 超时得到 000
code=$(local_code "${path}/")
if [[ "$code" =~ ^[234][0-9][0-9]$ ]]; then
    xray_fail=0
    if (( nginx_down == 1 )); then
        log_to_file "[watchdog] 本机 Nginx 已恢复响应"
        nginx_down=0
    fi
elif [[ "$(local_code /)" =~ ^[234][0-9][0-9]$ ]]; then
    # 首页正常、只有 XHTTP 不通：问题在 Xray
    nginx_down=0
    xray_fail=$((xray_fail + 1))
    if (( xray_fail >= 2 )); then
        log_to_file "[watchdog] XHTTP 连续 ${xray_fail} 轮无正常响应（最近一次 ${code}），首页正常，判定 Xray 不响应，重启 Xray"
        systemctl restart xray || log_to_file "[watchdog] Xray 重启失败"
        xray_fail=0
    fi
else
    # 首页也不通：Nginx 的问题，不在本看门狗处理范围；只在刚出现时记一条
    xray_fail=0
    if (( nginx_down == 0 )); then
        log_to_file "[watchdog] 本机 Nginx 无响应（XHTTP ${code}），不处理 Xray"
        nginx_down=1
    fi
fi
save_state
