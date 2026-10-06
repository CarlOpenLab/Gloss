package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/filetree"
	"diff-explainer/internal/gitrepo"
	"diff-explainer/internal/store"
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

	// —— 仓库 ——
	repoPath string
	repoName string
	branch   string
	state    loadState
	errMsg   string
	// 四个序号各自守住一路异步加载：每次发起自增，回调只在序号仍是最新的
	// 时候才写入结果，这样快速切换时旧请求的结果不会覆盖新的。
	//
	// 分开是必须的：早期只有一个 loadSeq，于是「读提交列表」和「读这个提交
	// 的 diff」会互相撞掉 —— 第二次进历史 tab 时，后发的那个把先发的序号
	// 顶掉，提交列表就永远停在旧数据上（新提交看不见）。
	repoSeq    int // openRepo：整个仓库
	logSeq     int // loadCommits：提交列表
	diffSeq    int // loadCommit：选中提交的 diff
	contentSeq int // showFiles/loadContents：文件内容与语法高亮

	// —— 模型 ——
	// allFiles 是当前显示的完整文件列表（工作区改动，或某个提交的 diff）；
	// files 是过了筛选词之后、界面真正显示的那一份。侧栏树、diff 行模型、
	// 统计都只看 files，三者才对得上。
	allFiles []gitrepo.File
	files    []gitrepo.File
	// workFiles 是工作区的文件列表。切到历史看某个提交时 allFiles 会换成
	// 那个提交的 diff，切回来要还原，所以单独留一份。
	workFiles []gitrepo.File
	// contents 按路径缓存文件在新侧的内容，用于展开未改动区间和语法高亮。
	// 用路径而不是下标索引：筛选会让文件列表变短，下标会错位。
	contents map[string]*fileContent

	// —— 派生的行模型 ——
	// rows 是喂给虚拟化列表的扁平行数组。它只在 rowsDirty 时重建 ——
	// 每帧重建一个大 diff 的几万行会白白烧 CPU。
	rows      []row
	rowsDirty bool
	diffList  ui.ListState
	// built 是上一帧构建了多少行。列表是虚拟化的，这个数只跟窗口高度有关，
	// 和 diff 有多长无关 —— 测试靠它钉住这一点（万一哪天不小心把整份 diff
	// 都建了出来，几万行的仓库会立刻卡死）。
	built int

	// —— 展开状态 ——
	// expanded 记录每个未改动区间展开了多少行，key 见 rows.go 的 gapKey。
	// 跨帧保留，所以刷新后展开状态还在（文件内容变了才会对不上）。
	expanded map[string]int

	// —— J/K 跳 hunk ——
	// hunkCur 是上一次跳到的那一段的 key，见 keys.go 的 hunkAnchor。
	hunkCur string

	// —— 视图 ——
	split    bool    // true = split（左右对照），false = unified
	codeSize float32 // 代码字号；0 表示用默认值

	// —— 侧栏 ——
	tree       []*filetree.Node
	treeByKey  map[string]*filetree.Node
	outline    ui.OutlineState[string]
	sidebarOn  bool
	filter     string
	treeFilter string // 建树时用的筛选词，和 filter 不一致就说明该重建了
	// seenDirs 记住出现过的目录 key。Outline 的目录默认是收起的，我们想让
	// 新目录默认展开；靠这个集合区分「第一次出现的目录」和「用户手动收起的
	// 目录」，否则每次筛选/刷新都会把用户收起的目录又弹开。
	seenDirs map[string]bool

	// —— 已 review：文件路径 -> 当时的内容指纹 ——
	// 指纹对不上就说明文件又变了，标记自动失效。
	viewed map[string]string

	// folded 是「折叠/展开」的手动覆盖：没记过的文件跟着 viewed 走
	// （标了已 review 就折起来），点过文件头的按点的来。
	// 用 map 的存在性区分「没手动设过」和「手动设成 false」。
	folded map[string]bool

	// —— 历史 ——
	tab        int // 0 = 改动，1 = 历史
	commits    []gitrepo.Commit
	commitList ui.ListState
	commitSel  int // 选中的提交下标，-1 表示还没选
	hist       loadState
	histErr    string

	// —— AI 解释 ——
	store *store.Store
	// modelName 是当前生效的模型名，解释缓存的 key 里带它。它在设置变化时
	// 更新一次，而不是每帧去问 store —— 那要加锁还可能要读环境变量。
	modelName string
	ex        explainState
	// settingsOn 是「模型设置」对话框是否打开；editCfg 是编辑中的副本，
	// 点确定之前不动真配置。
	settingsOn bool
	editCfg    store.Config
	editIdx    int

	// —— 命令栏（⌘K）——
	palette paletteState
	// focusFilter 是 ⌘F 的一次性请求：下帧把焦点放进筛选框。
	focusFilter bool
}

