package handlers

import (
	"encoding/json"
	"net/http"
	"os"
)

// xrayAlertPath Xray 自动更新兼容性告警标志文件，由 update-daemon.sh 在
// 检测到新版无法加载现有配置并回滚后写入。
const xrayAlertPath = "/opt/tproxy-gw/data/xray-update-alert.json"

// GetXrayAlert GET /api/xray/alert
// 读取告警标志。文件不存在或内容异常时返回 {active:false}。
func GetXrayAlert(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(xrayAlertPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"active": false})
		return
	}
	var a struct {
		Active  bool   `json:"active"`
		Time    string `json:"time"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &a); err != nil || !a.Active {
		writeJSON(w, http.StatusOK, map[string]interface{}{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active":  true,
		"time":    a.Time,
		"message": a.Message,
	})
}

// DismissXrayAlert POST /api/xray/alert/dismiss
// 用户在 WebUI 手动关闭告警：删除标志文件。
func DismissXrayAlert(w http.ResponseWriter, r *http.Request) {
	_ = os.Remove(xrayAlertPath)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
