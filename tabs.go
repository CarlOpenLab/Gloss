package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

const (
	viewModeGit    = 0
	viewModeTerm   = 1
	viewModeEditor = 2
)

// repoTab 是一个打开的项目标签页，维护该项目的独立工作区与内置终端。
type repoTab struct {
	path       string
	name       string
	branch     string
	detached   bool
	dirtyCount int
	viewMode   int

	// 内置终端会话（基于 Ghostty libghostty-vt）。一个项目可以并排开好几个
	// shell，termActive 是当前展示的那一个；termSeq 只增不减，给新会话发芯片
	// 兜底标题的编号（见 termSession.ord）。
	terms      []*termSession
	termActive int
	termSeq    int

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
	listID     string
	// review 是改动区那一栏的状态（review.go）。它进快照：切回一个标签页
	// 时看的还是那一个文件，滚动位置、展开过的未改动区间都在原来的地方。
	review reviewState

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
	tab.listID = a.listID
	tab.review = a.review

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
	a.closeEmbeddedEditors()
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
	a.listID = tab.listID
	a.review = tab.review

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

	// 从别的标签页切回来时，编辑器要重新读一份（closeEmbeddedEditors 刚清过）。
	a.syncEditorPane()
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
		// 终端不在这里起：起 shell 要一份当前配色，而配色只有视图那层有。
		// 切到终端视图那一帧，terminalArea 自己会把缺的会话补上。
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
	releaseTermSessions(target)

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

// setViewMode 显式设置视图模式（0: Git GUI, 1: 终端, 2: 编辑器）。
func (a *app) setViewMode(mode int) {
	if a.viewMode == mode {
		return
	}
	a.viewMode = mode
	tab := a.currentTab()
	if tab != nil {
		tab.viewMode = mode
	}
	if mode == viewModeEditor {
		// 进编辑器视图时先把左边选中的那个文件读进来；之后换文件由
		// filepane 的点击/导航接着调 syncEditorPane。
		a.syncEditorPane()
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

// termSession 是一个终端会话 —— 芯片带上的一枚芯片对应一个 shell 进程。
//
// 进程之外还记着两样芯片要用的东西：
//
//	ord 是兜底标题的编号。shell 没报标题（OSC 0/2）时芯片写「终端 N」，
//	    这个号按创建顺序发，不按数组下标现算：关掉中间一个之后，剩下的
//	    会话不该集体改名，新开的也不该顶掉别人的号。
//	started 记着「已经试着起过了」。term 为 nil 且没试过才是「还没起」：
//	    失败过当然不自己重试（重试由界面上那一下决定）。
type termSession struct {
	term    *terminal.Terminal
	err     string
	ord     int
	started bool
}

// terminalNew 建一个终端。抽成变量是为了测试能换上替身：会话列表的增删
// 切换是纯逻辑，为它每次真起一个 shell，既慢，又要在退出时照顾一堆子进程。
var terminalNew = terminal.New

// activeSession 返回当前标签页里激活的会话（还没有就返回 nil）。
func (a *app) activeSession() *termSession {
	tab := a.currentTab()
	if tab == nil || tab.termActive < 0 || tab.termActive >= len(tab.terms) {
		return nil
	}
	return tab.terms[tab.termActive]
}

// newTerminalSession 在标签页里再挂一个终端会话，并切到它。
//
// 只登记、不起 shell：起一个 shell 要一份当前配色，而 palette 只有视图那
// 层拿得到，这条命令却可能从菜单、快捷键进来。下一帧 terminalArea 会把它
// 起起来 —— 那一帧就在眼前，用户看不到这中间的差别。
func (a *app) newTerminalSession(tab *repoTab) {
	if tab == nil {
		return
	}
	tab.termSeq++
	tab.terms = append(tab.terms, &termSession{ord: tab.termSeq})
	tab.termActive = len(tab.terms) - 1
}

// newTerminalTab 是「新建终端标签页」那条命令（⌘⇧T 与菜单里的那一项）。
func (a *app) newTerminalTab() {
	tab := a.currentTab()
	if tab == nil {
		return
	}
	// 顺手切进终端视图：新会话开在看不见的地方，用户按完只看到「什么也没
	// 发生」—— 而它其实已经起了个 shell。
	a.setViewMode(viewModeTerm)
	a.newTerminalSession(tab)
}

// closeTerminalSession 是芯片上那个 ×：关掉一个会话，并且保证这个标签页
// 关完还剩至少一个。
//
// 关掉最后一个就立刻补一个空的，而不是留一块「没有终端」的空状态再摆个
// 按钮：能点到这一步的人手上正在用终端，他要的是「这个 shell 不要了」，
// 不是「关掉终端」这个功能 —— 再让他点一下 + 只是白走一步。窗口关闭、
// 关闭项目标签页那些路径走的是 releaseTermSessions，只放进程，不补。
func (a *app) closeTerminalSession(tab *repoTab, idx int) {
	dropTerminalSession(tab, idx)
	if tab != nil && len(tab.terms) == 0 {
		a.newTerminalSession(tab)
	}
}

// dropTerminalSession 放掉一个会话的进程并从列表里摘掉它，不做任何补偿。
func dropTerminalSession(tab *repoTab, idx int) {
	if tab == nil || idx < 0 || idx >= len(tab.terms) {
		return
	}
	if s := tab.terms[idx]; s != nil && s.term != nil {
		_ = s.term.Close()
		s.term = nil
	}
	tab.terms = append(tab.terms[:idx], tab.terms[idx+1:]...)
	// 激活的挪到相邻的一个。关掉当前会话时保持原下标即可 —— 那正是它的
	// 后一个（关的是最后一个时落到前一个），视线不用跳回第一枚芯片。
	if idx < tab.termActive {
		tab.termActive--
	}
	if tab.termActive >= len(tab.terms) {
		tab.termActive = len(tab.terms) - 1
	}
	if tab.termActive < 0 {
		tab.termActive = 0
	}
}

// releaseTermSessions 放掉一个标签页里所有会话的 shell 进程。
//
// 关窗、退出、关闭项目标签页这三条路走这里：它们要的只是「别把子进程留在
// 系统里」，不需要任何界面补偿。
func releaseTermSessions(tab *repoTab) {
	if tab == nil {
		return
	}
	for _, s := range tab.terms {
		if s != nil && s.term != nil {
			_ = s.term.Close()
			s.term = nil
		}
	}
	tab.terms = nil
	tab.termActive = 0
}

// closeAllTerminals 关窗或退出时释放所有标签页的终端进程。
func (a *app) closeAllTerminals() {
	for _, t := range a.tabs {
		releaseTermSessions(t)
	}
}

// terminalTheme 给 Ghostty 终端一套和 Gloss 同源的配色。
//
// 背景取内容面 pal.codeBg（深色 #0f1011 = Surface 1，浅色下是白），不是画布
// 色 #010102：终端和它周围那一片代码区（diff 正文、编辑器）本来就是同一个
// 面，这里塌一档更黑，一进终端视图屏幕中间就出现一块不属于任何东西的洞。
// 前景同理取 pal.ink。
func terminalTheme(dark bool, pal palette) *terminal.Theme {
	th := terminal.LightTheme()
	if dark {
		th = terminal.DarkTheme()
	}
	th.Background = pal.codeBg
	th.Foreground = pal.ink
	return th
}

// syncTerminalThemes 把这一帧的明暗推给所有活着的终端。
//
// 终端不像界面那样每帧从调色板取色：Ghostty 的 Theme 是启动时给的一份快照，
// 换明暗（⌘主题 / 跟随系统）它不会自己跟上。所以在这儿比一眼，只有真的换了
// 一套颜色才推 —— 否则每帧一次 SetTheme，每个终端每帧都要重刷一遍调色板。
func (a *app) syncTerminalThemes(dark bool, pal palette) {
	if a.termThemePushed && a.termThemeDark == dark {
		return
	}
	a.termThemePushed = true
	a.termThemeDark = dark
	// 同一份 Theme 喂给 Theme 和 DarkTheme 两处：它已经是按当前明暗算好的
	// 成品，插件挑哪一份都是它。指针同时也是插件判断「要不要重新上色」的
	// 依据，所以宁可留着这一份，也别每帧新建。
	th := terminalTheme(dark, pal)
	for _, tab := range a.tabs {
		if tab == nil {
			continue
		}
		for _, s := range tab.terms {
			if s != nil && s.term != nil {
				s.term.SetTheme(th, th)
			}
		}
	}
}

// startTerminal 给一个会话起 shell。
//
// Dir 用项目路径：终端标签页是「在这个仓库里再开一个 shell」，新会话落在
// 同一个仓库，而不是回到 home 让用户自己 cd 一趟。
func (a *app) startTerminal(tab *repoTab, s *termSession, dark bool, pal palette) {
	if tab == nil || s == nil || s.started {
		return
	}
	s.started = true
	dir := tab.path
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	// shell 改了标题就得重画一次，否则芯片上那句话要等到下一件别的事触发
	// 重绘才换。Title() 本身是并发安全的读法，回调里只做「请重画」。
	redraw := func() {
		if a.dispatch != nil {
			a.dispatch(func() {})
		}
	}
	term, err := terminalNew(terminal.Options{
		Dir:     dir,
		Theme:   terminalTheme(dark, pal),
		OnTitle: func(string) { redraw() },
		OnExit:  func(int) { redraw() },
	})
	if err != nil {
		s.err = err.Error()
		return
	}
	s.term = term
}

// restartCurrentTerminal 重启当前会话的 shell，其余会话不动。
func (a *app) restartCurrentTerminal() {
	s := a.activeSession()
	if s == nil {
		return
	}
	if s.term != nil {
		_ = s.term.Close()
		s.term = nil
	}
	// 把「起过」那笔账和错误一起抹掉，下一帧的 terminalArea 会重新起一个
	// （配色在它手上）。重来的仍是同一个会话、同一枚芯片，编号也不换 ——
	// 看到的是这个 shell 换了，而不是「少了一个、多了一个」。
	s.err = ""
	s.started = false
}

// clearCurrentTerminal 清屏（只清当前会话）。
func (a *app) clearCurrentTerminal() {
	if s := a.activeSession(); s != nil && s.term != nil {
		s.term.Send([]byte("\x0c"))
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
		// 顶栏和侧栏同色同材质：一并从半透明改回实色，理由见 sidebar 里那段
		// —— 两个面的明度差本来就小，再各乘一个 α 就是两片糊在一起的白/黑。
		Background(pal.sideBg).
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

			// [+] 新建项目标签页：先给一份「打开过的项目」，末位才是打开本地文件夹。
			newBtn := ui.ButtonBase(c).Size(26, 26).Radius(radiusSM).
				Cursor(ui.CursorPointer).Label(T("新建项目标签页", "New Project Tab")).
				Tooltip(Tf("打开项目或本地 Git 仓库 %s / %s", "Open a project or local Git repository %s / %s", K("⌘T"), K("⌘O")))
			if newBtn.Hovered() || a.newTabMenuOpen {
				newBtn.Background(pal.surface2)
			}
			fadeColors(newBtn)
			newBtn.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
					drawIcon(c, iconPlus, 13, pal.inkSubtle)
				})
			})
			if newBtn.Clicked() {
				// 同一时刻只留一个顶栏下拉，否则两个浮层会叠在一起。
				a.repoDropdownOpen = false
				a.branchDropdownOpen = false
				a.newTabMenuOpen = !a.newTabMenuOpen
			}
			if a.newTabMenuOpen {
				ui.PopoverBase(c, newBtn, &a.newTabMenuOpen, func(panel ui.Element) {
					a.newProjectMenuPanel(c, panel, pal)
				})
			}

			ui.Spacer(c).DragWindow()

			// 右侧快捷工具
			if toolIconButton(c, pal, T("模型设置…", "Settings…"), Tf("偏好与设置 %s", "Preferences & Settings %s", K("⌘,")), false, false, drawSettingsIcon) {
				a.openSettings()
			}
		})
}

