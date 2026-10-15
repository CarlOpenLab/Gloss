package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 主区下半：左边一栏文件列表，右边是 diff。
//
// 文件列表原来在侧栏，布局改成 Fork 那样之后侧栏让给了分支树，它搬到
// 这里 —— 也因此才放得下「点一下跳到那个文件」这种交互。
const (
	filePaneW      = float32(300)
	filePaneWSplit = float32(220) // split 模式下 diff 要并排两栏，让点宽度给它
	filePaneMinW   = float32(140)
)

func (a *app) filePaneDefaultW() float32 {
	if a.split {
		return filePaneWSplit
	}
	return filePaneW
}

// filePaneWidth 是文件列表那一栏的宽度。
//
// 窗口窄的时候要收：至少给 diff 留 240。支持拖动调整与记住用户偏好。
func (a *app) filePaneWidth(c *ui.Context) float32 {
	maxW := float32(1000)
	if c != nil {
		avail, _ := c.Size()
		if a.sidebarOn {
			avail -= a.sidebarWidth(c)
		}
		maxW = max(filePaneMinW, avail-240)
	}

	w := a.filePaneWidthVal
	if w <= 0 {
		w = a.filePaneDefaultW()
	}
	return clamp(w, filePaneMinW, maxW)
}

func (a *app) setFilePaneWidth(c *ui.Context, w float32) {
	maxW := float32(1000)
	if c != nil {
		avail, _ := c.Size()
		if a.sidebarOn {
			avail -= a.sidebarWidth(c)
		}
		maxW = max(filePaneMinW, avail-240)
	}
	clamped := clamp(w, filePaneMinW, maxW)
	if clamped != a.filePaneWidthVal {
		a.filePaneWidthVal = clamped
	}
}

func (a *app) resetFilePaneWidth() {
	a.filePaneWidthVal = a.filePaneDefaultW()
	a.saveViewSettings()
}

// filePaneSplitter 是文件列表与 Diff 视图之间的可拖拽分隔条。
func (a *app) filePaneSplitter(c *ui.Context, pal palette) {
	t := c.Theme()
	grip := float32(6)

	ui.Box(c).Width(1).Shrink(0).FillHeight().Children(func() {
		handle := ui.Box(c).Absolute().
			Top(0).Bottom(0).Left(-(grip - 1) / 2).Width(grip).
			Cursor(ui.CursorResizeEW).Role(ui.RoleNone)

		if handle.DoubleClicked() {
			a.resetFilePaneWidth()
		}
		if dx, _, held := handle.Dragged(); held && dx != 0 {
			a.setFilePaneWidth(c, a.filePaneWidth(c)+dx)
			a.filePaneDragging = true
		} else if a.filePaneDragging && !held {
			a.filePaneDragging = false
			a.saveViewSettings()
		}
		handle.Draw(func(p *ui.Painter, r ui.Rect) {
			line := pal.hairline
			if handle.Hovered() || handle.Pressed() {
				line = t.Accent
			}
			p.Fill(ui.Rect{X: r.X + (grip-1)/2, Y: r.Y, W: 1, H: r.H}, line, 0)
		})
	})
}

// detailPane 是主区下半：文件列表 + diff。
//
// 两个 tab 共用它 —— 改动 tab 的工作区改动、历史 tab 里选中提交的改动，
// 走的都是 a.files 这一份数据，视图没有理由分家。
func (a *app) detailPane(c *ui.Context, pal palette) {
	t := c.Theme()

	// 没打开仓库（或者打不开）时不摆文件栏：那时候它只能是一栏空架子
	// —— 一个没有东西可筛的筛选框、一条没有文件的总计，而真正要说的话
	// （「还没有打开仓库」）被挤在旁边一栏里，也不居中。直接让整块地方
	// 都归那句提示。
	if !a.hasDiff() && (a.state == loadIdle || a.state == loadFailed) {
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
				a.detailEmpty(c, t, pal)
			})
		})
		return
	}

	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
		a.fileListPane(c, pal, t)
		a.filePaneSplitter(c, pal)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if a.hasDiff() {
				a.reviewPane(c, pal)
				return
			}
			ui.Box(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
					a.detailEmpty(c, t, pal)
				})
			})
		})
	})
}

