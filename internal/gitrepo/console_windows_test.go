//go:build windows

package gitrepo

import (
	"context"
	"os/exec"
	"testing"
)

// Gloss 在 Windows 上是 -H=windowsgui 构建的，进程没有控制台。默认情况下
// CreateProcess 会给每个控制台子进程新建一个控制台窗口，于是 autorefresh
// 每两秒一次的 git status / show-ref 会让黑框一闪一闪。这条用例盯住两个
// 子进程入口都带上了隐藏窗口的标志。
func TestChildCommandsHideConsoleWindow(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		cmd  *exec.Cmd
	}{
		{"git", newGitCmd(ctx, ".", "status", "--porcelain")},
		{"gh", newGhCmd(ctx, ".", "pr", "list")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attr := tc.cmd.SysProcAttr
			if attr == nil {
				t.Fatalf("%s: SysProcAttr is nil; a console window would flash", tc.name)
			}
			if !attr.HideWindow {
				t.Errorf("%s: HideWindow not set", tc.name)
			}
			if attr.CreationFlags&createNoWindow == 0 {
				t.Errorf("%s: CREATE_NO_WINDOW not set (CreationFlags=%#x)", tc.name, attr.CreationFlags)
			}
		})
	}
}
