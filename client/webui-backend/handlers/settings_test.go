package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestXrayAutoUpdateToggle(t *testing.T) {
	old := noXrayAutoUpdatePath
	defer func() { noXrayAutoUpdatePath = old }()
	noXrayAutoUpdatePath = filepath.Join(t.TempDir(), "state", "no-xray-auto-update")

	if !xrayAutoUpdateEnabled() {
		t.Fatal("标记文件不存在时应默认开启")
	}
	if err := setXrayAutoUpdate(false); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if _, err := os.Stat(noXrayAutoUpdatePath); err != nil {
		t.Fatalf("关闭后应创建标记文件: %v", err)
	}
	if xrayAutoUpdateEnabled() {
		t.Fatal("标记文件存在时应为关闭")
	}
	if err := setXrayAutoUpdate(true); err != nil {
		t.Fatalf("开启失败: %v", err)
	}
	if !xrayAutoUpdateEnabled() {
		t.Fatal("开启后应删除标记文件")
	}
	// 重复开启（文件已不存在）不应报错
	if err := setXrayAutoUpdate(true); err != nil {
		t.Fatalf("重复开启不应报错: %v", err)
	}
}
