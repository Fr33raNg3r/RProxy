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

// 删过节点后 Order 不连续（如剩 1、2），新节点若取"节点数"会与已有节点重复，排序与故障转移顺序随之不确定
func TestNextNodeOrder(t *testing.T) {
	if got := nextNodeOrder(nil); got != 0 {
		t.Fatalf("没有节点时应为 0，实际 %d", got)
	}
	nodes := []config.Node{{ID: "a", Order: 1}, {ID: "b", Order: 2}}
	if got := nextNodeOrder(nodes); got != 3 {
		t.Fatalf("应排在最大 Order 之后（3），实际 %d", got)
	}
}
