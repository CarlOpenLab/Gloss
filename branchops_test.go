package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
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

// 新建分支对话框：点压暗的背景不关。
//
// 名字打了一半，手一抖点在对话框外面的背景上，连同步取消问都不问就没了 ——
// 这条盯着「误触不关」；Esc 和「取消」是显式动作，照旧关。
func TestNewBranchDialogSurvivesBackdropClick(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	a.openNewBranchDialog("", "HEAD")
	tt.Frame()
	if !a.newBranch.on {
		t.Fatal("新建分支对话框没打开")
	}

	tt.Type("feature/half-typed")
	tt.Frame()
	if a.newBranch.name != "feature/half-typed" {
		t.Fatalf("名字没进框：%q", a.newBranch.name)
	}

	// 面板居中，左上角这一下落在压暗的背景上。
	tt.ClickAt(4, 4)
	tt.Frame()

	if !a.newBranch.on {
		t.Fatal("点背景不该关掉新建分支对话框")
	}
	if a.newBranch.name != "feature/half-typed" {
		t.Errorf("点背景不该丢名字：%q", a.newBranch.name)
	}
	if !tt.HasText("新建分支") {
		t.Errorf("对话框该还在屏幕上：%q", tt.Texts())
	}

	// Esc 是显式关闭，照旧关。
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.newBranch.on {
		t.Error("Esc 该关掉对话框")
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

// 侧栏右键合并分支：把非 HEAD 分支干净合并进当前分支。
func TestMergeInFromRefMenu(t *testing.T) {
	a := branchApp(t) // main 上有 a.txt，topic 上多一个 b.txt
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("把 topic 合并进当前分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err != nil {
		t.Errorf("合并后工作区应该包含 b.txt：%v", err)
	}
	if !hasAnnouncement(tt.Announcements(), "已把 topic 合并进 main") {
		t.Errorf("应该播报合并提示：%q", tt.Announcements())
	}
}

// 合并撞冲突：git 退出非 0，进入 PendingMerge 状态并展示琥珀色状态条。
func TestMergeConflictEntersPendingBar(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "checkout", "-qb", "topic")
	writeFile(t, filepath.Join(dir, "a.txt"), "topic change\n")
	runGit(t, dir, "commit", "-qam", "topic commit")

	runGit(t, dir, "checkout", "-q", "main")
	writeFile(t, filepath.Join(dir, "a.txt"), "main change\n")
	runGit(t, dir, "commit", "-qam", "main commit")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("把 topic 合并进当前分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.pending.Kind != gitrepo.PendingMerge {
		t.Fatalf("pending.Kind = %v, want PendingMerge", a.pending.Kind)
	}
	if !tt.HasText("合并停在冲突上") {
		t.Errorf("应该展示合并中的状态条：%q", tt.Texts())
	}
}

// 变基到指定分支：弹确认对话框，确认后成功变基。
func TestRebaseOntoFromRefMenu(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	// 检出 topic
	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("检出 topic"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	// 右键 main 变基
	rightClick(t, tt, "main")
	if err := tt.ChooseMenuItem("把当前分支变基到 main…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("应该弹出变基确认框")
	}
	if !tt.HasText("变基 topic 到 main") {
		t.Errorf("确认框标题不对：%q", tt.Texts())
	}

	if err := tt.Click("开始变基"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !hasAnnouncement(tt.Announcements(), "已把 topic 变基到 main") {
		t.Errorf("应该播报变基完成：%q", tt.Announcements())
	}
}

// 强制推送二次确认：提示 --force-with-lease 并在确认后推送到远程。
func TestForcePushDialog(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath
	settle(t, a)
	tt.Frame()

	// 第一次推 main，建立 upstream
	a.doPush()
	settle(t, a)
	tt.Frame()

	// 本地 amend 改写提交
	writeFile(t, filepath.Join(dir, "a.txt"), "amended\n")
	runGit(t, dir, "commit", "-qam", "amended init", "--amend")

	a.promptForcePush()
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("应该弹出强制推送确认框")
	}
	if !tt.HasText("强制推送 main") {
		t.Errorf("确认框标题不对：%q", tt.Texts())
	}
	if !tt.HasText("force-with-lease") {
		t.Errorf("确认框说明应提及 force-with-lease：%q", tt.Texts())
	}

	if err := tt.Click("强制推送"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	localHead := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD"))
	if got := remoteHead(t, bare, "main"); got != localHead {
		t.Errorf("远程 HEAD = %q, want %q", got, localHead)
	}
	if !hasAnnouncement(tt.Announcements(), "已强制推送 main") {
		t.Errorf("应该播报强制推送成功：%q", tt.Announcements())
	}
}

// 标签右键菜单：推送标签到远程。
func TestPushTagFromMenu(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath

	runGit(t, dir, "tag", "v1.0")
	a.loadRefs()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "v1.0")
	if err := tt.ChooseMenuItem("推送标签 v1.0 到 origin"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	tagHead := strings.TrimSpace(gitOut(t, bare, "rev-parse", "refs/tags/v1.0"))
	localTag := strings.TrimSpace(gitOut(t, dir, "rev-parse", "refs/tags/v1.0"))
	if tagHead == "" || tagHead != localTag {
		t.Errorf("远程标签 = %q, want %q", tagHead, localTag)
	}
	if !hasAnnouncement(tt.Announcements(), "已推送标签 v1.0 到 origin") {
		t.Errorf("应该播报推送标签成功：%q", tt.Announcements())
	}
}

// 本地分支右键菜单里也能删远程那份：只在它真有上游时才出现，删掉的确实是
// 远程分支，本地分支留着。
func TestLocalBranchMenuDeletesItsUpstream(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath

	runGit(t, dir, "branch", "feat-up")
	runGit(t, dir, "push", "-u", "origin", "feat-up")
	// 没有上游的本地分支：菜单里不该冒出「删除远程分支」。
	runGit(t, dir, "branch", "feat-lonely")
	a.loadRefs()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "feat-lonely")
	if err := tt.ChooseMenuItem("删除远程分支 origin/feat-lonely…"); err == nil {
		t.Fatal("没有上游的分支不该有「删除远程分支」")
	}
	tt.CloseMenu()

	rightClick(t, tt, "feat-up")
	if err := tt.ChooseMenuItem("删除远程分支 origin/feat-up…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.confirm.on {
		t.Fatal("应该弹出删除远程分支确认框")
	}
	if err := tt.Click("删除远程分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := strings.TrimSpace(gitOut(t, bare, "branch", "--list", "feat-up")); got != "" {
		t.Errorf("远程分支该被删除了，实际：%q", got)
	}
	if got := strings.TrimSpace(gitOut(t, dir, "branch", "--list", "feat-up")); got == "" {
		t.Errorf("本地分支不该被一起删掉")
	}
}

// 远程分支右键菜单：删除远程分支（带确认）。
func TestDeleteRemoteBranchDialog(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	dir := a.repoPath

	runGit(t, dir, "branch", "feat-remote")
	runGit(t, dir, "push", "-u", "origin", "feat-remote")
	runGit(t, dir, "branch", "-d", "feat-remote") // 删掉本地分支，侧栏只保留远程那条
	a.loadRefs()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "feat-remote")
	if err := tt.ChooseMenuItem("删除远程分支 origin/feat-remote…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("应该弹出删除远程分支确认框")
	}
	if !tt.HasText("删除远程分支 origin/feat-remote") {
		t.Errorf("确认框标题不对：%q", tt.Texts())
	}

	if err := tt.Click("删除远程分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := strings.TrimSpace(gitOut(t, bare, "branch", "--list", "feat-remote")); got != "" {
		t.Errorf("远程分支该被删除了，实际：%q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已删除远程分支 origin/feat-remote") {
		t.Errorf("应该播报删除远程分支：%q", tt.Announcements())
	}
}
