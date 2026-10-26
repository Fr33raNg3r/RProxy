package services

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// ---------- 取 WAN IP ----------

func TestExtractIPv4(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"ipip 文本", "当前 IP：1.2.3.4  来自于：中国 广东", "1.2.3.4"},
		{"纯文本带换行", "203.0.113.9\n", "203.0.113.9"},
		{"JSON", `{"code":0,"data":{"ip":"8.8.4.4"}}`, "8.8.4.4"},
		{"非法八位组被跳过", "999.1.1.1 then 5.6.7.8", "5.6.7.8"},
		{"没有 IP", "<html>error</html>", ""},
	}
	for _, c := range cases {
		if got := extractIPv4(c.in); got != c.want {
			t.Errorf("%s: extractIPv4(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestFetchWANIPFallsBackToNextURL(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("当前 IP：9.9.9.9 来自于：x"))
	}))
	defer good.Close()

	ip, err := fetchWANIP([]string{bad.URL, good.URL})
	if err != nil || ip != "9.9.9.9" {
		t.Fatalf("got (%q, %v), want 9.9.9.9", ip, err)
	}
}

func TestFetchWANIPAllFail(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("no ip here"))
	}))
	defer bad.Close()
	if ip, err := fetchWANIP([]string{bad.URL}); err == nil {
		t.Fatalf("期望失败，却得到 %q", ip)
	}
}

// 查询公网 IP 的请求万一走了代理，拿到的是节点 VPS 的 IP：不能当成家宽 IP 写进 DDNS。
// 节点 IP 取自生成 Xray 配置时记下的解析结果（resolve-cache.json）；换下一个接口再试，都这样就报错
func TestFetchWANIPRejectsNodeIP(t *testing.T) {
	old := resolveCachePath
	t.Cleanup(func() { resolveCachePath = old })
	resolveCachePath = filepath.Join(t.TempDir(), "resolve-cache.json")
	if err := os.WriteFile(resolveCachePath, []byte(`{"vps.example.com":"203.0.113.9"}`), 0644); err != nil {
		t.Fatal(err)
	}
	viaProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.9"))
	}))
	defer viaProxy.Close()
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.2.3.4"))
	}))
	defer direct.Close()

	if ip, err := fetchWANIP([]string{viaProxy.URL, direct.URL}); err != nil || ip != "1.2.3.4" {
		t.Fatalf("应跳过节点 IP 取到 1.2.3.4，实际 (%q, %v)", ip, err)
	}
	if ip, err := fetchWANIP([]string{viaProxy.URL}); err == nil || !strings.Contains(err.Error(), "节点") {
		t.Fatalf("只取到节点 IP 时应报错并说明原因，实际 (%q, %v)", ip, err)
	}
}

// ---------- 同步逻辑 ----------

func TestSyncRecordsSkipsWhenIPUnchanged(t *testing.T) {
	recs := []config.DDNSRecord{{ID: "a", Provider: "cloudflare", Domain: "x.com", Sub: "h", Secret: "s"}}
	last := map[string]string{}
	calls := 0
	up := func(config.DDNSRecord, string) error { calls++; return nil }

	syncRecords(recs, "1.1.1.1", last, up)
	syncRecords(recs, "1.1.1.1", last, up)
	if calls != 1 {
		t.Fatalf("IP 未变应只调用 1 次，实际 %d", calls)
	}
	syncRecords(recs, "2.2.2.2", last, up)
	if calls != 2 {
		t.Fatalf("IP 变化后应再调用，实际 %d", calls)
	}
}

func TestSyncRecordsFailureIsRetriedAndIsolated(t *testing.T) {
	disabled := false
	recs := []config.DDNSRecord{
		{ID: "bad", Provider: "dnspod", Domain: "x.com", AuthID: "1", Secret: "s"},
		{ID: "ok", Provider: "namesilo", Domain: "y.com", Secret: "s"},
		{ID: "off", Provider: "cloudflare", Domain: "z.com"}, // 没填密钥：未配置完整
		{ID: "switched-off", Provider: "cloudflare", Domain: "w.com", Secret: "s", Enabled: &disabled},
	}
	last := map[string]string{}
	var mu sync.Mutex
	seen := map[string]int{}
	up := func(r config.DDNSRecord, ip string) error {
		mu.Lock()
		seen[r.ID]++
		mu.Unlock()
		if r.ID == "bad" {
			return errors.New("api down")
		}
		return nil
	}

	res := syncRecords(recs, "1.1.1.1", last, up)
	if res["bad"] == nil || res["ok"] != nil {
		t.Fatalf("结果不符: %v", res)
	}
	if _, ok := res["off"]; ok || seen["off"] != 0 {
		t.Fatal("没填完整的记录不应被处理")
	}
	if _, ok := res["switched-off"]; ok || seen["switched-off"] != 0 {
		t.Fatal("开关关闭的记录不应被处理")
	}
	syncRecords(recs, "1.1.1.1", last, up)
	if seen["bad"] != 2 || seen["ok"] != 1 {
		t.Fatalf("失败应重试、成功应跳过: %v", seen)
	}
}

