package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 储藏（Stash）和修改上次提交（Amend）在界面层的验收。

// 储藏工作区改动对话框：保存储藏、包含未跟踪文件、说明文字。
func TestStashSaveModal(t *testing.T) {
	dir := dirtyRepoDir(t)
	writeFile(t, filepath.Join(dir, "untracked.txt"), "未跟踪的新文件\n")
	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 从命令栏找到并打开储藏对话框
	a.openPalette()
	a.palette.query = "储藏"
	tt.Frame()
	if !tt.HasText("储藏工作区改动…") {
		t.Fatalf("命令栏里应该能搜到储藏命令：%q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if !a.stashDlg.on {
		t.Fatal("应该打开储藏对话框")
	}
	if !tt.HasText("储藏工作区改动") {
		t.Errorf("应该显示对话框标题：%q", tt.Texts())
	}
	if !tt.HasText("包含未跟踪的新建文件（-u）") {
		t.Errorf("应该有未跟踪文件勾选框说明：%q", tt.Texts())
	}

	a.stashDlg.message = "正在重构的临时改动"
	tt.Frame()

	if err := tt.Click("保存储藏"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.stashDlg.on {
		t.Error("储藏成功后对话框应该关闭")
	}

	// 验收回到 git：工作区变干净、stash list 有刚存的记录
	status := gitOut(t, dir, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Errorf("工作区应该恢复干净，实际 status:\n%s", status)
	}

	stashes := gitOut(t, dir, "stash", "list")
	if !strings.Contains(stashes, "正在重构的临时改动") {
		t.Errorf("git stash list 里应该包含储藏说明，实际：%q", stashes)
	}

	if !hasAnnouncement(tt.Announcements(), "已保存到储藏") {
		t.Errorf("应该播报保存储藏成功：%q", tt.Announcements())
	}
}

// 在干净工作区尝试储藏：Git 报错并提示，对话框保留并展示错误横幅。
func TestStashSaveModalCleanRepoShowsFailure(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	a.openStashDialog()
	tt.Frame()

	if err := tt.Click("保存储藏"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.stashDlg.on {
		t.Error("储藏失败时对话框应该保留")
	}
	if !tt.HasText("没有未提交的改动可储藏") {
		t.Errorf("应该提示没有未提交改动：%q", tt.Texts())
	}
}

// 侧栏储藏右键菜单：包含应用、取出、丢弃、复制储藏名。
func TestStashContextMenuItems(t *testing.T) {
	a := stashApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	items := rightClick(t, tt, "stash@{0}")
	for _, want := range []string{
		"应用储藏（Apply）",
		"应用并丢弃（Pop）",
		"丢弃储藏（Drop）…",
		"复制储藏名",
	} {
		if !menuHas(items, want) {
			t.Errorf("储藏右键菜单里缺少 %q，实际：%v", want, items)
		}
	}

	if err := tt.ChooseMenuItem("复制储藏名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != "stash@{0}" {
		t.Errorf("剪贴板 = %q, want stash@{0}", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已复制 stash@{0}") {
		t.Errorf("该播报复制了什么：%q", tt.Announcements())
	}
}

// 应用储藏（Apply）：工作区恢复改动，储藏本身依然保留在列表中。
func TestStashApply(t *testing.T) {
	a := stashApp(t)
	dir := a.repoPath
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "stash@{0}")
	if err := tt.ChooseMenuItem("应用储藏（Apply）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	// 储藏还在 stash list 里
	stashes := gitOut(t, dir, "stash", "list")
	if !strings.Contains(stashes, "stash@{0}") {
		t.Errorf("apply 之后储藏不该被删除，实际 stash list：%q", stashes)
	}

	// 工作区恢复了 a.txt 的改动
	content, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "攒了一半的一行") {
		t.Errorf("储藏的改动没有应用到 a.txt：%q", string(content))
	}
}

// 取出储藏（Pop）：工作区恢复改动，储藏从列表中删除。
func TestStashPop(t *testing.T) {
	a := stashApp(t)
	dir := a.repoPath
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "stash@{0}")
	if err := tt.ChooseMenuItem("应用并丢弃（Pop）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	// 储藏已从 stash list 移除
	stashes := gitOut(t, dir, "stash", "list")
	if strings.TrimSpace(stashes) != "" {
		t.Errorf("pop 之后储藏应该被丢弃，实际 stash list：%q", stashes)
	}

	// 工作区恢复了 a.txt 的改动
	content, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "攒了一半的一行") {
		t.Errorf("储藏的改动没有应用到 a.txt：%q", string(content))
	}
}

// 丢弃储藏（Drop）：弹出确认对话框，确认后彻底删除。
func TestStashDropPromptAndConfirm(t *testing.T) {
	a := stashApp(t)
	dir := a.repoPath
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "stash@{0}")
	if err := tt.ChooseMenuItem("丢弃储藏（Drop）…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("应该弹出二次确认对话框")
	}
	if !tt.HasText("丢弃储藏 stash@{0}") {
		t.Errorf("确认框标题不对：%q", tt.Texts())
	}
	if !tt.HasText("丢弃后该储藏的改动将无法找回。") {
		t.Errorf("确认框说明不对：%q", tt.Texts())
	}

	if err := tt.Click("丢弃储藏"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	stashes := gitOut(t, dir, "stash", "list")
	if strings.TrimSpace(stashes) != "" {
		t.Errorf("确认丢弃后储藏应该不存在，实际 stash list：%q", stashes)
	}
}

// 修改上次提交（Amend）：勾选修改上次提交，预填上次提交信息，提交后提交总数不变。
func TestCommitAmend(t *testing.T) {
	dir := dirtyRepoDir(t)
	// dirtyRepoDir 已经有一个初始提交 "init"，且 a.go 有未暂存改动
	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if !a.hasHead {
		t.Fatal("有初始提交应该 hasHead == true")
	}

	// 先把 a.go 暂存起来
	a.stageAll()
	settle(t, a)
	tt.Frame()

	if n := a.stagedCount(); n != 1 {
		t.Fatalf("应该有 1 个暂存文件，实际 %d", n)
	}

	// 勾选修改上次提交
	a.toggleAmend(true)
	settle(t, a)
	tt.Frame()

	if !a.amend {
		t.Fatal("应该进入 amend 状态")
	}
	if a.commitMsg != "init" {
		t.Errorf("应该自动预填上次提交的信息 init，实际：%q", a.commitMsg)
	}
	if !tt.HasText("修改上次提交 (1 个文件)") {
		t.Errorf("提交按钮应该反映修改状态：%q", tt.Texts())
	}

	// 修改提交信息
	a.commitMsg = "init amended with extra code"
	tt.Frame()

	if err := tt.Click("修改上次提交 (1 个文件)"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	// 验收回到 git：提交数依然只有 1，且消息更新
	logLines := strings.Split(gitOut(t, dir, "log", "--oneline"), "\n")
	if len(logLines) != 1 {
		t.Errorf("amend 后的提交数应该保持为 1，实际有 %d 条提交：\n%s", len(logLines), strings.Join(logLines, "\n"))
	}
	lastMsg := gitOut(t, dir, "log", "-1", "--format=%s")
	if lastMsg != "init amended with extra code" {
		t.Errorf("提交信息应该更新，实际：%q", lastMsg)
	}
	if a.amend {
		t.Error("提交成功后 amend 状态应该复位为 false")
	}
}

// 仅修改提交信息（无暂存文件时 Amend）：允许直接提交更新信息。
func TestCommitAmendWithoutStagedFiles(t *testing.T) {
	a := branchApp(t)
	dir := a.repoPath
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	beforeCount := len(strings.Split(gitOut(t, dir, "log", "--oneline"), "\n"))

	a.toggleAmend(true)
	settle(t, a)
	tt.Frame()

	if !tt.HasText("未暂存新改动，仅修改上次提交的信息。") {
		t.Errorf("应该提示仅修改上次提交的信息：%q", tt.Texts())
	}
	if !a.canCommit() {
		t.Error("无暂存文件但开启 amend 时应该允许提交")
	}

	a.commitMsg = "只改了提交说明，没改任何代码"
	tt.Frame()

	if err := tt.Click("修改上次提交"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	afterCount := len(strings.Split(gitOut(t, dir, "log", "--oneline"), "\n"))
	if afterCount != beforeCount {
		t.Errorf("提交数不应增加：before=%d, after=%d", beforeCount, afterCount)
	}
	lastMsg := gitOut(t, dir, "log", "-1", "--format=%s")
	if lastMsg != "只改了提交说明，没改任何代码" {
		t.Errorf("提交信息应该更新，实际：%q", lastMsg)
	}
}
