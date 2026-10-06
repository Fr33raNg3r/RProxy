package services

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

func testNodes() []config.Node {
	// 故意打乱存放顺序，验证按 Order 而不是切片位置判断优先级
	return []config.Node{
		{ID: "c", Order: 2},
		{ID: "a", Order: 0},
		{ID: "b", Order: 1},
	}
}

func ids(ns []config.Node) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func TestNodesBeforeReturnsHigherPriorityInOrder(t *testing.T) {
	got := ids(nodesBefore(testNodes(), "c"))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("应按 Order 返回 [a b]，实际 %v", got)
	}
}

func TestNodesBeforeEmptyWhenCurrentIsTopOrUnknown(t *testing.T) {
	if got := nodesBefore(testNodes(), "a"); len(got) != 0 {
		t.Fatalf("当前已是最靠前，应为空，实际 %v", ids(got))
	}
	if got := nodesBefore(testNodes(), "nope"); len(got) != 0 {
		t.Fatalf("当前节点不存在，应为空，实际 %v", ids(got))
	}
}

func TestDecideFailbackNeedsConsecutiveSuccesses(t *testing.T) {
	before := nodesBefore(testNodes(), "c") // a, b
	up := func(n config.Node) bool { return n.ID == "a" }

	streaks := map[string]int{}
	for i := 1; i <= 2; i++ {
		var target *config.Node
		streaks, target = decideFailback(before, streaks, up, 3)
		if target != nil {
			t.Fatalf("第 %d 次不应回切", i)
		}
	}
	streaks, target := decideFailback(before, streaks, up, 3)
	if target == nil || target.ID != "a" {
		t.Fatalf("第 3 次应回切到 a，实际 %v", target)
	}
	if streaks["b"] != 0 {
		t.Errorf("一直失败的节点计数应为 0，实际 %d", streaks["b"])
	}
}

func TestDecideFailbackFailureResetsStreak(t *testing.T) {
	before := nodesBefore(testNodes(), "c")
	ok := true
	probe := func(n config.Node) bool { return n.ID == "a" && ok }

	streaks := map[string]int{}
	streaks, _ = decideFailback(before, streaks, probe, 3)
	streaks, _ = decideFailback(before, streaks, probe, 3)
	ok = false
	streaks, _ = decideFailback(before, streaks, probe, 3)
	if streaks["a"] != 0 {
		t.Fatalf("失败后应清零，实际 %d", streaks["a"])
	}
	ok = true
	_, target := decideFailback(before, streaks, probe, 3)
	if target != nil {
		t.Fatal("清零后一次成功不应回切")
	}
}

func TestDecideFailbackPicksMostPreferredAmongReady(t *testing.T) {
	before := nodesBefore(testNodes(), "c") // a, b
	allUp := func(config.Node) bool { return true }
	streaks := map[string]int{"a": 2, "b": 5} // b 计数更高，但 a 优先级更高
	_, target := decideFailback(before, streaks, allUp, 3)
	if target == nil || target.ID != "a" {
		t.Fatalf("应选 Order 最靠前的 a，实际 %v", target)
	}
}

func TestDecideFailbackDropsStaleStreaks(t *testing.T) {
	before := nodesBefore(testNodes(), "b") // 只有 a
	streaks, _ := decideFailback(before, map[string]int{"gone": 4, "c": 2}, func(config.Node) bool { return false }, 3)
	if _, ok := streaks["gone"]; ok {
		t.Error("已删除节点的计数应被丢弃")
	}
	if _, ok := streaks["c"]; ok {
		t.Error("不再位于当前节点前面的计数应被丢弃")
	}
}

func TestBuildProbeConfigIsIsolatedFromMainXray(t *testing.T) {
	n := config.Node{ID: "x", Address: "1.2.3.4", Port: 443, UUID: "u", WSPath: "/p", Host: "h.example.com"}
	b, err := buildProbeConfig(n, 10810)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]interface{}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	ins := c["inbounds"].([]interface{})
	if len(ins) != 1 {
		t.Fatalf("临时实例只应有 1 个入站，实际 %d", len(ins))
	}
	in := ins[0].(map[string]interface{})
	if in["protocol"] != "socks" || in["listen"] != "127.0.0.1" || in["port"].(float64) != 10810 {
		t.Errorf("入站应为 127.0.0.1:10810 的 socks，实际 %v", in)
	}
	out := c["outbounds"].([]interface{})[0].(map[string]interface{})
	if out["protocol"] != "vmess" {
		t.Errorf("出站应为 vmess，实际 %v", out["protocol"])
	}
	mark := out["streamSettings"].(map[string]interface{})["sockopt"].(map[string]interface{})["mark"].(float64)
	if mark != 255 {
		t.Errorf("出站必须带 mark=255 以避开 TPROXY，实际 %v", mark)
	}
	s := string(b)
	if strings.Contains(s, "12345") || strings.Contains(s, "access.log") {
		t.Error("临时配置不应包含 TPROXY 入口或落盘日志")
	}
}