// ---------- Cloudflare ----------

func TestCloudflareUpdatesExistingRecord(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/zones":
			// 完整域名 home.example.com 先被当作 zone 查询（查不到），再退一级查 example.com
			if r.URL.Query().Get("name") != "example.com" {
				_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"Z1"}]}`))
		case r.URL.Path == "/zones/Z1/dns_records" && r.Method == "GET":
			if r.URL.Query().Get("name") != "home.example.com" || r.URL.Query().Get("type") != "A" {
				t.Errorf("record query = %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"R1","content":"1.1.1.1"}]}`))
		default:
			b, _ := io.ReadAll(r.Body)
			gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(b)
			_, _ = w.Write([]byte(`{"success":true,"result":{"id":"R1"}}`))
		}
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "cloudflare", Domain: "home.example.com", Secret: "TOK"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer TOK" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotMethod != "PUT" || gotPath != "/zones/Z1/dns_records/R1" {
		t.Errorf("应 PUT 更新，实际 %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{`"content":"5.5.5.5"`, `"proxied":false`, `"ttl":600`, `"type":"A"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body %s 缺少 %s", gotBody, want)
		}
	}
}

// 填了登录邮箱时按 Global API Key 方式认证（X-Auth-Email + X-Auth-Key），不能再发 Bearer
func TestCloudflareGlobalKeyUsesEmailHeaders(t *testing.T) {
	var gotAuth, gotEmail, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotEmail = r.Header.Get("X-Auth-Email")
		gotKey = r.Header.Get("X-Auth-Key")
		switch {
		case r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"Z1"}]}`))
		case r.Method == "GET":
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"result":{"id":"R1"}}`))
		}
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "cloudflare", Domain: "h.example.com",
		Email: "me@example.com", Secret: "GLOBALKEY"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if gotEmail != "me@example.com" || gotKey != "GLOBALKEY" || gotAuth != "" {
		t.Errorf("认证头不符: email=%q key=%q auth=%q", gotEmail, gotKey, gotAuth)
	}
}

func TestCloudflareCreatesMissingRecordAtApex(t *testing.T) {
	var gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"Z1"}]}`))
		case r.Method == "GET":
			if r.URL.Query().Get("name") != "example.com" {
				t.Errorf("根域名应直接用域名，实际 %q", r.URL.Query().Get("name"))
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		default:
			b, _ := io.ReadAll(r.Body)
			gotMethod, gotBody = r.Method, string(b)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	if err := ddnsUpsert(config.DDNSRecord{Provider: "cloudflare", Domain: "example.com", Secret: "T"}, "5.5.5.5"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || !strings.Contains(gotBody, `"name":"example.com"`) {
		t.Errorf("应 POST 新建: %s %s", gotMethod, gotBody)
	}
}

func TestCloudflareReportsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}]}`))
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "cloudflare", Domain: "h.example.com", Secret: "bad"}, "1.1.1.1")
	if err == nil || !strings.Contains(err.Error(), "Invalid access token") {
		t.Fatalf("应带出 Cloudflare 的错误信息，实际 %v", err)
	}
}

// ---------- DNSPod ----------

