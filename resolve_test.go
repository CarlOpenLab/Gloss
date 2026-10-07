package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 冲突文件在界面这一层的解决路径：卡片上的按钮、文件行的右键菜单、
// 「在编辑器打开」、栏脚的冲突数。
//
// 和别的界面测试一个规矩：验收回到 git 自己身上（gitOut / 磁盘上的字节），
// 拿我们自己的 Load 去验自己的写，两边一起错的时候测试还是绿的。

// deleteConflictApp 是卡在「对方删了 a.txt、我这边改了它」的合并里的 app。
func deleteConflictApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "checkout", "-q", "-b", "topic")
	runGit(t, dir, "rm", "-q", "a.txt")
	runGit(t, dir, "commit", "-qm", "topic 删了它")
	runGit(t, dir, "checkout", "-q", "main")
	writeFile(t, filepath.Join(dir, "a.txt"), "main 改的\n")
	runGit(t, dir, "commit", "-qam", "main 改了它")

	runGitFail(t, dir, "merge", "topic")
	return loadApp(t, dir), dir
}

// stashConflictApp 是卡在储藏 pop 撞冲突里的 app（没有 MERGE_HEAD）。
func stashConflictApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	writeFile(t, filepath.Join(dir, "a.txt"), "储藏里的那份\n")
	runGit(t, dir, "stash", "push", "-m", "x")
	writeFile(t, filepath.Join(dir, "a.txt"), "提交进去那份\n")
	runGit(t, dir, "commit", "-qam", "committed")
	runGitFail(t, dir, "stash", "pop")
	return loadApp(t, dir), dir
}

// fileText 读工作区里某个文件的字节。
func fileText(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("读 %s: %v", name, err)
	}
	return string(b)
}

// 变基里按钮必须说「用变基目标的 / 用重放中的」，点「用重放中的」拿到的是
// 正在重放那条提交的内容 —— 文案和事实一起钉住。照合并的习惯写「用我这
// 边的」，用户点下去拿到的是别人的代码，而且毫无提示。
func TestConflictCardResolvesTheirsInRebase(t *testing.T) {
	a, dir := rebaseConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("用变基目标的") || !tt.HasText("用重放中的") {
		t.Fatalf("变基里的按钮该按变基的两边说话：%q", tt.Texts())
	}
	// 白话把「反的」说清：光靠按钮词，用户还是会按合并的直觉去读
	if !tt.HasText("变基里两边是反的") {
		t.Errorf("卡片该解释变基里的左右和合并相反：%q", tt.Texts())
	}

	if err := tt.Click("用重放中的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, dir, "a.txt"); got != "topic\n" {
		t.Errorf("a.txt = %q，want 正在重放那条提交的内容", got)
	}
	if got := gitOut(t, dir, "ls-files", "-u"); got != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已按「用重放中的」解决 a.txt") {
		t.Errorf("该播报解决了哪个文件、按哪边：%q", tt.Announcements())
	}
	// 变基本身还没收尾：解决完一个文件不等于变基做完了
	if !a.pending.Active() || a.pending.Kind != gitrepo.PendingRebase {
		t.Errorf("解决文件不该动变基本身：%+v", a.pending)
	}
}

// 合并里的两边：「用当前分支的 / 用被并入的」，被并入的名字从引用表里找回来。
func TestConflictCardLabelsFollowKind(t *testing.T) {
	a, _ := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("用当前分支的") || !tt.HasText("用被并入的") {
		t.Errorf("合并里的按钮该按合并的两边说话：%q", tt.Texts())
	}
	if tt.HasText("用变基目标的") || tt.HasText("用重放中的") {
		t.Errorf("合并跟变基不是一回事，不该出现变基的词：%q", tt.Texts())
	}
	// 被并入的那边有名字（topic），tooltip 里该写出来 —— 「对方」是谁是
	// 用户此刻唯一在猜的事。悬停 tooltip 要等 700ms 才画，这里直接问算出
	// 来的那份，和按钮用的是同一个来源。
	var f gitrepo.File
	for _, nf := range a.files {
		if nf.Status == gitrepo.StatusConflict {
			f = nf
		}
	}
	if act := a.conflictActionsFor(f); !strings.Contains(act.theirsTip, "topic") {
		t.Errorf("被并入那一边的 tooltip 该带分支名：%q", act.theirsTip)
	}
}

