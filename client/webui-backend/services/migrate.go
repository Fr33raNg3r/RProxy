package services

import (
	"fmt"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 迁移探测用临时 Xray 的 SOCKS 端口（主 Xray 10808，回切探测 10810，错开避免冲突）
const migrateSocksPort = 10811

// migrateLegacyNodes 把 protocol 为空的旧节点逐个改成 vless-xhttp 探测，
// 探测通过的就地改为 vless-xhttp，失败的保持为空（服务端可能还没升级，下次再试）。
// 用户明确选择 vmess-ws 的节点不碰。返回被迁移的节点 ID。
func migrateLegacyNodes(nodes []config.Node, probe func(config.Node) bool) []string {
	var changed []string
	for i := range nodes {
		if nodes[i].Protocol != "" {
			continue
		}
		candidate := nodes[i]
		candidate.Protocol = config.ProtocolVLESSXHTTP
		if probe(candidate) {
			nodes[i].Protocol = config.ProtocolVLESSXHTTP
			changed = append(changed, nodes[i].ID)
		}
	}
	return changed
}

// MigrateXHTTP 由 install.sh upgrade 和每日更新调用：
// 自动把能连通 XHTTP 的旧节点迁移过去；当前节点被迁移时重新渲染并重启 Xray。
// 返回给日志看的结果说明。
func MigrateXHTTP() (string, error) {
	nodes, err := config.LoadNodes()
	if err != nil {
		return "", err
	}
	changed := migrateLegacyNodes(nodes, func(n config.Node) bool {
		return probeNodeOnPort(n, migrateSocksPort)
	})
	if len(changed) == 0 {
		return "没有需要迁移到 XHTTP 的节点（或服务端尚未支持）", nil
	}
	if err := config.SaveNodes(nodes); err != nil {
		return "", err
	}

	var names []string
	currentChanged := false
	cfg, _ := config.LoadWebUIConfig()
	for _, id := range changed {
		for _, n := range nodes {
			if n.ID == id {
				names = append(names, n.Name)
			}
		}
		if cfg != nil && cfg.CurrentNodeID == id {
			currentChanged = true
		}
	}
	msg := fmt.Sprintf("已迁移到 VLESS+XHTTP：%s", strings.Join(names, "、"))
	if currentChanged {
		if err := RenderXrayConfig(nodes, cfg.CurrentNodeID); err != nil {
			return msg, fmt.Errorf("当前节点已迁移，但渲染 Xray 配置失败: %w", err)
		}
		if err := RestartXray(); err != nil {
			return msg, fmt.Errorf("当前节点已迁移，但重启 Xray 失败: %w", err)
		}
		msg += "（当前节点已切换并重启 Xray）"
	}
	return msg, nil
}
