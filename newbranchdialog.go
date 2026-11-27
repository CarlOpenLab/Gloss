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
	// aiSuggested 是上一次 AI 给出的名字。名字框和「意图」共用一个输入位，
	// 再点一次时必须能认出框里这段字是 AI 自己的答案，而不是用户写的需求。
	aiSuggested string
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

var branchPrefixes = []string{"feat/", "fix/", "chore/", "refactor/"}

// applyBranchPrefix 为分支名套用、切换或取消前缀：
//   - 空串直接填入 prefix；
//   - 已有相同前缀时取消它；
//   - 已有其他常用前缀（含 feature/）时原地替换；
//   - 普通文字前直接拼上 prefix。
func applyBranchPrefix(name, prefix string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return prefix
	}
	if strings.HasPrefix(trimmed, prefix) {
		return strings.TrimPrefix(trimmed, prefix)
	}
	for _, p := range append([]string{"feature/"}, branchPrefixes...) {
		if strings.HasPrefix(trimmed, p) {
			return prefix + strings.TrimPrefix(trimmed, p)
		}
	}
	return prefix + trimmed
}

func (a *app) newBranchBody(c *ui.Context, pal palette) {
	t := c.Theme()
	bs := &a.newBranch
	blocker := branchBlocker(bs)
	nameErr := ""
	if !bs.aiLoading && strings.TrimSpace(bs.name) != "" {
		nameErr = refNameProblem(strings.TrimSpace(bs.name))
	}

	ui.Column(c).Width(480).Gap(14).Children(func() {
		// —— 顶栏：图标徽章 + 标题与起步提交胶囊 + 关闭键 ——
		ui.Row(c).AlignItems(ui.Start).Gap(12).FillWidth().Children(func() {
			ui.Box(c).Size(34, 34).Shrink(0).Radius(radiusMD).Center().
				Background(pal.chip).Border(1, pal.hairline).
				Children(func() {
					drawIcon(c, iconGitBranchPlus, 16, pal.inkMuted)
				})

			ui.Column(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "新建分支").FontSize(uiFontSize(textHeadline)).
					FontWeight(weightSemibold).TextColor(pal.ink)
				ui.Row(c).Shrink(1).MaxWidth(370).Padding(2, 8).Radius(radiusSM).
					Gap(5).AlignItems(ui.Center).
					Background(pal.chip).Border(1, pal.hairline).
					Children(func() {
						drawIcon(c, iconGitCommit, 11, pal.inkSubtle)
						ui.Text(c, "从 "+bs.revLabel+" 起步").FontSize(uiFontSize(textCaption)).
							SingleLine().MinWidth(0).TextColor(pal.inkMuted)
					})
			})

			if closeButton(c, pal) {
				a.cancelAIBranch()
				a.newBranch = branchState{}
				return
			}
		})

		ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.hairline)

		// —— 名称输入区：输入框与 AI 起名同高（34px）+ 常用前缀快选 ——
		ui.Column(c).Gap(8).FillWidth().Children(func() {
			ui.Text(c, "名称").FontSize(uiFontSize(textCaption)).
				FontWeight(weightMedium).TextColor(pal.inkMuted)

			ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
				var in ui.Element
				inputBox := ui.Row(c).Height(34).Grow(1).MinWidth(0).
					AlignItems(ui.Center).Gap(8).Padding(0, 10).Radius(radiusMD).
					Background(pal.chip)
				inputBox.Children(func() {
					drawIcon(c, iconGitBranch, 13, pal.inkSubtle)
					// placeholder 把两种用法都摆出来：这个框既是分支名，也是「意图」的输入位
					// —— AI 起名会把框里写的内容当成需求描述喂给模型（见 startAIBranch）。
					in = ui.TextInputBase(c, &bs.name).Grow(1).MinWidth(0).
						FontSize(uiFontSize(textBody)).
						Placeholder("feature/login 或 修复登录超时").Label("分支名称")
					if bs.focusName {
						in.Focus()
						bs.focusName = false
					}
					// 名字框里回车 = 点「创建分支」（和打标签一个做法）。
					if in.Submitted() {
						a.createBranch()
					}
					if bs.name != "" && !bs.aiLoading {
						clearBtn := ui.ButtonBase(c).Size(18, 18).Shrink(0).Radius(radiusPill).Center().
							Cursor(ui.CursorPointer).Label("清空分支名").Tooltip("清空")
						if clearBtn.Hovered() {
							clearBtn.Background(pal.rowSelected)
						}
						fadeColors(clearBtn)
						clearBtn.Children(func() {
							drawIcon(c, iconX, 10, pal.inkSubtle)
						})
						if clearBtn.Clicked() {
							bs.name = ""
							bs.aiSuggested = ""
							bs.focusName = true
						}
					}
				})

				border := pal.hairlineStrong
				switch {
				case nameErr != "":
					border = pal.danger
				case in.Valid() && in.Focused():
					border = t.Accent
				case inputBox.Hovered():
					border = pal.hairlineTertiary
				}
				inputBox.Border(1, border)

				a.aiBranchButton(c, pal)
			})

			// 常用分支前缀快选胶囊：一点即填/换前缀，减少手动敲斜杠。
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				for _, prefix := range branchPrefixes {
					p := prefix
					active := strings.HasPrefix(strings.TrimSpace(bs.name), p)
					chip := ui.ButtonBase(c).Height(22).Shrink(0).Padding(0, 8).Radius(radiusSM).
						AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("前缀 " + p)
					bg, border, fg := pal.chip, pal.hairline, pal.inkSubtle
					switch {
					case active:
						bg, border, fg = pal.selection, t.Accent.Alpha(0.45), t.Accent
					case chip.Hovered():
						bg, border, fg = pal.rowHover, pal.hairlineStrong, pal.ink
					}
					chip.Background(bg).Border(1, border)
					fadeColors(chip)
					chip.Children(func() {
						ui.Text(c, p).Font("monospace").FontSize(uiFontSize(textCaption)).
							FontWeight(weightMedium).TextColor(fg)
					})
					if chip.Clicked() {
						bs.name = applyBranchPrefix(bs.name, p)
						bs.focusName = true
					}
				}
			})

			// 校验或用法提示：同一行位置，不堆多段灰字。
			switch {
			case nameErr != "":
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					drawIcon(c, iconCircleAlert, 12, pal.danger)
					ui.Text(c, nameErr).FontSize(uiFontSize(textCaption)).TextColor(pal.danger)
				})
			case bs.aiLoading:
				ui.Text(c, "正在结合改动生成分支名建议…").
					FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			case strings.TrimSpace(bs.name) == "":
				ui.Text(c, "想不出名字就写一句要做什么，点「AI 起名」让 AI 想一个。").
					FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			}
		})

		// —— 选项卡片（内嵌洗色分组行）：整行可点，说明收纳在同一块里 ——
		optRow := ui.CheckboxBase(c, &bs.switchTo).FillWidth().Padding(10, 12).
			Radius(radiusMD).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).
			FocusRing(false).Label("建好就切过去").
			Tooltip("建好就把 HEAD 挪到新分支（工作区的改动跟着过去）；不勾只建一条，停在当前分支不动")
		optBg, optBorder := pal.chip, pal.hairline
		if optRow.Hovered() {
			optBg, optBorder = pal.rowHover, pal.hairlineStrong
		}
		optRow.Background(optBg).Border(1, optBorder)
		fadeColors(optRow)
		optRow.Children(func() {
			on := bs.switchTo
			box := ui.Box(c).Size(16, 16).Radius(radiusXS).Shrink(0)
			switch {
			case on:
				box.Background(t.Accent).Border(1, t.Accent)
			case optRow.Hovered():
				box.Background(pal.rowSelected).Border(1, pal.hairlineTertiary)
			default:
				box.Background(pal.chip).Border(1, pal.hairlineStrong)
			}
			fadeColors(box)
			box.DrawOver(func(p *ui.Painter, r ui.Rect) {
				if on {
					drawCheckMark(p, ui.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}, t.AccentText)
				}
				if optRow.FocusVisible() {
					p.FocusRing(r, [4]float32{radiusXS, radiusXS, radiusXS, radiusXS})
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, "建好就切过去").FontSize(uiFontSize(textBodySM)).
					FontWeight(weightMedium).TextColor(pal.ink)
				ui.Text(c, "工作区当前未提交的改动会一并带到新分支").
					FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			})
		})

		failureBox(c, pal, t, bs.err)

		// —— 底栏：发丝线分隔 + 快捷键提示 + 取消 / 创建分支 ——
		ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.hairline)
		ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			ui.Text(c, "Enter 创建 · Esc 取消").FontSize(uiFontSize(textCaption)).
				TextColor(pal.inkSubtle)
			ui.Spacer(c)
			if secondaryAction(c, pal, "取消", "", false).Clicked() {
				a.cancelAIBranch()
				a.newBranch = branchState{}
				return
			}
			btn := primaryAction(c, pal, "创建分支", "从 "+bs.revLabel+" 新建一条分支", blocker != "" || a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.createBranch()
			}
		})
	})
}

