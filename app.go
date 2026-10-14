package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/store"
)

// 线程约定 —— desktop 包里所有代码都要守这几条：
//
//  1. 只有主线程（view 函数内部，以及经 dispatch 提交的闭包）能读写 app 的字段。
//  2. 后台 goroutine 只接收值参数，绝不接收 *app；结果一律通过
//     a.dispatch(func(){...}) 交回主线程。Go 的 race detector 只能覆盖测试
//     跑到的路径，这里靠约定而不是靠工具。
//  3. ui.ListState / ui.ScrollState 这类组件状态创建一次就不再重建 ——
//     它们承载滚动位置、展开状态和选中项，重建等于把这些状态丢掉。
//  4. 元素构建过程中不要改 app 的持久状态。Draw 回调尤其不行：一帧内它
//     可能被调用多次，而且读到的是本帧最后一次构建的状态。
//
// dispatch 这个函数字段是测试的接缝：生产环境接 win.Update（跳主线程 +
// 重绘），测试里默认同步执行，于是异步流程可以无头驱动。

type loadState uint8

const (
	loadIdle loadState = iota
	loadLoading
	loadReady
	loadFailed
)

type app struct {
	win      *mygo.Window
	dispatch func(func())
	// bg 登记所有后台任务，测试靠它等异步流程跑完（生产环境不等 ——
	// 结果的落地走 dispatch，该等的是回调而不是 goroutine 结束）。
	bg sync.WaitGroup
	// loop 只在测试里用：dispatch 改成把闭包排进队列，由 settle 执行。
	// 生产环境它是零值，没人碰。
	loop mainLoop

	// —— 自动刷新（autorefresh.go）——
	// watchRoot 是当前要盯着自动刷新的仓库根（"" = 不盯）；watchVer 每次
	// 模型重读自增，轮询 goroutine 靠它重新对齐基线。两个都是 atomic，
	// 因为轮询线程要读它们，而写只发生在主线程。
	// autoStop 关掉轮询；autoReloadedAt 防自动刷新风暴（主线程专用）。
	watchRoot      atomic.Value // string
	watchVer       atomic.Int32
	autoStop       chan struct{}
	autoReloadedAt time.Time

	// —— 仓库 ——
	// 打开失败时这两个也记着东西：试的是哪个文件夹（标题栏要摆出来，卡片
	// 上的「这个文件夹」才有指代）。所以「repoPath 非空」不等于「仓库开着」，
	// 那件事只由 state 说。
	repoPath string
	repoName string
	branch   string
	// detached 是 HEAD 没附在任何分支上（检出过标签或某个提交）。工具栏的
	// 分支芯片靠它换色 —— 分离 HEAD 上提交不会丢，但没有分支接着，得让人
	// 在提交之前就知道。
	detached bool
	// remotes 是配置的远程名（`git remote`）。抓取 / 拉取 / 推送三颗按钮的
	// 可用性看它：`git fetch --all` 在一条远程都没有时是**静默成功**的，
	// 等命令报错等不到「这个仓库没有远程」。
	remotes []string
	// ahead 和 behind 是当前分支相对配置上游的超前和落后提交数。
	// 用于在推送/拉取按钮上渲染徽标，让用户一目了然待同步的提交数。
	ahead    int
	behind   int
	upstream string
	// pending 是仓库正卡在哪一步写操作中间（合并/变基撞了冲突停着），跟着
	// 每次 Load 一起读回来。状态条（pendingbar.go）和分支芯片靠它说话。
	pending gitrepo.Pending
	state   loadState
	errMsg  string
	// loadingPath 是这一趟正在读的仓库路径。它不是 repoPath：repoPath 只在
	// 读完（或读失败）之后才改，而「正在重读的旧内容还能不能继续显示」要
	// 比着它来判断 —— 换个仓库时旧仓库的 diff 一秒都不该留在屏幕上，同
	// 一个仓库刷新时则正好相反，留着才不会闪（见 hasDiff）。
	loadingPath string
	// errNotRepo 说明这次的失败是「选的文件夹不是仓库」。它和别的加载失败
	// 分开：那种重试有意义，这种重试多少次都一样，该请用户换一个文件夹。
	errNotRepo bool
	// 四个序号各自守住一路异步加载：每次发起自增，回调只在序号仍是最新的
	// 时候才写入结果，这样快速切换时旧请求的结果不会覆盖新的。
	//
	// 分开是必须的：早期只有一个 loadSeq，于是「读提交列表」和「读这个提交
	// 的 diff」会互相撞掉 —— 第二次进历史 tab 时，后发的那个把先发的序号
	// 顶掉，提交列表就永远停在旧数据上（新提交看不见）。
	repoSeq int // openRepo：整个仓库
	logSeq  int // loadCommits：提交列表
	diffSeq int // loadCommit：选中提交的 diff

	// —— 模型 ——
	// allFiles 是当前显示的完整文件列表（工作区改动，或某个提交的 diff）；
	// files 是过了筛选词之后、界面真正显示的那一份。文件树、改动区、
	// 统计都只看 files，三者才对得上。
	allFiles []gitrepo.File
	files    []gitrepo.File
	// workFiles 是工作区的文件列表。切到历史看某个提交时 allFiles 会换成
	// 那个提交的 diff，切回来要还原，所以单独留一份。
	workFiles []gitrepo.File
	// histFiles 是历史 tab 当前看的文件列表，切去改动 tab 时留一份；
	// 切回历史时立即恢复，避免两区同时清空、白屏闪烁。
	histFiles  []gitrepo.File
	histListID string

	// —— 改动区（review.go）——
	// review 是右边那一栏的状态：一次一个文件的 DiffView。
	review reviewState
	// reviewDrafts 记着「改了还没存」的缓冲区，key 是 fileKey。
	//
	// 在改动区里改字是很容易顺手做的一件事（那本来就是个编辑器），而切到
	// 另一个文件会换掉缓冲区 —— 没有这份草稿，切走就等于把刚打的字扔掉，
	// 而且一句提示都没有。草稿只在「同一个文件、同一份内容」时接回来
	// （指纹对不上说明磁盘上那份已经变了，旧草稿再摆出来就是骗人）。
	reviewDrafts map[string]reviewDraft
	// listID 是「现在这份文件列表是谁的」，见 showFiles。同一个对象刷新
	// （暂存之后重读、⌘R）ID 不变，选中的文件因此跟得住；换了对象就作废
	// —— 历史提交之间、储藏之间的路径完全一样，光看 key 分不出这是谁的。
	listID string

	// —— 视图 ——
	tabs      []*repoTab // 打开的多项目标签页
	activeTab int        // 当前激活的标签页下标
	viewMode  int        // 0 = Git GUI 面板, 1 = 内置终端
	split     bool       // true = split（左右对照），false = unified
	codeSize  float32    // 代码字号；0 表示用默认值
	// termThemePushed / termThemeDark 记着上一次推给终端的明暗。终端的配色
	// 是启动时给的一份快照，换主题它不会自己跟上 —— 比一眼，变了才推
	// （见 syncTerminalThemes）。第一个字段是「推过没有」：默认（浅色）
	// 那一次也得推。
	termThemePushed bool
	termThemeDark   bool
	// menu 是菜单栏里那几个要跟着状态走的项目，见 menu.go。测试里没有
	// 菜单，它是 nil —— syncMenu 因此什么都不做。
	menu *appMenu

	// —— 侧栏：分支 / 远程 / 标签 / 储藏 ——
	refs      []gitrepo.Ref
	refByFull map[string]*gitrepo.Ref // Full → Ref，点一行时靠它找回是哪个引用
	refTree   []*filetree.Node
	refsList  ui.ListState
	refsState loadState
	refErr    string
	refsSeq   int
	// refFilter 是侧栏的筛选词，fileFilter 是主区文件栏的。两个框筛的是
	// 完全不同的东西，合成一个的话切 tab 就会互相干扰。
	refFilter  string
	refsFilter string // 建树时用的那份，和 refFilter 不一致就说明该重建了
	refSel     string // 选中那条引用的 key
	// refsClosed 记的是用户**收起过**的分组。没记过的一律展开，所以不用
	// 再去追「哪些是第一次见到的」—— 默认展开，只有手动收起才记一笔。
	refsClosed map[string]bool
	// dblKey/dblAt 记着刚双击过的那一行引用，用来吃掉双击尾巴上那一次单击
	// （见 sidebar.go 的 swallowTrailingClick）。
	dblKey string
	dblAt  time.Time

	// —— 主区文件栏（原来的侧栏文件树）——
	tree             []*filetree.Node
	filesList        ui.ListState
	sidebarOn        bool
	sidebarWidthVal  float32 // 侧栏宽度，0 时取默认值
	sidebarDragging  bool
	filePaneWidthVal float32 // 主区文件列表栏宽度，0 时取默认值
	filePaneDragging bool
	fileFilter       string
	fileTreeFilter   string // 建树时用的筛选词，和 fileFilter 不一致就说明该重建了
	// filesClosed 记的是用户收起过的目录，同 refsClosed：默认展开。
	// review 改动时几乎总想直接看到文件列表，默认收起反而要先点一下。
	filesClosed map[string]bool
	// fileSel 是文件栏里选中的文件下标（不是行下标 —— 行下标一筛选就指错）。
	fileSel int

	// —— 已 review：文件路径 -> 当时的内容指纹 ——
	// 指纹对不上就说明文件又变了，标记自动失效。
	viewed map[string]string

	// —— 历史 ——
	tab        int // 0 = 改动，1 = 历史
	commits    []gitrepo.Commit
	graph      []graphRow // 和 commits 一一对应，见 setCommits
	maxLanes   int        // 整张图占几条泳道，图形栏按它定宽
	commitList ui.ListState
	commitSel  int // 选中的提交下标，-1 表示还没选
	// histRev 是历史看的是哪条引用（侧栏点的那条分支/标签），空串表示 HEAD。
	// 不为空时 Log 直接喂给它，不能悄悄退回 HEAD —— 点了分支看着没反应
	// 比报错还难受。
	histRev string
	// stashRev 是下半区正在显示的储藏（完整引用名），空串表示没在看储藏。
	//
	// 和 histRev 是两回事：储藏不属于任何一条分支历史，提交列表照旧显示
	// histRev 那份，只有下半区换成储藏的改动。
	//
	// 单独立一个字段，是因为 loadCommits 拿到结果时会「顺手选中最近那个
	// 提交」—— 那一下会把下半区顶掉。用户点的是储藏，不该被自动选中抢走。
	stashRev string
	// stashName 是同一个储藏给 git 的名字（stash@{n}）。改动区要拿它比
	// 「储藏里的样子 vs 它的父提交」，而 Full 那种 refs/… 形态 git show 不认。
	stashName string
	// histSplit 是提交列表那一栏的高度，SplitVertical 拖的就是它。
	// splitH 是上一帧量到的可用高度，用来在窗口变矮时把 split 收敛回来。
	histSplit float32
	splitH    float32
	// hist 是提交列表本身的状态，histDiff 是选中那个提交的 diff 的状态。
	// 必须分开：列表和 diff 是两路异步加载，共用一个的话点一个提交会让
	// 整个列表闪成「正在读提交…」。
	hist            loadState
	histErr         string
	histHasMore     bool
	histLoadingMore bool
	histDiff        loadState
	histDiffErr     string

	// —— AI 能力（智能提交、PR 评论、分支命名、风险扫描）——
	store *store.Store
	// commitLang 控制 AI 提交信息生成语言："" = 跟随预设默认, "zh" = 中文, "en" = 英文
	commitLang string
	// settingsOn 是「模型设置」对话框是否打开；editCfg 是编辑中的副本，
	// 点确定之前不动真配置。
	settingsOn    bool
	settingsTab   settingsTab
	editCfg       store.Config
	editIdx       int
	showAPIKey    bool
	magpieOnline  bool
	magpieModels  []string
	magpieProbing bool
	themeMode     string // "system", "light", "dark"
	locale        string // "zh", "en"
	appIcon       string // "black", "white"
	editThemeMode string
	editLocale    string
	editAppIcon   string

	// 自定义提示词配置
	editCommitPrompt string
	editBranchPrompt string

	// 模型设置搜索过滤状态
	providerSearch string
	modelSearch    string

	// —— 顶部弹窗（HeroUI 下拉）——
	repoDropdownOpen   bool
	branchDropdownOpen bool
	// newTabMenuOpen 是顶栏 [+] 那个「新建项目」下拉：列已经打开过的项目，
	// 末位才是「打开本地文件夹…」。以前 [+] 会直接弹系统文件夹选择器，
	// 想回到刚关掉的项目得重走一遍路径。参照 Zed 的做法改成先给一份历史。
	newTabMenuOpen bool

	// —— 命令栏（⌘K）——
	palette paletteState
	// focusFilter 是 ⌘P（以及命令栏里的「筛选文件」）的一次性请求：
	// 下帧把焦点放进筛选框。
	focusFilter bool

	// —— 写操作（暂存 / 提交 / 检出 / 抓取……）——
	// 同一时刻只允许一个写操作在飞：git 的 index 只有一把锁，两个写操作
	// 并发的结果是「谁先拿到锁谁赢、另一个报 index.lock」，对用户就是随机
	// 失败。所有写路径的第一件事都是问它（见 startOp）。
	op     gitrepo.Op
	opSeq  int    // 发起时自增；回调只认自己那一次，和上面四个序号同理
	opName string // 「正在暂存…」这类给界面看的说明
	// opErr / opNote 是操作结果要显示的东西：失败给翻成人话的说法（顶部
	// 横幅，自己关），成功给一句确认（Toast，几秒后自己消失）。下一次操作
	// 或点 × 时清掉。toasted 记着哪句成功提示已经播报过 —— 每帧重新 Toast
	// 会不断重置它的计时，永远不消失。
	opErr   *gitrepo.Failure
	opNote  string
	toasted string

	// tag 是「新建标签」对话框的状态（tagdialog.go）。
	tag tagState

	// reset / newBranch / rename 是表单对话框，和 tag 一个形状：状态挂在 app 上。
	reset     resetState
	newBranch branchState
	rename    renameState
	stashDlg  stashState
	// codeEdit 是「编辑代码」那个一次性对话框的编辑器状态；editorPane 是
	// 「编辑器」视图里那个常驻整文件编辑器（editor_pane.go）。两者都不进
	// 标签页快照，换仓库时一起收掉。
	codeEdit   codeEditorState
	editorPane editorPaneState
	// compare 是分支代码差异对比的状态（Fork 风格，见 compare.go）。
	compare    compareState
	compareSeq int

	// prDlg 与 prReview 是 GitHub Pull Request 检视与 AI Review 状态（见 prdialog.go）。
	prDlg    prDialogState
	prReview prReviewState

	// copyText 是「复制哈希」这类待写进剪贴板的文本。菜单回调里拿不到
	// *ui.Context（WriteClipboard 是它的方法），先记在这里，view 开头
	// 写进剪贴板并清掉 —— 一趟只有一帧的延迟。
	copyText string
	copyNote string

	// confirm 是确认对话框的状态（confirmdialog.go）。
	confirm confirmState

	// openPath 把文件交给系统默认的「打开方式」（冲突卡片上的「在编辑器
	// 打开」）。main.go 接上 mygo 的 Shell.OpenPath；测试里是 nil，点了
	// 什么都不发生 —— 无头环境里打开文件本来也无从谈起。
	openPath func(string)

	// revealPath 在系统的文件管理器里亮出这个文件（文件行右键的「在访达
	// 中显示」）。main.go 接 mygo 的 Shell.ShowItemInFolder，测试里同样是
	// nil —— 和 openPath 一个道理。
	revealPath func(string)

	// fileHist 是「文件历史」对话框的状态（filehist.go）。
	fileHist fileHistState

	// commitMsg 是提交框里的消息；草稿按仓库持久化（store.Drafts），
	// draftFor 记着当前这份草稿属于哪个仓库。
	commitMsg string
	draftFor  string
	// commitFocused 记上一帧提交框有没有焦点：焦点离开的那一帧落盘草稿。
	commitFocused bool

	// —— AI 提交信息生成 ——
	aiCommitLoading bool
	aiCommitCancel  context.CancelFunc
	aiCommitMail    *aiMail

	// —— 变更预检 ——
	riskScan riskScanState

	// amend 记录是否勾选了「修改上次提交（Amend）」。
	amend   bool
	hasHead bool
}

