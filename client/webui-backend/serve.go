package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/handlers"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/middleware"

	"github.com/go-chi/chi/v5"
)

func runServe() {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		log.Fatalf("无法加载 webui.json: %v", err)
	}

	// 启动版本检查（本地立即读，远程异步拉取）
	handlers.InitVersionCheck()

	r := chi.NewRouter()

	// 简单的请求日志
	r.Use(loggingMiddleware)

	// ---------- 公开路由 ----------
	r.Post("/api/login", handlers.Login)
	r.Post("/api/logout", handlers.Logout)

	// ---------- 受保护 API ----------
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		r.Get("/api/status", handlers.GetStatus)
		r.Get("/api/status/stream", handlers.StreamStatus)

		// 节点
		r.Get("/api/nodes", handlers.ListNodes)
		r.Post("/api/nodes", handlers.CreateNode)
		r.Put("/api/nodes/{id}", handlers.UpdateNode)
		r.Delete("/api/nodes/{id}", handlers.DeleteNode)
		r.Post("/api/nodes/{id}/switch", handlers.SwitchNode)
		r.Post("/api/nodes/{id}/test", handlers.TestNode)
		r.Post("/api/nodes/reorder", handlers.ReorderNodes)

		// WireGuard
		r.Get("/api/wireguard/peers", handlers.ListPeers)
		r.Post("/api/wireguard/peers", handlers.CreatePeer)
		r.Delete("/api/wireguard/peers/{id}", handlers.DeletePeer)
		r.Get("/api/wireguard/peers/{id}/qrcode", handlers.PeerQRCode)
		r.Get("/api/wireguard/peers/{id}/config", handlers.PeerConfig)
		r.Get("/api/wireguard/speed/stream", handlers.StreamPeerSpeed)
		r.Post("/api/wireguard/enable", handlers.EnableWireGuard)
		r.Post("/api/wireguard/disable", handlers.DisableWireGuard)
		r.Post("/api/wireguard/endpoint", handlers.SetEndpoint)

		// DNS
		r.Get("/api/dns/rules", handlers.GetDNSRules)
		r.Put("/api/dns/rules", handlers.UpdateDNSRules)

		// 设置
		r.Get("/api/settings", handlers.GetSettings)
		r.Put("/api/settings", handlers.UpdateSettings)
		r.Post("/api/settings/password", handlers.ChangePassword)
		r.Post("/api/emergency-stop", handlers.EmergencyStop)
		r.Post("/api/services/{name}/restart", restartComponentByPath)

		// 日志
		r.Get("/api/logs/{component}", handlers.GetLogs)

		// 版本与升级
		r.Get("/api/version", handlers.GetVersion)
		r.Post("/api/upgrade", handlers.TriggerUpgrade)
	})

	// ---------- 静态文件（Vue 构建产物） ----------
	r.Get("/*", spaHandler)

	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	log.Printf("WebUI 监听 %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// 把 /api/services/{name}/restart 的 chi.URLParam 包成 r.PathValue
// （settings.go 里那个 handler 用 chi.URLParam 即可，这里包一层方便复用）
func restartComponentByPath(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	r2 := r.Clone(r.Context())
	q := r2.URL.Query()
	q.Set("name", name)
	r2.URL.RawQuery = q.Encode()
	handlers.RestartComponent(w, r2)
}

// spaHandler: 返回静态文件，如果文件不存在则返回 index.html（SPA 路由）
func spaHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	full := filepath.Join(config.WWWDir, path)

	// 防止 path traversal
	abs, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(abs, config.WWWDir) {
		http.NotFound(w, r)
		return
	}

	stat, err := os.Stat(full)
	if err != nil || stat.IsDir() {
		// 不是文件或不存在，返回 index.html
		http.ServeFile(w, r, filepath.Join(config.WWWDir, "index.html"))
		return
	}
	http.ServeFile(w, r, full)
}