// newProjectMenuPanel 是顶栏 [+] 的下拉：先列「打开过的项目」，末位才是
// 「打开本地文件夹…」。
//
// 参照 Zed：在项目之间来回跳时，第一步应该是「从认识的名字里挑一个」，
// 而不是每次都被推去系统文件夹选择器、重走一遍路径。所以历史列表在前，
// 系统选择器退到末位。
//
// 高度是这块的主要约束。面板从窗口最顶端往下展开，而最近项目最多记 10 条：
// 窗口一矮，长历史就会把末尾的「打开本地文件夹…」顶到可视区外面 —— 它偏偏
// 是唯一没法靠滚动找到的项（它在滚动列表之外）。所以列表高度按当前窗口高度
// 现算，并给标题、分隔线、末项预留固定空间；行高也比工具栏那份压一档，
// 因为这里只需要认出项目名。
func (a *app) newProjectMenuPanel(c *ui.Context, panel ui.Element, pal palette) {
	panel.Radius(radiusLG).Background(pal.surface3).Border(1, pal.hairline).
		Shadow(0, 6, 16, 0, ui.RGBA(0, 0, 0, 0.18)).
		Margin(6, 0, 0, 0).Padding(6).MinWidth(300).MaxWidth(420).Gap(2)

	recent := make([]string, 0, len(a.store.State().RecentRepos))
	for _, p := range a.store.State().RecentRepos {
		if p != "" {
			recent = append(recent, p)
		}
	}

	// 标题
	ui.Row(c).Padding(6, 8, 4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		drawIcon(c, iconClock, 12, pal.inkSubtle)
		ui.Text(c, T("最近打开的项目", "Recent Projects")).FontSize(textCaption).
			FontWeight(weightSemibold).TextColor(pal.inkSubtle)
	})

	if len(recent) == 0 {
		ui.Box(c).Padding(4, 8, 8, 8).Children(func() {
			ui.Text(c, T("还没有打开过项目", "No projects opened yet")).
				FontSize(textBodySM).TextColor(pal.inkSubtle)
		})
	} else {
		_, winH := c.Size()
		// 先跟工具栏那份下拉一样封在 320（10 条历史在这个高度里放得下，
		// 不用滚），窗口不够高时再往下压：顶栏 + 面板留白 + 标题 + 分隔线
		// + 末项合计约 132。
		listMax := float32(320)
		if avail := winH - a.tabBarHeight(c) - 132; avail < listMax {
			listMax = avail
		}
		if listMax < 104 { // 至少露出三行
			listMax = 104
		}
		ui.Scroll(c).MaxHeight(listMax).FillWidth().Children(func() {
			for _, p := range recent {
				a.newProjectRecentRow(c, p, pal)
			}
		})
	}

	ui.Box(c).Height(1).FillWidth().Margin(4, 2).Background(pal.hairline)

	// 末位：打开本地文件夹
	addBtn := ui.ButtonBase(c).Radius(radiusSM).Padding(6, 8).
		Cursor(ui.CursorPointer).FillWidth().Label(T("打开本地文件夹…", "Open Local Folder…"))
	if addBtn.Hovered() {
		addBtn.Background(pal.surface4)
	}
	fadeColors(addBtn)
	addBtn.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
			drawIcon(c, iconFolderPlus, 13, pal.inkMuted)
			ui.Text(c, T("打开本地文件夹…", "Open Local Folder…")).
				FontSize(textButton).TextColor(pal.ink).Grow(1).SingleLine()
			specChip(c, pal, K("⌘O"), ui.Transparent, rowChipH, rowChipText)
		})
	})
	if addBtn.Clicked() {
		a.newTabMenuOpen = false
		a.chooseFolder()
	}
}

