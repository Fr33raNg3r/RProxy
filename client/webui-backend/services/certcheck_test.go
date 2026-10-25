package services

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 剩余时间不足总有效期的 1/5 才告警：90 天证书约剩 18 天，45 天证书约剩 9 天
func TestCertNeedsWarn(t *testing.T) {
	day := 24 * time.Hour
	issued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		lifetime, elapsed time.Duration
		want              bool
	}{
		{90 * day, 60 * day, false}, // 正常续期时间点（剩 30 天）
		{90 * day, 71 * day, false}, // 剩 19 天
		{90 * day, 73 * day, true},  // 剩 17 天 < 18 天
		{45 * day, 35 * day, false}, // 剩 10 天
		{45 * day, 37 * day, true},  // 剩 8 天 < 9 天
		{90 * day, 95 * day, true},  // 已过期
	}
	for _, c := range cases {
		if got := certNeedsWarn(issued, issued.Add(c.lifetime), issued.Add(c.elapsed)); got != c.want {
			t.Errorf("有效期 %v、已过 %v：期望 %v，实际 %v", c.lifetime, c.elapsed, c.want, got)
		}
	}
}

// 能取到节点证书的有效期；不校验证书链（过期证书也要能读出到期时间）
func TestFetchNodeCert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	cert, err := fetchNodeCert(config.Node{Address: host, Host: "example.com", Port: port}, 3*time.Second)
	if err != nil {
		t.Fatalf("取证书失败: %v", err)
	}
	if cert.NotAfter.Before(cert.NotBefore) || cert.NotAfter.IsZero() {
		t.Fatalf("有效期异常 %v ~ %v", cert.NotBefore, cert.NotAfter)
	}
}

func TestCertAlertMessage(t *testing.T) {
	a := CertAlert{NodeName: "东京", Host: "a.example.com", NotAfter: time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC), DaysLeft: 12}
	msg := a.Message()
	for _, want := range []string{"东京", "a.example.com", "2026-03-01", "12"} {
		if !strings.Contains(msg, want) {
			t.Errorf("告警文字缺少 %q：%s", want, msg)
		}
	}
}
