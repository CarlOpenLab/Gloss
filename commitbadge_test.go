package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 提交行上那枚分支芯片：双击 = 检出那条分支。
//
// 侧栏里双击一行早有这个动作（checkout_test.go），提交行上是后接的 ——
// 「这条线上有哪个分支」就摆在那儿，看见了想切过去的人第一反应就是点它。

// badgeApp 是一个三条分支的仓库：main、topic（HEAD 在 main 上）、far。
// topic 和 far 的尖端提交各带一枚分支芯片。
func badgeApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "checkout", "-q", "-b", "topic")
	writeFile(t, filepath.Join(dir, "b.txt"), "topic\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "topic 上的提交")

	runGit(t, dir, "checkout", "-q", "main")
	runGit(t, dir, "checkout", "-q", "-b", "far")
	writeFile(t, filepath.Join(dir, "c.txt"), "far\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "far 上的提交")
	runGit(t, dir, "checkout", "-q", "main")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()
	return a, tt
}

// 双击「topic」那枚芯片：HEAD 挪到 topic，界面回改动 tab。
func TestDoubleClickCommitBadgeChecksOut(t *testing.T) {
	a, tt := badgeApp(t)
	if _, ok := tt.Find("topic"); !ok {
		t.Fatalf("提交行上找不到 topic 芯片：%q", tt.Texts())
	}
	doubleClick(t, tt, "topic")
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/topic" {
		t.Errorf("HEAD 指向 %q，want refs/heads/topic", got)
	}
	if a.branch != "topic" {
		t.Errorf("界面上的分支是 %q，want topic", a.branch)
	}
	if a.tab != 0 {
		t.Errorf("检出之后该回到改动 tab，实际 %d", a.tab)
	}
}

// 芯片上的双击不能顺手把选中切回那个提交：检出会带走界面，那一读是白读。
func TestBadgeDoubleClickDoesNotReselectCommit(t *testing.T) {
	a, tt := badgeApp(t)
	before := a.commitSel
	doubleClick(t, tt, "topic")
	settle(t, a)
	tt.Frame()

	if a.tab == 1 && a.commitSel != before {
		t.Errorf("双击芯片把选中从 %d 改成了 %d", before, a.commitSel)
	}
}

// HEAD 那一枚没有可检出的目标，点它什么都不会发生（也不会炸）。
func TestHeadBadgeIsNotClickable(t *testing.T) {
	a, tt := badgeApp(t)
	// HEAD → main 在最新那个提交上；找得到就说明它在，双击它不该改变 HEAD
	if _, ok := tt.Find("HEAD"); !ok {
		t.Fatalf("找不到 HEAD 芯片：%q", tt.Texts())
	}
	doubleClick(t, tt, "HEAD")
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/main" {
		t.Errorf("双击 HEAD 芯片把 HEAD 挪到了 %q", got)
	}
	if a.branch != "main" {
		t.Errorf("界面上的分支该还是 main，实际 %q", a.branch)
	}
}