// newProjectRecentRow 画 [+] 下拉里的一行最近项目：选中即在新标签页打开
// （已经在标签页里的会切过去，而不是开出第二份）。
func (a *app) newProjectRecentRow(c *ui.Context, path string, pal palette) {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = path
	}
	opened := false
	for _, t := range a.tabs {
		if sameRepo(t.path, path) {
			opened = true
			break
		}
	}

	row := ui.ButtonBase(c).Radius(radiusSM).Padding(6, 8).
		Cursor(ui.CursorPointer).FillWidth().Label(path)
	if row.Hovered() {
		row.Background(pal.surface4)
	}
	fadeColors(row)
	row.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
			drawIcon(c, iconFolder, 13, pal.inkMuted)
			ui.Text(c, base).FontSize(textBodySM).TextColor(pal.ink).
				Grow(1).SingleLine()
			if opened {
				specChip(c, pal, T("已打开", "Open"), ui.Transparent, rowChipH, rowChipText)
			} else {
				ui.Text(c, filepath.Dir(path)).FontSize(textCaption).
					TextColor(pal.inkSubtle).MaxWidth(150).SingleLine()
			}
		})
	})
	if row.Clicked() {
		a.newTabMenuOpen = false
		a.openInNewTab(path)
	}
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

// termBarHeight 是终端会话芯片带的高度。
//
// 比顶栏的项目标签（38）压一档：这条带子长在主区里面，下面紧跟着就是终端
// 正文 —— 它每高一个像素，正文就少一个像素。28 高的芯片加两边各 3 的气口，
// 够放下这一行，也不再多了。
const termBarHeight = 34

