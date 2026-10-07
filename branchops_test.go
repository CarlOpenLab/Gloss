package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 本地分支管理（新建、重命名、删除、复制分支名）在界面层的验收。

// 快捷键 ⌘⇧N / 菜单「新建分支…」：从当前 HEAD 起步，建好切过去。
func TestCreateBranchFromMenu(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath

	tt.Key(ui.Cmd|ui.Shift, ui.KeyN)
	tt.Frame()

	if !a.newBranch.on {
		t.Fatal("⌘⇧N 应该打开新建分支对话框")
	}
	if !tt.HasText("从 main 起步") {
		t.Errorf("应该标明从 HEAD/main 起步：%q", tt.Texts())
	}

	tt.Type("feature/from-menu")
	tt.Frame()
	if err := tt.Click("创建分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, dir); got != "refs/heads/feature/from-menu" {
		t.Errorf("HEAD = %q, want refs/heads/feature/from-menu", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已新建并切换到 feature/from-menu") {
		t.Errorf("该播报建了什么：%q", tt.Announcements())
	}
}

// 侧栏右键重命名非当前分支：原分支名消失，新分支名出现。
func TestRenameBranchDialog(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("重命名 topic…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !a.rename.on {
		t.Fatal("应该打开重命名分支对话框")
	}
	if !tt.HasText("将 topic 改名为") {
		t.Errorf("应该提示原分支名称：%q", tt.Texts())
	}

	a.rename.newName = "topic-renamed"
	tt.Frame()
	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "rev-parse", "--verify", "refs/heads/topic-renamed"); got == "" {
		t.Error("新分支 topic-renamed 应该存在")
	}
	if got := gitOut(t, dir, "branch", "--list", "topic"); strings.TrimSpace(got) != "" {
		t.Errorf("原分支 topic 应该已不存在，实际：%q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已将分支 topic 重命名为 topic-renamed") {
		t.Errorf("应该播报重命名成功：%q", tt.Announcements())
	}
}

// 重命名当前分支：HEAD 自动跟随更新，工具栏分支芯片刷新。
func TestRenameCurrentBranchUpdatesChip(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "main")
	if err := tt.ChooseMenuItem("重命名 main…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	a.rename.newName = "trunk"
	tt.Frame()
	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "symbolic-ref", "--short", "HEAD"); got != "trunk\n" && got != "trunk" {
		t.Errorf("HEAD = %q, want trunk", got)
	}
	if a.branch != "trunk" {
		t.Errorf("a.branch = %q, want trunk", a.branch)
	}
	if !tt.HasText("trunk") {
		t.Errorf("界面上该出现 trunk 分支名：%q", tt.Texts())
	}
}

// 重命名撞同名：git 拒绝，错误留在对话框中，对话框保持开启。
func TestRenameFailureStaysInDialog(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("重命名 topic…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	a.rename.newName = "main" // 与已有分支重名
	tt.Frame()
	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.rename.on {
		t.Fatal("失败时对话框该保持开启")
	}
	if !tt.HasText("本地已经有这条分支了") {
		t.Errorf("该提示重名错误：%q", tt.Texts())
	}
}

// 删除已合并分支：弹一次确认框，点确认后分支被删除。
func TestDeleteBranchAsksThenDeletes(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "branch", "feat-done")
	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "feat-done")
	if err := tt.ChooseMenuItem("删除分支 feat-done…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("该弹出确认框")
	}
	if !tt.HasText("确定要删除本地分支 feat-done 吗？") {
		t.Errorf("确认文案不对：%q", tt.Texts())
	}

	if err := tt.Click("删除分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "branch", "--list", "feat-done"); strings.TrimSpace(got) != "" {
		t.Errorf("分支该被删掉了，实际：%q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已删除分支 feat-done") {
		t.Errorf("该播报删除：%q", tt.Announcements())
	}
}

// 删除未合并分支：初次 -d 拒绝，自动触发二次确认「仍然删除」，确认后强制 -D 删除。
func TestDeleteUnmergedBranchAsksTwice(t *testing.T) {
	a := branchApp(t) // topic 上有未合并的提交
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("删除分支 topic…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	// 第一次确认
	if !a.confirm.on {
		t.Fatal("第一次应该弹删除确认框")
	}
	if err := tt.Click("删除分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	// git -d 拒绝后，应该自动弹出二次确认框
	if !a.confirm.on {
		t.Fatal("未合并分支被拒后，该弹出二次确认框")
	}
	if !tt.HasText("topic 还没合并进 main") {
		t.Errorf("二次确认标题不对：%q", tt.Texts())
	}
	if !tt.HasText("它上面有没并进当前分支的提交") {
		t.Errorf("二次确认说明不对：%q", tt.Texts())
	}

	// 点「仍然删除」强制删除
	if err := tt.Click("仍然删除"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "branch", "--list", "topic"); strings.TrimSpace(got) != "" {
		t.Errorf("强制删除后分支该消失了，实际：%q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已删除分支 topic") {
		t.Errorf("该播报删除：%q", tt.Announcements())
	}
}

// 当前分支（HEAD 所在）在菜单里：「检出」与「删除分支」均置灰。
func TestDeleteCurrentBranchDisabled(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "main")

	// 检出 main 该是灰的
	if err := tt.ChooseMenuItem("检出 main"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("检出当前分支该是灰的：err = %v", err)
	}

	// 删除分支 main… 该是灰的
	if err := tt.ChooseMenuItem("删除分支 main…"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("删除当前分支该是灰的：err = %v", err)
	}

	// 重命名 main… 应该是可用的
	if err := tt.ChooseMenuItem("重命名 main…"); err != nil {
		t.Errorf("重命名当前分支该可用：err = %v", err)
	}
}

// 复制分支名写进剪贴板。
func TestCopyBranchName(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("复制分支名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if got := tt.Clipboard(); got != "topic" {
		t.Errorf("剪贴板 = %q, want topic", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已复制 topic") {
		t.Errorf("该播报复制了什么：%q", tt.Announcements())
	}
}
