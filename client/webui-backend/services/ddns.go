package services

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 查 WAN IP 的接口（全部是国内服务，命中 cn_ips 规则直连，拿到的才是家宽出口 IP）
var wanIPURLs = []string{
	"https://ddns.oray.com/checkip",
	"https://ip.3322.net",
	"https://v4.yinghualuo.cn/bejson",
	"https://myip.ipip.net",
}

// 各厂商 API 根地址（变量形式，测试时指向 httptest 服务）
var (
	cloudflareBase = "https://api.cloudflare.com/client/v4"
	dnspodBase     = "https://dnsapi.cn"
	namesiloBase   = "https://www.namesilo.com/api"
)

const (
	ddnsInterval = 5 * time.Minute
	ddnsTTL      = 600  // Cloudflare / DNSPod
	namesiloTTL  = 3600 // NameSilo 允许的最小值
)

var ddnsHTTP = &http.Client{Timeout: 10 * time.Second}

var ipv4Re = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`)

// extractIPv4 从文本里取第一个合法的 IPv4（每段 0-255、无前导零），没有则返回空串
func extractIPv4(s string) string {
	for _, m := range ipv4Re.FindAllStringSubmatch(s, -1) {
		ok := true
		for _, p := range m[1:] {
			n := 0
			for _, c := range p {
				n = n*10 + int(c-'0')
			}
			if n > 255 || (len(p) > 1 && p[0] == '0') {
				ok = false
				break
			}
		}
		if ok {
			return m[0]
		}
	}
	return ""
}

// fetchWANIP 依次请求各接口，返回第一个取到的 IPv4
func fetchWANIP(urls []string) (string, error) {
	var lastErr error
	for _, u := range urls {
		resp, err := ddnsHTTP.Get(u)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s 返回 %d", u, resp.StatusCode)
			continue
		}
		if ip := extractIPv4(string(body)); ip != "" {
			return ip, nil
		}
		lastErr = fmt.Errorf("%s 返回内容里没有 IPv4", u)
	}
	return "", fmt.Errorf("获取 WAN IP 失败: %v", lastErr)
}

// syncRecords 对每条填写完整的记录：IP 没变就跳过，变了就调用 upsert。
// last 保存"记录 ID -> 上次成功写入的指纹"，指纹包含厂商/域名/IP，
// 所以改了记录的域名或厂商也会重新写入。
// 返回本轮实际尝试过的记录的结果（nil 表示成功），跳过的记录不在其中。
func syncRecords(recs []config.DDNSRecord, ip string, last map[string]string,
	upsert func(config.DDNSRecord, string) error) map[string]error {
	res := map[string]error{}
	for _, r := range recs {
		if !r.Complete() {
			continue
		}
		fp := strings.Join([]string{r.Provider, r.Domain, r.Sub, ip}, "|")
		if last[r.ID] == fp {
			continue
		}
		err := upsert(r, ip)
		res[r.ID] = err
		if err == nil {
			last[r.ID] = fp
		}
	}
	return res
}

// ---------- 厂商 API ----------

// ddnsUpsert 把记录对应的 A 记录设置为 ip：有则改，无则增
func ddnsUpsert(r config.DDNSRecord, ip string) error {
	switch r.Provider {
	case "cloudflare":
		return cloudflareUpsert(r, ip)
	case "dnspod":
		return dnspodUpsert(r, ip)
	case "namesilo":
		return namesiloUpsert(r, ip)
	}
	return fmt.Errorf("未知的 DNS 服务商: %q", r.Provider)
}

// isApex 判断是否为根域名记录
func isApex(sub string) bool { return sub == "" || sub == "@" }

// fqdn 返回完整域名
func fqdn(domain, sub string) string {
	if isApex(sub) {
		return domain
	}
	return sub + "." + domain
}

// ----- Cloudflare -----

type cfResp struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func cloudflareCall(token, method, path string, query url.Values, body interface{}) (json.RawMessage, error) {
	u := cloudflareBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := ddnsHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out cfResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("Cloudflare 响应解析失败(HTTP %d): %w", resp.StatusCode, err)
	}
	if !out.Success {
		msgs := make([]string, 0, len(out.Errors))
		for _, e := range out.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
	}
	return out.Result, nil
}

func cloudflareUpsert(r config.DDNSRecord, ip string) error {
	raw, err := cloudflareCall(r.Secret, "GET", "/zones", url.Values{"name": {r.Domain}}, nil)
	if err != nil {
		return err
	}
	var zones []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &zones); err != nil || len(zones) == 0 {
		return fmt.Errorf("Cloudflare: 找不到域名 %s", r.Domain)
	}
	zone := zones[0].ID
	name := fqdn(r.Domain, r.Sub)

	raw, err = cloudflareCall(r.Secret, "GET", "/zones/"+zone+"/dns_records",
		url.Values{"type": {"A"}, "name": {name}}, nil)
	if err != nil {
		return err
	}
	var recs []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &recs)

	body := map[string]interface{}{
		"type": "A", "name": name, "content": ip, "ttl": ddnsTTL, "proxied": false,
	}
	if len(recs) > 0 {
		_, err = cloudflareCall(r.Secret, "PUT", "/zones/"+zone+"/dns_records/"+recs[0].ID, nil, body)
	} else {
		_, err = cloudflareCall(r.Secret, "POST", "/zones/"+zone+"/dns_records", nil, body)
	}
	return err
}

// ----- DNSPod -----

type dnspodResp struct {
	Status struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Records []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"records"`
}

