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
		{"打开仓库…", "⌘O", func(a *app) { a.chooseFolder() }},
		{"刷新", "⌘R", func(a *app) { a.reload() }},
		{"提交", "⌘⏎", func(a *app) { a.doCommit() }},
		{"暂存全部改动", "", func(a *app) { a.stageAll() }},
		{"预检暂存区代码风险…", "", func(a *app) { a.startRiskScan() }},
		{"储藏工作区改动…", "", func(a *app) { a.openStashDialog() }},
		{"新建标签…", "", func(a *app) { a.newTagAtHead() }},
		{"新建分支…", "⌘⇧N", func(a *app) { a.newBranchAtHead() }},
		{"抓取", "⌘⇧F", func(a *app) { a.doFetch() }},
		{"拉取", "⌘⇧P", func(a *app) { a.doPull() }},
		{"推送", "⌘⇧U", func(a *app) { a.doPush() }},
		{"查看改动", "⌘1", func(a *app) { a.setTab(0) }},
		{"查看历史", "⌘2", func(a *app) { a.setTab(1) }},
		{"筛选文件", "⌘F", func(a *app) { a.focusFilter = true }},
		{"统一视图（Unified）", "⌘U", func(a *app) { a.setSplit(false) }},
		{"左右对照（Split）", "", func(a *app) { a.setSplit(true) }},
		{"模型设置…", "⌘,", func(a *app) { a.openSettings() }},
		{"放大代码字号", "⌘+", func(a *app) { a.setCodeSize(a.codeSizeValue() + codeSizeStep) }},
		{"缩小代码字号", "⌘−", func(a *app) { a.setCodeSize(a.codeSizeValue() - codeSizeStep) }},
		{"恢复默认字号", "⌘0", func(a *app) { a.setCodeSize(0) }},
	}

	if a.sidebarOn {
		cmds = append(cmds, command{"隐藏侧栏", "⌘⇧B", func(a *app) { a.setSidebar(false) }})
	} else {
		cmds = append(cmds, command{"显示侧栏", "⌘⇧B", func(a *app) { a.setSidebar(true) }})
	}
	if a.canReachRemote() && !a.detached && a.hasUpstream(a.branch) {
		cmds = append(cmds, command{"强制推送（--force-with-lease）…", "", func(a *app) { a.promptForcePush() }})
	}
	if a.explainOpen() {
		cmds = append(cmds, command{"收起 AI 解释", "Esc", func(a *app) { a.closeExplain() }})
	}
	if a.repoReady() {
		cmds = append(cmds, command{"对比分支代码差异…", "⌘⇧D", func(a *app) { a.openCompareDialog("") }})
	}
	if a.compare.on {
		cmds = append(cmds, command{"退出分支对比", "Esc", func(a *app) { a.exitCompare() }})
		cmds = append(cmds, command{"交换基准与对比分支方向", "", func(a *app) { a.swapCompare() }})
	}
	for _, p := range a.store.State().RecentRepos {
		if p == "" {
			continue
		}
		pathCopy := p
		base := filepath.Base(p)
		cmds = append(cmds, command{
			label: "打开最近项目: " + base,
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
	a.rowsDirty = true
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
	ui.DialogBase(c, &open, func(back, panel *ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(520).Padding(0).Radius(t.Radius+4).
			Background(t.Background).Border(1, pal.border).
			Shadow(0, 12, 36, 0, ui.RGBA(0, 0, 0, 0.28))
		panel.Children(func() {
			var in *ui.Element
			ui.Box(c).Padding(10, 12).Shrink(0).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
				in = ui.TextInput(c, &a.palette.query).Placeholder("输入命令…").Focus()
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
					ui.Text(c, "没有匹配的命令").FontSize(12).TextColor(t.TextMuted).Padding(8)
					return
				}
				for i := range cmds {
					a.paletteRow(c, t, i, cmds)
				}
			})
		})
	})
	if !open {
		a.closePalette()
	}
}

func (a *app) paletteRow(c *ui.Context, t *ui.Theme, i int, cmds []command) {
	cmd := cmds[i]
	sel := i == a.palette.sel

	bg := ui.Transparent
	if sel {
		bg = t.Accent
	}
	text := t.Text
	if sel {
		text = t.AccentText
	}

	row := ui.Row(c).Height(paletteRowH).Shrink(0).Padding(0, 10).Radius(t.Radius).Gap(10).
		AlignItems(ui.Center).Background(bg).Cursor(ui.CursorPointer)
	if row.Hovered() && !sel {
		row.Background(t.Surface)
	}
	fadeColors(row)
	var clicked bool
	row.Children(func() {
		ui.Text(c, cmd.label).FontSize(12.5).Grow(1).SingleLine().TextColor(text)
		if cmd.hint != "" {
			ui.Text(c, cmd.hint).FontSize(11).SingleLine().
				TextColor(t.TextMuted).Opacity(0.9)
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
