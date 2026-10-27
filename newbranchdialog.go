package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/explain"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/llm"
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

	// AI 分支名生成状态
	aiLoading bool
	aiCancel  context.CancelFunc
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
//
// 走 specModalSticky：点压暗的背景不关 —— 名字打了一半，误触一下下面的背景
// 就把打的内容连同步取消一起丢了。Esc 和「取消」照旧关。
func (a *app) newBranchDialog(c *ui.Context, pal palette) {
	if !a.newBranch.on {
		return
	}
	open := true
	specModalSticky(c, pal, &open, func() { a.newBranchBody(c, pal) })
	if !open {
		a.cancelAIBranch()
		a.newBranch = branchState{}
	}
}

func (a *app) newBranchBody(c *ui.Context, pal palette) {
	t := c.Theme()
	bs := &a.newBranch

	ui.Column(c).Width(460).Gap(12).Children(func() {
		ui.Text(c, "新建分支").FontSize(textHeadline).FontWeight(weightSemibold)
		ui.Text(c, "从 "+bs.revLabel+" 起步").FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "名称").FontSize(11).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				// placeholder 把两种用法都摆出来：这个框既是分支名，也是「意图」的输入位
				// —— ✨ 会把框里写的内容当成需求描述喂给模型（见 startAIBranch）。
				in := ui.TextInput(c, &bs.name).Placeholder("feature/login 或 修复登录超时").Label("分支名称").Grow(1)
				if bs.focusName {
					in.Focus()
					bs.focusName = false
				}
				// 名字框里回车 = 点「创建分支」（和打标签一个做法）。
				if in.Submitted() {
					a.createBranch()
				}
				a.aiBranchButton(c, pal)
			})
			// 「输入一句意图 → AI 起名」这条用法原来只有按钮的 tooltip 里提了一句，
			// 框上又只写着 feature/login，等于藏起来了：写得出一整句话的人，
			// 恰恰是最需要这功能的人。
			ui.Text(c, "想不出名字就写一句要做什么（如「修复登录超时」），再点 ✨ 让 AI 起名；自己填也行。").
				FontSize(11).TextColor(t.TextMuted)
		})

		ui.Checkbox(c, &bs.switchTo, "建好就切过去").Label("建好就切过去")
		ui.Text(c, "勾上就把 HEAD 挪到新分支（工作区的改动跟着过去）；不勾只建一条，停在这条分支上不动。").
			FontSize(11).TextColor(t.TextMuted)

		if msg := branchBlocker(bs); msg != "" {
			ui.Text(c, msg).FontSize(11).TextColor(t.TextMuted)
		}
		failureBox(c, pal, t, bs.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, "取消", "", false).Clicked() {
				a.cancelAIBranch()
				a.newBranch = branchState{}
				return
			}
			btn := primaryAction(c, pal, "创建分支", "从 "+bs.revLabel+" 新建一条分支", branchBlocker(bs) != "" || a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.createBranch()
			}
		})
	})
}

// aiBranchButton 是「✨ AI 建议分支名」按钮。
//
// 走 §4.1 的次级按钮：Surface 1 底 + hairline 边 + Ink Muted 文字。这个
// 对话框里没有主操作键（「创建」走的是回车），所以它不该是蓝的 ——
// 生成中才用强调色文字说明「正在跑」。
func (a *app) aiBranchButton(c *ui.Context, pal palette) {
	t := c.Theme()
	bs := &a.newBranch
	tip := "✨ AI 建议分支名"
	if bs.aiLoading {
		tip = "正在生成建议… 点击取消"
	}
	btn := ui.ButtonBase(c).Height(28).Shrink(0).Radius(radiusMD).
		Padding(0, 10).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label("✨ AI 建议分支名").Tooltip(tip)

	hovered := btn.Hovered()
	var bg, border, fg ui.Color
	if bs.aiLoading {
		bg, border, fg = pal.surface2, pal.hairlineStrong, t.Accent
	} else if hovered {
		bg, border, fg = pal.surface3, pal.hairlineStrong, pal.ink
	} else {
		bg, border, fg = pal.surface1, pal.hairline, pal.inkMuted
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		if bs.aiLoading {
			ui.Spinner(c)
			ui.Text(c, "推荐中…").FontSize(textButton).FontWeight(weightMedium).TextColor(fg)
		} else {
			sparkleIcon(c, sparkleSize, fg)
			ui.Text(c, "✨ AI 建议分支名").FontSize(textButton).
				FontWeight(weightMedium).TextColor(fg)
		}
	})

	if btn.Clicked() {
		if bs.aiLoading {
			a.cancelAIBranch()
		} else {
			a.startAIBranch()
		}
	}
}

// startAIBranch 发起 AI 分支名推荐请求。
func (a *app) startAIBranch() {
	bs := &a.newBranch
	if bs.aiLoading {
		a.cancelAIBranch()
		return
	}
	if !a.store.Provider().Ready() {
		a.openSettings()
		return
	}
	a.cancelAIBranch()

	ctx, cancel := context.WithCancel(context.Background())
	bs.aiCancel = cancel
	bs.aiLoading = true

	hint := strings.TrimSpace(bs.name)
	var diffs []diff.FileDiff
	files := a.allFiles
	if len(files) == 0 && len(a.workFiles) > 0 {
		files = a.workFiles
	}
	for _, f := range files {
		diffs = append(diffs, f.FileDiff)
	}

	system, user := explain.BuildBranchPrompt(hint, diffs)
	system = a.store.Config().EffectiveBranchPrompt(system)
	prov := a.store.Provider()

	a.run(func() {
		text, err := llm.StreamExplanation(ctx, prov, system, user, nil)
		a.dispatch(func() {
			if !a.newBranch.on || a.newBranch.aiCancel == nil {
				return
			}
			a.newBranch.aiLoading = false
			a.newBranch.aiCancel = nil
			if err != nil {
				if ctx.Err() == nil {
					a.newBranch.err = gitrepo.Failure{
						Title: "AI 推荐分支名失败",
						Hint:  llm.Describe(err).Hint,
					}
				}
				return
			}
			cleaned := explain.CleanBranchName(text)
			if cleaned != "" {
				a.newBranch.name = cleaned
			}
		})
	})
}

// cancelAIBranch 取消进行中的 AI 分支名生成。
func (a *app) cancelAIBranch() {
	if a.newBranch.aiCancel != nil {
		a.newBranch.aiCancel()
		a.newBranch.aiCancel = nil
	}
	a.newBranch.aiLoading = false
}

// branchBlocker 是「还差什么才能创建」。
//
// 和 tagBlocker 一个脾气：名字没开始填时一声不吭（空框加 placeholder 已经
// 说清了要干什么）；填了但一眼看着不对，就把那句话摆出来。空名字点下去
// 由 git 拒绝（” is not a valid branch name →「这个名字不能当分支名」），
// 不在这里预判。
func branchBlocker(bs *branchState) string {
	if bs.aiLoading {
		return "正在生成分支名建议…"
	}
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
			a.cancelAIBranch()
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
