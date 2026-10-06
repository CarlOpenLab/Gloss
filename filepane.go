package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/filetree"
	"diff-explainer/internal/gitrepo"
)

// 主区下半：左边一栏文件列表，右边是 diff。
//
// 文件列表原来在侧栏，布局改成 Fork 那样之后侧栏让给了分支树，它搬到
// 这里 —— 也因此才放得下「点一下跳到那个文件」这种交互。
const (
	filePaneW      = 300
	filePaneWSplit = 220 // split 模式下 diff 要并排两栏，让点宽度给它
)

// filePaneWidth 是文件列表那一栏的宽度。
//
// 窗口窄的时候要收：侧栏 280 + 文件栏 300 = 580，720 的窗口（最小尺寸）
// 只剩 140 给 diff。夹到「至少给 diff 留 240」为止。
func (a *app) filePaneWidth(c *ui.Context) float32 {
	w := float32(filePaneW)
	if a.split {
		w = filePaneWSplit
	}
	avail, _ := c.Size()
	if a.sidebarOn {
		avail -= sidebarWidth
	}
	return min(w, max(120, avail-240))
}

// detailPane 是主区下半：文件列表 + diff。
//
// 两个 tab 共用它 —— 改动 tab 的工作区改动、历史 tab 里选中提交的改动，
// 走的都是 a.files 这一份数据，视图没有理由分家。
func (a *app) detailPane(c *ui.Context, pal palette) {
	t := c.Theme()

	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
		a.fileListPane(c, pal, t)
		ui.Box(c).Width(1).Shrink(0).FillHeight().Background(pal.border)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if a.hasDiff() {
				a.diffView(c, pal)
				return
			}
			ui.Box(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
					a.detailEmpty(c, t)
				})
			})
		})
	})
}

// hasDiff 判断下半区该画 diff 还是空状态。
func (a *app) hasDiff() bool {
	return a.state == loadReady && len(a.files) > 0
}

// fileListPane 是文件列表那一栏：栏头 + 筛选框 + 文件树 + footer。
func (a *app) fileListPane(c *ui.Context, pal palette, t *ui.Theme) {
	ui.Column(c).Width(a.filePaneWidth(c)).Shrink(0).FillHeight().
		Background(pal.sideBg.Alpha(0.35)).Children(func() {
		ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			n := "文件"
			if a.state == loadReady {
				n = strconv.Itoa(len(a.files)) + " 个文件"
			}
			ui.Text(c, n).FontSize(11).Font("monospace").FontFeatures("tnum").
				TextColor(t.TextMuted)
			ui.Spacer(c)
			if a.state == loadReady && len(a.files) > 0 {
				ui.Textf(c, "+%d −%d", a.totalAdd(), a.totalDel()).
					FontSize(11).Font("monospace").FontFeatures("tnum").
					TextColor(pal.ref)
			}
		})
		ui.Box(c).Shrink(0).Padding(0, 12, 8, 12).Children(func() {
			a.fileFilterInput(c)
		})
		a.treeView(c, pal, t)
		a.filePaneFooter(c, pal, t)
	})
}

// fileFilterInput 是文件栏的筛选框。
//
// Label 和侧栏那个（「筛选分支」）必须分得开：两个框筛的是完全不同的东西，
// 名字一样的话测试和无障碍工具都认不出找的是哪一个。
func (a *app) fileFilterInput(c *ui.Context) {
	in := ui.TextInput(c, &a.fileFilter).Placeholder("筛选文件").Label("筛选文件")
	// ⌘F 之后把焦点放进来。只做一次 —— 每帧都 Focus 的话这个框就再也
	// 交不出焦点了。
	if a.focusFilter {
		a.focusFilter = false
		in.Focus()
	}
}

// treeView 画文件树。筛选词变了就重算显示的文件 —— 直接比对值而不是依赖
// 控件的 Changed()，少一处需要小心维护的时序假设。
func (a *app) treeView(c *ui.Context, pal palette, t *ui.Theme) {
	if a.fileFilter != a.fileTreeFilter {
		a.applyFilter()
	}

	if len(a.tree) == 0 {
		emptyNote(c, t, a.emptyTreeMsg())
		return
	}

	// 筛选态一律展开：搜出来的文件藏在收起的目录里，等于没搜到。
	rows := filetree.Flatten(a.tree, a.filesClosed, a.fileFilter != "")
	ui.List(c, &a.filesList, len(rows), func(i int) {
		a.treeRow(c, pal, rows[i])
	}).Grow(1)
}

