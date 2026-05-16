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

mkdir -p dist
echo "==> 打包 ${artifact}"
tar -C "$staging" -czf "dist/${artifact}" .
ls -lh "dist/${artifact}"

echo "==> 完成。产物路径: dist/${artifact}"
