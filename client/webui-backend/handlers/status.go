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
			"mosdns": services.IsServiceActive("tproxy-gw-mosdns"),
			"webui":  true, // 自己在跑
			"wg":     services.IsServiceActive("wg-quick@wg0"),
		},
		"current_node_id":   currentNode,
		"current_node_name": currentNodeName,
		"node_count":        len(nodes),
	}

	if health != nil {
		resp["health"] = health
	}
	return resp
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
