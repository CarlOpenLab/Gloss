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

// 分支代码对比全流程测试（Fork 风格）：开启对比、查看差异、交换方向、切换模式、退出恢复。
func TestBranchCompareFlow(t *testing.T) {
	a, _ := compareApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 初始状态：工作区有 1 个未提交文件（worktree.txt）
	if len(a.files) != 1 || a.files[0].Path() != "worktree.txt" {
		t.Fatalf("初始工作区文件异常：%+v", a.files)
	}

	// 1. 开启对比：main vs feature（三点对比 / PR 模式）
	a.startCompare("main", "feature", true)
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

	// 三点对比只包含 feature 在分叉后的新文件（feature.txt），不含 main_only.txt
	if len(a.files) != 1 || a.files[0].Path() != "feature.txt" {
		t.Fatalf("三点对比结果应该只有 feature.txt, 实际为：%+v", a.files)
	}
	if len(a.compare.commits) != 1 || a.compare.commits[0].Subject != "feature commit" {
		t.Fatalf("对比提交列表异常：%+v", a.compare.commits)
	}

	// 2. 切换为两点对比（直接对比 ..）
	a.setCompareTripleDot(false)
	settle(t, a)
	tt.Frame()

	// 两点对比包含 feature.txt（新增）和 main_only.txt（删除）
	if len(a.files) != 2 {
		t.Fatalf("两点对比应该有 2 个文件, 实际为：%+v", a.files)
	}

	// 3. 交换对比方向 ⇄
	a.swapCompare()
	settle(t, a)
	tt.Frame()

	if a.compare.base != "feature" || a.compare.target != "main" {
		t.Fatalf("交换后 base=%s, target=%s", a.compare.base, a.compare.target)
	}

	// 4. 退出对比模式
	if err := tt.Click("退出对比"); err != nil {
		t.Fatalf("点击退出对比失败：%v", err)
	}
	tt.Frame()

	if a.compare.on {
		t.Fatal("点击退出对比后应该已退出")
	}
	// 恢复工作区未提交的文件
	if len(a.files) != 1 || a.files[0].Path() != "worktree.txt" {
		t.Fatalf("退出对比后应该还原工作区文件：%+v", a.files)
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
