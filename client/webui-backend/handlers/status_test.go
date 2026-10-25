package handlers

import (
	"testing"
	"time"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"
)

const tsLayout = "2006-01-02 15:04:05"

// 手动切换后 WebUI 立即探测的结果比 watchdog 最近一次探测新，且针对当前节点：状态页用它；
// watchdog 之后再探测（last_probe 更新）就换回 watchdog 的结果
func TestMergeManualProbe(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local)
	watchdogAt := now.Add(-5 * time.Minute).Format(tsLayout)
	manual := services.ProbeResult{NodeID: "b", OK: true, Time: now.Add(-1 * time.Minute)}

	h := mergeManualProbe(&services.HealthData{ProxyOK: 0, LastProbe: watchdogAt}, "b", manual)
	if h.ProxyOK != 1 {
		t.Fatalf("手动探测更新、针对当前节点，应显示其结果，实际 proxy_ok=%d", h.ProxyOK)
	}

	later := now.Format(tsLayout)
	h = mergeManualProbe(&services.HealthData{ProxyOK: 0, LastProbe: later}, "b", manual)
	if h.ProxyOK != 0 {
		t.Fatal("watchdog 之后又探测过，应以 watchdog 结果为准")
	}

	h = mergeManualProbe(&services.HealthData{ProxyOK: 0, LastProbe: watchdogAt}, "a", manual)
	if h.ProxyOK != 0 {
		t.Fatal("手动探测的不是当前节点，不能用")
	}

	if h := mergeManualProbe(nil, "b", manual); h == nil || h.ProxyOK != 1 {
		t.Fatal("watchdog 还没写过 health.json 时也应显示手动探测结果")
	}

	if h := mergeManualProbe(nil, "b", services.ProbeResult{}); h != nil {
		t.Fatal("没有手动探测记录时保持原样")
	}
}