func dnspodCall(r config.DDNSRecord, action string, form url.Values) (*dnspodResp, error) {
	form.Set("login_token", r.AuthID+","+r.Secret)
	form.Set("format", "json")
	form.Set("domain", r.Domain)
	resp, err := ddnsHTTP.PostForm(dnspodBase+"/"+action, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out dnspodResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("DNSPod 响应解析失败(HTTP %d): %w", resp.StatusCode, err)
	}
	return &out, nil
}

func dnspodUpsert(r config.DDNSRecord, ip string) error {
	sub := r.Sub
	if isApex(sub) {
		sub = "@"
	}
	list, err := dnspodCall(r, "Record.List", url.Values{"sub_domain": {sub}, "record_type": {"A"}})
	if err != nil {
		return err
	}
	// 10 = 记录列表为空，不是错误
	if list.Status.Code != "1" && list.Status.Code != "10" {
		return fmt.Errorf("DNSPod: %s (code %s)", list.Status.Message, list.Status.Code)
	}
	recordID := ""
	for _, rec := range list.Records {
		if rec.Name == sub && rec.Type == "A" {
			recordID = rec.ID
			break
		}
	}

	form := url.Values{
		"sub_domain": {sub}, "record_type": {"A"}, "record_line": {"默认"},
		"value": {ip}, "ttl": {fmt.Sprint(ddnsTTL)},
	}
	action := "Record.Create"
	if recordID != "" {
		action = "Record.Modify"
		form.Set("record_id", recordID)
	}
	out, err := dnspodCall(r, action, form)
	if err != nil {
		return err
	}
	if out.Status.Code != "1" {
		return fmt.Errorf("DNSPod: %s (code %s)", out.Status.Message, out.Status.Code)
	}
	return nil
}

// ----- NameSilo -----

type namesiloResp struct {
	Reply struct {
		Code    int    `xml:"code"`
		Detail  string `xml:"detail"`
		Records []struct {
			ID    string `xml:"record_id"`
			Type  string `xml:"type"`
			Host  string `xml:"host"`
			Value string `xml:"value"`
		} `xml:"resource_record"`
	} `xml:"reply"`
}

func namesiloCall(r config.DDNSRecord, action string, q url.Values) (*namesiloResp, error) {
	q.Set("version", "1")
	q.Set("type", "xml")
	q.Set("key", r.Secret)
	q.Set("domain", r.Domain)
	resp, err := ddnsHTTP.Get(namesiloBase + "/" + action + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out namesiloResp
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("NameSilo 响应解析失败(HTTP %d): %w", resp.StatusCode, err)
	}
	if out.Reply.Code != 300 {
		return nil, fmt.Errorf("NameSilo: %s (code %d)", out.Reply.Detail, out.Reply.Code)
	}
	return &out, nil
}

