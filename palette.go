package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
)

// ⌘K 命令栏。
//
// 有菜单栏能做的事这里都再放一份：菜单是用来「发现」功能的，命令栏是用来
// 「用」功能的 —— 熟悉之后手不用离开键盘。
type paletteState struct {
	on    bool
	query string
	sel   int
}

// command 是一条命令。hint 显示在右边，写快捷键或当前状态。
type command struct {
	label string
	hint  string
	run   func(a *app)
}

// commands 返回当前可用的命令。有的命令带状态（「隐藏侧栏」还是「显示侧栏」），
// 所以它每次都要重新算。
func (a *app) commands() []command {
	cmds := []command{
		{T("切换终端 / Git 面板", "Toggle Terminal / Git Panel"), KS("⌘J / Ctrl+`"), func(a *app) { a.toggleViewMode() }},
		{T("新建项目标签页…", "New Project Tab…"), K("⌘T"), func(a *app) { a.chooseFolder() }},
		{T("关闭标签页", "Close Tab"), K("⌘W"), func(a *app) { a.closeTab(a.activeTab) }},
		{T("上一个项目标签页", "Previous Project Tab"), K("⌘⇧["), func(a *app) { a.prevTab() }},
		{T("下一个项目标签页", "Next Project Tab"), K("⌘⇧]"), func(a *app) { a.nextTab() }},
		{T("打开仓库…", "Open Repository…"), K("⌘O"), func(a *app) { a.chooseFolder() }},
		{T("刷新", "Refresh"), K("⌘R"), func(a *app) { a.reload() }},
		{T("提交", "Commit"), K("⌘⏎"), func(a *app) { a.doCommit() }},
		{T("暂存全部改动", "Stage All Changes"), K("⌘⇧A"), func(a *app) { a.stageAll() }},
		{T("取消暂存全部改动", "Unstage All Changes"), "", func(a *app) { a.unstageAll() }},
		{T("预检暂存区代码风险…", "Scan Staged Changes for Risks…"), "", func(a *app) { a.startRiskScan() }},
		{T("储藏工作区改动…", "Stash Changes…"), "", func(a *app) { a.openStashDialog() }},
		{T("新建标签…", "New Tag…"), "", func(a *app) { a.newTagAtHead() }},
		{T("新建分支…", "New Branch…"), K("⌘⇧N"), func(a *app) { a.newBranchAtHead() }},
		{T("抓取", "Fetch"), K("⌘⇧F"), func(a *app) { a.doFetch() }},
		{T("拉取", "Pull"), K("⌘⇧P"), func(a *app) { a.doPull() }},
		{T("推送", "Push"), K("⌘⇧U"), func(a *app) { a.doPush() }},
		{T("查看改动", "Show Changes"), K("⌘1"), func(a *app) { a.setTab(0) }},
		{T("查看历史", "Show History"), K("⌘2"), func(a *app) { a.setTab(1) }},
		{T("筛选文件", "Filter Files"), K("⌘P"), func(a *app) { a.focusFilter = true }},
		{T("统一视图（Unified）", "Unified View"), K("⌘U"), func(a *app) { a.setSplit(false) }},
		{T("左右对照（Split）", "Split View"), "", func(a *app) { a.setSplit(true) }},
		{T("模型设置…", "Settings…"), K("⌘,"), func(a *app) { a.openSettings() }},
		{T("外观: 跟随系统", "Theme: Follow System"), "", func(a *app) { a.setThemeMode("system") }},
		{T("外观: 浅色模式", "Theme: Light Mode"), "", func(a *app) { a.setThemeMode("light") }},
		{T("外观: 深色模式", "Theme: Dark Mode"), "", func(a *app) { a.setThemeMode("dark") }},
		{T("语言: 简体中文", "Language: 简体中文"), "", func(a *app) { a.setLocale("zh") }},
		{T("语言: English", "Language: English"), "", func(a *app) { a.setLocale("en") }},
		{T("放大代码字号", "Increase Font Size"), K("⌘+"), func(a *app) { a.setCodeSize(a.codeSizeValue() + codeSizeStep) }},
		{T("缩小代码字号", "Decrease Font Size"), K("⌘−"), func(a *app) { a.setCodeSize(a.codeSizeValue() - codeSizeStep) }},
		{T("恢复默认字号", "Reset Font Size"), K("⌘0"), func(a *app) { a.setCodeSize(0) }},
	}

	if a.sidebarOn {
		cmds = append(cmds, command{T("隐藏侧栏", "Hide Sidebar"), K("⌘⇧B"), func(a *app) { a.setSidebar(false) }})
	} else {
		cmds = append(cmds, command{T("显示侧栏", "Show Sidebar"), K("⌘⇧B"), func(a *app) { a.setSidebar(true) }})
	}
	if a.canReachRemote() && !a.detached && a.hasUpstream(a.branch) {
		cmds = append(cmds, command{T("强制推送（--force-with-lease）…", "Force Push (--force-with-lease)…"), "", func(a *app) { a.promptForcePush() }})
	}
	if a.repoReady() {
		cmds = append(cmds, command{T("对比分支代码差异…", "Compare Branches…"), K("⌘⇧D"), func(a *app) { a.openCompareDialog("") }})
		cmds = append(cmds, command{T("查看 GitHub Pull Requests…", "GitHub Pull Requests…"), "", func(a *app) { a.openPRDialog() }})
	}
	if a.compare.on {
		cmds = append(cmds, command{T("退出分支对比", "Exit Compare"), "Esc", func(a *app) { a.exitCompare() }})
		cmds = append(cmds, command{T("交换基准与对比分支方向", "Swap Compare Branches"), "", func(a *app) { a.swapCompare() }})
	}
	if a.prReview.on {
		cmds = append(cmds, command{T("退出 PR 检视", "Exit PR Review"), "Esc", func(a *app) { a.exitPRReview() }})
		cmds = append(cmds, command{T("✨ 生成并发布 AI Review 评论…", "✨ Generate and Post AI Review Comment…"), "", func(a *app) { a.openPRReviewModal() }})
	}
	for _, p := range a.store.State().RecentRepos {
		if p == "" {
			continue
		}
		pathCopy := p
		base := filepath.Base(p)
		cmds = append(cmds, command{
			label: T("打开最近项目: ", "Open Recent: ") + base,
			hint:  p,
			run:   func(a *app) { a.openRepo(pathCopy) },
		})
	}
	return cmds
}

