package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestHistoryNavJK 测试使用 J / K 键盘导航历史提交列表。
func TestHistoryNavJK(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Fatalf("初始应该选中最新提交，实际 commitSel = %d", a.commitSel)
	}
	if !tt.HasText("b.txt") {
		t.Errorf("初始应该显示第 0 个提交（第三条）的 diff: %q", tt.Texts())
	}

	// J 移动到下一个（较旧的）提交
	tt.Key(0, ui.KeyJ)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 1 {
		t.Fatalf("按 J 之后 commitSel = %d, want 1", a.commitSel)
	}
	if !tt.HasText("two") {
		t.Errorf("主区应该显示第 1 个提交（第二条）的 diff: %q", tt.Texts())
	}

	// 再次按 J 移动到第 2 个提交（第一条）
	tt.Key(0, ui.KeyJ)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 2 {
		t.Fatalf("再次按 J 之后 commitSel = %d, want 2", a.commitSel)
	}
	if !tt.HasText("one") {
		t.Errorf("主区应该显示第 2 个提交（第一条）的 diff: %q", tt.Texts())
	}

	// 到底部后再按 J 应该被夹住，不超出范围
	tt.Key(0, ui.KeyJ)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 2 {
		t.Errorf("到底后再按 J 应该保持在 2，实际 commitSel = %d", a.commitSel)
	}

	// K 移动到上一个（较新的）提交
	tt.Key(0, ui.KeyK)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 1 {
		t.Fatalf("按 K 之后 commitSel = %d, want 1", a.commitSel)
	}
	if !tt.HasText("two") {
		t.Errorf("主区应该显示第 1 个提交（第二条）的 diff: %q", tt.Texts())
	}

	// 再次按 K 移动回最新提交
	tt.Key(0, ui.KeyK)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Fatalf("再次按 K 之后 commitSel = %d, want 0", a.commitSel)
	}
	if !tt.HasText("b.txt") {
		t.Errorf("主区应该恢复显示第 0 个提交（第三条）的 diff: %q", tt.Texts())
	}

	// 到顶后再按 K 应该被夹在 0
	tt.Key(0, ui.KeyK)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Errorf("到顶后再按 K 应该保持在 0，实际 commitSel = %d", a.commitSel)
	}
}

// TestHistoryNavArrows 测试使用 Down / Up 方向键导航历史提交列表。
func TestHistoryNavArrows(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Fatalf("初始 commitSel = %d, want 0", a.commitSel)
	}

	// Down 移动到下一个提交
	tt.Key(0, ui.KeyDown)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 1 {
		t.Fatalf("按 Down 之后 commitSel = %d, want 1", a.commitSel)
	}
	if !tt.HasText("two") {
		t.Errorf("按 Down 之后应该加载第 1 个提交的内容: %q", tt.Texts())
	}

	// 再次 Down
	tt.Key(0, ui.KeyDown)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 2 {
		t.Fatalf("再次按 Down 之后 commitSel = %d, want 2", a.commitSel)
	}

	// 到底后按 Down 夹在 2
	tt.Key(0, ui.KeyDown)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 2 {
		t.Errorf("到底后按 Down 应该保持在 2，实际 %d", a.commitSel)
	}

	// Up 移动到上一个提交
	tt.Key(0, ui.KeyUp)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 1 {
		t.Fatalf("按 Up 之后 commitSel = %d, want 1", a.commitSel)
	}

	// 再次 Up 回到 0
	tt.Key(0, ui.KeyUp)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Fatalf("再次按 Up 之后 commitSel = %d, want 0", a.commitSel)
	}

	// 到顶后按 Up 夹在 0
	tt.Key(0, ui.KeyUp)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Errorf("到顶后按 Up 应该保持在 0，实际 %d", a.commitSel)
	}
}

