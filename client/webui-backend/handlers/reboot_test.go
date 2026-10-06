package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRebootSystemRespondsThenReboots(t *testing.T) {
	oldFn, oldDelay := rebootFn, rebootDelay
	defer func() { rebootFn, rebootDelay = oldFn, oldDelay }()

	called := make(chan struct{}, 1)
	rebootFn = func() error { called <- struct{}{}; return nil }
	rebootDelay = 10 * time.Millisecond

	w := httptest.NewRecorder()
	RebootSystem(w, httptest.NewRequest(http.MethodPost, "/api/system/reboot", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("应先返回 200，实际 %d", w.Code)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("响应之后应触发重启")
	}
}
