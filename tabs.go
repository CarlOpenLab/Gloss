package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

const (
	viewModeGit  = 0
	viewModeTerm = 1
)

// repoTab 是一个打开的项目标签页，维护该项目的独立工作区与内置终端。
type repoTab struct {
	path       string
	name       string
	branch     string
	detached   bool
	dirtyCount int
	viewMode   int

	// 内置终端会话（基于 Ghostty libghostty-vt）
	term    *terminal.Terminal
	termErr string

	// Git 状态快照
	remotes    []string
	ahead      int
	behind     int
	upstream   string
	pending    gitrepo.Pending
	state      loadState
	errMsg     string
	errNotRepo bool

	allFiles   []gitrepo.File
	files      []gitrepo.File
	workFiles  []gitrepo.File
	histFiles  []gitrepo.File
	histListID string
	contents   map[string]*fileContent
	contentFP  map[string]string

	rows      []row
	rowsDirty bool
	diffList  ui.ListState
	built     int

	expanded map[string]int
	listID   string
	hunkCur  string

	tree             []*filetree.Node
	filesList        ui.ListState
	filePaneWidthVal float32
	fileFilter       string
	fileTreeFilter   string
	filesClosed      map[string]bool
	fileSel          int

	tab         int // 0 = 改动, 1 = 历史
	commits     []gitrepo.Commit
	graph       []graphRow
	maxLanes    int
	commitList  ui.ListState
	commitSel   int
	histRev     string
	hist        loadState
	histDiff    loadState
	histErr     string
	histDiffErr string

	refs       []gitrepo.Ref
	refByFull  map[string]*gitrepo.Ref
	refTree    []*filetree.Node
	refsList   ui.ListState
	refsState  loadState
	refErr     string
	refFilter  string
	refsFilter string
	refSel     string
	refsClosed map[string]bool

	commitMsg string
	draftFor  string
}

// currentTab 返回当前激活的标签页。
func (a *app) currentTab() *repoTab {
	if a.activeTab >= 0 && a.activeTab < len(a.tabs) {
		return a.tabs[a.activeTab]
	}
	return nil
}

// saveActiveTab 将主应用的仓库及视图字段同步备份进当前激活的标签页。
func (a *app) saveActiveTab() {
	if a.activeTab < 0 || a.activeTab >= len(a.tabs) {
		return
	}
	tab := a.tabs[a.activeTab]
	tab.path = a.repoPath
	tab.name = a.repoName
	tab.branch = a.branch
	tab.detached = a.detached
	tab.dirtyCount = len(a.workFiles)
	tab.viewMode = a.viewMode

	tab.remotes = a.remotes
	tab.ahead = a.ahead
	tab.behind = a.behind
	tab.upstream = a.upstream
	tab.pending = a.pending
	tab.state = a.state
	tab.errMsg = a.errMsg
	tab.errNotRepo = a.errNotRepo

	tab.allFiles = a.allFiles
	tab.files = a.files
	tab.workFiles = a.workFiles
	tab.histFiles = a.histFiles
	tab.histListID = a.histListID
	tab.contents = a.contents
	tab.contentFP = a.contentFP

	tab.rows = a.rows
	tab.rowsDirty = a.rowsDirty
	tab.diffList = a.diffList
	tab.built = a.built

	tab.expanded = a.expanded
	tab.listID = a.listID
	tab.hunkCur = a.hunkCur

	tab.tree = a.tree
	tab.filesList = a.filesList
	tab.filePaneWidthVal = a.filePaneWidthVal
	tab.fileFilter = a.fileFilter
	tab.fileTreeFilter = a.fileTreeFilter
	tab.filesClosed = a.filesClosed
	tab.fileSel = a.fileSel

	tab.tab = a.tab
	tab.commits = a.commits
	tab.graph = a.graph
	tab.maxLanes = a.maxLanes
	tab.commitList = a.commitList
	tab.commitSel = a.commitSel
	tab.histRev = a.histRev
	tab.hist = a.hist
	tab.histDiff = a.histDiff
	tab.histErr = a.histErr
	tab.histDiffErr = a.histDiffErr

	tab.refs = a.refs
	tab.refByFull = a.refByFull
	tab.refTree = a.refTree
	tab.refsList = a.refsList
	tab.refsState = a.refsState
	tab.refErr = a.refErr
	tab.refFilter = a.refFilter
	tab.refsFilter = a.refsFilter
	tab.refSel = a.refSel
	tab.refsClosed = a.refsClosed

	tab.commitMsg = a.commitMsg
	tab.draftFor = a.draftFor
}

