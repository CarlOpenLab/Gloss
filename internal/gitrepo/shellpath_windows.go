//go:build windows

package gitrepo

import (
	"os"
	"os/exec"
)

// shellPATHEnv 在 Windows 上不需要补任何东西。
//
// 「GUI 进程的 PATH 比终端少一截」是 macOS 从 Finder / Dock 启动 .app 才有的
// 问题：那里进程环境是 launchd 给的。Windows 上从开始菜单、资源管理器启动的
// 进程直接继承用户环境（含 HKCU\Environment 里的 Path），终端里能跑的 node、
// gh 在 GUI 里同样在 PATH 上。硬去「猜一份 PATH」只会把用户配好的环境弄坏，
// 所以原样返回。
func shellPATHEnv() []string {
	return os.Environ()
}

// lookPathInShellPATH 在 Windows 上就是 exec.LookPath：进程看见的 PATH 就是
// 子进程会看见的 PATH，而且 PATHEXT（.exe/.cmd/.bat…）那套只有它处理得对。
func lookPathInShellPATH(file string) (string, error) {
	return exec.LookPath(file)
}
