package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func TestTabCreationAndSwitching(t *testing.T) {
	a := newApp()
	repo1 := filepath.Join(t.TempDir(), "repo1")
	repo2 := filepath.Join(t.TempDir(), "repo2")

	a.openInNewTab(repo1)
	if len(a.tabs) != 1 {
		t.Fatalf("预期 1 个标签页，实际 %d", len(a.tabs))
	}
	if a.activeTab != 0 {
		t.Errorf("预期当前激活第 0 个标签页，实际 %d", a.activeTab)
	}

	a.openInNewTab(repo2)
	if len(a.tabs) != 2 {
		t.Fatalf("预期 2 个标签页，实际 %d", len(a.tabs))
	}
	if a.activeTab != 1 {
		t.Errorf("预期当前激活第 1 个标签页，实际 %d", a.activeTab)
	}

	// 切换回第 0 个
	a.switchTab(0)
	if a.activeTab != 0 {
		t.Errorf("switchTab(0) 后应激活第 0 个，实际 %d", a.activeTab)
	}

	// 再次打开已存在的 repo1，应直接激活已有 tab 而不是新建
	a.openInNewTab(repo1)
	if len(a.tabs) != 2 {
		t.Fatalf("重复打开已有仓库不应新建 tab，预期 2 个，实际 %d", len(a.tabs))
	}
	if a.activeTab != 0 {
		t.Errorf("重复打开已存在的 repo1 应切回第 0 个标签页，实际 %d", a.activeTab)
	}
}

func TestTabCycle(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	a.openInNewTab("/tmp/proj2")
	a.openInNewTab("/tmp/proj3")

	if a.activeTab != 2 {
		t.Fatalf("当前应激活最后新开的第 2 个 tab，实际 %d", a.activeTab)
	}

	a.nextTab() // 循环回到 0
	if a.activeTab != 0 {
		t.Errorf("nextTab 预期切到 0，实际 %d", a.activeTab)
	}

	a.prevTab() // 循环回到 2
	if a.activeTab != 2 {
		t.Errorf("prevTab 预期切到 2，实际 %d", a.activeTab)
	}
}

func TestTabClosing(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	a.openInNewTab("/tmp/proj2")
	a.openInNewTab("/tmp/proj3")

	// 当前在 tab 2，关闭 tab 2 后应激活 tab 1
	a.closeTab(2)
	if len(a.tabs) != 2 {
		t.Fatalf("关闭后预期剩余 2 个 tab，实际 %d", len(a.tabs))
	}
	if a.activeTab != 1 {
		t.Errorf("关闭当前 tab 后预期激活第 1 个，实际 %d", a.activeTab)
	}

	// 关闭剩下的全部 tab
	a.closeTab(0)
	a.closeTab(0)
	if len(a.tabs) != 1 {
		t.Fatalf("关闭所有 tab 后应保留一个空兜底 tab，实际 %d", len(a.tabs))
	}
	if a.repoPath != "" {
		t.Errorf("清空所有 tab 后 repoPath 应为空，实际 %q", a.repoPath)
	}
}

func TestViewModeToggle(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")

	if a.viewMode != viewModeGit {
		t.Errorf("默认视图模式应为 Git GUI (0)，实际 %d", a.viewMode)
	}

	a.toggleViewMode()
	if a.viewMode != viewModeTerm {
		t.Errorf("toggleViewMode 后应为终端模式 (1)，实际 %d", a.viewMode)
	}

	tab := a.currentTab()
	if tab == nil || tab.viewMode != viewModeTerm {
		t.Errorf("标签页内部视图模式应同步为终端模式")
	}

	a.toggleViewMode()
	if a.viewMode != viewModeGit {
		t.Errorf("再次 toggleViewMode 应切回 Git GUI 模式 (0)，实际 %d", a.viewMode)
	}
}

// stubTerminal 把「起 shell」换成替身。
//
// 会话列表的增、删、切换是纯逻辑：为它每次真起一个 shell，既慢，又要在
// 用例结束之后照顾一堆 SIGHUP 还没走完的子进程。替身不碰 pty，只记一笔
// 「有人要起终端了」—— 用例要断言的正是这一步有没有发生、发生了几次。
func stubTerminal(t *testing.T) *int {
	t.Helper()
	calls := new(int)
	old := terminalNew
	terminalNew = func(terminal.Options) (*terminal.Terminal, error) {
		*calls++
		return nil, nil
	}
	t.Cleanup(func() { terminalNew = old })
	return calls
}

