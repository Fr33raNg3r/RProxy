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
	// 静态 IP/CIDR：用户在黑白名单里填的 IP 行拆出来单独存，由 load-static-ips.sh
	// 灌进 nftables whitelist_ips/blacklist_ips set（域名仍走上面的 .txt + mosdns）
	DNSWhitelistIPsPath = "/opt/tproxy-gw/config/dns/whitelist_ips.txt"
	DNSBlacklistIPsPath = "/opt/tproxy-gw/config/dns/blacklist_ips.txt"
	DNSHostsPath        = "/opt/tproxy-gw/config/dns/hosts.txt"
	DNSUpstreamsPath = "/opt/tproxy-gw/config/dns/upstreams.json"

	MosdnsConfigPath = "/opt/tproxy-gw/config/mosdns/config.yaml"

	NftablesConfPath = "/etc/nftables.conf"

	XraySocksPort = 10808
	WGInterface   = "wg0"
)
