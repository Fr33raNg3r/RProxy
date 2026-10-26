package handlers

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
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
// 替换 nodes / dns_upstreams 以及 webui 中允许的字段，然后重渲染并重启。
// 尽量做到"要么全部生效，要么不改"：先全部校验并做 Xray 配置校验，再写文件；
// 写入或渲染中途失败时，把各 JSON 配置恢复到导入前并按旧配置重新生成。
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
	if err := validateImportBundle(&bundle); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg(err.Error()))
		return
	}

	// 最容易失败的一步（xray -test 校验）放在任何写入之前：失败时什么都没改
	if err := services.RenderXrayConfig(bundle.Nodes, bundle.WebUI.CurrentNodeID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("渲染 Xray 配置失败（未做任何修改）: "+err.Error()))
		return
	}

	snap := snapshotFiles([]string{
		config.NodesPath, config.DNSUpstreamsPath, config.WebUIConfigPath, config.WGPeersPath, config.DDNSConfigPath,
	})
	fail := func(msg string) {
		restoreFiles(snap)
		rerenderFromDisk()
		writeJSON(w, http.StatusInternalServerError, errorMsg(msg+"（已恢复为导入前的配置）"))
	}

	// 写 nodes.json
	if err := config.SaveNodes(bundle.Nodes); err != nil {
		fail("保存 nodes.json 失败: " + err.Error())
		return
	}

	// 写 dns_upstreams.json（如果导入包含）
	if bundle.DNS != nil {
		if err := config.SaveDNSUpstreams(*bundle.DNS); err != nil {
			fail("保存 dns_upstreams.json 失败: " + err.Error())
			return
		}
	}

	// 更新 webui.json（保留密码哈希、session_secret 等敏感字段）
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		fail(err.Error())
		return
	}
	oldListenPort, oldWGPort := cfg.ListenPort, cfg.WGListenPort
	cfg.ListenPort = bundle.WebUI.ListenPort
	cfg.UpdateHour = bundle.WebUI.UpdateHour
	cfg.UpdateMinute = bundle.WebUI.UpdateMinute
	cfg.CurrentNodeID = bundle.WebUI.CurrentNodeID
	// 当前节点由导入文件指定：清掉故障转移标记，否则之后会被自动回切到别的节点
	cfg.AutoSwitched = false
	cfg.WGEnabled = bundle.WebUI.WGEnabled
	cfg.WGListenPort = bundle.WebUI.WGListenPort
	cfg.WGSubnet = bundle.WebUI.WGSubnet
	cfg.WGEndpoint = bundle.WebUI.WGEndpoint
	if err := config.SaveWebUIConfig(cfg); err != nil {
		fail(err.Error())
		return
	}

	// WireGuard peers：导入文件带了才整体替换
	if bundle.WGPeers != nil {
		if err := config.SaveWGPeers(bundle.WGPeers); err != nil {
			fail("保存 peers.json 失败: " + err.Error())
			return
		}
	}

	// DDNS：导入文件带了才整体替换
	if bundle.DDNS != nil {
		if err := config.SaveDDNSConfig(*bundle.DDNS); err != nil {
			fail("保存 ddns.json 失败: " + err.Error())
			return
		}
	}

	// 渲染 mosdns / wg0.conf（Xray 已在写入前渲染）
	if bundle.DNS != nil {
		if err := services.RenderMosdnsConfig(*bundle.DNS); err != nil {
			fail("渲染 mosdns 配置失败: " + err.Error())
			return
		}
	}
	if bundle.WGPeers != nil || cfg.WGEnabled {
		peers, _ := config.LoadWGPeers()
		if err := services.RenderWGConfig(cfg, peers); err != nil {
			fail("渲染 wg0.conf 失败: " + err.Error())
			return
		}
	}

	// 全部写入、渲染成功后再让各项生效（与设置页保存时的处理一致）
	var warnings []string
	if cfg.WGEnabled {
		_ = services.RestartWG()
	} else {
		_ = services.StopWG()
	}
	// WG 端口变了要同步 nftables 放行规则，否则 WG 响应包被 TPROXY 劫持、客户端握手失败
	if cfg.WGListenPort != oldWGPort && cfg.WGListenPort > 0 {
		if err := services.SyncWGPortToNftables(cfg.WGListenPort); err != nil {
			warnings = append(warnings, "同步 nftables WG 端口失败: "+err.Error())
		}
	}
	if err := services.WriteUpdateTimer(cfg.UpdateHour, cfg.UpdateMinute); err != nil {
		warnings = append(warnings, "更新每日自动更新时间失败: "+err.Error())
	}
	if bundle.DDNS != nil {
		go services.RunDDNSNow()
	}
	if bundle.DNS != nil {
		// 新上游的 IP 同步到 nftables（国内上游 → cn_ips，国外上游 → force_proxy_ips），与 DNS 页保存时一致
		if err := services.SyncDNSToNftables(*bundle.DNS); err != nil {
			warnings = append(warnings, "同步 nftables 部分失败: "+err.Error())
		}
		if err := services.RestartMosdns(); err != nil {
			warnings = append(warnings, "重启 mosdns 失败: "+err.Error())
		}
	}
	if err := services.RestartXray(); err != nil {
		warnings = append(warnings, "重启 Xray 失败: "+err.Error())
	}

	resp := map[string]interface{}{
		"ok":         true,
		"node_count": len(bundle.Nodes),
		"dns":        bundle.DNS != nil,
		"wg_peers":   bundle.WGPeers != nil,
		"ddns":       bundle.DDNS != nil,
	}
	if len(warnings) > 0 {
		resp["warning"] = strings.Join(warnings, "；")
	}
	if cfg.ListenPort != oldListenPort {
		resp["webui_will_restart"] = true
		restartWebUIAsync()
	}
	writeJSON(w, http.StatusOK, resp)
}

