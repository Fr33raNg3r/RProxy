#!/bin/bash
# site.sh：状态判定、随机生成、渲染、部署（升级时外观不变、旧测速站被替换、自定义站不动）
HERE="$(cd "$(dirname "$0")" && pwd)"
SERVER="$(cd "$HERE/.." && pwd)"
source "$HERE/lib.sh"

# site.sh 依赖的日志函数：测试里用无输出的替身；die 退出当前（子）shell
log_info() { :; }; log_step() { :; }; log_done() { :; }; log_warn() { :; }
die() { echo "DIE: $*" >&2; exit 1; }

FIX="$HERE/fixtures/site"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
export SITE_ROOT="$tmp/www" SITE_CONF="$tmp/config/site.conf"
source "$SERVER/scripts/site.sh"

reset_site() { rm -rf "$SITE_ROOT" "$tmp/config" "$tmp"/www.bak-*; mkdir -p "$SITE_ROOT"; }

echo "-- 状态判定"
reset_site;                                             assert_eq "$(site_state)" empty   "空目录 → empty"
mkdir -p "$SITE_ROOT/.well-known/acme-challenge";       assert_eq "$(site_state)" empty   "只有 .well-known → empty"
echo hi > "$SITE_ROOT/index.html";                      assert_eq "$(site_state)" custom  "有别的文件 → custom"
touch "$SITE_ROOT/empty.php" "$SITE_ROOT/garbage.php";  assert_eq "$(site_state)" legacy  "测速站文件 → legacy"
reset_site; mkdir -p "$tmp/config"; echo 'SITE_NAME="x"' > "$SITE_CONF"
assert_eq "$(site_state)" managed "有 site.conf → managed"

echo "-- 空目录首次部署"
reset_site
site_deploy "$FIX"
assert_file "$SITE_CONF" "生成了 site.conf"
assert_file "$SITE_ROOT/index.html" "渲染了 index.html"
assert_file "$SITE_ROOT/404.html" "渲染了 404.html"
assert_file "$SITE_ROOT/robots.txt" "复制了 robots.txt"
assert_file "$SITE_ROOT/favicon.ico" "复制了 favicon.ico"
assert_no_grep '\{\{' "$SITE_ROOT/index.html" "index.html 没有遗留占位符"
assert_no_grep '\{\{' "$SITE_ROOT/404.html" "404.html 没有遗留占位符"
assert_eq "$(ls "$SITE_ROOT/posters" | wc -l | tr -d ' ')" 3 "三部影片的海报都复制了"
assert_no_file "$SITE_ROOT/site.conf" "site.conf 不在 Web 根目录（否则会被公开下载）"
source "$SITE_CONF"
assert_grep "$SITE_NAME" "$SITE_ROOT/index.html" "页面里是 site.conf 的站名"
assert_grep "^SITE_ACCENT=\"#[0-9a-f]{6}\"" "$SITE_CONF" "主题色是 #rrggbb"

echo "-- 影片信息写成合法的 JS（含引号与反斜杠）"
if command -v node >/dev/null; then
    js=$(awk '/^<script>$/{f=1;next} /^<\/script>$/{f=0} f' "$SITE_ROOT/index.html")
    got=$(node -e 'eval(require("fs").readFileSync(0,"utf8") + ";console.log(FILMS.length + \"|\" + FILMS.find(f => f.slug === \"alpha\").d)")' <<<"$js")
    assert_eq "$got" '3|含"引号"与\反斜杠的简介' "JS 数组可解析，特殊字符转义正确"
else
    echo "  （跳过：没有 node）"
fi

echo "-- 再次部署（升级）：外观保持不变"
sed -i 's/^SITE_NAME=.*/SITE_NAME="测试站"/' "$SITE_CONF"
conf_before=$(cat "$SITE_CONF")
site_deploy "$FIX"
assert_grep '测试站' "$SITE_ROOT/index.html" "按 site.conf 重新渲染，站名沿用"
assert_eq "$(cat "$SITE_CONF")" "$conf_before" "site.conf 未被改写"

echo "-- site.conf 里的影片已不在新版片库：重新挑选"
sed -i 's/^SITE_FILMS=.*/SITE_FILMS="gone1,gone2"/' "$SITE_CONF"
site_deploy "$FIX"
assert_eq "$([[ $(ls "$SITE_ROOT/posters" | wc -l) -ge 1 ]] && echo yes)" yes "重新挑选后至少有一部影片"

