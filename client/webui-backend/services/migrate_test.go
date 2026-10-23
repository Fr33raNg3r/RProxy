package services

import (
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

func TestMigrateLegacyNodes(t *testing.T) {
	nodes := []config.Node{
		{ID: "ok"},   // 旧节点，探测通过 → 迁移
		{ID: "down"}, // 旧节点，探测失败 → 保持为空，下次再试
		{ID: "ws", Protocol: config.ProtocolVMessWS},    // 用户明确选择 WS → 不碰
		{ID: "xh", Protocol: config.ProtocolVLESSXHTTP}, // 已是 XHTTP → 不碰
	}
	var probed []string
	probe := func(n config.Node) bool {
		if n.Protocol != config.ProtocolVLESSXHTTP {
			t.Fatalf("探测时节点应已改为 vless-xhttp，实际 %q", n.Protocol)
		}
		probed = append(probed, n.ID)
		return n.ID == "ok"
	}

	changed := migrateLegacyNodes(nodes, probe)

	if len(changed) != 1 || changed[0] != "ok" {
		t.Fatalf("应只迁移 [ok]，实际 %v", changed)
	}
	if len(probed) != 2 || probed[0] != "ok" || probed[1] != "down" {
		t.Fatalf("应只探测空协议的节点 [ok down]，实际 %v", probed)
	}
	want := map[string]string{"ok": config.ProtocolVLESSXHTTP, "down": "", "ws": config.ProtocolVMessWS, "xh": config.ProtocolVLESSXHTTP}
	for _, n := range nodes {
		if n.Protocol != want[n.ID] {
			t.Fatalf("节点 %s 的 protocol 应为 %q，实际 %q", n.ID, want[n.ID], n.Protocol)
		}
	}
}
