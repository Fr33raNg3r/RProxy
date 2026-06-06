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

// RefreshLatest POST /api/version/refresh
// 手动重新拉取 GitHub 最新版本（不依赖启动时的一次性缓存）。
func RefreshLatest(w http.ResponseWriter, r *http.Request) {
	v := fetchLatestClientRelease()
	if v != "" {
		versionMu.Lock()
		cachedLatestVer = v
		versionMu.Unlock()
	}
	GetVersion(w, r)
}

// upgradeLogPath 升级日志（与 TriggerUpgrade 中写入路径保持一致）
const upgradeLogPath = "/opt/tproxy-gw/logs/upgrade.log"

// upgradeDoneMarker install.sh 在 action_upgrade 末尾打印的成功标志。
const upgradeDoneMarker = "升级完成（"

// GetUpgradeLog GET /api/upgrade/log?offset=N
// 返回升级日志（从 offset 起的增量内容），并标识是否已完成。
// done=true 表示日志中已出现成功标志，前端可以提示"升级完毕"。
func GetUpgradeLog(w http.ResponseWriter, r *http.Request) {
	offsetStr := r.URL.Query().Get("offset")
	var offset int64
	if offsetStr != "" {
		// 简易解析，失败按 0 处理
		_, _ = fmtSscan(offsetStr, &offset)
	}

	st, err := os.Stat(upgradeLogPath)
	if err != nil {
		// 日志还没生成（升级刚触发，install.sh 尚未输出）
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"content": "",
			"size":    0,
			"done":    false,
		})
		return
	}
	size := st.Size()
	if offset > size {
		offset = 0 // 文件被截断（理论不应发生）
	}
	f, err := os.Open(upgradeLogPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"content": "",
			"size":    size,
			"done":    false,
		})
		return
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"content": "",
			"size":    size,
			"done":    false,
		})
		return
	}
	// 限制单次读取防御异常大文件
	body, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		body = nil
	}
	content := string(body)
	done := strings.Contains(content, upgradeDoneMarker)
	if !done {
		// offset>0 的增量请求里可能没拿到 marker（出现在更早位置）
		// 这里再扫一次整个文件末尾 64KB
		done = scanDoneMarker()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": content,
		"size":    offset + int64(len(body)),
		"done":    done,
	})
}

// scanDoneMarker 扫描日志末尾，判断是否已写入升级完成标志。
func scanDoneMarker() bool {
	st, err := os.Stat(upgradeLogPath)
	if err != nil {
		return false
	}
	f, err := os.Open(upgradeLogPath)
	if err != nil {
		return false
	}
	defer f.Close()
	const tailSize int64 = 64 * 1024
	start := st.Size() - tailSize
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, 0); err != nil {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(f, tailSize))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), upgradeDoneMarker)
}

// fmtSscan 解析 offset 字符串；避免引入 strconv 误用的小封装。
func fmtSscan(s string, out *int64) (int, error) {
	var v int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			*out = v
			return i, nil
		}
		v = v*10 + int64(c-'0')
	}
	*out = v
	return len(s), nil
}

// TriggerUpgrade POST /api/upgrade
// 后台异步执行 install.sh upgrade
// 立即返回，1-2 分钟后 WebUI 自己会被升级流程重启
func TriggerUpgrade(w http.ResponseWriter, r *http.Request) {
	// 写一个标记文件，install.sh 升级时能识别"从 WebUI 触发"（可选）
	_ = os.WriteFile("/tmp/rproxy-upgrade-triggered", []byte(time.Now().Format(time.RFC3339)), 0600)

	// 截断旧日志，确保前端从 offset=0 读到的是本次升级的日志
	// （也避免上一次的"升级完成（"标志被误判为本次完成）
	_ = os.Truncate(upgradeLogPath, 0)

	// 异步执行 install.sh upgrade，输出到日志。
	// 必须用 systemd-run 起一个独立的临时 unit：升级末段会 restart tproxy-gw-webui，
	// 而本服务未设 KillMode（默认 control-group），重启时 systemd 会连同 cgroup 内的
	// 所有子进程一起杀掉。setsid/nohup 只改会话/进程组，改不了 cgroup 归属，升级进程
	// 仍会在重启 webui 时被杀 —— install.sh 走不到结尾的"升级完成"标志，前端永远等不到完成。
	// 放进独立 unit 后升级进程在自己的 cgroup 里，重启 webui 不影响它，能跑到结尾写出标志。
	go func() {
		cmd := exec.Command("systemd-run", "--collect",
			"/bin/bash", "-c",
			"bash /opt/tproxy-gw/install.sh upgrade >> /opt/tproxy-gw/logs/upgrade.log 2>&1")
		_ = cmd.Start()
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "升级已启动，请等待 1-2 分钟后刷新页面。升级期间 WebUI 可能短暂不可用。",
	})
}
