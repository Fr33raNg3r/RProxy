package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// xrayTmpConfPattern 是先交给 xray -test 校验的临时文件名模式（* 处随机，WebUI 与 watchdog 同时渲染也不会互相覆盖）。
// 必须以 .json 结尾：Xray v26.10.21 起按扩展名识别格式，.tmp 会被直接拒绝（-format=json 也无效）。
const xrayTmpConfPattern = "config.*.tmp.json"

// RenderXrayConfig 根据节点池和当前节点 ID 生成 Xray 配置文件
// 如果 currentNodeID 为空，生成一个最小可用配置（无代理出站）
func RenderXrayConfig(nodes []config.Node, currentNodeID string) error {
	cfg := buildXrayBaseConfig()

	// 找到当前节点
	var current *config.Node
	if currentNodeID != "" {
		for i := range nodes {
			if nodes[i].ID == currentNodeID {
				current = &nodes[i]
				break
			}
		}
	}

	if current != nil {
		// 加入 proxy 出站
		proxyOut := buildProxyOutbound(current)
		// 把 proxy 放最前
		cfg["outbounds"] = append([]interface{}{proxyOut}, cfg["outbounds"].([]interface{})...)
		// 加入完整路由规则
		cfg["routing"] = buildRoutingWithProxy()
	} else {
		// 没有可用节点：所有流量直连
		cfg["routing"] = buildRoutingDirectOnly()
	}

	if err := os.MkdirAll(config.XrayConfDir, 0755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(config.XrayConfDir, xrayTmpConfPattern)
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp)
		return werr
	}

	// 验证配置语法（用 -format=json 显式指定）
	if out, err := RunCmdCombined(CmdTimeoutLong, "xray", "run", "-test", "-format=json", "-c", tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("xray 配置验证失败: %s\n%s", err, string(out))
	}

	// 临时文件只用于 -test 校验；正式配置走 fsync 的原子写，断电不会留下空配置
	os.Remove(tmp)
	return config.WriteFileAtomic(config.XrayConfPath, b, 0644)
}

func buildXrayBaseConfig() map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			// access log 每条透明代理连接写一行，写盘量大、磨损 eMMC/SD 卡，默认关闭（"none"）；
			// 排障需要时可临时把 config.json 里的 access 改为 /var/log/xray/access.log 并重启 Xray
			// （下次切换节点重新生成配置时会恢复为 none）。
			// error log 写到 /var/log/xray/，这个目录由 Xray 官方安装脚本创建，nobody 有写权限
			"access": "none",
			"error":  config.XrayErrorLogPath,
		},
		"inbounds": []interface{}{
			// TPROXY 入口
			map[string]interface{}{
				"tag":      "tproxy-in",
				"port":     12345,
				"protocol": "tunnel", // 原名 dokodemo-door
				"settings": map[string]interface{}{
					"allowedNetwork": "tcp,udp",
					"followRedirect": true,
				},
				"streamSettings": map[string]interface{}{
					"sockopt": map[string]interface{}{
						"tproxy": "tproxy",
						"mark":   255,
					},
				},
				// 不开 sniffing：路由不按域名匹配，嗅探结果用不上
			},
			// SOCKS 内部端口（watchdog 健康检查走这里）
			map[string]interface{}{
				"tag":      "socks-in",
				"port":     config.XraySocksPort,
				"listen":   "127.0.0.1",
				"protocol": "socks",
				"settings": map[string]interface{}{
					"auth": "noauth",
					"udp":  true,
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{
				"tag":      "direct",
				"protocol": "freedom",
				"streamSettings": map[string]interface{}{
					// freedom.domainStrategy 已弃用，改写在 sockopt
					"sockopt": map[string]interface{}{"mark": 255, "domainStrategy": "UseIP"},
				},
			},
			map[string]interface{}{
				"tag":      "block",
				"protocol": "blackhole",
			},
		},
		"policy": map[string]interface{}{
			"levels": map[string]interface{}{
				"0": map[string]interface{}{
					"handshake":    4,
					"connIdle":     300,
					"uplinkOnly":   1,
					"downlinkOnly": 1,
				},
			},
		},
	}
}

