package handlers

import (
	"encoding/json"
	"net/http"
	"os"
)

// xrayAlertPath 自动更新回滚告警标志文件，由 rproxy-rollback 在
// RProxy 升级或 Xray 升级出问题、回滚到升级前快照后写入。
const xrayAlertPath = "/opt/rproxy-gw/data/xray-update-alert.json"

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