func newApp() *app {
	SetLocale("zh")
	a := &app{
		split:     true,
		sidebarOn: true,
		themeMode: "system",
		locale:    "zh",
		viewed:    map[string]string{},
		commitSel: -1,
		fileSel:   -1,
		// 没接上真实目录之前先用一个空 store，这样 newApp 之后随时可以
		// 直接跑（测试就是这么用的）；main 里再换成真正的数据目录。
		store: store.Open(""),
	}
	// 测试里 dispatch 不直接执行，而是排进队列，由 settle 在测试 goroutine
	// （也就是扮演主线程的那一个）上跑完。直接执行的话这些闭包会在后台
	// goroutine 上改 app 的字段，第 1 条约定就破了 —— race detector 也就
	// 失去了意义。生产环境由 main 换成 win.Update，同样是跳回主线程。
	a.dispatch = a.loop.post
	return a
}

// mainLoop 是测试里的「主线程」：一个先进先出的闭包队列。
type mainLoop struct {
	mu sync.Mutex
	q  []func()
}

func (l *mainLoop) post(f func()) {
	l.mu.Lock()
	l.q = append(l.q, f)
	l.mu.Unlock()
}

// drain 把队列里的闭包依次执行完，返回是否执行过。
//
// 执行过程中可能又排进新的闭包（比如落盘任务结束后的回调），所以循环到
// 队列真的空了为止。
func (l *mainLoop) drain() bool {
	ran := false
	for {
		l.mu.Lock()
		batch := l.q
		l.q = nil
		l.mu.Unlock()
		if len(batch) == 0 {
			return ran
		}
		ran = true
		for _, f := range batch {
			f()
		}
	}
}