// restoreActiveTab 将当前激活标签页的快照恢复到主应用的仓库及视图字段中。
func (a *app) restoreActiveTab() {
	if a.activeTab < 0 || a.activeTab >= len(a.tabs) {
		return
	}
	tab := a.tabs[a.activeTab]
	a.repoPath = tab.path
	a.repoName = tab.name
	a.branch = tab.branch
	a.detached = tab.detached
	a.viewMode = tab.viewMode

	a.remotes = tab.remotes
	a.ahead = tab.ahead
	a.behind = tab.behind
	a.upstream = tab.upstream
	a.pending = tab.pending
	a.state = tab.state
	a.errMsg = tab.errMsg
	a.errNotRepo = tab.errNotRepo

	a.allFiles = tab.allFiles
	a.files = tab.files
	a.workFiles = tab.workFiles
	a.histFiles = tab.histFiles
	a.histListID = tab.histListID
	a.contents = tab.contents
	a.contentFP = tab.contentFP

	a.rows = tab.rows
	a.rowsDirty = tab.rowsDirty
	a.diffList = tab.diffList
	a.built = tab.built

	a.expanded = tab.expanded
	a.listID = tab.listID
	a.hunkCur = tab.hunkCur

	a.tree = tab.tree
	a.filesList = tab.filesList
	a.filePaneWidthVal = tab.filePaneWidthVal
	a.fileFilter = tab.fileFilter
	a.fileTreeFilter = tab.fileTreeFilter
	a.filesClosed = tab.filesClosed
	a.fileSel = tab.fileSel

	a.tab = tab.tab
	a.commits = tab.commits
	a.graph = tab.graph
	a.maxLanes = tab.maxLanes
	a.commitList = tab.commitList
	a.commitSel = tab.commitSel
	a.histRev = tab.histRev
	a.hist = tab.hist
	a.histDiff = tab.histDiff
	a.histErr = tab.histErr
	a.histDiffErr = tab.histDiffErr

	a.refs = tab.refs
	a.refByFull = tab.refByFull
	a.refTree = tab.refTree
	a.refsList = tab.refsList
	a.refsState = tab.refsState
	a.refErr = tab.refErr
	a.refFilter = tab.refFilter
	a.refsFilter = tab.refsFilter
	a.refSel = tab.refSel
	a.refsClosed = tab.refsClosed

	a.commitMsg = tab.commitMsg
	a.draftFor = tab.draftFor
}

// switchTab 切换到指定下标的标签页。
func (a *app) switchTab(idx int) {
	if idx < 0 || idx >= len(a.tabs) || idx == a.activeTab {
		return
	}
	a.saveActiveTab()
	a.activeTab = idx
	a.restoreActiveTab()
	// 切了仓库，自动刷新（autorefresh.go）重新盯新 tab 的仓库。
	a.syncAutoWatch()

	tab := a.currentTab()
	if tab != nil {
		if tab.state == loadIdle && tab.path != "" {
			a.openRepo(tab.path)
		}
		if a.viewMode == viewModeTerm && tab.term == nil {
			a.initTabTerminal(tab)
		}
	}
	a.persistOpenRepos()
}

// openInNewTab 在新标签页中打开指定仓库，如果已存在则切换过去。
func (a *app) openInNewTab(path string) {
	cleaned := filepath.Clean(path)
	for i, t := range a.tabs {
		if sameRepo(t.path, cleaned) {
			a.switchTab(i)
			return
		}
	}

	if len(a.tabs) == 1 && a.tabs[0].path == "" {
		a.tabs[0].path = cleaned
		a.tabs[0].name = filepath.Base(cleaned)
		a.openRepo(cleaned)
		a.persistOpenRepos()
		return
	}

	a.saveActiveTab()
	name := filepath.Base(cleaned)
	if name == "" || name == "." || name == "/" {
		name = "Gloss"
	}
	tab := &repoTab{
		path:     cleaned,
		name:     name,
		viewMode: a.viewMode,
		state:    loadIdle,
	}
	a.tabs = append(a.tabs, tab)
	a.activeTab = len(a.tabs) - 1
	a.restoreActiveTab()
	a.openRepo(cleaned)
	a.persistOpenRepos()
}

