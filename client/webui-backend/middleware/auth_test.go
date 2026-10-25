package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 前端只在 JSON 响应里识别 code=password_change_required 并跳回登录页，
// http.Error 返回的是 text/plain，会被当成普通错误把原始 JSON 文本显示出来
func TestWriteAuthErrorIsJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAuthError(rec, http.StatusForbidden, "must change password first", "password_change_required")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type 应为 application/json，实际 %q", ct)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("状态码 %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if body["code"] != "password_change_required" || body["error"] == "" {
		t.Fatalf("响应内容 %v", body)
	}
}
