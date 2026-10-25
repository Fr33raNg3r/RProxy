// 路径常量与配置数据结构
package config

const (
	InstallDir   = "/opt/rproxy-gw"
	WWWDir       = "/var/www/rproxy-gw"
	XrayConfDir  = "/usr/local/etc/xray"
	XrayConfPath = "/usr/local/etc/xray/config.json"

	WebUIConfigPath = "/opt/rproxy-gw/config/webui.json"
	NodesPath       = "/opt/rproxy-gw/config/xray/nodes.json"
	WGPeersPath     = "/opt/rproxy-gw/config/wireguard/peers.json"
	WGServerPrivKey = "/opt/rproxy-gw/config/wireguard/server_privatekey"
	WGServerPubKey  = "/opt/rproxy-gw/config/wireguard/server_publickey"
	WGConfPath      = "/etc/wireguard/wg0.conf"
	HealthFile      = "/run/rproxy-gw/health.json" // watchdog 每轮都写，放内存文件系统免得磨损闪存
	FailbackState   = "/opt/rproxy-gw/data/failback.json"

	DNSWhitelistPath = "/opt/rproxy-gw/config/dns/whitelist.txt"
	DNSBlacklistPath = "/opt/rproxy-gw/config/dns/blacklist.txt"
	// 静态 IP/CIDR：用户在黑白名单里填的 IP 行拆出来单独存，由 load-static-ips.sh
	// 灌进 nftables whitelist_ips/blacklist_ips set（域名仍走上面的 .txt + mosdns）
	DNSWhitelistIPsPath = "/opt/rproxy-gw/config/dns/whitelist_ips.txt"
	DNSBlacklistIPsPath = "/opt/rproxy-gw/config/dns/blacklist_ips.txt"
	DNSHostsPath        = "/opt/rproxy-gw/config/dns/hosts.txt"
	DNSUpstreamsPath = "/opt/rproxy-gw/config/dns/upstreams.json"

	MosdnsConfigPath  = "/opt/rproxy-gw/config/mosdns/config.yaml"
	MosdnsLogPath     = "/opt/rproxy-gw/logs/mosdns.log"
	XrayAccessLogPath = "/var/log/xray/access.log"
	XrayErrorLogPath  = "/var/log/xray/error.log"

	NftablesConfPath = "/etc/nftables.conf"

	XraySocksPort = 10808
	WGInterface   = "wg0"
)