func newApp() *app {
	a := &app{
		sidebarOn: true,
		viewed:    map[string]string{},
		folded:    map[string]bool{},
		commitSel: -1,
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
	a.split = cfg.Split
	a.sidebarOn = !cfg.SidebarOff
	a.viewed = s.State().Viewed
	if a.viewed == nil {
		a.viewed = map[string]string{}
	}
	a.editIdx = cfg.Active
	a.modelName = s.Provider().EffectiveModel()
}

// persist 把设置、状态和缓存落盘。写文件不占主线程，扔到后台去。
func (a *app) persist() {
	s := a.store
	a.run(func() { _ = s.Save() })
}

// saveViewSettings 记下跟界面有关的几项设置（视图模式、侧栏、字号）。
//
// 只覆盖这几项，别的字段原样留着 —— 模型设置的保存走 applySettings，
// 两边各管各的，不会互相冲掉。
func (a *app) saveViewSettings() {
	cfg := a.store.Config()
	cfg.Split = a.split
	cfg.SidebarOff = !a.sidebarOn
	cfg.CodeSize = a.codeSize
	a.store.SetConfig(cfg)
	a.persist()
}

func (a *app) view(c *ui.Context) {
	pal := paletteFor(c.Theme())
	// 快捷键要在最前面处理：c.Shortcut 是「注册即认领」的，注册得越早
	// 越先拿到键。
	a.handleKeys(c)

	// 窗口开了 vibrancy：根节点不铺底色，让系统的毛玻璃材质透上来；
	// 只有主区不透明，保证代码区域的对比度稳定、不受壁纸颜色影响。
	c.Root().Background(ui.Transparent)

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if a.sidebarOn {
			a.sidebar(c, pal)
		}
		ui.Column(c).Grow(1).MinWidth(0).Background(pal.appBg).Children(func() {
			a.toolbar(c, pal)
			a.mainArea(c, pal)
		})
	})
	a.paletteDialog(c, pal)
	a.settingsDialog(c, pal)

	// 卡片画在最后：这一帧的 ✨ 按钮位置刚写好，卡片当帧就能摆对地方。
	// 它在 Overlay 层，不受上面的布局影响。
	a.explainCard(c, pal)

	// Esc 关掉卡片。放在最后，让卡片里正在编辑的输入框先有机会处理它。
	// 命令栏和设置对话框开着的时候它们自己会处理 Esc，这里让位。
	if a.explainOpen() && !a.palette.on && !a.settingsOn && c.Shortcut(0, ui.KeyEscape) {
		a.closeExplain()
	}
}

// toolbar 是自绘的标题栏：窗口用 TitleBarHidden，标题栏由 view 画出来，
// 红绿灯浮在上面。高度取 max(bar.Height, 52)：窗口那边用 TrafficLightPosition
// 把红绿灯摆在 52 高的带子正中间，算出来的 bar.Height 正好也是 52，于是
// 红绿灯和这里的按钮落在同一条水平线上。
//
// 水平方向要不要让位，取决于侧栏开没开：侧栏在的时候它顶在最左边，红绿灯
// 落在它头上（sidebar 里留了位），工具栏左边就按普通间距来；侧栏收起来了
// 工具栏才直接贴着窗口左边缘，这时得把 bar.Left 让出来。
func (a *app) toolbar(c *ui.Context, pal palette) {
	t := c.Theme()
	bar := c.TitleBar()

	padLeft := float32(12)
	if !a.sidebarOn {
		padLeft = bar.Left + 12
	}

	ui.Row(c).Height(max(bar.Height, 52)).Shrink(0).
		Padding(0, bar.Right+12, 0, padLeft).
		AlignItems(ui.Center).Gap(10).
		DragWindow().
		Background(pal.appBg).
		Border(0, pal.border).
		Children(func() {
			ui.Column(c).Gap(1).MinWidth(0).Children(func() {
				name := a.repoName
				if name == "" {
					name = "Diff Explainer"
				}
				ui.Text(c, name).Bold().SingleLine()
				sub := "未打开仓库"
				if a.repoPath != "" {
					sub = a.repoPath
				}
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})

			if a.branch != "" {
				ui.Text(c, a.branch).FontSize(11).TextColor(pal.ref).
					Padding(2, 8).Radius(4).Background(t.Surface)
			}

			ui.Spacer(c)

			// 改动 / 历史。⌘1 ⌘2 也能切。
			tab := a.tab
			ui.Segmented(c, &tab, "改动", "历史")
			if tab != a.tab {
				a.setTab(tab)
			}

			// unified / split 切换。和 Checkbox 一样传副本进去、变了再回写：
			// 控件要的是 *int，而模式存在 app 上。
			split := a.split
			mode := 0
			if split {
				mode = 1
			}
			ui.Segmented(c, &mode, "Unified", "Split")
			if (mode == 1) != split {
				a.split = mode == 1
				a.rowsDirty = true
				a.saveViewSettings()
			}

			switch {
			case a.state == loadLoading:
				ui.Spinner(c)
			case a.state == loadReady && len(a.files) > 0:
				// 两个 tab 都报当前主区里的文件数：改动是工作区的，
				// 历史是选中那个提交的
				ui.Textf(c, "%d 个文件", len(a.files)).FontSize(12).TextColor(t.TextMuted)
			case a.tab == 1 && a.hist == loadLoading:
				ui.Spinner(c)
			}

			if ui.Button(c, "刷新").Clicked() {
				a.reload()
			}
			if ui.Button(c, "模型设置…").Clicked() {
				a.openSettings()
			}
			if ui.Button(c, "打开仓库…").Clicked() {
				a.chooseFolder()
			}
		})
}

