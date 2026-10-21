package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"
)

// projectSidebar puts projects first, like a desktop session sidebar. The
// selected project expands into the workspace views, while branches remain
// the repository's secondary navigation below them.
func (a *app) projectSidebar(c *ui.Context, pal palette) {
	sw := a.sidebarWidth(c)
	bar := c.TitleBar()

	lead := float32(12)
	if bar.Left > 0 {
		lead = bar.Left + shellCardGap
	}

	ui.Column(c).Width(sw).Shrink(0).Children(func() {
		ui.Row(c).Height(shellTopBar).Shrink(0).FillWidth().
			AlignItems(ui.Center).Padding(0, 12, 0, lead).Gap(2).
			DragWindow().Children(func() {
			if isMacPlatform() {
				ui.Spacer(c)
			}
			shellIconButton(c, pal, T("隐藏侧栏", "Hide Sidebar"),
				Tf("隐藏项目侧栏 %s", "Hide project sidebar %s", K("⌘⌥B")),
				iconSidebar, false, func() { a.setSidebar(false) })

			plus := shellIconElement(c, pal, T("新建项目标签页", "New Project Tab"),
				Tf("打开项目或本地 Git 仓库 %s / %s", "Open a project or local Git repository %s / %s", K("⌘T"), K("⌘O")),
				iconPlus, false)
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
			if !isMacPlatform() {
				ui.Spacer(c)
			}
		})

		a.projectGroups(c, pal)

		shellDivider(c, pal)

		ui.Row(c).Height(28).Shrink(0).FillWidth().Padding(0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
			shellSectionLabel(c, pal, T("分支", "BRANCHES"))
			ui.Spacer(c)
			if a.repoReady() {
				ui.Text(c, strconv.Itoa(len(a.refs))).FontSize(11).
					TextColor(pal.textFaint).FontFeatures("tnum")
			}
		})

		if a.repoReady() {
			ui.Box(c).Key("ref-filter-wrap").Padding(0, 10, 6, 10).Shrink(0).Children(func() {
				a.refFilterInput(c, pal)
			})
		}
		ui.Box(c).Key("sidebar-refs-wrap").Grow(1).FillWidth().MinHeight(0).Children(func() {
			a.refsView(c, pal)
		})

		a.projectSidebarFooter(c, pal)

		grip := float32(8)
		resizer := ui.Box(c).Absolute().Top(0).Bottom(0).Right(-2).Width(grip).
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
				p.Fill(ui.Rect{X: r.X + r.W/2 - 0.5, Y: r.Y, W: 1, H: r.H}, pal.accent.Alpha(0.7), 0)
			}
		})
	})
}

func (a *app) projectGroups(c *ui.Context, pal palette) {
	n := len(a.tabs)
	if n == 0 {
		n = 1
	}

	h := float32(22 + 30 + (n-1)*46)
	if a.currentTab() != nil && a.currentTab().path != "" {
		h += 20 + 2*(shellRowH+1)
	}
	h = min(h, 300)

	projects := func() {
		ui.Row(c).Height(22).FillWidth().Padding(0, 2).AlignItems(ui.Center).Children(func() {
			shellSectionLabel(c, pal, T("项目", "PROJECTS"))
			ui.Spacer(c)
		})

		if len(a.tabs) == 0 {
			fallback := &repoTab{path: a.repoPath, name: a.repoName, dirtyCount: len(a.workFiles)}
			a.projectGroup(c, pal, 0, fallback, true)
			return
		}
		for i, tab := range a.tabs {
			a.projectGroup(c, pal, i, tab, i == a.activeTab)
		}
	}
	if h < 300 {
		ui.Column(c).Shrink(0).FillWidth().Padding(0, 10, 2, 10).Children(projects)
		return
	}
	ui.Scroll(c).Key("projects-scroll").Height(h).Shrink(0).FillWidth().
		Padding(0, 10, 2, 10).Children(func() {
		ui.Column(c).FillWidth().Children(projects)
	})
}

