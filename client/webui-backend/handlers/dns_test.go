package handlers

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 保存黑白名单后 mosdns 起不来（如某行规则它无法解析）：文件恢复原样并再应用一次，
// 局域网 DNS 回到保存前的状态，同时把错误返回给用户
func TestApplyDNSFilesRollsBackWhenApplyFails(t *testing.T) {
	f := filepath.Join(t.TempDir(), "whitelist.txt")
	if err := os.WriteFile(f, []byte("old.example.com"), 0600); err != nil {
		t.Fatal(err)
	}
	applies := 0
	err := applyDNSFiles([]string{f},
		func() error { return os.WriteFile(f, []byte("regexp:["), 0600) },
		func() error {
			applies++
			if applies == 1 {
				return errors.New("mosdns 启动后崩溃")
			}
			return nil
		})
	if err == nil {
		t.Fatal("应用失败时应返回错误")
	}
	if b, _ := os.ReadFile(f); string(b) != "old.example.com" {
		t.Fatalf("文件应恢复为原内容，实际 %q", b)
	}
	if applies != 2 {
		t.Fatalf("恢复文件后应再应用一次，实际应用 %d 次", applies)
	}
}

func TestApplyDNSFilesKeepsNewContentOnSuccess(t *testing.T) {
	f := filepath.Join(t.TempDir(), "whitelist.txt")
	applies := 0
	err := applyDNSFiles([]string{f},
		func() error { return os.WriteFile(f, []byte("new.example.com"), 0600) },
		func() error { applies++; return nil })
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if b, _ := os.ReadFile(f); string(b) != "new.example.com" || applies != 1 {
		t.Fatalf("成功时应保留新内容且只应用一次，实际 %q、%d 次", b, applies)
	}
}

// DNS 上游校验：支持 IP 与域名、可带端口；写法错误、端口越界、含引号等会弄坏 mosdns 配置的字符、指向本机，都在保存前拒绝
func TestValidateUpstreamAddr(t *testing.T) {
	self := []net.IP{net.ParseIP("192.168.1.2"), net.ParseIP("172.16.7.1")}
	ok := []string{
		"udp://61.128.128.68", "udp://61.128.128.68:5353", "61.128.128.68", "61.128.128.68:53",
		"tcp://223.5.5.5", "tls://dns.alidns.com", "tls://1.1.1.1:853",
		"https://1.1.1.1/dns-query", "https://dns.google/dns-query", "https://doh.pub:443/dns-query",
		"dns.alidns.com", "HTTPS://Dns.Google/dns-query",
	}
	for _, a := range ok {
		if err := validateUpstreamAddr(a, self); err != nil {
			t.Errorf("%q 应合法，实际报错 %v", a, err)
		}
	}
	bad := []string{
		"quic://1.1.1.1",                                           // 不支持的协议
		"udp://",                                                   // 没有主机
		"udp://1.1.1.1:0", "udp://1.1.1.1:70000", "udp://1.1.1.1:", // 端口越界 / 空端口
		"udp://1.1.1.1/x",                                                                                     // 非 https 不能带路径
		`https://1.1.1.1/dns-query"`,                                                                          // 引号会弄坏 YAML
		"udp://1.1.1.1 #x",                                                                                    // 空格
		"udp://bad_domain..com",                                                                               // 域名格式错误
		"udp://300.1.1.1",                                                                                     // 非法 IP（按域名也不合法）
		"udp://[2001:db8::1]",                                                                                 // IPv6 不支持（客户端已禁用 IPv6）
		"udp://127.0.0.1", "127.0.0.53:53", "udp://0.0.0.0", "udp://localhost", "https://localhost/dns-query", // 本机
		"udp://192.168.1.2", "udp://172.16.7.1:53", // 本机网卡 IP
	}
	for _, a := range bad {
		if err := validateUpstreamAddr(a, self); err == nil {
			t.Errorf("%q 应被拒绝", a)
		}
	}
}

// 报错要指明是国内还是国外上游的第几行，方便用户定位
func TestValidateDNSUpstreamsReportsLine(t *testing.T) {
	u := config.DNSUpstreams{
		Local:  []string{"udp://61.128.128.68"},
		Remote: []string{"https://1.1.1.1/dns-query", "udp://127.0.0.1"},
	}
	err := validateDNSUpstreams(u, nil)
	if err == nil || !strings.Contains(err.Error(), "国外上游第 2 行") || !strings.Contains(err.Error(), "udp://127.0.0.1") {
		t.Fatalf("应指出国外上游第 2 行，实际 %v", err)
	}
	if err := validateDNSUpstreams(config.DNSUpstreams{Local: []string{"223.5.5.5"}}, nil); err == nil {
		t.Fatal("国外上游为空时应报错")
	}
}
