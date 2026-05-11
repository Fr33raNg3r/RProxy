package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// RenderXrayConfig 根据节点池和当前节点 ID 生成 Xray 配置文件
// 如果 currentNodeID 为空，生成一个最小可用配置（无代理出站）
func RenderXrayConfig(nodes []config.Node, currentNodeID string) error {
	cfg := buildXrayBaseConfig()

	// 找到当前节点
	var current *config.Node
	if currentNodeID != "" {
		for i := range nodes {
			if nodes[i].ID == currentNodeID && nodes[i].Enabled {
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

	tmp := config.XrayConfPath + ".tmp"
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}

	// 验证配置语法（用 -format=json 显式指定）
	if out, err := exec.Command("xray", "run", "-test", "-format=json", "-c", tmp).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("xray 配置验证失败: %s\n%s", err, string(out))
	}

	return os.Rename(tmp, config.XrayConfPath)
}

func buildXrayBaseConfig() map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			// 写到 /var/log/xray/，这个目录由 Xray 官方安装脚本创建，nobody 有写权限
			"access": "/var/log/xray/access.log",
			"error":  "/var/log/xray/error.log",
		},
		"inbounds": []interface{}{
			// TPROXY 入口
			map[string]interface{}{
				"tag":      "tproxy-in",
				"port":     12345,
				"protocol": "dokodemo-door",
				"settings": map[string]interface{}{
					"network":        "tcp,udp",
					"followRedirect": true,
				},
				"streamSettings": map[string]interface{}{
					"sockopt": map[string]interface{}{
						"tproxy": "tproxy",
						"mark":   255,
					},
				},
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
					"routeOnly":    true,
				},
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
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
					"routeOnly":    true,
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{
				"tag":      "direct",
				"protocol": "freedom",
				"settings": map[string]interface{}{
					"domainStrategy": "UseIP",
				},
				"streamSettings": map[string]interface{}{
					"sockopt": map[string]interface{}{"mark": 255},
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

// buildProxyOutbound: VMess + WebSocket + TLS
// 服务端架构：客户端 → TLS+VMess+WS → Nginx (TLS 终结) → Xray (VMess WS) → 出网
// 所以客户端的 streamSettings.security = "tls"，network = "ws"
func buildProxyOutbound(n *config.Node) map[string]interface{} {
	host := n.Host
	if host == "" {
		host = n.Address // 默认与 address 相同
	}
	security := n.Security
	if security == "" {
		security = "auto"
	}
	wsPath := n.WSPath
	if wsPath == "" {
		wsPath = "/"
	}

	return map[string]interface{}{
		"tag":      "proxy",
		"protocol": "vmess",
		"settings": map[string]interface{}{
			"vnext": []interface{}{
				map[string]interface{}{
					"address": n.Address, // 用域名连接，不解析为 IP
					"port":    n.Port,
					"users": []interface{}{
						map[string]interface{}{
							"id":       n.UUID,
							"alterId":  n.AlterID,
							"security": security,
						},
					},
				},
			},
		},
		"streamSettings": map[string]interface{}{
			"network":  "ws",
			"security": "tls",
			"wsSettings": map[string]interface{}{
				"path": wsPath,
				"headers": map[string]interface{}{
					"Host": host,
				},
			},
			"tlsSettings": map[string]interface{}{
				"serverName":    host,
				"allowInsecure": false,
				"alpn":          []string{"h2", "http/1.1"},
			},
			"sockopt": map[string]interface{}{
				"mark":        255,
				"tcpFastOpen": true,
			},
		},
	}
}

// 路由规则：
// - 局域网 IP 直连
// - geoip:cn 直连（兜底，正常情况下国内 IP 已经在 nftables 层就绕过 Xray 了）
// - geosite:cn 直连
// - geosite:geolocation-!cn 走代理
// - 其余走代理
func buildRoutingWithProxy() map[string]interface{} {
	return map[string]interface{}{
		"domainStrategy": "IPIfNonMatch",
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"ip":          []string{"geoip:private"},
				"outboundTag": "direct",
			},
			map[string]interface{}{
				"type":        "field",
				"ip":          []string{"geoip:cn"},
				"outboundTag": "direct",
			},
			map[string]interface{}{
				"type":        "field",
				"domain":      []string{"geosite:cn", "geosite:private"},
				"outboundTag": "direct",
			},
			map[string]interface{}{
				"type":        "field",
				"domain":      []string{"geosite:geolocation-!cn"},
				"outboundTag": "proxy",
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

// SwitchToNextNode 故障转移：切换到下一个 enabled 节点
func SwitchToNextNode() error {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		return err
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		return err
	}

	enabled := []config.Node{}
	for _, n := range nodes {
		if n.Enabled {
			enabled = append(enabled, n)
		}
	}
	if len(enabled) == 0 {
		return errors.New("没有任何启用的节点")
	}

	// 按 Order 排序
	for i := 0; i < len(enabled); i++ {
		for j := i + 1; j < len(enabled); j++ {
			if enabled[i].Order > enabled[j].Order {
				enabled[i], enabled[j] = enabled[j], enabled[i]
			}
		}
	}

	// 找当前位置
	curIdx := -1
	for i, n := range enabled {
		if n.ID == cfg.CurrentNodeID {
			curIdx = i
			break
		}
	}
	nextIdx := (curIdx + 1) % len(enabled)
	if curIdx == -1 {
		nextIdx = 0
	}
	cfg.CurrentNodeID = enabled[nextIdx].ID
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
	if err := exec.Command("systemctl", "restart", "xray").Run(); err != nil {
		return fmt.Errorf("systemctl restart 失败: %w", err)
	}
	// 等待 2 秒让 Xray 完成启动（或崩溃）
	time.Sleep(2 * time.Second)
	if !IsServiceActive("xray") {
		// 取最近 20 行 Xray 日志返回给调用者
		out, _ := exec.Command("journalctl", "-u", "xray", "-n", "20", "--no-pager").CombinedOutput()
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
