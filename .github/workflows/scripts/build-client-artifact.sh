#!/usr/bin/env bash
# 在 CI 上编译 client 的 Go 后端 + Vue 前端，打包为可直接部署的 tarball。
# 调用方式: build-client-artifact.sh <version>
# 例: build-client-artifact.sh 1.1.3
# 产出: ./dist/client-v<version>-linux-amd64.tar.gz
#
# 包内布局（install.sh 解压到 BUILD_DIR 即可用）:
#   bin/webui                # 已编译 Go 二进制 (linux/amd64)
#   www/                     # 已编译 Vue 静态资源
#   scripts/                 # 部署期 shell 脚本
#   configs/                 # nft / systemd / 模板等
#   install.sh
#   VERSION
set -euo pipefail

version="${1:?用法: $0 <version>  例: $0 1.1.3}"
arch="linux-amd64"
artifact="client-v${version}-${arch}.tar.gz"

repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$repo_root"

echo "==> repo_root=$repo_root  version=$version  arch=$arch"

# ---------- 1) Go 后端 ----------
echo "==> 编译 Go 后端"
pushd client/webui-backend >/dev/null
# 与原 install.sh 中的 build_webui 保持一致：trimpath + 去符号
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /tmp/webui .
popd >/dev/null

# ---------- 2) Vue 前端 ----------
echo "==> 编译 Vue 前端"
pushd client/webui-frontend >/dev/null
npm ci --no-audit --no-fund
npm run build
popd >/dev/null

# ---------- 3) 组装 staging 目录 ----------
echo "==> 组装 staging 目录"
staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT

mkdir -p "${staging}/bin" "${staging}/www"
install -m 0755 /tmp/webui "${staging}/bin/webui"
cp -a client/webui-frontend/dist/. "${staging}/www/"

# 部署期需要的非源码资产
cp -a client/scripts  "${staging}/scripts"
cp -a client/configs  "${staging}/configs"
cp -a client/install.sh "${staging}/install.sh"
cp -a VERSION         "${staging}/VERSION"

# 一个简单的 manifest，让用户/调试者一眼看出包是什么
cat > "${staging}/MANIFEST" <<EOF
component: client
version: ${version}
arch: ${arch}
built_at: $(date -u +%Y-%m-%dT%H:%M:%SZ)
git_sha: ${GITHUB_SHA:-unknown}
go_version: $(go version)
node_version: $(node --version)
EOF

# ---------- 4) 打包 ----------
mkdir -p dist
echo "==> 打包 ${artifact}"
tar -C "$staging" -czf "dist/${artifact}" .
ls -lh "dist/${artifact}"

echo "==> 完成。产物路径: dist/${artifact}"
