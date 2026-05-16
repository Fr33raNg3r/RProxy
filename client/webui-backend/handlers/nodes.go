package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"

	"github.com/go-chi/chi/v5"
)

// ListNodes GET /api/nodes
func ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	cfg, _ := config.LoadWebUIConfig()
	currentID := ""
	if cfg != nil {
		currentID = cfg.CurrentNodeID
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes":           nodes,
		"current_node_id": currentID,
	})
}

// CreateNode POST /api/nodes
func CreateNode(w http.ResponseWriter, r *http.Request) {
	var n config.Node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	if err := validateNode(&n); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg(err.Error()))
		return
	}

	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	// 生成 ID 和 Order
	n.ID = services.GenerateID()
	n.Order = len(nodes)
	applyNodeDefaults(&n)

	nodes = append(nodes, n)
	if err := config.SaveNodes(nodes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	// 如果当前没有活动节点（首次添加），自动选定为当前并应用配置
	cfg, _ := config.LoadWebUIConfig()
	if cfg != nil && cfg.CurrentNodeID == "" && n.Enabled {
		cfg.CurrentNodeID = n.ID
		if err := config.SaveWebUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"node":    n,
				"warning": "节点已添加，但保存当前节点 ID 失败: " + err.Error(),
			})
			return
		}
		if err := services.RenderXrayConfig(nodes, n.ID); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"node":    n,
				"warning": "节点已添加并自动设为当前节点，但 Xray 配置渲染失败: " + err.Error(),
			})
			return
		}
		if err := services.RestartXray(); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"node":    n,
				"warning": "节点已添加并设为当前，但 Xray 重启失败: " + err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":              true,
			"node":            n,
			"current_node_id": n.ID,
			"message":         "节点已添加并自动启用",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "node": n})
}

// UpdateNode PUT /api/nodes/{id}
func UpdateNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var n config.Node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	if err := validateNode(&n); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg(err.Error()))
		return
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	found := false
	for i := range nodes {
		if nodes[i].ID == id {
			n.ID = id
			n.Order = nodes[i].Order
			applyNodeDefaults(&n)
			nodes[i] = n
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorMsg("节点不存在"))
		return
	}
	if err := config.SaveNodes(nodes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	// 如果修改的是当前节点，需要重新渲染并重启
	cfg, _ := config.LoadWebUIConfig()
	if cfg != nil && cfg.CurrentNodeID == id {
		if err := services.RenderXrayConfig(nodes, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("渲染配置失败: "+err.Error()))
			return
		}
		if err := services.RestartXray(); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("重启 Xray 失败: "+err.Error()))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// DeleteNode DELETE /api/nodes/{id}
func DeleteNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	newNodes := make([]config.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.ID != id {
			newNodes = append(newNodes, n)
		}
	}
	if err := config.SaveNodes(newNodes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	// 如果删除的是当前节点，清空 current_node_id 并切到第一个可用节点
	cfg, _ := config.LoadWebUIConfig()
	if cfg != nil && cfg.CurrentNodeID == id {
		cfg.CurrentNodeID = ""
		for _, n := range newNodes {
			if n.Enabled {
				cfg.CurrentNodeID = n.ID
				break
			}
		}
		if err := config.SaveWebUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("保存当前节点 ID 失败: "+err.Error()))
			return
		}
		if err := services.RenderXrayConfig(newNodes, cfg.CurrentNodeID); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("渲染 Xray 配置失败: "+err.Error()))
			return
		}
		if err := services.RestartXray(); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("重启 Xray 失败: "+err.Error()))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// SwitchNode POST /api/nodes/{id}/switch
func SwitchNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	var target *config.Node
	for i := range nodes {
		if nodes[i].ID == id {
			target = &nodes[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, errorMsg("节点不存在"))
		return
	}
	if !target.Enabled {
		writeJSON(w, http.StatusBadRequest, errorMsg("该节点未启用"))
		return
	}
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	cfg.CurrentNodeID = id
	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	if err := services.RenderXrayConfig(nodes, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("渲染配置失败: "+err.Error()))
		return
	}
	if err := services.RestartXray(); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("重启 Xray 失败: "+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "current_node_id": id})
}

// TestNode POST /api/nodes/{id}/test
// 简化版本：只测试当前活动节点（要求用户先切换）
func TestNode(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.LoadWebUIConfig()
	id := chi.URLParam(r, "id")
	if cfg == nil || cfg.CurrentNodeID != id {
		writeJSON(w, http.StatusBadRequest, errorMsg("请先切换到该节点再测试"))
		return
	}
	d, err := services.TestNode(config.XraySocksPort)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      false,
			"error":   err.Error(),
			"latency": 0,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"latency": d.Milliseconds(),
	})
}

// ReorderNodes POST /api/nodes/reorder
// body: { "ids": ["id1","id2",...] }
func ReorderNodes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	idIndex := map[string]int{}
	for i, id := range req.IDs {
		idIndex[id] = i
	}
	for i := range nodes {
		if idx, ok := idIndex[nodes[i].ID]; ok {
			nodes[i].Order = idx
		}
	}
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Order > nodes[j].Order {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
	if err := config.SaveNodes(nodes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// ----------------- 工具 -----------------

// applyNodeDefaults 给新建/编辑的节点填充默认值
func applyNodeDefaults(n *config.Node) {
	if n.Security == "" {
		n.Security = "auto"
	}
	if n.WSPath == "" {
		n.WSPath = "/"
	}
	if n.Host == "" {
		n.Host = n.Address
	}
}

// validateNode 校验节点必填字段
// VMess+WS+TLS 必须用域名连接（不接受 IP，因为需要 TLS SNI）
func validateNode(n *config.Node) error {
	if strings.TrimSpace(n.Name) == "" {
		return simpleErrMsg("name 不能为空")
	}
	if strings.TrimSpace(n.Address) == "" {
		return simpleErrMsg("address 不能为空")
	}
	if isIPAddress(n.Address) {
		return simpleErrMsg("address 必须是域名（VMess+WS+TLS 协议不能用 IP）")
	}
	if n.Port <= 0 || n.Port > 65535 {
		return simpleErrMsg("port 无效")
	}
	if strings.TrimSpace(n.UUID) == "" {
		return simpleErrMsg("uuid 不能为空")
	}
	if strings.TrimSpace(n.WSPath) == "" {
		return simpleErrMsg("ws_path 不能为空")
	}
	if !strings.HasPrefix(n.WSPath, "/") {
		return simpleErrMsg("ws_path 必须以 / 开头")
	}
	// 验证 security 字段
	if n.Security != "" {
		validSec := map[string]bool{
			"auto": true, "aes-128-gcm": true, "chacha20-poly1305": true,
			"none": true, "zero": true,
		}
		if !validSec[n.Security] {
			return simpleErrMsg("security 必须是 auto / aes-128-gcm / chacha20-poly1305 / none / zero 之一")
		}
	}
	return nil
}

func simpleErrMsg(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

// isIPAddress 简单检查字符串是否为 IPv4 或 IPv6 地址
func isIPAddress(s string) bool {
	// 含冒号 → 可能是 IPv6
	if strings.Contains(s, ":") {
		return true
	}
	// IPv4 形如 1.2.3.4
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 {
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
