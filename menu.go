package main

import (
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 菜单栏。
//
// 这份菜单在 main 里通过 mygo.App.SetMenu 装上。不装的话框架会给一份
// 默认的（App / File / Edit / View / Window），那份有一处会真的坏事：
// 「View」里带着 Reload、Actual Size、Zoom In、Zoom Out，也就是 ⌘R、⌘0、
// ⌘+、⌘− 四个键被它们占了。菜单的键等价是原生的 —— NSMenu 的 key
// equivalent 在按键进到视图之前就拦下它 —— 而 Gloss 的界面是原生绘制的，
// 框架执行这几个角色时会发现「这个窗口没有网页」，然后什么都不做。
// 净效果是：真实应用里按 ⌘R 刷新、按 ⌘+/⌘−/⌘0 调字号全都没反应，
// 尽管 keys.go 里这几个键都接好了。自己搭一份菜单，就是把这几个键接回
// 应用自己的命令。
//
// 顺带还有语言：默认菜单是英文的，而这个应用从头到脚是中文的。这里每一项
// 都写中文标签，行为仍旧交给框架的角色（撤销、拷贝、退出……还是系统那一套
// 选择器，输入框里的复制粘贴、关于面板、隐藏、最小化都照常）。
//
// 键位和 keys.go 的 handleKeys 必须对得上：真实 macOS 应用里菜单先把键
// 吃掉，视图那条路根本走不到；测试和无菜单的环境（比如将来跑在 Linux 上）
// 走的才是视图那条。两处写的是同一组键，改一处就得改另一处。

// appName 是菜单里要写进「关于 X」这类标题的名字。产品名就是 Gloss，
// 和窗口标题、工具栏上那个兜底的名字是同一个。
const appName = "Gloss"

// updaterMenuItem 是「检查更新…」那一项。单独抽成一个变量，好让测试替换。
//
// 这一项由 mygo 的自动更新插件提供，而插件是在 App.Run 之前注册的
// （main.go 的 mygo.Use）。测试既不注册插件也没有事件循环，直接调它要来的
// 只有两种下场：没注册时 panic，注册了则卡在等主线程队列上 —— 而那个队列
// 只有真的跑起来的事件循环才会倒。菜单本身和这一项无关，换成替身不影响
// 别处的断言。
var updaterMenuItem = func() *mygo.MenuItem { return updater.MenuItem() }

// appMenu 是菜单里需要跟着界面状态走的那几项，其余的项目建好就不再动。
//
// 要记着的有两种：单选/复选的勾（菜单展开时看得见，勾错了很扎眼），和
// 有没有东西可作用的可用性（没开仓库时，刷新和两个 tab 都没有意义）。
type appMenu struct {
	tabDiff     *mygo.MenuItem
	tabHist     *mygo.MenuItem
	sidebar     *mygo.MenuItem
	unified     *mygo.MenuItem
	split       *mygo.MenuItem
	refresh     *mygo.MenuItem
	commit      *mygo.MenuItem
	newBranch   *mygo.MenuItem
	newTag      *mygo.MenuItem
	stash       *mygo.MenuItem
	stageAll    *mygo.MenuItem
	unstageAll  *mygo.MenuItem
	fetch       *mygo.MenuItem
	pull        *mygo.MenuItem
	push        *mygo.MenuItem
	forcePush   *mygo.MenuItem
	prList      *mygo.MenuItem
	themeSystem *mygo.MenuItem
	themeLight  *mygo.MenuItem
	themeDark   *mygo.MenuItem
	langZh      *mygo.MenuItem
	langEn      *mygo.MenuItem
}

// buildAppMenu 搭出应用的菜单栏，并把需要跟着状态走的几项记在 a.menu 上。
func buildAppMenu(a *app) *mygo.Menu {
	m := &appMenu{}

	m.tabDiff = a.menuToggle(T("查看改动", "Show Changes"), "CmdOrCtrl+1", mygo.MenuItemRadio, true, func() { a.setTab(0) })
	m.tabHist = a.menuToggle(T("查看历史", "Show History"), "CmdOrCtrl+2", mygo.MenuItemRadio, false, func() { a.setTab(1) })
	m.unified = a.menuToggle(T("统一视图", "Unified View"), "CmdOrCtrl+U", mygo.MenuItemRadio, !a.split, func() { a.setSplit(false) })
	m.split = a.menuToggle(T("左右对照", "Split View"), "", mygo.MenuItemRadio, a.split, func() { a.setSplit(true) })
	m.sidebar = a.menuToggle(T("显示侧栏", "Show Sidebar"), "CmdOrCtrl+Shift+B", mygo.MenuItemCheckbox, true, a.toggleSidebar)
	m.refresh = a.menuCmd(T("刷新", "Refresh"), "CmdOrCtrl+R", a.reload)
	m.commit = a.menuCmd(T("提交", "Commit"), "CmdOrCtrl+Return", a.doCommit)
	m.stageAll = a.menuCmd(T("暂存全部改动", "Stage All Changes"), "CmdOrCtrl+Shift+A", func() {
		if a.tab == 0 && a.repoReady() && a.op == gitrepo.OpNone {
			a.stageAll()
		}
	})
	m.unstageAll = a.menuCmd(T("取消暂存全部改动", "Unstage All Changes"), "", func() {
		if a.tab == 0 && a.repoReady() && a.op == gitrepo.OpNone {
			a.unstageAll()
		}
	})
	m.newBranch = a.menuCmd(T("新建分支…", "New Branch…"), "CmdOrCtrl+Shift+N", a.newBranchAtHead)
	m.newTag = a.menuCmd(T("新建标签…", "New Tag…"), "", a.newTagAtHead)
	m.stash = a.menuCmd(T("储藏工作区改动…", "Stash Changes…"), "", a.openStashDialog)
	m.prList = a.menuCmd(T("查看 GitHub Pull Requests…", "GitHub Pull Requests…"), "", a.openPRDialog)

	// 抓取 / 拉取 / 推送的键是挑出来的：F 和 P 是 Fetch 和 Pull，U 是「上传」
	// 那个 up（Push 的 P 被拉取占了）。三个都带 ⇧：⌘P 归「筛选文件」，
	// 单键轮不到它们。
	m.fetch = a.menuCmd(T("抓取", "Fetch"), "CmdOrCtrl+Shift+F", a.doFetch)
	m.pull = a.menuCmd(T("拉取", "Pull"), "CmdOrCtrl+Shift+P", a.doPull)
	m.push = a.menuCmd(T("推送", "Push"), "CmdOrCtrl+Shift+U", a.doPush)
	m.forcePush = a.menuCmd(T("强制推送（--force-with-lease）…", "Force Push (--force-with-lease)…"), "", a.promptForcePush)

	m.themeSystem = a.menuToggle(T("跟随系统", "Follow System"), "", mygo.MenuItemRadio, a.themeMode == "system" || a.themeMode == "", func() { a.setThemeMode("system") })
	m.themeLight = a.menuToggle(T("浅色模式", "Light Mode"), "", mygo.MenuItemRadio, a.themeMode == "light", func() { a.setThemeMode("light") })
	m.themeDark = a.menuToggle(T("深色模式", "Dark Mode"), "", mygo.MenuItemRadio, a.themeMode == "dark", func() { a.setThemeMode("dark") })

	m.langZh = a.menuToggle("简体中文", "", mygo.MenuItemRadio, a.locale != "en", func() { a.setLocale("zh") })
	m.langEn = a.menuToggle("English", "", mygo.MenuItemRadio, a.locale == "en", func() { a.setLocale("en") })

	themeSub := []*mygo.MenuItem{
		m.themeSystem,
		m.themeLight,
		m.themeDark,
	}
	langSub := []*mygo.MenuItem{
		m.langZh,
		m.langEn,
	}

	view := []*mygo.MenuItem{
		m.tabDiff, m.tabHist,
		a.menuCmd(T("切换终端 / Git 面板", "Toggle Terminal / Git Panel"), "CmdOrCtrl+J", a.toggleViewMode),
		mygo.Separator(),
		m.unified, m.split,
		mygo.Separator(),
		m.sidebar,
		mygo.Separator(),
		{Label: T("外观主题", "Appearance"), Submenu: themeSub},
		{Label: T("界面语言", "Language"), Submenu: langSub},
		mygo.Separator(),
		a.menuCmd(T("放大代码字号", "Increase Font Size"), "CmdOrCtrl+Plus", func() {
			a.setCodeSize(a.codeSizeValue() + codeSizeStep)
		}),
		a.menuCmd(T("缩小代码字号", "Decrease Font Size"), "CmdOrCtrl+-", func() {
			a.setCodeSize(a.codeSizeValue() - codeSizeStep)
		}),
		a.menuCmd(T("恢复默认字号", "Reset Font Size"), "CmdOrCtrl+0", func() { a.setCodeSize(0) }),
		mygo.Separator(),
		m.refresh,
		a.menuCmd(T("命令栏…", "Command Palette…"), "CmdOrCtrl+K", a.openPalette),
		// 「筛选文件」本质是 quick-open（找的是路径），所以摆在 ⌘P 上。
		// ⌘F 留给改动区自己的查找（DiffView 里那一栏）：找内容和找文件是
		// 两件事，而且内容查找的「当前哪一条」只有编辑器自己知道。
		a.menuCmd(T("筛选文件", "Filter Files"), "CmdOrCtrl+P", func() { a.focusFilter = true }),
		mygo.Separator(),
		{Role: mygo.RoleToggleFullScreen, Label: T("进入全屏", "Toggle Full Screen")},
	}

	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: appName, Submenu: a.appSubmenu()},
		{Label: T("文件", "File"), Submenu: []*mygo.MenuItem{
			a.menuCmd(T("新建项目标签页…", "New Project Tab…"), "CmdOrCtrl+T", a.chooseFolder),
			a.menuCmd(T("新建终端标签页", "New Terminal Tab"), "CmdOrCtrl+Shift+T", a.newTerminalTab),
			a.menuCmd(T("打开仓库…", "Open Repository…"), "CmdOrCtrl+O", a.chooseFolder),
			a.recentReposMenuItem(),
			mygo.Separator(),
			a.menuCmd(T("关闭标签页", "Close Tab"), "CmdOrCtrl+W", func() { a.closeTab(a.activeTab) }),
			a.menuCmd(T("上一个标签页", "Previous Tab"), "CmdOrCtrl+Shift+[", a.prevTab),
			a.menuCmd(T("下一个标签页", "Next Tab"), "CmdOrCtrl+Shift+]", a.nextTab),
			mygo.Separator(),
			m.commit,
			m.stageAll,
			m.unstageAll,
			m.stash,
			m.newBranch,
			m.newTag,
			a.menuCmd(T("对比分支代码差异…", "Compare Branches…"), "CmdOrCtrl+Shift+D", func() { a.openCompareDialog("") }),
			m.prList,
			mygo.Separator(),
			{Role: mygo.RoleClose, Accelerator: "CmdOrCtrl+Shift+W", Label: T("关闭窗口", "Close Window")},
		}},
		{Label: T("编辑", "Edit"), Submenu: editSubmenu()},
		// 远程单开一层：这三条命令和「文件」里那些（打开、提交、打标签）不
		// 是一回事 —— 它们要和网络、和凭证打交道，会等、会失败，出事的地方
		// 也不一样。摆在一层自己的菜单里，用户找「拉取」的时候知道往哪儿看。
		{Label: T("远程", "Remote"), Submenu: []*mygo.MenuItem{
			m.fetch,
			m.pull,
			m.push,
			mygo.Separator(),
			m.forcePush,
		}},
		{Label: T("视图", "View"), Submenu: view},
		// 「窗口」这一层用的是 RoleWindow：菜单名和内容都归我们写，但框架
		// 会把它注册成系统的窗口菜单（最小化、缩放那些系统项才认得它）。
		{Label: T("窗口", "Window"), Role: mygo.RoleWindow, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: T("最小化", "Minimize")},
			{Role: mygo.RoleZoom, Label: T("缩放", "Zoom")},
			mygo.Separator(),
			{Role: mygo.RoleFront, Label: T("前置全部窗口", "Bring All to Front")},
		}},
	})

	a.menu = m
	// 建好就按当前状态勾一遍：菜单第一次拉开时，勾的位置就得是对的。
	a.syncMenu()
	return menu
}