func (a *app) projectGroup(c *ui.Context, pal palette, idx int, tab *repoTab, active bool) {
	if tab == nil {
		return
	}
	name := tab.name
	if name == "" {
		name = "Gloss"
	}
	top := float32(shellSectionGap)
	if idx == 0 {
		top = 2
	}

	row := ui.Row(c).Height(28).FillWidth().Margin(top, 0, 2, 0).
		Padding(0, 6, 0, 10).Gap(6).AlignItems(ui.Center).Radius(radiusSM).
		Cursor(ui.CursorPointer).Label(name).Tooltip(tab.path)
	if row.Hovered() {
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
		ui.Text(c, name).FontSize(12.5).FontWeight(weightSemibold).
			TextColor(pal.textMuted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).
			Tooltip(tab.path)

		dirty := tab.dirtyCount
		if active {
			dirty = len(a.workFiles)
		}
		if dirty > 0 {
			ui.Text(c, strconv.Itoa(dirty)).FontSize(12).FontFeatures("tnum").
				TextColor(pal.textFaint)
		}
		ui.Spacer(c)
		if active {
			drawIcon(c, iconChevronDown, 11, pal.textFaint)
		} else {
			drawIcon(c, iconChevronRight, 11, pal.textFaint.Alpha(0.65))
		}
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
		row.Drag(tabDrag{from: idx})
		after := dragAfterY(row)
		if _, over := ui.DragOver[tabDrag](row); over {
			drawRowDropMarker(row, after, pal.accent)
		}
		if d, ok := ui.Drop[tabDrag](row); ok {
			at := idx
			if after {
				at = idx + 1
			}
			if d.from != idx && d.from != at {
				a.moveTab(d.from, at)
			}
		}
	}

	if !active || tab.path == "" {
		return
	}
	if tab.branch != "" {
		ui.Row(c).Height(20).FillWidth().Padding(0, 10, 0, 25).Gap(5).
			AlignItems(ui.Center).Children(func() {
			drawIcon(c, iconGitBranch, 11, pal.textFaint)
			ui.Text(c, tab.branch).FontSize(11.5).TextColor(pal.textFaint).
				SingleLine().Ellipsis("…")
		})
	}
	a.workspaceRow(c, pal, sectionChanges, T("改动", "Changes"), iconFileDiff, len(a.workFiles))
	a.workspaceRow(c, pal, sectionHistory, T("历史", "History"), iconGitCommit, 0)
}

func (a *app) workspaceRow(c *ui.Context, pal palette, section int, label string, icon *ui.SVG, badge int) {
	selected := a.section() == section && a.repoReady()
	accessible := Tf("项目视图：%s", "Project view: %s", label)
	row := ui.Row(c).Height(shellRowH).FillWidth().Padding(0, 10, 0, 12).
		Gap(10).AlignItems(ui.Center).Radius(radiusMD).Margin(0, 0, 1, 0).
		Cursor(ui.CursorPointer).Label(accessible)
	if pal.dark {
		row.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	fg := pal.textMuted
	switch {
	case selected:
		bg = pal.rowSelected
		fg = pal.text
		selectedSurface(row, pal)
	case row.Hovered():
		bg = pal.rowHover
		fg = pal.text
	}
	row.Background(bg)
	fadeColors(row)
	row.Children(func() {
		drawIcon(c, icon, 15, fg)
		txt := ui.Text(c, label).Label(accessible).FontSize(13.5).SingleLine().Grow(1).
			MinWidth(0).TextColor(fg)
		if selected {
			txt.FontWeight(weightMedium)
		}
		if badge > 0 {
			ui.Text(c, strconv.Itoa(badge)).FontSize(12).FontFeatures("tnum").
				TextColor(pal.textFaint)
		}
	})
	if row.Clicked() && a.repoReady() {
		a.setSection(section)
	}
}

func (a *app) projectSidebarFooter(c *ui.Context, pal palette) {
	ui.Row(c).Height(38).Shrink(0).FillWidth().Padding(0, 12, 4, 12).
		Gap(2).AlignItems(ui.Center).DragWindow().Children(func() {
		shellIconButton(c, pal, T("打开项目", "Open Project"),
			Tf("打开本地 Git 仓库 %s", "Open a local Git repository %s", K("⌘O")),
			iconFolder, false, a.chooseFolder)
		shellIconButton(c, pal, T("命令面板", "Command Palette"),
			Tf("搜索命令与项目 %s", "Search commands and projects %s", K("⌘K")),
			iconSearch, false, a.openPalette)
		shellIconButton(c, pal, T("模型设置…", "Model Settings…"),
			Tf("偏好与设置 %s", "Preferences %s", K("⌘,")),
			iconSettings, false, a.openSettings)
		ui.Spacer(c)
	})
}
