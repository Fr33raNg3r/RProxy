#!/bin/bash
# ============================================================================
# RProxy 客户端 每日自动更新守护脚本
# 由 systemd timer 触发：rproxy-gw-update.timer
# 任务：
#   1. 检查 GitHub 仓库 VERSION 是否有新版 → 拉新代码 → 升级
#   2. 检查 Xray 版本（通过官方脚本自动判断）
#   3. 更新 GeoIP / GeoSite 数据
#   4. 重新解析所有节点域名，比对 IP 是否变化（DoH）
# ============================================================================

set -e

source /opt/rproxy-gw/scripts/common.sh

LOCK_FILE="/var/run/rproxy-gw-update.lock"
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
    # 升级后曾自动回滚的版本：不再自动升级，等发布更新的版本（手动升级不受限制）
    local held
    held=$(cat /var/lib/rproxy-gw/hold-rproxy-version 2>/dev/null || true)
    if [[ -n "$held" && "$remote_ver" == "$held" ]]; then
        log_to_file "版本 ${held} 升级后曾自动回滚，跳过自动升级"
        return 1
    fi
    if [[ "$local_ver" != "$remote_ver" ]]; then
        log_to_file "发现新版本，开始升级"
        # 下载最新 install.sh 并以 upgrade 模式执行
        local tmp_install
        tmp_install=$(mktemp)
        if curl -fsSL --max-time 60 "${RAW_URL}/install.sh" -o "$tmp_install"; then
            chmod +x "$tmp_install"
            if bash "$tmp_install" upgrade >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1; then
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
    log_to_file "已是最新版，跳过"
    return 1
}

# ---------- 2. 升级 Xray ----------
# 有新版本才升级。升级前打本机快照（rproxy-rollback），装新版后：
#   - 新版无法加载现有配置（如弃用功能被移除）
#   - 能加载但实际连不通（如 1.5.0 遇到的 ALPN 行为变化，仅在升级前是通的时才判断）
# 任一情况都整体回滚到快照——连同官方脚本对 systemd 单元、目录等的改动一起撤销，
# 并记录问题版本，之后跳过它直到上游发布新版本。告警写入 data/xray-update-alert.json 供 WebUI 展示。
update_xray() {
    log_to_file "检查 Xray 版本"
    local logf="${LOG_DIR}/$(date +%Y%m%d).log"
    local xray_bin="/usr/local/bin/xray"
    local xray_cfg="/usr/local/etc/xray/config.json"
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
    held=$(cat /var/lib/rproxy-gw/hold-xray-version 2>/dev/null || true)
    if [[ "$latest" == "$held" ]]; then
        log_to_file "Xray ${held} 曾导致问题并已回滚，跳过，等待上游发布新版本"
        return
    fi

    # 升级前的连通性：只有"升级前通、升级后不通"才归咎于 Xray 升级
    local pre_ok=0 snap
    proxy_health_check 10808 && pre_ok=1
    if ! snap=$(/usr/local/sbin/rproxy-rollback create "Xray 升级前 ${cur} → ${latest}" 2>>"$logf"); then
        log_to_file "快照创建失败（磁盘空间不足？），为安全起见跳过本次 Xray 升级"
        return
    fi
    log_to_file "Xray ${cur} → ${latest}，已创建快照 ${snap}"

    # --without-logfiles：不让官方脚本把 /var/log/xray 改成 xray:xray 700（root 无 CAP_DAC_OVERRIDE 会打不开 log）
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install --without-logfiles \
        >>"$logf" 2>&1 || log_to_file "Xray 安装脚本返回失败"

    local new
    new=$("$xray_bin" version 2>/dev/null | head -n 1 | awk '{print $2}')
    if [[ -z "$new" ]]; then
        xray_rollback "$snap" "Xray 升级后程序无法运行"
    elif [[ "$new" == "$cur" ]]; then
        log_to_file "Xray 版本未变化（${cur}），可能是下载失败，保持原状"
    elif [[ -f "$xray_cfg" ]] && ! "$xray_bin" run -test -format=json -c "$xray_cfg" >>"$logf" 2>&1; then
        xray_rollback "$snap" "新版 Xray ${new} 无法加载现有配置（可能是被弃用的功能已被移除）"
    elif [[ $pre_ok -eq 1 ]] && ! xray_health_after_upgrade; then
        xray_rollback "$snap" "新版 Xray ${new} 能加载配置但代理连不通"
    else
        log_to_file "Xray 已升级：${cur} → ${new}"
        rm -f /var/lib/rproxy-gw/hold-xray-version
    fi
}

# xray_rollback <快照名> <原因>：整体回滚到升级前快照，并写入告警与问题版本
xray_rollback() {
    local logf="${LOG_DIR}/$(date +%Y%m%d).log"
    log_to_file "$2，回滚到快照 $1"
    /usr/local/sbin/rproxy-rollback --yes --reason "$2" "$1" >>"$logf" 2>&1 \
        || log_to_file "回滚失败，请 SSH 登录后执行 rproxy-rollback"
}

# xray_health_after_upgrade 等新版 Xray 起来后检测代理，最多 3 次（官方脚本重启 Xray 需要几秒）
xray_health_after_upgrade() {
    local i
    for i in 1 2 3; do
        sleep 5
        proxy_health_check 10808 && return 0
    done
    return 1
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
        systemctl restart rproxy-gw-load-cn-ips.service 2>/dev/null || true
        systemctl restart xray
        systemctl restart rproxy-gw-mosdns
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
# 服务端可能比客户端晚升级到支持 XHTTP 的版本，每天再试一次自动迁移旧节点
log_to_file "检查节点能否自动迁移到 VLESS + XHTTP"
"${BIN_DIR}/webui" migrate-xhttp >>"${LOG_DIR}/$(date +%Y%m%d).log" 2>&1 \
    || log_to_file "XHTTP 迁移检查失败（不影响现有连接）"
prune_logs

log_to_file "================ 更新完成 ================"
