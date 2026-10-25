//go:build !linux

package services

import "syscall"

// markControl 非 Linux 平台（仅开发机上跑测试）没有 SO_MARK，什么都不做
func markControl(network, address string, c syscall.RawConn) error { return nil }
