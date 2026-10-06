package handlers

import (
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"gopkg.in/yaml.v3"
)

func TestMaskDDNSSecrets(t *testing.T) {
	in := config.DDNSConfig{Records: []config.DDNSRecord{
		{ID: "1", Secret: "real"},
		{ID: "2", Secret: ""},
	}}
	out := maskDDNSSecrets(in)
	if out.Records[0].Secret != secretMask || out.Records[1].Secret != "" {
		t.Fatalf("打码结果不符: %+v", out.Records)
	}
	if in.Records[0].Secret != "real" {
		t.Fatal("不应修改原数据")
	}
}

func TestMergeDDNSSecretsKeepsOldWhenMasked(t *testing.T) {
	old := config.DDNSConfig{Records: []config.DDNSRecord{{ID: "1", Secret: "old-secret"}}}
	in := config.DDNSConfig{Records: []config.DDNSRecord{
		{ID: "1", Secret: secretMask},  // 沿用旧密钥
		{ID: "2", Secret: "brand-new"}, // 新记录
		{ID: "3", Secret: secretMask},  // 找不到旧值，清空而不是保存占位符
	}}
	out := mergeDDNSSecrets(in, old)
	if out.Records[0].Secret != "old-secret" || out.Records[1].Secret != "brand-new" || out.Records[2].Secret != "" {
		t.Fatalf("合并结果不符: %+v", out.Records)
	}
}

// 旧版导出文件没有 wg_peers / ddns 字段，导入时必须得到 nil，
// 这样导入逻辑才能区分"没带"和"带了空列表"，不会误清空现有数据
func TestBundleFromLegacyYAMLHasNoPeersOrDDNS(t *testing.T) {
	legacy := "version: \"1\"\nwebui:\n  listen_port: 8080\nnodes: []\n"
	var b ClientConfigBundle
	if err := yaml.Unmarshal([]byte(legacy), &b); err != nil {
		t.Fatal(err)
	}
	if b.WGPeers != nil || b.DDNS != nil {
		t.Fatalf("旧文件应得到 nil: peers=%v ddns=%v", b.WGPeers, b.DDNS)
	}
}

func TestBundleRoundTripKeepsPeersAndDDNS(t *testing.T) {
	in := ClientConfigBundle{
		Version: "1",
		WGPeers: []config.WGPeer{{ID: "p1", Name: "phone", PrivateKey: "priv", PublicKey: "pub", Address: "172.16.7.2/32"}},
		DDNS:    &config.DDNSConfig{Records: []config.DDNSRecord{{ID: "namesilo", Provider: "namesilo", Domain: "a.com", Secret: "k"}}},
	}
	b, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ClientConfigBundle
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.WGPeers) != 1 || out.WGPeers[0].PrivateKey != "priv" || out.DDNS == nil || out.DDNS.Records[0].Secret != "k" {
		t.Fatalf("往返不一致: %+v", out)
	}
}

func TestValidateDDNSConfigBlankIsOKPartialIsRejected(t *testing.T) {
	blank := config.NormalizeDDNS(config.DDNSConfig{})
	if err := validateDDNSConfig(&blank); err != nil {
		t.Fatalf("全部留空应通过（不生效），实际 %v", err)
	}

	partial := config.NormalizeDDNS(config.DDNSConfig{Records: []config.DDNSRecord{
		{Provider: "cloudflare", Domain: "a.com"}, // 填了域名没填密钥
	}})
	if err := validateDDNSConfig(&partial); err == nil {
		t.Fatal("只填了一半应报错")
	}

	dnspodNoID := config.NormalizeDDNS(config.DDNSConfig{Records: []config.DDNSRecord{
		{Provider: "dnspod", Domain: "a.com", Secret: "t"},
	}})
	if err := validateDDNSConfig(&dnspodNoID); err == nil {
		t.Fatal("DNSPod 缺 API ID 应报错")
	}

	ok := config.NormalizeDDNS(config.DDNSConfig{Records: []config.DDNSRecord{
		{Provider: "dnspod", Domain: " a.com ", Sub: " h ", AuthID: "1", Secret: "t"},
	}})
	if err := validateDDNSConfig(&ok); err != nil {
		t.Fatalf("填全应通过，实际 %v", err)
	}
	for _, r := range ok.Records {
		if r.Provider == "dnspod" && (r.Domain != "a.com" || r.Sub != "h") {
			t.Errorf("应去掉首尾空格: %+v", r)
		}
	}
}
