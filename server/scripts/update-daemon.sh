#!/bin/bash
# ============================================================================
# RProxy 服务端 每日自动更新守护脚本
# 由 systemd timer 触发：tproxy-server-update.timer (04:05 daily)
# 任务：
#   1. 检查脚本仓库 VERSION 是否有新版 → 拉新代码升级
#   2. 检查 Xray 版本（通过官方脚本自动判断）
#   3. acme.sh 自动续期（acme.sh 自带 cron，但这里再触发一次保险）
# ============================================================================

set -e

source /opt/tproxy-server/scripts/common.sh

LOCK_FILE="/var/run/tproxy-server-update.lock"
exec 200>"$LOCK_FILE" || die "无法创建锁文件"
flock -n 200 || { log_warn "已有更新任务在运行，跳过"; exit 0; }

log_to_file "================ 开始每日更新 ================"

# ---------- 1. 检查脚本自身版本 ----------
check_self_update() {
    log_to_file "检查 RProxy 服务端 自身版本"
    local local_ver remote_ver
    local_ver=$(cat "${INSTALL_DIR}/VERSION")
    remote_ver=$(curl -fsSL --max-time 30 "${RAW_URL}/VERSION" | tr -d '[:space:]')
    if [[ -z "$remote_ver" ]]; then
        log_to_file "无法获取远端版本，跳过自我升级"
        return 1
    fi
    log_to_file "本地版本：${local_ver}    远端版本：${remote_ver}"
    if [[ "$local_ver" != "$remote_ver" ]]; then
        log_to_file "发现新版本，开始升级"
        local tmp_install
        tmp_install=$(mktemp)
        if curl -fsSL --max-time 60 "${RAW_URL}/install.sh" -o "$tmp_install"; then
            chmod +x "$tmp_install"
            bash "$tmp_install" upgrade >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1
            rm -f "$tmp_install"
            log_to_file "自我升级完成"
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
update_xray() {
    log_to_file "检查 Xray 版本"
    if bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install \
        >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1; then
        log_to_file "Xray 检查/升级完成"
    else
        log_to_file "Xray 升级失败"
    fi
}

# ---------- 3. 触发证书续期 ----------
renew_cert() {
    if [[ -d "${ACME_HOME}" ]]; then
        log_to_file "触发 acme.sh 证书续期检查"
        "${ACME_HOME}/acme.sh" --cron --home "${ACME_HOME}" >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1 || \
            log_to_file "acme.sh 续期检查失败"
    fi
}

# ---------- 主流程 ----------

if check_self_update; then
    log_to_file "================ 更新完成（含自我升级） ================"
    exit 0
fi

update_xray
renew_cert

log_to_file "================ 更新完成 ================"