// validateImportBundle 在写入任何文件之前完成全部校验与默认值填充
func validateImportBundle(b *ClientConfigBundle) error {
	// 先填默认值：1.7.0 及更早导出的节点可能没有 protocol 或仍是 vmess-ws
	for i := range b.Nodes {
		applyNodeDefaults(&b.Nodes[i])
		if err := validateNode(&b.Nodes[i]); err != nil {
			return fmt.Errorf("节点校验失败: %v", err)
		}
	}
	for _, p := range b.WGPeers {
		if err := validatePeerName(p.Name); err != nil {
			return fmt.Errorf("WireGuard peer %q 校验失败: %v", p.Name, err)
		}
	}
	wu := &b.WebUI
	if wu.ListenPort < 1 || wu.ListenPort > 65535 {
		return fmt.Errorf("listen_port 无效")
	}
	if wu.WGListenPort < 0 || wu.WGListenPort > 65535 || (wu.WGEnabled && wu.WGListenPort == 0) {
		return fmt.Errorf("wg_listen_port 无效")
	}
	if wu.WGSubnet != "" {
		if ip, _, err := net.ParseCIDR(wu.WGSubnet); err != nil || ip.To4() == nil {
			return fmt.Errorf("wg_subnet 无效: %s", wu.WGSubnet)
		}
	}
	if wu.UpdateHour < 0 || wu.UpdateHour > 23 {
		return fmt.Errorf("update_hour 无效")
	}
	if wu.UpdateMinute < 0 || wu.UpdateMinute > 59 {
		return fmt.Errorf("update_minute 无效")
	}

	// DNS 上游与 WebUI 保存时同样校验（写法有误会让 mosdns 起不来），通过后去掉空行
	if b.DNS != nil {
		if err := validateDNSUpstreams(*b.DNS, localIPv4s()); err != nil {
			return fmt.Errorf("DNS 上游校验失败: %v", err)
		}
		b.DNS.Local = cleanUpstreamList(b.DNS.Local)
		b.DNS.Remote = cleanUpstreamList(b.DNS.Remote)
	}

	// DDNS 配置整理成固定三条并校验（格式不对整体拒绝，避免写入一半）
	if b.DDNS != nil {
		n := config.NormalizeDDNS(*b.DDNS)
		b.DDNS = &n
		if err := validateDDNSConfig(b.DDNS); err != nil {
			return fmt.Errorf("DDNS 配置校验失败: %v", err)
		}
	}

	// current_node_id 指向不存在的节点时回退到 Order 最小的节点（与启动时规范化一致），并同步 Enabled
	found := false
	for _, n := range b.Nodes {
		if n.ID == wu.CurrentNodeID {
			found = true
			break
		}
	}
	if !found {
		wu.CurrentNodeID = ""
		minIdx := -1
		for i := range b.Nodes {
			if minIdx == -1 || b.Nodes[i].Order < b.Nodes[minIdx].Order {
				minIdx = i
			}
		}
		if minIdx >= 0 {
			wu.CurrentNodeID = b.Nodes[minIdx].ID
		}
	}
	for i := range b.Nodes {
		b.Nodes[i].Enabled = b.Nodes[i].ID == wu.CurrentNodeID
	}
	return nil
}

// fileSnapshot 导入前某个文件的状态
type fileSnapshot struct {
	existed bool
	data    []byte
	mode    os.FileMode
}

// snapshotFiles 记录各文件导入前的内容；读取失败（非"不存在"）的文件不纳入，避免误删
func snapshotFiles(paths []string) map[string]fileSnapshot {
	snap := make(map[string]fileSnapshot, len(paths))
	for _, p := range paths {
		st, err := os.Stat(p)
		if os.IsNotExist(err) {
			snap[p] = fileSnapshot{}
			continue
		}
		if err != nil {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		snap[p] = fileSnapshot{existed: true, data: b, mode: st.Mode().Perm()}
	}
	return snap
}

// restoreFiles 把文件恢复到快照时的状态：原本存在的写回原内容，原本不存在的删除
func restoreFiles(snap map[string]fileSnapshot) {
	for p, s := range snap {
		if s.existed {
			_ = config.WriteFileAtomic(p, s.data, s.mode)
		} else {
			_ = os.Remove(p)
		}
	}
}

// rerenderFromDisk 导入失败恢复 JSON 后，按恢复后的配置重新生成 Xray / mosdns 配置
// （Xray 在写入前已按导入内容渲染过，mosdns 可能也已渲染）。wg0.conf 渲染是最后一步，失败时未写入，无需处理
func rerenderFromDisk() {
	if cfg, err := config.LoadWebUIConfig(); err == nil {
		if nodes, err := config.LoadNodes(); err == nil {
			_ = services.RenderXrayConfig(nodes, cfg.CurrentNodeID)
		}
	}
	if u, err := config.LoadDNSUpstreams(); err == nil {
		_ = services.RenderMosdnsConfig(u)
	}
}