// appSubmenu 是应用菜单（macOS 上第一项，加粗的那个）的内容。
//
// 每一项都带角色：关于、隐藏、退出走的是系统选择器，行为和任何一个 Mac
// 应用一样，只是标题换成了中文。「服务」要留一个子菜单，哪怕它是空的 ——
// 系统会把别的应用提供的服务往这里填，没有子菜单的话这一项就是个死条目。
//
// 「模型设置…」按 Mac 的老规矩摆在这儿、用 ⌘,：设置不在这个位置、不认这个
// 键的话，Mac 用户会找不到它 —— 他在任何别的应用里都是这么找的。
func (a *app) appSubmenu() []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Role: mygo.RoleAbout, Label: T("关于 ", "About ") + appName},
		updaterMenuItem(),
		mygo.Separator(),
		a.menuCmd(T("模型设置…", "Settings…"), "CmdOrCtrl+,", a.openSettings),
		mygo.Separator(),
		{Role: mygo.RoleServices, Label: T("服务", "Services"), Submenu: []*mygo.MenuItem{}},
		mygo.Separator(),
		{Role: mygo.RoleHide, Label: T("隐藏 ", "Hide ") + appName},
		{Role: mygo.RoleHideOthers, Label: T("隐藏其他", "Hide Others")},
		{Role: mygo.RoleUnhide, Label: T("全部显示", "Show All")},
		mygo.Separator(),
		{Role: mygo.RoleQuit, Label: T("退出 ", "Quit ") + appName},
	}
}

