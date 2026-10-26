#!/bin/bash
# ============================================================================
# RProxy 服务端 默认网站（私人影音库伪装站）：生成、渲染、部署、导入、重置
# 被 install.sh / rproxy-server 引入（source），依赖 common.sh 的日志函数与常量。
#
# 站点设置存在 ${SITE_CONF}（不在 Web 根目录里，避免被公开下载）：
#   文件存在 = 当前网站由本项目生成，升级时据此重新渲染，站名/配色/片单保持不变；
#   文件不存在且目录里有内容 = 用户自定义网站，升级不覆盖。
# ============================================================================

# 路径默认取 common.sh 的常量；测试时可在 source 前预先设置覆盖
: "${SITE_ROOT:=${WEB_ROOT:-/var/www/rproxy-server}}"
: "${SITE_CONF:=${CONFIG_DIR:-/opt/rproxy-server/config}/site.conf}"

SITE_PICK_MAX=7   # 首页公开展示的影片数量上限（片库不足时全部展示）

SITE_NAME_PREFIX=(北岸 栖木 白塔 听潮 远山 星野 青屿 松间 云渡 回声 澄湖 晚风)
SITE_NAME_SUFFIX=(放映室 影音 影库 影院 媒体库 视听室)
# 主题色：强调色|强调色上的文字色
SITE_PALETTES=("#5aa9e6|#06121e" "#e8b04a|#1a1406" "#7bc47f|#0a1a0c" "#e07a5f|#1f0d08" "#b794f4|#150a24" "#4fd1c5|#031a18")

# 当前网站目录的状态：legacy（旧 LibreSpeed 测速站）/ managed（本项目生成）/ empty / custom（用户自己的）
site_state() {
    if [[ -f "${SITE_ROOT}/empty.php" && -f "${SITE_ROOT}/garbage.php" ]]; then
        echo legacy
    elif [[ -f "$SITE_CONF" ]]; then
        echo managed
    elif [[ -z "$(ls -A "$SITE_ROOT" 2>/dev/null | grep -v '^\.well-known$')" ]]; then
        echo empty
    else
        echo custom
    fi
}

# 清掉网站目录下除 .well-known（证书续期验证目录）外的全部内容
site_clear() {
    [[ -d "$SITE_ROOT" ]] || return 0
    find "$SITE_ROOT" -mindepth 1 -maxdepth 1 ! -name '.well-known' -exec rm -rf {} +
}

# site_gen_conf <模板目录>：随机生成站名、主题色与片单，写入 ${SITE_CONF}
site_gen_conf() {
    local tpl="$1" name pal accent fg slugs
    name="${SITE_NAME_PREFIX[RANDOM % ${#SITE_NAME_PREFIX[@]}]}${SITE_NAME_SUFFIX[RANDOM % ${#SITE_NAME_SUFFIX[@]}]}"
    pal="${SITE_PALETTES[RANDOM % ${#SITE_PALETTES[@]}]}"
    accent="${pal%%|*}"
    fg="${pal##*|}"
    slugs=$(cut -f1 "${tpl}/films.tsv" | grep -v '^$' | shuf | head -n "$SITE_PICK_MAX" | paste -sd, -)
    [[ -n "$slugs" ]] || die "影片列表为空：${tpl}/films.tsv"
    mkdir -p "$(dirname "$SITE_CONF")"
    cat > "$SITE_CONF" <<EOF
# 本项目生成的默认网站设置：升级时据此重新渲染，保持外观不变。删除此文件即视为自定义网站，升级不再覆盖。
SITE_NAME="${name}"
SITE_ACCENT="${accent}"
SITE_ACCENT_FG="${fg}"
SITE_FILMS="${slugs}"
EOF
}

# JS 字符串字面量：转义反斜杠和双引号
site_js_str() {
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    printf '"%s"' "$s"
}

