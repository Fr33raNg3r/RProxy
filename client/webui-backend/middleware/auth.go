package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

const (
	CookieName      = "rproxy_session"
	SessionLifetime = 24 * time.Hour
	// RememberSessionLifetime 登录时勾选"记住我"使用的更长会话有效期
	RememberSessionLifetime = 30 * 24 * time.Hour
)

type ctxKey string

const userKey ctxKey = "user"

// MakeSessionToken 生成 cookie 值: <expiry_unix>.<hmac>
func MakeSessionToken(secret string, expiry time.Time) string {
	exp := strconv.FormatInt(expiry.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(exp))
	return exp + "." + hex.EncodeToString(mac.Sum(nil))
}

// VerifySessionToken 校验 cookie，返回是否有效
func VerifySessionToken(secret, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > exp {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// RequireAuth 包装需要登录的路由
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg, err := config.LoadWebUIConfig()
		if err != nil {
			writeAuthError(w, http.StatusServiceUnavailable, "service unavailable", "")
			return
		}
		c, err := r.Cookie(CookieName)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
		if !VerifySessionToken(cfg.SessionSecret, c.Value) {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
		// 首次未改密：只放行登出和改密接口
		if !cfg.PasswordChanged && !isPasswordChangeBypass(r.URL.Path) {
			writeAuthError(w, http.StatusForbidden, "must change password first", "password_change_required")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, cfg.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeAuthError 以 JSON 返回鉴权错误：前端只在 application/json 响应里识别 code（如强制改密时跳回登录页）
func writeAuthError(w http.ResponseWriter, status int, msg, code string) {
	body := map[string]string{"error": msg}
	if code != "" {
		body["code"] = code
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func isPasswordChangeBypass(path string) bool {
	return path == "/api/settings/password" || path == "/api/logout"
}
