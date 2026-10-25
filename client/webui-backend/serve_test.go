package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeStatic() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("console.log('rproxy');", 200)))
	})
}

// 带哈希文件名的 /assets/* 可长期缓存；index.html 等其他路径每次协商，升级后能立即拿到新页面
func TestStaticHandlerCacheHeaders(t *testing.T) {
	cases := map[string]string{
		"/assets/index-4yGYQLxs.js": "public, max-age=31536000, immutable",
		"/":                         "no-cache",
		"/index.html":               "no-cache",
		"/status":                   "no-cache",
	}
	h := staticHandler(fakeStatic())
	for path, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control 期望 %q，实际 %q", path, want, got)
		}
	}
}

func TestStaticHandlerGzip(t *testing.T) {
	req := httptest.NewRequest("GET", "/assets/index-4yGYQLxs.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	staticHandler(fakeStatic()).ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("浏览器支持 gzip 时 JS 应压缩返回，实际 Content-Encoding=%q", got)
	}
}
