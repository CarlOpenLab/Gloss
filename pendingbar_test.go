package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 状态条（pendingbar.go）在界面这一层的行为：文案、按钮的可用性、
// 确认框那道闸，以及每一个按钮真的把仓库推进（或退回）了。
//
// 和别的界面测试一个规矩：验收回到 git 自己身上（gitOut / headOf），
// 拿我们自己的 Load 去验自己的写，两边一起错的时候测试还是绿的。

// runGitFail 跑一条本来就该失败的命令 —— 合并/变基撞冲突是它工作的样子。
// runGit 会把它 t.Fatalf 掉。
func runGitFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("git %v 本该失败（用来造冲突），却成功了：\n%s", args, out)
	}
}

// conflictDir 造一个 main 与 topic 在 a.txt 上冲突的仓库，回到 main。
func conflictDir(t *testing.T) string {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "checkout", "-q", "-b", "topic")
	writeFile(t, filepath.Join(dir, "a.txt"), "topic\n")
	runGit(t, dir, "commit", "-qam", "topic")
	runGit(t, dir, "checkout", "-q", "main")
	writeFile(t, filepath.Join(dir, "a.txt"), "main\n")
	runGit(t, dir, "commit", "-qam", "main")
	return dir
}

// mergeConflictApp 是卡在一次合并冲突里的 app。
func mergeConflictApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := conflictDir(t)
	runGitFail(t, dir, "merge", "topic")
	return loadApp(t, dir), dir
}

// rebaseConflictApp 是卡在一次变基冲突里的 app（topic 正变基到 main）。
func rebaseConflictApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := conflictDir(t)
	runGit(t, dir, "checkout", "-q", "topic")
	runGitFail(t, dir, "rebase", "main")
	return loadApp(t, dir), dir
}

// resolveA 在「外面」把 a.txt 的冲突解决掉（界面里的逐文件按钮是下一步
// 的事），然后刷新 —— 模拟用户手工解决完回到应用。
func resolveA(t *testing.T, a *app, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "a.txt"), "resolved\n")
	runGit(t, dir, "add", "a.txt")
	a.reload()
	settle(t, a)
}

