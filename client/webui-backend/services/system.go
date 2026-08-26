package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// UpdateTimerFile update timer 的 systemd 单元路径
const UpdateTimerFile = "/etc/systemd/system/tproxy-gw-update.timer"

// WriteUpdateTimer 按指定时分重写 update timer 的 OnCalendar 并使其生效。
// OnCalendar 固定带 Asia/Shanghai 时区后缀：不管系统时区是否为 UTC，
// 每日更新都按北京时间触发。供 WebUI 设置保存与升级时重建 timer 共用。
func WriteUpdateTimer(hour, minute int) error {
	content := fmt.Sprintf(`[Unit]
Description=RProxy Daily Update Timer (%02d:%02d daily, Asia/Shanghai)

[Timer]
# 固定北京时间，避免系统时区为 UTC 时偏移 8 小时
OnCalendar=*-*-* %02d:%02d:00 Asia/Shanghai
RandomizedDelaySec=30m
Persistent=true
Unit=tproxy-gw-update.service

[Install]
WantedBy=timers.target
`, hour, minute, hour, minute)
	if err := os.WriteFile(UpdateTimerFile, []byte(content), 0644); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return err
	}
	return exec.Command("systemctl", "restart", "tproxy-gw-update.timer").Run()
}

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
	// --reverse：最新的日志排在最前面，WebUI 无需手动拖到底部
	args := []string{"-u", unit, "--no-pager", "--reverse", "-n", strconv.Itoa(lines)}
	out, err := exec.Command("journalctl", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// TailFile 读取日志文件最后 N 行，最新的排在最前面（与 JournalLog 行为一致）。
// 文件不存在时不算错误，返回提示文案（mosdns.log / xray 的 access.log、error.log
// 首次启动或刚被 logrotate 轮转后可能暂时不存在）。
func TailFile(path string, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "（日志文件不存在，可能尚未产生任何记录）", nil
		}
		return "", err
	}
	out, err := exec.Command("tail", "-n", strconv.Itoa(lines), path).Output()
	if err != nil {
		return "", err
	}
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return "", nil
	}
	rows := strings.Split(text, "\n")
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return strings.Join(rows, "\n"), nil
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
