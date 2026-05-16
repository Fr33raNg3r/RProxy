#!/usr/bin/env bash
# 给指定组件 (client / server) 打 tag + 创建 GitHub Release
# 调用方式: make-release.sh client | make-release.sh server
# 要求环境: GH_TOKEN, 运行在 actions/checkout 出来的工作树里
set -euo pipefail

component="${1:-}"
case "$component" in
  client|server) ;;
  *) echo "ERR: 必须指定 client 或 server"; exit 2 ;;
esac

version_file="${component}/VERSION"
[[ -f "$version_file" ]] || { echo "ERR: 找不到 $version_file"; exit 2; }

version="$(tr -d '[:space:]' < "$version_file")"
[[ -n "$version" ]] || { echo "ERR: $version_file 为空"; exit 2; }
# 简单校验：必须 X.Y.Z 数字
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERR: $version_file 内容不是合法版本号: '$version'"
  exit 2
fi

tag="${component}-v${version}"

# 已存在则跳过——常见于重跑工作流
if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  echo "==> tag ${tag} 已存在，跳过"
  exit 0
fi
# 远端是否已有同名 tag（罕见但可能：本地未拉到）
if git ls-remote --tags origin "refs/tags/${tag}" | grep -q "${tag}$"; then
  echo "==> 远端已有 tag ${tag}，跳过"
  exit 0
fi

git config user.name  "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git tag -a "$tag" -m "$tag"
git push origin "$tag"

# 找上一个同前缀 tag 作为 changelog 起点
prev_tag="$(git tag --list "${component}-v*" --sort=-version:refname | sed -n '2p' || true)"

notes_file="$(mktemp)"
{
  echo "## ${component^} ${version}"
  echo
  if [[ -n "$prev_tag" ]]; then
    echo "### 变更（自 ${prev_tag} 起）"
    # 只取动了对应组件目录或顶层文件的提交，减少噪音
    git log "${prev_tag}..${tag}" --pretty=format:'- %s （%h）' -- "${component}/" "VERSION" ".github/" | head -200
  else
    echo "首个发布版本。"
  fi
  echo
  echo
  echo "### 安装"
  echo
  if [[ "$component" == "client" ]]; then
    echo '```bash'
    echo "wget -O- https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/main/client/install.sh | bash -s -- install v${version}"
    echo '```'
  else
    echo '```bash'
    echo "wget -O- https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/main/server/install.sh | bash -s -- install v${version}"
    echo '```'
  fi
} > "$notes_file"

gh release create "$tag" \
  --title "${component^} ${version}" \
  --notes-file "$notes_file" \
  --target main
