package services

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

//go:embed templates/mosdns-config.yaml.tpl
var mosdnsConfigTpl string

// RenderMosdnsConfig 根据 DNS 上游配置渲染 mosdns config.yaml。
// 写入是原子的（写 .tmp 再 rename）。不重启 mosdns——由调用者决定。
func RenderMosdnsConfig(u config.DNSUpstreams) error {
	tpl, err := template.New("mosdns").Parse(mosdnsConfigTpl)
	if err != nil {
		return fmt.Errorf("解析 mosdns 模板: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, u); err != nil {
		return fmt.Errorf("渲染 mosdns 模板: %w", err)
	}
	return config.WriteFileAtomic(config.MosdnsConfigPath, buf.Bytes(), 0644)
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
