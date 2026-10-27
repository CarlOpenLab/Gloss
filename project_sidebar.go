package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// sidebarMotion 是侧栏展开/收起的那条曲线。
//
// 用曲线而不是弹簧：弹簧的尾巴常常一帧挪不到一个像素，侧栏边缘在收尾的
// 十几帧里会一格一格地跳；这条 260ms 的缓出曲线每一帧都在动，落点在
// 最后一帧对齐。
var sidebarMotion = ui.Animation{
	Duration: 260 * time.Millisecond,
	Ease:     ui.CubicBezier(0.25, 1, 0.5, 1),
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// 项目栏的高度：默认跟着内容长，拖过分隔线之后按用户给的高度固定下来。
// 0 表示「跟着内容」，这时内容再长也只在 max 以内滚。
const (
	projectsMinH    = float32(56)
	projectsRowH    = float32(30)
	projectsGap     = float32(2)
	projectsHeadH   = float32(22)
	projectsDragHit = float32(9)
)

// projectsContentHeight 是项目列表自然需要的高度：分组标题 + 每条项目。
func (a *app) projectsContentHeight() float32 {
	n := len(a.tabs)
	if n == 0 {
		n = 1
	}
	return projectsHeadH + projectsGap + float32(n)*(projectsRowH+projectsGap)
}

// projectsMaxHeight 是分隔线能拉到的最下面：给分支那一段留下至少
// 一条分支的高度，项目再多也不把分支区挤没。
func (a *app) projectsMaxHeight(c *ui.Context) float32 {
	_, winH := c.Size()
	reserved := float32(shellTopBar + 38 + 28 + 38 + 120)
	return max(projectsMinH, winH-reserved)
}

// projectsPaneHeight 取这一帧项目栏该有多高：用户拖过就用用户的，
// 没拖过就按内容，最后都夹在 [min, max] 里。
func (a *app) projectsPaneHeight(c *ui.Context) float32 {
	return a.projectsPaneHeightWithin(a.projectsMaxHeight(c))
}

// projectsPaneHeightWithin 是不带 Context 的那一版：拖动发生在帧与帧
// 之间，这时候 c.Size() 已经是零，只能在建视图的那一帧先把上限算好
// 交给回调。
func (a *app) projectsPaneHeightWithin(maxH float32) float32 {
	h := a.projectsHeightVal
	if h <= 0 {
		h = a.projectsContentHeight()
	}
	return clamp(h, projectsMinH, maxH)
}

// projectsDivider 是项目区与分支区之间那条静息线。它只负责「看得见」，
// 拖动由压在上面的绝对抓手接（见 projectSidebar）。
func projectsDivider(c *ui.Context, pal palette) {
	ui.Box(c).Height(projectsDragHit).Shrink(0).FillWidth().Children(func() {
		ui.Box(c).Height(1).FillWidth().Margin(4, 10, 4, 10).Background(pal.hairline)
	})
}

// sidebarShell 画侧栏以及它展开/收起的那段宽度动画。
//
// progress 来自根行上的 AnimateTo（见 app.view）。侧栏的内容始终按「展开
// 后」的宽度排版，外壳只裁掉右边多出来的部分 —— 让内容跟着宽度重排的话，
// 收起过程里项目名会先被省略号截断、分支计数先消失，看起来像界面坏了，
// 而不是像侧栏滑走。
func (a *app) sidebarShell(c *ui.Context, pal palette, progress float32) {
	shown := clamp(progress, 0, 1)
	sideW := a.sidebarWidth(c)
	w := float32(math.Round(float64(sideW * shown)))

	ui.Row(c).Key("sidebar-shell").Width(w).Shrink(0).FillHeight().
		AlignItems(ui.Stretch).ClipX().Children(func() {
		if w <= 0.5 {
			return
		}
		// 负的右边距让固定宽度的内容在外壳里保持左对齐：不这么做的话，
		// 布局会去挤那根比外壳还宽的柱子，项目名在动画中途就开始折行。
		ui.Column(c).Width(sideW).Shrink(0).FillHeight().
			Margin(0, w-sideW, 0, 0).
			Opacity(0.35 + 0.65*shown).
			Children(func() { a.projectSidebar(c, pal) })
	})
}

// projectSidebar puts projects first, like a desktop session sidebar. A
// project is a workspace: clicking one switches to it, with its terminal
// sessions kept alive (see switchTab). Branches remain the repository's
// secondary navigation below the project list.
func (a *app) projectSidebar(c *ui.Context, pal palette) {
	bar := c.TitleBar()
	topH := max(float32(shellTopBar), bar.Height)
	sideW := a.sidebarWidth(c)

	ui.Column(c).FillHeight().Children(func() {
		// The title row stays a drag surface; a fixed overlay keeps its SVG
		// buttons from participating in flex sizing as the sidebar is resized.
		ui.Box(c).Height(topH).Shrink(0).FillWidth().DragWindow()
		a.sidebarToolbar(c, pal, false, sideW, bar)

		a.projectGroups(c, pal, a.projectsPaneHeight(c))

		// 项目与分支之间这条缝可以拖：往上收项目区、往下放项目区。
		// 视觉上只有一条 1px 的静息线，拖动时不画强调色 —— 指针形状
		// 已经说了这里能拖，不需要再亮一条蓝线。
		projectsDivider(c, pal)
		gripTop := float32(shellTopBar) + a.projectsPaneHeight(c)
		projectsGrip := ui.Box(c).Key("projects-resizer").Absolute().
			Top(gripTop).Left(0).Right(0).Height(projectsDragHit).
			Cursor(ui.CursorResizeNS).Role(ui.RoleNone).
			Label(T("调整项目与分支的高度", "Resize projects and branches"))
		maxH := a.projectsMaxHeight(c)
		projectsGrip.HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputPointerDown:
				// 双击复位：项目区回到「跟着内容长」。HandleInput 拿走
				// 了按下事件，框架记不到「双击」，所以按两次抬起之间的
				// 间隔自己判。
				if !a.projectsGripUpAt.IsZero() && time.Since(a.projectsGripUpAt) < 350*time.Millisecond {
					a.projectsHeightVal = 0
					a.projectsDragging = false
					a.projectsGripUpAt = time.Time{}
					a.saveViewSettings()
					return true
				}
				a.projectsDragging = true
				a.projectsGripMoved = false
				// 抓手本身会跟着高度一起往下走，事件里的 Y 是相对抓手的：
				// 直接拿它算增量，第二次移动就会变成 0（元素跟着指针跑了）。
				// 换成「抓手顶端 + 相对 Y」= 指针在侧栏里的绝对 Y。
				a.projectsGripLastY = gripTop + ev.Y
				return true
			case ui.InputPointerMove:
				if !a.projectsDragging {
					return false
				}
				if abs := gripTop + ev.Y; abs != a.projectsGripLastY {
					dy := abs - a.projectsGripLastY
					a.projectsGripLastY = abs
					a.projectsGripMoved = true
					a.projectsHeightVal = clamp(
						a.projectsPaneHeightWithin(maxH)+dy, projectsMinH, maxH)
				}
				return true
			case ui.InputPointerUp:
				if a.projectsDragging {
					a.projectsDragging = false
					if a.projectsGripMoved {
						a.projectsGripUpAt = time.Time{}
					} else {
						a.projectsGripUpAt = time.Now()
					}
					a.saveViewSettings()
				}
				return true
			}
			return false
		})
		projectsGrip.Draw(func(p *ui.Painter, r ui.Rect) {
			if projectsGrip.Hovered() || projectsGrip.Pressed() {
				// 悬停时中间露出一小段抓握纹，告诉人这条缝能动。
				p.Fill(ui.Rect{X: r.X + r.W/2 - 16, Y: r.Y + r.H/2 - 1.5, W: 32, H: 3},
					pal.textFaint.Alpha(0.55), 1.5)
			}
		})

		ui.Row(c).Height(28).Shrink(0).FillWidth().Padding(0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
			shellSectionLabel(c, pal, T("分支", "BRANCHES"))
			ui.Spacer(c)
		})

		if a.repoReady() {
			ui.Box(c).Key("ref-filter-wrap").Padding(0, 10, 6, 10).Shrink(0).Children(func() {
				a.refFilterInput(c, pal)
			})
		}
		ui.Box(c).Key("sidebar-refs-wrap").Grow(1).FillWidth().MinHeight(0).Children(func() {
			a.refsView(c, pal)
		})

		// 底部留一口气：命令面板与设置那两个图标在工具栏左端（见 toolbar），
		// 这一行原来占的 38px 变成 8px 的留白。
		ui.Box(c).Height(8).Shrink(0)
	})
}