// aiBranchButton 是「AI 起名」按钮。
//
// 走 §4.1 的洗色次级按钮（chip + hairlineStrong，悬停升 rowSelected），与左侧
// 输入框同高（34px），深浅色下不再出现不透明底色跳变。生成中用强调色文字与转圈
// 说明「正在跑」。
//
// 图标和文字分工：sparkle 矢量图标管视觉识别，文字只说「AI 起名」；完整的
// 「AI 建议分支名」放进无障碍 label 和 tooltip，读屏和测试按此定位。
func (a *app) aiBranchButton(c *ui.Context, pal palette) {
	t := c.Theme()
	bs := &a.newBranch
	tip := "把框里写的当需求描述，让 AI 起个分支名；自己填也行"
	if bs.aiLoading {
		tip = "正在生成建议… 点击取消"
	}
	btn := ui.ButtonBase(c).Height(34).Shrink(0).Radius(radiusMD).
		Padding(0, 12).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label("AI 建议分支名").Tooltip(tip)

	var bg, border, fg ui.Color
	switch {
	case bs.aiLoading:
		bg, border, fg = pal.rowSelected, pal.hairlineStrong, t.Accent
	case btn.Pressed(), btn.Hovered():
		bg, border, fg = pal.rowSelected, pal.hairlineTertiary, pal.ink
	default:
		bg, border, fg = pal.chip, pal.hairlineStrong, pal.inkMuted
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		if bs.aiLoading {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, "推荐中…").FontSize(uiFontSize(textButton)).FontWeight(weightMedium).TextColor(fg)
		} else {
			sparkleIcon(c, sparkleSize, fg)
			ui.Text(c, "AI 起名").FontSize(uiFontSize(textButton)).
				FontWeight(weightMedium).TextColor(fg)
		}
	})

	if btn.Clicked() {
		if bs.aiLoading {
			a.cancelAIBranch()
		} else {
			// 推迟到下一帧再发：点按钮会让名字框失焦，中文输入法的组合态
			// 正是在这一下才提交的。这一帧里 bs.name 还是提交前的值，
			// 直接发会把用户刚打的字漏掉。
			a.dispatch(a.startAIBranch)
		}
	}
}

