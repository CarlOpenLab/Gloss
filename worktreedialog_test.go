package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// worktreeProjectApp 把主仓库和一个 linked worktree 都作为项目打开，活动
// 的是 worktree。返回的 a.repoPath 等于 wt。
func worktreeProjectApp(t *testing.T) (*app, string) {
	t.Helper()
	a, wt := worktreeApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.openInNewTab(wt)
	settle(t, a)
	if a.repoPath != wt {
		t.Fatalf("前置条件：活动项目该是 worktree %q，实际 %q", wt, a.repoPath)
	}
	return a, wt
}

// 路径归一化与预检：相对路径按仓库根解析，绝对路径 Clean，空/已存在给出
// 禁用原因。
func TestResolveWorktreePath(t *testing.T) {
	a := branchApp(t)
	a.worktree = worktreeState{on: true, branch: "free", path: "../wt-x"}

	want := filepath.Join(filepath.Dir(a.repoPath), "wt-x")
	if got := a.resolveWorktreePath(); got != want {
		t.Errorf("相对路径解析 = %q, want %q", got, want)
	}

	abs := filepath.Join(t.TempDir(), "abs-wt")
	a.worktree.path = abs + string(filepath.Separator) + "."
	if got := a.resolveWorktreePath(); got != abs {
		t.Errorf("绝对路径该 Clean 成 %q，实际 %q", abs, got)
	}

	a.worktree.path = "   "
	if got := a.worktreeBlocker(); got == "" {
		t.Error("空路径该给出禁用原因")
	}

	if got := a.worktreeBlocker(); !strings.Contains(got, "路径") {
		t.Errorf("空路径的原因该提到路径：%q", got)
	}

	a.worktree.path = t.TempDir()
	if got := a.worktreeBlocker(); got != "这个目录已经存在" {
		t.Errorf("已存在目录的提示 = %q", got)
	}

	a.worktree.path = filepath.Join(t.TempDir(), "not-yet")
	if got := a.worktreeBlocker(); got != "" {
		t.Errorf("新目录不该有禁用原因：%q", got)
	}
}

// 从分支右键走完新建流程：对话框打开、创建后目录里是那条分支、新项目成为
// 活动项目，原项目的 refs 刷新出占用。
func TestCreateWorktreeFromBranchMenu(t *testing.T) {
	a := branchApp(t)
	mainRoot := a.repoPath
	runGit(t, mainRoot, "branch", "free")
	a.loadRefs()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	rightClick(t, tt, "free")
	if err := tt.ChooseMenuItem("在新 Worktree 中检出…"); err != nil {
		t.Fatalf("点「在新 Worktree 中检出…」：%v", err)
	}
	tt.Frame()
	if !a.worktree.on {
		t.Fatal("该打开新建 Worktree 对话框")
	}
	if a.worktree.branch != "free" {
		t.Errorf("对话框分支 = %q, want free", a.worktree.branch)
	}

	wt := filepath.Join(filepath.Dir(mainRoot), "created-"+filepath.Base(mainRoot))
	t.Cleanup(func() { os.RemoveAll(wt) })
	a.worktree.path = wt
	tt.Frame()
	if err := tt.Click("创建 Worktree"); err != nil {
		t.Fatalf("点「创建 Worktree」：%v", err)
	}
	settle(t, a)

	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree 目录没建出来：%v", err)
	}
	if got := headOf(t, wt); got != "refs/heads/free" {
		t.Errorf("worktree HEAD = %q, want refs/heads/free", got)
	}
	if got := readDisk(t, filepath.Join(wt, "a.txt")); !strings.Contains(got, "base") {
		t.Errorf("worktree 里该有 free 分支的文件：%q", got)
	}
	if a.repoPath != wt {
		t.Errorf("创建后该打开新项目 %q，实际 %q", wt, a.repoPath)
	}
	if a.worktree.on {
		t.Error("创建成功后该关掉对话框")
	}

	a.openInNewTab(mainRoot)
	settle(t, a)
	if ref := a.refByFull["refs/heads/free"]; ref == nil || ref.Worktree != wt {
		t.Errorf("原项目的 refs 该把 free 标到 %q：%+v", wt, ref)
	}
}

