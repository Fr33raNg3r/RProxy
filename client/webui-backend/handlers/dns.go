package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// GetDNSRules GET /api/dns/rules
func GetDNSRules(w http.ResponseWriter, r *http.Request) {
	wl, _ := os.ReadFile(config.DNSWhitelistPath)
	bl, _ := os.ReadFile(config.DNSBlacklistPath)
	hosts, _ := os.ReadFile(config.DNSHostsPath)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"whitelist": string(wl),
		"blacklist": string(bl),
		"hosts":     string(hosts),
	})
}

// UpdateDNSRules PUT /api/dns/rules
func UpdateDNSRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Whitelist *string `json:"whitelist"`
		Blacklist *string `json:"blacklist"`
		Hosts     *string `json:"hosts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	if req.Whitelist != nil {
		if err := os.WriteFile(config.DNSWhitelistPath, []byte(*req.Whitelist), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
			return
		}
	}
	if req.Blacklist != nil {
		if err := os.WriteFile(config.DNSBlacklistPath, []byte(*req.Blacklist), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
			return
		}
	}
	if req.Hosts != nil {
		if err := os.WriteFile(config.DNSHostsPath, []byte(*req.Hosts), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
			return
		}
	}
	// 清空白/黑名单 IP 集合——避免旧 IP 残留干扰
	// 错误忽略：如果集合不存在（如 nftables 还没加载），不算严重问题
	_ = exec.Command("nft", "flush", "set", "inet", "tp", "whitelist_ips").Run()
	_ = exec.Command("nft", "flush", "set", "inet", "tp", "blacklist_ips").Run()

	// 通知 mosdns reload（v5 支持 SIGHUP 重载，或直接重启）
	// 重启 mosdns 同时也会清掉它内部的 DNS 缓存
	_ = exec.Command("systemctl", "restart", "tproxy-gw-mosdns").Run()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
