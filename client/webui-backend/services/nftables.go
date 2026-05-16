package services

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Fr33raNg3r/RProxy/client/webui-backend/config"
)

// 匹配带 sentinel 注释的 WG sport 规则行（持久化文件用）。
// 必须依赖 sentinel —— 万一以后再加入其它 udp sport 规则也不会误改。
var wgSportRuleFileRe = regexp.MustCompile(`(?m)^([ \t]*)udp sport \d+ return([ \t]*#[ \t]*rproxy:wg-listen-port.*)$`)

// 从 `nft -a list chain inet tp output` 输出中匹配 `udp sport <port> return ... # handle <n>`
// nft 在 -a 模式下会把 handle 编号作为行尾注释打印出来。
var wgSportRuleRuntimeRe = regexp.MustCompile(`udp sport (\d+) return\s*#\s*handle\s+(\d+)`)

// SyncWGPortToNftables 把 nftables 中放行 WG 响应包的规则更新为新端口。
// 为什么需要：output 链里 `udp sport <wg-port> return` 是用于放行 WG 内核模块
// 作为响应方发出的包；如果该规则的端口和 wg-quick 实际监听端口不一致，
// 响应包会被 TPROXY 劫持，远端客户端永远握手不上。
//
// 同时做两件事：
//  1. 运行时原子替换规则（不触发 flush ruleset，避免清空 cn_ips/whitelist_ips/blacklist_ips
//     这些动态集合，否则会破坏 mosdns 已建立的分流状态）。
//  2. 改写持久化文件 /etc/nftables.conf，保证下次开机或手动重载时端口依然正确。
func SyncWGPortToNftables(newPort int) error {
	if newPort < 1 || newPort > 65535 {
		return fmt.Errorf("非法端口: %d", newPort)
	}

	if err := updateNftablesConfFile(newPort); err != nil {
		return err
	}
	if err := replaceWGSportRuleRuntime(newPort); err != nil {
		return fmt.Errorf("运行时规则替换失败: %w", err)
	}
	return nil
}

// updateNftablesConfFile 改写 /etc/nftables.conf，仅供持久化。
func updateNftablesConfFile(newPort int) error {
	b, err := os.ReadFile(config.NftablesConfPath)
	if err != nil {
		return fmt.Errorf("读取 %s: %w", config.NftablesConfPath, err)
	}
	if !wgSportRuleFileRe.Match(b) {
		// 文件里没有带 sentinel 的规则 —— 说明配置已被手工改过，不主动注入。
		return fmt.Errorf("未在 %s 找到带 `# rproxy:wg-listen-port` 标记的规则，请手工修改", config.NftablesConfPath)
	}
	replacement := fmt.Sprintf("${1}udp sport %d return${2}", newPort)
	updated := wgSportRuleFileRe.ReplaceAll(b, []byte(replacement))

	tmp := config.NftablesConfPath + ".tmp"
	if err := os.WriteFile(tmp, updated, 0644); err != nil {
		return fmt.Errorf("写临时文件: %w", err)
	}
	if err := os.Rename(tmp, config.NftablesConfPath); err != nil {
		return fmt.Errorf("替换 %s: %w", config.NftablesConfPath, err)
	}
	return nil
}

// replaceWGSportRuleRuntime 在运行中的 nftables 里找到 WG sport 放行规则的 handle，
// 然后用 nft replace rule 原子替换。
//
// 如果 nftables 还没载入（表/链不存在）就跳过 —— 此时持久化文件已更新，
// 下次 nft -f 时新端口自动生效。
func replaceWGSportRuleRuntime(newPort int) error {
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "tp", "output").Output()
	if err != nil {
		// 表/链未加载（如 nftables 服务未启动），不算致命 —— 文件已写，下次加载即生效。
		return nil
	}
	m := wgSportRuleRuntimeRe.FindStringSubmatch(string(out))
	if m == nil {
		// 运行中规则没有匹配的行 —— 也许是用户已经手工改成别的形式。
		return fmt.Errorf("在运行的 inet tp output 链中未找到 `udp sport <port> return` 规则")
	}
	handle := m[2]

	// nft replace rule <family> <table> <chain> handle <h> <new-statement>
	args := []string{
		"replace", "rule", "inet", "tp", "output",
		"handle", handle,
		"udp", "sport", fmt.Sprintf("%d", newPort), "return",
	}
	cmdOut, err := exec.Command("nft", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft replace rule: %w, 输出: %s", err, strings.TrimSpace(string(cmdOut)))
	}
	return nil
}
