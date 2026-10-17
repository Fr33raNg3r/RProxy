package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// DDNSConfigPath 是 DDNS 配置文件路径（含密钥，权限 0600）
// 声明为变量而不是常量，方便测试时改写到临时目录
var DDNSConfigPath = "/opt/rproxy-gw/config/ddns.json"

var muDDNS sync.Mutex

// DDNSProviders 是固定支持的三家服务商，页面上也固定按此顺序展示
var DDNSProviders = []string{"cloudflare", "dnspod", "namesilo"}

// DDNSRecord 是一家服务商对应的 DDNS 记录（ID 固定等于服务商名）
// 填写完整（见 Complete）才生效，留空即不生效
type DDNSRecord struct {
	ID       string `json:"id" yaml:"id"`
	Provider string `json:"provider" yaml:"provider"` // cloudflare / dnspod / namesilo
	Domain   string `json:"domain" yaml:"domain"`     // 完整域名，如 home.example.com；主域名由后端自动探测
	// Sub 是旧版本遗留字段（子域名），读取时由 NormalizeDDNS 合并进 Domain，之后恒为空
	Sub    string `json:"sub,omitempty" yaml:"sub,omitempty"`
	AuthID string `json:"auth_id" yaml:"auth_id"` // 仅 DNSPod 使用（API ID）
	Secret string `json:"secret" yaml:"secret"`   // Cloudflare Token / DNSPod Token / NameSilo Key
	// Email 仅 Cloudflare 使用：填了表示 Secret 是 Global API Key（X-Auth-Email 方式），留空表示 API Token
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
	// SecretHint 只在返回给前端时由后端填充（密钥末 4 位），不落盘
	SecretHint string `json:"secret_hint,omitempty" yaml:"-"`
}

// Complete 判断这条记录是否填写完整、可以生效：域名和密钥必填，DNSPod 还需要 API ID
func (r DDNSRecord) Complete() bool {
	if r.Domain == "" || r.Secret == "" {
		return false
	}
	return r.Provider != "dnspod" || r.AuthID != ""
}

// DDNSConfig 是 ddns.json 的整体结构
type DDNSConfig struct {
	Records []DDNSRecord `json:"records" yaml:"records"`
}

// NormalizeDDNS 整理成固定的三条记录（按 DDNSProviders 顺序，ID=服务商名）。
// 兼容旧版本保存的任意条记录：同一家有多条时优先取填写完整的第一条，未知服务商丢弃。
func NormalizeDDNS(in DDNSConfig) DDNSConfig {
	out := DDNSConfig{Records: make([]DDNSRecord, 0, len(DDNSProviders))}
	for _, p := range DDNSProviders {
		var pick *DDNSRecord
		for i := range in.Records {
			r := &in.Records[i]
			if r.Provider != p {
				continue
			}
			if pick == nil || (!pick.Complete() && r.Complete()) {
				pick = r
			}
		}
		rec := DDNSRecord{Provider: p}
		if pick != nil {
			rec = *pick
		}
		// 旧版本把子域名单独存放：合并成完整域名（@ 或留空表示根域名）
		if sub := strings.TrimSpace(rec.Sub); sub != "" && sub != "@" && rec.Domain != "" {
			rec.Domain = sub + "." + rec.Domain
		}
		rec.Sub = ""
		rec.ID = p
		out.Records = append(out.Records, rec)
	}
	return out
}

// LoadDDNSConfig 读取 ddns.json 并整理成固定三条；文件不存在时返回三张空白记录
func LoadDDNSConfig() (DDNSConfig, error) {
	muDDNS.Lock()
	defer muDDNS.Unlock()
	b, err := os.ReadFile(DDNSConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NormalizeDDNS(DDNSConfig{}), nil
		}
		return DDNSConfig{}, fmt.Errorf("读取 ddns.json: %w", err)
	}
	var c DDNSConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return DDNSConfig{}, fmt.Errorf("解析 ddns.json: %w", err)
	}
	return NormalizeDDNS(c), nil
}

// SaveDDNSConfig 原子写入 ddns.json
func SaveDDNSConfig(c DDNSConfig) error {
	muDDNS.Lock()
	defer muDDNS.Unlock()
	return writeJSONAtomic(DDNSConfigPath, NormalizeDDNS(c), 0600)
}