// sidebarToolbar is the three fixed title controls shared by the expanded
// sidebar and the folded card. Expanded controls hug the sidebar's right edge;
// folded controls stay in the same title-bar slot after the traffic lights.
func (a *app) sidebarToolbar(c *ui.Context, pal palette, hidden bool, width float32, bar ui.TitleBar) {
	row := ui.Row(c).Key("sidebar-toolbar").Absolute().
		Top((max(float32(shellTopBar), bar.Height) - shellIconButtonH) / 2).
		Height(shellIconButtonH).Gap(shellIconButtonGap).AlignItems(ui.Center).
		Shrink(0)
	if hidden {
		row.Left(sidebarControlsLeft(bar)).Width(sidebarControlsW).Justify(ui.Start)
	} else {
		row.Left(0).Width(width).Padding(0, 9).Justify(ui.End)
	}
	row.Children(func() {
		shellIconButton(c, pal, T("命令面板", "Command Palette"),
			Tf("搜索命令与项目 %s", "Search commands and projects %s", K("⌘K")),
			iconSearch, false, a.openPalette)
		shellIconButton(c, pal, T("模型设置…", "Model Settings…"),
			Tf("偏好与设置 %s", "Preferences %s", K("⌘,")),
			iconSettings, false, a.openSettings)
		toggle := func() { a.setSidebar(hidden) }
		shellIconButton(c, pal, a.sidebarToggleLabel(), a.sidebarToggleTip(),
			iconSidebar, false, toggle)
	})
}

