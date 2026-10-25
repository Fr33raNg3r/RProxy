package services

import (
	"os"
	"testing"
	"time"
)

// 测试进程自身充当"卡住的外部命令"：带上环境变量重新执行测试二进制即可进入这里
func TestMain(m *testing.M) {
	if os.Getenv("RPROXY_TEST_HANG") == "1" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRunCmdTimeoutKillsHungCommand(t *testing.T) {
	t.Setenv("RPROXY_TEST_HANG", "1")
	start := time.Now()
	_, err := RunCmd(300*time.Millisecond, os.Args[0])
	if err == nil {
		t.Fatal("超时的命令应返回错误")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("超时后应尽快返回，实际耗时 %v", d)
	}
}

func TestRunCmdReturnsOutput(t *testing.T) {
	out, err := RunCmd(10*time.Second, os.Args[0], "-test.run=^$")
	if err != nil {
		t.Fatalf("正常命令不应失败: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("应返回命令输出")
	}
}
