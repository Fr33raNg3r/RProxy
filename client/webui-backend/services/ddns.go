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
	"os"
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
			if nodeIPs()[ip] {
				// 这次查询走了代理（接口的 IP 不在 cn_ips 里），取到的是 VPS 的出口 IP，不能写进 DDNS
				lastErr = fmt.Errorf("%s 返回的 %s 是节点 VPS 的地址（查询请求走了代理）", u, ip)
				continue
			}
			return ip, nil
		}
		lastErr = fmt.Errorf("%s 返回内容里没有 IPv4", u)
	}
	return "", fmt.Errorf("获取 WAN IP 失败: %v", lastErr)
}

// nodeIPs 各节点 VPS 的 IP：取自生成 Xray 配置时记下的域名解析结果（resolve-cache.json）
func nodeIPs() map[string]bool {
	cache := map[string]string{}
	if b, err := os.ReadFile(resolveCachePath); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	ips := make(map[string]bool, len(cache))
	for _, ip := range cache {
		ips[ip] = true
	}
	return ips
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
		fp := strings.Join([]string{r.Provider, r.Domain, r.Email, ip}, "|")
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

// ddnsUpsert 把记录对应的 A 记录设置为 ip：有则改，无则增。
// r.Domain 是完整域名（如 home.example.com），其中哪一段是账号下的主域名由这里自动探测，
// 探测结果缓存起来，之后每轮更新不再重复探测。
func ddnsUpsert(r config.DDNSRecord, ip string) error {
	var exists func(string) (bool, error)
	var upsert func(config.DDNSRecord, string) error
	switch r.Provider {
	case "cloudflare":
		exists, upsert = cloudflareZoneExists(r), cloudflareUpsert
	case "dnspod":
		exists, upsert = dnspodZoneExists(r), dnspodUpsert
	case "namesilo":
		exists, upsert = namesiloZoneExists(r), namesiloUpsert
	default:
		return fmt.Errorf("未知的 DNS 服务商: %q", r.Provider)
	}

	key := strings.Join([]string{r.Provider, r.Domain, r.Email}, "|")
	ddnsZoneMu.Lock()
	zone := ddnsZones[key]
	ddnsZoneMu.Unlock()
	if zone == "" {
		var err error
		if zone, err = detectZone(r.Domain, exists); err != nil {
			return err
		}
	}

	// 交给各厂商实现的记录：Domain 是主域名，Sub 是其前面的部分（根域名为空）
	zr := r
	zr.Domain = zone
	zr.Sub = strings.TrimSuffix(strings.TrimSuffix(r.Domain, zone), ".")
	err := upsert(zr, ip)

	ddnsZoneMu.Lock()
	defer ddnsZoneMu.Unlock()
	if err != nil {
		delete(ddnsZones, key) // 失败时丢掉缓存，下一轮重新探测（可能是域名被移走了）
	} else {
		ddnsZones[key] = zone
	}
	return err
}

// 探测到的主域名缓存：键为 厂商|完整域名|邮箱
var (
	ddnsZoneMu sync.Mutex
	ddnsZones  = map[string]string{}
)

// zoneCandidates 返回完整域名可能的主域名，从最长到最短，至少保留两段
// （home.example.com.cn → home.example.com.cn / example.com.cn / com.cn）
func zoneCandidates(full string) []string {
	labels := strings.Split(full, ".")
	var out []string
	for i := 0; len(labels)-i >= 2; i++ {
		out = append(out, strings.Join(labels[i:], "."))
	}
	return out
}

// detectZone 依次询问各候选是否是账号下的域名，返回第一个命中的
func detectZone(full string, exists func(string) (bool, error)) (string, error) {
	for _, z := range zoneCandidates(full) {
		ok, err := exists(z)
		if err != nil {
			return "", err
		}
		if ok {
			return z, nil
		}
	}
	return "", fmt.Errorf("账号下找不到 %s 对应的主域名，请确认域名已托管在该服务商且密钥有权限", full)
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

// cloudflareCall 发起一次 API 请求：email 非空按 Global API Key 认证，否则按 API Token（Bearer）认证
func cloudflareCall(email, token, method, path string, query url.Values, body interface{}) (json.RawMessage, error) {
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
	if email != "" {
		req.Header.Set("X-Auth-Email", email)
		req.Header.Set("X-Auth-Key", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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

// cloudflareZoneExists 返回"按名称查 zone"的探测函数
func cloudflareZoneExists(r config.DDNSRecord) func(string) (bool, error) {
	return func(zone string) (bool, error) {
		raw, err := cloudflareCall(r.Email, r.Secret, "GET", "/zones", url.Values{"name": {zone}}, nil)
		if err != nil {
			return false, err
		}
		var zones []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &zones); err != nil {
			return false, nil
		}
		return len(zones) > 0, nil
	}
}

func cloudflareUpsert(r config.DDNSRecord, ip string) error {
	raw, err := cloudflareCall(r.Email, r.Secret, "GET", "/zones", url.Values{"name": {r.Domain}}, nil)
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

	raw, err = cloudflareCall(r.Email, r.Secret, "GET", "/zones/"+zone+"/dns_records",
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
		_, err = cloudflareCall(r.Email, r.Secret, "PUT", "/zones/"+zone+"/dns_records/"+recs[0].ID, nil, body)
	} else {
		_, err = cloudflareCall(r.Email, r.Secret, "POST", "/zones/"+zone+"/dns_records", nil, body)
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
	Domains []struct {
		Name string `json:"name"`
	} `json:"domains"`
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

// dnspodZoneExists 返回探测函数：第一次调用时拉取账号下的域名列表，之后在列表里匹配
func dnspodZoneExists(r config.DDNSRecord) func(string) (bool, error) {
	var names map[string]bool
	return func(zone string) (bool, error) {
		if names == nil {
			lr := r
			lr.Domain = ""
			out, err := dnspodCall(lr, "Domain.List", url.Values{})
			if err != nil {
				return false, err
			}
			if out.Status.Code != "1" {
				return false, fmt.Errorf("DNSPod: %s (code %s)", out.Status.Message, out.Status.Code)
			}
			names = map[string]bool{}
			for _, d := range out.Domains {
				names[strings.ToLower(d.Name)] = true
			}
		}
		return names[zone], nil
	}
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
		Code    int      `xml:"code"`
		Detail  string   `xml:"detail"`
		Domains []string `xml:"domains>domain"`
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
	if r.Domain != "" {
		q.Set("domain", r.Domain)
	}
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

// namesiloZoneExists 返回探测函数：第一次调用时拉取账号下的域名列表，之后在列表里匹配
func namesiloZoneExists(r config.DDNSRecord) func(string) (bool, error) {
	var names map[string]bool
	return func(zone string) (bool, error) {
		if names == nil {
			lr := r
			lr.Domain = ""
			out, err := namesiloCall(lr, "listDomains", url.Values{})
			if err != nil {
				return false, err
			}
			names = map[string]bool{}
			for _, d := range out.Reply.Domains {
				names[strings.ToLower(strings.TrimSpace(d))] = true
			}
		}
		return names[zone], nil
	}
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

// GetWANIP 返回家庭宽带的公网 IPv4（WG 客户端 Endpoint 的默认值），取不到返回空串：
// 优先用 DDNS 最近一次取到的，没有再现查（查询接口都是国内站点，直连，不会取到 VPS 的 IP）
func GetWANIP() string {
	if ip := GetDDNSStatus().WANIP; ip != "" {
		return ip
	}
	ip, _ := fetchWANIP(wanIPURLs)
	return ip
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
