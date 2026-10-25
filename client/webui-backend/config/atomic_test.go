package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// 多个进程/协程同时写同一个文件时，各自的临时文件不能互相踩，
// 每次写入都应成功，最终文件是某一次完整写入的内容，且不残留临时文件
func TestWriteJSONAtomicConcurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows 上多个进程同时 rename 到同一目标会 Access denied（系统限制）；
		// 生产环境是 Linux，rename(2) 原子替换，此测试只在 Linux 上有意义
		t.Skip("仅在 Linux 上验证并发原子写")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.json")

	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := writeJSONAtomic(path, map[string]int{"g": g, "i": i}, 0600); err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发写入失败: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]int
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("最终文件不是完整 JSON: %v\n%s", err, b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("目录里应只剩目标文件，实际: %v", names)
	}
}

func TestWriteFileAtomicReplacesContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "a.conf")
	if err := WriteFileAtomic(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "new" {
		t.Fatalf("内容应为 new，实际 %q", b)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("不应残留临时文件，目录内有 %d 个文件", len(entries))
	}
}
