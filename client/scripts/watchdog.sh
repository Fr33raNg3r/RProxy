#!/bin/bash
# ============================================================================
# RProxy 客户端 健康检查 / 故障转移守护脚本
# 由 systemd timer 每 60 秒触发：tproxy-gw-watchdog.timer
# 任务：
#   1. 通过 Xray SOCKS 出站测试 google.com/generate_204（大陆无法直连）
#   2. 连续 3 次失败 → 重启 Xray
#   3. 重启后再连续 3 次失败 → 切到 nodes.json 中下一个启用的节点
#   4. 检查 mosdns / webui 进程，inactive 则重启
#   5. 状态写入 health.json 供 WebUI 读取
# ============================================================================

set -e

source /opt/tproxy-gw/scripts/common.sh

HEALTH_FILE="${DATA_DIR}/health.json"
STATE_FILE="${DATA_DIR}/watchdog.state"
mkdir -p "${DATA_DIR}"

LOCK_FILE="/var/run/tproxy-gw-watchdog.lock"
exec 201>"$LOCK_FILE"
flock -n 201 || exit 0

# ---------- 状态文件 ----------
# 形如：fail_count=N restart_count=M
load_state() {
    if [[ -f "$STATE_FILE" ]]; then
        # shellcheck disable=SC1090
        source "$STATE_FILE"
    fi
    : "${fail_count:=0}"
    : "${restart_count:=0}"
}

save_state() {
    cat > "$STATE_FILE" <<EOF
fail_count=${fail_count}
restart_count=${restart_count}
EOF
}

# ---------- 健康检查 ----------
load_state

xray_active=0
mosdns_active=0
webui_active=0
proxy_ok=0
current_node=""
last_check_time=$(date '+%Y-%m-%d %H:%M:%S')
last_action=""

is_service_active xray            && xray_active=1
is_service_active tproxy-gw-mosdns && mosdns_active=1
is_service_active tproxy-gw-webui  && webui_active=1

# 取得当前节点 ID
if [[ -f "${CONFIG_DIR}/webui.json" ]]; then
    current_node=$(jq -r '.current_node_id // ""' "${CONFIG_DIR}/webui.json")
fi

# ---------- 探测代理 ----------
# 仅在 Xray 运行中且有当前节点时探测
if [[ $xray_active -eq 1 && -n "$current_node" ]]; then
    if proxy_health_check 10808; then
        proxy_ok=1
        fail_count=0
        restart_count=0
    else
        fail_count=$((fail_count + 1))
        log_to_file "代理健康检查失败（连续 ${fail_count} 次）"
    fi
fi

# ---------- 故障转移 ----------
if [[ $proxy_ok -eq 0 && $xray_active -eq 1 && -n "$current_node" ]]; then
    if [[ $fail_count -ge 3 ]]; then
        if [[ $restart_count -lt 1 ]]; then
            # 第一次：先重启 Xray
            log_to_file "尝试重启 Xray"
            systemctl restart xray
            last_action="restart_xray"
            restart_count=$((restart_count + 1))
            fail_count=0
        else
            # 第二次：切换节点
            log_to_file "重启 Xray 仍失败，切换到下一个节点"
            "${BIN_DIR}/webui" switch-next-node 2>&1 | tee -a "${LOG_DIR}/$(date +%Y%m%d).log"
            last_action="switch_node"
            fail_count=0
            restart_count=0
        fi
    fi
fi

# ---------- 重启失活的辅助组件 ----------
if [[ $mosdns_active -eq 0 ]]; then
    log_to_file "mosdns 未运行，尝试重启"
    systemctl restart tproxy-gw-mosdns || true
fi

if [[ $webui_active -eq 0 ]]; then
    log_to_file "webui 未运行，尝试重启"
    systemctl restart tproxy-gw-webui || true
fi

save_state

# ---------- 写入 health.json ----------
cat > "$HEALTH_FILE" <<EOF
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