// attachStore 换上真正的数据目录，并把上次的状态恢复进来。
//
// 它同时承担「启动时恢复现场」的职责：调用的地方在窗口建好、view 挂上之后，
// 所以这里改状态会自然反映到第一帧。
func (a *app) attachStore(s *store.Store) {
	a.store = s
	cfg := s.Config()
	a.codeSize = cfg.CodeSize
	a.split = !cfg.Unified
	a.sidebarOn = !cfg.SidebarOff
	a.sidebarWidthVal = cfg.SidebarWidth
	a.filePaneWidthVal = cfg.FilePaneWidth
	a.themeMode = cfg.ThemeMode
	if a.themeMode == "" {
		a.themeMode = "system"
	}
	a.locale = cfg.Locale
	if a.locale == "" {
		a.locale = "zh"
	}
	SetLocale(a.locale)
	a.appIcon = cfg.AppIcon
	if a.appIcon == "" {
		a.appIcon = "black"
	}
	a.syncAppIcon()
	a.viewed = s.State().Viewed
	if a.viewed == nil {
		a.viewed = map[string]string{}
	}
	a.editIdx = cfg.Active

	// 恢复已打开的多项目标签页
	a.tabs = nil
	for _, p := range s.State().OpenRepos {
		if p != "" {
			a.tabs = append(a.tabs, &repoTab{
				path:     p,
				name:     filepath.Base(p),
				viewMode: viewModeGit,
				state:    loadIdle,
			})
		}
	}
	if len(a.tabs) == 0 && s.State().LastRepo != "" {
		p := s.State().LastRepo
		a.tabs = append(a.tabs, &repoTab{
			path:     p,
			name:     filepath.Base(p),
			viewMode: viewModeGit,
			state:    loadIdle,
		})
	}
	if len(a.tabs) > 0 {
		a.activeTab = 0
	}
}

// persist 把设置、状态和缓存落盘。写文件不占主线程，扔到后台去。
func (a *app) persist() {
	s := a.store
	a.run(func() { _ = s.Save() })
}

// saveViewSettings 记下跟界面有关的几项设置（视图模式、侧栏、字号、面板宽度、外观与语言）。
func (a *app) saveViewSettings() {
	cfg := a.store.Config()
	cfg.Split = a.split
	cfg.Unified = !a.split
	cfg.SidebarOff = !a.sidebarOn
	cfg.CodeSize = a.codeSize
	if a.sidebarWidthVal > 0 {
		cfg.SidebarWidth = a.sidebarWidthVal
	}
	if a.filePaneWidthVal > 0 {
		cfg.FilePaneWidth = a.filePaneWidthVal
	}
	cfg.ThemeMode = a.themeMode
	cfg.Locale = a.locale
	cfg.AppIcon = a.appIcon
	a.store.SetConfig(cfg)
	a.persist()
}

// syncAppIcon 同步当前选中的图标至 macOS Dock。
func (a *app) syncAppIcon() {
	if a.win == nil {
		return
	}
	if a.appIcon == "white" {
		_ = mygo.App.Dock.SetIcon(iconPureWhitePNG)
	} else {
		_ = mygo.App.Dock.SetIcon(iconPureBlackPNG)
	}
}

// setAppIcon 切换应用图标风格（"black" / "white"）。
func (a *app) setAppIcon(mode string) {
	if mode != "white" {
		mode = "black"
	}
	if a.appIcon == mode {
		return
	}
	a.appIcon = mode
	a.syncAppIcon()

	// 将对应图标写回 resources/icon.png，保证未来打包与构建时生效
	src := iconPureBlackPNG
	if mode == "white" {
		src = iconPureWhitePNG
	}
	if len(src) > 0 {
		_ = os.WriteFile("resources/icon.png", src, 0o644)
	}

	a.saveViewSettings()
}

// setThemeMode 切换外观色彩模式（"system" / "light" / "dark"）。
func (a *app) setThemeMode(mode string) {
	if mode != "light" && mode != "dark" {
		mode = "system"
	}
	if a.themeMode == mode {
		return
	}
	a.themeMode = mode
	a.saveViewSettings()
	a.syncMenu()
}

// setLocale 切换全局语言设定（"zh" / "en"）。
func (a *app) setLocale(loc string) {
	if loc != "en" {
		loc = "zh"
	}
	if a.locale == loc {
		return
	}
	a.locale = loc
	SetLocale(loc)
	a.saveViewSettings()
	a.rebuildRefTree()
	a.rebuildTree()
	if a.menu != nil {
		mygo.App.SetMenu(buildAppMenu(a))
	}
	a.syncMenu()
}

// isDark 根据用户外观偏好和当前环境计算生效的暗黑状态。
func (a *app) isDark(c *ui.Context) bool {
	switch a.themeMode {
	case "light":
		return false
	case "dark":
		return true
	default:
		return c.Theme().Dark
	}
}

func (a *app) view(c *ui.Context) {
	// 换成 Gloss 自己的主题。必须在建任何控件之前：SetTheme 只作用于
	// 调用之后创建的控件，放在后面的话这一帧前半截还是框架默认配色。
	// 明暗由 isDark 综合系统外观与用户偏好计算。
	dark := a.isDark(c)
	c.SetTheme(themeFor(dark, c.Preferences()))
	pal := paletteFor(c.Theme())

	// 终端自己留着一份配色，不像界面那样每帧从调色板取色 —— 明暗变了得推给它。
	a.syncTerminalThemes(dark, pal)

	// 晚一帧的剪贴板写入（「复制哈希」这类菜单命令，见 copyText 字段）。
	// 放在最前面：这一帧的 Toast 就能把它报出来。
	if a.copyText != "" {
		c.WriteClipboard(a.copyText)
		note := a.copyNote
		if note == "" {
			if strings.Contains(a.copyText, "\n") || len(a.copyText) > 60 {
				note = T("已复制到剪贴板", "Copied to clipboard")
			} else {
				note = Tf("已复制 %s", "Copied %s", a.copyText)
			}
		}
		a.opNote = note
		a.copyText = ""
		a.copyNote = ""
	}

	// 从文件历史跳去看某次提交（见 consumeHistoryJump）。和上面那次剪贴板
	// 写入一样，都是「回调里只记一笔、下一帧开头再做」的事。
	a.consumeHistoryJump()

	// 快捷键要在最前面处理：c.Shortcut 是「注册即认领」的，注册得越早
	// 越先拿到键。
	a.handleKeys(c)

	// 跟上这一帧的状态：菜单里的勾要在被拉开之前就是对的，而状态这一帧
	// 刚被快捷键改过。
	a.syncMenu()

	// 改动区跟着文件栏的选中 / 段落 / 仓库状态走：选中换了就换缓冲区，
	// 指纹变了就重读两侧（这一帧先看到旧的，读盘回来自己刷新）。
	a.syncReview()

	// 窗口开了 vibrancy：根节点不铺底色，让系统的毛玻璃材质透上来；
	// 只有主区不透明，保证代码区域的对比度稳定、不受壁纸颜色影响。
	c.Root().Background(ui.Transparent)

	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.projectTabBar(c, pal)

		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.sidebarOn {
				a.sidebar(c, pal)
			}
			ui.Column(c).Grow(1).MinWidth(0).Background(pal.appBg).Children(func() {
				a.toolbar(c, pal)
				a.mainArea(c, pal)
			})
		})
	})
	a.paletteDialog(c, pal)
	a.settingsDialog(c, pal)
	a.tagDialog(c, pal)
	a.resetDialog(c, pal)
	a.newBranchDialog(c, pal)
	a.renameBranchDialog(c, pal)
	a.confirmDialog(c, pal)
	a.stashDialog(c, pal)
	a.compareDialog(c, pal)
	a.prDialog(c, pal)
	a.prReviewModal(c, pal)
	a.riskScanDialog(c, pal)
	a.fileHistDialog(c, pal)
	a.codeEditorDialog(c, pal)

	// Esc 退出分支对比/PR 检视。放在最后，让对话框里正在编辑的输入框先有机会处理它。
	if a.compare.on && !a.modalOn() && c.Shortcut(0, ui.KeyEscape) {
		a.exitCompare()
	}
	if a.prReview.on && !a.modalOn() && c.Shortcut(0, ui.KeyEscape) {
		a.exitPRReview()
	}
}

// titleMaxW 是工具栏左边那块「仓库名 + 路径」的宽度上限。
//
// 300 是在 1280 宽的窗口里量出来的：把整条工具栏摆得下（两边都开着侧栏）
// 的前提下，够放一个项目名加一段看得出来的路径。
const titleMaxW = 300

// titleMinW 是它再挤也要留下的宽度：仓库名那一行是「我在看哪个仓库」，
// 压没了整条工具栏就没剩下什么了。
const titleMinW = 140

// titleWidth 是这一趟给标题块多少宽度。
//
// 上限 300 留给宽窗口：路径再长也不该占掉半条工具栏。窄窗口要跟着可用
// 宽度收 —— 工具栏把摆不下的控件收进「»」是从右往左收的，左边多占一寸，
// 右边就少一寸：窗口收到最小（720）时标题块还占满 300 的话，连「改动 /
// 历史」都会被挤进菜单，而那是主区唯一的导航。
//
// 按主区宽度算而不是窗口宽度：侧栏不归工具栏。
func (a *app) titleWidth(c *ui.Context) float32 {
	winW, _ := c.Size()
	if a.sidebarOn {
		winW -= a.sidebarWidth(c)
	}
	return clamp(winW*0.4, titleMinW, titleMaxW)
}