func (a *app) setSplit(on bool) {
	if a.split == on {
		return
	}
	a.split = on
	a.saveViewSettings()
}

func (a *app) setSidebar(on bool) {
	if a.sidebarOn == on {
		return
	}
	a.sidebarOn = on
	a.saveViewSettings()
}

func (a *app) openPalette() {
	a.palette = paletteState{on: true}
}

func (a *app) closePalette() {
	a.palette = paletteState{}
}

// matches 按模糊匹配挑命令，并保持原顺序 —— 命令栏里的顺序本身就是
// 「常用度」的排序，按匹配度重排反而让肌肉记忆失效。
func (a *app) matchedCommands() []command {
	all := a.commands()
	q := strings.TrimSpace(a.palette.query)
	if q == "" {
		return all
	}
	var out []command
	for _, cmd := range all {
		if filetree.FuzzyMatch(cmd.label, q) {
			out = append(out, cmd)
		}
	}
	return out
}

// 命令栏的尺寸。行高写死是为了让面板能按条数算出自己该多高 ——
// 高矮不一的话最后一条会被裁掉，而命令栏里被裁掉的正好是「有没有这条」。
const (
	paletteRowH = 28
	paletteGap  = 1
	palettePad  = 6
	paletteMaxH = 360
)

// paletteListH 是命令列表该占的高度：够放下所有条目，最多到 paletteMaxH。
func paletteListH(n int) float32 {
	if n == 0 {
		return 0
	}
	h := n*(paletteRowH+paletteGap) + 2*palettePad
	return float32(min(h, paletteMaxH))
}

