package handlers

import (
	"net/http"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
	"github.com/go-chi/chi/v5"
)

// GetLogs GET /api/logs/{component}?lines=200
// mosdns / xray 的日志由自身配置直接写入文件（不打印到 stdout），
// 所以这两类走 TailFile 读文件；其余组件没有专门的日志文件，走 journalctl。
func GetLogs(w http.ResponseWriter, r *http.Request) {
	component := chi.URLParam(r, "component")
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

	var out string
	var err error
	switch component {
	case "mosdns":
		out, err = services.TailFile(config.MosdnsLogPath, lines)
	case "xray-access":
		out, err = services.TailFile(config.XrayAccessLogPath, lines)
	case "xray-error":
		out, err = services.TailFile(config.XrayErrorLogPath, lines)
	case "webui":
		out, err = services.JournalLog("tproxy-gw-webui", lines)
	case "watchdog":
		out, err = services.JournalLog("tproxy-gw-watchdog", lines)
	case "update":
		out, err = services.JournalLog("tproxy-gw-update", lines)
	case "wg":
		out, err = services.JournalLog("wg-quick@wg0", lines)
	default:
		writeJSON(w, http.StatusBadRequest, errorMsg("不支持的组件"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"component": component,
		"content":   out,
	})
}