// editSubmenu 是「编辑」。全是角色：这些选择器最终会落到第一响应者身上，
// 也就是界面里正在编辑的那个输入框，剪切拷贝粘贴因此和系统里其它应用一样。
//
// 没有照搬框架默认菜单里的「Speech」一项：输入框是自绘的，语音那几个选择器
// 没人响应，摆在那里只是让菜单更长。
func editSubmenu() []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Role: mygo.RoleUndo, Label: T("撤销", "Undo")},
		{Role: mygo.RoleRedo, Label: T("重做", "Redo")},
		mygo.Separator(),
		{Role: mygo.RoleCut, Label: T("剪切", "Cut")},
		{Role: mygo.RoleCopy, Label: T("拷贝", "Copy")},
		{Role: mygo.RolePaste, Label: T("粘贴", "Paste")},
		{Role: mygo.RoleDelete, Label: T("删除", "Delete")},
		mygo.Separator(),
		{Role: mygo.RoleSelectAll, Label: T("全选", "Select All")},
	}
}

// menuCmd 是一条「点一下就做一件事」的菜单项。
//
// 命令都包一层 menuDo：菜单点下去的时候人在主线程上，但不在某一帧的构建
// 过程里，状态改了得自己请求一次重绘。
func (a *app) menuCmd(label, accel string, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Accelerator: accel, Click: func(*mygo.MenuItem, *mygo.Window) {
		a.menuDo(fn)
	}}
}

