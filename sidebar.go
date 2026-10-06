package main

import (
	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/filetree"
	"diff-explainer/internal/gitrepo"
)

const sidebarWidth = 280

// sidebar 是左侧栏：筛选框 + 文件树（或提交列表）+ 底部统计。
//
// 底色是半透明的：开着 vibrancy 时它只是给系统材质叠一层淡淡的色调，
// 和 Finder 侧栏一个观感；系统关掉透明效果时它又能兜住底，不至于让
// 深色主题的文字落在白底上。
func (a *app) sidebar(c *ui.Context, pal palette) {
	// 侧栏是顶到窗口最上边的，所以红绿灯（macOS 的三个窗口按钮）浮在它上面。
	// 顶上空出 TitleBar.Height 那条带子，筛选框和提交列表才不会被关闭按钮
	// 压住；这条空带子顺手当成标题栏用，拖着能移窗口。工具栏是自己画的，
	// 那边用 bar.Left 在水平方向让位，这里是垂直方向。
	bar := c.TitleBar()

	ui.Column(c).Width(sidebarWidth).Shrink(0).
		Background(pal.sideBg.Alpha(0.55)).
		Children(func() {
			if bar.Height > 0 {
				ui.Box(c).Height(bar.Height).Shrink(0).DragWindow()
			}

			if a.tab == 0 {
				ui.Box(c).Padding(8).Shrink(0).Children(func() {
					a.filterInput(c)
				})
				a.treeView(c, pal)
				a.sidebarFooter(c, pal)
				return
			}
			a.commitSidebar(c, pal)
			a.historyFooter(c, pal)
		})
}

// filterInput 是侧栏的筛选框。
//
// 用 TextInput 而不是 ui.SearchField：SearchField 的 placeholder 是内部
// 写死的 "Search"，从外面盖不掉，而整个界面是中文的。
//
// Label 和 placeholder 一样是「筛选文件」：输入框没有可见的标签文字，
// 得有个名字给辅助技术念，测试也靠它定位。
func (a *app) filterInput(c *ui.Context) {
	in := ui.TextInput(c, &a.filter).Placeholder("筛选文件").Label("筛选文件")
	// ⌘F 之后把焦点放进来。只做一次 —— 每帧都 Focus 的话这个框就再也
	// 交不出焦点了。
	if a.focusFilter {
		a.focusFilter = false
		in.Focus()
	}
}

func (a *app) treeView(c *ui.Context, pal palette) {
	t := c.Theme()

	// 筛选词变了就重算显示的文件。直接比对值而不是依赖控件的 Changed()：
	// 少一处需要小心维护的时序假设。
	if a.filter != a.treeFilter {
		a.applyFilter()
	}

	switch {
	case a.state == loadIdle:
		a.treeEmpty(c, t, "还没打开仓库")
		return
	case a.state != loadReady:
		return
	case len(a.tree) == 0 && a.filter != "":
		a.treeEmpty(c, t, "没有匹配的文件")
		return
	case len(a.tree) == 0:
		a.treeEmpty(c, t, "没有改动")
		return
	}

	roots := make([]string, 0, len(a.tree))
	for _, n := range a.tree {
		roots = append(roots, n.Key)
	}

	ui.Outline(c, &a.outline, roots, a.childKeys, func(key string) {
		n, ok := a.treeByKey[key]
		if !ok {
			return
		}
		a.treeRow(c, pal, n)
	}).Grow(1)
}

func (a *app) treeEmpty(c *ui.Context, t *ui.Theme, msg string) {
	ui.Box(c).Grow(1).Padding(16).Children(func() {
		ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted)
	})
}

// treeRow 画文件树的一行。目录只画名字；文件画名字 + 增删行数 + 状态字母。
func (a *app) treeRow(c *ui.Context, pal palette, n *filetree.Node) {
	t := c.Theme()

	if n.Dir {
		ui.Text(c, n.Label).FontSize(12).SingleLine().TextColor(t.TextMuted)
		return
	}
	if n.FileIdx < 0 || n.FileIdx >= len(a.files) {
		ui.Text(c, n.Label).FontSize(12).SingleLine()
		return
	}

	f := a.files[n.FileIdx]

	// 已 review 的文件名变灰，一眼能看出哪些还没看
	nameColor := t.Text
	if a.isViewed(f) {
		nameColor = t.TextMuted
	}
	ui.Text(c, n.Label).FontSize(12).SingleLine().Grow(1).TextColor(nameColor)

	// 增删行数用来快速判断改动规模。用等宽字体 + tnum 让数字对齐，
	// 一列扫下来不会参差不齐。
	if f.AddCount > 0 {
		ui.Textf(c, "+%d", f.AddCount).FontSize(10).Bold().
			Font("monospace").FontFeatures("tnum").TextColor(pal.addBar)
	}
	if f.DelCount > 0 {
		ui.Textf(c, "−%d", f.DelCount).FontSize(10).Bold().
			Font("monospace").FontFeatures("tnum").TextColor(pal.delBar)
	}

	ui.Text(c, string(f.Status.Letter())).FontSize(10).Bold().
		Width(12).TextAlign(ui.Center).
		TextColor(statusColor(f.Status, pal)).
		Tooltip(f.Status.Label())
}

func (a *app) sidebarFooter(c *ui.Context, pal palette) {
	t := c.Theme()
	ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "Total: +%d −%d", a.totalAdd(), a.totalDel()).
			FontSize(11).Font("monospace").FontFeatures("tnum").
			TextColor(t.TextMuted)
		ui.Spacer(c)
		if n := a.viewedCount(); n > 0 {
			ui.Textf(c, "✓ %d/%d", n, len(a.files)).
				FontSize(11).TextColor(pal.viewed)
		}
	})
}

// historyFooter 是历史 tab 的侧栏底部：提交条数，以及当前这个提交的统计。
func (a *app) historyFooter(c *ui.Context, pal palette) {
	t := c.Theme()
	if a.hist != loadReady || len(a.commits) == 0 {
		return
	}
	ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "%d 个提交", len(a.commits)).
			FontSize(11).Font("monospace").FontFeatures("tnum").
			TextColor(t.TextMuted)
		ui.Spacer(c)
		if a.commitSel >= 0 && a.commitSel < len(a.commits) {
			ui.Textf(c, "+%d −%d", a.totalAdd(), a.totalDel()).
				FontSize(11).Font("monospace").FontFeatures("tnum").
				TextColor(pal.ref)
		}
	})
}

func (a *app) viewedCount() int {
	n := 0
	for _, f := range a.files {
		if a.isViewed(f) {
			n++
		}
	}
	return n
}

func statusColor(s gitrepo.Status, pal palette) ui.Color {
	switch s {
	case gitrepo.StatusAdded, gitrepo.StatusUntracked:
		return pal.addBar
	case gitrepo.StatusDeleted:
		return pal.delBar
	case gitrepo.StatusConflict:
		return pal.delBar
	}
	return pal.ref
}
