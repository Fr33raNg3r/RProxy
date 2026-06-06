package services

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// SyncDNSToNftables 把用户配置的 DNS 上游同步到 nftables：
//   - Local 上游里的 IP 加进 cn_ips（auto-merge，幂等；非 IP 形式跳过）
//   - Remote DoH 的 host 解析为 IPv4，整批替换 force_proxy_ips
//
// 注意：cn_ips 不 flush——还有 bootstrap、mosdns 动态填充、load-cn-ips 三个来源。
// 旧 DNS IP 残留在 cn_ips 里无害（最多多几条直连规则）。
func SyncDNSToNftables(u config.DNSUpstreams) error {
	// ---------- 国内 DNS 上游 → cn_ips ----------
	for _, raw := range u.Local {
		host := extractHostFromMosdnsAddr(raw)
		if host == "" {
			continue
		}
		if net.ParseIP(host) == nil {
			// 国内上游正常都是 IP；域名形式跳过（无法判断地区，盲加可能误送代理）
			continue
		}
		_ = exec.Command("nft", "add", "element", "inet", "tp", "cn_ips",
			fmt.Sprintf("{ %s/32 }", host)).Run()
	}

	// ---------- 国外 DoH 上游 → force_proxy_ips ----------
	var dohIPs []string
	var resolveErrs []string
	for _, raw := range u.Remote {
		host := extractHostFromMosdnsAddr(raw)
		if host == "" {
			continue
		}
		if net.ParseIP(host) != nil {
			dohIPs = append(dohIPs, host)
			continue
		}
		ips, err := resolveAllIPv4(host)
		if err != nil || len(ips) == 0 {
			resolveErrs = append(resolveErrs, fmt.Sprintf("%s: %v", host, err))
			continue
		}
		dohIPs = append(dohIPs, ips...)
	}

	if err := exec.Command("nft", "flush", "set", "inet", "tp", "force_proxy_ips").Run(); err != nil {
		return fmt.Errorf("flush force_proxy_ips: %w", err)
	}
	for _, ip := range dohIPs {
		if err := exec.Command("nft", "add", "element", "inet", "tp", "force_proxy_ips",
			fmt.Sprintf("{ %s }", ip)).Run(); err != nil {
			return fmt.Errorf("add %s to force_proxy_ips: %w", ip, err)
		}
	}

	if len(resolveErrs) > 0 {
		return fmt.Errorf("部分 DoH 域名解析失败: %s", strings.Join(resolveErrs, "; "))
	}
	return nil
}

// extractHostFromMosdnsAddr 从 mosdns upstream addr 提取 host 部分。
// 示例：udp://61.128.128.68 → 61.128.128.68；https://1.1.1.1/dns-query → 1.1.1.1
func extractHostFromMosdnsAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	// 没 scheme 时 url.Parse 会把整串当 path，兜底加 scheme
	if !strings.Contains(addr, "://") {
		addr = "udp://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

// resolveAllIPv4 用硬编码的国内 DNS 解析域名，避免循环依赖 mosdns。
// 223.5.5.5 在 nftables cn_ips bootstrap 里有硬编码，不会被错送代理。
func resolveAllIPv4(host string) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", "223.5.5.5:53")
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := r.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	var v4 []string
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			v4 = append(v4, a)
		}
	}
	return v4, nil
}