func (a *app) sidebarWidth(c *ui.Context) float32 {
	w := a.sidebarWidthVal
	if w <= 0 {
		w = sidebarDefaultW
	}
	maxW := sidebarMaxW
	if c != nil {
		if winW, _ := c.Size(); winW > 0 {
			maxW = min(sidebarMaxW, max(sidebarMinW, winW-360))
		}
	}
	return clamp(w, sidebarMinW, maxW)
}

func (a *app) setSidebarWidth(c *ui.Context, w float32) {
	maxW := sidebarMaxW
	if c != nil {
		if winW, _ := c.Size(); winW > 0 {
			maxW = min(sidebarMaxW, max(sidebarMinW, winW-360))
		}
	}
	clamped := clamp(w, sidebarMinW, maxW)
	if clamped != a.sidebarWidthVal {
		a.sidebarWidthVal = clamped
	}
}

func (a *app) resetSidebarWidth() {
	a.sidebarWidthVal = sidebarDefaultW
	a.saveViewSettings()
}

// toolbarHeight 是工具栏那条带子的高度。toolbar 用的是同一个公式 ——
// 分成两处算的话，主区的高度就会和实际差几个像素，分隔条的夹逼跟着错。
func (a *app) toolbarHeight(c *ui.Context) float32 {
	return 46
}

// toolbar 是二级工具栏：提供 [Git 面板 | 终端 | 编辑器] 模式切换及当前模式对应的操作。
func (a *app) toolbar(c *ui.Context, pal palette) {
	t := c.Theme()
	bar := c.TitleBar()

	padLeft := float32(12)

	ui.Toolbar(c, func() {
		// 视图模式切换：Git 面板 ⟷ 内置终端 ⟷ 代码编辑器
		vm := a.viewMode
		toolbarSegmentedIcons(c, pal, &vm, false,
			segmentedIconItem{label: "Git", tip: Tf("Git 可视化面板 %s / Ctrl+~", "Git GUI Panel %s / Ctrl+~", K("⌘1")), icon: iconFileDiff},
			segmentedIconItem{label: "Terminal", tip: Tf("内置终端 %s / Ctrl+~", "Built-in Terminal %s / Ctrl+~", K("⌘2")), icon: iconTerminal},
			segmentedIconItem{label: "Editor", tip: Tf("代码编辑器 %s", "Code Editor %s", K("⌘3")), icon: iconCode},
		)
		if vm != a.viewMode {
			a.setViewMode(vm)
		}

		toolbarDivider(c, pal)

		if a.viewMode == viewModeTerm {
			if a.repoPath != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					drawIcon(c, iconTerminal, 12, pal.inkSubtle)
					ui.Text(c, a.repoPath).FontSize(textCaption).TextColor(pal.inkMuted).SingleLine()
				})
			}
			ui.Spacer(c)
			if toolIconButton(c, pal, T("清屏", "Clear"), T("清空终端屏幕", "Clear Terminal Screen"), false, false, drawTrashIcon) {
				a.clearCurrentTerminal()
			}
			if toolIconButton(c, pal, T("重启", "Restart"), T("重启终端 Shell 进程", "Restart Terminal Shell Process"), false, false, drawZapIcon) {
				a.restartCurrentTerminal()
			}
			if toolIconButton(c, pal, T("打开仓库…", "Open…"), Tf("打开本地 Git 仓库 %s", "Open Local Git Repository %s", K("⌘O")), false, false, drawFolderIcon) {
				a.chooseFolder()
			}
			return
		}

		// 侧栏未开启时在工具栏左侧展示仓库名下拉与分支；侧栏开启时仓库名已位于侧栏顶部，工具栏直接展示当前分支
		if !a.sidebarOn {
			a.repoHeader(c, t, pal)
		} else if a.branch != "" {
			a.branchChip(c, t, pal)
		}

		// 耗时写操作（抓取/拉取/推送/变基/合并/检出等）在标题和 Spacer 之间展示，
		// 宽度由 Spacer 弹性吸收，避免右侧核心工具栏发生任何水平抖动与重排。
		// 暂存/取消暂存为瞬时操作，提交在底部提交栏展示，均不在此喧宾夺主。
		if a.toolbarOpVisible() {
			ui.Box(c).Radius(radiusPill).Padding(3, 9).
				Background(pal.surface2).
				Border(1, pal.hairline).
				Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
						ui.Spinner(c).Size(12, 12)
						ui.Text(c, a.opName).FontSize(textCaption).
							TextColor(pal.inkMuted).SingleLine()
					})
				})
		}

		ui.Spacer(c)

		if a.viewMode == viewModeEditor {
			a.editorToolbar(c, pal)
			return
		}

		noRepo := !a.repoReady()

		// unified / split 切换（Lucide rows-3 / columns-2 矢量图标）。
		split := a.split
		mode := 0
		if split {
			mode = 1
		}
		toolbarSegmentedIcons(c, pal, &mode, noRepo,
			segmentedIconItem{label: "Unified", tip: Tf("统一视图（单栏） %s", "Unified View %s", K("⌘U")), icon: iconUnified},
			segmentedIconItem{label: "Split", tip: T("左右对照（分栏）", "Split View"), icon: iconSplit},
		)
		if (mode == 1) != split {
			a.setSplit(mode == 1)
		}

		if len(a.remotes) > 0 {
			toolbarDivider(c, pal)
			a.remoteBar(c, pal)
		}

		toolbarDivider(c, pal)

		// 刷新、设置、打开仓库（纯图标按钮，对齐 Fork 与 macOS 原生设计规范）
		refreshing := a.state == loadLoading
		refreshTip := Tf("刷新仓库状态 %s", "Refresh Repository %s", K("⌘R"))
		if refreshing {
			refreshTip = T("正在刷新仓库状态…", "Refreshing repository…")
		}
		if toolIconButton(c, pal, T("刷新", "Refresh"), refreshTip, noRepo, refreshing, drawRefreshIcon) {
			a.reload()
		}
		if toolIconButton(c, pal, T("打开仓库…", "Open…"), Tf("打开本地 Git 仓库 %s", "Open Local Git Repository %s", K("⌘O")), false, false, drawFolderIcon) {
			a.chooseFolder()
		}
		// 设置图标不在这里 —— 它已经在上方标签栏的右上角（projectTabBar）。
		// 两处同时摆一枚一模一样的齿轮、相隔 48px，看着像画重了；而且终端
		// 模式下这条工具栏右侧换成了清屏 / 重启 / 打开，设置只会在标签栏那
		// 一枚上固定出现。⌘, 两条路都通。
	}).Height(a.toolbarHeight(c)).Shrink(0).
		Padding(0, bar.Right+12, 0, padLeft).
		Gap(10).
		DragWindow().
		Background(pal.appBg).
		Border(0, pal.hairline).
		Label(T("工具栏", "Toolbar"))
}

// repoHeader 是工具栏左侧的仓库选择器：仓库名（带下拉箭头，点击展开最近
// 项目）与当前分支徽章。
//
// 名字下面原先还压着一行精简路径。那行正好贴在项目标签正下方 —— 标签刚
// 报过项目名，下一行又是一条路径，同一个项目在顶栏这一带要自我介绍两遍。
// 路径本身没被藏起来：项目标签的悬停提示给完整路径，最近项目下拉里每条
// 都带着。
func (a *app) repoHeader(c *ui.Context, t *ui.Theme, pal palette) {
	tw := a.titleWidth(c)
	name := a.repoName
	if name == "" {
		name = "Gloss"
	}

	ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).MaxWidth(tw).Children(func() {
		repoBtn := ui.ButtonBase(c).Radius(radiusSM).Padding(2, 6, 2, 0).
			Cursor(ui.CursorPointer).Label(name).
			Tooltip(Tf("点击切换最近项目（或 %s 打开本地仓库）", "Switch recent project (or %s to open repository)", K("⌘O")))
		if repoBtn.Hovered() || a.repoDropdownOpen {
			repoBtn.Background(pal.surface2)
		}
		fadeColors(repoBtn)
		repoBtn.Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontWeight(weightSemibold).FontSize(textBody).SingleLine().MaxWidth(180)
				drawIcon(c, iconChevronDown, 10, pal.inkSubtle)
			})
		})
		if repoBtn.Clicked() {
			a.branchDropdownOpen = false
			a.newTabMenuOpen = false
			a.repoDropdownOpen = !a.repoDropdownOpen
		}
		if a.repoDropdownOpen {
			ui.PopoverBase(c, repoBtn, &a.repoDropdownOpen, func(panel ui.Element) {
				a.repoDropdownPanel(c, panel, pal)
			})
		}

		a.branchChip(c, t, pal)
	})
}

