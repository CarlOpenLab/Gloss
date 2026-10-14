//go:build !windows

package gitrepo

import "os/exec"

// hideConsole 在非 Windows 平台上不需要做任何事：这里没有「子进程自带窗口」的
// 问题。保留这个空实现，让调用处不必按平台分叉。
func hideConsole(*exec.Cmd) {}
