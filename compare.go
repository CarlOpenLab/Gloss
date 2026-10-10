package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// compareState 记录分支间代码差异对比的状态（Fork 风格）。
type compareState struct {
	on        bool             // 是否处于分支对比模式
	base      string           // 基准分支名（如 main）
	target    string           // 对比分支名（如 feature）
	tripleDot bool             // true = base...target（PR 模式），false = base..target（直接对比）
	commits   []gitrepo.Commit // 两个分支之间的提交列表
	state     loadState        // 加载状态
	err       string           // 错误信息

	// 弹窗状态
	dialog     bool   // 是否打开对比分支选择弹窗
	pickBase   string // 弹窗中选中的基准分支
	pickTarget string // 弹窗中选中的对比分支
}

// comparableBranches 返回当前所有可用于对比的引用短名列表（本地分支、远程分支、标签）。
func (a *app) comparableBranches() []string {
	var names []string
	seen := map[string]bool{}

	// 当前检出分支优先放最前
	if a.branch != "" {
		names = append(names, a.branch)
		seen[a.branch] = true
	}

	for _, r := range a.refs {
		if r.Kind == gitrepo.RefLocal || r.Kind == gitrepo.RefRemote || r.Kind == gitrepo.RefTag {
			if !seen[r.Name] {
				seen[r.Name] = true
				names = append(names, r.Name)
			}
		}
	}
	return names
}

// startCompare 开启两个分支的代码差异对比。
func (a *app) startCompare(base, target string, tripleDot bool) {
	if a.repoPath == "" || base == "" || target == "" {
		return
	}
	// 如果正在改动 tab，留好工作区文件以便退出对比时恢复
	if a.tab == 0 && a.workFiles == nil {
		a.workFiles = a.allFiles
	}

	a.compare.on = true
	a.compare.base = base
	a.compare.target = target
	a.compare.tripleDot = tripleDot
	a.compare.state = loadLoading
	a.compare.err = ""
	a.compare.commits = nil

	a.loadCompareDiff()
}

// loadCompareDiff 异步加载当前基准与对比分支的差异及提交列表。
func (a *app) loadCompareDiff() {
	if !a.compare.on || a.repoPath == "" {
		return
	}
	a.compare.state = loadLoading
	a.compare.err = ""

	a.compareSeq++
	seq := a.compareSeq
	base, target := a.compare.base, a.compare.target
	tripleDot := a.compare.tripleDot
	dir := a.repoPath

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, dir)
		var files []gitrepo.File
		var commits []gitrepo.Commit
		if err == nil {
			files, err = repo.BranchDiff(ctx, base, target, tripleDot)
			if err == nil {
				commits, _ = repo.BranchCommits(ctx, base, target, 100)
			}
		}
		a.dispatch(func() {
			if a.compareSeq != seq || !a.compare.on {
				return
			}
			if err != nil {
				a.compare.state = loadFailed
				a.compare.err = err.Error()
				return
			}
			a.compare.state = loadReady
			a.compare.commits = commits
			id := fmt.Sprintf("cmp:%s:%s:%t", base, target, tripleDot)
			a.showFiles(files, id)
		})
	})
}

// swapCompare 交换基准分支与对比分支方向。
func (a *app) swapCompare() {
	if !a.compare.on {
		return
	}
	a.compare.base, a.compare.target = a.compare.target, a.compare.base
	a.loadCompareDiff()
}

// setCompareTripleDot 切换三点对比（PR 模式）与两点对比（直接对比）。
func (a *app) setCompareTripleDot(tripleDot bool) {
	if a.compare.tripleDot == tripleDot {
		return
	}
	a.compare.tripleDot = tripleDot
	a.loadCompareDiff()
}

// exitCompare 退出分支对比模式，恢复进入对比前的文件。
func (a *app) exitCompare() {
	if !a.compare.on {
		return
	}
	a.compare.on = false
	a.compare.state = loadIdle
	a.compare.commits = nil

	if a.tab == 0 {
		if a.workFiles != nil {
			a.showFiles(a.workFiles, workListID(a.repoPath, a.workBranch()))
		} else {
			a.allFiles = nil
			a.applyFilter()
		}
	} else if a.tab == 1 {
		if a.histFiles != nil {
			a.showFiles(a.histFiles, a.histListID)
		} else if a.commitSel >= 0 && a.commitSel < len(a.commits) {
			a.loadCommit(a.commitSel)
		}
	}
}