func namesiloUpsert(r config.DDNSRecord, ip string) error {
	host := r.Sub
	if isApex(host) {
		host = ""
	}
	list, err := namesiloCall(r, "dnsListRecords", url.Values{})
	if err != nil {
		return err
	}
	recordID := ""
	for _, rec := range list.Reply.Records {
		if rec.Type == "A" && strings.EqualFold(rec.Host, fqdn(r.Domain, r.Sub)) {
			recordID = rec.ID
			break
		}
	}

	q := url.Values{"rrhost": {host}, "rrvalue": {ip}, "rrttl": {fmt.Sprint(namesiloTTL)}}
	action := "dnsAddRecord"
	if recordID != "" {
		action = "dnsUpdateRecord"
		q.Set("rrid", recordID)
	} else {
		q.Set("rrtype", "A")
	}
	_, err = namesiloCall(r, action, q)
	return err
}

// ---------- 定时运行与状态 ----------

// DDNSRecordStatus 是单条记录最近一次尝试的结果
type DDNSRecordStatus struct {
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
	IP    string    `json:"ip"`
	Time  time.Time `json:"time"`
}

// DDNSStatus 是页面展示用的整体状态
type DDNSStatus struct {
	WANIP   string                      `json:"wan_ip"`
	Error   string                      `json:"error,omitempty"`
	Checked time.Time                   `json:"checked"`
	Records map[string]DDNSRecordStatus `json:"records"`
}

var (
	ddnsRunMu  sync.Mutex // 串行化定时触发和手动触发
	ddnsMu     sync.Mutex // 保护下面的状态
	ddnsStatus = DDNSStatus{Records: map[string]DDNSRecordStatus{}}
	ddnsLast   = map[string]string{}
)

// GetDDNSStatus 返回状态快照
func GetDDNSStatus() DDNSStatus {
	ddnsMu.Lock()
	defer ddnsMu.Unlock()
	s := ddnsStatus
	s.Records = make(map[string]DDNSRecordStatus, len(ddnsStatus.Records))
	for k, v := range ddnsStatus.Records {
		s.Records[k] = v
	}
	return s
}

// RunDDNSNow 立即执行一轮：取 IP → 同步记录
func RunDDNSNow() {
	ddnsRunMu.Lock()
	defer ddnsRunMu.Unlock()

	cfg, err := config.LoadDDNSConfig()
	if err != nil {
		log.Printf("[ddns] %v", err)
		return
	}
	configured := false
	for _, r := range cfg.Records {
		configured = configured || r.Complete()
	}
	if !configured {
		return // 三家都没填，不生效
	}

	ip, err := fetchWANIP(wanIPURLs)
	ddnsMu.Lock()
	ddnsStatus.Checked = time.Now()
	if err != nil {
		ddnsStatus.Error = err.Error()
		ddnsMu.Unlock()
		log.Printf("[ddns] %v", err)
		return
	}
	ddnsStatus.WANIP, ddnsStatus.Error = ip, ""
	ddnsMu.Unlock()

	res := syncRecords(cfg.Records, ip, ddnsLast, ddnsUpsert)

	ddnsMu.Lock()
	defer ddnsMu.Unlock()
	// 清掉已删除记录的状态
	live := map[string]bool{}
	for _, r := range cfg.Records {
		if r.Complete() {
			live[r.ID] = true
		}
	}
	for id := range ddnsStatus.Records {
		if !live[id] {
			delete(ddnsStatus.Records, id)
			delete(ddnsLast, id)
		}
	}
	for id, e := range res {
		st := DDNSRecordStatus{OK: e == nil, IP: ip, Time: time.Now()}
		if e != nil {
			st.Error = e.Error()
			log.Printf("[ddns] 记录 %s 更新失败: %v", id, e)
		} else {
			log.Printf("[ddns] 记录 %s 已更新为 %s", id, ip)
		}
		ddnsStatus.Records[id] = st
	}
}

// StartDDNS 启动后台循环：立即执行一次，之后每 5 分钟一次
func StartDDNS() {
	go func() {
		RunDDNSNow()
		for range time.Tick(ddnsInterval) {
			RunDDNSNow()
		}
	}()
}
