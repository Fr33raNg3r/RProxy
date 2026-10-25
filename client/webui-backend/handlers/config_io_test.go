package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

func importNode(id string, order int) config.Node {
	return config.Node{ID: id, Name: id, Address: id + ".example.com", Port: 443, UUID: "u", WSPath: "/p", Order: order}
}

func validBundle() ClientConfigBundle {
	return ClientConfigBundle{
		WebUI: webUISection{ListenPort: 80, UpdateHour: 4, CurrentNodeID: "b", WGListenPort: 51820, WGSubnet: "172.16.7.0/24"},
		Nodes: []config.Node{importNode("a", 1), importNode("b", 0)},
	}
}

func TestValidateImportBundleAcceptsValid(t *testing.T) {
	b := validBundle()
	if err := validateImportBundle(&b); err != nil {
		t.Fatalf("合法配置不应被拒绝: %v", err)
	}
	if b.WebUI.CurrentNodeID != "b" {
		t.Fatalf("合法的当前节点不应被改动，实际 %q", b.WebUI.CurrentNodeID)
	}
}

// current_node_id 指向不存在的节点时，回退到 Order 最小的节点，并同步 Enabled
func TestValidateImportBundleFixesUnknownCurrentNode(t *testing.T) {
	b := validBundle()
	b.WebUI.CurrentNodeID = "gone"
	if err := validateImportBundle(&b); err != nil {
		t.Fatal(err)
	}
	if b.WebUI.CurrentNodeID != "b" {
		t.Fatalf("应回退到 Order 最小的节点 b，实际 %q", b.WebUI.CurrentNodeID)
	}
	for _, n := range b.Nodes {
		if n.Enabled != (n.ID == "b") {
			t.Fatalf("节点 %s Enabled=%v 与当前节点不一致", n.ID, n.Enabled)
		}
	}
}

func TestValidateImportBundleNoNodesClearsCurrent(t *testing.T) {
	b := validBundle()
	b.Nodes = nil
	if err := validateImportBundle(&b); err != nil {
		t.Fatal(err)
	}
	if b.WebUI.CurrentNodeID != "" {
		t.Fatalf("没有节点时当前节点应为空，实际 %q", b.WebUI.CurrentNodeID)
	}
}

func TestValidateImportBundleRejectsInvalid(t *testing.T) {
	cases := map[string]func(*ClientConfigBundle){
		"WG 子网格式错误":    func(b *ClientConfigBundle) { b.WebUI.WGSubnet = "not-a-cidr" },
		"WG 端口越界":      func(b *ClientConfigBundle) { b.WebUI.WGListenPort = 70000 },
		"启用 WG 但端口为 0": func(b *ClientConfigBundle) { b.WebUI.WGEnabled = true; b.WebUI.WGListenPort = 0 },
		"更新小时越界":       func(b *ClientConfigBundle) { b.WebUI.UpdateHour = 24 },
		"更新分钟越界":       func(b *ClientConfigBundle) { b.WebUI.UpdateMinute = 60 },
		"监听端口无效":       func(b *ClientConfigBundle) { b.WebUI.ListenPort = 0 },
	}
	for name, mutate := range cases {
		b := validBundle()
		mutate(&b)
		if err := validateImportBundle(&b); err == nil {
			t.Errorf("%s: 应被拒绝", name)
		}
	}
}

// 导入中途失败时，已改写的文件恢复原样、导入前不存在的文件被删除
func TestRestoreFilesRevertsChanges(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "nodes.json")
	created := filepath.Join(dir, "ddns.json")
	if err := os.WriteFile(existing, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	snap := snapshotFiles([]string{existing, created})
	_ = os.WriteFile(existing, []byte("new"), 0644)
	_ = os.WriteFile(created, []byte("new"), 0644)
	restoreFiles(snap)

	if b, _ := os.ReadFile(existing); string(b) != "old" {
		t.Fatalf("已有文件应恢复为 old，实际 %q", b)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("导入前不存在的文件应被删除")
	}
}