// TestHistoryNavMixedKeys 测试混用方向键与 J / K 键盘导航。
func TestHistoryNavMixedKeys(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	// Down 往下，K 往上
	tt.Key(0, ui.KeyDown)
	settle(t, a)
	tt.Frame()
	if a.commitSel != 1 {
		t.Fatalf("Down 后 commitSel = %d, want 1", a.commitSel)
	}

	tt.Key(0, ui.KeyK)
	settle(t, a)
	tt.Frame()
	if a.commitSel != 0 {
		t.Fatalf("K 后 commitSel = %d, want 0", a.commitSel)
	}

	// J 往下，Up 往上
	tt.Key(0, ui.KeyJ)
	settle(t, a)
	tt.Frame()
	if a.commitSel != 1 {
		t.Fatalf("J 后 commitSel = %d, want 1", a.commitSel)
	}

	tt.Key(0, ui.KeyUp)
	settle(t, a)
	tt.Frame()
	if a.commitSel != 0 {
		t.Fatalf("Up 后 commitSel = %d, want 0", a.commitSel)
	}
}

// TestHistoryNavFromStash 测试在查看储藏时按 J/Down 会切回提交并选中第一个提交。
func TestHistoryNavFromStash(t *testing.T) {
	a := stashApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 点一条储藏
	if err := tt.Click("stash@{0}"); err != nil {
		t.Fatalf("点储藏：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.stashRev == "" || a.commitSel != -1 {
		t.Fatalf("应该在看储藏，stashRev=%q commitSel=%d", a.stashRev, a.commitSel)
	}

	// 按 J 应该恢复提交列表导航，选中第 0 个提交
	tt.Key(0, ui.KeyJ)
	settle(t, a)
	tt.Frame()

	if a.stashRev != "" {
		t.Errorf("按 J 之后 stashRev 应该被清空，实际 %q", a.stashRev)
	}
	if a.commitSel != 0 {
		t.Errorf("按 J 之后应该选中第 0 个提交，实际 commitSel = %d", a.commitSel)
	}
}

// TestHistoryNavIgnoredWhenModalOrTyping 测试在打开对话框或在筛选框输入时忽略 J / K 导航。
func TestHistoryNavIgnoredWhenModalOrTyping(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	// 1. 打开命令栏模态对话框，按 J 不应改变选中提交
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !a.palette.on {
		t.Fatal("命令栏应该打开")
	}

	tt.Key(0, ui.KeyJ)
	settle(t, a)
	tt.Frame()
	if a.commitSel != 0 {
		t.Errorf("模态对话框打开时按 J 不该触发提交导航，commitSel = %d", a.commitSel)
	}

	// 关闭命令栏
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.palette.on {
		t.Fatal("命令栏应该已关闭")
	}

	// 2. 聚焦文件筛选框输入，打 j 应该进入输入框，不应导航提交
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()

	tt.TypeKey(0, ui.KeyJ, "j")
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Errorf("在筛选框打字时不该触发提交导航，commitSel = %d", a.commitSel)
	}
	if a.fileFilter != "j" {
		t.Errorf("输入应该进文件筛选框，fileFilter = %q", a.fileFilter)
	}
}

// TestHistoryNavEmptyRepo 测试空仓库中按 J / K / 方向键不崩溃。
func TestHistoryNavEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	// 空仓库没有提交，按键不能崩溃
	for _, k := range []ui.Key{ui.KeyJ, ui.KeyK, ui.KeyDown, ui.KeyUp} {
		tt.Key(0, k)
		tt.Frame()
	}

	if a.commitSel != -1 {
		t.Errorf("空仓库 commitSel 应为 -1，实际 %d", a.commitSel)
	}
}

// TestHistoryNavScrollsIntoView 测试长列表下键盘导航时 commitList 自动滚动保持选中项可见。
func TestHistoryNavScrollsIntoView(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(dir, "f.txt"), fmt.Sprintf("line %d\n", i))
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", fmt.Sprintf("commit %02d", i))
	}

	a := loadApp(t, dir)
	// 矮窗口确保无法一次容纳 20 个提交（commitH=30，列表高约 150）
	tt := ui.NewTester(a.view, 1280, 500)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.commitSel != 0 {
		t.Fatalf("commitSel = %d, want 0", a.commitSel)
	}

	// 连续向下导航 15 次
	for i := 0; i < 15; i++ {
		tt.Key(0, ui.KeyDown)
		settle(t, a)
		tt.Frame()
	}

	if a.commitSel != 15 {
		t.Fatalf("commitSel = %d, want 15", a.commitSel)
	}
	first, last := a.commitList.Visible()
	if a.commitSel < first || a.commitSel > last {
		t.Errorf("当前选中提交 %d 不在可视区 [%d, %d] 内", a.commitSel, first, last)
	}
}

