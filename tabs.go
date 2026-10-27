package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

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

// section 是工具栏那段分段控件上的四选一：改动 / 历史 / 终端 / 编辑器。
//
// 它不取代 viewMode 和 tab，只是把这两个字段「折」成一个下标 —— 分段控件
// 要的就是一个下标，而底下这两个字段各自还记着「切回 Git 面板时该看哪一
// 页」，合并掉就丢了那个信息。顺序就是工具栏上从左到右的顺序。
const (
	sectionChanges = iota
	sectionHistory
	sectionTerminal
	sectionEditor
)

// section 返回当前看的是哪一块（改动 / 历史 / 终端 / 编辑器）。
func (a *app) section() int {
	switch a.viewMode {
	case viewModeTerm:
		return sectionTerminal
	case viewModeEditor:
		return sectionEditor
	}
	if a.tab == 1 {
		return sectionHistory
	}
	return sectionChanges
}

// setSection 切到指定的那一块。两条路都从入口上把住：仓库没开成时
// setTab 自己不切，这里也就不动（见 setTab 上那段）。
func (a *app) setSection(i int) {
	switch i {
	case sectionChanges, sectionHistory:
		a.setViewMode(viewModeGit)
		a.setTab(i)
	case sectionTerminal:
		a.setViewMode(viewModeTerm)
	case sectionEditor:
		a.setViewMode(viewModeEditor)
	}
}

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

	// 「编辑器」视图那一栏列的是整个工作区，与上面那份改动列表并行存在。
	wsAll        []gitrepo.File
	wsFiles      []gitrepo.File
	wsTree       []*filetree.Node
	wsList       ui.ListState
	wsFilter     string
	wsTreeFilter string
	wsSel        int
	wsOpen       map[string]bool

	tab               int // 0 = 改动, 1 = 历史
	allCommits        []gitrepo.Commit
	commits           []gitrepo.Commit
	graph             []graphRow
	maxLanes          int
	commitList        ui.ListState
	commitSel         int
	histAll           bool
	histRev           string
	commitFilter      string
	commitFilterBuilt string
	hist              loadState
	histDiff          loadState
	histErr           string
	histDiffErr       string

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

	tab.wsAll = a.wsAll
	tab.wsFiles = a.wsFiles
	tab.wsTree = a.wsTree
	tab.wsList = a.wsList
	tab.wsFilter = a.wsFilter
	tab.wsTreeFilter = a.wsTreeFilter
	tab.wsSel = a.wsSel
	tab.wsOpen = a.wsOpen

	tab.tab = a.tab
	tab.allCommits = a.allCommits
	tab.commits = a.commits
	tab.graph = a.graph
	tab.maxLanes = a.maxLanes
	tab.commitList = a.commitList
	tab.commitSel = a.commitSel
	tab.histAll = a.histAll
	tab.histRev = a.histRev
	tab.commitFilter = a.commitFilter
	tab.commitFilterBuilt = a.commitFilterBuilt
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

	a.wsAll = tab.wsAll
	a.wsFiles = tab.wsFiles
	a.wsTree = tab.wsTree
	a.wsList = tab.wsList
	a.wsFilter = tab.wsFilter
	a.wsTreeFilter = tab.wsTreeFilter
	a.wsSel = tab.wsSel
	a.wsOpen = tab.wsOpen

	a.tab = tab.tab
	a.allCommits = tab.allCommits
	a.commits = tab.commits
	a.graph = tab.graph
	a.maxLanes = tab.maxLanes
	a.commitList = tab.commitList
	a.commitSel = tab.commitSel
	a.histAll = tab.histAll
	a.histRev = tab.histRev
	a.commitFilter = tab.commitFilter
	a.commitFilterBuilt = tab.commitFilterBuilt
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
		// 进了终端视图，shell 也只因「新建终端」而起（见 terminalArea）。
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

// reorder 把 s 里第 from 项挪到「第 to 项之前」（to == len(s) 就是挪到最末），
// 返回新的顺序；原地不动的情形原样返回。
//
// 项目标签页和终端芯片两处拖动共用这一套算法：拖动的落点都是「插到某一项
// 前面」，而摘掉一项之后，落点在它后面的下标都要减一 —— 这一步算错了，
// 表现是「往右拖一格没动、拖两格跑过头」，所以两处不各写一份。
func reorder[T any](s []T, from, to int) []T {
	if from < 0 || from >= len(s) || to < 0 || to > len(s) || to == from || to == from+1 {
		return s
	}
	moved := s[from]
	out := append(s[:from], s[from+1:]...)
	if to > from {
		to--
	}
	// rest 先拷一份：out[:to] 的 append 会写回同一块底层数组，直接从
	// out[to:] 上取会把自己搬没。
	rest := append([]T{}, out[to:]...)
	return append(out[:to], append([]T{moved}, rest...)...)
}

