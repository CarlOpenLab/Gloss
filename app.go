package main

import (
	"context"
	"sync"

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

	// —— 主区文件栏（原来的侧栏文件树）——
	tree           []*filetree.Node
	filesList      ui.ListState
	sidebarOn      bool
	fileFilter     string
	fileTreeFilter string // 建树时用的筛选词，和 fileFilter 不一致就说明该重建了
	// filesClosed 记的是用户收起过的目录，同 refsClosed：默认展开。
	// review 改动时几乎总想直接看到文件列表，默认收起反而要先点一下。
	filesClosed map[string]bool
	// fileSel 是文件栏里选中的文件下标（不是行下标 —— 行下标一筛选就指错）。
	fileSel int
	// pendingScrollFile 是「点了文件栏，diff 该滚到哪个文件」的一次性请求。
	// 存文件下标而不是行下标：发起点击时行模型可能还是脏的，行下标会指错。
	pendingScrollFile int

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
	// histSplit 是提交列表那一栏的高度，SplitVertical 拖的就是它。
	// splitH 是上一帧量到的可用高度，用来在窗口变矮时把 split 收敛回来。
	histSplit float32
	splitH    float32
	// hist 是提交列表本身的状态，histDiff 是选中那个提交的 diff 的状态。
	// 必须分开：列表和 diff 是两路异步加载，共用一个的话点一个提交会让
	// 整个列表闪成「正在读提交…」。
	hist        loadState
	histErr     string
	histDiff    loadState
	histDiffErr string

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
		sidebarOn:         true,
		viewed:            map[string]string{},
		folded:            map[string]bool{},
		commitSel:         -1,
		fileSel:           -1,
		pendingScrollFile: -1,
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

// toolbarHeight 是工具栏那条带子的高度。toolbar 用的是同一个公式 ——
// 分成两处算的话，主区的高度就会和实际差几个像素，分隔条的夹逼跟着错。
func (a *app) toolbarHeight(c *ui.Context) float32 {
	return max(c.TitleBar().Height, 52)
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

	ui.Row(c).Height(a.toolbarHeight(c)).Shrink(0).
		Padding(0, bar.Right+12, 0, padLeft).
		AlignItems(ui.Center).Gap(10).
		DragWindow().
		Background(pal.appBg).
		Border(0, pal.border).
		Children(func() {
			ui.Column(c).Gap(1).MinWidth(0).Children(func() {
				name := a.repoName
				if name == "" {
					name = "Gloss"
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
			case a.tab == 1 && (a.hist == loadLoading || a.histDiff == loadLoading):
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

// mainArea 是右侧主区。两个 tab 的下半区是同一样东西 ——「文件列表 + diff」，
// 见 detailPane；历史 tab 在那个上面还摞了一栏提交列表。
func (a *app) mainArea(c *ui.Context, pal palette) {
	if a.tab == 1 {
		a.historyArea(c, pal)
		return
	}
	a.detailPane(c, pal)
}

// historyEmpty 是历史 tab 主区没有 diff 可画时的几种情况。
//
// 先问列表的状态、再问这个提交的 diff 的状态 —— 两者是两路独立的加载，
// 列表没读出来时根本谈不上「选中了哪个提交」。
func (a *app) historyEmpty(c *ui.Context, t *ui.Theme) {
	switch {
	case a.hist == loadLoading:
		ui.Spinner(c)
		ui.Text(c, "正在读提交…").TextColor(t.TextMuted)
	case a.hist == loadFailed:
		ui.Text(c, "读不到历史").FontSize(18).Bold()
		ui.Text(c, a.histErr).TextColor(t.Danger)
		if ui.Button(c, "重试").Clicked() {
			a.loadCommits()
		}
	case len(a.commits) == 0:
		ui.Text(c, "还没有提交").FontSize(18).Bold()
		ui.Text(c, "这个仓库还没有任何历史。").TextColor(t.TextMuted)
	case a.histDiff == loadLoading:
		ui.Spinner(c)
		ui.Text(c, "正在读这个提交…").TextColor(t.TextMuted)
	case a.histDiff == loadFailed:
		ui.Text(c, "读不到这个提交").FontSize(18).Bold()
		ui.Text(c, a.histDiffErr).TextColor(t.Danger)
		if ui.Button(c, "重试").Clicked() {
			a.loadCommit(a.commitSel)
		}
	case a.commitSel >= 0 && a.commitSel < len(a.commits):
		cm := a.commits[a.commitSel]
		ui.Text(c, cm.Subject).FontSize(18).Bold()
		ui.Textf(c, "%s · %s · %s", cm.Short, cm.Author, cm.When.Format("2006-01-02 15:04")).
			TextColor(t.TextMuted)
		ui.Text(c, "这个提交没有改动内容。").TextColor(t.TextMuted)
	default:
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

	// 换了个仓库，上一个仓库的分支和「在看哪条历史」全部作废。刷新
	//（reload 也走这里，path 没变）时留着 —— 用户点过的分支不该被一次
	// 刷新弹掉。
	if path != a.repoPath {
		a.refs = nil
		a.refByFull = nil
		a.refTree = nil
		a.refsList = ui.ListState{}
		a.refsClosed = nil
		a.refFilter = ""
		a.refSel = ""
		a.histRev = ""
		a.commitSel = -1
	}

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
	a.loadRefs()
}

// loadRefs 在后台读分支、远程分支、标签和储藏。
//
// 单独一路，不并进 openRepo 那个请求：读引用失败不该让整个仓库变成
// 「打不开」—— 分支树空着，diff 照样能看。
func (a *app) loadRefs() {
	if a.repoPath == "" {
		return
	}
	a.refsState = loadLoading
	a.refErr = ""

	a.refsSeq++
	seq := a.refsSeq
	dir := a.repoPath
	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var refs []gitrepo.Ref
		if err == nil {
			refs, err = repo.Refs(context.Background())
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
			a.setRefs(refs)
			a.refsState = loadReady
		})
	})
}

// setRefs 换掉引用列表，并重建侧栏的树。
//
// 顺带按 Full 建一张索引：树节点里只存 key，点一行时得靠它找回是哪个引用。
func (a *app) setRefs(refs []gitrepo.Ref) {
	a.refs = refs
	a.refByFull = make(map[string]*gitrepo.Ref, len(refs))
	for i := range refs {
		a.refByFull[refs[i].Full] = &refs[i]
	}
	a.rebuildRefTree()
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
	// 选中的文件和「该滚到哪」都是下标，换一份列表就指到别的文件上了
	a.fileSel = -1
	a.pendingScrollFile = -1

	a.allFiles = files
	a.applyFilter()
	a.loadContents(a.contentSeq)
}

// applyFilter 依据筛选词算出要显示的文件，并同步树和 diff 行模型。
func (a *app) applyFilter() {
	a.files = filterFiles(a.allFiles, a.fileFilter)
	a.rebuildTree()
	a.rowsDirty = true
}

// filterFiles 按筛选词挑出要显示的文件。
//
// 筛掉的文件连 diff 一起不显示，而不是只从文件列表里藏起来 —— 筛选的
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

// rebuildTree 依据当前的文件列表和筛选词重建文件列表那一栏的树。
//
// 展开/收起的状态不用在这里处理：filesClosed 里记的是「用户收起过哪些」，
// 没记过的一律展开（见 filesClosed 的说明）。原来那套 seenDirs 是为了绕开
// 「Outline 的目录默认收起」，换成 ui.List 之后这个默认值本身就对了。
func (a *app) rebuildTree() {
	a.fileTreeFilter = a.fileFilter
	a.tree = filetree.Filter(filetree.Build(a.files), a.fileFilter)
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
