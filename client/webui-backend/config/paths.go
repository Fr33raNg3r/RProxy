// 路径常量与配置数据结构
package config

const (
	InstallDir   = "/opt/tproxy-gw"
	WWWDir       = "/var/www/tproxy-gw"
	XrayConfDir  = "/usr/local/etc/xray"
	XrayConfPath = "/usr/local/etc/xray/config.json"

	WebUIConfigPath = "/opt/tproxy-gw/config/webui.json"
	NodesPath       = "/opt/tproxy-gw/config/xray/nodes.json"
	WGPeersPath     = "/opt/tproxy-gw/config/wireguard/peers.json"
	WGServerPrivKey = "/opt/tproxy-gw/config/wireguard/server_privatekey"
	WGServerPubKey  = "/opt/tproxy-gw/config/wireguard/server_publickey"
	WGConfPath      = "/etc/wireguard/wg0.conf"
	HealthFile      = "/opt/tproxy-gw/data/health.json"

	DNSWhitelistPath = "/opt/tproxy-gw/config/dns/whitelist.txt"
	DNSBlacklistPath = "/opt/tproxy-gw/config/dns/blacklist.txt"
	DNSHostsPath     = "/opt/tproxy-gw/config/dns/hosts.txt"

	XraySocksPort = 10808
	WGInterface   = "wg0"
)
