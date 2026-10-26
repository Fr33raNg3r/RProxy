package handlers

import (
	"testing"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/middleware"
)

// 改密码后换掉会话密钥：之前签发的 cookie（含"记住我"的 30 天会话）全部失效，
// 当前浏览器拿到用新密钥签发、到期时间不变的新 cookie，不用重新登录
func TestRotateSessionInvalidatesOldTokens(t *testing.T) {
	cfg := &config.WebUIConfig{SessionSecret: "old-secret"}
	exp := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	oldToken := middleware.MakeSessionToken("old-secret", exp)

	newToken, newExp, err := rotateSession(cfg, oldToken)
	if err != nil {
		t.Fatalf("rotateSession 报错: %v", err)
	}
	if cfg.SessionSecret == "old-secret" || len(cfg.SessionSecret) < 32 {
		t.Fatalf("会话密钥应换成新的随机值，实际 %q", cfg.SessionSecret)
	}
	if middleware.VerifySessionToken(cfg.SessionSecret, oldToken) {
		t.Fatal("旧 cookie 在换密钥后应失效")
	}
	if !middleware.VerifySessionToken(cfg.SessionSecret, newToken) {
		t.Fatal("新 cookie 应能用新密钥校验通过")
	}
	if !newExp.Equal(exp) {
		t.Fatalf("新 cookie 应保持原到期时间 %v，实际 %v", exp, newExp)
	}
}
