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
// 不包含密码哈希、session_secret 等 WebUI 登录相关字段；
// 但包含 WireGuard peer 私钥和 DDNS 密钥，导出文件需妥善保管。
// wg_peers / ddns 为空（旧版导出文件没有这两项）时，导入不会动现有数据。
type ClientConfigBundle struct {
	Version  string                  `yaml:"version"`
	Exported string                  `yaml:"exported_at"`
	WebUI    webUISection            `yaml:"webui"`
	Nodes    []config.Node           `yaml:"nodes"`
	DNS      *config.DNSUpstreams   `yaml:"dns_upstreams,omitempty"`
	WGPeers  []config.WGPeer         `yaml:"wg_peers"`
	DDNS     *config.DDNSConfig      `yaml:"ddns,omitempty"`
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

	peers, err := config.LoadWGPeers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	ddns, err := config.LoadDDNSConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
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
		Nodes:   nodes,
		DNS:     dnsPtr,
		WGPeers: peers,
		DDNS:    &ddns,
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

	// DDNS 配置整理成固定三条并校验（格式不对整体拒绝，避免写入一半）
	if bundle.DDNS != nil {
		n := config.NormalizeDDNS(*bundle.DDNS)
		bundle.DDNS = &n
		if err := validateDDNSConfig(bundle.DDNS); err != nil {
			writeJSON(w, http.StatusBadRequest, errorMsg("DDNS 配置校验失败: "+err.Error()))
			return
		}
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

	// WireGuard peers：导入文件带了才整体替换，然后重渲染 wg0.conf
	if bundle.WGPeers != nil {
		if err := config.SaveWGPeers(bundle.WGPeers); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("保存 peers.json 失败: "+err.Error()))
			return
		}
	}
	if bundle.WGPeers != nil || cfg.WGEnabled {
		peers, _ := config.LoadWGPeers()
		if err := services.RenderWGConfig(cfg, peers); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("渲染 wg0.conf 失败: "+err.Error()))
			return
		}
		if cfg.WGEnabled {
			_ = services.RestartWG()
		}
	}

	// DDNS：导入文件带了才整体替换，并立刻跑一轮
	if bundle.DDNS != nil {
		if err := config.SaveDDNSConfig(*bundle.DDNS); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("保存 ddns.json 失败: "+err.Error()))
			return
		}
		go services.RunDDNSNow()
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
		"wg_peers":   bundle.WGPeers != nil,
		"ddns":       bundle.DDNS != nil,
	})
}