// paletteDialog 是命令栏本体。
func (a *app) paletteDialog(c *ui.Context, pal palette) {
	if !a.palette.on {
		return
	}
	t := c.Theme()
	cmds := a.matchedCommands()
	a.palette.sel = clampSel(a.palette.sel, len(cmds))

	open := true
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		// 模态落 Surface 4（§2.1），圆角 xl(16)（§3）；投影收到贴着边的一圈 ——
		// 阶梯已经把「它浮在最上面」说清楚了，大模糊只会糊出一圈灰边。
		panel.Width(520).Padding(0).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24))
		panel.Children(func() {
			var in ui.Element
			ui.Box(c).Padding(10, 12).Shrink(0).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
				in = ui.TextInput(c, &a.palette.query).Placeholder(T("输入命令…", "Type a command…")).Focus()
			})

			// 方向键和回车得问输入框本身。
			//
			// 窗口级的 c.Shortcut 在这儿是不通的：对话框开着的时候它直接
			// 返回 false（这正是「底下让位」的意思）。而输入框在编辑时又会
			// 先把按键吃掉。Element.Shortcut 认的是「焦点在我或我的子孙身
			// 上」，是唯一还通的那条路；回车更直接 —— 输入框把它变成了一次
			// 提交，不再往外传。
			switch {
			case in.Shortcut(0, ui.KeyDown):
				a.palette.sel = min(a.palette.sel+1, max(0, len(cmds)-1))
			case in.Shortcut(0, ui.KeyUp):
				a.palette.sel = max(a.palette.sel-1, 0)
			}
			if in.Submitted() {
				a.runCommand(cmds)
				// 命令可能已经让命令栏收起来了，重新算一遍再往下画
				cmds = a.matchedCommands()
				a.palette.sel = clampSel(a.palette.sel, len(cmds))
			}

			ui.Column(c).Padding(palettePad).Gap(paletteGap).
				Height(paletteListH(len(cmds))).Children(func() {
				if len(cmds) == 0 {
					ui.Text(c, T("没有匹配的命令", "No matching commands")).
						FontSize(textBodySM).TextColor(pal.inkSubtle).Padding(8)
					return
				}
				for i := range cmds {
					a.paletteRow(c, t, pal, i, cmds)
				}
			})
		})
	})
	if !open {
		a.closePalette()
	}
}

func (a *app) paletteRow(c *ui.Context, t *ui.Theme, pal palette, i int, cmds []command) {
	cmd := cmds[i]
	sel := i == a.palette.sel

	// 选中那一行原来是**整块实心强调色**。命令栏一屏十几行，一片蓝底上
	// 白字反而更不好读；§5 也把全屏的实心强调压在 1~2 处。选中用中性底带
	// （和树、列表同一档），选中的那一行靠底带 + 文字提亮到 Ink 说话。
	bg := ui.Transparent
	if sel {
		bg = pal.selection
	}
	text := pal.inkMuted
	if sel {
		text = pal.ink
	}

	row := ui.Row(c).Height(paletteRowH).Shrink(0).Padding(0, 10).Radius(radiusSM).Gap(10).
		AlignItems(ui.Center).Background(bg).Cursor(ui.CursorPointer)
	if row.Hovered() && !sel {
		row.Background(pal.surface2)
	}
	fadeColors(row)
	var clicked bool
	row.Children(func() {
		txt := ui.Text(c, cmd.label).FontSize(12.5).Grow(1).SingleLine().TextColor(text)
		if sel {
			txt.FontWeight(weightMedium)
		}
		if cmd.hint != "" {
			ui.Text(c, cmd.hint).FontSize(textCaption).SingleLine().
				TextColor(pal.inkSubtle)
		}
	})
	clicked = row.Clicked()
	if clicked {
		a.palette.sel = i
		a.runCommand(cmds)
	}
}

// runCommand 执行选中的那条命令并收起命令栏。
func (a *app) runCommand(cmds []command) {
	if a.palette.sel < 0 || a.palette.sel >= len(cmds) {
		return
	}
	cmd := cmds[a.palette.sel]
	a.closePalette()
	cmd.run(a)
}

// clampSel 把命令栏的选中项夹在可选项范围内。没有可选项时是 0。
func clampSel(sel, n int) int { return max(0, min(sel, n-1)) }
