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
	if byID["namesilo"].Domain != "a.old.com" || byID["namesilo"].Secret != "k1" {
		t.Errorf("namesilo 应取第一条: %+v", byID["namesilo"])
	}
	if byID["dnspod"].AuthID != "9" {
		t.Errorf("dnspod 迁移丢失: %+v", byID["dnspod"])
	}
	if byID["cloudflare"].Domain != "" {
		t.Errorf("cloudflare 应为空白卡片: %+v", byID["cloudflare"])
	}
}

// 旧版本把子域名和主域名分开保存；现在只保留一个完整域名，读取时自动合并
func TestNormalizeDDNSMergesLegacySubIntoDomain(t *testing.T) {
	cases := []struct {
		name, domain, sub, want string
	}{
		{"普通子域名", "example.com", "home", "home.example.com"},
		{"根域名写 @", "example.com", "@", "example.com"},
		{"根域名留空", "example.com", "", "example.com"},
		{"已经是完整域名", "home.example.com", "", "home.example.com"},
		{"多级后缀", "example.com.cn", "a.b", "a.b.example.com.cn"},
	}
	for _, c := range cases {
		got := NormalizeDDNS(DDNSConfig{Records: []DDNSRecord{
			{Provider: "cloudflare", Domain: c.domain, Sub: c.sub, Secret: "s"},
		}})
		if r := got.Records[0]; r.Domain != c.want || r.Sub != "" {
			t.Errorf("%s: 得到 domain=%q sub=%q，期望 %q", c.name, r.Domain, r.Sub, c.want)
		}
		// 再规范化一次结果不变（幂等）
		again := NormalizeDDNS(got)
		if again.Records[0].Domain != c.want {
			t.Errorf("%s: 二次规范化应不变，得到 %q", c.name, again.Records[0].Domain)
		}
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
		{Provider: "dnspod", Domain: "h.a.com", AuthID: "9", Secret: "s"},
	}})
	if err := SaveDDNSConfig(in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadDDNSConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Records {
		if r.Provider == "dnspod" && (r.Secret != "s" || r.AuthID != "9" || r.Domain != "h.a.com") {
			t.Fatalf("读回不一致: %+v", r)
		}
	}
}
