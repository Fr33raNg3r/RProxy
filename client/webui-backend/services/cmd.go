package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// 外部命令超时：避免 systemctl / nft / wg 等卡住时 HTTP 请求或 watchdog 调用的子命令一直挂着
const (
	CmdTimeoutShort = 10 * time.Second // 查询类：is-active、nft list、wg show、journalctl 等
	CmdTimeoutLong  = 60 * time.Second // 变更类：systemctl restart/start/stop、脚本、xray -test
)

// RunCmd 执行外部命令并返回 stdout，超过 timeout 杀掉进程并返回超时错误
func RunCmd(timeout time.Duration, name string, args ...string) ([]byte, error) {
	return runWithTimeout(timeout, false, name, args...)
}

// RunCmdCombined 同 RunCmd，但返回 stdout + stderr（用于把错误输出展示给用户）
func RunCmdCombined(timeout time.Duration, name string, args ...string) ([]byte, error) {
	return runWithTimeout(timeout, true, name, args...)
}

// RunCmdErr 同 RunCmd，只关心是否执行成功
func RunCmdErr(timeout time.Duration, name string, args ...string) error {
	_, err := RunCmd(timeout, name, args...)
	return err
}

func runWithTimeout(timeout time.Duration, combined bool, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// 子进程被杀后，若孙进程还占着输出管道，最多再等 1 秒就强制返回
	cmd.WaitDelay = time.Second
	var out []byte
	var err error
	if combined {
		out, err = cmd.CombinedOutput()
	} else {
		out, err = cmd.Output()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("%s 执行超时（%v）", name, timeout)
	}
	return out, err
}