// moveTab 把一个项目标签页挪到别的位置。活动的是哪一页不受影响 —— 挪的是
// 顺序，不是选择（活动页按指针认回来，而不是按老下标现算）。
//
// 终端会话挂在各自的 repoTab 上，所以标签页一挪，它带着自己的 shell 一起走。
func (a *app) moveTab(from, to int) {
	if a.reorderTab(from, to) {
		// 打开的项目是按顺序记的（见 persistOpenRepos），所以下一次启动看到的
		// 就是拖完的样子。
		a.persistOpenRepos()
	}
}

// reorderTab 只改内存里的顺序，不落盘：拖动的每一帧都在调它，每帧写一次
// 配置文件没有必要，落盘留给松手那一下（moveTab / 拖动结束）。
func (a *app) reorderTab(from, to int) bool {
	if from < 0 || from >= len(a.tabs) {
		return false
	}
	if to < 0 || to > len(a.tabs) || to == from || to == from+1 {
		return false
	}
	active := a.currentTab()
	a.tabs = reorder(a.tabs, from, to)
	if active != nil {
		for i, t := range a.tabs {
			if t == active {
				a.activeTab = i
				break
			}
		}
	}
	return true
}

// tabIndex 找某一页现在的下标，找不到返回 -1。
func (a *app) tabIndex(t *repoTab) int {
	for i, x := range a.tabs {
		if x == t {
			return i
		}
	}
	return -1
}

// removeRecentRepo 把一条项目从「最近打开的项目」里摘掉。
//
// 只摘这一条记录：它要是正开着，标签页照旧开着 —— 顶栏 [+] 那个下拉里每
// 一行右边的 × 就是这个意思（见 newProjectRecentRow）。
func (a *app) removeRecentRepo(path string) {
	if a.store == nil {
		return
	}
	a.store.RemoveRecentRepo(path)
	a.persist()
}

