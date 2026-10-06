package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

const sidebarWidth = 280

// sidebar 是左侧栏：筛选框 + 分支/远程/标签/储藏树。
//
// 这里原来挂的是文件树，布局改成 Fork 那样之后文件树搬去了主区下半
// （filepane.go）—— 侧栏腾出来放引用，点一条分支就看它的历史。
//
// 底色是半透明的：开着 vibrancy 时它只是给系统材质叠一层淡淡的色调，
// 和 Finder 侧栏一个观感；系统关掉透明效果时它又能兜住底，不至于让
// 深色主题的文字落在白底上。
func (a *app) sidebar(c *ui.Context, pal palette) {
	// 侧栏是顶到窗口最上边的，所以红绿灯（macOS 的三个窗口按钮）浮在它上面。
	// 顶上空出 TitleBar.Height 那条带子，筛选框和树才不会被关闭按钮压住；
	// 这条空带子顺手当成标题栏用，拖着能移窗口。工具栏是自己画的，那边用
	// bar.Left 在水平方向让位，这里是垂直方向。
	bar := c.TitleBar()

	ui.Column(c).Width(sidebarWidth).Shrink(0).
		Background(pal.sideBg.Alpha(0.55)).
		Children(func() {
			if bar.Height > 0 {
				ui.Box(c).Height(bar.Height).Shrink(0).DragWindow()
			}

			ui.Box(c).Padding(8).Shrink(0).Children(func() {
				a.refFilterInput(c)
			})
			a.refsView(c, pal)
		})
}

// refFilterInput 是侧栏的筛选框。
//
// 用 TextInput 而不是 ui.SearchField：SearchField 的 placeholder 是内部
// 写死的 "Search"，从外面盖不掉，而整个界面是中文的。
//
// Label 写成「筛选分支」而不是「筛选」：主区下半还有一个筛选框（筛文件），
// 两个框名字一样的话，测试和无障碍工具都分不出找的是哪一个。
func (a *app) refFilterInput(c *ui.Context) {
	ui.TextInput(c, &a.refFilter).Placeholder("筛选分支").Label("筛选分支")
}

func (a *app) refsView(c *ui.Context, pal palette) {
	t := c.Theme()

	switch a.refsState {
	case loadIdle:
		return
	case loadLoading:
		emptyNote(c, t, "正在读分支…")
		return
	case loadFailed:
		emptyNote(c, t, "读不到分支")
		return
	}

	if a.refFilter != a.refsFilter {
		a.rebuildRefTree()
	}
	if len(a.refTree) == 0 {
		emptyNote(c, t, "没有匹配的分支")
		return
	}

	// 筛选态一律展开：搜出来的分支藏在收起的分组里，等于没搜到。
	rows := filetree.Flatten(a.refTree, a.refsClosed, a.refFilter != "")
	ui.List(c, &a.refsList, len(rows), func(i int) {
		a.refRow(c, pal, rows[i])
	}).Grow(1)
}

// refRow 画 refs 树的一行。
func (a *app) refRow(c *ui.Context, pal palette, r filetree.Row) {
	t := c.Theme()
	n := r.Node

	if n.Dir {
		a.refGroupRow(c, t, pal, r)
		return
	}

	ref, ok := a.refByFull[strings.TrimPrefix(n.Key, refLeaf)]
	if !ok {
		ui.Text(c, n.Label).FontSize(12).SingleLine()
		return
	}

	// 「当前检出的分支」那个圆点画在折叠箭头槽里（见 treeRowOpts.Dot）
	opts := treeRowOpts{Depth: r.Depth, Selected: n.Key == a.refSel}
	if ref.Head {
		c := pal.laneColor(0)
		opts.Dot = &c
	}

	row, _ := treeRow(c, t, opts, func() {
		if ref.Kind == gitrepo.RefStash {
			// 储藏的名字（stash@{0}）人人都一样，认出它是哪一条全靠信息；
			// 两个都画，名字压暗当索引，信息才是给人读的。
			ui.Text(c, ref.Name).FontSize(11).Font("monospace").Shrink(0).
				TextColor(t.TextMuted)
			ui.Text(c, ref.Subject).FontSize(12).SingleLine().
				Grow(1).MinWidth(0).TextColor(t.Text)
			return
		}

		txt := ui.Text(c, n.Label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
		if ref.Head {
			txt.Bold().TextColor(t.Text)
		}
	})

	if row.Clicked() {
		a.selectRef(n.Key, *ref)
	}
}

// refGroupRow 画一个分组行（本地分支 / 远程 / 标签 / 储藏）。整行可点，
// 点哪儿都是收起或展开 —— 只有一个 12px 的小三角能点，用起来很别扭。
func (a *app) refGroupRow(c *ui.Context, t *ui.Theme, pal palette, r filetree.Row) {
	open := len(r.Node.Children) > 0 && !a.refsClosed[r.Node.Key]

	// 分组行不参与选中：它不是一个可以「去看历史」的东西
	row, arrow := treeRow(c, t, treeRowOpts{Depth: r.Depth, HasKids: true, Open: open}, func() {
		ui.Text(c, r.Node.Label).FontSize(11).Bold().SingleLine().
			TextColor(t.TextMuted)
	})

	if arrow.Clicked() || row.Clicked() {
		a.toggleRefGroup(r.Node.Key, open)
	}
}

// toggleRefGroup 收起/展开一个分组，并把状态记下来。
func (a *app) toggleRefGroup(key string, open bool) {
	if a.refsClosed == nil {
		a.refsClosed = map[string]bool{}
	}
	if open {
		a.refsClosed[key] = true
		return
	}
	delete(a.refsClosed, key)
}

// selectRef 是点了一条引用：把历史切到它上面。
//
// commitSel 必须一起清掉。不清的话，从一条长历史切到短历史时旧下标往往
// 仍然合法，于是「选中的提交」悄悄变成了另一个 —— 界面上没有任何异常，
// 只是显示的是别处的 diff。
func (a *app) selectRef(key string, ref gitrepo.Ref) {
	a.refSel = key

	if ref.Kind == gitrepo.RefStash {
		a.selectStash(ref)
		return
	}

	a.histRev = ref.Full
	a.commitSel = -1
	a.stashRev = "" // 换看分支了，下半区不再是储藏
	a.histDiff = loadIdle
	a.tab = 1
	a.loadCommits()
}

// rebuildRefTree 依据当前引用和筛选词重建侧栏的树。
//
// 展开/收起的状态不用在这里初始化：那一份存的是「用户收起过哪些」，
// 没记过的默认就是展开的（见 refsClosed）。
func (a *app) rebuildRefTree() {
	a.refsFilter = a.refFilter
	a.refTree = buildRefTree(a.refs, a.refFilter)
}
