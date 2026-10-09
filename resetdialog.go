package main

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 重置：提交行右键 →「重置 main 到 a1b2c3d…」。
//
// 单独一张对话框而不是三颗菜单项（软/混合/硬各一条），理由是那三档的差别
// **只有解释能说清**：同样一句「重置到 a1b2c3d」，软的重置完改动在暂存区、
// 混合的在工作区、硬的连工作区一起抹掉 —— 菜单上光写档名，用户得靠背。
// 摆在一张卡里，选中哪档、哪档干什么，一眼对得上。
//
// 这里也是「硬重置」唯一的一道闸：它会丢掉没提交的改动，而 git 不会拦、
// 不会问。打开时先数一遍工作区里有多少文件会被丢，选中硬重置就写在脸上。

// resetState 是重置对话框的状态。形状照 tagdialog.go：字段挂在 app 上，
// 失败时对话框不关，改一下再点一次。
type resetState struct {
	on     bool
	hash   string // 要重置到的提交（完整哈希）
	short  string // 它在界面上的短名字
	target string // 「重置谁」：分支名，分离 HEAD 时是 HEAD
	mode   gitrepo.ResetMode
	// dirty 是工作区里带着未提交改动的文件数（不含未跟踪）。只在选中
	// 硬重置时说话 —— 软/混合都不碰工作区里的改动。
	dirty int
	err   gitrepo.Failure
}

// openResetDialog 打开重置对话框。默认档是混合：git 自己的默认，
// 也是「撤下来重新挑挑」最常用的那档。
func (a *app) openResetDialog(cm gitrepo.Commit) {
	if !a.repoReady() || a.op != gitrepo.OpNone || a.pending.Active() {
		return
	}
	a.reset = resetState{
		on: true, hash: cm.Hash, short: cm.Short,
		target: a.commitMenuBranch(), mode: gitrepo.ResetMixed,
		dirty: a.dirtyFileCount(),
	}
}

// dirtyFileCount 数「硬重置会丢掉的改动」：工作区里有未提交改动的文件数。
//
// 不算未跟踪（reset --hard 不碰它们 —— 说成「N 个文件会丢」而其中一半是
// 未跟踪的，这句警告就成了吓唬）。按路径去重：一个文件同时带着暂存和未
// 暂存的改动时在列表里出现两次，但那是一个文件。
func (a *app) dirtyFileCount() int {
	seen := map[string]bool{}
	for _, f := range a.workFiles {
		if f.Status == gitrepo.StatusUntracked {
			continue
		}
		seen[pathOf(f)] = true
	}
	return len(seen)
}

// resetDialog 是对话框入口，画在 view 的对话框那一层。
func (a *app) resetDialog(c *ui.Context, pal palette) {
	if !a.reset.on {
		return
	}
	open := true
	specModal(c, pal, &open, func() { a.resetBody(c, pal) })
	if !open {
		a.reset = resetState{}
	}
}

// resetExplain 是选中那一档的一句话说明。
func resetExplain(mode gitrepo.ResetMode) string {
	switch mode {
	case gitrepo.ResetSoft:
		return "分支挪过去，改动原样留在暂存区 —— 相当于把这几个提交撤下来重新组织，东西一点不丢。"
	case gitrepo.ResetHard:
		return "分支、暂存区、工作区一起回到那个提交：这之后的改动如果没有提交，会丢。"
	}
	return "分支挪过去，改动留在工作区、暂存区清空（git 的默认档）—— 想重新挑哪些进下一次提交，用这档。"
}

func (a *app) resetBody(c *ui.Context, pal palette) {
	t := c.Theme()
	rs := &a.reset

	ui.Column(c).Width(460).Gap(12).Children(func() {
		ui.Text(c, "重置 "+rs.target+" 到 "+rs.short).FontSize(textHeadline).FontWeight(weightSemibold)
		ui.Text(c, "只挪这条分支的指向，提交本身不会丢 —— 弄错了随时能重置回去。").
			FontSize(11).TextColor(t.TextMuted)

		ui.Column(c).Gap(6).Children(func() {
			ui.Radio(c, &rs.mode, gitrepo.ResetSoft, "软重置（--soft）")
			ui.Radio(c, &rs.mode, gitrepo.ResetMixed, "混合重置（--mixed）")
			ui.Radio(c, &rs.mode, gitrepo.ResetHard, "硬重置（--hard）")
		})
		ui.Text(c, resetExplain(rs.mode)).FontSize(11).TextColor(t.TextMuted)

		// 硬重置 + 工作区有东西要丢：把数字写在按钮之上。dirty 是在打开
		// 对话框那一刻数的，够新 —— 期间仓库被动过的话，重置本身会重读。
		if rs.mode == gitrepo.ResetHard && rs.dirty > 0 {
			ui.Text(c, hardResetWarning(rs.dirty)).FontSize(12).TextColor(t.Danger)
		}

		failureBox(c, pal, t, rs.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, "取消", "", false).Clicked() {
				a.reset = resetState{}
				return
			}
			// 按钮上写全档名（「硬重置」），不写「确定」：档位是这个
			// 对话框唯一的选择，按钮把那句话再说一遍，用户不用回头找。
			if primaryAction(c, pal, resetButtonLabel(rs.mode), "把这颗按钮做的事说全，它就是选中的那一档", a.op != gitrepo.OpNone).Clicked() {
				a.resetTo()
			}
		})
	})
}

// hardResetWarning 是硬重置前面那句「会丢什么」。
func hardResetWarning(n int) string {
	if n == 1 {
		return "工作区里有 1 个文件带着没提交的改动，会一起丢（未跟踪的文件不动）。"
	}
	return "工作区里有 " + strconv.Itoa(n) + " 个文件带着没提交的改动，会一起丢（未跟踪的文件不动）。"
}

// resetButtonLabel 是确认键上的字：选中的那一档。
func resetButtonLabel(mode gitrepo.ResetMode) string {
	switch mode {
	case gitrepo.ResetSoft:
		return "软重置"
	case gitrepo.ResetHard:
		return "硬重置"
	}
	return "混合重置"
}

// resetTo 执行重置。
func (a *app) resetTo() {
	rs := &a.reset
	if !rs.on || a.op != gitrepo.OpNone {
		return
	}
	hash, short, target, mode := rs.hash, rs.short, rs.target, rs.mode
	rs.err = gitrepo.Failure{}

	note := "已重置 " + target + " 到 " + short + "，改动留在工作区"
	switch mode {
	case gitrepo.ResetSoft:
		note = "已重置 " + target + " 到 " + short + "，改动留在暂存区"
	case gitrepo.ResetHard:
		note = "已硬重置 " + target + " 到 " + short
	}

	a.startOp(gitrepo.OpReset, "正在重置 "+target+" 到 "+short,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return note, repo.ResetTo(ctx, hash, mode)
		}, func(ok bool) {
			if !ok {
				// 失败留在对话框里（和打标签一个做法）：横幅那份搬进来，
				// 一句话不说两遍。
				if a.opErr != nil {
					rs.err = *a.opErr
					a.opErr = nil
				}
				return
			}
			a.reset = resetState{}
			// 重置改的是工作区眼前的内容，收货地点在「改动」栏。
			a.finishCheckout()
			a.reloadAfterOp()
		})
}