// openCompareDialog 打开对比分支选择对话框。
func (a *app) openCompareDialog(defaultTarget string) {
	branches := a.comparableBranches()
	a.compare.dialog = true

	// 预设基准分支
	if a.branch != "" {
		a.compare.pickBase = a.branch
	} else if len(branches) > 0 {
		a.compare.pickBase = branches[0]
	}

	// 预设目标分支
	if defaultTarget != "" {
		a.compare.pickTarget = defaultTarget
	} else {
		for _, b := range branches {
			if b != a.compare.pickBase {
				a.compare.pickTarget = b
				break
			}
		}
		if a.compare.pickTarget == "" && len(branches) > 0 {
			a.compare.pickTarget = branches[0]
		}
	}
	a.compare.tripleDot = true
}

// compareBar 画顶部的分支对比控制栏（Fork 风格）。
// compareBarWideW 是分支对比栏能横向摆下的最小主区宽度。
//
// 这条带子的固有用量在 860 上下：两枚分支按钮、交换按钮、模式分段、提交
// 统计、退出按钮，中间还有几道 8px 间隔。低于它就跟提交栏一样折成两行 ——
// 720 的最小窗口减去侧栏只剩 ~430，横着摆会把「退出对比」整个顶到窗口外，
// 标题也会被 Shrink(0) 的邻居挤成竖排。
const compareBarWideW = 820

