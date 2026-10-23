package services

import (
	"path/filepath"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// Xray v26.10.21 起按扩展名识别配置文件，-format=json 也不能放行 .tmp，
// 校验用的临时文件必须以 .json 结尾，否则切换节点时 -test 必然失败。
func TestXrayTmpConfPathHasJSONExtension(t *testing.T) {
	if ext := filepath.Ext(xrayTmpConfPath); ext != ".json" {
		t.Fatalf("临时配置文件扩展名应为 .json，实际 %q（%s）", ext, xrayTmpConfPath)
	}
	if xrayTmpConfPath == config.XrayConfPath {
		t.Fatalf("临时配置文件不能与正式配置同名")
	}
}

func streamOf(out map[string]interface{}) map[string]interface{} {
	return out["streamSettings"].(map[string]interface{})
}

// 1.6.0 之前的节点没有 protocol 字段，必须仍按 VMess + WS 渲染，升级后行为不变
func TestLegacyNodeStillRendersVMessWS(t *testing.T) {
	out := buildProxyOutbound(&config.Node{Address: "1.2.3.4", Host: "example.com", Port: 443, UUID: "u", WSPath: "/p"})
	if out["protocol"] != "vmess" || streamOf(out)["network"] != "ws" {
		t.Fatalf("旧节点应渲染为 vmess+ws，实际 %v / %v", out["protocol"], streamOf(out)["network"])
	}
}

func TestXHTTPOutbound(t *testing.T) {
	n := &config.Node{Protocol: config.ProtocolVLESSXHTTP, Address: "1.2.3.4", Host: "example.com", Port: 443, UUID: "u", WSPath: "/p"}
	out := buildProxyOutbound(n)
	if out["protocol"] != "vless" {
		t.Fatalf("protocol 应为 vless，实际 %v", out["protocol"])
	}
	vnext := out["settings"].(map[string]interface{})["vnext"].([]interface{})[0].(map[string]interface{})
	user := vnext["users"].([]interface{})[0].(map[string]interface{})
	if user["id"] != "u" || user["encryption"] != "none" {
		t.Fatalf("VLESS 用户应为 id=u encryption=none，实际 %v", user)
	}
	st := streamOf(out)
	if st["network"] != "xhttp" || st["security"] != "tls" {
		t.Fatalf("应为 xhttp + tls，实际 %v / %v", st["network"], st["security"])
	}
	xh := st["xhttpSettings"].(map[string]interface{})
	if xh["path"] != "/p" || xh["host"] != "example.com" {
		t.Fatalf("xhttpSettings 的 path/host 不对：%v", xh)
	}
	tls := st["tlsSettings"].(map[string]interface{})
	if alpn, _ := tls["alpn"].([]string); len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("XHTTP 走 h2，alpn 应为 [h2]，实际 %v", tls["alpn"])
	}
	// 透明代理防环路依赖出站 mark=255
	if st["sockopt"].(map[string]interface{})["mark"] != 255 {
		t.Fatalf("出站必须带 mark=255")
	}
}

// Xray v26.10.21 起 alpn 恰为 ["h2","http/1.1"] 时会把 h2 原样发出，
// nginx 开了 http2 会选中 h2，而 WS 只能跑在 http/1.1 上，导致连不通。
func TestProxyOutboundALPNIsHTTP11Only(t *testing.T) {
	out := buildProxyOutbound(&config.Node{Address: "1.2.3.4", Host: "example.com", Port: 443, UUID: "u"})
	tls := out["streamSettings"].(map[string]interface{})["tlsSettings"].(map[string]interface{})
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "http/1.1" {
		t.Fatalf("alpn 应仅为 [http/1.1]，实际 %v", tls["alpn"])
	}
}
