package handlers

import (
	"net/http"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
	"github.com/go-chi/chi/v5"
)

// GetLogs GET /api/logs/{component}?lines=200
func GetLogs(w http.ResponseWriter, r *http.Request) {
	component := chi.URLParam(r, "component")
	allowed := map[string]string{
		"xray":     "xray",
		"mosdns":   "tproxy-gw-mosdns",
		"webui":    "tproxy-gw-webui",
		"watchdog": "tproxy-gw-watchdog",
		"update":   "tproxy-gw-update",
		"wg":       "wg-quick@wg0",
	}
	unit, ok := allowed[component]
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorMsg("不支持的组件"))
		return
	}
	lines := 200
	if l := r.URL.Query().Get("lines"); l != "" {
		// 简单解析
		n := 0
		for i := 0; i < len(l); i++ {
			c := l[i]
			if c < '0' || c > '9' {
				n = 0
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 && n <= 2000 {
			lines = n
		}
	}
	out, err := services.JournalLog(unit, lines)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"component": component,
		"unit":      unit,
		"content":   out,
	})
}
