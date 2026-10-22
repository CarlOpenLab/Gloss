package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// J / K 在这一个文件里一段一段走，走到头绕回另一端。
//
// 段是 DiffView 算的，Gloss 只记「上次跳到第几段」—— 所以这里盯的是下标
// 的推进和绕回，渲染位置由 mygo 管。
func TestJumpHunkCycles(t *testing.T) {
	a := richApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "internal/server/handler.go")))
	settleReview(t, a)

	n := len(a.review.diff.Hunks())
	if n < 3 {
		t.Fatalf("夹具该有三段以上改动，实际 %d", n)
	}
	if a.review.hunkIdx != -1 {
		t.Fatalf("刚装上时该是「还没跳过」，实际 %d", a.review.hunkIdx)
	}

	// 还没跳过时往下：落到第一段（不是第二段）。
	a.jumpHunk(1)
	if a.review.hunkIdx != 0 {
		t.Errorf("第一次按 J 该到第一段，实际 %d", a.review.hunkIdx)
	}
	// 一段一段往前走，走到头绕回第一段。
	var visited []int
	for i := 0; i < n; i++ {
		a.jumpHunk(1)
		visited = append(visited, a.review.hunkIdx)
	}
	if visited[n-1] != 0 {
		t.Errorf("走完一圈该绕回第一段，实际路径 %v", visited)
	}
	// 往回走一段：从上一步的 0 退到最后一段。
	a.jumpHunk(-1)
	if a.review.hunkIdx != n-1 {
		t.Errorf("在第一段往上该绕到最后一段（%d），实际 %d", n-1, a.review.hunkIdx)
	}
}

// 还没跳过时往上：落到最后一段。
func TestJumpHunkBackwardsFromTop(t *testing.T) {
	a := richApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "internal/server/handler.go")))
	settleReview(t, a)

	n := len(a.review.diff.Hunks())
	a.jumpHunk(-1)
	if a.review.hunkIdx != n-1 {
		t.Errorf("还没跳过时按 K 该落到最后一段（%d），实际 %d", n-1, a.review.hunkIdx)
	}
}

// 没有改动、没有仓库、没有选中文件时跳转不能崩。
func TestJumpHunkEmpty(t *testing.T) {
	a := newApp()
	a.jumpHunk(1)
	a.jumpHunk(-1)
	if a.review.key != "" || a.review.diff != nil {
		t.Errorf("没有仓库时改动区该是空的：key=%q diff=%v", a.review.key, a.review.diff)
	}
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

	// ⌘⇧= 和 ⌘= 是同一个意思：键盘上的加号本来就是 ⇧=，
	// 只认一个的话，用户照着菜单上写的「⌘+」按下去不会有反应。
	// （此时字号刚被 ⌘− 两次压到 defSize−1。）
	tt.Key(ui.Cmd|ui.Shift, ui.KeyEqual)
	tt.Frame()
	if got := a.codeSizeValue(); got != defSize {
		t.Errorf("⌘⇧= 之后 = %v, want %v", got, defSize)
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
//
// 判据用「本地分支」这个分组名，不能用「Total: +N −M」—— 那是文件栏的
// 底部统计，布局改成 Fork 那样之后它搬去主区下半了，收起侧栏收不掉它。
func TestSidebarToggleShortcut(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	// 收放是一条 260ms 的曲线，而无头渲染每帧的耗时取决于这一帧画了
	// 多少东西；只画一帧就断言，等于拿「渲染快不快」赌动画走没走完。
	// 这个用例验的是状态和落盘，不是曲线，所以关掉动效让它一步到位。
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})

	if !a.sidebarOn {
		t.Fatal("侧栏默认是开着的")
	}
	if !tt.HasText("本地分支") {
		t.Fatalf("侧栏内容没画出来：%q", tt.Texts())
	}

	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	tt.Frame()
	if a.sidebarOn {
		t.Fatal("⌘⇧B 应该收起侧栏")
	}
	if tt.HasText("本地分支") {
		t.Errorf("侧栏收起后不该还有分支树：%q", tt.Texts())
	}

	settle(t, a)
	if !a.store.Config().SidebarOff {
		t.Error("侧栏状态应该记进配置")
	}
}

