// RProxy WebUI Backend
// 入口文件，根据命令行参数分发到不同子命令：
//
//	webui serve              - 启动 HTTP 服务器（systemd 启动用）
//	webui hashpass <pwd>     - 生成密码 bcrypt 哈希（install.sh 调用）
//	webui render-xray        - 根据 nodes.json 重新生成 Xray 配置
//	webui render-wg          - 根据 peers.json 重新生成 wg0.conf
//	webui switch-next-node   - 切换到下一个启用节点（watchdog 调用）
//	webui failback-check     - 探测排在前面的节点，恢复后回切（watchdog 调用）
//	webui dns-check          - 检查本机 mosdns 能否应答（watchdog 调用）
//	webui sync-dns-nft       - 把 DNS 上游的 IP 同步到 nftables（nftables 重载后由 load-static-ips 服务调用）
package main

import (
	"fmt"
	"os"
	"time"

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
  webui render-mosdns      重新生成 mosdns config.yaml
  webui render-update-timer 按 webui.json 重建每日更新 timer（升级时用，保住自定义时间）
  webui switch-next-node   切换到下一个启用的节点
  webui failback-check     探测排在前面的节点，恢复后自动回切
  webui dns-check          检查本机 mosdns 能否应答（失败退出码 1）
  webui sync-dns-nft       把 DNS 上游的 IP 同步到 nftables（国内上游 → cn_ips，国外上游 → force_proxy_ips）
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
	case "render-mosdns":
		runRenderMosdns()
	case "render-update-timer":
		runRenderUpdateTimer()
	case "switch-next-node":
		runSwitchNextNode()
	case "failback-check":
		runFailbackCheck()
	case "dns-check":
		runDNSCheck()
	case "sync-dns-nft":
		runSyncDNSNft()
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

func runRenderMosdns() {
	// 首次运行时把默认值物化到 upstreams.json，方便 WebUI 直接读
	if _, err := os.Stat(config.DNSUpstreamsPath); os.IsNotExist(err) {
		if err := config.SaveDNSUpstreams(config.DefaultDNSUpstreams()); err != nil {
			fmt.Fprintln(os.Stderr, "初始化 DNS 上游配置失败:", err)
			os.Exit(1)
		}
	}
	u, err := config.LoadDNSUpstreams()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 DNS 上游配置失败:", err)
		os.Exit(1)
	}
	if err := services.RenderMosdnsConfig(u); err != nil {
		fmt.Fprintln(os.Stderr, "渲染 mosdns 配置失败:", err)
		os.Exit(1)
	}
	fmt.Println("mosdns 配置已生成: " + config.MosdnsConfigPath)
}

func runRenderUpdateTimer() {
	cfg, err := config.LoadWebUIConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 webui.json 失败:", err)
		os.Exit(1)
	}
	if err := services.WriteUpdateTimer(cfg.UpdateHour, cfg.UpdateMinute); err != nil {
		fmt.Fprintln(os.Stderr, "重建 update timer 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("update timer 已重建：每天 %02d:%02d（北京时间）\n", cfg.UpdateHour, cfg.UpdateMinute)
}

func runFailbackCheck() {
	msg, err := services.FailbackCheck()
	if err != nil {
		fmt.Fprintln(os.Stderr, "回切检查失败:", err)
		os.Exit(1)
	}
	if msg != "" {
		fmt.Println(msg)
	}
}

// runDNSCheck 向本机 mosdns 查询一个国内域名（不走代理，只检验 mosdns 本身能否应答），重试一次。
// 不用 dig：旁路由没装 dnsutils，也就不必为此在升级时新增 apt 依赖
func runDNSCheck() {
	var err error
	for i := 0; i < 2; i++ {
		if err = services.CheckDNS("127.0.0.1:53", "www.baidu.com", 3*time.Second); err == nil {
			return
		}
	}
	fmt.Fprintln(os.Stderr, "本机 DNS 无应答:", err)
	os.Exit(1)
}

// runSyncDNSNft 重启、nftables 重载都会把集合恢复成 nftables.conf 里预置的内容，
// WebUI 保存上游时同步进去的 IP 随之丢失：由 load-static-ips 服务在每次加载规则后重新同步
func runSyncDNSNft() {
	u, err := config.LoadDNSUpstreams()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 DNS 上游配置失败:", err)
		os.Exit(1)
	}
	if err := services.SyncDNSToNftables(u); err != nil {
		fmt.Fprintln(os.Stderr, "同步 DNS 上游到 nftables 失败:", err)
		os.Exit(1)
	}
}

func runSwitchNextNode() {
	if err := services.SwitchToNextNode(); err != nil {
		fmt.Fprintln(os.Stderr, "切换节点失败:", err)
		os.Exit(1)
	}
	fmt.Println("已切换到下一节点")
}
