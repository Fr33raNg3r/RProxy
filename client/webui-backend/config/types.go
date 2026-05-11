package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
}

// ----------------- 节点配置（VMess + WebSocket + TLS）-----------------

type Node struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`  // 域名（必须是域名，不能是 IP）
	Port     int    `json:"port"`     // 通常 443
	UUID     string `json:"uuid"`     // VMess 用户 ID
	AlterID  int    `json:"alter_id"` // 通常 0（启用 AEAD）
	Security string `json:"security"` // VMess 加密：auto / aes-128-gcm / chacha20-poly1305 / none / zero
	WSPath   string `json:"ws_path"`  // WebSocket 路径，如 /a1b2c3d4e5f60789
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
	return loadWebUIConfigUnsafe()
}

func loadWebUIConfigUnsafe() (*WebUIConfig, error) {
	b, err := os.ReadFile(WebUIConfigPath)
	if err != nil {
		return nil, fmt.Errorf("读取 webui.json: %w", err)
	}
	var c WebUIConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("解析 webui.json: %w", err)
	}
	return &c, nil
}

func SaveWebUIConfig(c *WebUIConfig) error {
	muWebUI.Lock()
	defer muWebUI.Unlock()
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
