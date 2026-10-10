package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

const (
	sidebarWidth    = 280
	sidebarDefaultW = float32(sidebarWidth)
	sidebarMinW     = float32(180)
	sidebarMaxW     = float32(600)
)

// sidebar 是左侧栏：项目名称 + 工作区导航（改动/历史）+ 分支/远程/标签/储藏树。
//
// 参照 Fork 布局规范，侧栏顶部承载最高层级项目卡片与工作区导航。
func (a *app) sidebar(c *ui.Context, pal palette) {
	bar := c.TitleBar()
	t := c.Theme()
	sw := a.sidebarWidth(c)

	ui.Column(c).Width(sw).Shrink(0).
		Background(pal.sideBg.Alpha(0.55)).
		Children(func() {
			if bar.Height > 0 {
				ui.Box(c).Height(bar.Height).Shrink(0).DragWindow()
			}

			// 侧栏顶部项目名称与工作区导航（Fork 风格：改动与历史）
			a.sidebarHeader(c, t, pal)

			if a.repoReady() {
				ui.Box(c).Key("ref-filter-wrap").Padding(0, 8, 6, 8).Shrink(0).Children(func() {
					a.refFilterInput(c, pal)
				})
			}
			ui.Box(c).Key("sidebar-refs-wrap").Grow(1).FillWidth().Children(func() {
				a.refsView(c, pal)
			})

			// 侧栏右边缘拖动手柄：鼠标悬停显示双向箭头，可拖拽实时调节侧栏宽度，双击复位默认
			grip := float32(6)
			resizer := ui.Box(c).Absolute().Top(0).Bottom(0).Right(0).Width(grip).
				Cursor(ui.CursorResizeEW).Role(ui.RoleNone)
			if resizer.DoubleClicked() {
				a.resetSidebarWidth()
			}
			if dx, _, held := resizer.Dragged(); held && dx != 0 {
				a.setSidebarWidth(c, sw+dx)
				a.sidebarDragging = true
			} else if a.sidebarDragging && !held {
				a.sidebarDragging = false
				a.saveViewSettings()
			}
			resizer.Draw(func(p *ui.Painter, r ui.Rect) {
				if resizer.Hovered() || resizer.Pressed() {
					p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, t.Accent, 0)
				}
			})
		})
}

// sidebarHeader 呈现侧边栏顶部：仓库名称下拉卡片 + 工作区核心导航（改动 / 历史）。
// 严格参照 Fork 交互设计，将仓库名与「改动 (带待提交计数角标)」/「历史」置于最高主导航层级。
func (a *app) sidebarHeader(c *ui.Context, t *ui.Theme, pal palette) {
	ui.Column(c).Padding(2, 8, 4, 8).Gap(3).Shrink(0).FillWidth().Children(func() {
		// 1. 仓库名称卡片（点击展开最近项目与打开仓库）
		a.sidebarRepoCard(c, t, pal)

		// 2. 工作区主导航（改动与历史）
		a.sidebarWorkspaceNav(c, t, pal)

		// 分割线
		ui.Box(c).Height(1).FillWidth().Margin(4, 2).Background(pal.hairline)
	})
}

// sidebarRepoCard 呈现仓库名称与最近项目下拉卡片入口。
func (a *app) sidebarRepoCard(c *ui.Context, t *ui.Theme, pal palette) {
	name := a.repoName
	if name == "" {
		name = "Gloss"
	}
	hasRepo := a.repoReady()
	sw := a.sidebarWidth(c)
	sub := ""
	if a.repoPath != "" {
		sub = shortPath(c, a.repoPath, sw-40)
	}

	card := ui.ButtonBase(c).FillWidth().Radius(radiusSM).
		Padding(5, 8).
		Cursor(ui.CursorPointer).Label(name).
		Tooltip(T("点击切换最近项目（或 ⌘O 打开本地仓库）", "Switch recent repository (⌘O)"))

	bg := ui.Transparent
	if card.Hovered() || a.repoDropdownOpen {
		bg = pal.surface2
	}
	card.Background(bg)
	fadeColors(card)

	card.Children(func() {
		ui.Column(c).Gap(2).FillWidth().Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				folderCol := pal.inkSubtle
				if hasRepo {
					folderCol = pal.inkMuted
				}
				drawIcon(c, iconFolder, 14, folderCol)

				ui.Text(c, name).FontSize(textBody).FontWeight(weightSemibold).
					SingleLine().Grow(1).MinWidth(0).TextColor(pal.ink)

				drawIcon(c, iconChevronDown, 10, pal.inkSubtle)
			})
			if sub != "" {
				ui.Text(c, sub).FontSize(textCaption).SingleLine().TextColor(pal.inkSubtle).
					Tooltip(a.repoPath)
			}
		})
	})

	if card.Clicked() {
		a.branchDropdownOpen = false
		a.repoDropdownOpen = !a.repoDropdownOpen
	}
	if a.repoDropdownOpen {
		ui.PopoverBase(c, card, &a.repoDropdownOpen, func(panel *ui.Element) {
			a.repoDropdownPanel(c, panel, pal)
		})
	}
}

