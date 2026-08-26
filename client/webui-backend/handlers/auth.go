package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/middleware"

	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// ---------- 登录失败限流（按来源 IP，进程内存，重启后重置） ----------
const (
	loginMaxFails    = 5                // 连续失败达到这个次数开始锁定
	loginBaseBackoff = 10 * time.Second // 第一次锁定时长，之后每次翻倍
	loginMaxBackoff  = 10 * time.Minute // 锁定时长上限
)

type loginAttemptState struct {
	fails       int
	lockedUntil time.Time
}

var (
	loginAttemptsMu sync.Mutex
	loginAttempts   = map[string]*loginAttemptState{}
)

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// checkLoginLock 返回是否仍在锁定中，及剩余时长
func checkLoginLock(ip string) (bool, time.Duration) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	s := loginAttempts[ip]
	if s == nil || s.lockedUntil.IsZero() || !time.Now().Before(s.lockedUntil) {
		return false, 0
	}
	return true, time.Until(s.lockedUntil)
}

func recordLoginFail(ip string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	s := loginAttempts[ip]
	if s == nil {
		s = &loginAttemptState{}
		loginAttempts[ip] = s
	}
	s.fails++
	if s.fails >= loginMaxFails {
		backoff := loginBaseBackoff * time.Duration(1<<uint(s.fails-loginMaxFails))
		if backoff > loginMaxBackoff {
			backoff = loginMaxBackoff
		}
		s.lockedUntil = time.Now().Add(backoff)
	}
}

func recordLoginSuccess(ip string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	delete(loginAttempts, ip)
}

func Login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if locked, remaining := checkLoginLock(ip); locked {
		writeJSON(w, http.StatusTooManyRequests, errorMsg(fmt.Sprintf("失败次数过多，请 %d 秒后重试", int(remaining.Seconds())+1)))
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("invalid body"))
		return
	}
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("server error"))
		return
	}
	if req.Username != cfg.Username {
		recordLoginFail(ip)
		writeJSON(w, http.StatusUnauthorized, errorMsg("用户名或密码错误"))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(req.Password)); err != nil {
		recordLoginFail(ip)
		writeJSON(w, http.StatusUnauthorized, errorMsg("用户名或密码错误"))
		return
	}
	recordLoginSuccess(ip)

	lifetime := middleware.SessionLifetime
	if req.Remember {
		lifetime = middleware.RememberSessionLifetime
	}
	exp := time.Now().Add(lifetime)
	token := middleware.MakeSessionToken(cfg.SessionSecret, exp)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Expires:  exp,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                   true,
		"must_change_password": !cfg.PasswordChanged,
	})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// 修改密码
type changePassRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePassRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("invalid body"))
		return
	}
	if len(req.NewPassword) < 6 {
		writeJSON(w, http.StatusBadRequest, errorMsg("新密码至少 6 位"))
		return
	}
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("server error"))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(req.OldPassword)); err != nil {
		writeJSON(w, http.StatusUnauthorized, errorMsg("旧密码错误"))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("加密失败"))
		return
	}
	cfg.PasswordHash = string(hash)
	cfg.PasswordChanged = true
	if err := config.SaveWebUIConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("保存失败"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