// buildProxyOutbound: VLESS + XHTTP + TLS（1.8.0 起唯一协议，不再看 n.Protocol）
// 服务端架构：客户端 → TLS(h2)+VLESS+XHTTP → Nginx (TLS 终结, grpc_pass h2c) → Xray (VLESS XHTTP) → 出网
func buildProxyOutbound(n *config.Node) map[string]interface{} {
	host := n.Host
	if host == "" {
		host = n.Address
	}
	path := n.WSPath
	if path == "" {
		path = "/"
	}
	return map[string]interface{}{
		"tag":      "proxy",
		"protocol": "vless",
		"settings": map[string]interface{}{
			"vnext": []interface{}{
				map[string]interface{}{
					"address": resolveAddress(n.Address), // 预解析为 IP，避免 Xray 启动时依赖 mosdns（循环依赖）
					"port":    n.Port,
					"users": []interface{}{
						map[string]interface{}{
							"id":         n.UUID,
							"encryption": "none", // 加密交给外层 TLS
						},
					},
				},
			},
		},
		"streamSettings": map[string]interface{}{
			"network":  "xhttp",
			"security": "tls",
			"xhttpSettings": map[string]interface{}{
				"path": path,
				"host": host,
				"mode": "auto",
			},
			"tlsSettings": map[string]interface{}{
				"serverName": host,
				// nginx 开了 http2，XHTTP 在 h2 上复用连接
				"alpn": []string{"h2"},
			},
			"sockopt": map[string]interface{}{
				"mark":        255,
				"tcpFastOpen": true,
			},
		},
	}
}

// resolveCachePath 记录每个节点域名上次成功解析出的 IP（{"域名":"IP"}），解析失败时沿用
var resolveCachePath = config.InstallDir + "/data/resolve-cache.json"

// 预解析用的国内 DNS（在 nftables cn_ips 预置里，直连，不需要走代理）
var resolveDNSServers = []string{"223.5.5.5:53", "119.29.29.29:53"}

// lookupIPv4 通过指定 DNS 解析域名，返回第一个 IPv4，失败返回空串（测试中可替换）
var lookupIPv4 = func(dnsServer, host string) string {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := r.LookupHost(ctx, host)
	if err != nil {
		return ""
	}
	for _, ip := range ips {
		if p := net.ParseIP(ip); p != nil && p.To4() != nil {
			return ip
		}
	}
	return ""
}

// resolveAddress 把节点域名解析为 IPv4，用于在生成 Xray 配置时预解析 VPS 域名，避免运行时依赖 mosdns。
// 依次尝试各国内 DNS（不走系统 resolv.conf，避免循环依赖）；都失败时沿用上次成功解析的 IP：
// 解析失败多发生在网络异常时，若把域名写进配置，Xray 只能经 mosdns → 国外 DoH → 代理自身去解析，形成循环。
// 从未解析成功过的域名只能原样返回，交给 Xray 自己尝试。
func resolveAddress(addr string) string {
	if net.ParseIP(addr) != nil {
		return addr
	}
	cache := map[string]string{}
	if b, err := os.ReadFile(resolveCachePath); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	for _, dns := range resolveDNSServers {
		if ip := lookupIPv4(dns, addr); ip != "" {
			if cache[addr] != ip {
				cache[addr] = ip
				if b, err := json.Marshal(cache); err == nil {
					_ = config.WriteFileAtomic(resolveCachePath, b, 0644)
				}
			}
			return ip
		}
	}
	if ip := cache[addr]; ip != "" {
		return ip
	}
	return addr
}