// sidebarResizer 是侧栏与主区之间那条 9px 宽的拖拽带。
//
// 它压在白色卡片的左沿上（往侧栏里 4px、往主区里 5px），而不是浮在侧栏
// 与卡片之间那段 8px 留白的中间：眼睛看见的边界是卡片的边线，指针就该在
// 那条线上变成左右箭头，拖起来也像是直接拽着卡片走。
//
// 这条带子自己不画线。卡片左沿本来就有自己那条 1px 硬边（raisedSurface
// 的 litSurfaceEdge），悬停 / 按下 / 松手时再描一档亮的，就是压在边界上的
// 一道白杠：按下去的那一帧、松手后指针还压在带子上的那一段，一次普通的
// 拖动两头各闪一下。能不能拖由光标说（CursorResizeEW），不由一条线说。
//
// 它挂在根行上、排在主区之后：主区那一层盖住侧栏的右边缘，抓手留在侧栏里
// 的话，伸出去的那一半收不到指针。
func (a *app) sidebarResizer(c *ui.Context) {
	// 只在侧栏完全展开时给抓手：收起 / 展开的动画途中按住的不是一条稳定
	// 的边线，拖起来会跳。
	if a.sidebarShown < 0.98 {
		return
	}

	// 侧栏外壳的宽度是 round 过的（见 sidebarShell），卡片左沿按同一个
	// 数算，抓手才不会落在边线外半个像素上。
	shellW := float32(math.Round(float64(a.sidebarWidth(c) * clamp(a.sidebarShown, 0, 1))))
	edge := shellW + shellCardGap

	handle := ui.Box(c).Key("sidebar-resizer").Absolute().
		Top(0).Bottom(0).Left(edge - sidebarHandleW/2).Width(sidebarHandleW).
		Cursor(ui.CursorResizeEW).Role(ui.RoleNone).
		Label(T("调整侧栏宽度", "Resize sidebar"))

	if handle.DoubleClicked() {
		a.resetSidebarWidth()
	}
	if dx, _, held := handle.Dragged(); held && dx != 0 {
		a.setSidebarWidth(c, a.sidebarWidth(c)+dx)
		a.sidebarDragging = true
	} else if a.sidebarDragging && !held {
		a.sidebarDragging = false
		a.saveViewSettings()
	}
}

