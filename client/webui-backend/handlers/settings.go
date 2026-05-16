package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
)

// GetSettings GET /api/settings
func GetSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"listen_port":     cfg.ListenPort,
		"username":        cfg.Username,
		"wg_enabled":      cfg.WGEnabled,
		"wg_listen_port":  cfg.WGListenPort,
		"wg_subnet":       cfg.WGSubnet,
		"wg_endpoint":     cfg.WGEndpoint,
		"update_hour":     cfg.UpdateHour,
		"update_minute":   cfg.UpdateMinute,
		"current_node_id": cfg.CurrentNodeID,
	})
}

// UpdateSettings PUT /api/settings
func UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ListenPort   *int `json:"listen_port"`
		WGListenPort *int `json:"wg_listen_port"`
		UpdateHour   *int `json:"update_hour"`
		UpdateMinute *int `json:"update_minute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	needRestartWebUI := false
	needUpdateTimer := false
	needRestartWG := false

	if req.ListenPort != nil && *req.ListenPort != cfg.ListenPort {
		if *req.ListenPort < 1 || *req.ListenPort > 65535 {
			writeJSON(w, http.StatusBadRequest, errorMsg("listen_port 无效"))
			return
		}
		cfg.ListenPort = *req.ListenPort
		needRestartWebUI = true
	}
	if req.WGListenPort != nil && *req.WGListenPort != cfg.WGListenPort {
		if *req.WGListenPort < 1 || *req.WGListenPort > 65535 {
			writeJSON(w, http.StatusBadRequest, errorMsg("wg_listen_port 无效"))
			return
		}
		cfg.WGListenPort = *req.WGListenPort
		needRestartWG = true
	}
	if req.UpdateHour != nil {
		if *req.UpdateHour < 0 || *req.UpdateHour > 23 {
			writeJSON(w, http.StatusBadRequest, errorMsg("update_hour 无效"))
			return
		}
		cfg.UpdateHour = *req.UpdateHour
		needUpdateTimer = true
	}
	if req.UpdateMinute != nil {
		if *req.UpdateMinute < 0 || *req.UpdateMinute > 59 {
			writeJSON(w, http.StatusBadRequest, errorMsg("update_minute 无效"))
			return
		}
		cfg.UpdateMinute = *req.UpdateMinute
		needUpdateTimer = true
	}

	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	if needRestartWG {
		// 1) 重新渲染 wg0.conf 并重启 WG 服务（让新端口监听生效）
		peers, _ := config.LoadWGPeers()
		if err := services.RenderWGConfig(cfg, peers); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("生成 wg0.conf 失败: "+err.Error()))
			return
		}
		if err := services.RestartWG(); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("重启 WireGuard 失败: "+err.Error()))
			return
		}
		// 2) 同步更新 nftables output 链中放行 WG 响应包的规则
		// 若不更新，wg-quick 已切到新端口监听，但 output 链仍按旧端口放行，
		// WG 响应包会被 TPROXY 劫持 → 远端客户端永远握手不上。
		if err := services.SyncWGPortToNftables(cfg.WGListenPort); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("同步 nftables WG 端口失败: "+err.Error()))
			return
		}
	}

	if needUpdateTimer {
		// 更新 systemd timer 的 OnCalendar
		_ = updateTimerSchedule(cfg.UpdateHour, cfg.UpdateMinute)
	}

	resp := map[string]interface{}{"ok": true}
	if needRestartWebUI {
		resp["webui_will_restart"] = true
		// 异步重启 WebUI（让本次响应先发完）
		go func() {
			// 等 1 秒让响应返回
			_ = exec.Command("bash", "-c", "sleep 1 && systemctl restart tproxy-gw-webui").Start()
		}()
	}
	writeJSON(w, http.StatusOK, resp)
}

// updateTimerSchedule 重写 update timer 的 OnCalendar
func updateTimerSchedule(hour, minute int) error {
	timerFile := "/etc/systemd/system/tproxy-gw-update.timer"
	content := fmt.Sprintf(`[Unit]
Description=RProxy Daily Update Timer (%02d:%02d daily)

[Timer]
OnCalendar=*-*-* %02d:%02d:00
RandomizedDelaySec=30m
Persistent=true
Unit=tproxy-gw-update.service

[Install]
WantedBy=timers.target
`, hour, minute, hour, minute)
	if err := os.WriteFile(timerFile, []byte(content), 0644); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return err
	}
	return exec.Command("systemctl", "restart", "tproxy-gw-update.timer").Run()
}

// EmergencyStop POST /api/emergency-stop
func EmergencyStop(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("/opt/tproxy-gw/scripts/emergency-stop.sh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":     false,
			"error":  err.Error(),
			"output": string(out),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"output": string(out),
	})
}

// RestartComponent POST /api/services/{name}/restart
func RestartComponent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name") // chi 不需要这个，统一用 chi.URLParam，但这里我们另外读取
	if name == "" {
		// fallback: 从 query
		name = r.URL.Query().Get("name")
	}
	allowed := map[string]string{
		"xray":   "xray",
		"mosdns": "tproxy-gw-mosdns",
		"wg":     "wg-quick@wg0",
	}
	svc, ok := allowed[name]
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorMsg("不支持的服务名"))
		return
	}
	if err := exec.Command("systemctl", "restart", svc).Run(); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