# site_films_js <模板目录> <逗号分隔的 slug>：按 slug 顺序输出 JS 对象字面量（每行一部），跳过片库里已不存在的
site_films_js() {
    local tpl="$1" slugs="$2" slug f1 f2 f3 f4 f5 f6 f7 f8
    for slug in ${slugs//,/ }; do
        f1=""
        IFS=$'\t' read -r f1 f2 f3 f4 f5 f6 f7 f8 < <(awk -F'\t' -v s="$slug" '$1 == s' "${tpl}/films.tsv") || true
        [[ -n "$f1" ]] || continue
        printf '  { slug: %s, t: %s, en: %s, y: %s, g: %s, len: %s, d: %s, lic: %s },\n' \
            "$(site_js_str "$f1")" "$(site_js_str "$f2")" "$(site_js_str "$f3")" "$(site_js_str "$f4")" \
            "$(site_js_str "$f5")" "$(site_js_str "$f6")" "$(site_js_str "$f7")" "$(site_js_str "$f8")"
    done
}

# site_render <模板目录>：按 ${SITE_CONF} 把默认网站渲染到 ${SITE_ROOT}（不碰 .well-known）
site_render() {
    local tpl="$1" films_js tmp_js year
    [[ -f "${tpl}/index.html.tpl" ]] || die "找不到网站模板：${tpl}/index.html.tpl"
    # shellcheck disable=SC1090
    source "$SITE_CONF"
    films_js=$(site_films_js "$tpl" "$SITE_FILMS")
    if [[ -z "$films_js" ]]; then
        # 设置里的影片都已不在新版片库里：重新挑选，站名和配色一并重新生成
        site_gen_conf "$tpl"
        # shellcheck disable=SC1090
        source "$SITE_CONF"
        films_js=$(site_films_js "$tpl" "$SITE_FILMS")
    fi

    mkdir -p "$SITE_ROOT"
    rm -rf "${SITE_ROOT}/posters"
    rm -f "${SITE_ROOT}/index.html" "${SITE_ROOT}/404.html" "${SITE_ROOT}/robots.txt" "${SITE_ROOT}/favicon.ico"
    mkdir -p "${SITE_ROOT}/posters"

    year=$(date +%Y)
    tmp_js=$(mktemp)
    printf '%s\n' "$films_js" > "$tmp_js"
    sed -e "s|{{SITE_NAME}}|${SITE_NAME}|g" -e "s|{{ACCENT}}|${SITE_ACCENT}|g" \
        -e "s|{{ACCENT_FG}}|${SITE_ACCENT_FG}|g" -e "s|{{YEAR}}|${year}|g" "${tpl}/index.html.tpl" \
        | awk -v f="$tmp_js" '/\{\{FILMS_JS\}\}/ { while ((getline l < f) > 0) print l; next } { print }' \
        > "${SITE_ROOT}/index.html"
    sed -e "s|{{SITE_NAME}}|${SITE_NAME}|g" -e "s|{{ACCENT}}|${SITE_ACCENT}|g" \
        "${tpl}/404.html.tpl" > "${SITE_ROOT}/404.html"
    rm -f "$tmp_js"
    cp "${tpl}/robots.txt" "${tpl}/favicon.ico" "${SITE_ROOT}/"

    local slug
    for slug in ${SITE_FILMS//,/ }; do
        [[ -f "${tpl}/posters/${slug}.jpg" ]] && cp "${tpl}/posters/${slug}.jpg" "${SITE_ROOT}/posters/"
    done
    chmod -R a+rX "$SITE_ROOT"
    chown -R www-data:www-data "$SITE_ROOT" 2>/dev/null || true
}

# site_deploy <模板目录>：安装与升级都调用它。按当前状态决定怎么处理
site_deploy() {
    local tpl="$1"
    case "$(site_state)" in
        custom)
            log_info "检测到自定义网站（${SITE_ROOT}），保持不动"
            ;;
        legacy)
            log_step "把旧的 LibreSpeed 测速站替换为新的默认网站"
            site_clear
            site_gen_conf "$tpl"
            site_render "$tpl"
            log_done "默认网站已部署到 ${SITE_ROOT}"
            ;;
        empty)
            log_step "部署默认网站"
            site_gen_conf "$tpl"
            site_render "$tpl"
            log_done "默认网站已部署到 ${SITE_ROOT}"
            ;;
        managed)
            log_step "更新默认网站（外观沿用 ${SITE_CONF}）"
            site_render "$tpl"
            log_done "默认网站已更新"
            ;;
    esac
}

