package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	b, err := buildProbeConfig(n, failbackSocksPort)
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
	addr := fmt.Sprintf("127.0.0.1:%d", failbackSocksPort)
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
	_, err = TestNode(failbackSocksPort)
	return err == nil
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
	if err := os.MkdirAll(filepath.Dir(config.FailbackState), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := config.FailbackState + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, config.FailbackState)
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

	// 切回目标节点；若前面还有更优先的节点，保持 AutoSwitched 以便继续回切
	cfg.AutoSwitched = len(nodesBefore(nodes, target.ID)) > 0
	if err := applyNodeSwitch(cfg, nodes, target.ID); err != nil {
		return "", err
	}
	_ = saveFailbackState(failbackState{Streaks: map[string]int{}})
	return fmt.Sprintf("节点 %s 已恢复，回切到该节点", target.Name), nil
}