// terminalTabBar 是终端视图顶上那条会话芯片带：一枚芯片一个 shell，点一下
// 切过去，右边那枚 + 再开一个。
//
// 底色取 pal.appBg（和它上面的工具栏同一块面），下面 1px 发丝线交代「正文
// 从这里开始」：正文那一块是内容面 codeBg，两者深浅只差一档，没有这条线
// 芯片带和终端就是糊在一起的一片。
func (a *app) terminalTabBar(c *ui.Context, pal palette, tab *repoTab) {
	ui.Row(c).Height(termBarHeight).FillWidth().Shrink(0).
		Padding(0, 8).Gap(4).AlignItems(ui.Center).
		Background(pal.appBg).
		// 只画下沿那一条：Border(0, …) 是零宽，画不出线，得用 BorderWidth
		// （连框架自己注释里推荐的都是这一种）。
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).
		Children(func() {
			// 关会话等这一圈走完再落（见 renderTerminalChip 的返回值）：× 是
			// 在画到一半时被点中的，就地删掉一个会让 append 把后面的会话整体
			// 前移一格 —— 剩下那几枚芯片这一帧就画成别人了。
			closeIdx := -1
			for i, s := range tab.terms {
				if a.renderTerminalChip(c, pal, tab, i, s) {
					closeIdx = i
				}
			}
			if closeIdx >= 0 {
				a.closeTerminalSession(tab, closeIdx)
			}
			if toolIconButton(c, pal, T("新建终端标签页", "New Terminal Tab"),
				Tf("在项目目录里再开一个 shell %s", "Open another shell in the project %s", K("⌘⇧T")),
				false, false, drawPlusIcon) {
				a.newTerminalSession(tab)
			}
		})
}

