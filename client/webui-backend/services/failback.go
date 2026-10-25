package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

const (
	failbackSocksPort = 10810 // 探测用临时 Xray 的 SOCKS 端口（主 Xray 用 10808）
	failbackNeed      = 3     // 连续探测成功多少次才回切（watchdog 每分钟一次，约 3 分钟）
)

// failbackState 是 data/failback.json：每个节点的连续探测成功次数
type failbackState struct {
	Streaks map[string]int `json:"streaks"`
}

// nodesBefore 返回按 Order 排在当前节点之前的节点（Order 升序）
// 当前节点不存在时返回空，避免误切
func nodesBefore(nodes []config.Node, currentID string) []config.Node {
	sorted := make([]config.Node, len(nodes))
	copy(sorted, nodes)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	for i, n := range sorted {
		if n.ID == currentID {
			return sorted[:i]
		}
	}
	return nil
}

// decideFailback 探测 before 里的每个节点并更新连续成功计数，
// 返回新的计数表（只保留 before 里的节点）和应回切的目标：
// 连续成功次数 >= need 的节点里 Order 最靠前的那个；没有则为 nil
func decideFailback(before []config.Node, streaks map[string]int, probe func(config.Node) bool, need int) (map[string]int, *config.Node) {
	next := make(map[string]int, len(before))
	var target *config.Node
	for i := range before {
		n := before[i]
		if probe(n) {
			next[n.ID] = streaks[n.ID] + 1
		} else {
			next[n.ID] = 0
		}
		if target == nil && next[n.ID] >= need {
			target = &before[i]
		}
	}
	return next, target
}

// buildProbeConfig 生成探测用的临时 Xray 配置：
// 只有一个 127.0.0.1 的 SOCKS 入站和被测节点的出站，不碰 TPROXY，也不写日志。
// 出站沿用 buildProxyOutbound，自带 mark=255，不会被透明代理再次劫持。
func buildProbeConfig(n config.Node, port int) ([]byte, error) {
	cfg := map[string]interface{}{
		"log": map[string]interface{}{"loglevel": "none"},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "probe-in",
				"listen":   "127.0.0.1",
				"port":     port,
				"protocol": "socks",
				"settings": map[string]interface{}{"auth": "noauth", "udp": false},
			},
		},
		"outbounds": []interface{}{buildProxyOutbound(&n)},
	}
	return json.Marshal(cfg)
}

// probeNodeViaXray 临时起一个独立的 Xray 实例走指定节点访问 generate_204，
// 测完无论成败都会杀掉进程并删除临时配置。主 Xray 全程不受影响。
func probeNodeViaXray(n config.Node) bool {
	port := failbackSocksPort
	b, err := buildProbeConfig(n, port)
	if err != nil {
		return false
	}
	f, err := os.CreateTemp("", "rproxy-probe-*.json")
	if err != nil {
		return false
	}
	defer os.Remove(f.Name())
	_, werr := f.Write(b)
	f.Close()
	if werr != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xray", "run", "-format=json", "-c", f.Name())
	if err := cmd.Start(); err != nil {
		return false
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// 等 SOCKS 端口就绪（最多约 3 秒）
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ready := false
	for i := 0; i < 30; i++ {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		return false
	}
	return probeViaSocks(port)
}

// probeTargets 代理探测地址，每次随机挑一个，访问目标不固定、与日常上网流量混在一起
// （与 scripts/watchdog.sh 的 PROBE_URLS 保持一致；WebUI 的节点"测试"按钮仍固定用 Google generate_204）
var probeTargets = []string{
	"https://www.google.com/generate_204",
	"https://www.youtube.com/generate_204",
	"https://www.facebook.com/favicon.ico",
	"https://www.instagram.com/favicon.ico",
	"https://x.com/favicon.ico",
	"https://www.netflix.com/favicon.ico",
}

// probeCodeOK 经代理收到 2xx/3xx/4xx 就说明整条链路是通的（favicon 返回 200、部分站点按地区跳转）；
// 5xx 与无响应（000）算失败。这些站点大陆都无法直连，不会把没走代理误判为正常
func probeCodeOK(code string) bool {
	return len(code) == 3 && code[0] >= '2' && code[0] <= '4'
}

// ProbeResult 一次由 WebUI 发起的代理探测（手动切换 / 编辑当前节点后立即测一次）
type ProbeResult struct {
	NodeID string
	OK     bool
	Time   time.Time
}

var (
	manualProbeMu sync.Mutex
	manualProbe   ProbeResult
)

// ProbeCurrentNode 经主 Xray（SOCKS 10808）立即探测一次并记下结果：watchdog 的探测间隔较长（白天 200～300 秒），
// 手动切换后不立即测，状态页要等好几分钟才更新连通性
func ProbeCurrentNode(nodeID string) bool {
	ok := probeViaSocks(config.XraySocksPort)
	manualProbeMu.Lock()
	manualProbe = ProbeResult{NodeID: nodeID, OK: ok, Time: time.Now()}
	manualProbeMu.Unlock()
	return ok
}

// LastManualProbe 最近一次由 WebUI 发起的探测（没有过则为零值）
func LastManualProbe() ProbeResult {
	manualProbeMu.Lock()
	defer manualProbeMu.Unlock()
	return manualProbe
}

// probeViaSocks 经指定 SOCKS 端口访问一个随机挑选的探测地址
func probeViaSocks(socksPort int) bool {
	url := probeTargets[rand.Intn(len(probeTargets))]
	out, _ := RunCmd(CmdTimeoutShort, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"--max-time", "8", "--socks5-hostname", fmt.Sprintf("127.0.0.1:%d", socksPort), url)
	return probeCodeOK(string(out))
}

func loadFailbackState() failbackState {
	var s failbackState
	if b, err := os.ReadFile(config.FailbackState); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Streaks == nil {
		s.Streaks = map[string]int{}
	}
	return s
}

func saveFailbackState(s failbackState) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(config.FailbackState, b, 0644)
}