// ⌘F 把焦点放进主区的文件筛选框，不打侧栏的主意。
//
// 这里原来还断言「顺手把侧栏打开」—— 侧栏的筛选框改成筛分支之后，
// 「找文件」和侧栏开不开就没关系了。
func TestFilterFocusesFilter(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.sidebarOn = false
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Frame()

	if a.sidebarOn {
		t.Error("⌘F 不该把侧栏拽开")
	}
	// 焦点在文件筛选框里：直接打进去的字应该进 fileFilter。
	// 占位符不是文本元素，测不了「焦点在哪」，但字打进去就说明焦点是对的。
	tt.Type("handler")
	tt.Frame()
	if a.fileFilter != "handler" {
		t.Errorf("输入没进文件筛选框，fileFilter = %q", a.fileFilter)
	}
	if a.refFilter != "" {
		t.Errorf("侧栏的筛选框不该收到这些字，refFilter = %q", a.refFilter)
	}
	// 筛的是文件，不是改动区本身：⌘P 只改 fileFilter，别的不动。
	if a.fileSel != -1 {
		t.Errorf("筛文件不该顺手动选中，fileSel = %d", a.fileSel)
	}
}

// 焦点在文件筛选框里时打 J 只是打了一个字母，不该跳改动段。
//
// 跳段的键在窗口这一层（c.Shortcut），而输入框会先把键吃掉 —— 这条盯着
// 「打字不会被当成快捷键」这件事没被改坏。
func TestJumpHunkIgnoredWhileTyping(t *testing.T) {
	a := richApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "internal/server/handler.go")))
	tt := settleReview(t, a)

	tt.Key(ui.Cmd, ui.KeyP) // 焦点给文件筛选框
	tt.Frame()
	a.review.hunkIdx = 0

	tt.TypeKey(0, ui.KeyJ, "j")
	tt.Frame()
	if a.review.hunkIdx != 0 {
		t.Errorf("在筛选框里打 j 不该跳段，hunkIdx = %d", a.review.hunkIdx)
	}
	if a.fileFilter != "j" {
		t.Errorf("j 该打进筛选框，fileFilter = %q", a.fileFilter)
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

// ⌘, 开的是模型设置 —— macOS 上这个键就是「偏好设置」。
//
// 它原来接在放大字号上（⌘= 那一路顺手多认了一个键），于是按 Mac 的习惯
// 想开设置，得到的却是代码大了一号。这条测试两头都盯着：设置要开，
// 字号要一动不动。
func TestSettingsShortcut(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if !a.settingsOn {
		t.Fatal("⌘, 该打开模型设置")
	}
	if got := a.codeSizeValue(); got != defSize {
		t.Errorf("⌘, 之后字号 = %v, want 不动（%v）", got, defSize)
	}
}

// 没有打开仓库时按这些键不能崩。
func TestShortcutsWithoutRepo(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1280, 860)

	// ⌘, 不依赖仓库，单独先试 —— 它开出来的对话框是模态的，会把后面
	// 这一串键全挡在外面，所以先问完再收起来。
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if !a.settingsOn {
		t.Fatal("没有仓库时 ⌘, 也该打开模型设置")
	}
	a.settingsOn = false
	tt.Frame()

	for _, c := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{
		{ui.Cmd, ui.KeyR}, {ui.Cmd, ui.KeyF}, {ui.Cmd, ui.KeyK},
		{ui.Cmd, ui.Key1}, {ui.Cmd, ui.Key2}, {ui.Cmd, ui.KeyEqual},
		{ui.Cmd | ui.Shift, ui.KeyEqual},
		{ui.Cmd, ui.KeyMinus}, {ui.Cmd, ui.Key0},
		// ⌘P 是筛文件，⌘G / ⌘⇧G 是「上/下一条命中」，Esc 收查找栏。
		{ui.Cmd, ui.KeyP}, {ui.Cmd, ui.KeyG}, {ui.Cmd | ui.Shift, ui.KeyG},
		{0, ui.KeyEscape},
		{0, ui.KeyJ}, {0, ui.KeyK},
		{0, ui.KeyDown}, {0, ui.KeyUp},
	} {
		tt.Key(c.mods, c.key)
		tt.Frame()
	}

	// 没有仓库、没有文件：这些键都不该把状态搅乱。
	if len(a.files) != 0 || a.review.key != "" {
		t.Errorf("没有仓库时不该有文件和改动区状态：files=%d review=%q",
			len(a.files), a.review.key)
	}
}
