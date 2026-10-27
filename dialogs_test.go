package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 表单类对话框点压暗的背景不关：打到一半的内容不该被误触丢掉。
//
// 规则（见 specModalSticky）：表单类粘住，确认框 / 纯查看的（文件历史、风险
// 预检）照旧点背景即关。 Esc 是显式关闭，每条用例末尾都验一遍还在关。
func TestFormDialogsSurviveBackdropClick(t *testing.T) {
	cases := []struct {
		name string
		open func(a *app)
		isOn func(a *app) bool
	}{
		{"新建分支", func(a *app) { a.openNewBranchDialog("", "HEAD") }, func(a *app) bool { return a.newBranch.on }},
		{"新建标签", func(a *app) { a.openTagDialog("", "HEAD") }, func(a *app) bool { return a.tag.on }},
		{"储藏", func(a *app) { a.openStashDialog() }, func(a *app) bool { return a.stashDlg.on }},
		{"重命名分支", func(a *app) { a.openRenameBranchDialog("main") }, func(a *app) bool { return a.rename.on }},
		{"对比分支", func(a *app) { a.openCompareDialog("") }, func(a *app) bool { return a.compare.dialog }},
		{"模型设置", func(a *app) { a.openSettings() }, func(a *app) bool { return a.settingsOn }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := richApp(t)
			tt := ui.NewTester(a.view, 1280, 860)
			tt.Frame()
			settle(t, a)
			tt.Frame()

			c.open(a)
			tt.Frame()
			if !c.isOn(a) {
				t.Fatalf("%s 对话框没打开", c.name)
			}

			// 面板居中，左上角这一下落在压暗的背景上。
			tt.ClickAt(4, 4)
			tt.Frame()

			if !c.isOn(a) {
				t.Fatalf("点背景不该关掉%s对话框", c.name)
			}

			// Esc 是显式关闭，照旧关。
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if c.isOn(a) {
				t.Errorf("Esc 该关掉%s对话框", c.name)
			}
		})
	}
}

// 「编辑代码」对话框：没保存的改动不该被背景上误触的一下丢掉。
func TestCodeEditorDialogSurvivesBackdropClick(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	a.openCodeEditor(listedFile(t, a, "README.md"))
	settle(t, a)
	if !a.codeEdit.on {
		t.Fatalf("编辑器没打开：%q", a.codeEdit.err)
	}
	a.codeEdit.buffer.Set("打到一半的改动\n")
	tt.Frame()

	tt.ClickAt(4, 4)
	tt.Frame()

	if !a.codeEdit.on {
		t.Fatal("点背景不该关掉编辑器")
	}
	if got := a.codeEdit.buffer.String(); got != "打到一半的改动\n" {
		t.Errorf("缓冲区不该被丢掉：%q", got)
	}
	if !a.codeEdit.buffer.Modified() {
		t.Error("改动状态不该被抹掉")
	}

	// Esc 照旧关。
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.codeEdit.on {
		t.Error("Esc 该关掉编辑器")
	}
}

// 反方向也钉住：确认框和纯查看的对话框点背景照旧关。
//
// 「表单类粘住」不是「全都粘住」—— 确认框点背景本来就是「取消」，文件历史和
// 风险预检只是看，没有会丢的东西。哪一天有人把它们也改成粘的，这条会红。
func TestConfirmAndViewerDialogsStillCloseOnBackdropClick(t *testing.T) {
	cases := []struct {
		name string
		open func(a *app)
		isOn func(a *app) bool
	}{
		{"确认框", func(a *app) {
			a.askConfirm(confirmState{title: "删掉这条分支？", body: "删掉就找不回来了。", yes: "删除"})
		}, func(a *app) bool { return a.confirm.on }},
		{"重置", func(a *app) { a.openResetDialog(gitrepo.Commit{Hash: "abc1234"}) }, func(a *app) bool { return a.reset.on }},
		{"文件历史", func(a *app) { a.openFileHistory(listedFile(t, a, "README.md")) }, func(a *app) bool { return a.fileHist.on }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := richApp(t)
			tt := ui.NewTester(a.view, 1280, 860)
			tt.Frame()
			settle(t, a)
			tt.Frame()

			c.open(a)
			tt.Frame()
			if !c.isOn(a) {
				t.Fatalf("%s 没打开", c.name)
			}

			tt.ClickAt(4, 4)
			tt.Frame()
			if c.isOn(a) {
				t.Errorf("点背景该关掉%s", c.name)
			}
		})
	}
}