// FailbackCheck 由 watchdog 每分钟调用（仅在代理正常时）。
// 只有当前节点是自动故障转移切到的、并且前面还有更优先的节点时才会探测；
// 前面的节点连续探测成功 failbackNeed 次后，切回其中 Order 最靠前的。
// 返回非空字符串表示发生了切换。
func FailbackCheck() (string, error) {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		return "", err
	}
	if !cfg.AutoSwitched {
		return "", nil
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		return "", err
	}
	before := nodesBefore(nodes, cfg.CurrentNodeID)
	if len(before) == 0 {
		// 当前已是最靠前的节点：标记失效，清掉
		cfg.AutoSwitched = false
		return "", config.SaveWebUIConfig(cfg)
	}

	state := loadFailbackState()
	streaks, target := decideFailback(before, state.Streaks, probeNodeViaXray, failbackNeed)
	if target == nil {
		return "", saveFailbackState(failbackState{Streaks: streaks})
	}

	// 探测耗时较长，期间管理员可能在 WebUI 改过配置：切换前重读最新配置，
	// 避免把探测前读到的旧配置整体写回去覆盖掉管理员的修改
	fresh, err := config.LoadWebUIConfig()
	if err != nil {
		return "", err
	}
	freshNodes, err := config.LoadNodes()
	if err != nil {
		return "", err
	}
	if !failbackStillValid(cfg.CurrentNodeID, fresh, freshNodes, target.ID) {
		// 已手动切换 / 目标被删：放弃本次回切，计数清零
		return "", saveFailbackState(failbackState{Streaks: map[string]int{}})
	}

	// 切回目标节点；若前面还有更优先的节点，保持 AutoSwitched 以便继续回切
	fresh.AutoSwitched = len(nodesBefore(freshNodes, target.ID)) > 0
	if err := applyNodeSwitch(fresh, freshNodes, target.ID); err != nil {
		return "", err
	}
	_ = saveFailbackState(failbackState{Streaks: map[string]int{}})
	return fmt.Sprintf("节点 %s 已恢复，回切到该节点", target.Name), nil
}

// failbackStillValid 判断重读到的最新配置是否仍允许回切：
// 仍处于自动切换状态、当前节点没被改动、目标节点还在
func failbackStillValid(probedCurrentID string, fresh *config.WebUIConfig, freshNodes []config.Node, targetID string) bool {
	if !fresh.AutoSwitched || fresh.CurrentNodeID != probedCurrentID {
		return false
	}
	for _, n := range freshNodes {
		if n.ID == targetID {
			return true
		}
	}
	return false
}
