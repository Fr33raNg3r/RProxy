#!/bin/bash
# 接线静态检查：install.sh / rproxy-server 已切换到新的网站与 nginx 流程，旧的 PHP / 测速站残留已清除
HERE="$(cd "$(dirname "$0")" && pwd)"
SERVER="$(cd "$HERE/.." && pwd)"
source "$HERE/lib.sh"

I="$SERVER/install.sh"
R="$SERVER/scripts/rproxy-server"

echo "-- 语法"
for f in "$I" "$R" "$SERVER"/scripts/*.sh "$SERVER/scripts/rproxy-rollback"; do
    bash -n "$f" 2>/dev/null; assert_eq "$?" 0 "bash -n $(basename "$f")"
done

echo "-- install.sh"
# 只看代码行：注释里会解释"为什么停用而不卸载 php-fpm""为什么替换旧测速站"，不应算残留
i_code=$(grep -vE '^\s*#' "$I")
assert_no_match 'php-fpm|php-cli|php-curl' "$i_code" "不再安装 PHP 软件包"
assert_no_match 'deploy_librespeed' "$i_code" "不再有 deploy_librespeed"
assert_no_grep 'PHP_SOCK|php_sock' "$I" "不再处理 PHP socket"
assert_grep 'deploy_site' "$I" "调用 deploy_site"
assert_grep 'site_disable_php' "$I" "升级时停用 PHP"
assert_grep 'render_nginx_conf' "$I" "用 render_nginx_conf 渲染 nginx"
assert_grep 'nginx_config_clean' "$I" "改 nginx 前先检查整体配置"
assert_grep 'site-template' "$I" "部署默认网站模板副本"
assert_no_grep 'apt(-get)? (purge|autoremove|remove)' "$I" "不卸载任何软件包"
assert_no_match 'LibreSpeed 测速' "$i_code" "完成页不再提测速站"
assert_no_file "$SERVER/configs/librespeed" "configs/librespeed 已删除"

echo "-- rproxy-server"
assert_no_grep 'PHP_SOCK|php_sock' "$R" "不再处理 PHP socket"
assert_grep 'render_nginx_conf' "$R" "用 render_nginx_conf 渲染 nginx"
assert_grep 'nginx_config_clean' "$R" "改 nginx 前先检查整体配置"
assert_grep '^cmd_site\(\)' "$R" "有 cmd_site"
assert_grep '^\s*site\)\s+shift; cmd_site' "$R" "入口分发了 site 子命令"
assert_grep 'site import' "$R" "帮助里有 site import"
assert_grep 'site reset' "$R" "帮助里有 site reset"

finish