// repoDropdownPanel 呈现最近项目下拉卡片。
//
// 这是浮层，按 §2.1 的阶梯落在 Surface 3（比它下面的侧栏亮一档），
// 分层交给这一档明度差和一道 hairline；投影只留一点收边，不再是原来那圈
// 30px 的大模糊 —— 暗色模式里粗重投影糊在深色底上，只会显脏。
func (a *app) repoDropdownPanel(c *ui.Context, panel ui.Element, pal palette) {
	panel.Radius(radiusLG).Background(pal.surface3).Border(1, pal.hairline).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.16)).
		Margin(4, 0, 0, 0).Padding(6).MinWidth(340).MaxWidth(440).Gap(2)

	recent := a.store.State().RecentRepos
	hasRecent := false
	for _, p := range recent {
		if p != "" {
			hasRecent = true
			break
		}
	}

	// 标题区域
	ui.Row(c).Padding(6, 8, 4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		drawIcon(c, iconClock, 12, pal.inkSubtle)
		ui.Text(c, T("最近打开的项目", "Recent Repositories")).FontSize(textCaption).
			FontWeight(weightSemibold).TextColor(pal.inkSubtle)
	})

	if !hasRecent {
		ui.Box(c).Padding(8, 10).Children(func() {
			ui.Text(c, T("暂无最近打开的项目", "No recent repositories")).
				FontSize(textBodySM).TextColor(pal.inkSubtle)
		})
	} else {
		ui.Scroll(c).MaxHeight(320).FillWidth().Children(func() {
			for _, p := range recent {
				if p == "" {
					continue
				}
				pathCopy := p
				base := filepath.Base(p)
				isCurrent := (p == a.repoPath)

				item := ui.ButtonBase(c).Height(40).Radius(radiusSM).Padding(4, 8).Gap(8).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)

				// 当前仓库那一行用中性选中底带，和树里「选中的这一行」是同一档
				// （pal.selection）。原来这里连底带、边框、名字、图标一起染成
				// 强调色 —— 一行里说了四遍，而它只是「你正在看的这个」。
				bg := ui.Transparent
				switch {
				case isCurrent && item.Hovered():
					bg = pal.surface4
				case isCurrent:
					bg = pal.selection
				case item.Hovered():
					bg = pal.surface2
				}
				item.Background(bg)
				fadeColors(item)

				item.Children(func() {
					iconCol := pal.inkSubtle
					if isCurrent {
						iconCol = pal.inkMuted
					}
					drawIcon(c, iconFolder, 14, iconCol)

					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						tElem := ui.Text(c, base).FontSize(12.5).SingleLine().TextColor(pal.ink)
						if isCurrent {
							tElem.FontWeight(weightMedium)
						}
						ui.Text(c, pathCopy).FontSize(textCaption).SingleLine().
							TextColor(pal.inkSubtle)
					})

					if isCurrent {
						// 语义靠圆点，不靠染色（§2.5）。
						specChip(c, pal, T("当前", "Current"), pal.laneColor(0), rowChipH, rowChipText)
					}
				})

				if item.Clicked() {
					a.repoDropdownOpen = false
					if !isCurrent {
						a.openRepo(pathCopy)
					}
				}
			}
		})
	}

	// 分隔线
	ui.Box(c).Height(1).FillWidth().Margin(4, 2).Background(pal.hairline)

	// 打开其他本地仓库… ⌘O
	openBtn := ui.ButtonBase(c).Height(30).Radius(radiusSM).Padding(0, 8).Gap(7).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if openBtn.Hovered() {
		openBtn.Background(pal.surface2)
	}
	fadeColors(openBtn)
	openBtn.Children(func() {
		drawIcon(c, iconFolderPlus, 13, pal.inkMuted)
		ui.Text(c, T("打开其他本地仓库…", "Open Other Local Repository…")).
			FontSize(textButton).TextColor(pal.ink)
		ui.Spacer(c)
		ui.Box(c).Radius(radiusXS).Padding(1, 5).Background(pal.surface1).
			Border(1, pal.hairlineStrong).Children(func() {
			ui.Text(c, K("⌘O")).FontSize(textCaption).Font("monospace").TextColor(pal.inkSubtle)
		})
	})
	if openBtn.Clicked() {
		a.repoDropdownOpen = false
		a.chooseFolder()
	}

	// 清除最近打开记录
	if hasRecent {
		clearBtn := ui.ButtonBase(c).Height(26).Radius(radiusXS).Padding(0, 8).Gap(6).
			AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if clearBtn.Hovered() {
			clearBtn.Background(pal.surface2)
		}
		fadeColors(clearBtn)
		clearBtn.Children(func() {
			drawIcon(c, iconTrash, 11, pal.inkSubtle)
			ui.Text(c, T("清除最近打开记录", "Clear Recent Repositories")).
				FontSize(textCaption).TextColor(pal.inkSubtle)
		})
		if clearBtn.Clicked() {
			a.repoDropdownOpen = false
			a.store.ClearRecentRepos()
			a.persist()
		}
	}
}

// branchChip 是工具栏上那枚「现在在哪条分支上」的小牌子。
// 采用 Lucide 矢量分支图标与微交互设计，点击可弹出本地分支快捷切换列表。
//
// 样子照 §4.2 来：Surface 1 底 + 1px hairline + Ink Muted 文字，悬停升 Surface 2、
// 边升到 hairline-strong。原来的版本给底和边都染了色（`pal.ref` 配 alpha），
// 看着像一枚「提示标签」而不是工具栏上的一个控件。
//
// 变基中 / 分离 HEAD 这两种异常**只把文字和图标染成 Warning**，底色和边框
// 照旧中性 —— 规范在这一条上写得很直白：异常状态仅用微弱的警示色文字点缀。
// 整块涂成琥珀会压过旁边真正的按钮，而这不是按钮。
func (a *app) branchChip(c *ui.Context, t *ui.Theme, pal palette) {
	if a.branch == "" {
		return
	}
	name := a.branch
	accent := pal.inkMuted
	tip := Tf("当前分支 %s（点击切换分支）", "Current branch %s (click to switch)", a.branch)
	switch {
	case a.pending.Kind == gitrepo.PendingRebase:
		name = a.rebaseLabel()
		accent = t.Warning
		tip = T("变基操作进行中。可在状态栏选择「继续」、「跳过」或「中止」。", "Rebase in progress. Use Continue, Skip, or Abort in the status bar.")
	case a.detached:
		name = T("分离 HEAD：", "Detached HEAD: ") + a.branch
		accent = t.Warning
		tip = T("HEAD 处于游离状态（分离 HEAD）。如需保留后续提交，请先创建新分支。", "HEAD is detached from any branch. Create a new branch if you wish to retain subsequent commits.")
	}

	chip := ui.ButtonBase(c).Height(28).Shrink(0).Radius(radiusSM).Padding(0, 9).
		Cursor(ui.CursorPointer).Tooltip(tip).Label(name)

	bg, border := pal.surface1, pal.hairline
	if chip.Hovered() || a.branchDropdownOpen {
		// 悬停用两个**不透明**的色值，不再对已经半透明的色再乘一次 alpha ——
		// 老的写法 `bg.Alpha(0.22)` 是相乘语义，作用在 0.12 的底色上只剩 0.026，
		// 鼠标指上去底色几乎消失。
		bg, border = pal.surface2, pal.hairlineStrong
	}
	chip.Background(bg).Border(1, border)
	fadeColors(chip)
	chip.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			drawIcon(c, iconGitBranch, 12, accent)
			ui.Text(c, name).FontSize(textButton).FontWeight(weightMedium).
				SingleLine().MaxWidth(220).TextColor(accent)
			drawIcon(c, iconChevronDown, 10, accent)
		})
	})
	if chip.Clicked() {
		a.repoDropdownOpen = false
		a.newTabMenuOpen = false
		a.branchDropdownOpen = !a.branchDropdownOpen
	}
	if a.branchDropdownOpen {
		ui.PopoverBase(c, chip, &a.branchDropdownOpen, func(panel ui.Element) {
			a.branchDropdownPanel(c, panel, pal)
		})
	}
}