// closeTab 关闭指定标签页。
//
// 这一页里还有没保存的编辑改动时先问一句：那些缓冲区跟着标签页一起没，
// 而「关掉」和「误触」长得一模一样（顶栏的 ×、侧栏的「关闭项目」、⌘W 三条
// 路都到这儿）。确认之后由 onYes 里显式丢掉缓冲再关，所以不会绕回本条
// 问第二次。
func (a *app) closeTab(idx int) {
	if idx < 0 || idx >= len(a.tabs) {
		return
	}
	if a.tabHasUnsavedEdits(idx) {
		a.askConfirm(confirmState{
			title: T("关闭这个项目标签页？", "Close this project tab?"),
			body: T("这一页里有还没保存的编辑器改动，关掉就丢了。保存（⌘S）或者先取消。",
				"This tab has unsaved editor changes; closing it discards them. Save (⌘S) or cancel."),
			yes: T("仍然关闭", "Close anyway"),
			onYes: func() {
				// 用户选了丢：把这一页的缓冲区清掉再关，否则 closeTab 又会
				// 走到这条确认上，问第二次。活动页的活状态在 app 上，非活动页
				// 只有 tab.review 那份快照（codeEdit / editorPane 不进快照，
				// 切页时就被收掉了，非活动页没有这两样）。
				a.closeEmbeddedEditors()
				if idx < len(a.tabs) && idx != a.activeTab {
					a.tabs[idx].review = reviewState{}
				}
				a.closeTab(idx)
			},
		})
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

// closeActive 是「关闭」那条命令（⌘W 和菜单里的「关闭标签页」）：关的是
// 眼前那一页，不是这一页下挂的所有东西。
//
// 终端视图里眼前那一页是一枚会话芯片 —— 终端会话全挂在同一个 repoTab 下
// （tab.terms），在终端视图里关标签页会把这一页所有 shell 一起放掉。所以
// 终端视图认会话：有关着的会话就关当前激活的那个（和芯片上的 × 一个脾气，
// 关光落空状态页）；一个会话都没有时才落回去关项目标签页 —— 那已经没什么
// 终端可关了。
func (a *app) closeActive() {
	if a.viewMode == viewModeTerm {
		tab := a.currentTab()
		if tab != nil && len(tab.terms) > 0 {
			idx := tab.termActive
			if idx < 0 || idx >= len(tab.terms) {
				idx = len(tab.terms) - 1
			}
			dropTerminalSession(tab, idx)
			a.requestRedraw()
			return
		}
	}
	a.closeTab(a.activeTab)
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
		// filepane / workspace 的点击与导航接着调 syncEditorPane。
		//
		// 编辑器栏列的是整个工作区，与改动栏不是同一份列表：刚从改动栏切过来
		// 时把那边的选中顺势带进来 —— 在改动列表里点开一个文件、再切过来细看，
		// 想看的就是它。
		if a.wsSel < 0 && a.fileSel >= 0 && a.fileSel < len(a.files) {
			a.selectWorkspaceKey(pathOf(a.files[a.fileSel]))
		}
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

// chipDrag 是一次终端芯片的拖动（值本身只用得上「拖的是哪一枚」，落点由
// 指针戳在哪一枚上算）。
type chipDrag struct {
	from int
}

// chipDragState 是一次拖动在窗口里的状态：拖的是哪个标签页的哪一枚，它从
// 原来的第几位来、现在预览在第几位。芯片带只画预览，松手才落到 tab.terms。
type chipDragState struct {
	tab  *repoTab
	term *termSession
	from int
	at   int
	// active 说这一帧还有芯片在报拖动。没被 drop 接管就不再报，预览收回。
	active bool
}

// chipSlide 是芯片被挤开时滑过去的那一下：短弹簧、不回弹。回弹的弹簧会让
// 一条正在排序的芯片带看起来在互相弹；取消回弹之后，第二次换位仍会从芯片
// 当前的位置接着走，不会重新起跳。
var chipSlide = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond}}

// chipKey 给芯片一个跨帧稳定的身份：拖到别的下标之后，它的状态（以及拖动
// 中的副本）还认得是同一枚。ord 在一个标签页里唯一，指针用来区分标签页。
type chipKey struct {
	tab *repoTab
	ord int
}

// chipPreview 是拖动期间芯片带的绘制顺序：被拖的那枚已经放在它将要落下的
// 位置，其余芯片保持原顺序。tab.terms 本身不动，松手才改。
func chipPreview(tab *repoTab, d *chipDragState) []*termSession {
	if d == nil || d.tab != tab || !d.active {
		return tab.terms
	}
	from := termIndex(tab, d.term)
	if from < 0 {
		return tab.terms
	}
	rest := make([]*termSession, 0, len(tab.terms)-1)
	rest = append(rest, tab.terms[:from]...)
	rest = append(rest, tab.terms[from+1:]...)
	at := min(max(d.at, 0), len(rest))
	out := make([]*termSession, 0, len(tab.terms))
	out = append(out, rest[:at]...)
	out = append(out, tab.terms[from])
	out = append(out, rest[at:]...)
	return out
}

// termIndex 是会话在标签页里的下标；关掉和挪动都按会话本身认回来，不按
// 预览顺序里那个会变的下标。
func termIndex(tab *repoTab, s *termSession) int {
	if tab == nil || s == nil {
		return -1
	}
	for i, o := range tab.terms {
		if o == s {
			return i
		}
	}
	return -1
}

// chipDropIndex 把指针在芯片带里的横向位置折成插入下标：第一枚中线右边的
// 那枚之前。被拖的那枚不参与 —— 它自己就是那个空位，问它会答自己。
func chipDropIndex(pointerX float32, terms []*termSession, xs, widths []float32, drag *termSession) int {
	at := 0
	for i, s := range terms {
		if s == drag || widths[i] <= 0 {
			continue
		}
		if pointerX < chipBarPad+xs[i]+widths[i]/2 {
			return at
		}
		at++
	}
	return at
}

// placeChip 把一枚芯片放在它这一帧该在的位置，顺序变了就从上一帧的位置滑
// 过去。被拖的那枚不滑：它自己不可见，指针上那份副本才是动的那个。
func (a *app) placeChip(chip ui.Element, s *termSession, x float32, animate bool) {
	if a.chipX == nil {
		a.chipX = make(map[*termSession]float32)
		a.chipSettle = make(map[*termSession]int)
	}
	prev, seen := a.chipX[s]
	a.chipX[s] = x
	if !animate {
		a.chipSettle[s] = 0
		return
	}
	if seen {
		if delta := prev - x; delta > 0.5 || delta < -0.5 {
			chip.AnimatePose(ui.Pose{X: delta}, chipSlide)
			a.chipSettle[s] = 24
			return
		}
	}
	if frames := a.chipSettle[s]; frames > 0 {
		chip.AnimatePose(ui.Pose{}, chipSlide)
		a.chipSettle[s] = frames - 1
	}
}

// requestRedraw 请框架再画一帧。拖动静止在松手那一帧时，状态是在这一帧
// 里清掉的：不再要一帧，芯片就会停在预览里（被拖的那枚还是透明的）。
func (a *app) requestRedraw() {
	if a.dispatch != nil {
		a.dispatch(func() {})
	}
}

// 芯片带的内边距与芯片间距，和 terminalTabBar 里画的一致：落点、槽位和
// 指针换算都按这几个数走。
const (
	chipBarPad = 8.0
	chipBarGap = 4.0
	chipHeight = 28.0
)

// moveTerminalSession 挪一枚终端芯片的位置。当前展示的那一枚跟着它自己
// 走 —— 挪的是顺序，不是「现在看哪个 shell」（按指针认回来，而不是按老
// 下标现算）。
//
// 收成包级函数而不是 app 上的方法：它只碰 tab 自己的会话列表，不碰别的
// 状态。
func moveTerminalSession(tab *repoTab, from, to int) {
	if tab == nil || from < 0 || from >= len(tab.terms) {
		return
	}
	var active *termSession
	if tab.termActive >= 0 && tab.termActive < len(tab.terms) {
		active = tab.terms[tab.termActive]
	}
	tab.terms = reorder(tab.terms, from, to)
	if active == nil {
		return
	}
	for i, s := range tab.terms {
		if s == active {
			tab.termActive = i
			return
		}
	}
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

// dropTerminalSession 放掉一个会话的进程并从列表里摘掉它，不做任何补偿。
//
// 芯片上的 × 走这里：关掉最后一个就落到底下的空状态页（terminalEmptyState），
// 不再立刻补一个新的 shell —— 会点到 × 的人要的是「这个 shell 不要了」，
// 补一个只是替他做决定。窗口关闭、关闭项目标签页那些路径走的是
// releaseTermSessions，只放进程，不补偿。
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

// termFontKey 是「上一次推给终端的那份字体」。
//
// terminal.Font 里有一个切片字段（连字特性 Features），两份值不能直接比，
// 所以账本上只记我们真正会改的这两项 —— 也正是「要不要重推」的判据。
type termFontKey struct {
	family string
	size   float32
}

// 内置终端的字号范围。小于 9 等宽字就开始糊笔画，大于 28 一屏剩不下几行 ——
// 终端是拿来看输出的，不是放大镜。
const (
	termFontMin  = 9
	termFontMax  = 28
	termFontStep = 1
	// termFontDefault 是插件在字号为 0 时用的那一档（见 mygo 的
	// plugins/terminal/font.go）：界面上要把这个数写给用户看，所以得跟它一致。
	termFontDefault = 13
	// termFontGenericMono 是「系统等宽」那个通用关键字：mygo 三边都认得它
	// （macOS 走 CoreText、Windows 映射到 Cascadia Mono / Consolas / Courier New、
	// Linux 交给 pango），拿到的是系统自己的等宽字体。
	termFontGenericMono = "monospace"
)

// curatedTermFonts 是下拉里那份固定的常见等宽字体表。
//
// 它管的是「没扫到也算数」的那一半：系统自带的 SF Mono / Menlo / Monaco /
// Consolas 不进用户字体目录，装了但不在用户目录下的字体也可能扫漏。排在扫出来
// 的已装字体后面 —— 你手上真有的（尤其是 Nerd Font）先出现。
var curatedTermFonts = []string{
	"SF Mono",
	"Menlo",
	"Monaco",
	"JetBrains Mono",
	"Fira Code",
	"Cascadia Mono",
	"Consolas",
	"Hack",
	"IBM Plex Mono",
	"Source Code Pro",
	"Roboto Mono",
	"Ubuntu Mono",
	"Noto Sans Mono",
	"DejaVu Sans Mono",
	"Liberation Mono",
	"Courier New",
	"Inconsolata",
	"Anonymous Pro",
	"MesloLGS NF",
	"Victor Mono",
}

// termFontOptions 是字体下拉里的选项，顺序就是「排序优先」的答案：
//
//	系统等宽 → 装着的 Nerd Font（按名字） → 装着的其他等宽字体 → 固定表
//
// 装了 Nerd Font 的人多半就是要在终端里用它的图标字形（powerline、devicons
// 那一套），所以它们排在最前面；挑了个这台机器没有的也不怕 —— 用的时候会补上
// monospace 兜底（见 fontFamilyWithFallback）。
func (a *app) termFontOptions() []string {
	out := make([]string, 0, len(a.fonts)+len(curatedTermFonts)+1)
	seen := map[string]bool{}
	add := func(family string) {
		family = strings.TrimSpace(family)
		if family == "" || seen[family] {
			return
		}
		seen[family] = true
		out = append(out, family)
	}
	add(termFontGenericMono)
	for _, f := range a.fonts {
		if f.Nerd {
			add(f.Family)
		}
	}
	for _, f := range a.fonts {
		if !f.Nerd {
			add(f.Family)
		}
	}
	for _, family := range curatedTermFonts {
		add(family)
	}
	return out
}

// ensureFontScan 起一次后台字体扫描，一个进程只起一次。
//
// 放在设置页第一次画到这一行的时候叫，而不是启动时：不开设置的人不该为它付
// 一次磁盘扫描（实测用户字体目录一两百毫秒）。扫完在主线程上换上新的一份，
// 下一帧下拉里就有它们了。
func (a *app) ensureFontScan() {
	if a.fontScanStarted {
		return
	}
	a.fontScanStarted = true
	dirs := userFontDirs()
	a.run(func() {
		fonts := scanFonts(dirs)
		a.dispatch(func() {
			a.fonts = fonts
			a.fontScanDone = true
		})
	})
}

// fontFamilyWithFallback 在字体族后面补一个 monospace 兜底。
//
// 这一个尾巴是量出来的，不是想当然：本机 1000px 宽的终端里，family 为空、
// "monospace"、"Menlo" 都是 87 列，而一个不存在的字体族 **没有兜底时只有
// 53 列** —— 引擎替你换了一支比例字体，字号没变、格子宽度变了，终端就错行。
// 所以下拉里那些「这台机器可能没装」的字族，靠它退到系统等宽。
func fontFamilyWithFallback(family string) string {
	f := strings.TrimSpace(family)
	if f == "" {
		return f // 空交给插件自己：它的默认就是系统等宽
	}
	for _, part := range strings.Split(f, ",") {
		if strings.EqualFold(strings.TrimSpace(part), termFontGenericMono) {
			return f // 已经带着兜底了
		}
	}
	return f + ", " + termFontGenericMono
}

// terminalFont 是内置终端当前该用的字体。
//
// 只交 Family 和 Size：字重、连字（calt / liga）、行高、加粗描边那几档留在
// 插件的默认值上 —— 每一档摆出来都得配一个界面和一条存储字段，而它们对
// 「终端读起来顺不顺」的影响远不如字号和字体族这两个。
func (a *app) terminalFont() terminal.Font {
	return terminal.Font{Family: fontFamilyWithFallback(a.termFontFamily), Size: a.termFontSize}
}

// termFontSizeValue 是当前生效的终端字号。0（没设过）交给插件默认，
// 界面上显示的那一档得是这个数，不是 0。
func (a *app) termFontSizeValue() float32 {
	if a.termFontSize <= 0 {
		return termFontDefault
	}
	return a.termFontSize
}

// syncTerminalFont 把字体推给所有活着的终端。
//
// 和配色一样：终端手里那份是起 shell 时交出去的快照，设置里改了要推给它才
// 看得见。SetFont 是运行时的，不用重启 shell —— 重启会把屏幕上的输出和正在
// 跑的命令一起丢掉，为了换个字号不值当。新会话在 startTerminal 里直接带上
// 当前字体，不走这条。
func (a *app) syncTerminalFont() {
	f := a.terminalFont()
	key := termFontKey{family: f.Family, size: f.Size}
	if a.termFontPushed && a.termFontSent == key {
		return
	}
	a.termFontPushed = true
	a.termFontSent = key
	for _, tab := range a.tabs {
		if tab == nil {
			continue
		}
		for _, s := range tab.terms {
			// 起失败的会话 term 是 nil（测试里的替身也是），跳过。
			if s != nil && s.term != nil {
				s.term.SetFont(f)
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
	// term 由下面的 OnPaste 引用：粘贴发生时 shell 早就起好了，指针已经落到
	// s.term 上；起失败时它是 nil，回调里直接让过去。
	var term *terminal.Terminal
	term, err := terminalNew(terminal.Options{
		Dir:     dir,
		Font:    a.terminalFont(),
		Theme:   terminalTheme(dark, pal),
		OnTitle: func(string) { redraw() },
		OnExit:  func(int) { redraw() },
		OnPaste: func(text string) bool {
			// 剪贴板上没有文本（一张图片）时，把 Ctrl+V 补发给前台程序。
			//
			// Claude Code 这类 AI CLI 的「粘贴图片」是自己读系统剪贴板的，
			// 等的就是这个按键；而插件的粘贴只把文本送进去，图片这一下被
			// 它整键吃掉 —— Ghostty 里能贴进 AI 的图，在这里石沉大海。
			// Ghostty 用户装 ghostty-paste 补的也正是这一下：吞掉 Cmd+V、
			// 改发 Ctrl+V。
			//
			// 有文本时让回去（false），插件照旧走 Bracketed Paste。
			if text != "" {
				return false
			}
			if term != nil {
				term.Send([]byte("\x16")) // Ctrl+V
			}
			return true
		},
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

// newProjectMenuPanel 是项目侧栏 [+] 的下拉：先列「打开过的项目」，末位才是
// 「打开本地文件夹…」。
//
// 参照 Zed：在项目之间来回跳时，第一步应该是「从认识的名字里挑一个」，
// 而不是每次都被推去系统文件夹选择器、重走一遍路径。所以历史列表在前，
// 系统选择器退到末位。
//
// 高度是这块的主要约束。面板从侧栏顶部往下展开，而最近项目最多记 10 条：
// 窗口一矮，长历史就会把末尾的「打开本地文件夹…」顶到可视区外面 —— 它偏偏
// 是唯一没法靠滚动找到的项（它在滚动列表之外）。所以列表高度按当前窗口高度
// 现算，并给标题、分隔线、末项预留固定空间；行高也比工具栏那份压一档，
// 因为这里只需要认出项目名。
func (a *app) newProjectMenuPanel(c *ui.Context, panel ui.Element, pal palette) {
	popoverSurface(panel, pal).Radius(radiusLG).
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

// newProjectRecentRow 画 [+] 下拉里的一行最近项目：选中即在新项目行打开
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

			if a.recentRemoveButton(c, pal, path, base) {
				a.removeRecentRepo(path)
			}
		})
	})
	if row.Clicked() {
		a.newTabMenuOpen = false
		a.openInNewTab(path)
	}
}

// recentRemoveButton 画最近项目行尾那枚「从最近列表移除」的 ×，返回这一帧
// 它有没有被点中。两处最近项目列表（侧栏 [+] 的下拉、工具栏的仓库下拉）共用
// 这一枚：同一份记录，一处能删、另一处删不掉，就成了两处不一致。
//
// × 一直挂着，不跟着悬停才出现：一颗只在悬停时长出来的 × 会让行宽（跟着
// 后面那一列）在指针扫过时变来变去，而且只看得见正指着的那一行 —— 想清理
// 几条时得先摸过去才知道哪里有得点。
func (a *app) recentRemoveButton(c *ui.Context, pal palette, path, base string) bool {
	del := ui.ButtonBase(c).Size(16, 16).Radius(radiusXS).Shrink(0).
		Cursor(ui.CursorPointer).
		Label(Tf("从最近列表移除 %s", "Remove %s from recent", base)).
		Tooltip(Tf("从最近打开的项目里移除 %s（不影响已打开的标签页）",
			"Remove %s from recent projects (open tabs stay open)", base))
	if del.Hovered() {
		del.Background(pal.surface3)
	}
	fadeColors(del)
	del.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
			drawIcon(c, iconX, 10, pal.inkSubtle)
		})
	})
	return del.Clicked()
}