func (a *app) compareBar(c *ui.Context, pal palette) {
	t := c.Theme()
	branches := a.comparableBranches()

	winW, _ := c.Size()
	mainW := winW
	if a.sidebarOn {
		mainW -= a.sidebarWidth(c)
	}
	narrow := mainW < compareBarWideW

	// 图标与标题。窄窗口里省掉「分支对比」四个字，把宽度让给分支按钮。
	title := func() {
		ui.Box(c).Size(16, 16).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Icon(iconGitCompare, r, pal.inkMuted)
		})
		if !narrow {
			ui.Text(c, "分支对比").FontSize(textCaption).
				FontWeight(weightSemibold).TextColor(pal.inkSubtle)
		}
	}

	// 基准 / 交换 / 对比 三件选择器。
	selectors := func() {
		baseBtn := ui.ButtonBase(c).Height(26).Shrink(1).Padding(0, 8).Radius(radiusMD).
			Background(pal.surface2).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Gap(4).
			Cursor(ui.CursorPointer).Tooltip("更换基准分支 (Base)").Label("基准分支")
		if baseBtn.Hovered() {
			baseBtn.Background(pal.surface3)
		}
		fadeColors(baseBtn)
		baseBtn.Children(func() {
			ui.Text(c, "基准:").FontSize(textCaption).TextColor(pal.inkSubtle)
			ui.Text(c, a.compare.base).FontSize(textButton).SingleLine().MinWidth(0).MaxWidth(180).
				FontWeight(weightMedium).TextColor(pal.ink)
			ui.Text(c, "▾").FontSize(textCaption).TextColor(pal.inkSubtle)
		})
		baseBtn.Menu(func(m *ui.Menu) {
			for _, b := range branches {
				name := b
				if m.Item(name).Checked(name == a.compare.base).Chosen() {
					a.startCompare(name, a.compare.target, a.compare.tripleDot)
				}
			}
		})

		swapBtn := ui.ButtonBase(c).Size(26, 26).Shrink(0).Radius(radiusMD).Center().
			Cursor(ui.CursorPointer).Tooltip("交换基准与对比分支方向").Label("交换分支")
		if swapBtn.Hovered() {
			swapBtn.Background(pal.surface2)
		}
		fadeColors(swapBtn)
		swapBtn.Children(func() {
			ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Icon(iconSwap, r, pal.inkMuted)
			})
		})
		if swapBtn.Clicked() {
			a.swapCompare()
		}

		targetBtn := ui.ButtonBase(c).Height(26).Shrink(1).Padding(0, 8).Radius(radiusMD).
			Background(pal.surface2).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Gap(4).
			Cursor(ui.CursorPointer).Tooltip("更换对比分支 (Compare)").Label("对比分支")
		if targetBtn.Hovered() {
			targetBtn.Background(pal.surface3)
		}
		fadeColors(targetBtn)
		targetBtn.Children(func() {
			ui.Text(c, "对比:").FontSize(textCaption).TextColor(pal.inkSubtle)
			ui.Text(c, a.compare.target).FontSize(textButton).SingleLine().MinWidth(0).MaxWidth(180).
				FontWeight(weightMedium).TextColor(pal.ink)
			ui.Text(c, "▾").FontSize(textCaption).TextColor(pal.inkSubtle)
		})
		targetBtn.Menu(func(m *ui.Menu) {
			for _, b := range branches {
				name := b
				if m.Item(name).Checked(name == a.compare.target).Chosen() {
					a.startCompare(a.compare.base, name, a.compare.tripleDot)
				}
			}
		})
	}

	// 模式切换 + 分隔线 + 状态统计。
	meta := func() {
		modeIdx := 0
		if !a.compare.tripleDot {
			modeIdx = 1
		}
		toolbarSegmentedText(c, pal, &modeIdx, false, "PR 模式 (...)", "两端对比 (..)")
		if (modeIdx == 0) != a.compare.tripleDot {
			a.setCompareTripleDot(modeIdx == 0)
		}

		toolbarDivider(c, pal)

		switch a.compare.state {
		case loadLoading:
			ui.Spinner(c)
			ui.Text(c, "正在计算改动…").FontSize(textCaption).TextColor(pal.inkSubtle)
		case loadFailed:
			ui.Text(c, "对比失败: "+a.compare.err).FontSize(textCaption).TextColor(t.Danger)
		default:
			commitText := fmt.Sprintf("%d 个提交", len(a.compare.commits))
			cBtn := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(radiusSM).AlignItems(ui.Center).Gap(4).
				Cursor(ui.CursorPointer).Tooltip("查看差异提交列表").Label("差异提交")
			if cBtn.Hovered() {
				cBtn.Background(pal.surface2)
			}
			cBtn.Children(func() {
				ui.Text(c, commitText).FontSize(textCaption).TextColor(pal.inkSubtle)
				ui.Text(c, "▾").FontSize(textCaption).TextColor(pal.inkSubtle)
			})
			cBtn.Menu(func(m *ui.Menu) {
				if len(a.compare.commits) == 0 {
					m.Item("两分支间没有独立提交").Disabled(true)
					return
				}
				for _, cm := range a.compare.commits {
					m.Item(cm.Short + "  " + cm.Subject).Disabled(true)
				}
			})

			ui.Textf(c, "·  %d 个改动文件", len(a.allFiles)).FontSize(textCaption).TextColor(pal.inkSubtle)
		}
	}

	// 退出对比。
	exit := func() {
		exitBtn := ui.ButtonBase(c).Height(26).Shrink(0).Padding(0, 10).Radius(radiusMD).Gap(4).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Background(ui.Transparent).Border(1, pal.hairline).
			Label("退出对比").Tooltip("退出分支对比 (Esc)")
		if exitBtn.Hovered() {
			exitBtn.Background(pal.surface2)
		}
		fadeColors(exitBtn)
		exitBtn.Children(func() {
			ui.Text(c, "✕  退出对比").FontSize(textButton).
				FontWeight(weightMedium).TextColor(pal.inkMuted)
		})
		if exitBtn.Clicked() {
			a.exitCompare()
		}
	}

	if narrow {
		// 折成两行：上一行「在比哪两条分支 + 退出」，下一行模式与统计。
		ui.Column(c).Shrink(0).FillWidth().
			Padding(4, 16).Gap(4).
			Background(pal.surface1).Border(1, pal.hairline).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				title()
				selectors()
				ui.Spacer(c)
				exit()
			})
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				meta()
			})
		})
		return
	}

	ui.Row(c).Height(38).Shrink(0).FillWidth().
		Padding(0, 16).Gap(8).AlignItems(ui.Center).
		Background(pal.surface1).Border(1, pal.hairline).Children(func() {
		title()
		selectors()
		meta()
		ui.Spacer(c)
		exit()
	})
}