// branchDropdownPanel 呈现本地分支快速切换卡片。
func (a *app) branchDropdownPanel(c *ui.Context, panel ui.Element, pal palette) {
	panel.Radius(radiusLG).Background(pal.surface3).Border(1, pal.hairline).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.16)).
		Margin(4, 0, 0, 0).Padding(6).MinWidth(220).MaxWidth(340).Gap(2)

	local, _, _, _ := splitRefs(a.refs)

	ui.Row(c).Padding(6, 8, 4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		drawIcon(c, iconGitBranch, 12, pal.inkSubtle)
		ui.Text(c, T("切换本地分支", "Switch Local Branch")).FontSize(textCaption).
			FontWeight(weightSemibold).TextColor(pal.inkSubtle)
	})

	if len(local) == 0 {
		ui.Box(c).Padding(8, 10).Children(func() {
			ui.Text(c, T("没有其他本地分支", "No other local branches")).
				FontSize(textBodySM).TextColor(pal.inkSubtle)
		})
		return
	}

	busy := a.op != gitrepo.OpNone
	ui.Scroll(c).MaxHeight(320).FillWidth().Children(func() {
		for _, b := range local {
			bName := b.Name
			isCurrent := (bName == a.branch)
			bRef := b

			btn := ui.ButtonBase(c).Height(30).Radius(radiusSM).Padding(0, 8).Gap(7).
				AlignItems(ui.Center).Cursor(ui.CursorPointer).Disabled(busy)

			bg := ui.Transparent
			if isCurrent {
				bg = pal.selection
			} else if btn.Hovered() {
				bg = pal.surface2
			}
			btn.Background(bg)
			fadeColors(btn)

			btn.Children(func() {
				fg := pal.inkSubtle
				nameCol := pal.ink
				if isCurrent {
					fg = pal.inkMuted
				}
				drawIcon(c, iconGitBranch, 12, fg)
				bText := ui.Text(c, bName).FontSize(12).SingleLine().MaxWidth(220).TextColor(nameCol)
				if isCurrent {
					bText.FontWeight(weightMedium)
				}
				ui.Spacer(c)
				if isCurrent {
					specChip(c, pal, T("当前", "Current"), pal.laneColor(0), rowChipH, rowChipText)
				}
			})
			if btn.Clicked() {
				a.branchDropdownOpen = false
				if !isCurrent && !busy {
					a.checkoutRef(bRef)
				}
			}
		}
	})

	// 分隔线与新建分支操作
	ui.Box(c).Height(1).FillWidth().Margin(4, 2).Background(pal.hairline)
	newBranchBtn := ui.ButtonBase(c).Height(28).Radius(radiusSM).Padding(0, 8).Gap(7).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Disabled(busy)
	if newBranchBtn.Hovered() {
		newBranchBtn.Background(pal.surface2)
	}
	fadeColors(newBranchBtn)
	newBranchBtn.Children(func() {
		drawIcon(c, iconGitBranch, 12, pal.inkMuted)
		ui.Text(c, T("新建分支…", "New Branch…")).FontSize(textButton).
			FontWeight(weightMedium).TextColor(pal.ink)
	})
	if newBranchBtn.Clicked() {
		a.branchDropdownOpen = false
		a.openNewBranchDialog(a.branch, Tf("从当前分支 %s 创建", "Create from branch %s", a.branch))
	}
}

// recentReposMenu 弹出最近打开的项目菜单。
func (a *app) recentReposMenu(m *ui.Menu) {
	recent := a.store.State().RecentRepos
	hasRecent := false
	for _, p := range recent {
		if p == "" {
			continue
		}
		hasRecent = true
		base := filepath.Base(p)
		pathCopy := p
		isCurrent := (p == a.repoPath)
		label := base + "  (" + p + ")"
		it := m.Item(label).Checked(isCurrent)
		if it.Chosen() && !isCurrent {
			a.openRepo(pathCopy)
		}
	}
	if hasRecent {
		m.Separator()
	}
	if m.Item(Tf("打开其他本地仓库… %s", "Open Other Local Repository… %s", K("⌘O"))).Chosen() {
		a.chooseFolder()
	}
	if hasRecent {
		if m.Item(T("清除最近打开记录", "Clear Recent Repositories")).Chosen() {
			a.store.ClearRecentRepos()
			a.persist()
		}
	}
}

// branchQuickMenu 弹出本地分支快速切换菜单。
func (a *app) branchQuickMenu(m *ui.Menu) {
	local, _, _, _ := splitRefs(a.refs)
	if len(local) == 0 {
		m.Item(T("没有其他本地分支", "No other local branches")).Disabled(true)
		return
	}
	busy := a.op != gitrepo.OpNone
	for _, b := range local {
		bName := b.Name
		isCurrent := (bName == a.branch)
		it := m.Item(bName).Checked(isCurrent).Disabled(busy || isCurrent)
		bRef := b
		if it.Chosen() {
			a.checkoutRef(bRef)
		}
	}
}

// mainArea 是右侧主区：顶上可能有一条操作结果横幅和一条「卡住了」状态条，
// 然后是两个 tab 的内容；「改动」tab 底下还压着一条提交栏。
//
// 状态条排在两个 tab 之前：合并/变基卡着的时候，不管在看改动还是历史，
// 「继续 / 跳过 / 中止」都得够得着 —— 它说的是仓库的状态，不是某个 tab 的。
func (a *app) mainArea(c *ui.Context, pal palette) {
	if a.viewMode == viewModeTerm {
		a.terminalArea(c, pal)
		return
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		a.opBanner(c, pal)
		a.pendingBar(c, pal)
		if a.viewMode == viewModeEditor {
			a.editorArea(c, pal)
			return
		}
		if a.compare.on {
			a.compareBar(c, pal)
			a.detailPane(c, pal)
			return
		}
		if a.prReview.on {
			a.prReviewBar(c, pal)
			a.detailPane(c, pal)
			return
		}
		if a.tab == 1 {
			a.historyArea(c, pal)
			return
		}
		a.detailPane(c, pal)
		a.commitBox(c, pal)
	})
}

// historyEmpty 是历史 tab 主区没有 diff 可画时的几种情况。
//
// 先问列表的状态、再问这个提交的 diff 的状态 —— 两者是两路独立的加载，
// 列表没读出来时根本谈不上「选中了哪个提交」。
func (a *app) historyEmpty(c *ui.Context, t *ui.Theme, pal palette) {
	switch {
	case a.hist == loadLoading && len(a.commits) == 0:
		ui.Spinner(c)
		ui.Text(c, T("正在读取提交记录…", "Loading commit history…")).TextColor(t.TextMuted)
	case a.hist == loadFailed && len(a.commits) == 0:
		headline(c, pal, T("无法读取提交记录", "Failed to Load History"))
		ui.Text(c, a.histErr).TextColor(t.Danger)
		if secondaryAction(c, pal, T("重试", "Retry"), "", false).Clicked() {
			a.loadCommits()
		}
	case len(a.commits) == 0:
		headline(c, pal, T("暂无提交记录", "No Commits Found"))
		ui.Text(c, T("当前仓库尚无任何提交记录。", "This repository has no commit history yet.")).TextColor(t.TextMuted)
	case a.histDiff == loadLoading && len(a.files) == 0:
		ui.Spinner(c)
		ui.Text(c, T("正在读取提交详情…", "Loading commit details…")).TextColor(t.TextMuted)
	case a.histDiff == loadFailed && len(a.files) == 0:
		headline(c, pal, T("无法读取提交详情", "Failed to Load Commit"))
		ui.Text(c, a.histDiffErr).TextColor(t.Danger)
		if secondaryAction(c, pal, T("重试", "Retry"), "", false).Clicked() {
			a.loadCommit(a.commitSel)
		}
	case a.commitSel >= 0 && a.commitSel < len(a.commits):
		cm := a.commits[a.commitSel]
		headline(c, pal, cm.Subject)
		ui.Textf(c, "%s · %s · %s", cm.Short, cm.Author, cm.When.Format("2006-01-02 15:04")).
			TextColor(t.TextMuted)
		ui.Text(c, T("此提交无文件变更内容。", "This commit contains no file changes.")).TextColor(t.TextMuted)
	default:
		headline(c, pal, T("选择提交记录", "Select a Commit"))
		ui.Text(c, T("请从列表中选择提交以查看变更详情。", "Select a commit from the list to view its changes.")).TextColor(t.TextMuted)
	}
}

func (a *app) totalAdd() int {
	n := 0
	for _, f := range a.files {
		n += f.AddCount
	}
	return n
}

func (a *app) totalDel() int {
	n := 0
	for _, f := range a.files {
		n += f.DelCount
	}
	return n
}

// dirtyCount 计算工作区未提交的文件数量（staged + unstaged），用于侧栏「改动」项上的数字角标。
func (a *app) dirtyCount() int {
	if a.workFiles != nil {
		return len(a.workFiles)
	}
	if a.tab == 0 {
		return len(a.allFiles)
	}
	return 0
}

// run 起一个后台任务，并登记到 bg 上。所有后台 goroutine 都从这里出发，
// 免得漏登记 —— 测试里漏一个就是随机失败。
func (a *app) run(f func()) {
	a.bg.Add(1)
	go func() {
		defer a.bg.Done()
		f()
	}()
}

// chooseFolder 弹原生目录选择框。Dialog.Open 是阻塞调用，必须放到
// goroutine 里，否则会把主线程（也就是整个界面）卡住。
func (a *app) chooseFolder() {
	a.run(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     T("选择 Git 仓库", "Select Git Repository"),
			Directory: true,
			// 允许直接选 .app 这类包，否则用户没法选中仓库根目录
			TreatPackagesAsDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return // 用户取消不算错误
		}
		a.dispatch(func() { a.openInNewTab(paths[0]) })
	})
}

