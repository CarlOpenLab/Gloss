package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/filetree"
	"diff-explainer/internal/gitrepo"
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

	// —— 仓库 ——
	repoPath string
	repoName string
	branch   string
	state    loadState
	errMsg   string
	// loadSeq 每次发起加载自增，回调只在序号仍是最新时才写入结果，
	// 这样快速切换仓库时旧请求的结果不会覆盖新的。
	loadSeq int

	// —— 模型 ——
	// allFiles 是仓库里的全部改动；files 是过了筛选词之后、界面真正显示的
	// 那一份。侧栏树、diff 行模型、统计都只看 files，三者才对得上。
	allFiles []gitrepo.File
	files    []gitrepo.File
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
}

func newApp() *app {
	a := &app{
		sidebarOn: true,
		viewed:    map[string]string{},
	}
	// 默认同步执行，测试可直接驱动；生产环境由 main 换成 win.Update。
	a.dispatch = func(f func()) { f() }
	return a
}

func (a *app) view(c *ui.Context) {
	pal := paletteFor(c.Theme())
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
}

// toolbar 是自绘的标题栏：窗口用 TitleBarHidden，标题栏由 view 画出来，
// 红绿灯浮在上面。高度取 max(bar.Height, 52) 让红绿灯垂直居中，
// 左右各留出 bar.Left/bar.Right 给窗口控件让位。
func (a *app) toolbar(c *ui.Context, pal palette) {
	t := c.Theme()
	bar := c.TitleBar()

	ui.Row(c).Height(max(bar.Height, 52)).Shrink(0).
		Padding(0, bar.Right+12, 0, bar.Left+12).
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
			}

			switch a.state {
			case loadLoading:
				ui.Spinner(c)
			case loadReady:
				ui.Textf(c, "%d 个文件", len(a.files)).FontSize(12).TextColor(t.TextMuted)
			}

			if ui.Button(c, "刷新").Clicked() {
				a.reload()
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
			switch a.state {
			case loadIdle:
				ui.Text(c, "还没有打开仓库").FontSize(18).Bold()
				ui.Text(c, "选一个本地 git 仓库，看看有哪些改动。").
					TextColor(t.TextMuted)
				if ui.PrimaryButton(c, "打开仓库…").Clicked() {
					a.chooseFolder()
				}
			case loadLoading:
				ui.Spinner(c)
				ui.Text(c, "正在读取仓库…").TextColor(t.TextMuted)
			case loadFailed:
				ui.Text(c, "打不开这个仓库").FontSize(18).Bold()
				ui.Text(c, a.errMsg).TextColor(t.Danger)
				if ui.Button(c, "重试").Clicked() {
					a.reload()
				}
			case loadReady:
				if a.filter != "" {
					ui.Text(c, "没有匹配的文件").FontSize(18).Bold()
					ui.Textf(c, "筛选词「%s」没有匹配到任何改动。", a.filter).
						TextColor(t.TextMuted)
				} else {
					ui.Text(c, "没有未提交的改动").FontSize(18).Bold()
					ui.Text(c, "工作区是干净的。").TextColor(t.TextMuted)
				}
			}
		})
	})
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
	a.loadSeq++
	seq := a.loadSeq
	a.state = loadLoading
	a.errMsg = ""

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, path)
		if err != nil {
			a.dispatch(func() {
				if a.loadSeq != seq {
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
				if a.loadSeq != seq {
					return
				}
				a.state = loadFailed
				a.errMsg = err.Error()
			})
			return
		}
		a.dispatch(func() {
			if a.loadSeq != seq {
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
	a.allFiles = m.Files
	// 换了仓库，上一个仓库的展开状态和文件内容都不再适用
	a.expanded = nil
	a.contents = map[string]*fileContent{}
	a.state = loadReady

	a.applyFilter()
	a.loadContents(a.loadSeq)
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
		return
	}
	a.viewed[f.NewPath] = f.Fingerprint
}
