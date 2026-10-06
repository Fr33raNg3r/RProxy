package config

import (
	"path/filepath"
	"testing"
)

func TestDDNSConfigRoundTripAndMissingFile(t *testing.T) {
	old := DDNSConfigPath
	DDNSConfigPath = filepath.Join(t.TempDir(), "ddns.json")
	defer func() { DDNSConfigPath = old }()

	c, err := LoadDDNSConfig()
	if err != nil || c.Enabled || len(c.Records) != 0 {
		t.Fatalf("文件不存在时应返回空配置，得到 %+v, %v", c, err)
	}

	in := DDNSConfig{Enabled: true, Records: []DDNSRecord{
		{ID: "1", Provider: "dnspod", Domain: "a.com", Sub: "h", AuthID: "9", Secret: "s", Enabled: true},
	}}
	if err := SaveDDNSConfig(in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadDDNSConfig()
	if err != nil || !out.Enabled || len(out.Records) != 1 || out.Records[0].Secret != "s" || out.Records[0].AuthID != "9" {
		t.Fatalf("读回不一致: %+v, %v", out, err)
	}
}