// sidebarWorkspaceNav 呈现 Fork 规范的工作区核心导航项：改动（带改动文件计数）与 历史。
func (a *app) sidebarWorkspaceNav(c *ui.Context, t *ui.Theme, pal palette) {
	noRepo := !a.repoReady()

	// 1. 改动（Local Changes）
	a.sidebarNavItem(c, t, pal, 0, T("改动", "Changes"), T("工作区改动 ⌘1", "Working tree changes ⌘1"), iconFileDiff, noRepo, a.dirtyCount())

	// 2. 历史（All Commits）
	a.sidebarNavItem(c, t, pal, 1, T("历史", "History"), T("提交历史与拓扑图 ⌘2", "Commit history & graph ⌘2"), iconGitCommit, noRepo, 0)
}

// sidebarNavItem 画侧栏主导航的一行（如「改动」或「历史」）。
//
// 激活态照 §4.3 来：Surface 2 底色 + 左侧 2px 强调白指示条 + 提亮到 Ink 的文字，
// 不再是原来那块满宽的蓝底。三条线索里只有指示条带颜色 —— 位置、明度、色相
// 各说一遍「就是这一项」。指示条是全屏少数几处实心强调之一，其余留给主按钮。
func (a *app) sidebarNavItem(c *ui.Context, t *ui.Theme, pal palette, tabIdx int, label, tip string, svg *ui.SVG, disabled bool, badgeCount int) {
	selected := a.tab == tabIdx && !disabled
	btn := ui.ButtonBase(c).Height(28).FillWidth().Radius(radiusSM).
		Padding(0, 8).Gap(7).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Disabled(disabled)

	bg := ui.Transparent
	fg := pal.inkMuted
	iconCol := pal.inkMuted
	switch {
	case disabled:
		// 禁用态用 Ink Tertiary（§2.3），不是把 Ink Subtle 调暗 ——
		// 那一档本身就是给「还没轮到你」的东西留的。
		fg = pal.inkTertiary
		iconCol = pal.inkTertiary
		btn.Cursor(ui.CursorDefault)
	case selected:
		bg = pal.surface2
		fg = pal.ink
		iconCol = pal.ink
	case btn.Hovered():
		bg = pal.surface2
		fg = pal.ink
		iconCol = pal.ink
	}
	btn.Background(bg)
	if selected {
		// 指示条直接画在按钮左沿：放进内容流里当第一个子元素的话，它会落在
		// 8px 内边距之后，看着像内容的一部分，而不是「这一行被选中了」。
		btn.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + 6, W: 2, H: r.H - 12}, t.Accent, 1)
		})
	}
	fadeColors(btn)

	btn.Children(func() {
		drawIcon(c, svg, 14, iconCol)

		txt := ui.Text(c, label).FontSize(textButton).Grow(1).SingleLine().TextColor(fg)
		if selected {
			txt.FontWeight(weightSemibold)
		}

		if badgeCount > 0 && !disabled {
			// 计数角标是 §3 里唯一用 pill 的地方。
			ui.Box(c).Radius(radiusPill).Padding(1, 6).
				Background(pal.surface3).Children(func() {
				ui.Text(c, strconv.Itoa(badgeCount)).FontSize(textCaption).
					FontWeight(weightMedium).
					Font("monospace").FontFeatures("tnum").
					TextColor(t.TextMuted)
			})
		}
	})

	if btn.Clicked() {
		a.setTab(tabIdx)
	}
}

// refFilterInput 是侧栏的筛选框。
//
// 名字写成「筛选分支」而不是「筛选」：主区下半还有一个筛选框（筛文件），
// 两个框名字一样的话，测试和无障碍工具都分不出找的是哪一个。
func (a *app) refFilterInput(c *ui.Context, pal palette) {
	searchField(c, pal, &a.refFilter, T("筛选分支", "Filter branches"))
}

