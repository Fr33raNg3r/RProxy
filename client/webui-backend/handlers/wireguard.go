package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"
)

// 全局速度采样器（多用户访问 WebUI 时共享）
var (
	speedSampler  = services.NewPeerSpeedSampler()
	speedSampleMu sync.Mutex
)

// ListPeers GET /api/wireguard/peers
func ListPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := config.LoadWGPeers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	cfg, _ := config.LoadWebUIConfig()
	wgActive := services.IsServiceActive("wg-quick@wg0")
	srvPub, _ := services.ReadServerPublicKey()
	resp := map[string]interface{}{
		"peers":             sanitizePeers(peers),
		"wg_active":         wgActive,
		"server_public_key": srvPub,
	}
	if cfg != nil {
		resp["wg_enabled"] = cfg.WGEnabled
		resp["wg_listen_port"] = cfg.WGListenPort
		resp["wg_subnet"] = cfg.WGSubnet
		resp["wg_endpoint"] = cfg.WGEndpoint
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetEndpoint POST /api/wireguard/endpoint
// body: { "endpoint": "myhome.ddns.net" }
// 设置 peer 配置文件中使用的 Endpoint 地址（公网 IP 或域名）。
// 不附带端口号——端口由 wg_listen_port 控制。
func SetEndpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
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
	// 简单清理：去掉前后空格、协议前缀、端口后缀
	ep := req.Endpoint
	// 不做严格校验——用户可以填 ddns 域名、IPv4、IPv6 等
	cfg.WGEndpoint = ep
	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"wg_endpoint": cfg.WGEndpoint,
	})
}

// EnableWireGuard POST /api/wireguard/enable
// 启动 wg-quick@wg0 服务（wireguard-tools 包始终保留，仅控制服务启停）
// 如果还没生成 wg0.conf，先生成一个空 peers 的配置
func EnableWireGuard(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	if cfg.WGEnabled && services.IsServiceActive("wg-quick@wg0") {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"message": "WireGuard 服务已经在运行",
		})
		return
	}

	// 生成 wg0.conf（含当前所有 peer）
	peers, _ := config.LoadWGPeers()
	if err := services.RenderWGConfig(cfg, peers); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("生成 wg0.conf 失败: "+err.Error()))
		return
	}

	// 启动 wg-quick@wg0 服务
	if err := services.RestartWG(); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("启动 WireGuard 失败: "+err.Error()))
		return
	}

	cfg.WGEnabled = true
	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "WireGuard 入站服务已启动",
	})
}

// DisableWireGuard POST /api/wireguard/disable
// 停止 wg-quick@wg0 服务，保留所有 peer 配置（peers.json 不动）
func DisableWireGuard(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	_ = services.StopWG()

	cfg.WGEnabled = false
	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "WireGuard 入站服务已停止（peer 配置已保留，再次启用时立即可用）",
	})
}

// 列表中不返回客户端私钥（仅生成二维码时使用）
func sanitizePeers(peers []config.WGPeer) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(peers))
	for _, p := range peers {
		out = append(out, map[string]interface{}{
			"id":         p.ID,
			"name":       p.Name,
			"address":    p.Address,
			"public_key": p.PublicKey,
			"created_at": p.CreatedAt,
		})
	}
	return out
}

// CreatePeer POST /api/wireguard/peers   body: { "name": "..." }
func CreatePeer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorMsg("名称不能为空"))
		return
	}
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	peers, err := config.LoadWGPeers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	priv, pub, err := services.GenerateKeyPair()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("生成密钥失败: "+err.Error()))
		return
	}
	psk, _ := services.GeneratePresharedKey()

	usedIPs := []string{}
	for _, p := range peers {
		usedIPs = append(usedIPs, p.Address)
	}
	addr, err := services.AllocatePeerIP(cfg.WGSubnet, usedIPs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	peer := config.WGPeer{
		ID:           services.GenerateID(),
		Name:         req.Name,
		PrivateKey:   priv,
		PublicKey:    pub,
		PresharedKey: psk,
		Address:      addr,
		CreatedAt:    time.Now().Unix(),
	}
	peers = append(peers, peer)
	if err := config.SaveWGPeers(peers); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	// 仅在 WG 服务已启用时才重新生成 wg0.conf 和重启服务
	// 服务未启用：peer 数据已保存，下次启用时会一并加载
	if cfg.WGEnabled {
		if err := services.RenderWGConfig(cfg, peers); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("生成 wg0.conf 失败: "+err.Error()))
			return
		}
		if err := services.RestartWG(); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg("重启 WG 失败: "+err.Error()))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": peer.ID})
}

// DeletePeer DELETE /api/wireguard/peers/{id}
func DeletePeer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, _ := config.LoadWebUIConfig()
	peers, err := config.LoadWGPeers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	newPeers := make([]config.WGPeer, 0, len(peers))
	for _, p := range peers {
		if p.ID != id {
			newPeers = append(newPeers, p)
		}
	}
	if err := config.SaveWGPeers(newPeers); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	if cfg != nil && cfg.WGEnabled {
		_ = services.RenderWGConfig(cfg, newPeers)
		// 启用状态下：peer 列表变了就 reload 服务（即使列表空也保持运行，由用户手动禁用）
		_ = services.RestartWG()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// PeerQRCode GET /api/wireguard/peers/{id}/qrcode
// 返回 PNG 图片
func PeerQRCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	peers, _ := config.LoadWGPeers()
	var target *config.WGPeer
	for i := range peers {
		if peers[i].ID == id {
			target = &peers[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, errorMsg("peer 不存在"))
		return
	}

	// Endpoint 来源优先级：URL 参数 > 已保存的 wg_endpoint > 自动探测公网 IP
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		endpoint = cfg.WGEndpoint
	}
	if endpoint == "" {
		endpoint = services.GetExternalIP()
	}

	clientCfg, err := services.BuildClientConfig(cfg, *target, endpoint)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}

	// 同时把 client config 文本作为 header 返回（前端可下载）
	w.Header().Set("X-WG-Config", "available")

	// 二维码
	png, err := qrcode.Encode(clientCfg, qrcode.Medium, 320)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

// PeerConfig GET /api/wireguard/peers/{id}/config
// 返回客户端配置文本（用户可下载或复制）
func PeerConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	peers, _ := config.LoadWGPeers()
	var target *config.WGPeer
	for i := range peers {
		if peers[i].ID == id {
			target = &peers[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, errorMsg("peer 不存在"))
		return
	}
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		endpoint = cfg.WGEndpoint
	}
	if endpoint == "" {
		endpoint = services.GetExternalIP()
	}
	clientCfg, err := services.BuildClientConfig(cfg, *target, endpoint)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+target.Name+`.conf"`)
	_, _ = w.Write([]byte(clientCfg))
}

// StreamPeerSpeed GET /api/wireguard/speed/stream
// SSE 推送实时速度（仅在 WebUI 打开时计算，关闭则停止）
func StreamPeerSpeed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	send := func() {
		speedSampleMu.Lock()
		speeds, err := speedSampler.Sample()
		speedSampleMu.Unlock()
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return
		}
		fmt.Fprintf(w, "data: ")
		_ = jsonEncode(w, map[string]interface{}{"speeds": speeds, "ts": time.Now().Unix()})
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
