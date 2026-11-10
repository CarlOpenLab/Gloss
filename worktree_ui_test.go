package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// worktreeApp 在 branchApp 的基础上把 topic 检出到一个 linked worktree。
// 返回的路径在测试结束后删掉（仓库本身由 t.TempDir 收拾）。
func worktreeApp(t *testing.T) (*app, string) {
	t.Helper()
	a := branchApp(t)
	wt := filepath.Join(filepath.Dir(a.repoPath), "wt-"+filepath.Base(a.repoPath))
	runGit(t, a.repoPath, "worktree", "add", wt, "topic")
	t.Cleanup(func() { os.RemoveAll(wt) })
	a.loadRefs()
	settle(t, a)
	return a, wt
}

// 被别的 worktree 检出的分支行带 WT 芯片；当前分支只有 HEAD，不叠加。
func TestRefRowShowsWTForOtherWorktree(t *testing.T) {
	a, wt := worktreeApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()

	if !tt.HasText("WT") {
		t.Errorf("被 worktree 检出的分支该带 WT 芯片：%q", tt.Texts())
	}
	if n := countText(tt.Texts(), "WT"); n != 1 {
		t.Errorf("WT 只该出现在被占用的那条分支上，实际 %d 次", n)
	}

	ref := a.refByFull["refs/heads/topic"]
	if ref == nil || ref.Worktree == "" {
		t.Fatalf("topic 该被注解为已检出：%+v", ref)
	}
	if ref.Worktree != wt {
		t.Errorf("topic.Worktree = %q, want %q", ref.Worktree, wt)
	}
	if main := a.refByFull["refs/heads/main"]; main == nil || !main.WorktreeMain {
		t.Errorf("main 该标在主工作区：%+v", main)
	}
}

// 分支右键菜单：被占用的分支给「打开所在 Worktree」「移除 Worktree…」，
// 空闲分支给「在新 Worktree 中检出…」；「打开」真的切到那个项目。
func TestLocalBranchMenuOffersWorktreeActions(t *testing.T) {
	a, wt := worktreeApp(t)
	runGit(t, a.repoPath, "branch", "free")
	a.loadRefs()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()

	items := rightClick(t, tt, "topic")
	for _, want := range []string{"打开所在 Worktree", "移除 Worktree…"} {
		if !menuHas(items, want) {
			t.Errorf("被占用分支的菜单缺 %q：%q", want, items)
		}
	}
	if menuHas(items, "在新 Worktree 中检出…") {
		t.Errorf("已被占用的分支不该再提供「在新 Worktree 中检出…」：%q", items)
	}
	mainRoot := a.repoPath
	if err := tt.ChooseMenuItem("打开所在 Worktree"); err != nil {
		t.Fatalf("点「打开所在 Worktree」：%v", err)
	}
	settle(t, a)
	if a.repoPath != wt {
		t.Errorf("该切到 worktree %q，实际 %q", wt, a.repoPath)
	}

	// 换回主项目，看空闲的 free 分支上有什么。
	a.openInNewTab(mainRoot)
	settle(t, a)
	tt.Frame()
	items = rightClick(t, tt, "free")
	if !menuHas(items, "在新 Worktree 中检出…") {
		t.Errorf("空闲分支的菜单缺「在新 Worktree 中检出…」：%q", items)
	}
	if menuHas(items, "打开所在 Worktree") {
		t.Errorf("空闲分支不该有「打开所在 Worktree」：%q", items)
	}
}

// 被别的 worktree 占用的分支：planCheckout 给出 noop 和说明，checkoutRef
// 不发命令，HEAD 不动。
func TestPlanCheckoutBlockedForWorktreeOccupiedBranch(t *testing.T) {
	a, wt := worktreeApp(t)
	ref := *a.refByFull["refs/heads/topic"]

	plan, ok := a.planCheckout(ref)
	if !ok {
		t.Fatal("本地分支该有检出计划")
	}
	if !plan.noop {
		t.Errorf("被 worktree 占用的分支该走 noop：%+v", plan)
	}
	if !strings.Contains(plan.note, wt) {
		t.Errorf("说明里该带 worktree 路径 %q：%q", wt, plan.note)
	}

	a.checkoutRef(ref)
	if a.op != gitrepo.OpNone {
		t.Errorf("noop 不该发写操作，op = %v", a.op)
	}
	if got := headOf(t, a.repoPath); got != "refs/heads/main" {
		t.Errorf("HEAD = %q, want refs/heads/main", got)
	}
}

// refsDirty 的项目切回来时重读引用并清掉标记。
func TestRefsDirtyReloadsOnSwitchBack(t *testing.T) {
	a, _ := worktreeApp(t)
	a.openInNewTab(a.repoPath)
	other := newRepoDir(t)
	a.openInNewTab(other)
	settle(t, a)

	mainIdx, otherIdx := -1, -1
	for i, tab := range a.tabs {
		switch {
		case sameRepo(tab.path, a.tabs[0].path):
			mainIdx = i
		case sameRepo(tab.path, other):
			otherIdx = i
		}
	}
	if mainIdx < 0 || otherIdx < 0 || mainIdx == otherIdx {
		t.Fatalf("该有两个项目 tab：%+v", a.tabs)
	}

	a.switchTab(mainIdx)
	settle(t, a)
	a.currentTab().refsDirty = true
	before := a.refsSeq

	a.switchTab(otherIdx)
	settle(t, a)
	a.switchTab(mainIdx)
	settle(t, a)

	if a.refsSeq <= before {
		t.Error("切回 refsDirty 项目该重新读取 refs")
	}
	if a.currentTab().refsDirty {
		t.Error("重读之后该清掉 refsDirty")
	}
}

// worktree 操作之后，同一仓库的所有项目（主工作区 + 各个 linked worktree）
// 都该标 refsDirty；别的仓库不受影响。
func TestMarkRefsDirtyForSameRepoProjects(t *testing.T) {
	a, wt := worktreeApp(t)
	a.openInNewTab(a.repoPath)
	a.openInNewTab(wt)
	other := newRepoDir(t)
	a.openInNewTab(other)
	settle(t, a)

	mainRoot := ""
	for _, tab := range a.tabs {
		if tab.path != wt && !sameRepo(tab.path, other) {
			mainRoot = tab.path
		}
	}
	if mainRoot == "" {
		t.Fatalf("找不到主工作区项目：%+v", a.tabs)
	}

	a.markRefsDirtyForRepo(mainRoot)
	settle(t, a)

	for _, tab := range a.tabs {
		want := sameRepo(tab.path, mainRoot) || sameRepo(tab.path, wt)
		if tab.refsDirty != want {
			t.Errorf("tab %q refsDirty = %v, want %v", tab.path, tab.refsDirty, want)
		}
	}
}
