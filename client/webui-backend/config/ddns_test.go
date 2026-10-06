package config

import (
	"path/filepath"
	"testing"
)

func TestDDNSRecordComplete(t *testing.T) {
	cases := []struct {
		name string
		r    DDNSRecord
		want bool
	}{
		{"cloudflare 填全", DDNSRecord{Provider: "cloudflare", Domain: "a.com", Secret: "t"}, true},
		{"缺密钥", DDNSRecord{Provider: "cloudflare", Domain: "a.com"}, false},
		{"缺域名", DDNSRecord{Provider: "namesilo", Secret: "k"}, false},
		{"dnspod 缺 API ID", DDNSRecord{Provider: "dnspod", Domain: "a.com", Secret: "t"}, false},
		{"dnspod 填全", DDNSRecord{Provider: "dnspod", Domain: "a.com", Secret: "t", AuthID: "1"}, true},
		{"全空", DDNSRecord{Provider: "namesilo"}, false},
	}
	for _, c := range cases {
		if got := c.r.Complete(); got != c.want {
			t.Errorf("%s: Complete() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNormalizeDDNSAlwaysThreeFixedProviders(t *testing.T) {
	got := NormalizeDDNS(DDNSConfig{})
	want := []string{"cloudflare", "dnspod", "namesilo"}
	if len(got.Records) != 3 {
		t.Fatalf("应固定 3 条，实际 %d", len(got.Records))
	}
	for i, p := range want {
		if got.Records[i].Provider != p || got.Records[i].ID != p {
			t.Errorf("第 %d 条应为 %s，实际 %+v", i, p, got.Records[i])
		}
	}
}

func TestNormalizeDDNSMigratesOldRecords(t *testing.T) {
	old := DDNSConfig{Records: []DDNSRecord{
		{ID: "x1", Provider: "namesilo", Domain: "old.com", Sub: "a", Secret: "k1"},
		{ID: "x2", Provider: "namesilo", Domain: "second.com", Secret: "k2"}, // 同一家的第 2 条被舍弃
		{ID: "x3", Provider: "dnspod", Domain: "d.com", AuthID: "9", Secret: "t"},
		{ID: "x4", Provider: "unknown", Domain: "z.com", Secret: "s"}, // 未知厂商丢弃
	}}
	got := NormalizeDDNS(old)
	byID := map[string]DDNSRecord{}
	for _, r := range got.Records {
		byID[r.ID] = r
	}
	if byID["namesilo"].Domain != "old.com" || byID["namesilo"].Secret != "k1" {
		t.Errorf("namesilo 应取第一条: %+v", byID["namesilo"])
	}
	if byID["dnspod"].AuthID != "9" {
		t.Errorf("dnspod 迁移丢失: %+v", byID["dnspod"])
	}
	if byID["cloudflare"].Domain != "" {
		t.Errorf("cloudflare 应为空白卡片: %+v", byID["cloudflare"])
	}
}

func TestDDNSConfigRoundTripAndMissingFile(t *testing.T) {
	old := DDNSConfigPath
	DDNSConfigPath = filepath.Join(t.TempDir(), "ddns.json")
	defer func() { DDNSConfigPath = old }()

	c, err := LoadDDNSConfig()
	if err != nil || len(c.Records) != 3 {
		t.Fatalf("文件不存在时应返回 3 张空白卡片，得到 %+v, %v", c, err)
	}

	in := NormalizeDDNS(DDNSConfig{Records: []DDNSRecord{
		{Provider: "dnspod", Domain: "a.com", Sub: "h", AuthID: "9", Secret: "s"},
	}})
	if err := SaveDDNSConfig(in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadDDNSConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Records {
		if r.Provider == "dnspod" && (r.Secret != "s" || r.AuthID != "9" || r.Domain != "a.com") {
			t.Fatalf("读回不一致: %+v", r)
		}
	}
}