// openRepo 在后台读仓库，读完回主线程写入状态。
func (a *app) openRepo(path string) {
	if len(a.tabs) == 0 {
		a.tabs = []*repoTab{{path: filepath.Clean(path), name: filepath.Base(path), state: loadLoading}}
		a.activeTab = 0
	} else if cur := a.currentTab(); cur != nil {
		cur.path = filepath.Clean(path)
		cur.name = filepath.Base(path)
	}

	a.repoSeq++
	seq := a.repoSeq
	a.state = loadLoading
	a.errMsg = ""
	a.loadingPath = filepath.Clean(path)

	// 换了个仓库，上一个仓库的分支和「在看哪条历史」全部作废。刷新
	//（reload 也走这里，path 没变）时留着 —— 用户点过的分支不该被一次
	// 刷新弹掉。
	if !sameRepo(path, a.repoPath) {
		a.closeEmbeddedEditors()
		a.cancelAICommit()
		a.cancelAIBranch()
		a.exitPRReview()
		// 老仓库的提交草稿先落盘（draftFor 还是老仓库的根）。
		a.saveCommitDraft()
		a.refs = nil
		a.refByFull = nil
		a.refTree = nil
		a.refsList = ui.ListState{}
		a.refsClosed = nil
		a.refFilter = ""
		a.refSel = ""
		a.histRev = ""
		a.commitSel = -1
		a.commits = nil
		a.histFiles = nil
		a.histListID = ""
		a.hist = loadIdle
		a.histDiff = loadIdle
		a.ahead = 0
		a.behind = 0
		a.upstream = ""
	}

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, path)
		if err != nil {
			a.dispatch(func() {
				if a.repoSeq != seq {
					return
				}
				a.failOpen(path, err)
			})
			return
		}
		m, err := repo.Load(ctx)
		if err != nil {
			a.dispatch(func() {
				if a.repoSeq != seq {
					return
				}
				a.failOpen(path, err)
			})
			return
		}
		a.dispatch(func() {
			if a.repoSeq != seq {
				return // 已经有更新的一次加载了，丢弃这次的结果
			}
			a.applyModel(m)
		})
	})
}

// repoReady 是「仓库开好了吗」—— 工具栏、菜单、setTab、侧栏筛选框都问它。
//
// 只能问 state，不能问 repoPath：打开失败时 repoPath 也记着东西（试的是哪个
// 文件夹）。这条判据原来在四个地方各写了一份，改一处漏三处，就出现了「按钮
// 灰着、菜单却亮着」这种各说各话，所以收成一份。
//
// 重新读取同一个仓库（reload）时也算 Ready：手上已经有开好的仓库数据，
// 界面各区域（提交栏、分支筛选框、视图切换）继续正常展示旧状态，
// 绝不能在重读的几十毫秒内闪退成「未打开」再弹回来。
func (a *app) repoReady() bool {
	switch a.state {
	case loadReady:
		return true
	case loadLoading:
		return a.repoPath != "" && sameRepo(a.loadingPath, a.repoPath)
	}
	return false
}

// clamp 把 v 钳进 [lo, hi]。
func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// sameRepo 判断两个路径指向的是否是同一个 git 仓库目录。
// 能自动处理绝对/相对路径、末尾斜杠、以及符号链接（如 macOS /var 与 /private/var）。
func sameRepo(p1, p2 string) bool {
	if p1 == "" || p2 == "" {
		return false
	}
	c1 := filepath.Clean(p1)
	c2 := filepath.Clean(p2)
	if c1 == c2 {
		return true
	}
	abs1, err1 := filepath.Abs(c1)
	abs2, err2 := filepath.Abs(c2)
	if err1 == nil && err2 == nil {
		if abs1 == abs2 {
			return true
		}
		r1, err1 := filepath.EvalSymlinks(abs1)
		r2, err2 := filepath.EvalSymlinks(abs2)
		if err1 == nil && err2 == nil && r1 == r2 {
			return true
		}
	}
	return false
}

// toolbarOpVisible 决定是否在工具栏展示当前正在运行的写操作。
//
// 暂存/取消暂存是瞬时（几毫秒）的本地索引操作，用户焦点在勾选框或 hunk 按钮本身；
// 提交和修改上次提交在主区底部的提交栏有专属状态行与转圈；
// 这几项若在工具栏短暂插播文字，会导致工具栏水平尺寸剧烈拉扯抖动。
// 其余耗时操作（如网络交互的抓取/拉取/推送，或较重的变基/合并/检出）才在此展示。
func (a *app) toolbarOpVisible() bool {
	switch a.op {
	case gitrepo.OpNone, gitrepo.OpStage, gitrepo.OpUnstage, gitrepo.OpCommit, gitrepo.OpAmend, gitrepo.OpDiscard:
		return false
	default:
		return true
	}
}

// startOp 是唯一的写操作入口。有写操作在飞、或者仓库没开好时直接返回 false，
// 调用方什么都不做（界面本来就把那些控件画成灰的，这里只是最后一道兜底）。
//
// work 在后台跑，拿到的 repo 已经打开；返回一句成功提示（可以为空），出错就
// 返回 error。done 在主线程上、a.op 已经清掉之后调用，参数是这次成没成 ——
// 重载之类的收尾放在那里。失败会被翻成人话放进 a.opErr（顶部横幅）。
func (a *app) startOp(kind gitrepo.Op, name string, work func(context.Context, *gitrepo.Repo) (string, error), done func(ok bool)) bool {
	if a.op != gitrepo.OpNone || a.state != loadReady {
		return false
	}
	a.op = kind
	a.opName = name
	a.opErr = nil
	a.opNote = ""
	a.opSeq++
	seq := a.opSeq
	dir := a.repoPath

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, dir)
		var note string
		if err == nil {
			note, err = work(ctx, repo)
		}
		a.dispatch(func() {
			if a.opSeq != seq {
				return // 已经有更新的一次操作了，这次的结果丢弃
			}
			a.op = gitrepo.OpNone
			if err != nil {
				f := gitrepo.Describe(kind, err)
				a.opErr = &f
			} else {
				a.opNote = note
			}
			if done != nil {
				done(err == nil)
			}
		})
	})
	return true
}

// reloadAfterOp 是「完成之后整体重读」那一档收尾：暂存、提交、检出、
// 拉取、推送都动过 index 或工作区，两段 diff 全变了。
func (a *app) reloadAfterOp() {
	a.reload()
}

// saveCommitDraft 把提交框里的消息按仓库存起来。空消息等于删掉这条。
func (a *app) saveCommitDraft() {
	if a.draftFor == "" {
		return
	}
	a.store.SetDraft(a.draftFor, a.commitMsg)
	a.persist()
}

// failOpen 记下一次「仓库没打开」。只在主线程调用。
//
// 记下试的是哪个文件夹：卡片上说「这个文件夹不是 git 仓库」，屏幕上总得有
// 个「这个」—— 标题栏把它摆在和打开成功时同一个位置，用户一眼就能看出自己
// 挑错的是哪一层（选了 ~/Desktop 而不是 ~/Desktop/proj）。
//
// 但不写进 lastRepo（那在 applyModel 里）：坏路径不该被记住，不然下次启动
// 又自己撞一遍。
func (a *app) failOpen(path string, err error) {
	a.state = loadFailed
	a.errMsg = err.Error()
	a.errNotRepo = errors.Is(err, gitrepo.ErrNotRepo)
	// 上一份状态条（如果还挂着）说的是上一个仓库的事，清掉。不清的话，
	// 从一个卡在合并中间的仓库切到一个坏路径，错误卡片上头会飘着一条
	// 属于别人的「合并停在冲突上」。
	a.pending = gitrepo.Pending{}
	if path != "" {
		a.repoPath = filepath.Clean(path)
		a.repoName = filepath.Base(a.repoPath)
	}
	// 打开的不是 git 仓库，没什么可自动刷新的；顺手把轮询停在该缩着的
	// 状态（reload 需要的 repoPath 字段还留着给错误卡片指代用）。
	a.setAutoWatch("")
}

// applyModel 把加载结果写进 app。只在主线程调用。
func (a *app) applyModel(m *gitrepo.Model) {
	a.repoPath = m.Repo.Root
	a.repoName = m.Repo.Name
	a.branch = m.Branch
	a.detached = m.Detached
	a.remotes = m.Remotes
	a.pending = m.Pending
	a.hasHead = m.HasHead
	a.ahead = m.Ahead
	a.behind = m.Behind
	a.upstream = m.Upstream
	a.amend = false

	// 换到另一个仓库（或者第一次打开）时把这个仓库存下的提交草稿拾回来。
	// 刷新同一个仓库不动它 —— 正在打的字不该被一次刷新吃掉。
	if a.draftFor != m.Repo.Root {
		a.draftFor = m.Repo.Root
		a.commitMsg = a.store.State().Drafts[m.Repo.Root]
	}

	a.workFiles = m.Files

	// 记住这个仓库，下次启动直接打开
	a.store.SetLastRepo(a.repoPath)
	if cur := a.currentTab(); cur != nil {
		cur.path = a.repoPath
		cur.name = a.repoName
		cur.branch = a.branch
		cur.detached = a.detached
		cur.dirtyCount = len(m.Files)
	}
	a.persistOpenRepos()

	// 正在看历史时别把主区切回工作区 —— 用户下一次刷新可能只是想
	// 更新一下提交列表。
	if a.tab == 0 {
		a.showFiles(a.workFiles, workListID(m.Repo.Root, a.workBranch()))
	} else {
		a.loadCommits()
	}
	a.state = loadReady
	a.loadRefs()

	// 这次加载完了，把自动刷新对齐到这份新状态上：外部改动了仓库才值得
	// 再刷一次。裸 ⌘R、写操作后的重读也都走这里，baseline 跟着归位。
	a.setAutoWatch(m.Repo.Root)
	a.syncEditorPane()
}