// menuToggle 是带勾的菜单项（单选、复选），和 menuCmd 一样跑一条命令。
//
// checked 只是建菜单时的初始值，之后由 syncMenu 每帧对着应用状态修正：
// 勾是「现在是什么样」的显示，不是命令自己的状态。
func (a *app) menuToggle(label, accel string, kind mygo.MenuItemType, checked bool, fn func()) *mygo.MenuItem {
	it := a.menuCmd(label, accel, fn)
	it.Type = kind
	it.Checked = checked
	return it
}

// menuDo 跑一条菜单命令。
//
// 对话框开着的时候让位：视图里的快捷键在模态下会被框架挡掉，菜单可挡不掉，
// 两边得一个脾气 —— 设置或命令栏开着的时候，⌘R 不该在背后偷偷刷新，
// 命令栏也不该再叠一层。判据收在 modalOn 里，视图那边 Esc 的让位用的是
// 同一个函数。
func (a *app) menuDo(fn func()) {
	if a.modalOn() {
		return
	}
	if a.dispatch == nil {
		fn()
		return
	}
	a.dispatch(fn)
}

// syncMenu 把界面状态推给菜单里那几个带勾、带可用性的项目。
//
// 挂在 view 上每帧调一次，而不是塞进每个改状态的函数里：状态能从好几个
// 地方改（快捷键、命令栏、工具栏、侧栏），漏掉一处就是一处对不上。每一项
// 都先比再写 —— 值没变的时候一声不吭，免得每帧都去动原生菜单。
func (a *app) syncMenu() {
	m := a.menu
	if m == nil {
		return
	}
	// 判 state 不判 repoPath：打开失败时 repoPath 也记着东西（试的是哪个
	// 文件夹），可那时候刷新和两个 tab 同样没有作用对象。和工具栏、setTab
	// 一个口径 —— 失败态的出路是屏幕中间那张卡片，不是这几项。
	noRepo := !a.repoReady()

	checkItem(m.tabDiff, a.tab == 0)
	checkItem(m.tabHist, a.tab == 1)
	checkItem(m.sidebar, a.sidebarOn)
	checkItem(m.unified, !a.split)
	checkItem(m.split, a.split)
	checkItem(m.themeSystem, a.themeMode == "system" || a.themeMode == "")
	checkItem(m.themeLight, a.themeMode == "light")
	checkItem(m.themeDark, a.themeMode == "dark")
	checkItem(m.langZh, a.locale != "en")
	checkItem(m.langEn, a.locale == "en")

	enableItem(m.refresh, !noRepo)
	enableItem(m.tabDiff, !noRepo)
	enableItem(m.tabHist, !noRepo)
	enableItem(m.unified, !noRepo)
	enableItem(m.split, !noRepo)
	enableItem(m.commit, a.canCommit())
	enableItem(m.stageAll, !noRepo && a.tab == 0 && a.op == gitrepo.OpNone && a.unstagedCount() > 0)
	enableItem(m.unstageAll, !noRepo && a.tab == 0 && a.op == gitrepo.OpNone && a.stagedCount() > 0)
	enableItem(m.stash, !noRepo && a.op == gitrepo.OpNone)
	enableItem(m.newBranch, !noRepo && a.op == gitrepo.OpNone)
	enableItem(m.newTag, !noRepo && a.op == gitrepo.OpNone)
	enableItem(m.prList, !noRepo)

	// 远程三条：没有远程就没得抓也没得推（`git fetch --all` 在这种情况下
	// 静默成功，等错误等不到）；分离 HEAD 上拉取和推送没有落点，抓取照常。
	// 判据和工具栏那三颗按钮用的是同一个（canReachRemote），两处不能各写
	// 一套 —— 菜单亮着而按钮是灰的，用户不知道该信哪个。
	canRemote := a.canReachRemote()
	enableItem(m.fetch, canRemote)
	enableItem(m.pull, canRemote && !a.detached)
	enableItem(m.push, canRemote && !a.detached)
	enableItem(m.forcePush, canRemote && !a.detached && a.hasUpstream(a.branch))
}

