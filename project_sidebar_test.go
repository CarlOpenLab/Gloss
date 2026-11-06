package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 侧栏项目行只剩一个项目时也该能关掉；关光之后落回空状态。
func TestProjectSidebarClosesLastProject(t *testing.T) {
	dir := newRepoDir(t)
	a := newApp()
	a.openInNewTab(dir)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Frame()

	if len(a.tabs) != 1 {
		t.Fatalf("应先有一个项目，实际 %d", len(a.tabs))
	}

	name := filepath.Base(dir)
	row, ok := tt.Find(name)
	if !ok {
		t.Fatalf("侧栏里找不到项目 %q：%q", name, tt.Texts())
	}
	// 悬停在项目行上，关闭按钮才会出现。
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()

	closeBtn, ok := tt.Find("关闭项目")
	if !ok {
		t.Fatalf("只剩一个项目时，悬停也该出现关闭按钮：%q", tt.Texts())
	}
	tt.ClickAt(closeBtn.X+closeBtn.W/2, closeBtn.Y+closeBtn.H/2)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if len(a.tabs) != 0 {
		t.Fatalf("最后一个项目也应该能关掉，实际还剩 %d 个", len(a.tabs))
	}
	if a.repoPath != "" {
		t.Errorf("关光项目后 repoPath 应为空，实际 %q", a.repoPath)
	}
	if !tt.HasText("还没有打开项目") {
		t.Errorf("没有项目时该显示空状态：%q", tt.Texts())
	}
}