// renderTerminalChip 画一枚终端会话芯片，返回这一帧它的 × 有没有被点中
// （关的动作由调用方在这一圈走完之后再做，理由见 terminalTabBar）。
//
// 芯片上写 shell 自己报的标题（OSC 0/2），它不报就用「终端 N」兜底 —— 一屏
// 里两个 shell 都叫「终端」，认不出哪一个是刚在跑构建的那个。
//
// 激活态只换颜色（底 surface2、边 hairlineStrong、字 ink），几何一律不动：
// 边框两态都是 1px，内边距、字号、字重也都一样。切一次会话整条芯片带跟着
// 重排的话，第二下就点不准了。
func (a *app) renderTerminalChip(c *ui.Context, pal palette, tab *repoTab, idx int, s *termSession) bool {
	title := ""
	if s != nil && s.term != nil {
		title = strings.TrimSpace(s.term.Title())
	}
	if title == "" {
		ord := 1
		if s != nil {
			ord = s.ord
		}
		title = Tf("终端 %d", "Terminal %d", ord)
	}
	// 提示给 shell 的当前目录（OSC 7）：连着几个 shell 时，「哪一个是那个
	// 还没退出来的」靠名字看不出来，靠目录能。
	tip := title
	if s != nil && s.term != nil {
		if dir := s.term.Dir(); dir != "" {
			tip = dir
		}
	}

	active := idx == tab.termActive
	chip := ui.ButtonBase(c).Height(28).Shrink(0).Radius(radiusSM).
		Padding(0, 6).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(title).Tooltip(tip)
	bg, fg, bd := ui.Transparent, pal.inkSubtle, pal.hairline
	if active {
		bg, fg, bd = pal.surface2, pal.ink, pal.hairlineStrong
	} else if chip.Hovered() {
		// 悬停只提一档底（§5）：非激活芯片本来就是透明底，指上去得有点动静。
		bg = pal.surface1
	}
	chip.Background(bg).Border(1, bd)
	fadeColors(chip)

	// × 的点击只记下来、不在这里动手：调用方要等这一圈芯片都画完再关
	// （理由见 terminalTabBar）。
	closeClicked := false
	chip.Children(func() {
		drawIcon(c, iconTerminal, 11, fg)
		ui.Text(c, title).FontSize(textCaption).SingleLine().MaxWidth(140).TextColor(fg)

		// 关闭 ×。一直挂在芯片上、不跟着悬停出现：一颗只在悬停时才长出来的
		// ×，会让芯片宽度在指针扫过时变来变去，下一枚芯片跟着挪 —— 那正是
		// §5 说的「布局不许跟着状态抖」。
		closeBtn := ui.ButtonBase(c).Size(16, 16).Radius(radiusXS).
			Cursor(ui.CursorPointer).Label(T("关闭终端标签页", "Close Terminal Tab"))
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
			closeClicked = true
		}
	})

	if chip.Clicked() {
		tab.termActive = idx
	}
	return closeClicked
}

