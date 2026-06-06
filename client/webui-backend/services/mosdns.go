package services

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	if err := os.MkdirAll(filepath.Dir(config.MosdnsConfigPath), 0755); err != nil {
		return err
	}

	tmp := config.MosdnsConfigPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if err := tpl.Execute(f, u); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("渲染 mosdns 模板: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, config.MosdnsConfigPath)
}

// RestartMosdns 重启 mosdns 并校验 2 秒后仍在运行。
func RestartMosdns() error {
	if err := exec.Command("systemctl", "restart", "tproxy-gw-mosdns").Run(); err != nil {
		return fmt.Errorf("systemctl restart 失败: %w", err)
	}
	time.Sleep(2 * time.Second)
	if !IsServiceActive("tproxy-gw-mosdns") {
		out, _ := exec.Command("journalctl", "-u", "tproxy-gw-mosdns", "-n", "20", "--no-pager").CombinedOutput()
		return fmt.Errorf("mosdns 启动后崩溃，最近日志:\n%s", string(out))
	}
	return nil
}