func dnspodServer(listResp string, calls *[]url.Values, paths *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path == "/Domain.List" {
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"domains":[{"name":"example.com"},{"name":"other.org"}]}`))
			return
		}
		*calls = append(*calls, r.PostForm)
		*paths = append(*paths, r.URL.Path)
		if r.URL.Path == "/Record.List" {
			_, _ = w.Write([]byte(listResp))
			return
		}
		_, _ = w.Write([]byte(`{"status":{"code":"1","message":"ok"}}`))
	}))
}

func TestDNSPodModifiesExistingRecord(t *testing.T) {
	var calls []url.Values
	var paths []string
	srv := dnspodServer(`{"status":{"code":"1"},"records":[{"id":"77","name":"home","type":"A","value":"1.1.1.1"},{"id":"78","name":"home2","type":"A","value":"1.1.1.1"}]}`, &calls, &paths)
	defer srv.Close()
	defer swap(&dnspodBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "dnspod", Domain: "home.example.com", AuthID: "123", Secret: "tok"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if paths[len(paths)-1] != "/Record.Modify" {
		t.Fatalf("应调用 Record.Modify，实际 %v", paths)
	}
	m := calls[len(calls)-1]
	if m.Get("login_token") != "123,tok" || m.Get("record_id") != "77" || m.Get("value") != "5.5.5.5" || m.Get("ttl") != "600" {
		t.Errorf("Modify 参数不符: %v", m)
	}
}

func TestDNSPodCreatesWhenListEmpty(t *testing.T) {
	var calls []url.Values
	var paths []string
	srv := dnspodServer(`{"status":{"code":"10","message":"记录列表为空"}}`, &calls, &paths)
	defer srv.Close()
	defer swap(&dnspodBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "dnspod", Domain: "example.com", AuthID: "1", Secret: "t"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if paths[len(paths)-1] != "/Record.Create" {
		t.Fatalf("应调用 Record.Create，实际 %v", paths)
	}
	if got := calls[len(calls)-1].Get("sub_domain"); got != "@" {
		t.Errorf("根域名 sub_domain 应为 @，实际 %q", got)
	}
}

func TestDNSPodReportsAPIError(t *testing.T) {
	var calls []url.Values
	var paths []string
	srv := dnspodServer(`{"status":{"code":"-1","message":"登录失败"}}`, &calls, &paths)
	defer srv.Close()
	defer swap(&dnspodBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "dnspod", Domain: "h.example.com", AuthID: "1", Secret: "t"}, "5.5.5.5")
	if err == nil || !strings.Contains(err.Error(), "登录失败") {
		t.Fatalf("应带出 DNSPod 的错误信息，实际 %v", err)
	}
}

// ---------- NameSilo ----------

func namesiloServer(listXML string, last *url.Values, lastPath *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/listDomains" {
			_, _ = w.Write([]byte(`<namesilo><reply><code>300</code><detail>success</detail><domains><domain>example.com</domain><domain>other.org</domain></domains></reply></namesilo>`))
			return
		}
		if r.URL.Path == "/dnsListRecords" {
			_, _ = w.Write([]byte(listXML))
			return
		}
		*last, *lastPath = r.URL.Query(), r.URL.Path
		_, _ = w.Write([]byte(`<namesilo><reply><code>300</code><detail>success</detail></reply></namesilo>`))
	}))
}

func TestNameSiloUpdatesExistingRecord(t *testing.T) {
	var q url.Values
	var path string
	srv := namesiloServer(`<namesilo><reply><code>300</code><detail>success</detail>
<resource_record><record_id>abc</record_id><type>A</type><host>home.example.com</host><value>1.1.1.1</value></resource_record>
<resource_record><record_id>zzz</record_id><type>A</type><host>other.example.com</host><value>1.1.1.1</value></resource_record>
</reply></namesilo>`, &q, &path)
	defer srv.Close()
	defer swap(&namesiloBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "namesilo", Domain: "home.example.com", Secret: "KEY"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/dnsUpdateRecord" || q.Get("rrid") != "abc" || q.Get("rrhost") != "home" ||
		q.Get("rrvalue") != "5.5.5.5" || q.Get("rrttl") != "3600" || q.Get("key") != "KEY" {
		t.Errorf("update 参数不符: %s %v", path, q)
	}
}

func TestNameSiloAddsMissingRecord(t *testing.T) {
	var q url.Values
	var path string
	srv := namesiloServer(`<namesilo><reply><code>300</code><detail>success</detail></reply></namesilo>`, &q, &path)
	defer srv.Close()
	defer swap(&namesiloBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "namesilo", Domain: "example.com", Secret: "KEY"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/dnsAddRecord" || q.Get("rrtype") != "A" || q.Get("rrhost") != "" || q.Get("rrvalue") != "5.5.5.5" {
		t.Errorf("add 参数不符: %s %v", path, q)
	}
}

func TestNameSiloReportsAPIError(t *testing.T) {
	var q url.Values
	var path string
	srv := namesiloServer(`<namesilo><reply><code>280</code><detail>Invalid API Key</detail></reply></namesilo>`, &q, &path)
	defer srv.Close()
	defer swap(&namesiloBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "namesilo", Domain: "h.example.com", Secret: "x"}, "5.5.5.5")
	if err == nil || !strings.Contains(err.Error(), "Invalid API Key") {
		t.Fatalf("应带出 NameSilo 的错误信息，实际 %v", err)
	}
}

// ---------- 主域名自动探测 ----------

func TestZoneCandidatesLongestFirstKeepsTwoLabels(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"example.com", []string{"example.com"}},
		{"home.example.com", []string{"home.example.com", "example.com"}},
		{"a.example.com.cn", []string{"a.example.com.cn", "example.com.cn", "com.cn"}},
		{"localhost", nil},
	}
	for _, c := range cases {
		got := zoneCandidates(c.in)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("zoneCandidates(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// 多级后缀（.com.cn）必须选中账号里真实存在的 example.com.cn，而不是切成 com.cn
func TestCloudflareDetectsMultiLabelSuffixZone(t *testing.T) {
	var recordQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones":
			if r.URL.Query().Get("name") == "example.com.cn" {
				_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"ZCN"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		case r.URL.Path == "/zones/ZCN/dns_records" && r.Method == "GET":
			recordQuery = r.URL.Query().Get("name")
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "cloudflare", Domain: "nas.home.example.com.cn", Secret: "T"}, "5.5.5.5")
	if err != nil {
		t.Fatal(err)
	}
	if recordQuery != "nas.home.example.com.cn" {
		t.Errorf("记录名应为完整域名，实际 %q", recordQuery)
	}
}

func TestCloudflareZoneNotFoundReportsClearError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "cloudflare", Domain: "nozone.example.net", Secret: "T"}, "1.1.1.1")
	if err == nil || !strings.Contains(err.Error(), "找不到") || !strings.Contains(err.Error(), "nozone.example.net") {
		t.Fatalf("应提示找不到主域名，实际 %v", err)
	}
}

// 探测到的主域名会缓存，之后每轮更新不再重复探测
func TestZoneDetectionIsCached(t *testing.T) {
	zoneCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones":
			zoneCalls++
			if r.URL.Query().Get("name") == "cache.example.org" {
				_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"Z9"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		case r.Method == "GET":
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	defer srv.Close()
	defer swap(&cloudflareBase, srv.URL)()

	rec := config.DDNSRecord{Provider: "cloudflare", Domain: "www.cache.example.org", Secret: "T"}
	if err := ddnsUpsert(rec, "1.1.1.1"); err != nil {
		t.Fatal(err)
	}
	first := zoneCalls
	if err := ddnsUpsert(rec, "2.2.2.2"); err != nil {
		t.Fatal(err)
	}
	// 第二轮只剩 cloudflareUpsert 自己查 zone 的那一次，不再逐级探测
	if zoneCalls-first != 1 {
		t.Errorf("第二轮 /zones 请求应只有 1 次，实际 %d（首轮 %d）", zoneCalls-first, first)
	}
}

func TestDNSPodDomainNotInAccount(t *testing.T) {
	var calls []url.Values
	var paths []string
	srv := dnspodServer(`{"status":{"code":"1"}}`, &calls, &paths)
	defer srv.Close()
	defer swap(&dnspodBase, srv.URL)()

	err := ddnsUpsert(config.DDNSRecord{Provider: "dnspod", Domain: "x.notmine.com", AuthID: "1", Secret: "t"}, "1.1.1.1")
	if err == nil || !strings.Contains(err.Error(), "找不到") {
		t.Fatalf("账号下没有该域名应报错，实际 %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("找不到主域名时不应继续调用记录接口: %v", paths)
	}
}

func TestUpsertRejectsUnknownProvider(t *testing.T) {
	if err := ddnsUpsert(config.DDNSRecord{Provider: "nope"}, "1.1.1.1"); err == nil {
		t.Fatal("未知厂商应报错")
	}
}

// swap 临时替换字符串变量，返回还原函数
func swap(p *string, v string) func() {
	old := *p
	*p = v
	return func() { *p = old }
}
