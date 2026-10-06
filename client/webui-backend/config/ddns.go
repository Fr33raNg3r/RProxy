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

// DDNSRecord 是一条 DDNS 记录
type DDNSRecord struct {
	ID       string `json:"id" yaml:"id"`
	Provider string `json:"provider" yaml:"provider"` // cloudflare / dnspod / namesilo
	Domain   string `json:"domain" yaml:"domain"`     // 主域名，如 example.com
	Sub      string `json:"sub" yaml:"sub"`           // 子域名，如 home；根域名留空或填 @
	AuthID   string `json:"auth_id" yaml:"auth_id"`   // 仅 DNSPod 使用（API ID）
	Secret   string `json:"secret" yaml:"secret"`     // Cloudflare Token / DNSPod Token / NameSilo Key
	Enabled  bool   `json:"enabled" yaml:"enabled"`
}

// DDNSConfig 是 ddns.json 的整体结构
type DDNSConfig struct {
	Enabled bool         `json:"enabled" yaml:"enabled"`
	Records []DDNSRecord `json:"records" yaml:"records"`
}

// LoadDDNSConfig 读取 ddns.json；文件不存在时返回空配置
func LoadDDNSConfig() (DDNSConfig, error) {
	muDDNS.Lock()
	defer muDDNS.Unlock()
	b, err := os.ReadFile(DDNSConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DDNSConfig{Records: []DDNSRecord{}}, nil
		}
		return DDNSConfig{}, fmt.Errorf("读取 ddns.json: %w", err)
	}
	var c DDNSConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return DDNSConfig{}, fmt.Errorf("解析 ddns.json: %w", err)
	}
	if c.Records == nil {
		c.Records = []DDNSRecord{}
	}
	return c, nil
}

// SaveDDNSConfig 原子写入 ddns.json
func SaveDDNSConfig(c DDNSConfig) error {
	muDDNS.Lock()
	defer muDDNS.Unlock()
	if c.Records == nil {
		c.Records = []DDNSRecord{}
	}
	return writeJSONAtomic(DDNSConfigPath, c, 0600)
}