// startAIBranch 发起 AI 分支名推荐请求。
func (a *app) startAIBranch() {
	bs := &a.newBranch
	if !bs.on {
		return
	}
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
	a.aiBranchSeq++
	seq := a.aiBranchSeq
	sentName := bs.name

	hint := aiBranchHint(bs)
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
			// 这个响应可能已经被取消、被新请求顶掉，或者对话框早就关了。
			// 对不上号就什么都不碰 —— 尤其别清 aiCancel，那可能已经是
			// 新一次请求的了。
			if a.aiBranchSeq != seq {
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
			if cleaned == "" {
				return
			}
			// 等结果的时候用户又改了名字框：这份结果已经对不上他写的东西，
			// 不覆盖。
			if a.newBranch.name != sentName {
				return
			}
			a.newBranch.name = cleaned
			a.newBranch.aiSuggested = cleaned
		})
	})
}

// aiBranchHint 返回这次「AI 起名」该当成需求描述的文本。
//
// 名字框同时是意图输入位：AI 给出名字后，框里留下的是它的答案，不是用户
// 写的需求。原样再喂回 BuildBranchPrompt 的话，模型看见一个现成的合法分支名
// 会直接抄回来（后面再补新意图也压不住），再点多少次都是同一个名字。所以
// 框里还是上一次的答案时，就当这次是「重新起一个」，只按 diff 起名。
func aiBranchHint(bs *branchState) string {
	hint := strings.TrimSpace(bs.name)
	if hint == bs.aiSuggested {
		return ""
	}
	return hint
}

// cancelAIBranch 取消进行中的 AI 分支名生成。
func (a *app) cancelAIBranch() {
	// 让还在路上的响应从此对不上号，不能回头清掉下一次请求的状态。
	a.aiBranchSeq++
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
