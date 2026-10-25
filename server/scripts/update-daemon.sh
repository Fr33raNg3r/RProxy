#!/bin/bash
# ============================================================================
# RProxy 服务端 每日自动更新守护脚本
# 由 systemd timer 触发：rproxy-server-update.timer (04:05 daily)
# 任务：
#   1. 检查脚本仓库 VERSION 是否有新版 → 拉新代码升级
#   2. 检查 Xray 版本（通过官方脚本自动判断）
#   3. acme.sh 自动续期（acme.sh 自带 cron，但这里再触发一次保险）
# ============================================================================

set -e

source /opt/rproxy-server/scripts/common.sh

LOCK_FILE="/var/run/rproxy-server-update.lock"
exec 200>"$LOCK_FILE" || die "无法创建锁文件"
flock -n 200 || { log_warn "已有更新任务在运行，跳过"; exit 0; }

log_to_file "================ 开始每日更新 ================"

# ---------- 1. 检查脚本自身版本 ----------
check_self_update() {
    log_to_file "检查 RProxy 服务端 自身版本"
    local local_ver remote_ver
    local_ver=$(cat "${INSTALL_DIR}/VERSION")
    # VERSION 在仓库根目录，RAW_URL 末尾带 /server，需去掉末段路径再取根目录的 VERSION
    remote_ver=$(curl -fsSL --max-time 30 "${RAW_URL%/*}/VERSION" | tr -d '[:space:]')
    if [[ -z "$remote_ver" ]]; then
        log_to_file "无法获取远端版本，跳过自我升级"
        return 1
    fi
    log_to_file "本地版本：${local_ver}    远端版本：${remote_ver}"
    # 升级后曾自动回滚的版本：不再自动升级，等发布更新的版本（手动升级不受限制）
    local held
    held=$(cat /var/lib/rproxy-server/hold-rproxy-version 2>/dev/null || true)
    if [[ -n "$held" && "$remote_ver" == "$held" ]]; then
        log_to_file "版本 ${held} 升级后曾自动回滚，跳过自动升级"
        return 1
    fi
    if [[ "$local_ver" != "$remote_ver" ]]; then
        log_to_file "发现新版本，开始升级"
        local tmp_install
        tmp_install=$(mktemp)
        if curl -fsSL --max-time 60 "${RAW_URL}/install.sh" -o "$tmp_install"; then
            chmod +x "$tmp_install"
            # RPROXY_AUTO_UPGRADE=1：告知 install.sh 本次是自动升级，需遵守 Xray 自动更新开关
            if RPROXY_AUTO_UPGRADE=1 bash "$tmp_install" upgrade >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1; then
                log_to_file "自我升级完成"
            else
                log_to_file "自我升级失败（升级流程会自动回滚到升级前快照，详见上方日志）"
            fi
            rm -f "$tmp_install"
            return 0
        else
            log_to_file "下载新 install.sh 失败"
            return 1
        fi
    fi
    log_to_file "已是最新版"
    return 1
}

