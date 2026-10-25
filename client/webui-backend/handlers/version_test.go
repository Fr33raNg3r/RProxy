package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
