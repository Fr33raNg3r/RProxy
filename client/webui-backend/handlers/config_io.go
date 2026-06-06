package handlers

import (
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"

	"gopkg.in/yaml.v3"
)

// ClientConfigBundle 是导入/导出的整体客户端配置快照
// 不包含密码哈希、session_secret 等运行时敏感字段
type ClientConfigBundle struct {
	Version  string                  `yaml:"version"`
	Exported string                  `yaml:"exported_at"`
	WebUI    webUISection            `yaml:"webui"`
	Nodes    []config.Node           `yaml:"nodes"`
	DNS      *config.DNSUpstreams   `yaml:"dns_upstreams,omitempty"`
}

type webUISection struct {
	ListenPort    int    `yaml:"listen_port"`
	UpdateHour    int    `yaml:"update_hour"`
	UpdateMinute  int    `yaml:"update_minute"`
	CurrentNodeID string `yaml:"current_node_id"`
	WGEnabled     bool   `yaml:"wg_enabled"`
	WGListenPort  int    `yaml:"wg_listen_port"`
	WGSubnet      string `yaml:"wg_subnet"`
	WGEndpoint    string `yaml:"wg_endpoint"`
}

// ExportConfig GET /api/config/export
// 返回 YAML 文本，浏览器侧用 Content-Disposition 触发下载
func ExportConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	dns, errDNS := config.LoadDNSUpstreams()
	var dnsPtr *config.DNSUpstreams
	if errDNS == nil {
		dnsPtr = &dns
	}

	bundle := ClientConfigBundle{
		Version:  "1",
		Exported: time.Now().Format(time.RFC3339),
		WebUI: webUISection{
			ListenPort:    cfg.ListenPort,
			UpdateHour:    cfg.UpdateHour,
			UpdateMinute:  cfg.UpdateMinute,
			CurrentNodeID: cfg.CurrentNodeID,
			WGEnabled:     cfg.WGEnabled,
			WGListenPort:  cfg.WGListenPort,
			WGSubnet:      cfg.WGSubnet,
			WGEndpoint:    cfg.WGEndpoint,
		},
		Nodes: nodes,
		DNS:   dnsPtr,
	}

	b, err := yaml.Marshal(bundle)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("生成 YAML 失败: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rproxy-client-config.yaml"`)
	_, _ = w.Write(b)
}

// ImportConfig POST /api/config/import
// body 为 YAML 文本（text/plain 或 application/x-yaml）
// 替换 nodes / dns_upstreams 以及 webui 中允许的字段，然后重渲染并重启
func ImportConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("读取请求体失败: "+err.Error()))
		return
	}
	var bundle ClientConfigBundle
	if err := yaml.Unmarshal(body, &bundle); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("YAML 解析失败: "+err.Error()))
		return
	}

	// 基本校验
	for i := range bundle.Nodes {
		if err := validateNode(&bundle.Nodes[i]); err != nil {
			writeJSON(w, http.StatusBadRequest, errorMsg("节点校验失败: "+err.Error()))
			return
		}
	}
	if bundle.WebUI.ListenPort < 1 || bundle.WebUI.ListenPort > 65535 {
		writeJSON(w, http.StatusBadRequest, errorMsg("listen_port 无效"))
		return
	}

	// 写 nodes.json
	if err := config.SaveNodes(bundle.Nodes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("保存 nodes.json 失败: "+err.Error()))
		return
	}

	// 写 dns_upstreams.json（如果导入包含）
	if bundle.DNS != nil {
		if err := config.SaveDNSUpstreams(*bundle.DNS); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("保存 dns_upstreams.json 失败: "+err.Error()))
			return
		}
	}

	// 更新 webui.json（保留密码哈希、session_secret 等敏感字段）
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	cfg.ListenPort = bundle.WebUI.ListenPort
	cfg.UpdateHour = bundle.WebUI.UpdateHour
	cfg.UpdateMinute = bundle.WebUI.UpdateMinute
	cfg.CurrentNodeID = bundle.WebUI.CurrentNodeID
	cfg.WGEnabled = bundle.WebUI.WGEnabled
	cfg.WGListenPort = bundle.WebUI.WGListenPort
	cfg.WGSubnet = bundle.WebUI.WGSubnet
	cfg.WGEndpoint = bundle.WebUI.WGEndpoint
	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	// 重渲染 Xray + mosdns 并重启关键服务
	if err := services.RenderXrayConfig(bundle.Nodes, cfg.CurrentNodeID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("渲染 Xray 配置失败: "+err.Error()))
		return
	}
	if bundle.DNS != nil {
		if err := services.RenderMosdnsConfig(*bundle.DNS); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("渲染 mosdns 配置失败: "+err.Error()))
			return
		}
		_ = exec.Command("systemctl", "restart", "tproxy-gw-mosdns").Run()
	}
	_ = services.RestartXray()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"node_count": len(bundle.Nodes),
		"dns":        bundle.DNS != nil,
	})
}
