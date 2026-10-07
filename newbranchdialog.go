package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 新建分支：提交行右键 →「从 a1b2c3d 建分支…」（「文件」菜单里那条
//「新建分支…」也走这里，rev 传空 = 从 HEAD 起步）。
//
// 和打标签（tagdialog.go）是同一形状的表单：名字、一个勾选、两颗按钮。
// 名字的本地体检查的是 refNameProblem —— 和标签共用一份，git 对这两类
// 名字的规矩本来就是一回事。

// branchState 是新建分支对话框的状态。
type branchState struct {
	on        bool
	name      string
	switchTo  bool   // 建好就切过去
	rev       string // 从哪个提交建（完整哈希）；空 = HEAD
	revLabel  string // 那个提交在界面上叫什么
	focusName bool   // 还欠名称框一次焦点
	err       gitrepo.Failure
}

// openNewBranchDialog 打开对话框。rev 为空表示从 HEAD 起步。
func (a *app) openNewBranchDialog(rev, revLabel string) {
	if !a.repoReady() || a.op != gitrepo.OpNone {
		return
	}
	if revLabel == "" {
		revLabel = "HEAD"
	}
	a.newBranch = branchState{on: true, switchTo: true, rev: rev, revLabel: revLabel, focusName: true}
}

// newBranchAtHead 是「文件」菜单里那条「新建分支…」：从 HEAD 起步。
//
// rev 传空、让 git 自己去解析 HEAD：我们手上那份提交列表可能是某条分支的
// 历史，它排第一的未必是 HEAD（和 newTagAtHead 同一个道理）。
func (a *app) newBranchAtHead() {
	a.openNewBranchDialog("", a.commitMenuBranch())
}

// newBranchDialog 是对话框入口。
func (a *app) newBranchDialog(c *ui.Context, pal palette) {
	if !a.newBranch.on {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.newBranchBody(c, pal) })
	if !open {
		a.newBranch = branchState{}
	}
}

func (a *app) newBranchBody(c *ui.Context, pal palette) {
	t := c.Theme()
	bs := &a.newBranch

	ui.Column(c).Width(460).Gap(12).Children(func() {
		ui.Text(c, "新建分支").Bold().FontSize(15)
		ui.Text(c, "从 "+bs.revLabel+" 起步").FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "名称").FontSize(11).TextColor(t.TextMuted)
			in := ui.TextInput(c, &bs.name).Placeholder("feature/login").Label("分支名称")
			if bs.focusName {
				in.Focus()
				bs.focusName = false
			}
			// 名字框里回车 = 点「创建分支」（和打标签一个做法）。
			if in.Submitted() {
				a.createBranch()
			}
		})

		ui.Checkbox(c, &bs.switchTo, "建好就切过去").Label("建好就切过去")
		ui.Text(c, "勾上就把 HEAD 挪到新分支（工作区的改动跟着过去）；不勾只建一条，停在这条分支上不动。").
			FontSize(11).TextColor(t.TextMuted)

		if msg := branchBlocker(bs); msg != "" {
			ui.Text(c, msg).FontSize(11).TextColor(t.TextMuted)
		}
		failureBox(c, t, bs.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.newBranch = branchState{}
				return
			}
			btn := ui.PrimaryButton(c, "创建分支").
				Tooltip("从 " + bs.revLabel + " 新建一条分支").
				Disabled(branchBlocker(bs) != "" || a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.createBranch()
			}
		})
	})
}

// branchBlocker 是「还差什么才能创建」。
//
// 和 tagBlocker 一个脾气：名字没开始填时一声不吭（空框加 placeholder 已经
// 说清了要干什么）；填了但一眼看着不对，就把那句话摆出来。空名字点下去
// 由 git 拒绝（” is not a valid branch name →「这个名字不能当分支名」），
// 不在这里预判。
func branchBlocker(bs *branchState) string {
	if name := strings.TrimSpace(bs.name); name != "" {
		return refNameProblem(name)
	}
	return ""
}

// createBranch 建分支。
func (a *app) createBranch() {
	bs := &a.newBranch
	if !bs.on || branchBlocker(bs) != "" || a.op != gitrepo.OpNone {
		return
	}
	name := strings.TrimSpace(bs.name)
	rev, switchTo := bs.rev, bs.switchTo
	bs.err = gitrepo.Failure{}

	note := "已新建分支 " + name
	if switchTo {
		note = "已新建并切换到 " + name
	}

	a.startOp(gitrepo.OpBranch, "正在新建分支 "+name,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return note, repo.CreateBranch(ctx, name, rev, switchTo)
		}, func(ok bool) {
			if !ok {
				// 失败留在对话框里：用户在改的就是那个名字
				//（重名、非法字符都是改一改就能过的），关掉让他重打一遍
				// 是对小事的过度惩罚。
				if a.opErr != nil {
					bs.err = *a.opErr
					a.opErr = nil
				}
				return
			}
			a.newBranch = branchState{}
			if switchTo {
				// 切过去了 = 换过 HEAD，收尾和检出一样
				a.finishCheckout()
				a.reloadAfterOp()
				return
			}
			// 没切：工作区一点没动，只需要侧栏里多出这一条
			a.loadRefs()
		})
}