// closeTab 关闭指定标签页。
func (a *app) closeTab(idx int) {
	if idx < 0 || idx >= len(a.tabs) {
		return
	}
	target := a.tabs[idx]
	if target.term != nil {
		_ = target.term.Close()
		target.term = nil
	}

	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
	if len(a.tabs) == 0 {
		a.tabs = []*repoTab{{name: "Gloss"}}
		a.activeTab = 0
		a.repoPath = ""
		a.repoName = "Gloss"
		a.branch = ""
		a.workFiles = nil
		a.files = nil
		a.state = loadIdle
	} else {
		if a.activeTab >= len(a.tabs) {
			a.activeTab = len(a.tabs) - 1
		} else if idx < a.activeTab {
			a.activeTab--
		}
		a.restoreActiveTab()
	}
	// 关掉/切换后活动仓库换了，自动刷新跟着换盯的对象。
	a.syncAutoWatch()
	a.persistOpenRepos()
}

// nextTab 循环切换到下一个项目标签页。
func (a *app) nextTab() {
	if len(a.tabs) <= 1 {
		return
	}
	next := (a.activeTab + 1) % len(a.tabs)
	a.switchTab(next)
}

// prevTab 循环切换到上一个项目标签页。
func (a *app) prevTab() {
	if len(a.tabs) <= 1 {
		return
	}
	prev := (a.activeTab - 1 + len(a.tabs)) % len(a.tabs)
	a.switchTab(prev)
}

// toggleViewMode 在 Git GUI 面板和内置终端之间秒切。
func (a *app) toggleViewMode() {
	if a.viewMode == viewModeGit {
		a.setViewMode(viewModeTerm)
	} else {
		a.setViewMode(viewModeGit)
	}
}

// setViewMode 显式设置视图模式（0: Git GUI, 1: 终端）。
func (a *app) setViewMode(mode int) {
	if a.viewMode == mode {
		return
	}
	a.viewMode = mode
	tab := a.currentTab()
	if tab != nil {
		tab.viewMode = mode
		if mode == viewModeTerm && tab.term == nil {
			a.initTabTerminal(tab)
		}
	}
}

// persistOpenRepos 将当前打开的项目路径列表记录到 store 中。
func (a *app) persistOpenRepos() {
	if a.store == nil {
		return
	}
	var paths []string
	for _, t := range a.tabs {
		if t.path != "" {
			paths = append(paths, t.path)
		}
	}
	a.store.SetOpenRepos(paths)
	a.persist()
}

// closeAllTerminals 关窗或退出时释放所有标签页的终端进程。
func (a *app) closeAllTerminals() {
	for _, t := range a.tabs {
		if t != nil && t.term != nil {
			_ = t.term.Close()
			t.term = nil
		}
	}
}

// terminalTheme 为 Ghostty 终端提供与 Gloss 契合的调色板。
func terminalTheme(dark bool) *terminal.Theme {
	if dark {
		th := terminal.DarkTheme()
		th.Background = ui.Hex("#010102") // Gloss 标志性 Canvas 底色
		th.Foreground = ui.Hex("#f7f8f8") // Ink 字体
		return th
	}
	th := terminal.LightTheme()
	th.Background = ui.Hex("#ffffff")
	th.Foreground = ui.Hex("#0f1115")
	return th
}

// initTabTerminal 启动当前标签页专属的 Ghostty 原生终端。
func (a *app) initTabTerminal(tab *repoTab) {
	if tab == nil || tab.term != nil {
		return
	}
	dir := tab.path
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	dark := true
	if a.themeMode == "light" {
		dark = false
	}
	th := terminalTheme(dark)

	term, err := terminal.New(terminal.Options{
		Dir:   dir,
		Theme: th,
		OnTitle: func(title string) {
			if a.dispatch != nil {
				a.dispatch(func() {})
			}
		},
		OnExit: func(code int) {
			if a.dispatch != nil {
				a.dispatch(func() {})
			}
		},
	})
	if err != nil {
		tab.termErr = err.Error()
		return
	}
	tab.term = term
	tab.termErr = ""
}

