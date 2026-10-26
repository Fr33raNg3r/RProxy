#!/bin/bash
# 真实的默认网站模板与素材：每部影片信息完整、有海报和署名；页面模板占位符齐全；渲染后内联脚本语法正确
HERE="$(cd "$(dirname "$0")" && pwd)"
SERVER="$(cd "$HERE/.." && pwd)"
source "$HERE/lib.sh"

log_info() { :; }; log_step() { :; }; log_done() { :; }; log_warn() { :; }
die() { echo "DIE: $*" >&2; exit 1; }

T="$SERVER/configs/site"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

echo "-- 片单"
n=$(grep -c . "$T/films.tsv")
assert_eq "$([[ $n -ge 6 ]] && echo yes)" yes "至少 6 部影片（实际 ${n}）"
bad=$(awk -F'\t' 'NF != 8 { print NR }' "$T/films.tsv" | paste -sd, -)
assert_eq "$bad" "" "每行恰好 8 个字段"
empty=$(awk -F'\t' '{ for (i = 1; i <= 8; i++) if ($i == "") print NR }' "$T/films.tsv" | paste -sd, -)
assert_eq "$empty" "" "没有空字段"
while IFS=$'\t' read -r slug _; do
    assert_file "$T/posters/${slug}.jpg" "${slug} 有海报"
    assert_grep "^${slug}	" "$T/CREDITS.tsv" "${slug} 有署名记录"
done < "$T/films.tsv"
assert_no_grep '(^|	)(CC )?BY-(SA|NC|ND)|CC BY-(SA|NC|ND)' "$T/CREDITS.tsv" "署名记录里没有 SA/NC/ND 授权"

echo "-- 文本素材使用 LF 换行（CRLF 会让 Linux 上的 awk / grep 读到带 \\r 的字段，发布打包也会被拦）"
for f in films.tsv CREDITS.tsv index.html.tpl 404.html.tpl robots.txt; do
    assert_eq "$(tr -cd '\r' < "$T/$f" | wc -c | tr -d ' ')" 0 "$f 没有 CR 字符"
done

echo "-- 页面模板"
for ph in 'SITE_NAME' 'ACCENT' 'ACCENT_FG' 'YEAR' 'FILMS_JS'; do
    assert_grep "\\{\\{${ph}\\}\\}" "$T/index.html.tpl" "index.html.tpl 含 {{${ph}}}"
done
assert_grep '/api/auth/login' "$T/index.html.tpl" "登录表单请求 /api/auth/login"
assert_no_grep 'https?://(fonts|cdn|ajax)\.' "$T/index.html.tpl" "不引用外部字体或脚本"
assert_eq "$(head -c 4 "$T/robots.txt")" "User" "robots.txt 内容"
assert_eq "$(od -An -tx1 -N4 "$T/favicon.ico" | tr -d ' \n')" "00000100" "favicon.ico 是 ICO 格式"
assert_eq "$([[ $(wc -c < "$T/favicon.ico") -lt 8192 ]] && echo yes)" yes "favicon.ico 小于 8KB"

echo "-- 渲染真实模板"
export SITE_ROOT="$tmp/www" SITE_CONF="$tmp/config/site.conf"
source "$SERVER/scripts/site.sh"
site_deploy "$T"
assert_no_grep '\{\{' "$SITE_ROOT/index.html" "index.html 没有遗留占位符"
assert_no_grep '\{\{' "$SITE_ROOT/404.html" "404.html 没有遗留占位符"
source "$SITE_CONF"
shown=$(echo "$SITE_FILMS" | tr ',' '\n' | grep -c .)
assert_eq "$(ls "$SITE_ROOT/posters" | wc -l | tr -d ' ')" "$shown" "展示的每部影片都复制了海报"
if command -v node >/dev/null; then
    awk '/^<script>$/{f=1;next} /^<\/script>$/{f=0} f' "$SITE_ROOT/index.html" > "$tmp/inline.js"
    node --check "$tmp/inline.js" 2>/dev/null; assert_eq "$?" 0 "内联脚本语法正确"
else
    echo "  （跳过：没有 node）"
fi

finish
