package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// J/K 找的是下一段/上一段改动，走到头绕到另一端。
func TestHunkRowNear(t *testing.T) {
	// H@L@L@：文件头 + 三段改动
	rows := []row{
		{kind: rowFileHeader},
		{kind: rowHunkHeader},
		{kind: rowLine},
		{kind: rowHunkHeader},
		{kind: rowLine},
		{kind: rowHunkHeader},
	}
	cases := []struct {
		from, dir, want int
		why             string
	}{
		{0, 1, 1, "从文件头往下是第一段"},
		{1, 1, 3, "从第一段往下是第二段"},
		{3, 1, 5, "从第二段往下是第三段"},
		{5, 1, 1, "最后一段再往下绕回第一段"},
		{0, -1, 5, "在最上面往上绕到最后一段"},
		{5, -1, 3, "从第三段往上是第二段"},
		{1, -1, 5, "第一段往上到头了，绕回最后一段"},
	}
	for _, c := range cases {
		if got := hunkRowNear(rows, c.from, c.dir); got != c.want {
			t.Errorf("hunkRowNear(rows, %d, %d) = %d, want %d（%s）",
				c.from, c.dir, got, c.want, c.why)
		}
	}

	// 没有改动时返回 -1，不能崩
	if got := hunkRowNear([]row{{kind: rowFileHeader}}, 0, 1); got != -1 {
		t.Errorf("没有 hunk 时 = %d, want -1", got)
	}
	if got := hunkRowNear(nil, 0, -1); got != -1 {
		t.Errorf("空列表 = %d, want -1", got)
	}
}

// J 真的会把列表滚下去：按一下之后，原本在屏幕外的改动看得见了。
func TestJumpHunkScrolls(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 30 行的小窗口，后面几段改动一开始肯定在屏幕外
	tt.SetSize(1280, 320)
	tt.Frame()

	const last = "line 60: some code here"
	if tt.HasText(last) {
		t.Skip("窗口里能看到最后一段，滚动的效果测不出来")
	}

	a.diffList.ScrollTo(0, ui.Start)
	tt.Frame()
	for i := 0; i < 8 && !tt.HasText(last); i++ {
		a.jumpHunk(1)
		tt.Frame()
	}
	if !tt.HasText(last) {
		t.Errorf("一路 J 应该能走到最后一段改动：%q", tt.Texts())
	}
}

// 连着按 J 要一段一段往前走，走到头绕回第一段。
//
// 回归测试：视口不一定跟得上跳转 —— 文件头钉在列表顶上时可见区第一行
// 永远是它，diff 滚到底之后视口干脆不动。只看 Visible() 的话每次都算出
// 同一个目标，J 就卡在原地了。
func TestJumpHunkAdvances(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	var visited []string
	for i := 0; i < 5; i++ {
		a.jumpHunk(1)
		tt.Frame()
		visited = append(visited, a.hunkCur)
	}

	seen := map[string]bool{}
	for _, k := range visited {
		if k == "" {
			t.Fatal("跳转之后应该记下跳到了哪一段")
		}
		seen[k] = true
	}
	if len(seen) < 3 {
		t.Errorf("连按五次 J 只到过 %d 段，应该一段一段往前走：%v", len(seen), visited)
	}
	// 这个仓库有四段改动（含开头那一段），第五次应该已经绕回来了
	if visited[4] != visited[0] {
		t.Errorf("走到头应该绕回来，实际 %v", visited)
	}
}

// 没有改动时跳转不能崩。
func TestJumpHunkEmpty(t *testing.T) {
	a := newApp()
	a.jumpHunk(1)
	a.jumpHunk(-1)
}

