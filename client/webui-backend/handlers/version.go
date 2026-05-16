package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// 版本检查状态（内存缓存，启动时拉一次）
var (
	versionMu        sync.RWMutex
	cachedCurrentVer string
	cachedLatestVer  string
)

const (
	versionFile = "/opt/tproxy-gw/VERSION"
	// 用 Releases API 而不是 raw VERSION 文件 —— 避免用户拿到尚未打 tag 的 main 分支中间版本。
	// per_page=30 足够找到最近一个 vX.Y.Z 前缀的 release（按发布时间倒序返回）。
	releasesURL = "https://api.github.com/repos/Fr33raNg3r/RProxy/releases?per_page=30"
	// v1.1.4 起 client/server 同步发布，tag 统一为 vX.Y.Z；
	// 之前一段时间用过 client-v* 分组件 tag，保留作为兼容前缀以便老 WebUI 升级期间能识别。
	releaseTagPrefix       = "v"
	releaseTagPrefixLegacy = "client-v"
)

// InitVersionCheck WebUI 启动时调用一次
// 立即读取本地版本号，5 秒后异步去 GitHub 拉最新版本
func InitVersionCheck() {
	// 立即读本地
	if data, err := os.ReadFile(versionFile); err == nil {
		versionMu.Lock()
		cachedCurrentVer = strings.TrimSpace(string(data))
		versionMu.Unlock()
	}
	// 5 秒后异步拉远程
	go func() {
		time.Sleep(5 * time.Second)
		v := fetchLatestClientRelease()
		if v != "" {
			versionMu.Lock()
			cachedLatestVer = v
			versionMu.Unlock()
		}
	}()
}

// fetchLatestClientRelease 查询 GitHub Releases API，返回最新的 client release 版本号
// （已去掉 "client-v" 前缀）。失败返回空串。
func fetchLatestClientRelease() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", releasesURL, nil)
	if err != nil {
		return ""
	}
	// 不带 token 的 API 调用有 60 次/小时/IP 速率限制，足以应对启动时的一次查询。
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	// 限制读取量，防御异常大响应
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return ""
	}
	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return ""
	}
	// API 按发布时间倒序返回，取第一个匹配前缀的即可。
	// 注意必须先严格校验是 vX.Y.Z 格式，否则 "v-anything" 也会被误识别。
	for _, r := range releases {
		if v := stripVersionPrefix(r.TagName); v != "" {
			return v
		}
	}
	return ""
}

// stripVersionPrefix 把 tag_name 校验并剥掉前缀，返回纯版本号 (e.g. "1.1.4")。
// 不是合法 tag 形式则返回空。
func stripVersionPrefix(tag string) string {
	for _, p := range []string{releaseTagPrefix, releaseTagPrefixLegacy} {
		if !strings.HasPrefix(tag, p) {
			continue
		}
		rest := strings.TrimPrefix(tag, p)
		// 只接受形如 1.1.4 的（避免误吞 "v-experimental" 等）
		if isSemverLike(rest) {
			return rest
		}
	}
	return ""
}

func isSemverLike(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// GetVersion GET /api/version
// 返回当前安装版本和 GitHub 最新版本（启动时拉的缓存）
func GetVersion(w http.ResponseWriter, r *http.Request) {
	versionMu.RLock()
	cur := cachedCurrentVer
	latest := cachedLatestVer
	versionMu.RUnlock()

	hasUpdate := false
	if latest != "" && cur != "" && latest != cur {
		hasUpdate = true
	}

	displayLatest := latest
	if displayLatest == "" {
		displayLatest = "未知"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current":    cur,
		"latest":     displayLatest,
		"has_update": hasUpdate,
	})
}

// TriggerUpgrade POST /api/upgrade
// 后台异步执行 install.sh upgrade
// 立即返回，1-2 分钟后 WebUI 自己会被升级流程重启
func TriggerUpgrade(w http.ResponseWriter, r *http.Request) {
	// 写一个标记文件，install.sh 升级时能识别"从 WebUI 触发"（可选）
	_ = os.WriteFile("/tmp/rproxy-upgrade-triggered", []byte(time.Now().Format(time.RFC3339)), 0600)

	// 异步执行 install.sh upgrade，输出到日志
	go func() {
		// 用 nohup + setsid，确保升级进程脱离 WebUI 的进程组
		// 即使 WebUI 在升级过程中被重启，升级进程不会被杀
		cmd := exec.Command("/bin/bash", "-c",
			"setsid nohup bash /opt/tproxy-gw/install.sh upgrade >> /opt/tproxy-gw/logs/upgrade.log 2>&1 < /dev/null &")
		cmd.SysProcAttr = nil
		_ = cmd.Start()
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "升级已启动，请等待 1-2 分钟后刷新页面。升级期间 WebUI 可能短暂不可用。",
	})
}
