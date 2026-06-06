package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// DNSUpstreams 是用户在 WebUI 中配置的 mosdns 上游列表。
// Local：国内 DNS（一般是 udp:// 或 tcp:// IP），保存后会同步到 nftables cn_ips。
// Remote：国外 DoH（一般是 https://...），保存后解析其域名/IP 同步到 force_proxy_ips。
type DNSUpstreams struct {
	Local  []string `json:"local"`
	Remote []string `json:"remote"`
}

var muDNSUpstreams sync.Mutex

// DefaultDNSUpstreams 返回与原 mosdns-config.yaml.tpl 一致的初始默认值：
// 电信 + 移动运营商 DNS + Cloudflare/Google DoH。
func DefaultDNSUpstreams() DNSUpstreams {
	return DNSUpstreams{
		Local: []string{
			// 中国电信
			"udp://61.128.128.68",
			"udp://61.128.192.68",
			// 中国移动
			"udp://218.201.4.3",
			"udp://218.201.21.132",
		},
		Remote: []string{
			"https://1.1.1.1/dns-query",
			"https://8.8.8.8/dns-query",
		},
	}
}

// LoadDNSUpstreams 读取用户配置；文件不存在时返回默认值（不创建文件）。
func LoadDNSUpstreams() (DNSUpstreams, error) {
	muDNSUpstreams.Lock()
	defer muDNSUpstreams.Unlock()
	b, err := os.ReadFile(DNSUpstreamsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultDNSUpstreams(), nil
		}
		return DNSUpstreams{}, fmt.Errorf("读取 upstreams.json: %w", err)
	}
	var u DNSUpstreams
	if err := json.Unmarshal(b, &u); err != nil {
		return DNSUpstreams{}, fmt.Errorf("解析 upstreams.json: %w", err)
	}
	return u, nil
}

func SaveDNSUpstreams(u DNSUpstreams) error {
	muDNSUpstreams.Lock()
	defer muDNSUpstreams.Unlock()
	return writeJSONAtomic(DNSUpstreamsPath, u, 0644)
}
