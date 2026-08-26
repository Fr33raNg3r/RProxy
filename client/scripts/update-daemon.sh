#!/bin/bash
# ============================================================================
# RProxy 客户端 每日自动更新守护脚本
# 由 systemd timer 触发：tproxy-gw-update.timer
# 任务：
#   1. 检查 GitHub 仓库 VERSION 是否有新版 → 拉新代码 → 升级
#   2. 检查 Xray 版本（通过官方脚本自动判断）
#   3. 更新 GeoIP / GeoSite 数据
#   4. 重新解析所有节点域名，比对 IP 是否变化（DoH）
# ============================================================================

set -e

source /opt/tproxy-gw/scripts/common.sh

LOCK_FILE="/var/run/tproxy-gw-update.lock"
exec 200>"$LOCK_FILE" || die "无法创建锁文件"
flock -n 200 || { log_warn "已有更新任务在运行，本次跳过"; exit 0; }

log_to_file "================ 开始每日更新 ================"

# ---------- 1. 检查脚本自身版本 ----------
check_self_update() {
    log_to_file "检查 RProxy 客户端 自身版本"
    local local_ver remote_ver
    local_ver=$(cat "${INSTALL_DIR}/VERSION")
    # VERSION 在仓库根目录，RAW_URL 末尾带 /client，需去掉末段路径再取根目录的 VERSION
    remote_ver=$(curl -fsSL --max-time 30 "${RAW_URL%/*}/VERSION" | tr -d '[:space:]')
    if [[ -z "$remote_ver" ]]; then
        log_to_file "无法获取远端版本，跳过自我升级"
        return 1
    fi
    log_to_file "本地版本：${local_ver}    远端版本：${remote_ver}"
    if [[ "$local_ver" != "$remote_ver" ]]; then
        log_to_file "发现新版本，开始升级"
        # 下载最新 install.sh 并以 upgrade 模式执行
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
    log_to_file "已是最新版，跳过"
    return 1
}

# ---------- 2. 升级 Xray ----------
# 升级前备份旧二进制；装新版后用新版校验现有配置能否加载，
# 不兼容（如 VMess/WS 等弃用功能被移除）则回滚并以旧版重启，
# 同时写告警标志 data/xray-update-alert.json 供 WebUI 展示。
update_xray() {
    log_to_file "检查 Xray 版本（官方脚本自动判断）"
    local logf="${LOG_DIR}/$(date +%Y%m%d).log"
    local xray_bin="/usr/local/bin/xray"
    local xray_cfg="/usr/local/etc/xray/config.json"
    local alert_file="${DATA_DIR}/xray-update-alert.json"
    local xray_bak=""
    if [[ -x "$xray_bin" ]]; then
        xray_bak=$(mktemp)
        cp -f "$xray_bin" "$xray_bak"
    fi

    if ! bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install \
        >>"$logf" 2>&1; then
        log_to_file "Xray 升级失败（保持原版本运行）"
        [[ -n "$xray_bak" ]] && rm -f "$xray_bak"
        return
    fi

    # 官方脚本可能已用新版重启 xray；用新版二进制校验现有配置能否加载。
    if [[ -f "$xray_cfg" ]] && ! "$xray_bin" run -test -format=json -c "$xray_cfg" >>"$logf" 2>&1; then
        log_to_file "新版 Xray 无法加载现有配置，回滚到旧版本"
        if [[ -n "$xray_bak" ]]; then
            cp -f "$xray_bak" "$xray_bin"
            systemctl restart xray >>"$logf" 2>&1 || true
            log_to_file "已回滚并以旧版本重启 Xray"
        fi
        mkdir -p "${DATA_DIR}"
        cat > "$alert_file" <<EOF
{
  "active": true,
  "time": "$(date '+%Y-%m-%d %H:%M:%S')",
  "message": "Xray 自动更新检测到新版本与当前配置不兼容（可能是 VMess / WebSocket 等被弃用功能已被移除），已自动回滚到旧版本以保持服务可用。建议尽快迁移协议（VMess→VLESS、WebSocket→XHTTP）后再升级。"
}
EOF
    else
        log_to_file "Xray 新版本配置校验通过"
        rm -f "$alert_file" 2>/dev/null || true
    fi
    [[ -n "$xray_bak" ]] && rm -f "$xray_bak"
}

# ---------- 3. 更新 GeoIP / GeoSite ----------
update_geo_data() {
    log_to_file "更新 GeoIP / GeoSite 数据"
    local geo_dir="${INSTALL_DIR}/data/geo"
    mkdir -p "$geo_dir"

    local urls=(
        "geoip.dat=https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat"
        "geosite.dat=https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat"
        "geosite-cn.txt=https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/direct-list.txt"
        "proxy-list.txt=https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/proxy-list.txt"
        "gfw.txt=https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/gfw.txt"
        "geoip-cn.txt=https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt"
    )

    local all_ok=1
    for entry in "${urls[@]}"; do
        local fname="${entry%%=*}"
        local url="${entry#*=}"
        local tmp
        tmp=$(mktemp)
        if curl -fsSL --max-time 60 -o "$tmp" "$url"; then
            mv "$tmp" "${geo_dir}/${fname}"
        else
            log_to_file "下载失败: ${fname}"
            rm -f "$tmp"
            all_ok=0
        fi
    done

    if [[ $all_ok -eq 1 ]]; then
        cp -f "${geo_dir}/geoip.dat"   /usr/local/share/xray/ 2>/dev/null
        cp -f "${geo_dir}/geosite.dat" /usr/local/share/xray/ 2>/dev/null
        cat "${geo_dir}/proxy-list.txt" "${geo_dir}/gfw.txt" \
            | sort -u > "${geo_dir}/geosite-no-cn.txt"
        log_to_file "GeoIP / GeoSite 已更新"
        # 重新加载 cn_ips（geoip-cn.txt 有更新）
        systemctl restart tproxy-gw-load-cn-ips.service 2>/dev/null || true
        systemctl restart xray
        systemctl restart tproxy-gw-mosdns
    else
        log_to_file "部分文件下载失败，跳过本次更新"
    fi
}

# ---------- 主流程 ----------
# 先做自我升级，如果升级了，自我升级流程已经处理了 Xray、Geo 等所有更新
if check_self_update; then
    log_to_file "================ 更新完成（含自我升级） ================"
    exit 0
fi

update_xray
update_geo_data
prune_logs

log_to_file "================ 更新完成 ================"