# ---------- 2. 升级 Xray ----------
# 有新版本才升级。升级前打本机快照（rproxy-rollback），装新版后：
#   - 新版无法加载现有配置（如弃用功能被移除）
#   - 本机自检不通过（经 Nginx 访问 XHTTP 应为 400，仅在升级前自检通过时才判断）
# 任一情况都整体回滚到快照，并记录问题版本，之后跳过它直到上游发布新版本。
# 服务端无 WebUI，结果记入日志，同时留存 data/xray-update-alert.json 便于排查。
update_xray() {
    # rproxy-server xray-autoupdate off 关闭了 Xray 自动更新（手动升级不受影响）
    if [[ -f /var/lib/rproxy-server/no-xray-auto-update ]]; then
        log_to_file "Xray 自动更新已关闭，跳过"
        return
    fi
    log_to_file "检查 Xray 版本"
    local logf="${LOG_DIR}/$(date +%Y%m%d).log"
    local xray_bin="/usr/local/bin/xray"
    local xray_cfg="${XRAY_CONFIG_FILE:-/usr/local/etc/xray/config.json}"
    local rollback="/usr/local/sbin/rproxy-rollback"
    local cur latest held
    cur=$("$xray_bin" version 2>/dev/null | head -n 1 | awk '{print $2}')
    latest=$(curl -fsSL --max-time 30 https://api.github.com/repos/XTLS/Xray-core/releases/latest \
                 | jq -r '.tag_name // empty' 2>/dev/null || true)
    latest="${latest#v}"
    if [[ -z "$latest" ]]; then
        log_to_file "无法获取 Xray 最新版本，本次跳过"
        return
    fi
    if [[ "$latest" == "$cur" ]]; then
        log_to_file "Xray 已是最新版 ${cur}"
        return
    fi
    held=$(cat /var/lib/rproxy-server/hold-xray-version 2>/dev/null || true)
    if [[ "$latest" == "$held" ]]; then
        log_to_file "Xray ${held} 曾导致问题并已回滚，跳过，等待上游发布新版本"
        return
    fi

    local pre_ok=0 snap
    "$rollback" check --xhttp >>"$logf" 2>&1 && pre_ok=1
    if ! snap=$("$rollback" create "Xray 升级前 ${cur} → ${latest}" 2>>"$logf"); then
        log_to_file "快照创建失败（磁盘空间不足？），为安全起见跳过本次 Xray 升级"
        return
    fi
    log_to_file "Xray ${cur} → ${latest}，已创建快照 ${snap}"

    # curl 必须带超时：连接僵住时本服务（oneshot）会一直不结束，之后每天的定时器都无法再触发
    bash -c "$(curl -fL --max-time 120 https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install \
        >>"$logf" 2>&1 || log_to_file "Xray 安装脚本返回失败"

    local new
    new=$("$xray_bin" version 2>/dev/null | head -n 1 | awk '{print $2}')
    if [[ -z "$new" ]]; then
        xray_rollback "$snap" "Xray 升级后程序无法运行"
    elif [[ "$new" == "$cur" ]]; then
        log_to_file "Xray 版本未变化（${cur}），可能是下载失败，保持原状"
    elif [[ -f "$xray_cfg" ]] && ! "$xray_bin" run -test -format=json -c "$xray_cfg" >>"$logf" 2>&1; then
        xray_rollback "$snap" "新版 Xray ${new} 无法加载现有配置（可能是被弃用的功能已被移除）"
    elif [[ $pre_ok -eq 1 ]] && ! xray_check_after_upgrade; then
        xray_rollback "$snap" "新版 Xray ${new} 本机自检未通过"
    else
        log_to_file "Xray 已升级：${cur} → ${new}"
        rm -f /var/lib/rproxy-server/hold-xray-version
    fi
}

# 新版 Xray 起来后做本机自检，最多 3 次（官方脚本重启 Xray 需要几秒）
xray_check_after_upgrade() {
    local i
    for i in 1 2 3; do
        sleep 5
        /usr/local/sbin/rproxy-rollback check --xhttp >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1 && return 0
    done
    return 1
}

# xray_rollback <快照名> <原因>：整体回滚到升级前快照，并记录问题版本
xray_rollback() {
    local logf="${LOG_DIR}/$(date +%Y%m%d).log"
    log_to_file "$2，回滚到快照 $1"
    /usr/local/sbin/rproxy-rollback --yes --reason "$2" "$1" >>"$logf" 2>&1 \
        || log_to_file "回滚失败，请执行 rproxy-rollback 手动恢复"
}

# ---------- 3. 触发证书续期 ----------
renew_cert() {
    if [[ -d "${ACME_HOME}" ]]; then
        log_to_file "触发 acme.sh 证书续期检查"
        "${ACME_HOME}/acme.sh" --cron --home "${ACME_HOME}" >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1 || \
            log_to_file "acme.sh 续期检查失败"
    fi
}

# ---------- 4. 证书剩余天数 ----------
# 服务端没有界面：剩余不足有效期 1/5 时记一条告警（客户端 WebUI 会同时显示告警），平时只记剩余天数
log_cert_days() {
    local crt="${SSL_DIR}/server.crt" start end now total left
    [[ -f "$crt" ]] || return 0
    start=$(date -d "$(openssl x509 -startdate -noout -in "$crt" | cut -d= -f2)" +%s 2>/dev/null) || return 0
    end=$(date -d "$(openssl x509 -enddate -noout -in "$crt" | cut -d= -f2)" +%s 2>/dev/null) || return 0
    now=$(date +%s)
    total=$(( end - start ))
    left=$(( end - now ))
    if (( left < total / 5 )); then
        log_to_file "【告警】证书剩余 $(( left / 86400 )) 天（有效期 $(( total / 86400 )) 天），自动续期可能一直失败，请检查 acme.sh 日志"
    else
        log_to_file "证书剩余 $(( left / 86400 )) 天"
    fi
}

# ---------- 主流程 ----------

if check_self_update; then
    log_to_file "================ 更新完成（含自我升级） ================"
    exit 0
fi

update_xray
renew_cert
log_cert_days
prune_logs

log_to_file "================ 更新完成 ================"