# 把当前网站目录整体备份到旁边（不在 Web 根目录里，不会被公开访问）；目录为空时不备份
site_backup() {
    [[ -n "$(ls -A "$SITE_ROOT" 2>/dev/null)" ]] || return 0
    local bak="${SITE_ROOT}.bak-$(date +%Y%m%d-%H%M%S)"
    cp -a "$SITE_ROOT" "$bak" && log_info "已备份原网站到 ${bak}"
}

# site_import <zip 文件或目录>：换上用户自己的网站（先备份）。之后升级不再覆盖
# 注意：自定义网站的未知路径由 nginx 返回 404（不再兜底到 index.html），单页应用需自行处理
site_import() {
    local src="$1" work="" content subs
    [[ -n "$src" && -e "$src" ]] || die "找不到 ${src:-<未指定>}"
    if [[ -d "$src" ]]; then
        content="$src"
    else
        command -v unzip >/dev/null || die "缺少 unzip，无法解压 ${src}"
        work=$(mktemp -d)
        unzip -q "$src" -d "$work" || { rm -rf "$work"; die "解压失败：${src}"; }
        content="$work"
    fi
    # 压缩包里常见多包一层目录：只有一个子目录且根上没有 index.html 时，进入那个子目录
    if [[ ! -f "${content}/index.html" ]]; then
        subs=$(find "$content" -mindepth 1 -maxdepth 1 -type d ! -name '__MACOSX')
        if [[ -n "$subs" && $(wc -l <<<"$subs") -eq 1 && -f "${subs}/index.html" ]]; then
            content="$subs"
        fi
    fi
    if [[ ! -f "${content}/index.html" ]]; then
        [[ -n "$work" ]] && rm -rf "$work"
        die "网站根目录缺少 index.html：${src}"
    fi

    site_backup
    mkdir -p "$SITE_ROOT"
    site_clear
    cp -a "${content}/." "${SITE_ROOT}/"
    rm -f "$SITE_CONF"   # 没有 site.conf：升级时识别为自定义网站，不覆盖
    chmod -R a+rX "$SITE_ROOT"
    chown -R www-data:www-data "$SITE_ROOT" 2>/dev/null || true
    [[ -n "$work" ]] && rm -rf "$work"
    log_done "已导入网站到 ${SITE_ROOT}（升级时不会被覆盖）"
}

# site_reset <模板目录>：换回默认网站（先备份当前网站），站名、配色、片单重新随机
site_reset() {
    local tpl="$1"
    site_backup
    mkdir -p "$SITE_ROOT"
    site_clear
    site_gen_conf "$tpl"
    site_render "$tpl"
    log_done "已恢复为默认网站"
}

# 停用（不卸载）PHP-FPM：新版网站不再需要它。不 apt purge / autoremove：
# 机器上可能有用户自己的服务依赖 PHP 或相关软件包，误删的代价远大于省下的几十 MB 磁盘
site_disable_php() {
    local unit
    while read -r unit _; do
        [[ -n "$unit" ]] || continue
        if systemctl disable --now "$unit" >/dev/null 2>&1; then
            log_info "已停用 ${unit}（软件包未卸载）"
        fi
    done < <(systemctl list-unit-files 'php*-fpm.service' --no-legend 2>/dev/null)
    return 0
}