// 合并在冲突里停住：状态条说出处境和出路，冲突没解决时「继续」点不动。
func TestPendingBarMergeConflict(t *testing.T) {
	a, dir := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if a.pending.Kind != gitrepo.PendingMerge {
		t.Fatalf("pending = %+v，want 合并", a.pending)
	}
	if !tt.HasText("合并停在冲突上") {
		t.Errorf("状态条该说清停在哪儿：%q", tt.Texts())
	}
	if !tt.HasText("还有 1 个冲突文件没解决") {
		t.Errorf("该数出还有几个冲突：%q", tt.Texts())
	}
	// merge 没有 --skip，不该画一颗按了 git 会回「no --skip」的按钮
	if _, ok := tt.Find("跳过"); ok {
		t.Error("合并不该有「跳过」")
	}

	// 冲突没解决，「继续」是灰的：点下去不该真跑，也不该留下错误横幅
	if err := tt.Click("继续"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	if !a.pending.Active() {
		t.Error("冲突没解决时「继续」不该真的跑")
	}
	if a.opErr != nil {
		t.Errorf("点灰按钮不该弹失败横幅：%v", a.opErr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "MERGE_HEAD")); err != nil {
		t.Errorf("仓库不该被动过：%v", err)
	}
}

// 冲突解决完之后「继续」变成可点的，点下去合并真的落地。
func TestPendingBarContinueCompletesMerge(t *testing.T) {
	a, dir := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	resolveA(t, a, dir)
	tt.Frame()
	if !tt.HasText("合并还没收尾") {
		t.Errorf("冲突解决后标题该跟着换：%q", tt.Texts())
	}

	if err := tt.Click("继续"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.pending.Active() {
		t.Errorf("继续之后该收尾：%+v", a.pending)
	}
	if !hasAnnouncement(tt.Announcements(), "合并完成") {
		t.Errorf("该播报「合并完成」：%q", tt.Announcements())
	}
	if got := gitOut(t, dir, "log", "-1", "--format=%s"); got != "Merge branch 'topic'" {
		t.Errorf("合并没真的落地：log -1 = %q", got)
	}
	if got := gitOut(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("合并完工作区该干净：%q", got)
	}
}

// 「中止」先过确认框：Esc 取消等于什么都没发生，确认之后仓库回到动手之前。
func TestPendingBarAbortAsksFirst(t *testing.T) {
	a, dir := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	before := headOf(t, dir)

	if err := tt.Click("中止"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.confirm.on {
		t.Fatal("「中止」该先问一句")
	}
	if !tt.HasText("中止这次合并？") {
		t.Errorf("确认框该把话说全：%q", tt.Texts())
	}
	// 还没确认：仓库一动不动
	if _, err := os.Stat(filepath.Join(dir, ".git", "MERGE_HEAD")); err != nil {
		t.Fatalf("确认之前不该动仓库：%v", err)
	}

	// Esc = 取消
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.confirm.on {
		t.Error("Esc 该关掉确认框")
	}
	if !a.pending.Active() {
		t.Error("取消之后合并该还停着")
	}

	// 再来一次，这回真确认
	if err := tt.Click("中止"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("中止合并"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.pending.Active() {
		t.Errorf("确认之后该中止：%+v", a.pending)
	}
	if got := headOf(t, dir); got != before {
		t.Errorf("HEAD = %q，want %q", got, before)
	}
	if got := gitOut(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("中止后工作区该干净：%q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Error("中止后 MERGE_HEAD 该没了")
	}
}

// 变基中途：状态条给进度，芯片跟着改口（不能再喊「分离 HEAD」，那是主动
// 误导）；「跳过」只在有 --skip 的操作上出现。
func TestPendingBarRebaseAndChip(t *testing.T) {
	a, dir := rebaseConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if a.pending.Kind != gitrepo.PendingRebase {
		t.Fatalf("pending = %+v，want 变基", a.pending)
	}
	if !tt.HasText("正在变基 topic → main（第 1/1 步）") {
		t.Errorf("芯片该说清变基到哪了：%q", tt.Texts())
	}
	if !tt.HasText("变基停在冲突上") {
		t.Errorf("状态条该说清停在哪一步：%q", tt.Texts())
	}
	if tt.HasText("分离 HEAD") {
		t.Error("变基期间说「分离 HEAD」是主动误导")
	}
	if _, ok := tt.Find("跳过"); !ok {
		t.Error("变基有 --skip，该有「跳过」")
	}

	// 解决之后跳过：topic 的提交被丢掉，变基收尾，分支回到 topic 上
	resolveA(t, a, dir)
	tt.Frame()
	if err := tt.Click("跳过"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.pending.Active() {
		t.Errorf("跳过之后该收尾：%+v", a.pending)
	}
	if got := headOf(t, dir); got != "refs/heads/topic" {
		t.Errorf("HEAD = %q，want refs/heads/topic", got)
	}
	if got := gitOut(t, dir, "log", "-1", "--format=%s"); got != "main" {
		t.Errorf("被跳过的提交不该在历史里：log -1 = %q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已跳过这一步") {
		t.Errorf("该播报跳过：%q", tt.Announcements())
	}
}

// 储藏 apply/pop 撞出来的冲突没有可继续的步骤：状态条只叙述，不摆
// 「继续 / 中止」—— 按了 git 也不认。
func TestPendingBarStashConflictHasNoButtons(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	writeFile(t, filepath.Join(dir, "a.txt"), "stash 里那份\n")
	runGit(t, dir, "stash", "push", "-m", "x")
	writeFile(t, filepath.Join(dir, "a.txt"), "提交进去那份\n")
	runGit(t, dir, "commit", "-qam", "committed")
	runGitFail(t, dir, "stash", "pop")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if a.pending.Active() {
		t.Fatalf("stash pop 撞冲突不该被当成可继续的状态：%+v", a.pending)
	}
	if !tt.HasText("有 1 个冲突文件要解决") {
		t.Errorf("状态条该数出冲突：%q", tt.Texts())
	}
	// 找按钮用 Find（精确）：提示语里就写着「没有「继续」」，HasText 会把它算进去
	if _, ok := tt.Find("继续"); ok {
		t.Error("没有可继续的步骤时不该摆「继续」")
	}
	if _, ok := tt.Find("中止"); ok {
		t.Error("储藏撞的冲突没有「中止」这条路")
	}
}

// 「静止」的仓库不该画状态条。
func TestPendingBarAbsentOnCleanRepo(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if tt.HasText("继续") || tt.HasText("停在冲突上") {
		t.Errorf("干净仓库不该有状态条：%q", tt.Texts())
	}
}

// 确认框或别的对话框开着时，菜单命令要在门口就被挡住 —— 视图里的快捷键
// 由框架挡，菜单只能自己挡，两边必须一个脾气。
func TestMenuYieldsWhileModalOpen(t *testing.T) {
	a := testApp(t)
	a.askConfirm(confirmState{title: "试试？", yes: "试试"})

	ran := false
	a.menuDo(func() { ran = true })
	a.loop.drain()
	if ran {
		t.Error("对话框开着时菜单命令该让位")
	}

	a.confirm = confirmState{}
	a.menuDo(func() { ran = true })
	a.loop.drain()
	if !ran {
		t.Error("对话框关掉之后菜单命令该恢复")
	}
}

// 卡住的仓库里，历史 tab 也能看见状态条：它说的是仓库的状态，不是某个
// tab 的事 —— 在历史里挑提交时，也随时要能点「中止」退出去。
func TestPendingBarShowsOnBothTabs(t *testing.T) {
	a, _ := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.setTab(1)
	settle(t, a)
	tt.Frame()
	if !tt.HasText("合并停在冲突上") {
		t.Errorf("历史 tab 也该看得见状态条：%q", tt.Texts())
	}
}

// 变基的列表身份认的是分支名而不是短哈希：整个变基过程中「改动」列表的
// id 不变，用户展开过的区间不会被一步一次地重置。
func TestWorkListIDStableDuringRebase(t *testing.T) {
	dir := conflictDir(t)
	runGit(t, dir, "checkout", "-q", "topic")
	runGitFail(t, dir, "rebase", "main")
	a := loadApp(t, dir)

	if got, want := a.listID, workListID(a.repoPath, "topic"); got != want {
		t.Errorf("变基期间 listID = %q，want %q（用短哈希的话每重放一步就换一个）", got, want)
	}
	if !strings.Contains(a.listID, "topic") {
		t.Error("listID 该带分支名")
	}
}
