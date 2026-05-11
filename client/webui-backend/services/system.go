package services

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// IsServiceActive 检查 systemd 服务是否运行
func IsServiceActive(name string) bool {
	out, _ := exec.Command("systemctl", "is-active", name).Output()
	return strings.TrimSpace(string(out)) == "active"
}

func RestartService(name string) error {
	return exec.Command("systemctl", "restart", name).Run()
}

func StopService(name string) error {
	return exec.Command("systemctl", "stop", name).Run()
}

// HealthData watchdog 写入的健康检查结果
type HealthData struct {
	LastCheck     string `json:"last_check"`
	XrayActive    int    `json:"xray_active"`
	MosdnsActive  int    `json:"mosdns_active"`
	WebUIActive   int    `json:"webui_active"`
	ProxyOK       int    `json:"proxy_ok"`
	CurrentNodeID string `json:"current_node_id"`
	FailCount     int    `json:"fail_count"`
	RestartCount  int    `json:"restart_count"`
	LastAction    string `json:"last_action"`
}

func ReadHealth() (*HealthData, error) {
	b, err := os.ReadFile(config.HealthFile)
	if err != nil {
		return nil, err
	}
	var h HealthData
	if err := json.Unmarshal(b, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// JournalLog 调用 journalctl 拉取最近 N 行
func JournalLog(unit string, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	args := []string{"-u", unit, "--no-pager", "-n", strconv.Itoa(lines)}
	out, err := exec.Command("journalctl", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GetExternalIP 取本机外网 IP（用于 WG 客户端 endpoint）
// 优先返回第一个非环回 IP；找不到则返回空字符串
func GetExternalIP() string {
	out, err := exec.Command("hostname", "-I").Output()
	if err != nil {
		return ""
	}
	ips := strings.Fields(string(out))
	for _, ip := range ips {
		return ip
	}
	return ""
}