// emptyTreeMsg 是文件树空着时该说的话。空的原因有好几种，说错了会让人以为
// 是真没有改动。
func (a *app) emptyTreeMsg() string {
	switch {
	case a.state == loadIdle:
		return "还没打开仓库"
	case a.state != loadReady:
		return "" // 仓库在读，diff 那边有转圈了，这里先空着
	case a.tab == 1 && a.hist == loadLoading:
		return "正在读提交…"
	case a.tab == 1 && a.hist == loadFailed:
		return "读不到历史"
	case a.tab == 1 && len(a.commits) == 0:
		return "还没有提交"
	case a.tab == 1 && a.histDiff == loadLoading:
		return "正在读这个提交…"
	case a.tab == 1 && a.histDiff == loadFailed:
		return "读不到这个提交"
	case a.fileFilter != "":
		return "没有匹配的文件"
	}
	return "没有改动"
}

// treeRow 画文件树的一行。目录只画名字；文件画名字 + 增删行数 + 状态字母。
//
// 行本身可点：目录行点了收起/展开，文件行点了让 diff 滚到那个文件的头上。
// 选中态按**文件下标**记，不用 ui.List 的 Selected —— 那是扁平行下标，树一
// 重建（筛选、折叠目录、刷新）就指到别的行去了，记文件下标怎么动都不会错。
func (a *app) treeRow(c *ui.Context, pal palette, r filetree.Row) {
	t := c.Theme()
	n := r.Node

	if n.Dir {
		open := !a.filesClosed[n.Key]
		row, arrow := treeRow(c, t, treeRowOpts{Depth: r.Depth, HasKids: true, Open: open}, func() {
			ui.Text(c, n.Label).FontSize(12).SingleLine().Grow(1).MinWidth(0).
				TextColor(t.TextMuted)
		})
		if arrow.Clicked() || row.Clicked() {
			a.toggleFileDir(n.Key, open)
		}
		return
	}

	if n.FileIdx < 0 || n.FileIdx >= len(a.files) {
		treeRow(c, t, treeRowOpts{Depth: r.Depth}, func() {
			ui.Text(c, n.Label).FontSize(12).SingleLine()
		})
		return
	}

	f := a.files[n.FileIdx]

	row, _ := treeRow(c, t, treeRowOpts{Depth: r.Depth, Selected: n.FileIdx == a.fileSel}, func() {
		// 已 review 的文件名变灰，一眼能看出哪些还没看
		nameColor := t.Text
		if a.isViewed(f) {
			nameColor = t.TextMuted
		}
		ui.Text(c, n.Label).FontSize(12).SingleLine().Grow(1).MinWidth(0).TextColor(nameColor)

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
	})

	if row.Clicked() {
		a.fileSel = n.FileIdx
		// 只记下「该滚到哪」，不在这帧直接 ScrollTo：行模型现在可能还是脏的
		// （筛选/展开/切视图都会弄脏它），按旧的行下标滚过去会落到别的文件上。
		// diffView 重建完行模型再消费这个请求。
		a.pendingScrollFile = n.FileIdx
	}
}

// toggleFileDir 收起/展开一个目录。记的是「用户收起过哪些」——
// 没记过的默认展开：review 改动时几乎总想直接看到文件列表。
func (a *app) toggleFileDir(key string, open bool) {
	if a.filesClosed == nil {
		a.filesClosed = map[string]bool{}
	}
	if open {
		a.filesClosed[key] = true
		return
	}
	delete(a.filesClosed, key)
}

func (a *app) filePaneFooter(c *ui.Context, pal palette, t *ui.Theme) {
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

// detailEmpty 是下半区没有 diff 可画时的几种情况。
//
// 先问列表的状态、再问这个提交的 diff 的状态 —— 两者是两路独立的加载，
// 列表没读出来时根本谈不上「选中了哪个提交」。
func (a *app) detailEmpty(c *ui.Context, t *ui.Theme) {
	switch {
	case a.state == loadIdle:
		ui.Text(c, "还没有打开仓库").FontSize(18).Bold()
		ui.Text(c, "选一个本地 git 仓库，看看有哪些改动。").
			TextColor(t.TextMuted)
		if ui.PrimaryButton(c, "打开仓库…").Clicked() {
			a.chooseFolder()
		}
	case a.state == loadLoading:
		ui.Spinner(c)
		ui.Text(c, "正在读取仓库…").TextColor(t.TextMuted)
	case a.state == loadFailed:
		ui.Text(c, "打不开这个仓库").FontSize(18).Bold()
		ui.Text(c, a.errMsg).TextColor(t.Danger)
		if ui.Button(c, "重试").Clicked() {
			a.reload()
		}
	case a.tab == 1:
		a.historyEmpty(c, t)
	case a.fileFilter != "":
		ui.Text(c, "没有匹配的文件").FontSize(18).Bold()
		ui.Textf(c, "筛选词「%s」没有匹配到任何改动。", a.fileFilter).
			TextColor(t.TextMuted)
	default:
		ui.Text(c, "没有未提交的改动").FontSize(18).Bold()
		ui.Text(c, "工作区是干净的。").TextColor(t.TextMuted)
	}
}
