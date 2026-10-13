package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// ⌘K 打开命令栏，Esc 关掉。
func TestPaletteOpensAndCloses(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !a.palette.on {
		t.Fatal("⌘K 应该打开命令栏")
	}
	if !tt.HasText("打开仓库…") || !tt.HasText("查看历史") {
		t.Errorf("命令栏没画出来：%q", tt.Texts())
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.palette.on {
		t.Error("Esc 应该关掉命令栏")
	}
}

// 打字按模糊匹配筛命令。
func TestPaletteFiltersCommands(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	tt.Type("历史")
	tt.Frame()

	cmds := a.matchedCommands()
	if len(cmds) != 1 || cmds[0].label != "查看历史" {
		t.Fatalf("匹配结果 = %+v", cmds)
	}
	if !tt.HasText("查看历史") {
		t.Errorf("筛选后的命令没画出来：%q", tt.Texts())
	}
	// 工具栏上也有个「打开仓库…」按钮，拿只有命令栏里才有的那条来验
	if tt.HasText("查看改动") {
		t.Errorf("不匹配的命令不该还留着：%q", tt.Texts())
	}
}

// 没有匹配时给提示，而不是一片空白。
func TestPaletteNoMatch(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.openPalette()
	a.palette.query = "zzzznothing"
	tt.Frame()

	if !tt.HasText("没有匹配的命令") {
		t.Errorf("无匹配时应给出提示：%q", tt.Texts())
	}
}

// 回车执行选中的那条，命令栏随即收起。
func TestPaletteRunsCommand(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.openPalette()
	a.palette.query = "查看历史"
	tt.Frame()

	tt.Key(0, ui.KeyEnter)
	settle(t, a)
	tt.Frame()

	if a.palette.on {
		t.Error("执行之后命令栏应该收起来")
	}
	if a.tab != 1 {
		t.Error("选中的是「查看历史」，应该切到 tab 1")
	}
}

// 上下键换选中项，选中的那条回车执行。
func TestPaletteArrowKeys(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.openPalette()
	tt.Frame()
	if a.palette.sel != 0 {
		t.Fatalf("初始应该选第一条，实际 %d", a.palette.sel)
	}

	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.palette.sel != 1 {
		t.Fatalf("下键之后应该选第二条，实际 %d", a.palette.sel)
	}

	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.palette.sel != 0 {
		t.Errorf("上键到头应该停在第一条，实际 %d", a.palette.sel)
	}
}

// 选中项只认键盘：点某一行就直接执行那一行。
func TestPaletteClickRunsRow(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.openPalette()
	a.palette.query = "历史"
	tt.Frame()

	if err := tt.Click("查看历史"); err != nil {
		t.Fatalf("点命令：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.tab != 1 {
		t.Errorf("点了「查看历史」应该切到历史 tab，实际 %d", a.tab)
	}
}

// 命令的可用状态跟着 app 走：「隐藏侧栏」和「显示侧栏」不会同时出现。
func TestPaletteCommandsFollowState(t *testing.T) {
	a := richApp(t)

	a.sidebarOn = true
	if !hasCommand(a.commands(), "隐藏侧栏") || hasCommand(a.commands(), "显示侧栏") {
		t.Error("侧栏开着时应该只有「隐藏侧栏」")
	}

	a.sidebarOn = false
	if hasCommand(a.commands(), "隐藏侧栏") || !hasCommand(a.commands(), "显示侧栏") {
		t.Error("侧栏收起时应该只有「显示侧栏」")
	}
}

func hasCommand(cmds []command, label string) bool {
	for _, c := range cmds {
		if c.label == label {
			return true
		}
	}
	return false
}

// 命令栏开着时窗口级快捷键让位，不会被底下吃掉。
func TestPaletteBlocksShortcuts(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.openPalette()
	tt.Frame()
	a.tab = 0

	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if a.tab != 0 {
		t.Error("命令栏开着时 ⌘2 不该切 tab")
	}
	if !a.palette.on {
		t.Error("命令栏应该还开着")
	}
}

// 命令栏里的每一条都真的做了那件事。这里挑「筛选文件」验：它和 ⌘P 走同一条
// 路（一次性的 focusFilter 请求），而焦点是不是真的落进筛选框，只有把字打
// 进去才看得出来 —— 命令栏自己也在抢焦点，关掉的那一帧未必交得出来。
//
// 用键盘走，不点行：文件栏那个筛选框的 Label 也叫「筛选文件」（命令名就该
// 是它要聚焦的那个东西的名字），而 Tester.Find 按渲染顺序取第一个「文字或
// Label」匹配 —— 框在底层先渲染，点到的是它，落在对话框的遮罩上，只是把
// 命令栏关掉。何况命令栏本来就是键盘的东西。
func TestPaletteFilterCommandFocusesInput(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Frame()
	a.openPalette()
	a.palette.query = "筛选"
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	settle(t, a)
	tt.Frame()

	if a.palette.on {
		t.Fatal("选中一条命令之后命令栏该关掉")
	}
	tt.Type("handler")
	tt.Frame()
	if a.fileFilter != "handler" {
		t.Errorf("字没进文件筛选框，fileFilter = %q", a.fileFilter)
	}
}