// checkItem 只在勾真的会变的时候才去动它。
func checkItem(it *mygo.MenuItem, on bool) {
	if it != nil && it.IsChecked() != on {
		it.SetChecked(on)
	}
}

// enableItem 同上，管可用性。
func enableItem(it *mygo.MenuItem, on bool) {
	if it != nil && it.IsEnabled() != on {
		it.SetEnabled(on)
	}
}

// recentReposMenuItem 构建菜单栏「文件 -> 最近打开的项目」子菜单。
func (a *app) recentReposMenuItem() *mygo.MenuItem {
	recent := a.store.State().RecentRepos
	var items []*mygo.MenuItem
	for _, p := range recent {
		if p == "" {
			continue
		}
		pathCopy := p
		base := filepath.Base(p)
		items = append(items, a.menuCmd(base+"  ("+p+")", "", func() {
			a.openRepo(pathCopy)
		}))
	}
	if len(items) > 0 {
		items = append(items, mygo.Separator())
		items = append(items, a.menuCmd(T("清除最近打开记录", "Clear Recent Repositories"), "", func() {
			a.store.ClearRecentRepos()
			a.persist()
		}))
	} else {
		items = append(items, &mygo.MenuItem{
			Label:    T("无最近项目", "No Recent Repositories"),
			Disabled: true,
		})
	}
	return &mygo.MenuItem{
		Label:   T("最近打开的项目", "Recent Repositories"),
		Submenu: items,
	}
}