// hasDiff 判断下半区该画 diff 还是空状态。
//
// 「正在重读」但手上还有同一个仓库的旧列表时继续画旧的那份：连续暂存十个
// hunk 就是十次重读，每次都把 diff 换成一句「正在读取仓库…」再换回来，画面
// 会闪成一串。换个仓库则一秒都不留 —— 旧仓库的 diff 留到新内容就绪，那是
// 实打实地显示错了东西。重读的进展由工具栏的转圈说，那是它该管的事。
func (a *app) hasDiff() bool {
	if len(a.files) == 0 {
		return false
	}
	switch a.state {
	case loadReady:
		return true
	case loadLoading:
		return sameRepo(a.loadingPath, a.repoPath)
	}
	return false
}

// fileListPane 是文件列表那一栏：栏头 + 筛选框 + 文件树 + footer。
func (a *app) fileListPane(c *ui.Context, pal palette, t *ui.Theme) {
	ui.Column(c).Width(a.filePaneWidth(c)).Shrink(0).FillHeight().
		Background(pal.surface1).Children(func() {
		// 栏头只留「几个文件」一件必读的信息，以及左边的三态勾选框。
		//
		// 这一栏的宽度是定死的 300（split 下 220）。原来还并排塞着
		// 「全部暂存 / 全部取消」两颗文字药丸和一组 +add −del：半暂存
		// （既有已暂存又有未暂存）时四样东西一起出现，一行摆不下，文字
		// 各自折成两行，勾选框也被挤得离开基线，整条栏头看着像坏了。
		//
		// 那三样都是冗余的：勾选框在三种状态下本来就是暂存 / 取消暂存的
		// 开关；增删总数在栏脚「总计」里已经报过一次；带文字的批量操作
		// 在提交栏底部（「+ 全部暂存 / 全部取消」）也都有。
		ui.Row(c).Shrink(0).Padding(7, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			if a.tab == 0 && a.repoReady() && len(a.files) > 0 {
				a.masterStageBox(c, t, pal)
			}
			n := T("文件", "Files")
			if a.hasDiff() || a.state == loadReady {
				n = Tf("%d 个文件", "%d files", len(a.files))
			}
			ui.Text(c, n).FontSize(11).Font("monospace").FontFeatures("tnum").
				SingleLine().TextColor(t.TextMuted)
		})
		ui.Box(c).Key("file-filter-wrap").Shrink(0).Padding(0, 12, 8, 12).Children(func() {
			a.fileFilterInput(c, pal)
		})
		ui.Box(c).Key("file-tree-wrap").Grow(1).FillWidth().Children(func() {
			a.treeView(c, pal, t)
		})
		a.filePaneFooter(c, pal, t)
	})
}

