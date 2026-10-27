package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 重命名分支对话框：侧栏本地分支右键 →「重命名 <name>…」。
//
// 和新建分支（newbranchdialog.go）是同一套表单骨架：名字输入、体检、失败说明、两颗按钮。
// 名字体检查的是 refNameProblem —— 统一的体检机制，重名或非法字符由 git 兜底报错并翻成人话。

type renameState struct {
	on        bool
	oldName   string
	newName   string
	focusName bool
	err       gitrepo.Failure
}

// openRenameBranchDialog 打开重命名分支对话框。
func (a *app) openRenameBranchDialog(oldName string) {
	if !a.repoReady() || a.op != gitrepo.OpNone {
		return
	}
	a.rename = renameState{
		on:        true,
		oldName:   oldName,
		newName:   oldName,
		focusName: true,
	}
}

// renameBranchDialog 是对话框入口。
func (a *app) renameBranchDialog(c *ui.Context, pal palette) {
	if !a.rename.on {
		return
	}
	open := true
	// 表单对话框：点背景不关 —— 新名字打到一半不该被误触丢掉（见 specModalSticky）。
	specModalSticky(c, pal, &open, func() { a.renameBranchBody(c, pal) })
	if !open {
		a.rename = renameState{}
	}
}

func (a *app) renameBranchBody(c *ui.Context, pal palette) {
	t := c.Theme()
	rs := &a.rename

	ui.Column(c).Width(440).Gap(12).Children(func() {
		ui.Text(c, "重命名分支").FontSize(textHeadline).FontWeight(weightSemibold)
		ui.Text(c, "将 "+rs.oldName+" 改名为").FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "新名称").FontSize(11).TextColor(t.TextMuted)
			in := ui.TextInput(c, &rs.newName).Placeholder(rs.oldName).Label("分支名称")
			if rs.focusName {
				in.Focus()
				rs.focusName = false
			}
			if in.Submitted() {
				a.renameBranch()
			}
		})

		if msg := renameBlocker(rs); msg != "" {
			ui.Text(c, msg).FontSize(11).TextColor(t.TextMuted)
		}
		failureBox(c, pal, t, rs.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, "取消", "", false).Clicked() {
				a.rename = renameState{}
				return
			}
			btn := primaryAction(c, pal, "重命名", "将 "+rs.oldName+" 重命名为 "+strings.TrimSpace(rs.newName), renameBlocker(rs) != "" || a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.renameBranch()
			}
		})
	})
}

// renameBlocker 检查新名字是否有效。未修改或格式不符时拦截。
func renameBlocker(rs *renameState) string {
	name := strings.TrimSpace(rs.newName)
	if name == "" {
		return ""
	}
	if name == rs.oldName {
		return "名称未改动。"
	}
	return refNameProblem(name)
}

// renameBranch 执行分支重命名。
func (a *app) renameBranch() {
	rs := &a.rename
	if !rs.on || renameBlocker(rs) != "" || a.op != gitrepo.OpNone {
		return
	}
	oldName := rs.oldName
	newName := strings.TrimSpace(rs.newName)
	rs.err = gitrepo.Failure{}

	a.startOp(gitrepo.OpBranch, "正在重命名分支 "+oldName,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已将分支 " + oldName + " 重命名为 " + newName, repo.RenameBranch(ctx, oldName, newName)
		}, func(ok bool) {
			if !ok {
				if a.opErr != nil {
					rs.err = *a.opErr
					a.opErr = nil
				}
				return
			}
			a.rename = renameState{}
			if a.branch == oldName {
				a.branch = newName
				a.reloadAfterOp()
				return
			}
			a.loadRefs()
		})
}