// restartCurrentTerminal 重启当前激活标签页的终端。
func (a *app) restartCurrentTerminal() {
	tab := a.currentTab()
	if tab == nil {
		return
	}
	if tab.term != nil {
		_ = tab.term.Close()
		tab.term = nil
	}
	tab.termErr = ""
	a.initTabTerminal(tab)
}

// clearCurrentTerminal 清屏。
func (a *app) clearCurrentTerminal() {
	tab := a.currentTab()
	if tab != nil && tab.term != nil {
		tab.term.Send([]byte("\x0c"))
	}
}

// tabBarHeight 计算顶栏高度。
func (a *app) tabBarHeight(c *ui.Context) float32 {
	return max(c.TitleBar().Height, 38)
}

// titleBarInset 是顶栏和第一个标签之间要留出的距离。
//
// bar.Left 量到的是 macOS 红绿灯里绿灯按钮的右沿。左内边距原先写死 80：本机上
// 绿灯右沿已经在 78 上下，两边只剩两三个像素 —— 第一个标签的圆角底色贴着窗口
// 按钮，鼠标一扫过去两个圆角凑在一起，像连成了一片。留到 18 这样一块完整的
// 气口，两块才是两块。Linux / Windows 上 bar.Left 是 0（它们的窗口按钮在
// 右上），走的是顶栏自己的 12。
const titleBarInset = 18

// projectTabBar 呈现 Zed 风格的窗口顶栏：给窗口控制按钮留出安全边距、多项目
// 标签页、新建项目 [+]、窗口拖拽区。
func (a *app) projectTabBar(c *ui.Context, pal palette) {
	bar := c.TitleBar()
	t := c.Theme()

	// 窗口控制按钮让出来的两边内边距。bar 的 Left/Right 量的是它们各自的
	// 右沿/左沿：macOS 是左上角的红绿灯，Linux 和 Windows 是右上角那一组
	// （框架给的，见 ui.Context.TitleBar）。两者都归零时（全屏下控制按钮收
	// 起来）顶栏自己留 12。
	padLeft := float32(12)
	if bar.Left > 0 {
		padLeft = bar.Left + titleBarInset
	}
	padRight := float32(12)
	if bar.Right > 0 {
		padRight = bar.Right + 12
	}

	h := a.tabBarHeight(c)

	ui.Row(c).Height(h).FillWidth().Shrink(0).
		Padding(3, padRight, 3, padLeft).
		Gap(4).
		AlignItems(ui.Center).
		Background(pal.sideBg.Alpha(0.6)).
		Border(0, pal.hairline).
		DragWindow().
		Children(func() {
			if len(a.tabs) == 0 {
				// 一个标签页都还没建起来（直接 load 一个仓库就是这种状态），
				// 顶栏也该是「当前打开的项目」，而不是写死的 Gloss —— 仓库
				// 开着、顶栏却报另一个名字，是这一栏里最刺眼的一种错。
				a.renderProjectTab(c, 0, &repoTab{
					path:       a.repoPath,
					name:       a.repoName,
					dirtyCount: len(a.workFiles),
				}, pal, t)
			} else {
				for i, tab := range a.tabs {
					a.renderProjectTab(c, i, tab, pal, t)
				}
			}

			// [+] 新建项目标签页
			newBtn := ui.ButtonBase(c).Size(26, 26).Radius(radiusSM).
				Cursor(ui.CursorPointer).Label(T("新建项目标签页", "New Project Tab")).
				Tooltip(Tf("打开本地 Git 仓库作为新标签页 %s / %s", "Open Git repository in new tab %s / %s", K("⌘T"), K("⌘O")))
			if newBtn.Hovered() {
				newBtn.Background(pal.surface2)
			}
			fadeColors(newBtn)
			newBtn.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
					drawIcon(c, iconPlus, 13, pal.inkSubtle)
				})
			})
			if newBtn.Clicked() {
				a.chooseFolder()
			}

			ui.Spacer(c).DragWindow()

			// 右侧快捷工具
			if toolIconButton(c, pal, T("模型设置…", "Settings…"), Tf("偏好与设置 %s", "Preferences & Settings %s", K("⌘,")), false, false, drawSettingsIcon) {
				a.openSettings()
			}
		})
}

