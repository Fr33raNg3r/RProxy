package services

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// switchStub 记录切换节点各步的调用顺序与最终写盘内容
type switchStub struct {
	calls      []string
	savedCfg   *config.WebUIConfig
	savedNodes []config.Node
	// 磁盘上的"当前"内容：渲染之后才读，模拟渲染期间被 WebUI 改过
	diskCfg   config.WebUIConfig
	diskNodes []config.Node
}

// stubSwitchSteps 把切换节点的各步换成假实现，测试结束后还原
func stubSwitchSteps(t *testing.T, renderErr error) *switchStub {
	s := &switchStub{
		diskCfg:   config.WebUIConfig{CurrentNodeID: "a"},
		diskNodes: []config.Node{{ID: "a", Enabled: true}, {ID: "b"}},
	}
	oldRender, oldLoadCfg, oldLoadNodes, oldNodes, oldCfg, oldRestart := renderXray, loadWebUI, loadNodes, saveNodes, saveWebUI, restartXray
	t.Cleanup(func() {
		renderXray, loadWebUI, loadNodes, saveNodes, saveWebUI, restartXray = oldRender, oldLoadCfg, oldLoadNodes, oldNodes, oldCfg, oldRestart
	})
	renderXray = func([]config.Node, string) error { s.calls = append(s.calls, "render"); return renderErr }
	loadWebUI = func() (*config.WebUIConfig, error) { c := s.diskCfg; return &c, nil }
	loadNodes = func() ([]config.Node, error) { return append([]config.Node(nil), s.diskNodes...), nil }
	saveNodes = func(n []config.Node) error { s.calls = append(s.calls, "saveNodes"); s.savedNodes = n; return nil }
	saveWebUI = func(c *config.WebUIConfig) error { s.calls = append(s.calls, "saveWebUI"); s.savedCfg = c; return nil }
	restartXray = func() error { s.calls = append(s.calls, "restart"); return nil }
	return s
}

// 新节点的配置没通过 xray -test 时什么都不保存：否则 current_node_id 已指向新节点，
// Xray 却还跑着旧配置，界面显示与实际不符
func TestApplyNodeSwitchRenderFailureSavesNothing(t *testing.T) {
	s := stubSwitchSteps(t, errors.New("xray 配置验证失败"))
	if err := ApplyNodeSwitch(s.diskNodes, "b", true); err == nil {
		t.Fatal("渲染失败时应返回错误")
	}
	if want := []string{"render"}; !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("渲染失败后不应保存或重启，实际调用 %v", s.calls)
	}
}

func TestApplyNodeSwitchRendersBeforeSaving(t *testing.T) {
	s := stubSwitchSteps(t, nil)
	if err := ApplyNodeSwitch(s.diskNodes, "b", true); err != nil {
		t.Fatalf("切换失败: %v", err)
	}
	if want := []string{"render", "saveNodes", "saveWebUI", "restart"}; !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("调用顺序应为 %v，实际 %v", want, s.calls)
	}
	if s.savedCfg.CurrentNodeID != "b" || !s.savedCfg.AutoSwitched || s.savedNodes[0].Enabled || !s.savedNodes[1].Enabled {
		t.Fatalf("切换后当前节点与 Enabled 应指向 b：%+v %+v", s.savedCfg, s.savedNodes)
	}
}

// 渲染（xray -test、解析节点域名）要几秒，期间管理员在 WebUI 改了密码、编辑了节点：
// 写回时要以最新的文件为准，只改当前节点相关字段，不能把切换开始时读到的旧内容整体写回去
func TestApplyNodeSwitchKeepsEditsMadeDuringRender(t *testing.T) {
	s := stubSwitchSteps(t, nil)
	started := []config.Node{{ID: "a", Enabled: true}, {ID: "b"}}
	s.diskCfg.PasswordHash = "改密后的哈希"
	s.diskNodes = []config.Node{{ID: "a", Enabled: true, Name: "改过名"}, {ID: "b"}}

	if err := ApplyNodeSwitch(started, "b", false); err != nil {
		t.Fatalf("切换失败: %v", err)
	}
	if s.savedCfg.PasswordHash != "改密后的哈希" || s.savedCfg.CurrentNodeID != "b" || s.savedCfg.AutoSwitched {
		t.Fatalf("应保留渲染期间的改密并切到 b：%+v", s.savedCfg)
	}
	if s.savedNodes[0].Name != "改过名" || !s.savedNodes[1].Enabled {
		t.Fatalf("应保留渲染期间对节点的编辑：%+v", s.savedNodes)
	}
}

// 渲染期间目标节点被删掉了：放弃切换，不写盘、不重启
func TestApplyNodeSwitchAbortsWhenTargetDeletedDuringRender(t *testing.T) {
	s := stubSwitchSteps(t, nil)
	started := []config.Node{{ID: "a", Enabled: true}, {ID: "b"}}
	s.diskNodes = []config.Node{{ID: "a", Enabled: true}}

	if err := ApplyNodeSwitch(started, "b", true); err == nil {
		t.Fatal("目标节点已被删除时应返回错误")
	}
	if want := []string{"render"}; !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("目标节点已被删除时不应保存或重启，实际调用 %v", s.calls)
	}
}