// workBranch 是「改动」列表该认的分支名。
//
// 变基期间 git 的 HEAD 是分离的，m.Branch 只是个短哈希 —— 拿它当列表身份
// 的话，每重放一步就换一个（每步 continue 都是一次整体重读），用户展开着
// 的未改动区间会被一路重置。用正在变基的那条分支名，整个变基过程里 id 是
// 同一个。（从分离态发起的 --onto 变基没有分支名，退回短哈希，和以前一样。）
func (a *app) workBranch() string {
	if a.pending.Kind == gitrepo.PendingRebase && a.pending.Branch != "" {
		return a.pending.Branch
	}
	return a.branch
}

// modalOn 说「有对话框开着」。菜单命令和视图快捷键都要在模态下让位 ——
// 菜单在系统层拦键、视图在自己那一帧拦，各写各的判断迟早漏掉一处
// （新加一个对话框，忘了改其中一个，⌘R 就会在对话框背后偷偷刷新）。
func (a *app) modalOn() bool {
	return a.codeEdit.on || a.palette.on || a.settingsOn || a.tag.on || a.reset.on || a.newBranch.on || a.rename.on || a.confirm.on || a.stashDlg.on || a.compare.dialog || a.prDlg.on || a.prReview.reviewModal || a.riskScan.dialog || a.fileHist.on
}

// toggleAmend 切换修改上次提交状态。勾选时预填最近一次提交的说明。
func (a *app) toggleAmend(on bool) {
	a.amend = on
	if on {
		if a.repoReady() {
			a.run(func() {
				repo, err := gitrepo.Open(context.Background(), a.repoPath)
				if err == nil {
					msg, err := repo.LastCommitMessage(context.Background())
					if err == nil {
						a.dispatch(func() {
							if a.amend {
								a.commitMsg = msg
							}
						})
					}
				}
			})
		}
	} else {
		a.commitMsg = a.store.State().Drafts[a.repoPath]
	}
}

// loadRefs 在后台读分支、远程分支、标签和储藏。
//
// 单独一路，不并进 openRepo 那个请求：读引用失败不该让整个仓库变成
// 「打不开」—— 分支树空着，diff 照样能看。
func (a *app) loadRefs() {
	if a.repoPath == "" {
		return
	}
	if len(a.refs) == 0 {
		a.refsState = loadLoading
		a.refErr = ""
	}

	a.refsSeq++
	seq := a.refsSeq
	dir := a.repoPath
	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var refs []gitrepo.Ref
		var ahead, behind int
		if err == nil {
			refs, err = repo.Refs(context.Background())
			ahead, behind, _ = repo.UpstreamCounts(context.Background())
		}
		a.dispatch(func() {
			if a.refsSeq != seq {
				return
			}
			if err != nil {
				a.refsState = loadFailed
				a.refErr = err.Error()
				return
			}
			a.ahead = ahead
			a.behind = behind
			a.setRefs(refs)
			a.refsState = loadReady
		})
	})
}

// setRefs 换掉引用列表，并重建侧栏的树。
//
// 顺带按 Full 建一张索引：树节点里只存 key，点一行时得靠它找回是哪个引用。
func (a *app) setRefs(refs []gitrepo.Ref) {
	if slices.Equal(a.refs, refs) {
		return
	}
	a.refs = refs
	a.refByFull = make(map[string]*gitrepo.Ref, len(refs))
	for i := range refs {
		a.refByFull[refs[i].Full] = &refs[i]
	}
	a.rebuildRefTree()
}

// showFiles 把一份文件列表换成当前显示的内容：重新筛选、重新建树。
// 工作区和历史提交共用这一条路。
//
// id 说明这份列表是谁的（"w:仓库路径" / "c:提交" / "k:储藏"）。换了 id 就是
// 换了个对象：筛选词和选中的文件都作废。同一个 id 是同一个对象的刷新，选中
// 的文件跟着留下 —— 暂存一段改动之后，光标不该跳回第一个文件。
func (a *app) showFiles(files []gitrepo.File, id string) {
	if a.listID != id {
		a.fileFilter = ""
		a.fileTreeFilter = ""
		a.fileSel = -1
		a.listID = id
	}
	a.allFiles = files
	a.applyFilter()
	a.pruneReviewDrafts(files)
}

// selectedFileKey 是文件栏当前选中那个文件的 key，没选中给空串。
func (a *app) selectedFileKey() string {
	if a.fileSel < 0 || a.fileSel >= len(a.files) {
		return ""
	}
	return fileKey(a.files[a.fileSel])
}

// selectFileKey 把文件栏的选中挪到某个 key 上（找不到就退到第一个）。
//
// 筛选、刷新、暂存都会挪动下标，而 key 不会 —— 所以「还是看这个文件」
// 这件事只能用 key 表达。
func (a *app) selectFileKey(key string) {
	a.fileSel = -1
	if key == "" {
		return
	}
	for i := range a.files {
		if fileKey(a.files[i]) == key {
			a.fileSel = i
			return
		}
	}
}

// applyFilter 依据筛选词算出要显示的文件，并同步文件树和选中的文件。
//
// 筛掉的文件连 diff 一起不显示，而不是只从文件列表里藏起来 —— 筛选的
// 用途是「只看这块子系统」，右边还画着别的文件会让人对不上。
func (a *app) applyFilter() {
	prev := a.selectedFileKey()
	a.files = filterFiles(a.allFiles, a.fileFilter)
	a.rebuildTree()
	a.selectFileKey(prev)
}

// filterFiles 按筛选词挑出要显示的文件。
//
// 筛掉的文件连 diff 一起不显示，而不是只从文件列表里藏起来 —— 筛选的
// 用途是「只看这块子系统」，diff 里还留着别的文件会让人对不上。
// 匹配的是纯路径（pathOf），不是 fileKey —— 后者带「s:/u:」段落前缀，
// 搜一个 "s" 会把整段已暂存的文件全捞进来。
func filterFiles(files []gitrepo.File, query string) []gitrepo.File {
	if query == "" {
		return files
	}
	out := make([]gitrepo.File, 0, len(files))
	for _, f := range files {
		if filetree.FuzzyMatch(pathOf(f), query) {
			out = append(out, f)
		}
	}
	return out
}

// reload 重新读取当前仓库。
func (a *app) reload() {
	if a.repoPath == "" {
		return
	}
	a.openRepo(a.repoPath)
}

// rebuildTree 依据当前的文件列表和筛选词重建文件列表那一栏的树。
//
// 已暂存是 a.files 的连续前缀（Load 保证）。两段都有文件时各建一棵树、
// 各挂一个分组行；只有一段时不分组建树 —— 单一段落再套一层分组，所有
// 文件名白白往右缩一格，什么信息都没多。分组 key 用 g:s / g:u，和侧栏
// refs 树的 g: 前缀各管各的（两棵树本来就不会同时出现在一个列表里）。
//
// 展开/收起的状态不用在这里处理：filesClosed 里记的是「用户收起过哪些」，
// 没记过的一律展开（见 filesClosed 的说明）。
func (a *app) rebuildTree() {
	a.fileTreeFilter = a.fileFilter

	stagedN := 0
	for stagedN < len(a.files) && a.files[stagedN].Section == gitrepo.SectionStaged {
		stagedN++
	}
	var roots []*filetree.Node
	switch {
	case stagedN == 0:
		roots = filetree.BuildAt(a.files, 0, "")
	case stagedN == len(a.files):
		roots = filetree.BuildAt(a.files, 0, "")
	default:
		roots = []*filetree.Node{
			{Key: "g:s", Label: T("已暂存", "Staged"), Path: "已暂存", Dir: true, FileIdx: -1,
				Children: filetree.BuildAt(a.files[:stagedN], 0, "s|")},
			{Key: "g:u", Label: T("未暂存", "Unstaged"), Path: "未暂存", Dir: true, FileIdx: -1,
				Children: filetree.BuildAt(a.files[stagedN:], stagedN, "u|")},
		}
	}
	a.tree = filetree.Filter(roots, a.fileFilter)
}

// isViewed 判断文件是否已被标记为看过。指纹对不上就说明文件内容变了，
// 之前的标记自动作废。
//
// key 用 fileKey（含段落前缀）：同一个路径在「已暂存 / 未暂存」是两份不同
// 的 diff，也是两次不同的 review。升级前按纯路径存的旧标记就此失效 ——
// 无害，它们只是再也不会被命中。
func (a *app) isViewed(f gitrepo.File) bool {
	return a.viewed[fileKey(f)] == f.Fingerprint && f.Fingerprint != ""
}

func (a *app) toggleViewed(f gitrepo.File) {
	if a.isViewed(f) {
		delete(a.viewed, fileKey(f))
	} else {
		a.viewed[fileKey(f)] = f.Fingerprint
	}
	a.store.SetViewed(a.viewed)
	a.persist()
}
