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
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func runServe() {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		log.Fatalf("无法加载 webui.json: %v", err)
	}

	// 规范化节点 Enabled 字段：保证只有 CurrentNodeID 对应的节点 enabled=true
	if err := normalizeNodesEnabled(cfg); err != nil {
		log.Printf("规范化节点状态失败（继续启动）: %v", err)
	}

	// 启动版本检查（本地立即读，远程异步拉取）
	handlers.InitVersionCheck()

	// 启动 DDNS 后台循环（配置未启用时每轮直接返回）
	services.StartDDNS()

	// 节点证书到期检查（每 12 小时）
	services.StartCertCheck()

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
		r.Get("/api/nodes/cert-alerts", handlers.GetCertAlerts)

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
		r.Get("/api/dns/upstreams", handlers.GetDNSUpstreams)
		r.Get("/api/dns/upstreams/defaults", handlers.GetDNSUpstreamsDefaults)
		r.Put("/api/dns/upstreams", handlers.UpdateDNSUpstreams)

		// DDNS
		r.Get("/api/ddns", handlers.GetDDNS)
		r.Put("/api/ddns", handlers.UpdateDDNS)
		r.Post("/api/ddns/run", handlers.RunDDNS)

		// 设置
		r.Get("/api/settings", handlers.GetSettings)
		r.Put("/api/settings", handlers.UpdateSettings)
		r.Post("/api/settings/password", handlers.ChangePassword)
		r.Post("/api/system/reboot", handlers.RebootSystem)
		r.Post("/api/emergency-stop", handlers.EmergencyStop)
		r.Post("/api/emergency-resume", handlers.EmergencyResume)

		// 配置导入/导出
		r.Get("/api/config/export", handlers.ExportConfig)
		r.Post("/api/config/import", handlers.ImportConfig)
		r.Post("/api/services/{name}/restart", restartComponentByPath)

		// 日志
		r.Get("/api/logs/{component}", handlers.GetLogs)

		// 版本与升级
		r.Get("/api/version", handlers.GetVersion)
		r.Post("/api/version/refresh", handlers.RefreshLatest)
		r.Post("/api/upgrade", handlers.TriggerUpgrade)
		r.Get("/api/upgrade/log", handlers.GetUpgradeLog)

		// Xray 自动更新兼容性告警
		r.Get("/api/xray/alert", handlers.GetXrayAlert)
		r.Post("/api/xray/alert/dismiss", handlers.DismissXrayAlert)
	})

	// ---------- 静态文件（Vue 构建产物） ----------
	r.Get("/*", staticHandler(http.HandlerFunc(spaHandler)).ServeHTTP)

	// 同时在 443 提供 HTTPS（自签名证书），这样 http / https 访问都能正常登录；
	// HTTPS 起不来不影响主端口
	if cfg.ListenPort != httpsPort {
		go serveHTTPS(r)
	}

	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	log.Printf("WebUI 监听 %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

const httpsPort = 443

// serveHTTPS 在 443 端口用自签名证书提供同一套路由
func serveHTTPS(h http.Handler) {
	certPath := filepath.Join(config.InstallDir, "config", "tls.crt")
	keyPath := filepath.Join(config.InstallDir, "config", "tls.key")
	if err := ensureSelfSignedCert(certPath, keyPath); err != nil {
		log.Printf("HTTPS 证书准备失败（仅提供 HTTP）: %v", err)
		return
	}
	addr := fmt.Sprintf(":%d", httpsPort)
	log.Printf("WebUI(HTTPS) 监听 %s", addr)
	if err := http.ListenAndServeTLS(addr, certPath, keyPath, h); err != nil {
		log.Printf("HTTPS 监听失败（仅提供 HTTP）: %v", err)
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

// staticHandler 给静态文件加缓存头和 gzip（只用于静态路由，不影响 API / SSE）：
// /assets/* 是 Vite 构建的带哈希文件名，内容变了文件名就变，可长期缓存；
// index.html 等其他路径用 no-cache，每次向服务端协商，升级后浏览器立即拿到新页面
func staticHandler(next http.Handler) http.Handler {
	compressed := chimw.Compress(5)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compressed.ServeHTTP(&cacheHeaderWriter{ResponseWriter: w, asset: strings.HasPrefix(r.URL.Path, "/assets/")}, r)
	})
}

// cacheHeaderWriter 在写状态码时才决定缓存头：只有成功返回的 /assets/ 文件才长期缓存，
// 404 等错误响应一律 no-cache，避免把错误结果缓存一年
type cacheHeaderWriter struct {
	http.ResponseWriter
	asset bool
	wrote bool
}

func (c *cacheHeaderWriter) WriteHeader(code int) {
	if !c.wrote {
		c.wrote = true
		if c.asset && (code == http.StatusOK || code == http.StatusNotModified) {
			c.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header().Set("Cache-Control", "no-cache")
		}
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *cacheHeaderWriter) Write(b []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(b)
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
	if (err != nil || stat.IsDir()) && strings.HasPrefix(path, "assets/") {
		// 带哈希的构建产物不存在（多为升级后旧页面请求旧文件）：返回 404，不回退成 index.html
		http.NotFound(w, r)
		return
	}
	if err != nil || stat.IsDir() {
		// 不是文件或不存在，返回 index.html
		http.ServeFile(w, r, filepath.Join(config.WWWDir, "index.html"))
		return
	}
	http.ServeFile(w, r, full)
}

// normalizeNodesEnabled 启动时规范化节点 Enabled 字段：
//   - 如果 CurrentNodeID 非空：只让该节点 enabled=true，其他 false
//   - 如果 CurrentNodeID 为空但存在节点：选 order 最小的设为 current 并 enabled=true
//   - 仅在有变化时写回，避免无谓 IO
func normalizeNodesEnabled(cfg *config.WebUIConfig) error {
	nodes, err := config.LoadNodes()
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}

	target := cfg.CurrentNodeID
	if target == "" {
		minIdx := 0
		for i := range nodes {
			if nodes[i].Order < nodes[minIdx].Order {
				minIdx = i
			}
		}
		target = nodes[minIdx].ID
	}

	changed := false
	hasTarget := false
	for i := range nodes {
		want := nodes[i].ID == target
		if want {
			hasTarget = true
		}
		if nodes[i].Enabled != want {
			nodes[i].Enabled = want
			changed = true
		}
	}
	// CurrentNodeID 指向了已删除节点 → 回退到 order 最小
	if !hasTarget {
		minIdx := 0
		for i := range nodes {
			if nodes[i].Order < nodes[minIdx].Order {
				minIdx = i
			}
		}
		target = nodes[minIdx].ID
		for i := range nodes {
			nodes[i].Enabled = nodes[i].ID == target
		}
		changed = true
	}

	if cfg.CurrentNodeID != target {
		cfg.CurrentNodeID = target
		if err := config.SaveWebUIConfig(cfg); err != nil {
			return err
		}
	}
	if changed {
		if err := config.SaveNodes(nodes); err != nil {
			return err
		}
	}
	return nil
}
