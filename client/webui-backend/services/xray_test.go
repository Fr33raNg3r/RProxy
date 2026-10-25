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

// 1.8.0 起服务端只有 VLESS + XHTTP：残留的空值 / vmess-ws 节点也必须按 XHTTP 渲染
func TestProxyOutboundAlwaysXHTTP(t *testing.T) {
	for _, p := range []string{"", "vmess-ws", config.ProtocolVLESSXHTTP} {
		out := buildProxyOutbound(&config.Node{Protocol: p, Address: "1.2.3.4", Host: "example.com", Port: 443, UUID: "u", WSPath: "/p"})
		if out["protocol"] != "vless" || streamOf(out)["network"] != "xhttp" {
			t.Fatalf("protocol=%q 应渲染为 vless+xhttp，实际 %v / %v", p, out["protocol"], streamOf(out)["network"])
		}
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

// dokodemo-door 已改名为 tunnel，network 字段改名为 allowedNetwork（旧名目前仍兼容）
func TestTProxyInboundUsesTunnel(t *testing.T) {
	in := buildXrayBaseConfig()["inbounds"].([]interface{})[0].(map[string]interface{})
	if in["tag"] != "tproxy-in" || in["protocol"] != "tunnel" {
		t.Fatalf("TPROXY 入口应为 tunnel，实际 %v / %v", in["tag"], in["protocol"])
	}
	s := in["settings"].(map[string]interface{})
	if s["allowedNetwork"] != "tcp,udp" || s["followRedirect"] != true {
		t.Fatalf("tunnel 应为 allowedNetwork=tcp,udp + followRedirect，实际 %v", s)
	}
	if _, ok := s["network"]; ok {
		t.Fatalf("不应再使用旧字段 network")
	}
}

// 路由不按域名匹配，嗅探结果用不上，入站不再开 sniffing
func TestInboundsHaveNoSniffing(t *testing.T) {
	for _, v := range buildXrayBaseConfig()["inbounds"].([]interface{}) {
		in := v.(map[string]interface{})
		if _, ok := in["sniffing"]; ok {
			t.Fatalf("入站 %v 不应开启 sniffing", in["tag"])
		}
	}
}

// freedom.domainStrategy 已弃用，须写在 sockopt.domainStrategy
func TestDirectOutboundDomainStrategyInSockopt(t *testing.T) {
	var direct map[string]interface{}
	for _, v := range buildXrayBaseConfig()["outbounds"].([]interface{}) {
		if o := v.(map[string]interface{}); o["tag"] == "direct" {
			direct = o
		}
	}
	if direct == nil {
		t.Fatal("缺少 direct 出站")
	}
	if _, ok := direct["settings"]; ok {
		t.Fatalf("direct 不应再有 settings.domainStrategy，实际 %v", direct["settings"])
	}
	so := streamOf(direct)["sockopt"].(map[string]interface{})
	if so["domainStrategy"] != "UseIP" || so["mark"] != 255 {
		t.Fatalf("direct 的 sockopt 应为 domainStrategy=UseIP + mark=255，实际 %v", so)
	}
}

// QUIC 套在 TCP 隧道里会双重重传，屏蔽后浏览器自动退回 TCP；规则须在 proxy 兜底规则之前
func TestRoutingBlocksQUICBeforeProxy(t *testing.T) {
	rules := buildRoutingWithProxy()["rules"].([]interface{})
	quic, proxy := -1, -1
	for i, v := range rules {
		r := v.(map[string]interface{})
		if r["network"] == "udp" && r["port"] == "443" && r["outboundTag"] == "block" {
			quic = i
		}
		if r["outboundTag"] == "proxy" {
			proxy = i
		}
	}
	if quic < 0 || proxy < 0 || quic > proxy {
		t.Fatalf("应有 udp/443 → block 规则且位于 proxy 规则之前，实际 %v", rules)
	}
}

// access log 会为每条透明代理连接写一行，写盘量大、磨损 eMMC/SD 卡，默认关闭；error log 保留用于排障
func TestXrayAccessLogDisabled(t *testing.T) {
	logCfg := buildXrayBaseConfig()["log"].(map[string]interface{})
	if logCfg["access"] != "none" {
		t.Fatalf("access log 应为 none，实际 %v", logCfg["access"])
	}
	if logCfg["error"] != config.XrayErrorLogPath {
		t.Fatalf("error log 应保留为 %s，实际 %v", config.XrayErrorLogPath, logCfg["error"])
	}
}
