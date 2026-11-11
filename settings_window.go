package main

// 设置窗口的建窗缝（issue #7 问题四）。
//
// 设置现在是它自己的一个窗口，不再画在主窗口里。真路子是 mygo.NewWindow，
// 但它在应用没就绪时会 panic，而无头测试里没有应用 —— 所以建窗抽成包级
// 变量，测试换上替身，验的是「视图交出去了」「主视图干净了」这些真行为，
// 不去验系统的窗口管理器。
//
// 主窗口那边也走这条缝：设置开着的时候主窗口要让位（Esc、⌘W 这些键归
// 设置窗口），所以两边都要知道窗口在不在。

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsWindowOptions 是建一个设置窗口要的东西。
type settingsWindowOptions struct {
	// View 是设置窗口自己的视图函数。
	View func(*ui.Context)
	// OnClosed 是系统那边关窗时的回调（用户点了窗口的 ×，或者 ⌘W）。
	OnClosed func()
	// Parent 是主窗口：设置窗口是它的子窗口，压在它上面、跟着它最小化。
	Parent *mygo.Window
}

// settingsWindow 是设置窗口要用的那几个窗口操作。真实现是 *mygo.Window。
type settingsWindow interface {
	Close()
	Invalidate()
}

// 设置窗口的尺寸。它不再受主窗口约束 —— 以前 panelW 是
// clamp(winW-40, 640, 980)，主窗口一窄，右边的供应商表单就被压成
// 「Provider N...」。980 的上限照旧（模型设置是卡中卡，再宽两边都空）。
const (
	settingsWindowW    = 940
	settingsWindowH    = 680
	settingsWindowMinW = 720
	settingsWindowMinH = 460
)

// settingsWindowNew 建一个设置窗口。测试会换成替身。
//
// 第一个参数是打开设置的那个 *app：窗口的底色、材质、尺寸都要从它身上取
// （外观偏好、亚克力开关）。做成变量而不是方法，是为了测试能整个换掉 ——
// 见 tabs.go 的 terminalNew。
var settingsWindowNew = func(a *app, o settingsWindowOptions) settingsWindow {
	if o.Parent == nil {
		// 没有主窗口（无头测试）就开不出来：这时设置照常打开，只是没有
		// 窗口可用，界面上一处都不崩。
		return nil
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           T("设置", "Settings"),
		Width:           settingsWindowW,
		Height:          settingsWindowH,
		MinWidth:        settingsWindowMinW,
		MinHeight:       settingsWindowMinH,
		TitleBarStyle:   mygo.TitleBarHidden,
		BackgroundColor: windowGround(a.isDarkMode()),
		Transparent:     true,
		Vibrancy:        vibrancyFor(a.backgroundBlur()),
		Parent:          o.Parent,
		Content:         ui.View(o.View),
	})
	win.OnClosed(o.OnClosed)
	return win
}

// closeSettings 关掉设置窗口。
//
// 「保存 / 取消 / 标题栏的 ×」和系统那边的关闭（用户点窗口的 ×，或者 ⌘W）
// 是两条不同的路，都汇到这里。汇合是必须的：一次用户操作关两次窗，macOS 上
// 表现为窗口没了但应用还占着菜单，Windows 上表现为托盘里多一个幽灵。
func (a *app) closeSettings() {
	if !a.settingsOn {
		return
	}
	a.settingsOn = false
	if a.settingsWin != nil {
		w := a.settingsWin
		a.settingsWin = nil
		// 先摘掉回调再关：系统那边的 OnClosed 会绕回来，那时 settingsWin
		// 已经是 nil，这一趟就是个空转，不会再关一次。
		w.Close()
	}
}
