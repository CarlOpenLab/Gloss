//go:build windows

package gitrepo

import (
	"os/exec"
	"syscall"
)

// createNoWindow 是 CreateProcess 的 CREATE_NO_WINDOW 标志：子进程照常跑，
// 但不给它分配控制台窗口。
const createNoWindow = 0x08000000

// hideConsole 让 git / gh 这类控制台程序在 Windows 上不弹黑框。
//
// Gloss 自己是用 -H=windowsgui 构建的（见 mygo 的 cmd/mygo/build.go），进程没有
// 控制台；默认情况下 CreateProcess 会给每个控制台子进程新建一个控制台窗口。
// 后台刷新每两秒就要跑一次 git status / git show-ref，窗口就跟着一闪一闪。
// 带上这两个标志后子进程仍拿得到管道里的 stdin/stdout/stderr，只是没有窗口。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
