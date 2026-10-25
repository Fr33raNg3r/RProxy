package services

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

// CheckDNS 向指定 DNS 服务器发一个 A 记录查询，只判断它**有没有应答**，超时或端口不通返回错误。
// watchdog 用它发现"mosdns 进程还在但不应答"（只看进程是否在运行发现不了，局域网会全部解析失败）。
// 应答的结果码不管：运营商 DNS 故障时 mosdns 回 SERVFAIL，但它还活着、还在用缓存应答局域网，
// 此时重启只会清空缓存、让断网更严重，所以不能用系统解析器（它把 SERVFAIL 也当成失败）
func CheckDNS(server, domain string, timeout time.Duration) error {
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	id := uint16(rand.Intn(1 << 16))
	if _, err := conn.Write(buildDNSQuery(id, domain)); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("%s 没有应答: %w", server, err)
		}
		// 同一个 ID、QR 位为 1（是应答）就算活着
		if n >= 12 && binary.BigEndian.Uint16(buf[0:2]) == id && buf[2]&0x80 != 0 {
			return nil
		}
	}
}

// buildDNSQuery 构造最简单的标准查询：期望递归、一个问题、A 记录、IN 类
func buildDNSQuery(id uint16, domain string) []byte {
	q := make([]byte, 12, 12+len(domain)+6)
	binary.BigEndian.PutUint16(q[0:], id)
	binary.BigEndian.PutUint16(q[2:], 0x0100) // RD
	binary.BigEndian.PutUint16(q[4:], 1)      // QDCOUNT
	for _, label := range strings.Split(strings.TrimSuffix(domain, "."), ".") {
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	return append(q, 0, 0, 1, 0, 1) // 根标签、QTYPE=A、QCLASS=IN
}
