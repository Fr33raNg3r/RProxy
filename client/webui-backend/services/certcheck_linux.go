//go:build linux

package services

import "syscall"

// markControl 给连接打 mark 255，nftables 据此放行、不经透明代理（与 Xray 出站的 sockopt.mark 相同）。
// 设置失败（非 root 运行的测试环境）时忽略，照常直连
func markControl(network, address string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, 255)
	})
}