echo "-- 旧的 LibreSpeed 测速站被替换，证书续期验证目录保留"
reset_site
touch "$SITE_ROOT/empty.php" "$SITE_ROOT/garbage.php"; echo old > "$SITE_ROOT/index.html"
mkdir -p "$SITE_ROOT/.well-known/acme-challenge"; echo token > "$SITE_ROOT/.well-known/acme-challenge/t1"
site_deploy "$FIX"
assert_no_file "$SITE_ROOT/empty.php" "empty.php 已删除"
assert_no_file "$SITE_ROOT/garbage.php" "garbage.php 已删除"
assert_no_grep '^old$' "$SITE_ROOT/index.html" "旧首页已被替换"
assert_file "$SITE_ROOT/.well-known/acme-challenge/t1" ".well-known 保留"
assert_file "$SITE_CONF" "生成了 site.conf"

echo "-- 用户自定义网站不动"
reset_site
echo mine > "$SITE_ROOT/index.html"
site_deploy "$FIX"
assert_eq "$(cat "$SITE_ROOT/index.html")" mine "自定义首页原样保留"
assert_no_file "$SITE_CONF" "没有生成 site.conf"

echo "-- 导入自己的网站（目录）"
reset_site
site_deploy "$FIX"
mkdir -p "$SITE_ROOT/.well-known/acme-challenge"; echo token > "$SITE_ROOT/.well-known/acme-challenge/t1"
mkdir -p "$tmp/mysite/css"; echo '<h1>mine</h1>' > "$tmp/mysite/index.html"; echo x > "$tmp/mysite/css/a.css"
site_import "$tmp/mysite"
assert_grep '<h1>mine</h1>' "$SITE_ROOT/index.html" "导入后首页是用户的"
assert_file "$SITE_ROOT/css/a.css" "子目录一并导入"
assert_no_file "$SITE_ROOT/posters" "旧默认网站的内容已清掉"
assert_no_file "$SITE_CONF" "删除 site.conf，之后升级视为自定义网站"
assert_file "$SITE_ROOT/.well-known/acme-challenge/t1" ".well-known 保留"
assert_eq "$(ls -d "$tmp"/www.bak-* | wc -l | tr -d ' ')" 1 "导入前备份了原网站"
assert_eq "$(site_state)" custom "导入后状态为 custom"

echo "-- 导入 zip（多包了一层目录）"
reset_site
( cd "$tmp" && PYTHONUTF8=1 python -I -c "
import zipfile
z = zipfile.ZipFile('s.zip', 'w')
z.writestr('mysite/index.html', '<h1>zip</h1>')
z.writestr('mysite/a.css', 'x')
z.close()" )
site_import "$tmp/s.zip"
assert_grep '<h1>zip</h1>' "$SITE_ROOT/index.html" "zip 里的网站已展开到根目录"
assert_file "$SITE_ROOT/a.css" "zip 内其他文件一并导入"

echo "-- 导入失败的情形"
reset_site; echo keep > "$SITE_ROOT/index.html"; mkdir -p "$tmp/noindex"; echo x > "$tmp/noindex/a.css"
( site_import "$tmp/noindex" ) 2>/dev/null; assert_eq "$?" 1 "缺少 index.html 时拒绝导入"
assert_eq "$(cat "$SITE_ROOT/index.html")" keep "拒绝导入时原网站不动"
( site_import "$tmp/不存在" ) 2>/dev/null; assert_eq "$?" 1 "路径不存在时拒绝导入"

echo "-- 恢复默认网站"
reset_site; echo mine > "$SITE_ROOT/index.html"
site_reset "$FIX"
assert_file "$SITE_CONF" "重新生成了 site.conf"
assert_grep '<title>' "$SITE_ROOT/index.html" "首页换回默认网站"
assert_eq "$(site_state)" managed "重置后状态为 managed"
assert_eq "$(ls -d "$tmp"/www.bak-* | wc -l | tr -d ' ')" 1 "重置前备份了原网站"

echo "-- 停用 PHP-FPM（只停用，不卸载）"
mkdir -p "$tmp/bin"
cat > "$tmp/bin/systemctl" <<'EOF'
#!/bin/bash
echo "systemctl $*" >> "$STUB_LOG"
[[ "$1" == "list-unit-files" ]] && echo "php8.4-fpm.service enabled enabled"
exit 0
EOF
cat > "$tmp/bin/apt-get" <<'EOF'
#!/bin/bash
echo "apt-get $*" >> "$STUB_LOG"
EOF
cp "$tmp/bin/apt-get" "$tmp/bin/apt"
chmod +x "$tmp/bin/"*
export STUB_LOG="$tmp/stub.log"; : > "$STUB_LOG"
PATH="$tmp/bin:$PATH" site_disable_php
assert_grep 'disable --now php8\.4-fpm\.service' "$STUB_LOG" "停用并禁用了 php8.4-fpm"
assert_no_grep 'apt' "$STUB_LOG" "没有调用 apt（不卸载软件包）"

finish