func (a *app) refsView(c *ui.Context, pal palette) {
	t := c.Theme()

	switch a.refsState {
	case loadIdle:
		// 空白不等于「没事发生」：说明这个位置以后会有什么，比留一片
		// 灰底好 —— 用户第一次打开时看到的就是这一屏。
		emptyNote(c, t, T("打开仓库后，此处将展示分支、远程与标签。", "Branches, remotes, and tags will appear here once a repository is opened."))
		return
	case loadLoading:
		if len(a.refs) == 0 {
			emptyNote(c, t, T("正在读取分支列表…", "Loading branches…"))
			return
		}
	case loadFailed:
		if len(a.refs) == 0 {
			emptyNote(c, t, T("无法读取分支列表", "Failed to load branches"))
			return
		}
	}

	if a.refFilter != a.refsFilter {
		a.rebuildRefTree()
	}
	if len(a.refTree) == 0 {
		emptyNote(c, t, T("无匹配分支", "No matching branches"))
		return
	}

	// 筛选态一律展开：搜出来的分支藏在收起的分组里，等于没搜到。
	rows := filetree.Flatten(a.refTree, a.refsClosed, a.refFilter != "")
	a.refsList.Key = func(i int) any { return rows[i].Node.Key }
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

	row, _ := treeRow(c, t, pal, opts, func() {
		if ref.Kind == gitrepo.RefStash {
			// 储藏的名字（stash@{0}）人人都一样，认出它是哪一条全靠信息；
			// 两个都画，名字压暗当索引，信息才是给人读的。
			ui.Text(c, ref.Name).FontSize(11).Font("monospace").Shrink(0).
				TextColor(t.TextMuted)
			ui.Text(c, ref.Subject).FontSize(12).SingleLine().
				Grow(1).MinWidth(0).TextColor(t.Text)
			return
		}

		// 常规分支名用 Ink Muted，只有当前检出的那条提到 Ink（§2.3）：
		// 一列分支读的是「有哪几条」，「我在哪条」才需要更亮的那一档。
		txt := ui.Text(c, n.Label).FontSize(12).SingleLine().
			Grow(1).MinWidth(0).TextColor(pal.inkMuted)
		if ref.Head {
			txt.FontWeight(weightSemibold).TextColor(pal.ink)
			// HEAD 芯片走 §2.5 的配方，和提交行里那些引用芯片同一副面孔。
			specChip(c, pal, "HEAD", pal.laneColor(0), rowChipH, rowChipText)
		}
	})

	// 右键菜单按引用的种类给出可用操作；双击 = 检出（仅分支和标签支持）。
	row.ContextMenu(func(m *ui.Menu) { a.refMenu(m, *ref) })

	// 双击要问在单击之前：这两下是各自独立的（第二下按下去时两个都是真），
	// 反过来写的话双击会先切一次历史再检出，屏幕上闪一下完全无关的东西。
	if row.DoubleClicked() && checkoutable(*ref) {
		a.refDoubleClicked(n.Key)
		a.checkoutRef(*ref)
		return
	}
	if row.Clicked() && !a.swallowTrailingClick(n.Key) {
		a.selectRef(n.Key, *ref)
	}
}

// refDoubleClicked 记下「这一行刚被双击过」，见 swallowTrailingClick。
func (a *app) refDoubleClicked(key string) {
	a.dblKey, a.dblAt = key, time.Now()
}

// swallowTrailingClick 吃掉双击之后紧跟的那一次单击，返回 true。
//
// 一次双击是两次完整的按下抬起，而每次抬起都会让框架渲染一帧（真实鼠标也
// 是这样，按键之间隔着好几帧）：第一下抬起已经算了一次单击，第二下抬起又
// 算一次。后面这一次要是不吃掉，刚双击检出完，它就把界面又切去历史 tab ——
// 用户看到的是「双击了，人却去了别的地方」。
//
// 时间窗用框架自己认双击的那 500ms（ui/input.go 的 lastPress）：同一个窗口
// 里、同一行上的紧跟单击就是双击的尾巴，再晚就是用户真的又点了一下。
func (a *app) swallowTrailingClick(key string) bool {
	if a.dblKey == "" || a.dblKey != key || time.Since(a.dblAt) > 500*time.Millisecond {
		return false
	}
	a.dblKey = ""
	return true
}

// refGroupRow 画一个分组行（本地分支 / 远程 / 标签 / 储藏）。整行可点，
// 点哪儿都是收起或展开 —— 只有一个 12px 的小三角能点，用起来很别扭。
func (a *app) refGroupRow(c *ui.Context, t *ui.Theme, pal palette, r filetree.Row) {
	open := len(r.Node.Children) > 0 && !a.refsClosed[r.Node.Key]

	// 分组行不参与选中：它不是一个可以「去看历史」的东西
	row, arrow := treeRow(c, t, pal, treeRowOpts{Depth: r.Depth, HasKids: true, Open: open}, func() {
		ui.Text(c, r.Node.Label).FontSize(textCaption).FontWeight(weightSemibold).
			SingleLine().TextColor(pal.inkSubtle)
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

	if a.histRev != ref.Full {
		a.histRev = ref.Full
		a.commitSel = -1
		a.commits = nil
		a.histFiles = nil
		a.histListID = ""
		a.stashRev = "" // 换看分支了，下半区不再是储藏
		a.histDiff = loadIdle
	}
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