// mainArea 是右侧主区：仓库就绪就画 diff，否则画空/加载/出错状态。
func (a *app) mainArea(c *ui.Context, pal palette) {
	t := c.Theme()

	if a.state == loadReady && len(a.files) > 0 {
		a.diffView(c, pal)
		return
	}

	ui.Box(c).Grow(1).FillWidth().Background(pal.appBg).Children(func() {
		ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
			switch {
			case a.tab == 1:
				a.historyEmpty(c, t)

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
			case a.filter != "":
				ui.Text(c, "没有匹配的文件").FontSize(18).Bold()
				ui.Textf(c, "筛选词「%s」没有匹配到任何改动。", a.filter).
					TextColor(t.TextMuted)
			default:
				ui.Text(c, "没有未提交的改动").FontSize(18).Bold()
				ui.Text(c, "工作区是干净的。").TextColor(t.TextMuted)
			}
		})
	})
}

// historyEmpty 是历史 tab 主区没有 diff 可画时的几种情况。
func (a *app) historyEmpty(c *ui.Context, t *ui.Theme) {
	switch a.hist {
	case loadLoading:
		ui.Spinner(c)
		ui.Text(c, "正在读提交…").TextColor(t.TextMuted)
	case loadFailed:
		ui.Text(c, "读不到历史").FontSize(18).Bold()
		ui.Text(c, a.histErr).TextColor(t.Danger)
		if ui.Button(c, "重试").Clicked() {
			a.loadCommits()
		}
	default:
		if len(a.commits) == 0 {
			ui.Text(c, "还没有提交").FontSize(18).Bold()
			ui.Text(c, "这个仓库还没有任何历史。").TextColor(t.TextMuted)
			return
		}
		if a.commitSel >= 0 && a.commitSel < len(a.commits) {
			cm := a.commits[a.commitSel]
			ui.Text(c, cm.Subject).FontSize(18).Bold()
			ui.Textf(c, "%s · %s · %s", cm.Short, cm.Author, cm.When.Format("2006-01-02 15:04")).
				TextColor(t.TextMuted)
			ui.Text(c, "这个提交没有改动内容。").TextColor(t.TextMuted)
			return
		}
		ui.Text(c, "选一个提交").FontSize(18).Bold()
		ui.Text(c, "左边挑一个提交，看看它改了什么。").TextColor(t.TextMuted)
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
			Title:     "选择 git 仓库",
			Directory: true,
			// 允许直接选 .app 这类包，否则用户没法选中仓库根目录
			TreatPackagesAsDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return // 用户取消不算错误
		}
		a.dispatch(func() { a.openRepo(paths[0]) })
	})
}

