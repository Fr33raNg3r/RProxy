package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
)

// loadStaticIPsScript 把黑白名单里的静态 IP 灌进 nftables set
const loadStaticIPsScript = "/opt/tproxy-gw/scripts/load-static-ips.sh"

// GetDNSRules GET /api/dns/rules
func GetDNSRules(w http.ResponseWriter, r *http.Request) {
	// 黑白名单回显：域名(.txt) 与静态 IP(_ips.txt) 合并成一个文本框内容
	hosts, _ := os.ReadFile(config.DNSHostsPath)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"whitelist": combineRules(config.DNSWhitelistPath, config.DNSWhitelistIPsPath),
		"blacklist": combineRules(config.DNSBlacklistPath, config.DNSBlacklistIPsPath),
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
	// 黑白名单按行拆分：IP/CIDR 走静态文件（nftables set），域名走 .txt（mosdns）
	if req.Whitelist != nil {
		domains, ips := splitRules(*req.Whitelist)
		if err := os.WriteFile(config.DNSWhitelistPath, []byte(domains), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
			return
		}
		if err := os.WriteFile(config.DNSWhitelistIPsPath, []byte(ips), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
			return
		}
	}
	if req.Blacklist != nil {
		domains, ips := splitRules(*req.Blacklist)
		if err := os.WriteFile(config.DNSBlacklistPath, []byte(domains), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
			return
		}
		if err := os.WriteFile(config.DNSBlacklistIPsPath, []byte(ips), 0600); err != nil {
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
	// flush 后把用户填的静态 IP 重新灌进两个 set（域名靠下面 mosdns 重启后解析自愈）
	_ = exec.Command(loadStaticIPsScript).Run()

	// 通知 mosdns reload（v5 支持 SIGHUP 重载，或直接重启）
	// 重启 mosdns 同时也会清掉它内部的 DNS 缓存
	_ = exec.Command("systemctl", "restart", "tproxy-gw-mosdns").Run()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// splitRules 把黑白名单文本按行拆成 域名 和 IP/CIDR 两部分。
// IP/CIDR → nftables 静态 set（强制直连/代理）；其余（域名、注释）→ mosdns。
// 空行丢弃；行首尾空白规整。
func splitRules(text string) (domains, ips string) {
	var dLines, iLines []string
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "#") && isIPOrCIDR(t) {
			iLines = append(iLines, t)
		} else {
			dLines = append(dLines, t) // 域名 / 注释
		}
	}
	return strings.Join(dLines, "\n"), strings.Join(iLines, "\n")
}

// isIPOrCIDR 判断是否为 IPv4 地址或 CIDR（IPv6 系统已禁用，不处理）
func isIPOrCIDR(s string) bool {
	if ip := net.ParseIP(s); ip != nil {
		return ip.To4() != nil
	}
	if ip, _, err := net.ParseCIDR(s); err == nil {
		return ip.To4() != nil
	}
	return false
}

// combineRules 把域名文件与静态 IP 文件合并成一个文本框内容回显
func combineRules(domainPath, ipPath string) string {
	d, _ := os.ReadFile(domainPath)
	i, _ := os.ReadFile(ipPath)
	var parts []string
	if s := strings.TrimRight(string(d), "\n"); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimRight(string(i), "\n"); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

// GetDNSUpstreams GET /api/dns/upstreams
func GetDNSUpstreams(w http.ResponseWriter, r *http.Request) {
	u, err := config.LoadDNSUpstreams()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// GetDNSUpstreamsDefaults GET /api/dns/upstreams/defaults
// 用于前端"恢复默认"按钮。
func GetDNSUpstreamsDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, config.DefaultDNSUpstreams())
}

// UpdateDNSUpstreams PUT /api/dns/upstreams
// 流程：校验 → 保存 JSON → 渲染 mosdns config → 同步 nftables → 重启 mosdns。
// 任一步失败立刻返回错误，前面已完成的步骤保留（用户重试时会覆盖）。
func UpdateDNSUpstreams(w http.ResponseWriter, r *http.Request) {
	var u config.DNSUpstreams
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}

	// 清理空白条目
	u.Local = cleanUpstreamList(u.Local)
	u.Remote = cleanUpstreamList(u.Remote)
	if len(u.Local) == 0 {
		writeJSON(w, http.StatusBadRequest, errorMsg("国内 DNS 上游不能为空"))
		return
	}
	if len(u.Remote) == 0 {
		writeJSON(w, http.StatusBadRequest, errorMsg("国外 DoH 上游不能为空"))
		return
	}

	if err := config.SaveDNSUpstreams(u); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("保存配置失败: "+err.Error()))
		return
	}
	if err := services.RenderMosdnsConfig(u); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("渲染 mosdns 配置失败: "+err.Error()))
		return
	}
	if err := services.SyncDNSToNftables(u); err != nil {
		// nftables 同步失败不算致命（mosdns 仍能跑），但要让用户看到
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"warning": "同步 nftables 部分失败: " + err.Error(),
		})
		_ = services.RestartMosdns()
		return
	}
	if err := services.RestartMosdns(); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("重启 mosdns 失败: "+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func cleanUpstreamList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
