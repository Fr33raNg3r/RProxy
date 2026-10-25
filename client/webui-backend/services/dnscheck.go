package services

import (
	"context"
	"fmt"
	"net"
	"time"
)

// CheckDNS 向指定 DNS 服务器查询域名的 A 记录，超时或没有结果返回错误。
// watchdog 用它发现"mosdns 进程还在但不应答"（只看进程是否在运行发现不了，局域网会全部解析失败）
func CheckDNS(server, domain string, timeout time.Duration) error {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, "udp", server)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ips, err := r.LookupIP(ctx, "ip4", domain)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("%s 没有返回 A 记录", domain)
	}
	return nil
}