// masterStageBox 画全选/全不选的主勾选框（三态：全暂存、全未暂存、部分暂存）。
func (a *app) masterStageBox(c *ui.Context, t *ui.Theme, pal palette) {
	staged := a.stagedCount()
	unstaged := a.unstagedCount()
	if staged == 0 && unstaged == 0 {
		return
	}
	lock := a.op != gitrepo.OpNone
	btn := ui.ButtonBase(c).Size(stageBoxSize, stageBoxSize).Shrink(0).Radius(radiusXS).
		Cursor(ui.CursorPointer).Disabled(lock)

	allStaged := unstaged == 0 && staged > 0
	indeterminate := staged > 0 && unstaged > 0

	switch {
	case allStaged:
		btn.Label(T("全部取消暂存", "Unstage All")).Tooltip(T("全部取消暂存 (将所有已暂存文件移出暂存区)", "Unstage all files (remove from index)"))
	case indeterminate:
		btn.Label(T("全部暂存", "Stage All")).Tooltip(T("全部暂存剩余改动 (将剩余未暂存改动加入暂存区)", "Stage remaining changes (add to index)"))
	default:
		btn.Label(T("全部暂存", "Stage All")).Tooltip(T("全部暂存 (将所有改动加入暂存区)", "Stage all changes (add to index)"))
	}

	hovered := btn.Hovered() && !lock
	var border, fill ui.Color
	switch {
	case allStaged, indeterminate:
		// 勾上是一个「开着」的控件，和主按钮同属于 §5 留给强调色的那几处，
		// 不改成中性。
		fill, border = t.Accent, t.Accent
		if hovered {
			fill, border = t.AccentHover, t.AccentHover
		}
	default:
		border = pal.hairlineStrong
		if hovered {
			border = pal.hairlineTertiary
			fill = pal.surface2
		}
	}
	btn.Background(fill).Border(1, border)
	fadeColors(btn)

	if allStaged {
		btn.Children(func() {
			ui.Box(c).Size(stageBoxSize-6, stageBoxSize-6).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) { drawCheckMark(p, r, t.AccentText) })
		})
	} else if indeterminate {
		btn.Children(func() {
			ui.Box(c).Size(stageBoxSize-6, stageBoxSize-6).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) { drawMinusMark(p, r, t.AccentText) })
		})
	}

	if btn.Clicked() {
		if allStaged {
			a.unstageAll()
		} else {
			a.stageAll()
		}
	}
}