// sidebarHandleW 是侧栏拖拽带的命中宽度：卡片左沿压在正中间，左右各留
// 4px 的容错 —— 一条边线是世界上最难点中的东西。
const sidebarHandleW = float32(9)

// projectGroups 列出所有打开的项目。项目之间没有展开关系：一条项目就是
// 一个工作区，点一下切过去，终端会话留在各自的项目里（见 switchTab）。
//
// 拖动排序由这一层统一接：指针落在整个列表的哪一段，就插到哪儿 ——
// 不是「必须压在某一行的上半条/下半条」，更不需要先去碰那条蓝线。
// 拖动过程中被拖的那行留在原位压淡，落点处浮出一块浅色的「空位」。
func (a *app) projectGroups(c *ui.Context, pal palette, h float32) {
	var rowEls []ui.Element

	projects := func() {
		ui.Row(c).Height(22).FillWidth().Padding(0, 2).AlignItems(ui.Center).Children(func() {
			shellSectionLabel(c, pal, T("项目", "PROJECTS"))
			ui.Spacer(c)

			plus := ui.ButtonBase(c).Size(22, 22).Radius(radiusXS).Center().
				Cursor(ui.CursorPointer).Label(T("添加项目", "Add Project")).
				Tooltip(Tf("打开项目或本地 Git 仓库 %s / %s", "Open a project or local Git repository %s / %s", K("⌘T"), K("⌘O")))
			if plus.Hovered() {
				plus.Background(pal.rowHover)
			}
			fadeColors(plus)
			plus.Children(func() { drawIcon(c, iconPlus, 14, pal.textMuted) })
			if plus.Clicked() {
				a.repoDropdownOpen = false
				a.branchDropdownOpen = false
				a.newTabMenuOpen = !a.newTabMenuOpen
			}
			if a.newTabMenuOpen {
				ui.PopoverBase(c, plus, &a.newTabMenuOpen, func(panel ui.Element) {
					a.newProjectMenuPanel(c, panel, pal)
				})
			}
		})

		if len(a.tabs) == 0 {
			fallback := &repoTab{path: a.repoPath, name: a.repoName, dirtyCount: len(a.workFiles)}
			a.projectGroup(c, pal, 0, fallback, true, &rowEls)
			return
		}
		ui.Column(c).Gap(projectsGap).FillWidth().Children(func() {
			for i, tab := range a.tabs {
				a.projectGroup(c, pal, i, tab, i == a.activeTab, &rowEls)
			}
		})
	}
	scroll := ui.Scroll(c).Key("projects-scroll").Height(h).Shrink(0).FillWidth().
		Padding(0, 10, 2, 10)
	scroll.Children(func() {
		ui.Column(c).Gap(projectsGap).FillWidth().Children(projects)
	})

	// 落点按整张列表算：指针在哪两行之间，就插在那儿。行与行之间那
	// 2px 的缝也算数，不用精确压到某一行的一半。
	if d, over := ui.DragOver[tabDrag](scroll); over && d.tab != nil {
		at := a.projectDropIndex(scroll, rowEls)
		if at >= 0 {
			drawProjectDropSlot(scroll, rowEls, at, pal)
		}
	}
	if d, ok := ui.Drop[tabDrag](scroll); ok && d.tab != nil {
		from := a.tabIndex(d.tab)
		at := a.projectDropIndex(scroll, rowEls)
		if from >= 0 && at >= 0 {
			a.moveTab(from, at)
		}
	}
}

// projectDropIndex 把指针在项目列表里的纵向位置折成插入下标：从 0 到
// len(rows)，等于「插到第几行前面」。
func (a *app) projectDropIndex(scroll ui.Element, rows []ui.Element) int {
	_, py, over := scroll.PointerPosition()
	if !over {
		return -1
	}
	sb := scroll.Bounds()
	for i, row := range rows {
		b := row.Bounds()
		if py < b.Y+b.H/2-sb.Y {
			return i
		}
	}
	return len(rows)
}