// renderProjectTab 呈现单个项目标签页。
func (a *app) renderProjectTab(c *ui.Context, idx int, tab *repoTab, pal palette, t *ui.Theme) {
	isActive := idx == a.activeTab
	name := tab.name
	if name == "" {
		if tab.path != "" {
			name = filepath.Base(tab.path)
		} else {
			name = "Gloss"
		}
	}

	tabBtn := ui.ButtonBase(c).Height(28).Radius(radiusSM).
		Padding(0, 6, 0, 8).
		Cursor(ui.CursorPointer).Label(name).
		Tooltip(tab.path)

	if isActive {
		tabBtn.Background(pal.surface2).Border(1, pal.hairlineStrong)
	} else if tabBtn.Hovered() {
		tabBtn.Background(pal.surface1).Border(1, ui.Transparent)
	} else {
		tabBtn.Background(ui.Transparent).Border(1, ui.Transparent)
	}
	fadeColors(tabBtn)

	tabBtn.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			// 图标
			drawIcon(c, iconFolder, 12, pal.inkSubtle)

			// 仓库名。标签只回答「这是哪个项目」—— 一个标签里名字后面再
			// 跟一段被截掉一半的分支名，多项目并排时反而谁都不好认；分支
			// 归下面工具栏的分支徽章（侧栏开着时是侧栏的分支树）。
			//
			// 激活态只换颜色，不动几何：边框三态一律 1px（只在透明与
			// hairlineStrong 之间换色），字重也保持 regular —— semibold 的
			// 字宽比 regular 宽一个多百分点，切一次标签这个框就胖一点，
			// 右边所有标签连同 [+] 跟着一起挪，点一下、晃一下。
			txt := ui.Text(c, name).FontSize(textCaption).SingleLine().MaxWidth(180)
			if isActive {
				txt.TextColor(pal.ink)
			} else {
				txt.TextColor(pal.inkMuted)
			}

			// 待提交改动标记
			if tab.dirtyCount > 0 {
				ui.Box(c).Size(6, 6).Radius(radiusPill).Background(pal.addBar)
			}

			// 关闭按钮 ×
			closeBtn := ui.ButtonBase(c).Size(16, 16).Radius(radiusXS).
				Cursor(ui.CursorPointer).Label(T("关闭标签页", "Close Tab")).
				Tooltip(Tf("关闭标签页 %s", "Close Tab %s", K("⌘W")))
			if closeBtn.Hovered() {
				closeBtn.Background(pal.surface3)
			}
			fadeColors(closeBtn)
			closeBtn.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
					drawIcon(c, iconX, 10, pal.inkSubtle)
				})
			})
			if closeBtn.Clicked() {
				a.closeTab(idx)
			}
		})
	})

	if tabBtn.Clicked() {
		a.switchTab(idx)
	}
}

// terminalArea 呈现内置终端的主区全屏视图。
func (a *app) terminalArea(c *ui.Context, pal palette) {
	tab := a.currentTab()
	if tab == nil {
		ui.Column(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
			ui.Text(c, T("请先打开一个项目", "Please open a project first")).TextColor(pal.inkSubtle)
		})
		return
	}

	if tab.term == nil && tab.termErr == "" {
		a.initTabTerminal(tab)
	}

	if tab.termErr != "" {
		ui.Column(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
			ui.Column(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, T("终端启动失败", "Failed to start terminal")).FontWeight(weightSemibold).TextColor(pal.ink)
				ui.Text(c, tab.termErr).FontSize(textCaption).TextColor(pal.inkSubtle)
				if secondaryAction(c, pal, T("重试", "Retry"), "", false).Clicked() {
					tab.termErr = ""
					a.initTabTerminal(tab)
				}
			})
		})
		return
	}

	if tab.term != nil {
		ui.Box(c).Fill().Background(pal.codeBg).Padding(4, 6).Children(func() {
			terminal.View(c, tab.term).Fill().AutoFocus()
		})
	}
}
