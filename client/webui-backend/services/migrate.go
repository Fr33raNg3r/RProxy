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
// 用户明确选择 vmess-ws 的节点不碰。返回被迁移的、以及探测未通过的节点 ID。
func migrateLegacyNodes(nodes []config.Node, probe func(config.Node) bool) (changed, failed []string) {
	for i := range nodes {
		if nodes[i].Protocol != "" {
			continue
		}
		candidate := nodes[i]
		candidate.Protocol = config.ProtocolVLESSXHTTP
		if probe(candidate) {
			nodes[i].Protocol = config.ProtocolVLESSXHTTP
			changed = append(changed, nodes[i].ID)
		} else {
			failed = append(failed, nodes[i].ID)
		}
	}
	return changed, failed
}

// nodeNames 按 ID 列出节点名称，用于日志
func nodeNames(nodes []config.Node, ids []string) string {
	var names []string
	for _, id := range ids {
		for _, n := range nodes {
			if n.ID == id {
				names = append(names, n.Name)
			}
		}
	}
	return strings.Join(names, "、")
}

// MigrateXHTTP 由 install.sh upgrade 和每日更新调用：
// 自动把能连通 XHTTP 的旧节点迁移过去；当前节点被迁移时重新渲染并重启 Xray。
// 返回给日志看的结果说明。
func MigrateXHTTP() (string, error) {
	nodes, err := config.LoadNodes()
	if err != nil {
		return "", err
	}
	changed, failed := migrateLegacyNodes(nodes, func(n config.Node) bool {
		return probeNodeOnPort(n, migrateSocksPort)
	})
	var notes []string
	if len(failed) > 0 {
		notes = append(notes, fmt.Sprintf("XHTTP 探测未通过、暂保持 WS：%s（服务端可能尚未升级到 1.6.0，或节点暂时不通；每日更新时会再试）",
			nodeNames(nodes, failed)))
	}
	if len(changed) == 0 {
		if len(notes) == 0 {
			return "没有待迁移的旧节点", nil
		}
		return strings.Join(notes, "\n"), nil
	}
	if err := config.SaveNodes(nodes); err != nil {
		return "", err
	}

	currentChanged := false
	cfg, _ := config.LoadWebUIConfig()
	for _, id := range changed {
		if cfg != nil && cfg.CurrentNodeID == id {
			currentChanged = true
		}
	}
	msg := strings.Join(append([]string{"已迁移到 VLESS+XHTTP：" + nodeNames(nodes, changed)}, notes...), "\n")
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
