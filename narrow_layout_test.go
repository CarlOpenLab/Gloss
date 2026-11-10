package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 窄窗口下的排版回归：文件 tab 栏和终端会话栏都不能把关键控件挤成零宽。

// 760 宽（应用最小 720）时编辑器栏只剩两百来像素：右侧的完整路径和状态
// chip 该让位，把宽度留给文件 tab —— 否则第一枚 tab 的关闭 × 都是零宽。
func TestEditorTabStripUsableInNarrowWindow(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	for _, p := range []string{"internal/server/handler.go", "README.md", "internal/server/router.go"} {
		selectEditorFile(t, a, p)
		settle(t, a)
	}

	tt := ui.NewTester(a.view, 760, 800)
	tt.Frame()

	// 三枚 tab 在 760 宽下放不全，栏本身横向滚动；当前激活的那枚必须完整
	// 可见、关闭 × 可点 —— 右侧的路径与状态 chip 让位正是为了这个。
	closeRect, ok := tt.Find("关闭文件标签页 router.go")
	if !ok {
		t.Fatalf("界面上找不到当前文件 tab 的关闭按钮：%q", tt.Texts())
	}
	if closeRect.W <= 0 || closeRect.H <= 0 {
		t.Errorf("窄窗口下当前文件 tab 的关闭按钮被挤没了：%+v", closeRect)
	}
	if closeRect.X < 0 || closeRect.X+closeRect.W > 760.5 {
		t.Errorf("当前文件 tab 的关闭按钮不在窗口里：x=%.1f w=%.1f", closeRect.X, closeRect.W)
	}
}

// 会话多到超过栏宽时，清屏/重启仍要留在窗口里：芯片带自己横向滚动，
// 动作键固定在右端。
func TestTerminalActionsStayVisibleWithManySessions(t *testing.T) {
	stubTerminal(t)
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeTerm)
	tab := a.currentTab()
	if tab == nil {
		t.Fatal("前置条件：该有一个项目")
	}
	for i := 0; i < 8; i++ {
		a.newTerminalSession(tab)
	}

	tt := ui.NewTester(a.view, 900, 800)
	tt.Frame()

	clear, okClear := tt.Find("清屏")
	restart, okRestart := tt.Find("重启")
	if !okClear || !okRestart {
		t.Fatalf("找不到清屏/重启：%q", tt.Texts())
	}
	for name, r := range map[string]ui.Rect{"清屏": clear, "重启": restart} {
		if r.W <= 0 || r.H <= 0 {
			t.Errorf("%s 被挤出栏外（零宽）：%+v", name, r)
		}
		if r.X < 0 || r.X+r.W > 900.5 {
			t.Errorf("%s 不在窗口里：x=%.1f w=%.1f", name, r.X, r.W)
		}
	}
	a.closeAllTerminals()
}

// 关掉活动项目、落到一个还没加载过的项目上时，要像 switchTab 一样把它
// 打开 —— 否则界面停在空状态，保存按钮亮着但 startOp 拒绝执行（静默失败）。
func TestCloseActiveProjectLoadsIdleNeighbor(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	first := a.repoPath

	other := newRepoDir(t)
	a.openInNewTab(other)
	settle(t, a)

	a.switchTab(0)
	settle(t, a)
	// 复刻「在别的项目里保存过」留下的快照：项目还在，但整体状态要重读。
	// 直接写快照，走 switchTab 会被 saveActiveTab 用当前活状态盖回去。
	a.switchTab(1)
	settle(t, a)
	a.tabs[0].state = loadIdle

	a.closeTab(1)
	settle(t, a)

	if a.repoPath != first {
		t.Fatalf("该落到 %q，实际 %q", first, a.repoPath)
	}
	if a.state != loadReady {
		t.Errorf("落到未加载的项目上该自动打开，state = %v", a.state)
	}
}
