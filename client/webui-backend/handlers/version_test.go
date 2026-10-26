package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFetchLatestReleaseErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"限流", http.StatusForbidden, `{"message":"rate limit"}`, "403"},
		{"无合法 tag", http.StatusOK, `[{"tag_name":"v-test"}]`, "未找到"},
		{"JSON 损坏", http.StatusOK, `[{`, "解析"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			v, err := fetchLatestRelease(srv.URL)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("期望包含 %q 的错误，得到 v=%q err=%v", c.wantErr, v, err)
			}
		})
	}
}

func TestFetchLatestReleaseOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v-test"},{"tag_name":"v1.8.3"},{"tag_name":"v1.8.2"}]`))
	}))
	defer srv.Close()
	v, err := fetchLatestRelease(srv.URL)
	if err != nil || v != "1.8.3" {
		t.Fatalf("得到 v=%q err=%v", v, err)
	}
}

func TestGetVersionReportsCheckError(t *testing.T) {
	versionMu.Lock()
	cachedCurrentVer, cachedLatestVer, cachedCheckErr = "1.8.2", "", "连接超时"
	versionMu.Unlock()
	defer func() {
		versionMu.Lock()
		cachedCurrentVer, cachedLatestVer, cachedCheckErr = "", "", ""
		versionMu.Unlock()
	}()

	rec := httptest.NewRecorder()
	GetVersion(rec, httptest.NewRequest("GET", "/api/version", nil))
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["check_error"] != "连接超时" {
		t.Fatalf("check_error = %v", resp["check_error"])
	}
}

// 一键升级的外层脚本：无论下载 install.sh 失败，还是 install.sh 没写标志就失败退出，
// 日志里都必须出现"升级完成（"，否则前端一直显示"升级中"；install.sh 自己写过标志时不重复写
func TestUpgradeShellScriptAlwaysWritesMarker(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("没有 bash")
	}
	cases := []struct {
		name, fakeInstall string
		downloadOK        bool
		wantMarkers       int
		wantText          string
	}{
		{"下载失败", "", false, 1, "下载升级脚本失败"},
		{"打快照前失败", "echo 预编译产物下载失败; exit 1", true, 1, "升级失败，退出码 1"},
		{"已自动回滚", "echo '" + upgradeDoneMarker + "结果：已自动回滚）'; exit 1", true, 1, "已自动回滚"},
		{"成功", "echo '" + upgradeDoneMarker + "1.8.3 → 1.8.4）'", true, 1, "1.8.4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.ToSlash(t.TempDir())
			os.MkdirAll(dir+"/opt/logs", 0755)
			src := dir + "/fake_install.sh"
			os.WriteFile(src, []byte("#!/bin/bash\n"+c.fakeInstall+"\n"), 0755)
			s := strings.ReplaceAll(upgradeShellScript("v1.8.7"), "/opt/rproxy-gw", dir+"/opt")
			fetch := "false"
			if c.downloadOK {
				fetch = "cp " + src + " " + dir + "/opt/install.sh.new"
			}
			s = regexp.MustCompile(`curl -fsSL --max-time 60 \S+ -o \S+`).ReplaceAllString(s, fetch)
			_ = exec.Command("bash", "-c", s).Run()
			b, _ := os.ReadFile(dir + "/opt/logs/upgrade.log")
			if n := strings.Count(string(b), upgradeDoneMarker); n != c.wantMarkers || !strings.Contains(string(b), c.wantText) {
				t.Fatalf("完成标志 %d 处（应为 %d），日志：\n%s", n, c.wantMarkers, b)
			}
		})
	}
}

// 一键升级按最新 Release 的 tag 取 install.sh 并把 tag 传给它：
// 不取 main 分支的脚本，避免用 main 上尚未发布的脚本去装旧版本的产物
func TestUpgradeShellScriptUsesReleaseTag(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("没有 bash")
	}
	s := upgradeShellScript("v1.8.7")
	if !strings.Contains(s, "https://raw.githubusercontent.com/Fr33raNg3r/RProxy/v1.8.7/client/install.sh") {
		t.Fatalf("应按 tag 下载 install.sh，脚本：\n%s", s)
	}
	dir := filepath.ToSlash(t.TempDir())
	os.MkdirAll(dir+"/opt/logs", 0755)
	src := dir + "/fake_install.sh"
	os.WriteFile(src, []byte("#!/bin/bash\necho \"参数：$*\"\n"), 0755)
	s = strings.ReplaceAll(s, "/opt/rproxy-gw", dir+"/opt")
	s = regexp.MustCompile(`curl -fsSL --max-time 60 \S+ -o \S+`).ReplaceAllString(s, "cp "+src+" "+dir+"/opt/install.sh.new")
	_ = exec.Command("bash", "-c", s).Run()
	b, _ := os.ReadFile(dir + "/opt/logs/upgrade.log")
	if !strings.Contains(string(b), "参数：upgrade v1.8.7") {
		t.Fatalf("install.sh 应收到 upgrade v1.8.7，日志：\n%s", b)
	}
}
