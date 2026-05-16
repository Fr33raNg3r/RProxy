package handlers

import (
	"context"
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
	versionFile  = "/opt/tproxy-gw/VERSION"
	versionURL   = "https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/client/VERSION"
	upgradeShell = "/opt/tproxy-gw/install.sh upgrade"
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "GET", versionURL, nil)
		if err != nil {
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return
		}
		buf := make([]byte, 32)
		n, _ := resp.Body.Read(buf)
		v := strings.TrimSpace(string(buf[:n]))
		if v != "" {
			versionMu.Lock()
			cachedLatestVer = v
			versionMu.Unlock()
		}
	}()
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
