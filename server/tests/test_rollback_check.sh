#!/bin/bash
# rproxy-rollback check --xhttp：XHTTP 路径返回 400（改造前）或 404（改造后）都算通过，502 / 无响应 / 200 不通过
HERE="$(cd "$(dirname "$0")" && pwd)"
SERVER="$(cd "$HERE/.." && pwd)"
source "$HERE/lib.sh"

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/opt/rproxy-server/config"
echo '{}' > "$tmp/opt/rproxy-server/config/server.json"

# 假命令：systemctl 一律成功；jq 按过滤式返回域名/路径；curl 按 URL 返回状态码
cat > "$tmp/bin/systemctl" <<'EOF'
#!/bin/bash
exit 0
EOF
cat > "$tmp/bin/jq" <<'EOF'
#!/bin/bash
case "$*" in
    *.domain*)  echo example.test ;;
    *.ws_path*) echo /abc123 ;;
esac
EOF
cat > "$tmp/bin/curl" <<'EOF'
#!/bin/bash
for a in "$@"; do url="$a"; done
case "$url" in
    */abc123/) printf '%s' "${XHTTP_CODE:-400}" ;;
    *)         printf '200' ;;
esac
EOF
chmod +x "$tmp/bin/"*

run_check() {
    PATH="$tmp/bin:$PATH" RPROXY_ROOT="$tmp" XHTTP_CODE="$1" \
        bash "$SERVER/scripts/rproxy-rollback" check --xhttp >/dev/null 2>&1
    echo $?
}

assert_eq "$(run_check 400)" 0 "XHTTP 路径 400 通过（旧 nginx 配置）"
assert_eq "$(run_check 404)" 0 "XHTTP 路径 404 通过（新 nginx 配置）"
assert_eq "$(run_check 502)" 1 "XHTTP 路径 502 不通过（Xray 不通）"
assert_eq "$(run_check 000)" 1 "XHTTP 路径无响应不通过"
assert_eq "$(run_check 200)" 1 "XHTTP 路径 200 不通过"

finish
