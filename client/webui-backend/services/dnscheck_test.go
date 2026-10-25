package services

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// fakeDNSServer 起一个只会回答 A 记录的本地 UDP DNS：answer=false 时收到查询不回复（模拟 mosdns 卡死）
func fakeDNSServer(t *testing.T, answer bool) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if !answer || n < 12 {
				continue
			}
			// 只带回问题段（域名 + QTYPE + QCLASS）；查询里还可能带 EDNS 附加记录，不能一并带回
			end := 12
			for end < n && buf[end] != 0 {
				end += int(buf[end]) + 1
			}
			end += 5
			if end > n {
				continue
			}
			q := buf[12:end]
			resp := make([]byte, 12, 12+len(q)+16)
			copy(resp[0:2], buf[0:2])                    // ID
			binary.BigEndian.PutUint16(resp[2:], 0x8180) // 标准应答、无错误
			binary.BigEndian.PutUint16(resp[4:], 1)      // QDCOUNT
			binary.BigEndian.PutUint16(resp[6:], 1)      // ANCOUNT
			resp = append(resp, q...)
			// 答案：指向问题里的域名，A 记录，TTL 60，1.2.3.4
			resp = append(resp, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 1, 2, 3, 4)
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	return pc.LocalAddr().String()
}

func TestCheckDNSAnswers(t *testing.T) {
	addr := fakeDNSServer(t, true)
	if err := CheckDNS(addr, "www.baidu.com", 2*time.Second); err != nil {
		t.Fatalf("能应答的 DNS 应检查通过: %v", err)
	}
}

// mosdns 进程还在但不应答：必须在超时内判为失败，watchdog 才会去重启它
func TestCheckDNSHungServerFails(t *testing.T) {
	addr := fakeDNSServer(t, false)
	start := time.Now()
	if err := CheckDNS(addr, "www.baidu.com", time.Second); err == nil {
		t.Fatal("不应答的 DNS 应判为失败")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("超时控制失效，耗时 %v", d)
	}
}