// compareDialog 是选择两分支进行对比的模态对话框。
func (a *app) compareDialog(c *ui.Context, pal palette) {
	if !a.compare.dialog {
		return
	}
	t := c.Theme()
	branches := a.comparableBranches()

	open := true
	ui.DialogBase(c, &open, func(back, panel *ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(460).Padding(20).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24))
		panel.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Box(c).Size(18, 18).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Icon(iconGitCompare, r, pal.inkMuted)
				})
				ui.Text(c, "对比分支代码差异").FontSize(textHeadline).
					FontWeight(weightSemibold).TextColor(pal.ink)
			})

			ui.Box(c).Height(16)

			// 基准分支 (Base)
			ui.Text(c, "基准分支 (Base):").FontSize(textBodySM).TextColor(pal.inkMuted)
			ui.Box(c).Height(4)
			baseBtn := ui.ButtonBase(c).Height(30).FillWidth().Padding(0, 10).Radius(radiusMD).
				Background(pal.surface2).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Justify(ui.SpaceBetween).
				Cursor(ui.CursorPointer)
			baseBtn.Children(func() {
				ui.Text(c, a.compare.pickBase).FontSize(textBody).TextColor(pal.ink)
				ui.Text(c, "▾").FontSize(textCaption).TextColor(pal.inkSubtle)
			})
			baseBtn.Menu(func(m *ui.Menu) {
				for _, b := range branches {
					name := b
					if m.Item(name).Checked(name == a.compare.pickBase).Chosen() {
						a.compare.pickBase = name
					}
				}
			})

			ui.Box(c).Height(12)

			// 快速交换按钮
			ui.Row(c).Justify(ui.Center).Children(func() {
				sw := ui.ButtonBase(c).Size(28, 28).Radius(radiusMD).Center().
					Cursor(ui.CursorPointer).Tooltip("交换两分支").Label("交换基准与对比分支")
				if sw.Hovered() {
					sw.Background(pal.surface2)
				}
				fadeColors(sw)
				sw.Children(func() {
					ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) {
						p.Icon(iconSwap, r, pal.inkMuted)
					})
				})
				if sw.Clicked() {
					a.compare.pickBase, a.compare.pickTarget = a.compare.pickTarget, a.compare.pickBase
				}
			})

			ui.Box(c).Height(12)

			// 对比分支 (Compare)
			ui.Text(c, "对比分支 (Compare):").FontSize(textBodySM).TextColor(pal.inkMuted)
			ui.Box(c).Height(4)
			targetBtn := ui.ButtonBase(c).Height(30).FillWidth().Padding(0, 10).Radius(radiusMD).
				Background(pal.surface2).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Justify(ui.SpaceBetween).
				Cursor(ui.CursorPointer)
			targetBtn.Children(func() {
				ui.Text(c, a.compare.pickTarget).FontSize(textBody).TextColor(pal.ink)
				ui.Text(c, "▾").FontSize(textCaption).TextColor(pal.inkSubtle)
			})
			targetBtn.Menu(func(m *ui.Menu) {
				for _, b := range branches {
					name := b
					if m.Item(name).Checked(name == a.compare.pickTarget).Chosen() {
						a.compare.pickTarget = name
					}
				}
			})

			ui.Box(c).Height(16)

			// 对比模式选项
			mode := 0
			if !a.compare.tripleDot {
				mode = 1
			}
			ui.Text(c, "对比模式:").FontSize(12).TextColor(t.TextMuted)
			ui.Box(c).Height(4)
			toolbarSegmentedText(c, pal, &mode, false, "PR 模式 (三点 ...)", "直接对比 (两点 ..)")
			if (mode == 0) != a.compare.tripleDot {
				a.compare.tripleDot = (mode == 0)
			}

			ui.Box(c).Height(20)

			// 底部按钮
			ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
				if secondaryAction(c, pal, "取消", "", false).Clicked() {
					a.compare.dialog = false
				}
				startBtn := primaryAction(c, pal, "开始对比", "", false)
				if a.compare.pickBase == "" || a.compare.pickTarget == "" || a.compare.pickBase == a.compare.pickTarget {
					startBtn.Disabled(true)
				}
				if startBtn.Clicked() {
					a.compare.dialog = false
					a.startCompare(a.compare.pickBase, a.compare.pickTarget, a.compare.tripleDot)
				}
			})
		})
	})
	if !open {
		a.compare.dialog = false
	}
}
