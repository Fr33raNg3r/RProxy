package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeWebUIFile(t *testing.T, path, user string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`{"username":"`+user+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// 文件 mtime 与大小都没变时直接用缓存，不再读盘解析
func TestLoadWebUICachedSkipsUnchangedFile(t *testing.T) {
	resetWebUICache()
	path := filepath.Join(t.TempDir(), "webui.json")
	mt := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeWebUIFile(t, path, "aaa", mt)
	if c, err := loadWebUICached(path); err != nil || c.Username != "aaa" {
		t.Fatalf("首次读取应得到 aaa，实际 %v %v", c, err)
	}
	// 内容变了但 mtime、大小都相同 → 说明命中缓存
	writeWebUIFile(t, path, "bbb", mt)
	if c, _ := loadWebUICached(path); c.Username != "aaa" {
		t.Fatalf("mtime/大小未变应命中缓存，实际读到 %q", c.Username)
	}
}

// 其他进程（如 watchdog 调用的 webui 子命令）改了文件，mtime 变化后要重新读取
func TestLoadWebUICachedReloadsChangedFile(t *testing.T) {
	resetWebUICache()
	path := filepath.Join(t.TempDir(), "webui.json")
	mt := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeWebUIFile(t, path, "aaa", mt)
	_, _ = loadWebUICached(path)
	writeWebUIFile(t, path, "ccc", mt.Add(time.Second))
	if c, _ := loadWebUICached(path); c.Username != "ccc" {
		t.Fatalf("文件变化后应重新读取，实际 %q", c.Username)
	}
}

// 调用方会修改返回值再保存，缓存必须每次返回独立副本
func TestLoadWebUICachedReturnsCopy(t *testing.T) {
	resetWebUICache()
	path := filepath.Join(t.TempDir(), "webui.json")
	writeWebUIFile(t, path, "aaa", time.Now().Add(-time.Hour))
	c1, _ := loadWebUICached(path)
	c1.Username = "changed"
	if c2, _ := loadWebUICached(path); c2.Username != "aaa" {
		t.Fatalf("修改返回值不应影响缓存，实际 %q", c2.Username)
	}
}
