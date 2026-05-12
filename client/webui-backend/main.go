// RProxy WebUI Backend
// 入口文件，根据命令行参数分发到不同子命令：
//
//	webui serve              - 启动 HTTP 服务器（systemd 启动用）
//	webui hashpass <pwd>     - 生成密码 bcrypt 哈希（install.sh 调用）
//	webui render-xray        - 根据 nodes.json 重新生成 Xray 配置
//	webui render-wg          - 根据 peers.json 重新生成 wg0.conf
//	webui switch-next-node   - 切换到下一个启用节点（watchdog 调用）
package main

import (
	"fmt"
	"os"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
	"github.com/Fr33raNg3r/RProxy/client/webui-backend/services"

	"golang.org/x/crypto/bcrypt"
)

const Version = "0.1.0"

func usage() {
	fmt.Printf(`RProxy WebUI Backend %s

用法：
  webui serve              启动 HTTP 服务器
  webui hashpass <pwd>     生成密码 bcrypt 哈希
  webui render-xray        重新生成 Xray 配置文件
  webui render-wg          重新生成 wg0.conf
  webui switch-next-node   切换到下一个启用的节点
  webui version            打印版本号
`, Version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServe()
	case "hashpass":
		runHashPass()
	case "render-xray":
		runRenderXray()
	case "render-wg":
		runRenderWG()
	case "switch-next-node":
		runSwitchNextNode()
	case "version":
		fmt.Println(Version)
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(1)
	}
}

func runHashPass() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: webui hashpass <password>")
		os.Exit(1)
	}
	pwd := os.Args[2]
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成密码哈希失败:", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}

func runRenderXray() {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 webui.json 失败:", err)
		os.Exit(1)
	}
	nodes, err := config.LoadNodes()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 nodes.json 失败:", err)
		os.Exit(1)
	}
	if err := services.RenderXrayConfig(nodes, cfg.CurrentNodeID); err != nil {
		fmt.Fprintln(os.Stderr, "渲染 Xray 配置失败:", err)
		os.Exit(1)
	}
	fmt.Println("Xray 配置已生成: /usr/local/etc/xray/config.json")
}

func runRenderWG() {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 webui.json 失败:", err)
		os.Exit(1)
	}
	peers, err := config.LoadWGPeers()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 peers.json 失败:", err)
		os.Exit(1)
	}
	if err := services.RenderWGConfig(cfg, peers); err != nil {
		fmt.Fprintln(os.Stderr, "渲染 WG 配置失败:", err)
		os.Exit(1)
	}
	fmt.Println("WG 配置已生成: /etc/wireguard/wg0.conf")
}

func runSwitchNextNode() {
	if err := services.SwitchToNextNode(); err != nil {
		fmt.Fprintln(os.Stderr, "切换节点失败:", err)
		os.Exit(1)
	}
	fmt.Println("已切换到下一节点")
}
