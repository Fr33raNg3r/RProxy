package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
)

// GetStatus 返回组件状态、当前节点等信息
func GetStatus(w http.ResponseWriter, r *http.Request) {
	resp := buildStatusResponse()
	writeJSON(w, http.StatusOK, resp)
}

func buildStatusResponse() map[string]interface{} {
	cfg, _ := config.LoadWebUIConfig()
	nodes, _ := config.LoadNodes()
	health, _ := services.ReadHealth()

	currentNode := ""
	currentNodeName := ""
	if cfg != nil {
		currentNode = cfg.CurrentNodeID
		for _, n := range nodes {
			if n.ID == currentNode {
				currentNodeName = n.Name
				break
			}
		}
	}

	resp := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"services": map[string]interface{}{
			"xray":   services.IsServiceActive("xray"),
			"mosdns": services.IsServiceActive("rproxy-gw-mosdns"),
			"webui":  true, // 自己在跑
			"wg":     services.IsServiceActive("wg-quick@wg0"),
		},
		"current_node_id":   currentNode,
		"current_node_name": currentNodeName,
		"node_count":        len(nodes),
	}

	if h := mergeManualProbe(health, currentNode, services.LastManualProbe()); h != nil {
		resp["health"] = h
	}
	return resp
}

// mergeManualProbe WebUI 手动切换 / 编辑节点后立即探测过、结果比 watchdog 最近一次探测新、且针对当前节点时，
// 用它作为代理连通性；watchdog 之后再探测（last_probe 更新）就自然换回 watchdog 的结果。
// 两边各写各的，WebUI 不改 health.json，不会被 watchdog 每轮重写覆盖
func mergeManualProbe(h *services.HealthData, currentNodeID string, m services.ProbeResult) *services.HealthData {
	if m.Time.IsZero() || m.NodeID != currentNodeID {
		return h
	}
	if h == nil {
		h = &services.HealthData{CurrentNodeID: currentNodeID}
	} else {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", h.LastProbe, time.Local); err == nil && !t.Before(m.Time) {
			return h
		}
		c := *h // 副本：不改 ReadHealth 返回的数据
		h = &c
	}
	h.ProxyOK = 0
	if m.OK {
		h.ProxyOK = 1
	}
	h.LastProbe = m.Time.Format("2006-01-02 15:04:05")
	return h
}

// StreamStatus 通过 SSE 每 3 秒推送一次状态
func StreamStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	send := func() {
		resp := buildStatusResponse()
		fmt.Fprintf(w, "data: ")
		_ = jsonEncode(w, resp)
		fmt.Fprintf(w, "\n\n")
		flusher.Flush()
	}
	send()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			send()
		}
	}
}
