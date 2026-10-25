package handlers

import (
	"mime"
	"strings"
	"testing"
)

// peer 名称会写进 wg0.conf 的注释行：带换行就能另起一行写入任意配置项（如 PostUp），必须拒绝
func TestValidatePeerName(t *testing.T) {
	bad := []string{"", "   ", "a\nPostUp = rm -rf /", "a\rb", "tab\tname", strings.Repeat("长", 65)}
	for _, n := range bad {
		if err := validatePeerName(n); err == nil {
			t.Errorf("名称 %q 应被拒绝", n)
		}
	}
	for _, n := range []string{"手机", "Alex's iPhone", "laptop-01", strings.Repeat("长", 64)} {
		if err := validatePeerName(n); err != nil {
			t.Errorf("名称 %q 应允许: %v", n, err)
		}
	}
}

// 中文名直接放进 filename="..." 会被浏览器按 Latin-1 解码成乱码，需用 RFC 2231 编码
func TestPeerConfigDisposition(t *testing.T) {
	for _, name := range []string{"手机", `a"b`, "laptop"} {
		_, params, err := mime.ParseMediaType(peerConfigDisposition(name))
		if err != nil {
			t.Fatalf("%q: 头部无法解析: %v", name, err)
		}
		if params["filename"] != name+".conf" {
			t.Errorf("%q: 解析出的文件名为 %q", name, params["filename"])
		}
	}
}

// Endpoint 优先级：URL 参数 > 已保存 > 公网 IP（不能退回局域网 IP，外网用不了）
func TestPickEndpoint(t *testing.T) {
	wan := func() string { return "203.0.113.5" }
	if got := pickEndpoint("q.example.com", "saved.example.com", wan); got != "q.example.com" {
		t.Errorf("URL 参数优先，实际 %q", got)
	}
	if got := pickEndpoint("", "saved.example.com", wan); got != "saved.example.com" {
		t.Errorf("其次用已保存的，实际 %q", got)
	}
	if got := pickEndpoint("", "", wan); got != "203.0.113.5" {
		t.Errorf("都没有时用公网 IP，实际 %q", got)
	}
	if got := pickEndpoint("", "", func() string { return "" }); got != "" {
		t.Errorf("取不到公网 IP 时返回空，由调用方提示，实际 %q", got)
	}
}
