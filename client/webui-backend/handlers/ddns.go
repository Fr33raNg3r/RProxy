package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
)

// secretMask 是返回给前端的密钥占位符；前端原样提交回来时表示"沿用旧密钥"
const secretMask = "******"

// maskDDNSSecrets 返回密钥已打码的副本（空密钥保持为空，方便前端区分"未设置"）
func maskDDNSSecrets(c config.DDNSConfig) config.DDNSConfig {
	out := config.DDNSConfig{Records: make([]config.DDNSRecord, len(c.Records))}
	copy(out.Records, c.Records)
	for i := range out.Records {
		if out.Records[i].Secret != "" {
			out.Records[i].Secret = secretMask
		}
	}
	return out
}

// mergeDDNSSecrets 把提交内容里的占位符还原成旧密钥；找不到旧值的占位符清空
func mergeDDNSSecrets(in, old config.DDNSConfig) config.DDNSConfig {
	oldByID := map[string]string{}
	for _, r := range old.Records {
		oldByID[r.ID] = r.Secret
	}
	out := config.DDNSConfig{Records: make([]config.DDNSRecord, len(in.Records))}
	copy(out.Records, in.Records)
	for i := range out.Records {
		if out.Records[i].Secret == secretMask {
			out.Records[i].Secret = oldByID[out.Records[i].ID]
		}
	}
	return out
}

var ddnsLabels = map[string]string{"cloudflare": "Cloudflare", "dnspod": "DNSPod", "namesilo": "NameSilo"}

// validateDDNSConfig 去掉各字段首尾空格并校验：整张卡片留空 = 不生效，直接通过；
// 填了一部分但不完整则报错，避免用户以为已经生效
func validateDDNSConfig(c *config.DDNSConfig) error {
	for i := range c.Records {
		r := &c.Records[i]
		r.Domain = strings.TrimSpace(r.Domain)
		r.Sub = strings.TrimSpace(r.Sub)
		r.AuthID = strings.TrimSpace(r.AuthID)
		r.Secret = strings.TrimSpace(r.Secret)
		if r.Domain == "" && r.Sub == "" && r.AuthID == "" && r.Secret == "" {
			continue
		}
		label := ddnsLabels[r.Provider]
		switch {
		case r.Domain == "":
			return fmt.Errorf("%s：请填写主域名", label)
		case r.Provider == "dnspod" && r.AuthID == "":
			return fmt.Errorf("%s：请填写 API ID", label)
		case r.Secret == "":
			return fmt.Errorf("%s：请填写密钥", label)
		}
	}
	return nil
}

// GetDDNS GET /api/ddns
func GetDDNS(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadDDNSConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"config": maskDDNSSecrets(cfg),
		"status": services.GetDDNSStatus(),
	})
}

// UpdateDDNS PUT /api/ddns
// 整体替换配置（固定三家，留空的不生效）；保存后异步触发一轮更新
func UpdateDDNS(w http.ResponseWriter, r *http.Request) {
	var raw config.DDNSConfig
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg("请求体格式错误"))
		return
	}
	in := config.NormalizeDDNS(raw)
	old, err := config.LoadDDNSConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg(err.Error()))
		return
	}
	merged := mergeDDNSSecrets(in, old)
	if err := validateDDNSConfig(&merged); err != nil {
		writeJSON(w, http.StatusBadRequest, errorMsg(err.Error()))
		return
	}
	if err := config.SaveDDNSConfig(merged); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorMsg("保存失败: "+err.Error()))
		return
	}
	go services.RunDDNSNow()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "config": maskDDNSSecrets(merged)})
}

// RunDDNS POST /api/ddns/run
// 立即执行一轮并返回最新状态
func RunDDNS(w http.ResponseWriter, r *http.Request) {
	services.RunDDNSNow()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "status": services.GetDDNSStatus()})
}
