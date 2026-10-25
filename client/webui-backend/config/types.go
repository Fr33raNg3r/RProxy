package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ----------------- WebUI 总体配置 -----------------

type WebUIConfig struct {
	ListenPort    int    `json:"listen_port"`
	Username      string `json:"username"`
	PasswordHash  string `json:"password_hash"`
	SessionSecret string `json:"session_secret"`
	WGEnabled     bool   `json:"wg_enabled"` // 是否启用 WireGuard 入站服务
	WGListenPort  int    `json:"wg_listen_port"`
	WGSubnet      string `json:"wg_subnet"`
	WGEndpoint    string `json:"wg_endpoint"` // peer 配置中的 Endpoint，如 myhome.ddns.net 或公网 IP
	UpdateHour    int    `json:"update_hour"`
	UpdateMinute  int    `json:"update_minute"`
	CurrentNodeID string `json:"current_node_id"`
	// AutoSwitched: 当前节点是否由 watchdog 故障转移自动切到的（手动切换会清除）。
	// 仅为 true 时，才会在前面的节点恢复后自动回切
	AutoSwitched bool `json:"auto_switched"`
	// PasswordChanged: 是否已修改过初始随机密码。false（含字段缺失）= 首次登录需强制改密
	PasswordChanged bool `json:"password_changed"`
	// 本机静态网络配置（写入 /etc/network/interfaces）
	LanIface   string `json:"lan_iface"`
	LanIPCIDR  string `json:"lan_ip_cidr"`
	LanGateway string `json:"lan_gateway"`
}

// ----------------- 节点配置（VLESS + XHTTP + TLS）-----------------

// 节点协议。1.8.0 起只有 XHTTP；仍写入 nodes.json，回滚到 1.7.0 时旧程序据此按 XHTTP 渲染
const ProtocolVLESSXHTTP = "vless-xhttp"

type Node struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // 固定为 vless-xhttp
	Address  string `json:"address"`  // 域名（必须是域名，不能是 IP）
	Port     int    `json:"port"`     // 通常 443
	UUID     string `json:"uuid"`     // VLESS 用户 ID
	WSPath   string `json:"ws_path"`  // XHTTP 路径，如 /a1b2c3d4e5f60789（沿用旧字段名）
	Host     string `json:"host"`     // SNI 和 HTTP Host，通常等于 Address
	Enabled  bool   `json:"enabled"`
	Order    int    `json:"order"`
}

type NodesFile struct {
	Nodes []Node `json:"nodes"`
}

// ----------------- WireGuard peer -----------------

type WGPeer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PrivateKey   string `json:"private_key"` // 客户端私钥（用于生成客户端配置/二维码）
	PublicKey    string `json:"public_key"`  // 客户端公钥（写入服务端 wg0.conf）
	PresharedKey string `json:"preshared_key,omitempty"`
	Address      string `json:"address"` // peer 在 WG 网络内的 IP，如 172.16.7.2/32
	CreatedAt    int64  `json:"created_at"`
}

type WGPeersFile struct {
	Peers []WGPeer `json:"peers"`
}

// ----------------- 加载 / 保存 -----------------

var (
	muWebUI sync.Mutex
	muNodes sync.Mutex
	muPeers sync.Mutex
)

func LoadWebUIConfig() (*WebUIConfig, error) {
	muWebUI.Lock()
	defer muWebUI.Unlock()
	return loadWebUICached(WebUIConfigPath)
}

// webui.json 解析缓存：鉴权中间件每个请求都要读配置，文件 mtime 和大小都没变时
// 直接用上次的解析结果；watchdog 等其他进程改写文件后 mtime 会变，自动重新读取。
// 调用前需持有 muWebUI
var webUICache struct {
	path    string
	modTime time.Time
	size    int64
	cfg     WebUIConfig
	valid   bool
}

func resetWebUICache() { webUICache.valid = false }

func loadWebUICached(path string) (*WebUIConfig, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("读取 webui.json: %w", err)
	}
	if webUICache.valid && webUICache.path == path &&
		webUICache.modTime.Equal(st.ModTime()) && webUICache.size == st.Size() {
		c := webUICache.cfg // 返回副本：调用方会修改后再保存
		return &c, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 webui.json: %w", err)
	}
	var c WebUIConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("解析 webui.json: %w", err)
	}
	webUICache.path, webUICache.modTime, webUICache.size = path, st.ModTime(), st.Size()
	webUICache.cfg, webUICache.valid = c, true
	return &c, nil
}

func SaveWebUIConfig(c *WebUIConfig) error {
	muWebUI.Lock()
	defer muWebUI.Unlock()
	resetWebUICache()
	return writeJSONAtomic(WebUIConfigPath, c, 0600)
}

func LoadNodes() ([]Node, error) {
	muNodes.Lock()
	defer muNodes.Unlock()
	return loadNodesUnsafe()
}

func loadNodesUnsafe() ([]Node, error) {
	b, err := os.ReadFile(NodesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Node{}, nil
		}
		return nil, fmt.Errorf("读取 nodes.json: %w", err)
	}
	var f NodesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("解析 nodes.json: %w", err)
	}
	return f.Nodes, nil
}

func SaveNodes(nodes []Node) error {
	muNodes.Lock()
	defer muNodes.Unlock()
	return writeJSONAtomic(NodesPath, NodesFile{Nodes: nodes}, 0644)
}

func LoadWGPeers() ([]WGPeer, error) {
	muPeers.Lock()
	defer muPeers.Unlock()
	b, err := os.ReadFile(WGPeersPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []WGPeer{}, nil
		}
		return nil, err
	}
	var f WGPeersFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Peers, nil
}

func SaveWGPeers(peers []WGPeer) error {
	muPeers.Lock()
	defer muPeers.Unlock()
	return writeJSONAtomic(WGPeersPath, WGPeersFile{Peers: peers}, 0600)
}

// ----------------- 工具 -----------------

func writeJSONAtomic(path string, v interface{}, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, b, mode)
}

// WriteFileAtomic 原子写文件：写同目录下的唯一临时文件 → fsync → rename → fsync 目录。
// 旁路由常被直接断电：不 fsync 时 rename 可能先于数据落盘，开机后得到 0 字节文件；
// 临时文件名唯一，WebUI 与 watchdog 调用的 webui 子命令同时写同一文件也不会互相踩。
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	// 目录 fsync 让 rename 本身落盘；部分平台（如 Windows）不支持，忽略错误
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}