// TestTerminalSessions 走一遍终端会话的增、删、切换，以及「关掉最后一个」。
func TestTerminalSessions(t *testing.T) {
	spawns := stubTerminal(t)
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	tab := a.currentTab()
	tt := ui.NewTester(a.view, 1000, 700)

	// Git 面板里不该有 shell：终端是进了终端视图才有的东西。
	tt.Frame()
	if len(tab.terms) != 0 || *spawns != 0 {
		t.Fatalf("没进终端视图就起了会话：%d 个，起了 %d 次", len(tab.terms), *spawns)
	}

	// 进终端视图：自动补一个会话，并在这一帧把它起起来。
	a.setViewMode(viewModeTerm)
	tt.Frame()
	if len(tab.terms) != 1 || tab.termActive != 0 {
		t.Fatalf("进终端视图该有一个会话：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}
	if *spawns != 1 {
		t.Fatalf("进终端视图该起一次 shell，实际 %d 次", *spawns)
	}
	if !tt.HasText("终端 1") {
		t.Errorf("空标题该回落到「终端 1」，实际 %q", tt.Texts())
	}

	// 多渲染几帧不该重复起：一枚芯片就是一个 shell。
	tt.Frame()
	tt.Frame()
	if *spawns != 1 {
		t.Errorf("渲染多帧只该起一次 shell，实际 %d 次", *spawns)
	}

	// ⌘⇧T：再开一个，并切到新的那个。
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	if len(tab.terms) != 2 || tab.termActive != 1 {
		t.Fatalf("⌘⇧T 该再开一个会话并切过去：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}
	if *spawns != 2 {
		t.Errorf("新会话该起一次 shell，实际共 %d 次", *spawns)
	}
	if !tt.HasText("终端 2") {
		t.Errorf("第二枚芯片该是「终端 2」，实际 %q", tt.Texts())
	}

	// 芯片带右边那枚 + 是同一个动作。
	if err := tt.Click("新建终端标签页"); err != nil {
		t.Fatalf("芯片带上找不到新建按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 3 || tab.termActive != 2 {
		t.Fatalf("+ 该再开一个会话并切过去：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}

	// 点第一枚芯片切回第一个会话。
	if err := tt.Click("终端 1"); err != nil {
		t.Fatalf("找不到第一枚芯片：%v（%q）", err, tt.Texts())
	}
	if tab.termActive != 0 || a.activeSession() != tab.terms[0] {
		t.Fatalf("点芯片该切过去：激活 %d", tab.termActive)
	}

	// × 关掉第一枚：剩下的两枚各自留在原地，激活的跟到原来的第二个上。
	before := *spawns
	if err := tt.Click("关闭终端标签页"); err != nil {
		t.Fatalf("芯片上找不到关闭按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 2 {
		t.Fatalf("关掉一个后该剩两个会话，实际 %d", len(tab.terms))
	}
	if tab.terms[0].ord != 2 || tab.terms[1].ord != 3 {
		t.Fatalf("剩下的会话不该重编号：%d, %d", tab.terms[0].ord, tab.terms[1].ord)
	}
	if *spawns != before {
		t.Errorf("关会话不该顺手再起 shell，多起了 %d 次", *spawns-before)
	}

	// 再关两下：倒数第二个关掉之后剩一个，最后一个关掉时自动补一个新的。
	for range 2 {
		if err := tt.Click("关闭终端标签页"); err != nil {
			t.Fatalf("找不到关闭按钮：%v（%q）", err, tt.Texts())
		}
		if len(tab.terms) != 1 {
			t.Fatalf("关会话后该一直有且只有一个会话可用，实际 %d", len(tab.terms))
		}
	}
	if tab.terms[0].ord != 4 {
		t.Errorf("关掉最后一个该补一个编号继续往后发的新会话（ord 4），实际 %d", tab.terms[0].ord)
	}

	// 重启只换壳不换芯片：会话还是那一个，编号也不动。
	cur := a.activeSession()
	if cur == nil {
		t.Fatal("没有激活的会话")
	}
	before = *spawns
	a.restartCurrentTerminal()
	tt.Frame()
	if a.activeSession() != cur {
		t.Errorf("重启后该还是同一个会话")
	}
	if cur.ord != tab.terms[0].ord {
		t.Errorf("重启不该换编号：%d", cur.ord)
	}
	if *spawns != before+1 {
		t.Errorf("重启该重新起一次 shell，实际 %d 次", *spawns-before)
	}

	// 清屏只作用于当前会话，不碰列表。
	a.clearCurrentTerminal()
	if len(tab.terms) != 1 || a.activeSession() != cur {
		t.Errorf("清屏不该动会话列表：%d 个", len(tab.terms))
	}

	// 关窗 / 退出那条路：全放掉，不补。
	a.closeAllTerminals()
	if len(tab.terms) != 0 {
		t.Errorf("closeAllTerminals 后不该有会话，实际 %d", len(tab.terms))
	}
}
