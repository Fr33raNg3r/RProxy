package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// DDNSConfigPath 是 DDNS 配置文件路径（含密钥，权限 0600）
// 声明为变量而不是常量，方便测试时改写到临时目录
var DDNSConfigPath = "/opt/tproxy-gw/config/ddns.json"

var muDDNS sync.Mutex

// DDNSProviders 是固定支持的三家服务商，页面上也固定按此顺序展示
var DDNSProviders = []string{"cloudflare", "dnspod", "namesilo"}

// DDNSRecord 是一家服务商对应的 DDNS 记录（ID 固定等于服务商名）
// 填写完整（见 Complete）才生效，留空即不生效
type DDNSRecord struct {
	ID       string `json:"id" yaml:"id"`
	Provider string `json:"provider" yaml:"provider"` // cloudflare / dnspod / namesilo
	Domain   string `json:"domain" yaml:"domain"`     // 主域名，如 example.com
	Sub      string `json:"sub" yaml:"sub"`           // 子域名，如 home；根域名留空或填 @
	AuthID   string `json:"auth_id" yaml:"auth_id"`   // 仅 DNSPod 使用（API ID）
	Secret   string `json:"secret" yaml:"secret"`     // Cloudflare Token / DNSPod Token / NameSilo Key
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
