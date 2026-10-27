package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 储藏对话框：储藏工作区改动（可选输入备注信息、可选包含未跟踪文件）。

type stashState struct {
	on               bool
	message          string
	includeUntracked bool
	focusMessage     bool
	err              gitrepo.Failure
}

func (a *app) openStashDialog() {
	if !a.repoReady() || a.op != gitrepo.OpNone {
		return
	}
	a.stashDlg = stashState{on: true, includeUntracked: true, focusMessage: true}
}

func (a *app) stashDialog(c *ui.Context, pal palette) {
	if !a.stashDlg.on {
		return
	}
	open := true
	// 表单对话框：点背景不关（见 specModalSticky）。
	specModalSticky(c, pal, &open, func() { a.stashDialogBody(c, pal) })
	if !open {
		a.stashDlg = stashState{}
	}
}

func (a *app) stashDialogBody(c *ui.Context, pal palette) {
	t := c.Theme()
	s := &a.stashDlg

	ui.Column(c).Width(460).Gap(12).Children(func() {
		ui.Text(c, "储藏工作区改动").FontSize(textHeadline).FontWeight(weightSemibold)
		ui.Text(c, "将手头未提交的修改存入储藏栈，工作区与暂存区恢复干净。").
			FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "说明（可选）").FontSize(11).TextColor(t.TextMuted)
			in := ui.TextInput(c, &s.message).Placeholder("例如：重构前的临时改动").Label("储藏说明")
			if s.focusMessage {
				in.Focus()
				s.focusMessage = false
			}
			if in.Submitted() {
				a.saveStash()
			}
		})

		ui.Checkbox(c, &s.includeUntracked, "包含未跟踪的新建文件（-u）").Label("包含未跟踪文件")
		ui.Text(c, "勾上会把尚未 git add 的新建文件一并收进储藏；不勾则只储藏已跟踪文件的修改。").
			FontSize(11).TextColor(t.TextMuted)

		failureBox(c, pal, t, s.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, "取消", "", false).Clicked() {
				a.stashDlg = stashState{}
				return
			}
			btn := primaryAction(c, pal, "保存储藏", "把当前改动保存到储藏", a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.saveStash()
			}
		})
	})
}

func (a *app) saveStash() {
	s := &a.stashDlg
	if !s.on || a.op != gitrepo.OpNone {
		return
	}
	msg := strings.TrimSpace(s.message)
	includeUntracked := s.includeUntracked
	s.err = gitrepo.Failure{}

	a.startOp(gitrepo.OpStash, "正在保存储藏…",
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已保存到储藏", repo.StashPush(ctx, msg, includeUntracked)
		}, func(ok bool) {
			if !ok {
				if a.opErr != nil {
					s.err = *a.opErr
					a.opErr = nil
				}
				return
			}
			a.stashDlg = stashState{}
			a.reloadAfterOp()
		})
}