// tabDrag 是一次项目标签页的拖动（值本身只用得上「从哪一页来」，落点由
// 指针戳在哪一页上算）。
type tabDrag struct {
	from int
	// tab 是拖着走的那一页本身，不只是它当初的下标：拖动过程中列表会
	// 一边拖一边重排，下标每帧都在变，只有按对象认才追得住。
	tab *repoTab
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
	// 被拖的那枚芯片留出的空位：这一帧在预览顺序里的 x 和宽。画在带子
	// 自己的 Draw 里（先于芯片），被拖的芯片本身是透明的，只剩这个槽。
	var slotX, slotW float32
	bar := ui.Row(c).Height(termBarHeight).FillWidth().Shrink(0).
		Padding(0, chipBarPad).Gap(chipBarGap).AlignItems(ui.Center).
		Background(pal.appBg).
		// 只画下沿那一条：Border(0, …) 是零宽，画不出线，得用 BorderWidth
		// （连框架自己注释里推荐的都是这一种）。
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline)
	bar.Draw(func(p *ui.Painter, r ui.Rect) {
		if slotW <= 0 {
			return
		}
		p.Fill(ui.Rect{
			X: r.X + chipBarPad + slotX,
			Y: r.Y + (termBarHeight-chipHeight)/2,
			W: slotW,
			H: chipHeight,
		}, pal.surface1, radiusSM)
	})
	bar.Children(func() {
		// 拖动期间画的是预览顺序：被拖的芯片已经挪到它将要落下的位置，
		// tab.terms 一根手指都不动，松手才提交。
		terms := chipPreview(tab, a.chipDrag)
		var dragTerm *termSession
		if d := a.chipDrag; d != nil && d.tab == tab && d.active {
			dragTerm = d.term
		}
		widths := make([]float32, len(terms))
		xs := make([]float32, len(terms))
		known := true
		x := float32(0)
		for i, s := range terms {
			widths[i] = a.chipWidth[s]
			if widths[i] <= 0 {
				// 标签页刚打开的头两帧还没量到宽度：这一帧不摆位、不判落点，
				// 否则所有芯片会从 x=0 滑到自己的位置，看起来像开场乱弹。
				known = false
			}
			xs[i] = x
			x += widths[i] + chipBarGap
		}
		// 关会话等这一圈走完再落（见 renderTerminalChip 的返回值）：× 是
		// 在画到一半时被点中的，就地删掉一个会让 append 把后面的会话整体
		// 前移一格 —— 剩下那几枚芯片这一帧就画成别人了。拖动同理。
		var closeSession *termSession
		var dragging, dropped bool
		for i, s := range terms {
			closeClicked, rowDragging, rowDropped := a.renderTerminalChip(c, pal, tab, i, s, xs[i], s == dragTerm, known)
			if closeClicked {
				closeSession = s
			}
			if rowDragging {
				dragging = true
				d := a.chipDrag
				if d == nil || d.tab != tab || d.term != s {
					// 刚起拖：从列表里的原位开始预览，第一帧不动任何芯片。
					if from := termIndex(tab, s); from >= 0 {
						d = &chipDragState{tab: tab, term: s, from: from, at: from}
						a.chipDrag = d
					}
				}
				if d != nil {
					d.active = true
				}
			}
			if rowDropped {
				dropped = true
			}
			if s == dragTerm {
				slotX, slotW = xs[i], widths[i]
			}
		}
		if closeSession != nil {
			if idx := termIndex(tab, closeSession); idx >= 0 {
				dropTerminalSession(tab, idx)
			}
		} else if d := a.chipDrag; d != nil && d.tab == tab {
			// 落点按这一帧的预览和指针算；宽度还没量到（标签页刚打开的第一
			// 帧）就先不动，否则会把芯片甩到末尾。
			if known && (dragging || dropped) {
				px, _, _ := bar.PointerPosition()
				d.at = chipDropIndex(px, terms, xs, widths, d.term)
			}
			if dropped {
				from, to := d.from, d.at
				a.chipDrag = nil
				if to > from {
					to++
				}
				moveTerminalSession(tab, from, to)
				a.requestRedraw()
			} else if !dragging {
				// 松在芯片带以外：预览退回原来的顺序。
				a.chipDrag = nil
				a.requestRedraw()
			}
		}
		if toolIconButton(c, pal, T("新建终端标签页", "New Terminal Tab"),
			Tf("在项目目录里再开一个 shell %s", "Open another shell in the project %s", K("⌘⇧T")),
			false, false, drawPlusIcon) {
			a.newTerminalSession(tab)
		}
	})
}

