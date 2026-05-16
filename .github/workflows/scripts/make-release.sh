#!/usr/bin/env bash
# 给项目打 tag + 创建 GitHub Release，附带 client/server 两个产物。
# 调用方式: make-release.sh
# 要求环境: GH_TOKEN, 已经跑过 build-client-artifact.sh 和 build-server-artifact.sh
set -euo pipefail

version="$(tr -d '[:space:]' < VERSION)"
[[ -n "$version" ]] || { echo "ERR: VERSION 为空"; exit 2; }
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERR: VERSION 内容不是合法版本号: '$version'"
  exit 2
fi

tag="v${version}"

# 已存在则跳过——常见于重跑工作流
if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  echo "==> tag ${tag} 已存在，跳过"
  exit 0
fi
if git ls-remote --tags origin "refs/tags/${tag}" | grep -q "${tag}$"; then
  echo "==> 远端已有 tag ${tag}，跳过"
  exit 0
fi

# 校验产物都在
client_artifact="dist/client-v${version}-linux-amd64.tar.gz"
server_artifact="dist/server-v${version}.tar.gz"
for f in "$client_artifact" "$server_artifact"; do
  [[ -f "$f" ]] || { echo "ERR: 找不到产物 $f"; exit 2; }
done

git config user.name  "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git tag -a "$tag" -m "$tag"
git push origin "$tag"

# 找上一个 vX.Y.Z tag 作为 changelog 起点（兼容旧 client-v* / server-v* legacy tag）
prev_tag="$(git tag --list 'v*' --sort=-version:refname | sed -n '2p' || true)"
if [[ -z "$prev_tag" ]]; then
  prev_tag="$(git tag --list 'client-v*' --sort=-version:refname | sed -n '1p' || true)"
fi

notes_file="$(mktemp)"
{
  echo "## RProxy ${version}"
  echo
  if [[ -n "$prev_tag" ]]; then
    echo "### 变更（自 ${prev_tag} 起）"
    git log "${prev_tag}..${tag}" --pretty=format:'- %s （%h）' | head -200
  else
    echo "首个发布版本。"
  fi
  echo
  echo
  echo "### 安装"
  echo
  echo '**客户端（旁路由）：**'
  echo '```bash'
  echo "wget -O- https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/main/client/install.sh | bash -s -- install v${version}"
  echo '```'
  echo
  echo '**服务端（VPS）：**'
  echo '```bash'
  echo "wget -O- https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/main/server/install.sh | bash -s -- install v${version}"
  echo '```'
  echo
  echo "未指定版本号即装最新 release。"
} > "$notes_file"

gh release create "$tag" \
  --title "RProxy ${version}" \
  --notes-file "$notes_file" \
  --target main \
  "$client_artifact" "$server_artifact"