// 移除一个作为项目打开的 worktree：目录消失、项目 tab 关闭、原项目 refs
// 刷新掉占用。
func TestRemoveWorktreeClosesProjectTabAndRefreshesRefs(t *testing.T) {
	a, wt := worktreeProjectApp(t)
	mainRoot := ""
	for _, tab := range a.tabs {
		if !sameRepo(tab.path, wt) {
			mainRoot = tab.path
		}
	}
	if mainRoot == "" {
		t.Fatal("前置条件：该有主工作区项目")
	}

	a.promptRemoveWorktree(wt, false)
	if !a.confirm.on {
		t.Fatal("该先弹一次确认")
	}
	if !strings.Contains(a.confirm.body, wt) {
		t.Errorf("确认框正文该写明路径 %q：%q", wt, a.confirm.body)
	}
	yes := a.confirm.onYes
	a.confirm = confirmState{}
	yes()
	settle(t, a)

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("目录该被删掉，stat err = %v", err)
	}
	for _, tab := range a.tabs {
		if sameRepo(tab.path, wt) {
			t.Errorf("该 worktree 的项目 tab 该关闭：%+v", a.tabs)
		}
	}
	if a.repoPath != mainRoot {
		t.Errorf("关掉活动项目后该切到相邻项目 %q，实际 %q", mainRoot, a.repoPath)
	}
	if ref := a.refByFull["refs/heads/topic"]; ref == nil || ref.Worktree != "" {
		t.Errorf("原项目 refs 里 topic 不该再被标占用：%+v", ref)
	}
}

// 脏 worktree：第一次移除失败后弹「强制移除」二次确认，确认后目录消失。
func TestRemoveDirtyWorktreeAsksForce(t *testing.T) {
	a, wt := worktreeApp(t)
	writeFile(t, filepath.Join(wt, "dirty.txt"), "还没提交\n")

	a.promptRemoveWorktree(wt, false)
	if !a.confirm.on {
		t.Fatal("该先弹一次确认")
	}
	yes := a.confirm.onYes
	a.confirm = confirmState{}
	yes()
	settle(t, a)

	if !a.confirm.on {
		t.Fatal("脏目录普通移除失败后该弹「强制移除」确认")
	}
	if a.confirm.yes != "强制移除" {
		t.Errorf("确认键 = %q, want 强制移除", a.confirm.yes)
	}
	if !strings.Contains(a.confirm.body, "未提交") {
		t.Errorf("二次确认该说明会丢掉未提交内容：%q", a.confirm.body)
	}

	yes = a.confirm.onYes
	a.confirm = confirmState{}
	yes()
	settle(t, a)

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("强制移除后目录该消失，stat err = %v", err)
	}
}

// 要移除的项目里还有未保存的编辑器内容：第一次确认的正文必须提一句。
func TestRemoveWorktreeWarnsAboutUnsavedEditorContent(t *testing.T) {
	a, wt := worktreeProjectApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	s := a.activeEditorTab()
	if s == nil || s.code == nil {
		t.Fatal("前置条件：编辑器该已打开一个文件")
	}
	s.buffer.Set("没保存的改动\n")

	a.promptRemoveWorktree(wt, false)
	if !a.confirm.on {
		t.Fatal("该弹确认框")
	}
	if !strings.Contains(a.confirm.body, "未保存") {
		t.Errorf("确认框该提醒未保存的编辑：%q", a.confirm.body)
	}
}

// 目录被外部删掉后记录还在：菜单给「清理失效 Worktree 记录」，清理之后
// 分支恢复空闲（菜单重新给「在新 Worktree 中检出…」）。
func TestPruneStaleWorktreeRecord(t *testing.T) {
	a, wt := worktreeApp(t)
	if err := os.RemoveAll(wt); err != nil {
		t.Fatalf("删掉 worktree 目录：%v", err)
	}
	a.loadRefs()
	settle(t, a)
	if ref := a.refByFull["refs/heads/topic"]; ref == nil || !ref.WorktreePrunable {
		t.Fatalf("topic 该标成 prunable：%+v", ref)
	}

	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	items := rightClick(t, tt, "topic")
	if !menuHas(items, "清理失效 Worktree 记录") {
		t.Fatalf("菜单缺「清理失效 Worktree 记录」：%q", items)
	}
	if err := tt.ChooseMenuItem("清理失效 Worktree 记录"); err != nil {
		t.Fatalf("点「清理失效 Worktree 记录」：%v", err)
	}
	settle(t, a)

	if out := gitOut(t, a.repoPath, "worktree", "list", "--porcelain"); strings.Contains(out, "refs/heads/topic") {
		t.Errorf("prune 之后不该再有 topic 的记录：%s", out)
	}

	tt.Frame()
	items = rightClick(t, tt, "topic")
	if menuHas(items, "清理失效 Worktree 记录") {
		t.Errorf("prune 之后不该再显示清理项：%q", items)
	}
	if !menuHas(items, "在新 Worktree 中检出…") {
		t.Errorf("prune 之后 topic 恢复空闲，菜单该给新建入口：%q", items)
	}
}
