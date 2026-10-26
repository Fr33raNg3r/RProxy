#!/bin/bash
# nginx 模板：不再含 PHP；XHTTP 路径的 400 改写为站点 404；不写 http 层；渲染后无遗留占位符
HERE="$(cd "$(dirname "$0")" && pwd)"
SERVER="$(cd "$HERE/.." && pwd)"
source "$HERE/lib.sh"
source "$SERVER/scripts/common.sh"

tpl="$SERVER/configs/nginx/rproxy-server.conf.tpl"
body=$(grep -vE '^\s*#' "$tpl")

assert_no_match 'php|fastcgi|PHP_SOCK' "$body" "模板不再含 PHP / fastcgi"
assert_no_match 'try_files \$uri \$uri/ /index\.html' "$body" "不再把任意路径兜底到首页"
assert_match 'grpc_intercept_errors on' "$body" "XHTTP 路径开启 grpc_intercept_errors"
assert_match 'error_page 400 404 405 = @site_404' "$body" "XHTTP 的 400/404/405 改写为站点 404"
assert_match 'location = \{\{WS_PATH\}\} *\{' "$body" "无结尾斜杠的 XHTTP 路径单独处理（消除 301）"
assert_match 'location = /api/auth/login' "$body" "登录接口返回 401"
assert_match 'client_max_body_size 1m' "$body" "443 server 块限制请求体 1m"
assert_match 'client_max_body_size 0' "$body" "XHTTP 路径放开请求体"
assert_match 'X-Robots-Tag' "$body" "全站 X-Robots-Tag"
assert_match 'access_log off' "$body" "关闭访问日志"
assert_no_match '^\s*http\s*\{' "$body" "不写 http 层"

# XHTTP location 里必须有 grpc_pass，且不能直接 return 404（那样请求到不了 Xray，watchdog 看不到 Xray 故障）
xhttp_block=$(awk '/location \{\{WS_PATH\}\}\/ *\{/{f=1} f{print} f && /^    \}/{exit}' "$tpl")
assert_match 'grpc_pass grpc://127\.0\.0\.1:9891' "$xhttp_block" "XHTTP location 转发给 Xray"
assert_no_match 'return 404' "$xhttp_block" "XHTTP location 不直接 return 404"

# 渲染
out=$(render_nginx_conf "$tpl" example.test /abc123)
assert_no_match '\{\{' "$out" "渲染后没有遗留占位符"
assert_match 'server_name example\.test;' "$out" "渲染了域名"
assert_match 'location /abc123/ *\{' "$out" "渲染了 XHTTP 路径"
open=$(tr -cd '{' <<<"$out" | wc -c)
close=$(tr -cd '}' <<<"$out" | wc -c)
assert_eq "$open" "$close" "花括号成对"

finish
