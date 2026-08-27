#!/usr/bin/env bash
# 把 server/ 目录打包为可直接部署的 tarball。
# server 端没有需要编译的二进制（纯 shell + nginx 配置），
# 所以这里只是个简单的打包脚本——和 client 保持一致的发布模型。
#
# 调用方式: build-server-artifact.sh <version>
# 例: build-server-artifact.sh 1.1.4
# 产出: ./dist/server-v<version>.tar.gz
set -euo pipefail

version="${1:?用法: $0 <version>  例: $0 1.1.4}"
artifact="server-v${version}.tar.gz"

repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$repo_root"

echo "==> repo_root=$repo_root  version=$version"

staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT

cp -a server/scripts  "${staging}/scripts"
cp -a server/configs  "${staging}/configs"
cp -a server/install.sh "${staging}/install.sh"
cp -a VERSION         "${staging}/VERSION"

cat > "${staging}/MANIFEST" <<EOF
component: server
version: ${version}
arch: noarch
built_at: $(date -u +%Y-%m-%dT%H:%M:%SZ)
git_sha: ${GITHUB_SHA:-unknown}
EOF

# CRLF 校验：见 build-client-artifact.sh 里的说明。
echo "==> 校验换行符（不允许 CRLF）"
crlf=""
while IFS= read -r f; do
    if [ "$(tr -cd '\r' < "$f" | wc -c)" -gt 0 ]; then
        crlf="${crlf}${f}
"
    fi
done < <(find "${staging}/scripts" "${staging}/configs" "${staging}/install.sh" -type f)
if [ -n "$crlf" ]; then
    echo "ERR: 以下文件含 CRLF 换行符，打包中止：" >&2
    printf '%s' "$crlf" >&2
    exit 2
fi

mkdir -p dist
echo "==> 打包 ${artifact}"

# --mode=755: 见 build-client-artifact.sh 里的说明（NTFS 不保存 Unix 可执行位）。
tar -C "$staging" --mode=755 -czf "dist/${artifact}" .
ls -lh "dist/${artifact}"

echo "==> 完成。产物路径: dist/${artifact}"
