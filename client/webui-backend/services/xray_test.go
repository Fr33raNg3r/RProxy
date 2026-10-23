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