// renderTerminalChip 画一枚终端会话芯片，返回这一帧它的 × 有没有被点中、
// 它是不是正在被拖、以及它有没有接到一次 drop。三件事都由调用方在这一圈
// 芯片都画完之后再做，理由见 terminalTabBar。
//
// 芯片上写 shell 自己报的标题（OSC 0/2），它不报就用「终端 N」兜底 —— 一屏
// 里两个 shell 都叫「终端」，认不出哪一个是刚在跑构建的那个。
//
// 激活态只换颜色（底 surface2、边 hairlineStrong、字 ink），几何一律不动：
// 边框两态都是 1px，内边距、字号、字重也都一样。切一次会话整条芯片带跟着
// 重排的话，第二下就点不准了。
func (a *app) renderTerminalChip(c *ui.Context, pal palette, tab *repoTab, idx int, s *termSession, x float32, dragged, known bool) (closeClicked, dragging, dropped bool) {
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

	// 按会话本身认「当前在看哪一枚」：拖动期间下标是预览顺序里的，拿它去
	// 比 tab.termActive 会把激活态画到别人身上。
	active := s != nil && tab.termActive >= 0 && tab.termActive < len(tab.terms) && tab.terms[tab.termActive] == s
	key := chipKey{tab: tab, ord: idx}
	if s != nil {
		key.ord = s.ord
	}
	chip := ui.ButtonBase(c.Key(key)).Height(chipHeight).Shrink(0).Radius(radiusSM).
		Padding(0, 6).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(title).Tooltip(tip)
	bg, fg, bd := ui.Transparent, pal.inkSubtle, pal.hairline
	switch {
	case dragged:
		// 原位的芯片不可见，只留空位；框架画在指针上的那份副本才是「拿
		// 起来」的那一枚，所以底色和阴影给的是它。
		bg, fg, bd = pal.surface3, pal.ink, pal.hairlineStrong
	case active:
		bg, fg, bd = pal.surface2, pal.ink, pal.hairlineStrong
	case chip.Hovered() && a.chipDrag == nil:
		// 悬停只提一档底（§5）：非激活芯片本来就是透明底，指上去得有点动静。
		// 拖动期间别的芯片从指针下经过，不许闪。
		bg = pal.surface1
	}
	chip.Background(bg).Border(1, bd)
	if dragged {
		chip.Opacity(0).Shadow(0, 6, 18, -6, ui.RGBA(0, 0, 0, 0.24))
	}
	fadeColors(chip)

	// 拖一下换位置。
	chip.Drag(chipDrag{from: idx})
	if chip.Dragging() {
		chip.Cursor(ui.CursorGrabbing)
		dragging = true
	}
	if _, ok := ui.Drop[chipDrag](chip); ok {
		dropped = true
	}

	// × 的点击只记下来、不在这里动手：调用方要等这一圈芯片都画完再关
	// （理由见 terminalTabBar）。
	chip.Children(func() {
		drawIcon(c, iconTerminal, 11, fg)
		ui.Text(c, title).FontSize(textCaption).SingleLine().MaxWidth(140).TextColor(fg)

		// 关闭 ×。一直挂在芯片上、不跟着悬停出现：一颗只在悬停时才长出来的
		// ×，会让芯片宽度在指针扫过时变来变去，下一枚芯片跟着挪 —— 那正是
		// §5 说的「布局不许跟着状态抖」。
		closeBtn := ui.ButtonBase(c).Size(16, 16).Radius(radiusXS).
			Cursor(ui.CursorPointer).Label(T("关闭终端标签页", "Close Terminal Tab"))
		if closeBtn.Hovered() && a.chipDrag == nil {
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
		// 点击按会话认回真实下标：预览里的 idx 只在拖动时有意义。
		if i := termIndex(tab, s); i >= 0 {
			tab.termActive = i
		}
	}
	if w := chip.Bounds().W; w > 0 {
		if a.chipWidth == nil {
			a.chipWidth = make(map[*termSession]float32)
		}
		a.chipWidth[s] = w
	}
	// 只在拖动期间让芯片滑动：关掉或新开一枚是几何变化，不该让旁边的芯片
	// 一边滑一边等人点它的 ×。
	if known {
		a.placeChip(chip, s, x, !dragged && a.chipDrag != nil)
	}
	return closeClicked, dragging, dropped
}

// terminalArea 呈现内置终端的主区全屏视图：顶上一条会话芯片，下面是当前会话。
//
// 一个会话都不剩时（从没开过、或最后一个被 × 关掉）显示空状态页
// terminalEmptyState —— 终端视图不擅自起 shell：会话只因为用户点过
// 「新建终端」（空页上的按钮、芯片带上的 +、⌘⇧T）而存在。
func (a *app) terminalArea(c *ui.Context, pal palette) {
	tab := a.currentTab()
	if tab == nil {
		ui.Column(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
			ui.Text(c, T("请先打开一个项目", "Please open a project first")).TextColor(pal.inkSubtle)
		})
		return
	}

	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.terminalTabBar(c, pal, tab)

		if len(tab.terms) == 0 {
			a.terminalEmptyState(c, pal, tab)
			return
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

// terminalEmptyState 是一个终端会话都不剩时的那一页：正中间一句话、一颗
// 「新建终端」。
//
// 关掉最后一个会话就落到这里，而不是像从前那样立刻补一个新的 shell：会点
// 到 × 的人要的就是「这个 shell 不要了」，补一个只是替他做决定。从没开过
// 会话的终端视图也是这一页 —— 空状态是唯一显式说「这里可以没有终端」的地方。
// 芯片带上的 + 和 ⌘⇧T 指向同一个动作（newTerminalSession）。
func (a *app) terminalEmptyState(c *ui.Context, pal palette, tab *repoTab) {
	t := c.Theme()
	ui.Box(c).Grow(1).FillWidth().Background(pal.codeBg).Children(func() {
		ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
			drawIcon(c, iconTerminal, 28, pal.inkSubtle)
			headline(c, pal, T("没有终端会话", "No Terminal Sessions"))
			ui.Text(c, T("在这个仓库里开一个 shell：跑构建、看日志、敲 git，或者跑 AI CLI。",
				"Open a shell in this repository for builds, logs, git, or an AI CLI.")).
				TextColor(t.TextMuted)
			if primaryAction(c, pal, T("新建终端", "New Terminal"),
				Tf("在项目目录里开一个 shell %s", "Open a shell in the project %s", K("⌘⇧T")), false).Clicked() {
				a.newTerminalSession(tab)
			}
			// 空页上顺便把快捷键说一遍：下次就不用找按钮。
			ui.Text(c, Tf("快捷键 %s", "Press %s to open", K("⌘⇧T"))).FontSize(textCaption).TextColor(t.TextMuted)
		})
	})
}