// terminalArea 呈现内置终端的主区全屏视图：顶上一条会话芯片，下面是当前会话。
func (a *app) terminalArea(c *ui.Context, pal palette) {
	tab := a.currentTab()
	if tab == nil {
		ui.Column(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
			ui.Text(c, T("请先打开一个项目", "Please open a project first")).TextColor(pal.inkSubtle)
		})
		return
	}

	// shell 是进了终端视图才起的：在 Git 面板里点来点去，不该在背后悄悄
	// 攒下一排 shell 进程。
	if len(tab.terms) == 0 {
		a.newTerminalSession(tab)
	}
	if tab.termActive < 0 || tab.termActive >= len(tab.terms) {
		tab.termActive = 0
	}
	cur := tab.terms[tab.termActive]
	// 明暗从这一帧的主题读（view 开头刚按 isDark 设过），和推给终端的
	// 那一份是同一个判断。
	dark := a.isDark(c)
	if !cur.started {
		a.startTerminal(tab, cur, dark, pal)
	}

	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.terminalTabBar(c, pal, tab)

		switch {
		case cur.err != "":
			ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Column(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, T("终端启动失败", "Failed to start terminal")).FontWeight(weightSemibold).TextColor(pal.ink)
					ui.Text(c, cur.err).FontSize(textCaption).TextColor(pal.inkSubtle)
					if secondaryAction(c, pal, T("重试", "Retry"), "", false).Clicked() {
						// 重来一次：把「起过」那笔账抹掉，下一帧再走一遍
						// startTerminal。
						cur.err = ""
						cur.started = false
					}
				})
			})
		case cur.term != nil:
			ui.Box(c).Grow(1).MinHeight(0).Background(pal.codeBg).Padding(4, 6).Children(func() {
				terminal.View(c, cur.term).Fill().AutoFocus()
			})
		}
	})
}
