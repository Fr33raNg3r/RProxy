package handlers

import (
	"log"
	"net/http"
	"os/exec"
	"time"
)

// 重启动作与延迟做成变量，测试里替换，避免真的重启
var (
	rebootFn    = func() error { return exec.Command("systemctl", "reboot").Run() }
	rebootDelay = time.Second
)

// RebootSystem POST /api/system/reboot
// 先返回成功，稍后再重启整台旁路由，保证前端能收到响应
func RebootSystem(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	go func() {
		time.Sleep(rebootDelay)
		if err := rebootFn(); err != nil {
			log.Printf("重启系统失败: %v", err)
		}
	}()
}
