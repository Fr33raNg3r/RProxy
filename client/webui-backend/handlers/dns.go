package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
)

// loadStaticIPsScript 把黑白名单里的静态 IP 灌进 nftables set
const loadStaticIPsScript = "/opt/rproxy-gw/scripts/load-static-ips.sh"

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
	var paths []string
	if req.Whitelist != nil {
		paths = append(paths, config.DNSWhitelistPath, config.DNSWhitelistIPsPath)
	}
	if req.Blacklist != nil {
		paths = append(paths, config.DNSBlacklistPath, config.DNSBlacklistIPsPath)
	}
	if req.Hosts != nil {
		paths = append(paths, config.DNSHostsPath)
	}
	write := func() error {
		// 黑白名单按行拆分：IP/CIDR 走静态文件（nftables set），域名走 .txt（mosdns）
		if req.Whitelist != nil {
			domains, ips := splitRules(*req.Whitelist)
			if err := config.WriteFileAtomic(config.DNSWhitelistPath, []byte(domains), 0600); err != nil {
				return err
			}
			if err := config.WriteFileAtomic(config.DNSWhitelistIPsPath, []byte(ips), 0600); err != nil {
				return err
			}
		}
		if req.Blacklist != nil {
			domains, ips := splitRules(*req.Blacklist)
			if err := config.WriteFileAtomic(config.DNSBlacklistPath, []byte(domains), 0600); err != nil {
				return err
			}
			if err := config.WriteFileAtomic(config.DNSBlacklistIPsPath, []byte(ips), 0600); err != nil {
				return err
			}
		}
		if req.Hosts != nil {
			if err := config.WriteFileAtomic(config.DNSHostsPath, []byte(*req.Hosts), 0600); err != nil {
				return err
			}
		}
		return nil
	}
	if err := applyDNSFiles(paths, write, reloadDNSRules); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// applyDNSFiles 写入名单文件后应用；应用失败（mosdns 起不来）时把文件恢复原样并再应用一次，
// 避免一条 mosdns 无法解析的规则让局域网 DNS 全部失效
func applyDNSFiles(paths []string, write, apply func() error) error {
	snap := snapshotFiles(paths)
	if err := write(); err != nil {
		restoreFiles(snap)
		return err
	}
	if err := apply(); err != nil {
		restoreFiles(snap)
		if rerr := apply(); rerr != nil {
			return fmt.Errorf("%v；恢复原名单后仍失败: %v", err, rerr)
		}
		return fmt.Errorf("新名单未能生效，已恢复原名单: %v", err)
	}
	return nil
}

// reloadDNSRules 让名单文件生效：重灌静态 IP 集合，重启 mosdns 并确认它仍在运行
func reloadDNSRules() error {
	// 清空白/黑名单 IP 集合——避免旧 IP 残留干扰
	// 错误忽略：如果集合不存在（如 nftables 还没加载），不算严重问题
	_ = services.RunCmdErr(services.CmdTimeoutShort, "nft", "flush", "set", "inet", "tp", "whitelist_ips")
	_ = services.RunCmdErr(services.CmdTimeoutShort, "nft", "flush", "set", "inet", "tp", "blacklist_ips")
	// flush 后把用户填的静态 IP 重新灌进两个 set（域名靠下面 mosdns 重启后解析自愈）
	_ = services.RunCmdErr(services.CmdTimeoutLong, loadStaticIPsScript)

	// 重启 mosdns 同时也会清掉它内部的 DNS 缓存
	return services.RestartMosdns()
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

	// 保存前校验：写法有误的上游会让 mosdns 起不来、局域网 DNS 全部失效
	if err := validateDNSUpstreams(u, localIPv4s()); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg(err.Error()))
		return
	}
	// 清理空白条目
	u.Local = cleanUpstreamList(u.Local)
	u.Remote = cleanUpstreamList(u.Remote)

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

// 上游可用的协议；不写协议时按 udp
var upstreamSchemes = map[string]bool{"udp": true, "tcp": true, "tls": true, "https": true}

// 域名：各段字母数字与连字符、不以连字符开头结尾，至少两段，顶级域以字母开头
var upstreamDomainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

// localIPv4s 本机各网卡的 IPv4 地址（测试中可替换）
var localIPv4s = func() []net.IP {
	var ips []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			ips = append(ips, n.IP)
		}
	}
	return ips
}

// validateDNSUpstreams 校验国内、国外上游列表（空行跳过，行号按用户填写的顺序计），
// 出错时指明是哪一组的第几行
func validateDNSUpstreams(u config.DNSUpstreams, self []net.IP) error {
	groups := []struct {
		name string
		list []string
	}{{"国内上游", u.Local}, {"国外上游", u.Remote}}
	for _, g := range groups {
		n := 0
		for i, raw := range g.list {
			a := strings.TrimSpace(raw)
			if a == "" {
				continue
			}
			n++
			if err := validateUpstreamAddr(a, self); err != nil {
				return fmt.Errorf("%s第 %d 行（%s）：%v", g.name, i+1, a, err)
			}
		}
		if n == 0 {
			return fmt.Errorf("%s不能为空", g.name)
		}
	}
	return nil
}

// validateUpstreamAddr 校验一条 mosdns 上游：udp:// tcp:// tls:// https:// 或不写协议；
// 主机为 IPv4 或域名，端口可省略（用协议默认端口）；不能指向本机（mosdns 自己就是本机的 DNS）
func validateUpstreamAddr(addr string, self []net.IP) error {
	// 会弄坏渲染出的 mosdns 配置（YAML）的字符
	if strings.ContainsAny(addr, " \"'`\\") || strings.IndexFunc(addr, unicode.IsControl) >= 0 {
		return errors.New("不能包含空格、引号、反斜杠等字符")
	}
	s := addr
	if !strings.Contains(s, "://") {
		s = "udp://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return errors.New("格式不正确")
	}
	if !upstreamSchemes[u.Scheme] {
		return fmt.Errorf("不支持 %s://，可用 udp:// tcp:// tls:// https://", u.Scheme)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("格式不正确")
	}
	if u.Scheme != "https" && u.Path != "" {
		return errors.New("只有 https:// 可以带路径")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("缺少服务器地址")
	}
	if strings.HasSuffix(u.Host, ":") {
		return errors.New("端口不能为空")
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return errors.New("端口必须在 1-65535 之间")
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return errors.New("不支持 IPv6 地址")
		}
		if ip.IsLoopback() || ip.IsUnspecified() {
			return errors.New("不能是本机")
		}
		for _, s := range self {
			if ip.Equal(s) {
				return errors.New("不能是本机（本机网卡地址）")
			}
		}
		return nil
	}
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return errors.New("不能是本机")
	}
	if !upstreamDomainRe.MatchString(h) {
		return errors.New("不是合法的 IP 或域名")
	}
	return nil
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