// fileFilterInput 是文件栏的筛选框。
//
// Label 和侧栏那个（「筛选分支」）必须分得开：两个框筛的是完全不同的东西，
// 名字一样的话测试和无障碍工具都认不出找的是哪一个。
func (a *app) fileFilterInput(c *ui.Context, pal palette) {
	in := searchField(c, pal, &a.fileFilter, T("筛选文件", "Filter files"))
	// ⌘P（以及命令栏里的「筛选文件」）之后把焦点放进来。
	//
	// 置位的那一帧文件栏可能已经画过（命令栏在覆盖层里，晚于它），所以要
	// 留到下一帧才轮到这儿。那时命令栏已经收掉、它的输入框不再构建，没有
	// 谁跟这里抢 —— `Focus()` 当场生效，放一次就够。
	if a.focusFilter {
		in.Focus()
		a.focusFilter = false
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
	a.filesList.Key = func(i int) any { return rows[i].Node.Key }
	ui.List(c, &a.filesList, len(rows), func(i int) {
		a.treeRow(c, pal, rows[i])
	}).Grow(1)
}

// emptyTreeMsg 是文件树空着时该说的话。空的原因有好几种，说错了会让人以为
// 是真没有改动。
func (a *app) emptyTreeMsg() string {
	switch {
	case a.state == loadIdle:
		return T("未打开仓库", "No repository opened")
	case a.state != loadReady && !a.repoReady():
		return "" // 仓库在读，diff 那边有转圈了，这里先空着
	case a.compare.on && a.compare.state == loadLoading:
		return T("正在计算分支差异…", "Calculating branch diff…")
	case a.compare.on && a.compare.state == loadFailed:
		return T("无法获取分支差异", "Failed to load branch diff")
	case a.compare.on && len(a.allFiles) == 0:
		return T("分支间无变更", "No changes between branches")
	case a.tab == 1 && a.hist == loadLoading && len(a.commits) == 0:
		return T("正在读取提交记录…", "Loading commit history…")
	case a.tab == 1 && a.hist == loadFailed && len(a.commits) == 0:
		return T("无法读取提交记录", "Failed to load history")
	case a.tab == 1 && len(a.commits) == 0:
		return T("暂无提交记录", "No commits found")
	case a.tab == 1 && a.histDiff == loadLoading && len(a.files) == 0:
		return T("正在读取提交详情…", "Loading commit details…")
	case a.tab == 1 && a.histDiff == loadFailed && len(a.files) == 0:
		return T("无法读取提交详情", "Failed to load commit")
	case a.fileFilter != "":
		return T("没有匹配的文件", "No matching files")
	}
	return T("工作区无改动", "Working tree clean")
}

// treeRow 画改动栏（左侧文件列表）里的一行。见 treeRowIn。
func (a *app) treeRow(c *ui.Context, pal palette, r filetree.Row) {
	a.treeRowIn(c, pal, r, rowSpec{list: a.files, sel: &a.fileSel, onClick: a.syncEditorPane, tree: treeChanges})
}

// rowSpec 是 treeRowIn 的入参：列表、选中下标、点击回调，加上「折叠状态
// 记在哪一份」「要不要按改动上色」这两条只跟所在栏有关的开关。同一套画法
// 因此能画两份列表：「改动」那份（a.files）和「编辑器」视图里整个工作区那
// 份（a.wsFiles）。
type rowSpec struct {
	list     []gitrepo.File
	sel      *int
	onClick  func()
	tree     treeFor
	decorate bool
}

// treeRowIn 画文件树的一行。目录只画名字；文件画名字 + 增删行数 + 状态字母。
//
// 行本身可点：目录行点了收起/展开，文件行点了让 diff 滚到那个文件的头上。
// 选中态按**文件下标**记，不用 ui.List 的 Selected —— 那是扁平行下标，树一
// 重建（筛选、折叠目录、刷新）就指到别的行去了，记文件下标怎么动都不会错。
// 文件从 o.list 里取、下标记在 o.sel 上，文件行点中之后再调 o.onClick，让
// 那一栏自己把内容同步过去。
func (a *app) treeRowIn(c *ui.Context, pal palette, r filetree.Row, o rowSpec) {
	t := c.Theme()
	n := r.Node

	if n.Dir {
		open := a.dirOpen(o.tree, n.Key)
		// 目录自己没有状态，「动没动过」看的是底下的文件。全收起的树里不给
		// 目录上色，等于把哪儿动过整条藏起来 —— 那就白收起一层了。
		labelColor := t.TextMuted
		if o.decorate {
			if s, ok := dirChangeStatus(n, o.list); ok {
				labelColor = statusColor(s, pal)
			}
		}
		row, arrow := treeRow(c, t, pal, treeRowOpts{Depth: r.Depth, HasKids: true, Open: open}, func() {
			ui.Text(c, n.Label).FontSize(12).SingleLine().Grow(1).MinWidth(0).
				TextColor(labelColor)
		})
		if arrow.Clicked() || row.Clicked() {
			a.toggleDir(o.tree, n.Key, open)
		}
		return
	}

	if n.FileIdx < 0 || n.FileIdx >= len(o.list) {
		treeRow(c, t, pal, treeRowOpts{Depth: r.Depth}, func() {
			ui.Text(c, n.Label).FontSize(12).SingleLine()
		})
		return
	}

	f := o.list[n.FileIdx]

	// 勾选框要留到行构建完之后再判点击（见下面的 early return）。它在行里
	// 是后建的子元素，指针事件会一路冒泡到整行 —— 不先问它，点一下既暂存
	// 了文件又跳转了 diff。
	var box ui.Element

	row, _ := treeRow(c, t, pal, treeRowOpts{Depth: r.Depth, Selected: n.FileIdx == *o.sel}, func() {
		// 历史提交和储藏的 diff 是只读的（SectionNone），那里不摆勾选框：
		// 一个点了会对别的仓库状态动手的方框，比没有方框更糟。
		if f.Section != gitrepo.SectionNone {
			box = stageBox(c, pal, stageStateOf(f), a.op != gitrepo.OpNone)
		}

		// 编辑器栏按改动给文件名上色（VSCode 那种：一眼扫出哪儿动过），
		// 改动栏里则只有「已 review 变灰」这一种。
		//
		// 编辑器栏里上色优先于变灰：那一栏本来就是按改动找文件的，灰掉
		// 等于把要找的东西藏起来。改动栏里反过来 —— 每个文件都改了，
		// 全都上色等于全彩，反而没了信号，所以那一栏不上色。
		nameColor := t.Text
		switch {
		case o.decorate && f.Status != "":
			nameColor = statusColor(f.Status, pal)
		case a.isViewed(f):
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

	// 右键菜单：暂存、丢弃、复制路径、看这个文件的历史……都在这里（见
	// fileMenu）。挂在每一行上，不挑状态 —— 一条路径能做的事本来就差不多，
	// 该灰的项菜单自己会灰。历史/储藏里的行也一样（那边的 diff 是只读的，
	// 菜单里就只剩读的那几项）。
	row.ContextMenu(func(m *ui.Menu) { a.fileMenu(m, f) })

	if box.Valid() && box.Clicked() {
		a.toggleStageFile(f)
		return
	}
	if row.Clicked() {
		*o.sel = n.FileIdx
		// 改动区跟着选中走（它在下一帧自己发现选中的文件变了）。
		if o.onClick != nil {
			o.onClick()
		}
	}
}

// treeFor 选折叠状态记在哪一份上。两份名单的默认值正好相反。
type treeFor int

const (
	treeChanges   treeFor = iota // 改动栏：默认展开
	treeWorkspace                // 编辑器栏的工作区：默认全收起
)

// dirSet 是该树的折叠记录。
func (a *app) dirSet(f treeFor) map[string]bool {
	if f == treeWorkspace {
		if a.wsOpen == nil {
			a.wsOpen = map[string]bool{}
		}
		return a.wsOpen
	}
	if a.filesClosed == nil {
		a.filesClosed = map[string]bool{}
	}
	return a.filesClosed
}

// dirOpen 问一个目录是不是展开的。
//
// 改动栏默认展开、只记「收起过哪些」—— review 改动时几乎总想直接看到
// 文件列表；编辑器栏的工作区树默认全收起、只记「展开过哪些」—— 一进
// 编辑器先看到的应该是目录结构，而不是一屏文件。记录里只存偏离默认的
// 那些，回到默认就把键删掉，来回切标签页不会越记越多。
func (a *app) dirOpen(f treeFor, key string) bool {
	set := a.dirSet(f)
	if f == treeWorkspace {
		return set[key] // 默认收起：记过的才是展开的
	}
	return !set[key] // 默认展开：记过的才是收起的
}

// toggleDir 翻转一个目录。open 是当前状态，翻转后回到默认值就把记录删掉，
// 于是来回点不会在记录里越攒越多。
func (a *app) toggleDir(f treeFor, key string, open bool) {
	set := a.dirSet(f)
	nw := !open
	// 两份记录的语义相反（见 dirOpen）：改动栏存「收起」，工作区存「展开」。
	if f == treeWorkspace {
		if nw { // 展开 —— 偏离默认，要记
			set[key] = true
			return
		}
		delete(set, key) // 回到默认收起
		return
	}
	if !nw { // 收起 —— 偏离默认，要记
		set[key] = true
		return
	}
	delete(set, key) // 回到默认展开
}

// dirChangeStatus 找目录里最要紧的那个改动状态。目录自己不会改，改的是
// 底下的文件；全收起的树里不给目录上色，等于把「哪儿动过」整条藏起来。
//
// 优先级只用来在目录上挑一个代表的颜色，按「看见它的迫切程度」排：冲突
// 和删除最要紧（文件可能已经不在工作区里了），其次是修改，再次是重命名
// 或复制，最后才是新增 —— 新增的文件至少还好好待在那儿。
func dirChangeStatus(n *filetree.Node, list []gitrepo.File) (gitrepo.Status, bool) {
	var best gitrepo.Status
	found := false
	for _, kid := range n.Children {
		var s gitrepo.Status
		var ok bool
		switch {
		case kid.Dir:
			s, ok = dirChangeStatus(kid, list)
		case kid.FileIdx >= 0 && kid.FileIdx < len(list):
			s, ok = list[kid.FileIdx].Status, list[kid.FileIdx].Status != ""
		}
		if !ok {
			continue
		}
		if !found || statusRank(s) < statusRank(best) {
			best, found = s, true
		}
	}
	return best, found
}

// statusRank 越小越要紧，只用于在目录上挑一个代表的颜色（见 dirChangeStatus）。
func statusRank(s gitrepo.Status) int {
	switch s {
	case gitrepo.StatusConflict:
		return 0
	case gitrepo.StatusDeleted:
		return 1
	case gitrepo.StatusModified:
		return 2
	case gitrepo.StatusRenamed, gitrepo.StatusCopied:
		return 3
	}
	return 4
}

// filePaneFooter 是文件栏底下的那条栏脚：整份改动一共增删了多少行，
// 以及已 review 的进度。
//
// 顶上描一条细线、底色抬一档：它是常驻的状态栏，不是列表的最后一行。
// 不画出来，这几个字就直接浮在底色上，像是谁忘了删的一行；画出来，
// 文件栏一整栏也就有了下沿。
//
// 数字和文件行里的 +/− 用同一套颜色和等宽字体：同一份数据在哪儿都长
// 一个样，扫一眼就知道是对得上的。
func (a *app) filePaneFooter(c *ui.Context, pal palette, t *ui.Theme) {
	ui.Row(c).Shrink(0).Padding(7, 12).Gap(6).AlignItems(ui.Center).
		Background(pal.surface2).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
		ui.Text(c, T("总计", "Total")).FontSize(textCaption).TextColor(pal.inkSubtle)
		ui.Textf(c, "+%d", a.totalAdd()).FontSize(11).Bold().
			Font("monospace").FontFeatures("tnum").TextColor(pal.addBar)
		ui.Textf(c, "−%d", a.totalDel()).FontSize(11).Bold().
			Font("monospace").FontFeatures("tnum").TextColor(pal.delBar)
		// 冲突数摆在总计旁边：它是「这份改动现在能不能提交」的答案 ——
		// 不解决完，提交上去的会是带着 <<<<<<< 的文件。
		if n := a.conflictCount(); n > 0 {
			warnIcon(c, 11, t.Warning)
			ui.Textf(c, T("%d 个冲突待解决", "%d conflicts to resolve"), n).FontSize(11).
				FontFeatures("tnum").TextColor(t.Warning)
		}
		ui.Spacer(c)
		if n := a.viewedCount(); n > 0 {
			ui.Box(c).Size(checkSize, checkSize).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) { drawCheckMark(p, r, pal.viewed) })
			ui.Textf(c, "%d/%d", n, len(a.files)).
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

// statusColor 是文件行尾那个状态字母的颜色（§2.5 的小标签亮色）。
//
// 字母只有 10px，A/D 沿用 diff 的绿与红（同一行里出现两种相近的绿比没有绿更糟），
// M 和 R/C 各自拿一支亮色 —— 这两个原来一起落在 pal.ref 的灰上，
// 「改了」「重命名了」在一列灰字母里得逐个读才分得出来。
func statusColor(s gitrepo.Status, pal palette) ui.Color {
	switch s {
	case gitrepo.StatusAdded, gitrepo.StatusUntracked:
		return pal.addBar
	case gitrepo.StatusDeleted, gitrepo.StatusConflict:
		return pal.delBar
	case gitrepo.StatusModified:
		return pal.accentAmber
	case gitrepo.StatusRenamed, gitrepo.StatusCopied:
		return pal.accentBlue
	}
	return pal.ref // 剩下的（理论上走不到）留中性，别硬塞一个色
}

// detailEmpty 是下半区没有 diff 可画时的几种情况。
func (a *app) detailEmpty(c *ui.Context, t *ui.Theme, pal palette) {
	switch {
	case a.state == loadIdle:
		headline(c, pal, T("还没有打开仓库", "No Repository Opened"))
		ui.Text(c, T("请选择本地 Git 仓库以浏览与审查代码变更。", "Select a local Git repository to inspect changes.")).
			TextColor(t.TextMuted)
		if primaryAction(c, pal, T("打开仓库…", "Open Repository…"), "", false).Clicked() {
			a.chooseFolder()
		}
		// 第一眼的地方，顺手把菜单里的那个键说一遍：下次就不用找按钮了。
		ui.Text(c, Tf("快捷键 %s", "Press %s to open", K("⌘O"))).FontSize(textCaption).TextColor(t.TextMuted)
	case a.state == loadLoading && !a.repoReady():
		ui.Spinner(c)
		ui.Text(c, T("正在读取仓库…", "Loading repository…")).TextColor(t.TextMuted)
	case a.state == loadFailed && a.errNotRepo:
		headline(c, pal, T("打不开这个仓库", "Unable to Open Repository"))
		ui.Text(c, T("这个文件夹不是 git 仓库。", "The selected folder is not a valid Git repository.")).TextColor(t.TextMuted)
		if primaryAction(c, pal, T("换一个文件夹…", "Choose Another Folder…"), "", false).Clicked() {
			a.chooseFolder()
		}
	case a.state == loadFailed:
		headline(c, pal, T("无法打开仓库", "Unable to Open Repository"))
		ui.Text(c, a.errMsg).Font("monospace").FontSize(textBodySM).TextColor(t.TextMuted)
		if secondaryAction(c, pal, T("重试", "Retry"), "", false).Clicked() {
			a.reload()
		}
	case a.compare.on:
		headline(c, pal, T("两分支之间无代码差异", "No Code Differences"))
		ui.Textf(c, T("分支 %s 与 %s 内容完全一致。", "Branches %s and %s are identical."), a.compare.base, a.compare.target).
			TextColor(t.TextMuted)
	case a.tab == 1:
		a.historyEmpty(c, t, pal)
	case a.fileFilter != "":
		headline(c, pal, T("没有匹配的文件", "No Matching Files"))
		ui.Textf(c, T("未检索到包含「%s」的改动文件。", "No changed files matching \"%s\"."), a.fileFilter).
			TextColor(t.TextMuted)
	default:
		headline(c, pal, T("没有未提交的改动", "No Uncommitted Changes"))
		ui.Text(c, T("工作区状态良好，暂无代码变更。", "Working tree is clean, no uncommitted changes.")).TextColor(t.TextMuted)
	}
}

// navFile 用方向键在「改动」文件列表里移动选中（dir：+1 下一个，-1 上一个）。
//
// 走的是和点击同一套状态：只改 fileSel，右边那一栏下一帧自己发现选中的文件
// 变了。列表是树形扁平化出来的，中间夹着目录行（FileIdx 为 -1），所以要
// 跳过它们 —— 方向键一下跳过一个目录，落到下一个真正的文件上。
//
// 没选中过任何文件时（fileSel < 0）从第一个文件开始，不至于按下去没反应。
func (a *app) navFile(dir int) {
	// 编辑器视图的左栏列的是整个工作区，方向键该在那份列表里走 ——
	// 两个视图各记各的选中下标。
	if a.viewMode == viewModeEditor {
		a.navWorkspaceFile(dir)
		return
	}
	if len(a.files) == 0 {
		return
	}
	rows := filetree.Flatten(a.tree, a.filesClosed, a.fileFilter != "")

	// 当前选中的文件在这份扁平行里的位置；没选中时起算点要落在第一个
	// **文件**行上 —— 列表的头一行常常是目录（FileIdx = -1），直接取 0
	// 会撞在目录上，下面那一关又要求是文件，结果等于什么都没发生。
	cur := -1
	if a.fileSel >= 0 {
		for i, r := range rows {
			if r.Node.FileIdx == a.fileSel {
				cur = i
				break
			}
		}
	}

	// nextFrom 是「从哪儿开始找」：没选中过就从列表起点（↓）或终点（↑），
	// 找的时候只认真实文件行。
	next := -1
	nextFrom := cur + dir
	if cur < 0 {
		if dir < 0 {
			nextFrom = len(rows) - 1
		} else {
			nextFrom = 0
		}
	}
	for i := nextFrom; i >= 0 && i < len(rows); i += dir {
		if rows[i].Node.FileIdx >= 0 {
			next = i
			break
		}
	}
	if next < 0 {
		return // 已经到边界了
	}

	idx := rows[next].Node.FileIdx
	if idx < 0 || idx >= len(a.files) {
		return
	}
	a.fileSel = idx
	a.syncEditorPane()
	a.filesList.ScrollIntoView(next)
}
