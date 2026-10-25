package services

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 节点证书到期预警：服务端证书由 acme.sh 自动续期，续期一直失败时到期当天所有客户端 TLS 校验失败、全部断网，
// 而服务端没有界面，只有日志。客户端定期和每个节点做一次 TLS 握手读出证书有效期，快到期时在 WebUI 告警。

const certCheckInterval = 12 * time.Hour

// CertAlert 一个证书快到期（或已过期）的节点
type CertAlert struct {
	NodeID   string    `json:"node_id"`
	NodeName string    `json:"node_name"`
	Host     string    `json:"host"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
}

var cstZone = time.FixedZone("CST", 8*3600)

// Message 给 WebUI 显示的告警文字
func (a CertAlert) Message() string {
	date := a.NotAfter.In(cstZone).Format("2006-01-02")
	if a.DaysLeft < 0 {
		return fmt.Sprintf("节点「%s」（%s）的证书已于 %s 过期，请检查服务端证书自动续期", a.NodeName, a.Host, date)
	}
	return fmt.Sprintf("节点「%s」（%s）的证书将于 %s 到期（剩余 %d 天），请检查服务端证书自动续期", a.NodeName, a.Host, date, a.DaysLeft)
}

// certNeedsWarn 剩余时间不足总有效期的 1/5 时告警（90 天证书约剩 18 天；有效期缩短后按比例自动适应）。
// 正常情况下 acme.sh 在剩约 1/3 时就已续期，不会触发
func certNeedsWarn(notBefore, notAfter, now time.Time) bool {
	total := notAfter.Sub(notBefore)
	if total <= 0 {
		return true
	}
	return notAfter.Sub(now) < total/5
}

// fetchNodeCert 与节点做一次 TLS 握手，取服务端证书。
// 连接打 mark 255（同 Xray 出站）直接发出、不经透明代理，当前节点故障时也能检查其他节点；
// 不校验证书链：目的只是读出有效期，过期的证书也要能读到
func fetchNodeCert(n config.Node, timeout time.Duration) (*x509.Certificate, error) {
	host := n.Host
	if host == "" {
		host = n.Address
	}
	addr := net.JoinHostPort(resolveAddress(n.Address), strconv.Itoa(n.Port))
	d := &net.Dialer{Timeout: timeout, Control: markControl}
	conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
	})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("%s 没有返回证书", addr)
	}
	return certs[0], nil
}

var (
	certMu     sync.Mutex
	certAlerts []CertAlert
)

// CheckNodeCertsNow 检查所有节点的证书；握手失败（节点不通）不告警，那属于连通性问题
func CheckNodeCertsNow() {
	nodes, err := config.LoadNodes()
	if err != nil {
		log.Printf("[cert] 读取节点失败: %v", err)
		return
	}
	now := time.Now()
	alerts := []CertAlert{}
	for _, n := range nodes {
		cert, err := fetchNodeCert(n, 10*time.Second)
		if err != nil {
			log.Printf("[cert] 节点 %s 握手失败，跳过: %v", n.Name, err)
			continue
		}
		if !certNeedsWarn(cert.NotBefore, cert.NotAfter, now) {
			continue
		}
		host := n.Host
		if host == "" {
			host = n.Address
		}
		a := CertAlert{NodeID: n.ID, NodeName: n.Name, Host: host, NotAfter: cert.NotAfter,
			DaysLeft: int(cert.NotAfter.Sub(now).Hours() / 24)}
		if cert.NotAfter.Before(now) {
			a.DaysLeft = -1
		}
		log.Printf("[cert] %s", a.Message())
		alerts = append(alerts, a)
	}
	certMu.Lock()
	certAlerts = alerts
	certMu.Unlock()
}

// GetCertAlerts 返回最近一次检查发现的告警
func GetCertAlerts() []CertAlert {
	certMu.Lock()
	defer certMu.Unlock()
	return append([]CertAlert{}, certAlerts...)
}

// StartCertCheck 启动后 1 分钟检查一次（等 Xray、mosdns 就绪），之后每 12 小时一次
func StartCertCheck() {
	go func() {
		time.Sleep(time.Minute)
		for {
			CheckNodeCertsNow()
			time.Sleep(certCheckInterval)
		}
	}()
}
