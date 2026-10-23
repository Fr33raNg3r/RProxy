package handlers

import (
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

func validNode(protocol string) config.Node {
	return config.Node{Name: "n", Address: "vps.example.com", Port: 443, UUID: "u", WSPath: "/p", Protocol: protocol}
}

func TestValidateNodeProtocol(t *testing.T) {
	for _, p := range []string{"", config.ProtocolVMessWS, config.ProtocolVLESSXHTTP} {
		n := validNode(p)
		if err := validateNode(&n); err != nil {
			t.Fatalf("protocol=%q 应合法，实际报错 %v", p, err)
		}
	}
	n := validNode("trojan")
	if err := validateNode(&n); err == nil {
		t.Fatal("未知 protocol 应被拒绝")
	}
}

// Xray v26.10.21 已移除 VMess 的 none / zero，允许保存会导致配置渲染失败
func TestValidateNodeRejectsRemovedVMessSecurity(t *testing.T) {
	for _, s := range []string{"none", "zero"} {
		n := validNode(config.ProtocolVMessWS)
		n.Security = s
		if err := validateNode(&n); err == nil {
			t.Fatalf("security=%q 应被拒绝", s)
		}
	}
}

// VLESS 不使用 VMess 的 security 字段，残留值不应阻止保存
func TestValidateNodeXHTTPIgnoresVMessSecurity(t *testing.T) {
	n := validNode(config.ProtocolVLESSXHTTP)
	n.Security = "none"
	if err := validateNode(&n); err != nil {
		t.Fatalf("XHTTP 节点不应校验 security，实际报错 %v", err)
	}
}
