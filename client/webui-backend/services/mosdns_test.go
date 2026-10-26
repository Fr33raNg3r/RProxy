package services

import (
	"strings"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 上游写成域名时要带 bootstrap：mosdns 就是本机的 DNS，不指定的话它解析上游域名会问到自己。
// 国内上游用国内 DNS 解析；国外上游用 8.8.8.8（经代理，不会拿到被污染的结果）。写成 IP 的上游不加
func TestRenderMosdnsBootstrapForDomainUpstreams(t *testing.T) {
	b, err := renderMosdnsBytes(config.DNSUpstreams{
		Local:  []string{"udp://61.128.128.68", "tls://dns.alidns.com"},
		Remote: []string{"https://1.1.1.1/dns-query", "https://dns.google/dns-query"},
	})
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	out := string(b)
	want := []string{
		"- addr: \"tls://dns.alidns.com\"\n          enable_pipeline: true\n          bootstrap: \"223.5.5.5\"",
		"- addr: \"https://dns.google/dns-query\"\n          bootstrap: \"8.8.8.8\"",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("渲染结果应包含：\n%s\n实际：\n%s", w, out)
		}
	}
	if n := strings.Count(out, "bootstrap:"); n != 2 {
		t.Errorf("只有 2 个域名上游需要 bootstrap，实际 %d 处", n)
	}
}