// openRepo 在后台读仓库，读完回主线程写入状态。
func (a *app) openRepo(path string) {
	a.repoSeq++
	seq := a.repoSeq
	a.state = loadLoading
	a.errMsg = ""

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, path)
		if err != nil {
			a.dispatch(func() {
				if a.repoSeq != seq {
					return
				}
				a.state = loadFailed
				a.errMsg = err.Error()
			})
			return
		}
		m, err := repo.Load(ctx)
		if err != nil {
			a.dispatch(func() {
				if a.repoSeq != seq {
					return
				}
				a.state = loadFailed
				a.errMsg = err.Error()
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

// applyModel 把加载结果写进 app。只在主线程调用。
func (a *app) applyModel(m *gitrepo.Model) {
	a.repoPath = m.Repo.Root
	a.repoName = m.Repo.Name
	a.branch = m.Branch
	a.workFiles = m.Files

	// 记住这个仓库，下次启动直接打开
	a.store.SetLastRepo(a.repoPath)

	// 正在看历史时别把主区切回工作区 —— 用户下一次刷新可能只是想
	// 更新一下提交列表。
	if a.tab == 0 {
		a.showFiles(a.workFiles)
	} else {
		a.loadCommits()
	}
	a.state = loadReady
}

// showFiles 把一份文件列表换成当前显示的内容：重置展开状态和缓存内容，
// 重新筛选、重新读内容。工作区和历史提交共用这一条路。
func (a *app) showFiles(files []gitrepo.File) {
	// 上一个列表的异步结果（内容加载）从此作废
	a.contentSeq++
	// 文件列表换了，展开状态和已读内容都不再对应得上
	a.expanded = nil
	a.contents = map[string]*fileContent{}
	// 记下的跳转位置同样作废：同样的 key 可能落在完全不同的行上
	a.hunkCur = ""
	// 卡片指向的是「第几个文件的第几段」，同样作废
	a.closeExplain()

	a.allFiles = files
	a.applyFilter()
	a.loadContents(a.contentSeq)
}

// applyFilter 依据筛选词算出要显示的文件，并同步树和 diff 行模型。
func (a *app) applyFilter() {
	a.files = filterFiles(a.allFiles, a.filter)
	a.rebuildTree()
	a.rowsDirty = true
}

// filterFiles 按筛选词挑出要显示的文件。
//
// 筛掉的文件连 diff 一起不显示，而不是只从侧栏树里藏起来 —— 筛选的
// 用途是「只看这块子系统」，diff 里还留着别的文件会让人对不上。
func filterFiles(files []gitrepo.File, query string) []gitrepo.File {
	if query == "" {
		return files
	}
	out := make([]gitrepo.File, 0, len(files))
	for _, f := range files {
		if filetree.FuzzyMatch(fileKey(f), query) {
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

// rebuildTree 依据当前的文件列表和筛选词重建侧栏树。
func (a *app) rebuildTree() {
	a.treeFilter = a.filter

	tree := filetree.Build(a.files)
	tree = filetree.Filter(tree, a.filter)

	a.tree = tree
	a.treeByKey = make(map[string]*filetree.Node)
	var index func([]*filetree.Node)
	index = func(ns []*filetree.Node) {
		for _, n := range ns {
			a.treeByKey[n.Key] = n
			index(n.Children)
		}
	}
	index(tree)

	// 新出现的目录默认展开 —— review 改动时几乎总想直接看到文件列表。
	// 只展开第一次见到的，用户手动收起的不再打扰。
	if a.seenDirs == nil {
		a.seenDirs = map[string]bool{}
	}
	var openNew func([]*filetree.Node)
	openNew = func(ns []*filetree.Node) {
		for _, n := range ns {
			if !n.Dir {
				continue
			}
			if !a.seenDirs[n.Key] {
				a.seenDirs[n.Key] = true
				a.outline.Open.Add(n.Key)
			}
			openNew(n.Children)
		}
	}
	openNew(tree)
}

// childKeys 是给 ui.Outline 的 children 回调：返回某个目录节点的子节点 key。
//
// 叶子节点必须返回 nil 而不是空切片 —— Outline 用 `kids != nil` 判断一个
// 条目是不是分支，返回空切片会让文件也画上展开三角。
func (a *app) childKeys(key string) []string {
	n, ok := a.treeByKey[key]
	if !ok || len(n.Children) == 0 {
		return nil
	}
	out := make([]string, 0, len(n.Children))
	for _, k := range n.Children {
		out = append(out, k.Key)
	}
	return out
}

// isViewed 判断文件是否已被标记为看过。指纹对不上就说明文件内容变了，
// 之前的标记自动作废。
func (a *app) isViewed(f gitrepo.File) bool {
	return a.viewed[f.NewPath] == f.Fingerprint && f.Fingerprint != ""
}

func (a *app) toggleViewed(f gitrepo.File) {
	if a.isViewed(f) {
		delete(a.viewed, f.NewPath)
	} else {
		a.viewed[f.NewPath] = f.Fingerprint
	}
	// 标记变了，之前手动折叠/展开的覆盖就不作数了 ——
	// 「勾上已 review 就折起来」这条规则要立刻生效。
	delete(a.folded, fileKey(f))
	a.rowsDirty = true
	a.store.SetViewed(a.viewed)
	a.persist()
}

// isFolded 判断一个文件在 diff 里是不是折着的。默认跟着「已 review」走，
// 点过文件头的以手动设置为准。
func (a *app) isFolded(f gitrepo.File) bool {
	if v, ok := a.folded[fileKey(f)]; ok {
		return v
	}
	return a.isViewed(f)
}

// toggleFold 是点文件头时调用的：折起来 / 展开。
func (a *app) toggleFold(f gitrepo.File) {
	if a.folded == nil {
		a.folded = map[string]bool{}
	}
	a.folded[fileKey(f)] = !a.isFolded(f)
	a.rowsDirty = true
}