// 路由规则（v1.1.1 起简化）：
// 信任 nftables 的分流决策——能到达 Xray 的流量都应该走代理。
// nftables 已经在前面完成了所有分流：国内 IP / 白名单 IP / 私网都不会到达 Xray。
// 因此 Xray 内部不再做 geoip:cn / geosite 二次判断——避免和 nftables 决策冲突
// （特别是黑名单：nftables 把它送进 Xray 想强制走代理，Xray 又用 geoip 判断为 cn 直连——翻案了 nftables 的决定）。
//
// 仅保留 geoip:private 作为最后的安全兜底——万一某种异常路径让私网包到了 Xray，避免代理私网。
func buildRoutingWithProxy() map[string]interface{} {
	return map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"ip":          []string{"geoip:private"},
				"outboundTag": "direct",
			},
			// 屏蔽走代理的 QUIC：套在 XHTTP 的 TCP 隧道里会双重重传，浏览器被拒后自动退回 TCP/h2
			map[string]interface{}{
				"type":        "field",
				"network":     "udp",
				"port":        "443",
				"outboundTag": "block",
			},
			map[string]interface{}{
				"type":        "field",
				"network":     "tcp,udp",
				"outboundTag": "proxy",
			},
		},
	}
}

func buildRoutingDirectOnly() map[string]interface{} {
	return map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"network":     "tcp,udp",
				"outboundTag": "direct",
			},
		},
	}
}

// SwitchToNextNode 故障转移：按 Order 切换到下一个节点
// 所有节点都是候选；切换后 Enabled 字段跟随 CurrentNodeID 同步翻转
func SwitchToNextNode() error {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		return err
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		return err
	}

	if len(nodes) == 0 {
		return errors.New("没有任何节点")
	}

	// 按 Order 排序的索引列表
	sorted := make([]int, len(nodes))
	for i := range nodes {
		sorted[i] = i
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if nodes[sorted[i]].Order > nodes[sorted[j]].Order {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// 找当前位置
	curIdx := -1
	for i, idx := range sorted {
		if nodes[idx].ID == cfg.CurrentNodeID {
			curIdx = i
			break
		}
	}
	nextIdx := (curIdx + 1) % len(sorted)
	if curIdx == -1 {
		nextIdx = 0
	}
	nextID := nodes[sorted[nextIdx]].ID

	cfg.AutoSwitched = true // 自动故障转移：前面的节点恢复后允许回切
	return applyNodeSwitch(cfg, nodes, nextID)
}

// applyNodeSwitch 切换到指定节点：同步 Enabled、保存配置、渲染并重启 Xray
// 调用前应已设置好 cfg.AutoSwitched
func applyNodeSwitch(cfg *config.WebUIConfig, nodes []config.Node, nextID string) error {
	// 同步 Enabled：只有 next 是 true
	for i := range nodes {
		nodes[i].Enabled = nodes[i].ID == nextID
	}
	if err := config.SaveNodes(nodes); err != nil {
		return err
	}

	cfg.CurrentNodeID = nextID
	if err := config.SaveWebUIConfig(cfg); err != nil {
		return err
	}
	if err := RenderXrayConfig(nodes, cfg.CurrentNodeID); err != nil {
		return err
	}
	return RestartXray()
}

// RestartXray 重启 Xray 服务，并验证 2 秒后仍在运行
// systemctl restart 是异步的，命令返回 0 不代表 Xray 真的稳定运行
// 这里等待 2 秒后用 systemctl is-active 验证，捕获崩溃情况
func RestartXray() error {
	if err := RunCmdErr(CmdTimeoutLong, "systemctl", "restart", "xray"); err != nil {
		return fmt.Errorf("systemctl restart 失败: %w", err)
	}
	// 等待 2 秒让 Xray 完成启动（或崩溃）
	time.Sleep(2 * time.Second)
	if !IsServiceActive("xray") {
		// 取最近 20 行 Xray 日志返回给调用者
		out, _ := RunCmdCombined(CmdTimeoutShort, "journalctl", "-u", "xray", "-n", "20", "--no-pager")
		return fmt.Errorf("Xray 启动后崩溃，最近日志:\n%s", string(out))
	}
	return nil
}

// TestNode 用 SOCKS 出站测试节点连通性，返回延迟
func TestNode(socksPort int) (time.Duration, error) {
	start := time.Now()
	cmd := exec.Command("curl",
		"-s", "-o", "/dev/null",
		"-w", "%{http_code}",
		"--max-time", "8",
		"--socks5-hostname", fmt.Sprintf("127.0.0.1:%d", socksPort),
		"https://www.google.com/generate_204",
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	if string(out) != "204" {
		return 0, fmt.Errorf("非预期响应: %s", string(out))
	}
	return time.Since(start), nil
}
