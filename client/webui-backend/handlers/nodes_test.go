package handlers

import (
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

func validNode(protocol string) config.Node {
	return config.Node{Name: "n", Address: "vps.example.com", Port: 443, UUID: "u", WSPath: "/p", Protocol: protocol}
}

func TestValidateNodeProtocol(t *testing.T) {
	n := validNode(config.ProtocolVLESSXHTTP)
	if err := validateNode(&n); err != nil {
		t.Fatalf("vless-xhttp 应合法，实际报错 %v", err)
	}
	for _, p := range []string{"", "vmess-ws", "trojan"} {
		n := validNode(p)
		if err := validateNode(&n); err == nil {
			t.Fatalf("protocol=%q 应被拒绝", p)
		}
	}
}

// 1.8.0 起只有 XHTTP：旧备份里的空值 / vmess-ws 节点改按 XHTTP 保存（UUID 与路径通用）
func TestApplyNodeDefaultsForcesXHTTP(t *testing.T) {
	for _, p := range []string{"", "vmess-ws"} {
		n := validNode(p)
		applyNodeDefaults(&n)
		if n.Protocol != config.ProtocolVLESSXHTTP {
			t.Fatalf("protocol=%q 应被改为 vless-xhttp，实际 %q", p, n.Protocol)
		}
		if err := validateNode(&n); err != nil {
			t.Fatalf("填充默认值后应能通过校验，实际报错 %v", err)
		}
	}
}
