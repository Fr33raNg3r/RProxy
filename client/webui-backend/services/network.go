package services

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

const interfacesPath = "/etc/network/interfaces"

// ValidateLanCIDR 校验本机 IP：必须是 IPv4 内网地址且为 CIDR 格式（前缀 8-30）
func ValidateLanCIDR(cidr string) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("IP 地址必须为 CIDR 格式，如 192.168.1.2/24")
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("仅支持 IPv4 地址")
	}
	if !ip4.IsPrivate() {
		return fmt.Errorf("IP 必须为内网地址（10.x / 172.16-31.x / 192.168.x）")
	}
	prefix, _ := ipnet.Mask.Size()
	if prefix < 8 || prefix > 30 {
		return fmt.Errorf("子网前缀必须在 8-30 之间")
	}
	return nil
}

// ValidateLanGateway 校验网关：合法 IPv4 且与给定 CIDR 同子网
func ValidateLanGateway(gw, cidr string) error {
	gwIP := net.ParseIP(gw)
	if gwIP == nil || gwIP.To4() == nil {
		return fmt.Errorf("网关地址不合法：%s", gw)
	}
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	if !ipnet.Contains(gwIP) {
		return fmt.Errorf("网关 %s 与本机 IP %s 不在同一子网", gw, cidr)
	}
	return nil
}

// WriteInterfacesFile 把静态网络配置写入 /etc/network/interfaces（ifupdown）
// 首次写入前把原始配置备份到 *.rproxy.orig（只备份一次）
func WriteInterfacesFile(iface, cidr, gateway string) error {
	if iface == "" {
		return fmt.Errorf("网卡接口不能为空")
	}
	if _, err := os.Stat(interfacesPath); err == nil {
		if _, err := os.Stat(interfacesPath + ".rproxy.orig"); os.IsNotExist(err) {
			if b, e := os.ReadFile(interfacesPath); e == nil {
				_ = os.WriteFile(interfacesPath+".rproxy.orig", b, 0644)
			}
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Managed by RProxy —— 本机网络由 WebUI【设置】页管理，请勿手动编辑\n")
	fmt.Fprintf(&buf, "# 原始配置已备份到 %s.rproxy.orig\n", interfacesPath)
	fmt.Fprintf(&buf, "auto lo\niface lo inet loopback\n\n")
	fmt.Fprintf(&buf, "auto %s\niface %s inet static\n", iface, iface)
	fmt.Fprintf(&buf, "    address %s\n", cidr)
	fmt.Fprintf(&buf, "    gateway %s\n", gateway)

	tmp := interfacesPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, interfacesPath)
}

// DetectCurrentNetwork 实时探测本机当前默认网卡 / IP(CIDR) / 网关，
// 供 WebUI 在 webui.json 尚未记录 lan_* 时显示真实现状。探测失败的字段返回空串。
func DetectCurrentNetwork() (iface, cidr, gateway string) {
	if out, err := exec.Command("ip", "route").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 5 || f[0] != "default" {
				continue
			}
			for i := 0; i < len(f)-1; i++ {
				switch f[i] {
				case "via":
					gateway = f[i+1]
				case "dev":
					iface = f[i+1]
				}
			}
			break
		}
	}
	if iface != "" {
		if out, err := exec.Command("ip", "-o", "-f", "inet", "addr", "show", iface).Output(); err == nil {
			f := strings.Fields(string(out))
			for i := 0; i < len(f)-1; i++ {
				if f[i] == "inet" {
					cidr = f[i+1]
					break
				}
			}
		}
	}
	return
}

// RestartNetworking 异步重启 networking 使新 IP 生效。
// IP 变更会断开当前 WebUI 连接，故延迟 1 秒让本次 HTTP 响应先发完。
func RestartNetworking() {
	go func() {
		_ = exec.Command("bash", "-c",
			"sleep 1 && systemctl enable networking >/dev/null 2>&1; systemctl restart networking").Start()
	}()
}
