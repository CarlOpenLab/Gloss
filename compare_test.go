package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// compareApp 创建一个带两条分支（main 和 feature）的测试应用。
func compareApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := newRepoDir(t)

	// 1. main 上的初次提交
	writeFile(t, filepath.Join(dir, "common.txt"), "hello base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init commit")

	// 2. 切出 feature 分支并提交改动
	runGit(t, dir, "branch", "feature")

	// 3. main 分支单独加一个提交
	writeFile(t, filepath.Join(dir, "main_only.txt"), "only in main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "main update")

	// 4. feature 分支加文件
	runGit(t, dir, "checkout", "feature")
	writeFile(t, filepath.Join(dir, "feature.txt"), "line 1\nline 2\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feature commit")

	// 5. 切回 main 并留下工作区未提交改动
	runGit(t, dir, "checkout", "main")
	writeFile(t, filepath.Join(dir, "worktree.txt"), "uncommitted\n")

	a := loadApp(t, dir)
	return a, dir
}

// 分支内容对比全流程测试（Fork 风格）：开启对比、查看差异、交换方向、退出恢复。
func TestBranchCompareFlow(t *testing.T) {
	a, _ := compareApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 初始状态：工作区有 1 个未提交文件（worktree.txt）
	if len(a.files) != 1 || a.files[0].Path() != "worktree.txt" {
		t.Fatalf("初始工作区文件异常：%+v", a.files)
	}

	// 1. 开启对比：main vs feature（两端内容对比 base..target）
	a.startCompare("main", "feature")
	settle(t, a)
	tt.Frame()

	if !a.compare.on {
		t.Fatal("对比模式应该处于开启状态")
	}
	if a.compare.state != loadReady {
		t.Fatalf("对比加载状态 = %v, want loadReady, err=%s", a.compare.state, a.compare.err)
	}
	if !tt.HasText("分支对比") {
		t.Errorf("界面应该显示「分支对比」标题：%q", tt.Texts())
	}
	if !tt.HasText("退出对比") {
		t.Errorf("界面应该有「退出对比」按钮：%q", tt.Texts())
	}
	// 两端对比包含 feature.txt（新增）和 main_only.txt（删除）。
	if len(a.compare.files) != 2 {
		t.Fatalf("两端对比应该有 2 个文件, 实际为：%+v", a.compare.files)
	}
	// 主区一动不动：还停在进来之前的工作区改动上（见 TestCompareModalKeepsMainPageSelection）。
	if len(a.files) != 1 || a.files[0].Path() != "worktree.txt" {
		t.Fatalf("对比开着的时候主区该还是工作区改动，实际 %+v", a.files)
	}
	if len(a.compare.commits) != 1 || a.compare.commits[0].Subject != "feature commit" {
		t.Fatalf("对比提交列表异常：%+v", a.compare.commits)
	}

	// 2. 交换对比方向 ⇄
	a.swapCompare()
	settle(t, a)
	tt.Frame()

	if a.compare.base != "feature" || a.compare.target != "main" {
		t.Fatalf("交换后 base=%s, target=%s", a.compare.base, a.compare.target)
	}

	// 3. 退出对比模式
	if err := tt.Click("退出对比"); err != nil {
		t.Fatalf("点击退出对比失败：%v", err)
	}
	tt.Frame()

	if a.compare.on {
		t.Fatal("点击退出对比后应该已退出")
	}
	// 工作区改动从没被换掉，退出只是把弹窗收起来。
	if len(a.files) != 1 || a.files[0].Path() != "worktree.txt" {
		t.Fatalf("退出对比后应该还是工作区文件：%+v", a.files)
	}
}

// 对比改成弹窗之后，主区的那一份选中不能被它动：用户在弹窗里点来点去，
// 关上之后该回到原来在看哪儿。
//
// 这是「不要影响到主页面的交互」那一句的落点 —— 弹窗读的是自己的一份 sel，
// 不是跟着 tab 快照走的 a.fileSel。
func TestCompareModalKeepsMainPageSelection(t *testing.T) {
	a, _ := compareApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 主区先选中一个文件
	a.fileSel = 0
	mainSel, mainFollow := a.fileSel, a.filesList.FollowEnd

	a.startCompare("main", "feature")
	settle(t, a)
	tt.Frame()

	// 弹窗里点第二个文件（feature.txt / main_only.txt 那两行）
	if err := tt.Click("main_only.txt"); err != nil {
		t.Fatalf("弹窗文件栏里点 main_only.txt 失败：%v", err)
	}
	tt.Frame()

	if a.compare.sel == mainSel {
		t.Errorf("弹窗里点了文件，a.compare.sel 该跟着动，还是 %d", a.compare.sel)
	}
	if a.fileSel != mainSel {
		t.Errorf("弹窗选中把主区的 fileSel 动了：%d → %d", mainSel, a.fileSel)
	}
	if a.filesList.FollowEnd != mainFollow {
		t.Errorf("弹窗选中把主区的列表状态动了：%v → %v", mainFollow, a.filesList.FollowEnd)
	}

	// 关掉弹窗：主区还是原来那个文件、原来那个滚动位置
	if err := tt.Click("退出对比"); err != nil {
		t.Fatalf("点击退出对比失败：%v", err)
	}
	tt.Frame()

	if a.fileSel != mainSel || a.filesList.FollowEnd != mainFollow {
		t.Errorf("退出后主区的选中该原样：fileSel %d→%d, FollowEnd %v→%v",
			mainSel, a.fileSel, mainFollow, a.filesList.FollowEnd)
	}
}

// 对比对话框直接按两端内容对比启动，不再提供三点/PR 模式选项。
func TestCompareDialogStartsContentDiff(t *testing.T) {
	a, _ := compareApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	a.openCompareDialog("feature")
	tt.Frame()

	if tt.HasText("PR 模式") || tt.HasText("对比模式") {
		t.Fatalf("对话框里不该再有模式选择：%q", tt.Texts())
	}

	if err := tt.Click("开始对比"); err != nil {
		t.Fatalf("点击开始对比失败：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if len(a.compare.files) != 2 {
		t.Fatalf("两端对比应该有 2 个文件, 实际为：%+v", a.compare.files)
	}
}

// 侧栏右键菜单中包含「与当前分支 (main) 对比代码差异」。
func TestBranchCompareContextMenu(t *testing.T) {
	a, _ := compareApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	items := rightClick(t, tt, "feature")
	want := "与当前分支 (" + a.branch + ") 对比代码差异"
	if !menuHas(items, want) {
		t.Fatalf("右键菜单该包含 %q，实际为 %v", want, items)
	}
}
