package services

import (
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

//go:embed templates/mosdns-config.yaml.tpl
var mosdnsConfigTpl string

// RenderMosdnsConfig 根据 DNS 上游配置渲染 mosdns config.yaml。
// 写入是原子的（写 .tmp 再 rename）。不重启 mosdns——由调用者决定。
func RenderMosdnsConfig(u config.DNSUpstreams) error {
	b, err := renderMosdnsBytes(u)
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(config.MosdnsConfigPath, b, 0644)
}

func renderMosdnsBytes(u config.DNSUpstreams) ([]byte, error) {
	tpl, err := template.New("mosdns").Funcs(template.FuncMap{"hostIsDomain": upstreamHostIsDomain}).Parse(mosdnsConfigTpl)
	if err != nil {
		return nil, fmt.Errorf("解析 mosdns 模板: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, u); err != nil {
		return nil, fmt.Errorf("渲染 mosdns 模板: %w", err)
	}
	return buf.Bytes(), nil
}

// upstreamHostIsDomain 上游的主机部分是域名（不是 IP）时为 true：这类上游需要 bootstrap 来解析自身的域名
func upstreamHostIsDomain(addr string) bool {
	if !strings.Contains(addr, "://") {
		addr = "udp://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h != "" && net.ParseIP(h) == nil
}

// RestartMosdns 重启 mosdns 并校验 2 秒后仍在运行。
func RestartMosdns() error {
	if err := RunCmdErr(CmdTimeoutLong, "systemctl", "restart", "rproxy-gw-mosdns"); err != nil {
		return fmt.Errorf("systemctl restart 失败: %w", err)
	}
	time.Sleep(2 * time.Second)
	if !IsServiceActive("rproxy-gw-mosdns") {
		out, _ := RunCmdCombined(CmdTimeoutShort, "journalctl", "-u", "rproxy-gw-mosdns", "-n", "20", "--no-pager")
		return fmt.Errorf("mosdns 启动后崩溃，最近日志:\n%s", string(out))
	}
	return nil
}
