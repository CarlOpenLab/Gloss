package main

import (
	"github.com/egoist/mygo"
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

// appMenu 是菜单里需要跟着界面状态走的那几项，其余的项目建好就不再动。
//
// 要记着的有两种：单选/复选的勾（菜单展开时看得见，勾错了很扎眼），和
// 有没有东西可作用的可用性（没开仓库时，刷新和两个 tab 都没有意义）。
type appMenu struct {
	tabDiff *mygo.MenuItem
	tabHist *mygo.MenuItem
	sidebar *mygo.MenuItem
	unified *mygo.MenuItem
	split   *mygo.MenuItem
	refresh *mygo.MenuItem
}

// buildAppMenu 搭出应用的菜单栏，并把需要跟着状态走的几项记在 a.menu 上。
func buildAppMenu(a *app) *mygo.Menu {
	m := &appMenu{}

	m.tabDiff = a.menuToggle("查看改动", "CmdOrCtrl+1", mygo.MenuItemRadio, true, func() { a.setTab(0) })
	m.tabHist = a.menuToggle("查看历史", "CmdOrCtrl+2", mygo.MenuItemRadio, false, func() { a.setTab(1) })
	m.unified = a.menuToggle("统一视图", "", mygo.MenuItemRadio, true, func() { a.setSplit(false) })
	m.split = a.menuToggle("左右对照", "", mygo.MenuItemRadio, false, func() { a.setSplit(true) })
	m.sidebar = a.menuToggle("显示侧栏", "CmdOrCtrl+Shift+B", mygo.MenuItemCheckbox, true, a.toggleSidebar)
	m.refresh = a.menuCmd("刷新", "CmdOrCtrl+R", a.reload)

	view := []*mygo.MenuItem{
		m.tabDiff, m.tabHist,
		mygo.Separator(),
		m.unified, m.split,
		mygo.Separator(),
		m.sidebar,
		mygo.Separator(),
		a.menuCmd("放大代码字号", "CmdOrCtrl+Plus", func() {
			a.setCodeSize(a.codeSizeValue() + codeSizeStep)
		}),
		a.menuCmd("缩小代码字号", "CmdOrCtrl+-", func() {
			a.setCodeSize(a.codeSizeValue() - codeSizeStep)
		}),
		a.menuCmd("恢复默认字号", "CmdOrCtrl+0", func() { a.setCodeSize(0) }),
		mygo.Separator(),
		m.refresh,
		a.menuCmd("命令栏…", "CmdOrCtrl+K", a.openPalette),
		a.menuCmd("筛选文件", "CmdOrCtrl+F", func() { a.focusFilter = true }),
		mygo.Separator(),
		{Role: mygo.RoleToggleFullScreen, Label: "进入全屏"},
	}

	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: appName, Submenu: a.appSubmenu()},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			a.menuCmd("打开仓库…", "CmdOrCtrl+O", a.chooseFolder),
			mygo.Separator(),
			{Role: mygo.RoleClose, Label: "关闭窗口"},
		}},
		{Label: "编辑", Submenu: editSubmenu()},
		{Label: "视图", Submenu: view},
		// 「窗口」这一层用的是 RoleWindow：菜单名和内容都归我们写，但框架
		// 会把它注册成系统的窗口菜单（最小化、缩放那些系统项才认得它）。
		{Label: "窗口", Role: mygo.RoleWindow, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: "最小化"},
			{Role: mygo.RoleZoom, Label: "缩放"},
			mygo.Separator(),
			{Role: mygo.RoleFront, Label: "前置全部窗口"},
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
		{Role: mygo.RoleAbout, Label: "关于 " + appName},
		mygo.Separator(),
		a.menuCmd("模型设置…", "CmdOrCtrl+,", a.openSettings),
		mygo.Separator(),
		{Role: mygo.RoleServices, Label: "服务", Submenu: []*mygo.MenuItem{}},
		mygo.Separator(),
		{Role: mygo.RoleHide, Label: "隐藏 " + appName},
		{Role: mygo.RoleHideOthers, Label: "隐藏其他"},
		{Role: mygo.RoleUnhide, Label: "全部显示"},
		mygo.Separator(),
		{Role: mygo.RoleQuit, Label: "退出 " + appName},
	}
}

// editSubmenu 是「编辑」。全是角色：这些选择器最终会落到第一响应者身上，
// 也就是界面里正在编辑的那个输入框，剪切拷贝粘贴因此和系统里其它应用一样。
//
// 没有照搬框架默认菜单里的「Speech」一项：输入框是自绘的，语音那几个选择器
// 没人响应，摆在那里只是让菜单更长。
func editSubmenu() []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Role: mygo.RoleUndo, Label: "撤销"},
		{Role: mygo.RoleRedo, Label: "重做"},
		mygo.Separator(),
		{Role: mygo.RoleCut, Label: "剪切"},
		{Role: mygo.RoleCopy, Label: "拷贝"},
		{Role: mygo.RolePaste, Label: "粘贴"},
		{Role: mygo.RoleDelete, Label: "删除"},
		mygo.Separator(),
		{Role: mygo.RoleSelectAll, Label: "全选"},
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
// 命令栏也不该再叠一层。
func (a *app) menuDo(fn func()) {
	if a.palette.on || a.settingsOn {
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

	enableItem(m.refresh, !noRepo)
	enableItem(m.tabDiff, !noRepo)
	enableItem(m.tabHist, !noRepo)
}

// checkItem 只在勾真的会变的时候才去动它。
func checkItem(it *mygo.MenuItem, on bool) {
	if it.IsChecked() != on {
		it.SetChecked(on)
	}
}

// enableItem 同上，管可用性。
func enableItem(it *mygo.MenuItem, on bool) {
	if it.IsEnabled() != on {
		it.SetEnabled(on)
	}
}