// 字号：⌘+/⌘−/⌘0，并且夹在上下限内。
func TestCodeSizeShortcuts(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if got := a.codeSizeValue(); got != defSize {
		t.Fatalf("默认字号 = %v, want %v", got, defSize)
	}

	tt.Key(ui.Cmd, ui.KeyEqual)
	tt.Frame()
	if got := a.codeSizeValue(); got != defSize+codeSizeStep {
		t.Errorf("⌘+ 之后 = %v, want %v", got, defSize+codeSizeStep)
	}

	tt.Key(ui.Cmd, ui.KeyMinus)
	tt.Key(ui.Cmd, ui.KeyMinus)
	tt.Frame()
	if got := a.codeSizeValue(); got != defSize-codeSizeStep {
		t.Errorf("⌘− 两次之后 = %v, want %v", got, defSize-codeSizeStep)
	}

	tt.Key(ui.Cmd, ui.Key0)
	tt.Frame()
	if a.codeSizeValue() != defSize {
		t.Errorf("⌘0 之后 = %v, want 默认值 %v", a.codeSizeValue(), defSize)
	}

	// 一路按到底也不能超出范围
	for i := 0; i < 40; i++ {
		a.setCodeSize(a.codeSize + codeSizeStep)
	}
	if got := a.codeSizeValue(); got != codeSizeMax {
		t.Errorf("按到底 = %v, want 上限 %v", got, codeSizeMax)
	}
	for i := 0; i < 80; i++ {
		a.setCodeSize(a.codeSize - codeSizeStep)
	}
	if got := a.codeSizeValue(); got != codeSizeMin {
		t.Errorf("缩到底 = %v, want 下限 %v", got, codeSizeMin)
	}
}

// 字号要落进配置，下次启动才恢复得了。
func TestCodeSizePersists(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Key(ui.Cmd, ui.KeyEqual)
	settle(t, a)

	if got := a.store.Config().CodeSize; got != defSize+codeSizeStep {
		t.Errorf("配置里的字号 = %v, want %v", got, defSize+codeSizeStep)
	}
}

// ⌘⇧B 收放侧栏，状态同样要记下来。
func TestSidebarToggleShortcut(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if !a.sidebarOn {
		t.Fatal("侧栏默认是开着的")
	}
	if !tt.HasText("Total: ") {
		t.Fatalf("侧栏内容没画出来：%q", tt.Texts())
	}

	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	tt.Frame()
	if a.sidebarOn {
		t.Fatal("⌘⇧B 应该收起侧栏")
	}
	if tt.HasText("Total: ") {
		t.Errorf("侧栏收起后不该还有底部统计：%q", tt.Texts())
	}

	settle(t, a)
	if !a.store.Config().SidebarOff {
		t.Error("侧栏状态应该记进配置")
	}
}

// ⌘F 把焦点放进筛选框，收起侧栏时顺手把它打开。
func TestFindFocusesFilter(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.sidebarOn = false
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()

	if !a.sidebarOn {
		t.Error("⌘F 应该把侧栏打开")
	}
	// 焦点在筛选框里：直接打进去的字应该进 filter。
	// 占位符不是文本元素，测不了「焦点在哪」，但字打进去就说明焦点是对的。
	tt.Type("handler")
	tt.Frame()
	if a.filter != "handler" {
		t.Errorf("输入没进筛选框，filter = %q", a.filter)
	}
}

// 焦点在筛选框里时打 J 只是打了一个字母，不该跳 hunk。
func TestJumpHunkIgnoredWhileTyping(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()

	a.diffList.ScrollTo(0, ui.Start)
	tt.TypeKey(0, ui.KeyJ, "j")
	tt.Frame()

	first, _ := a.diffList.Visible()
	if first > 0 {
		t.Errorf("在筛选框里打 j 不该跳 hunk，可视区从第 %d 行开始了", first)
	}
	if a.filter != "j" {
		t.Errorf("j 应该打进了筛选框，filter = %q", a.filter)
	}
}

// ⌘1 / ⌘2 切 tab。
func TestTabShortcuts(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Key(ui.Cmd, ui.Key2)
	settle(t, a)
	tt.Frame()
	if a.tab != 1 {
		t.Fatalf("⌘2 之后 tab = %d, want 1", a.tab)
	}
	if !tt.HasText("3 个提交") {
		t.Errorf("历史 tab 没画出来：%q", tt.Texts())
	}

	tt.Key(ui.Cmd, ui.Key1)
	settle(t, a)
	tt.Frame()
	if a.tab != 0 {
		t.Errorf("⌘1 之后 tab = %d, want 0", a.tab)
	}
}

// 没有打开仓库时按这些键不能崩。
func TestShortcutsWithoutRepo(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1280, 860)

	for _, c := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{
		{ui.Cmd, ui.KeyR}, {ui.Cmd, ui.KeyF}, {ui.Cmd, ui.KeyK},
		{ui.Cmd, ui.Key1}, {ui.Cmd, ui.Key2}, {ui.Cmd, ui.KeyEqual},
		{ui.Cmd, ui.KeyMinus}, {ui.Cmd, ui.Key0},
		{0, ui.KeyJ}, {0, ui.KeyK},
	} {
		tt.Key(c.mods, c.key)
		tt.Frame()
	}
}
