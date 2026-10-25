#!/bin/bash
# ============================================================================
# RProxy 服务端 Xray 看门狗
# 由 systemd timer 触发：rproxy-server-watchdog.timer（上一轮结束 60 秒后再运行）
#   - Xray 未运行：直接重启（配置错误退出时 systemd 不会自动重启）
#   - Xray 在运行但不响应：经本机 Nginx 访问 XHTTP 路径，连续 2 轮返回 502 就重启 Xray
# 只访问本机 127.0.0.1，不出公网。Nginx 本身的故障不在这里处理（只记日志）。
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

STATE_FILE="/run/rproxy-server-watchdog.state"
fail_502=0
# shellcheck disable=SC1090
[[ -f "$STATE_FILE" ]] && source "$STATE_FILE"

save_state() { echo "fail_502=${fail_502}" > "$STATE_FILE"; }

if ! systemctl is-active --quiet xray; then
    log_to_file "[watchdog] Xray 未运行，重启"
    systemctl reset-failed xray 2>/dev/null || true
    systemctl restart xray || log_to_file "[watchdog] Xray 重启失败，请执行 journalctl -u xray 查看"
    fail_502=0
    save_state
    exit 0
fi

domain=$(json_get "${SERVER_CFG}" '.domain')
path=$(json_get "${SERVER_CFG}" '.ws_path')
if [[ -z "$domain" || -z "$path" ]]; then
    exit 0
fi

# Xray 正常时 XHTTP 路径对普通 GET 返回 400；Xray 不响应时 Nginx 返回 502
code=$(curl -sk -o /dev/null -w '%{http_code}' --max-time 8 \
    --resolve "${domain}:443:127.0.0.1" "https://${domain}${path}/" 2>/dev/null)
case "$code" in
    502)
        fail_502=$((fail_502 + 1))
        if (( fail_502 >= 2 )); then
            log_to_file "[watchdog] XHTTP 连续 ${fail_502} 次返回 502（Xray 不响应），重启 Xray"
            systemctl restart xray || log_to_file "[watchdog] Xray 重启失败"
            fail_502=0
        fi
        ;;
    000)
        # Nginx 没有响应：不在本看门狗处理范围
        log_to_file "[watchdog] 本机 Nginx 无响应（不处理 Xray）"
        fail_502=0
        ;;
    *)
        fail_502=0
        ;;
esac
save_state