// 点「用当前分支的」解决合并冲突：内容落地、卡片消失、状态条改口
// 「合并还没收尾」—— 出路只剩「继续」。
func TestConflictCardResolvesMergeOurs(t *testing.T) {
	a, dir := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if err := tt.Click("用当前分支的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, dir, "a.txt"); got != "main\n" {
		t.Errorf("a.txt = %q，want 当前分支那份", got)
	}
	if a.conflictCount() != 0 {
		t.Errorf("解决之后冲突数该归零：%d", a.conflictCount())
	}
	if !tt.HasText("合并还没收尾") {
		t.Errorf("状态条该改口说「还没收尾」：%q", tt.Texts())
	}
	// 用 Find（精确）而不是 HasText：成功 Toast 里也带着「用当前分支的」
	// 这几个字，substring 匹配会把那条播报误认成按钮。
	if _, ok := tt.Find("用当前分支的"); ok {
		t.Errorf("卡片该消失了：%q", tt.Texts())
	}
}

// 删除-修改：卡片要把「按下去是删除」提前说清，点「用被并入的」之后
// 文件真的从工作区消失，未合并条目清零。
func TestDeleteConflictSaysWhatTheButtonDoes(t *testing.T) {
	a, dir := deleteConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("接受删除") {
		t.Fatalf("对方删了文件时，卡片该说清「用被并入的」= 接受删除：%q", tt.Texts())
	}

	if err := tt.Click("用被并入的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("接受删除之后文件该没了：%v", err)
	}
	if got := gitOut(t, dir, "ls-files", "-u"); got != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", got)
	}
	if a.conflictCount() != 0 {
		t.Errorf("解决之后冲突数该归零：%d", a.conflictCount())
	}
}

// 文件行的右键菜单：卡片上那几颗按钮在这里再摆一份（外加「标记为已解决」），
// 从文件列表就能动手，不用先滚到 diff 那边。
func TestConflictMenuOnFileRow(t *testing.T) {
	a, dir := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	items := rightClick(t, tt, "a.txt")
	for _, want := range []string{"用当前分支的", "用被并入的", "在编辑器打开", "标记为已解决"} {
		if !menuHas(items, want) {
			t.Fatalf("冲突文件的右键菜单里该有 %q，实际 %v", want, items)
		}
	}
	if err := tt.ChooseMenuItem("用被并入的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, dir, "a.txt"); got != "topic\n" {
		t.Errorf("菜单挑的那一边没落地：a.txt = %q", got)
	}
}

// 「在编辑器打开」交给注入的打开器，拿到的是仓库里的绝对路径。
func TestOpenInEditorReceivesAbsPath(t *testing.T) {
	a, _ := mergeConflictApp(t)
	var got string
	a.openPath = func(p string) { got = p }

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if err := tt.Click("在编辑器打开"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)

	want := filepath.Join(a.repoPath, "a.txt")
	if got != want {
		t.Errorf("打开器收到 %q，want %q", got, want)
	}
}

// 储藏撞的冲突没有可继续的步骤，但卡片照样能挑一边 —— ours 是当前分支，
// theirs 是储藏里的那份。
func TestStashConflictCardPicksStashedSide(t *testing.T) {
	a, dir := stashConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("用对方的") {
		t.Fatalf("储藏撞的冲突也该能挑一边：%q", tt.Texts())
	}
	if err := tt.Click("用对方的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, dir, "a.txt"); got != "储藏里的那份\n" {
		t.Errorf("a.txt = %q，want 储藏里的那份", got)
	}
	if got := gitOut(t, dir, "ls-files", "-u"); got != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", got)
	}
}

// 栏脚数出还有几个冲突待解决；解决完就没有了。
func TestFooterShowsConflictCount(t *testing.T) {
	a, _ := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("1 个冲突待解决") {
		t.Fatalf("栏脚该数出冲突：%q", tt.Texts())
	}

	if err := tt.Click("用当前分支的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if tt.HasText("个冲突待解决") {
		t.Errorf("解决完栏脚不该还挂着冲突数：%q", tt.Texts())
	}
}

// 竞态：卡片是上一帧的，文件已经被解决过了 —— 点下去得到的是一句说得清
// 的失败（「这个文件已经没有冲突了」），而不是静默地按旧状态覆盖。
func TestResolveWhenAlreadyResolvedExplains(t *testing.T) {
	a, dir := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 界面之外（终端里）把冲突解决掉了，界面还拿着旧卡片
	writeFile(t, filepath.Join(dir, "a.txt"), "外面解好的\n")
	runGit(t, dir, "add", "a.txt")
	if err := tt.Click("用当前分支的"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("点旧卡片该留下一条说得清的失败")
	}
	if a.opErr.Title != "这个文件已经没有冲突了" {
		t.Errorf("标题 = %q（原话：%q）", a.opErr.Title, a.opErr.Detail)
	}
	if got := fileText(t, dir, "a.txt"); got != "外面解好的\n" {
		t.Errorf("拒绝的那一下不该动文件：a.txt = %q", got)
	}
	if !strings.Contains(a.opErr.Hint, "重新读过") {
		t.Errorf("该说明列表已经重读过：%q", a.opErr.Hint)
	}
}