// drawProjectDropSlot 在落点处画一块浅色的「空位」。
//
// 不是一道 2px 的蓝线：一条线只说「这里有条缝」，而一块和行同高的
// 圆角浅色块说的是「这一行会挪到这里」，和 kero 的分栏落点预览同一个说法。
func drawProjectDropSlot(scroll ui.Element, rows []ui.Element, at int, pal palette) {
	if len(rows) == 0 {
		return
	}
	var y float32
	switch {
	case at <= 0:
		b := rows[0].Bounds()
		y = b.Y
	case at >= len(rows):
		b := rows[len(rows)-1].Bounds()
		y = b.Y + b.H
	default:
		y = rows[at].Bounds().Y
	}
	scroll.DrawOver(func(p *ui.Painter, r ui.Rect) {
		h := float32(28)
		// DrawOver 的画笔坐标和 Bounds 一样已经在窗口坐标系里，行边界
		// 可以直接用，不用再减 Scroll 的 y。
		slot := ui.Rect{X: r.X + 8, Y: y - h/2, W: r.W - 16, H: h}
		p.Fill(slot, pal.accent.Alpha(0.10), radiusSM)
		// 左端一颗短胶囊：落点在哪一侧一眼看得出来，又不用横贯整行。
		p.Fill(ui.Rect{X: slot.X + 4, Y: slot.Y + 6, W: 3, H: h - 12},
			pal.accent.Alpha(0.75), 1.5)
	})
}

// projectGroup 画一条项目。rowEls 收集这一帧的行，供整张列表算落点。
func (a *app) projectGroup(c *ui.Context, pal palette, idx int, tab *repoTab, active bool, rowEls *[]ui.Element) {
	if tab == nil {
		return
	}
	name := tab.name
	if name == "" {
		name = "Gloss"
	}

	row := ui.Row(c).Height(projectsRowH).FillWidth().
		Padding(0, 8).Gap(8).AlignItems(ui.Center).Radius(radiusMD).
		Cursor(ui.CursorPointer).Label(name).Tooltip(tab.path)
	if rowEls != nil {
		*rowEls = append(*rowEls, row)
	}
	switch {
	case active:
		row.Background(pal.rowSelected)
		if !pal.dark {
			selectedSurface(row, pal)
		}
	case row.Hovered():
		row.Background(pal.rowHover)
	}
	fadeColors(row)
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(T("打开项目", "Open Project")).Chosen() {
			a.chooseFolder()
		}
		m.Separator()
		if m.Item(T("关闭项目", "Close Project")).Chosen() {
			a.closeTab(idx)
		}
	})
	row.Children(func() {
		fg := pal.textMuted
		if active {
			fg = pal.ink
		}
		ui.Text(c, name).FontSize(12.5).FontWeight(weightSemibold).
			TextColor(fg).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).
			Tooltip(tab.path)

		ui.Spacer(c)
		if row.Hovered() && len(a.tabs) > 1 {
			close := ui.ButtonBase(c).Size(20, 20).Radius(radiusXS).Center().
				Cursor(ui.CursorPointer).Label(T("关闭项目", "Close Project")).
				Tooltip(Tf("关闭项目 %s", "Close project %s", name))
			if close.Hovered() {
				close.Background(pal.rowHover)
			}
			close.Children(func() {
				drawIcon(c, iconX, 11, pal.textMuted)
			})
			if close.Clicked() {
				a.closeTab(idx)
			}
		}
	})
	if row.Clicked() {
		a.switchTab(idx)
	}

	if idx >= 0 && idx < len(a.tabs) {
		row.Drag(tabDrag{from: idx, tab: tab})
		if row.Dragging() {
			// 被拖的那一行留在原位但压淡：看得见「从哪儿拿起」，
			// 又不会在原地留下一个洞。
			row.Cursor(ui.CursorGrabbing).Opacity(0.45)
		}
	}
}
