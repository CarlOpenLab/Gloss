package main

// 设置窗口自己是一个窗口，那它就得像主窗口那样：接着用系统的窗口控件、有
// 地方拖动、给系统控件让位、内容不溢出去。
//
// 用户报的是：
//
//	「弹窗 拖动不了 而且样式也乱了」「布局乱了 而且不能拖动位置了。」
//	「macos 的按钮不能还是用原生的吗」「Windows 的 关闭 icon 都乱了」
//	「为哈破坏这么大」
//
// 截图里看得见的是左上角三颗红绿灯正压在「设置」两个字上 —— 量那张 940x680
// 的图：红灯 x10..21、黄灯 x33..44、绿灯 x56..67（都在 y10..21），而「设置」
// 两个字的墨在 x22..43、y17..27 —— 正好横在红灯和黄灯中间。
//
// 但**看得见的这一处不是根因**。根因在 settings_window.go 建窗那一行：它照抄了
// 主窗口的 TitleBarHidden，却没抄另一半 —— macOS 的 TrafficLightPosition、
// Windows 的 Frameless。于是 macOS 按 AppKit 默认位置画那三颗（紧贴左上角，
// 压在内容上），Windows 上系统那三颗照画、而视图又在卡片顶栏画一颗自己的
// 关闭按钮（两颗挤在一处）。所以真正的修法是把主窗口那一套抄全，视图只补
// 它自己该补的两件事：让位和拖动。
//
// 主窗口这两件事都做过（shell_style.go 的 shellTopBar、app.go 工具栏按
// c.TitleBar() 算 padLeft / padRight，主区 / 工具栏 / 侧栏标题行三处
// DragWindow），设置窗口搬进自己的窗口时忘了带过去。
//
// 顺带量过一件**不是**问题的事：底栏那句「设置存储于本地配置文件，所有请求由
// 本机直连服务端」在真实窗口里没有折行 —— 默认档 286px 宽、一行，和用户那张
// 图量出来的一致（按钮 792/858 对 793/857）。大字号档涨到 428px，在 940 宽里
// 也还放得下。TestSettingsFooter* 两条留作回归防线，不是用户报的那件事。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 红绿灯在设置窗口里占的地方。测试按最宽的那一版算：主窗口用
// TrafficLightPosition{18, 23}，设置窗口现在也用同一个（修好之后），
// AppKit 按它摆位。取宽一点的余量，免得测试放过的位置用户看着还是压着。
const (
	// settingsBarLead 是卡片左沿让给系统窗口控件的距离：三颗按钮里最右
	// 一颗的右边，再加一道内边距。主窗口同一条算式在 titleBarLead。
	settingsBarLead = 100
	settingsBarTop  = 60
)

// settingsTitleBarTester 按 macOS 的样子建一个设置窗口 Tester：红绿灯占着
// 左上角，和真实窗口一致（不设的话 TitleBar 是零值，视图一像素都不让，
// 于是这个问题在测试里永远看不见）。
func settingsTitleBarTester(t *testing.T, a *app) *ui.Tester {
	t.Helper()
	installSettingsWindowStub(t)
	a.openSettings()
	st := newSettingsViewTester(t, a, settingsWindowW, settingsWindowH)
	st.SetTitleBar(ui.TitleBar{Height: settingsBarTop, Left: settingsBarLead})
	st.Frame()
	st.Frame()
	return st
}

// TestSettingsNavTopRowClearsTheWindowControls 导航栏顶上那一行（拖动带）
// 必须在系统窗口控件底下穿过去之前让开。
//
// 这一条是用户截图的那件事：标题压在红绿灯上。TitleBarHidden 的窗口在 macOS
// 上照样画那三颗，视图要按 c.TitleBar() 让位 —— 主窗口的工具栏（app.go）和
// 侧栏（project_sidebar.go）都这么做，设置窗口搬进自己的窗口时是唯一没做的。
//
// 「设置」两个字后来撤了：窗口标题已经是「设置」（settings_window.go 的 Title），
// 系统控件也在这一行 —— 同一件事在 40px 里自我介绍两遍，还挤掉了拖动带的面积。
// 撤掉之后红绿灯那一带（y < bar.Height 且 x < bar.Left）一片空白，这条就改成
// 守这个不变量：那一带里什么文字都不该有。
func TestSettingsNavTopRowClearsTheWindowControls(t *testing.T) {
	a := richApp(t)
	st := settingsTitleBarTester(t, a)

	// 灯占的带子：从左沿到 bar.Left 宽、从顶端到 bar.Height 高。
	var under []string
	for _, s := range st.Texts() {
		r, ok := st.Find(s)
		if !ok {
			continue
		}
		if r.Y < settingsBarTop && r.X < settingsBarLead {
			under = append(under, s)
		}
	}
	if len(under) > 0 {
		t.Errorf("有内容画在系统窗口控件底下（y<%d 且 x<%d）：%v —— "+
			"视图该按 c.TitleBar() 让位", settingsBarTop, settingsBarLead, under)
	}
}

// TestSettingsWindowHasADragHandle 设置窗口必须有一片能拖动它的地方。
//
// 用户报的是「不能拖动位置了」。设置搬进自己的窗口之后，视图里一处
// DragWindow 都没有：整个窗口按 AppKit 的规矩来（标题栏区透明、正文区不拖），
// 而这一片正文区铺满整个窗口 —— 于是哪里都拖不动。
//
// 主窗口有三处（主区、工具栏、侧栏标题行），设置窗口一处都没有。
//
// 这条按源码查，不按画面查：无头环境里 rt.host.startDrag() 是空操作
// （ui/headless.go），按下去的可见后果（窗口挪了）一样都没有 —— 所以画面
// 上「有没有 DragWindow」是验不出来的，只有源码里那一处 .DragWindow() 是真的。
// 按 AST 找调用点，和 TestUIFontScaleReachesEveryToken 同一个路子。
func TestSettingsWindowHasADragHandle(t *testing.T) {
	handles := dragHandleLines(t, "settings.go", "settingsDialog", "settingsBody")
	if len(handles) == 0 {
		t.Fatal("设置窗口的视图里一处 .DragWindow() 都没有 —— 整个窗口都是正文，" +
			"AppKit 只让标题栏区拖动，于是哪里都拖不动（用户报的「不能拖动位置了」）")
	}
}

// TestSettingsWindowKeepsTheNativeControls 设置窗口必须接着用系统那套窗口
// 控件，一个平台都不能少。
//
// 用户报的是「macos 的按钮不能还是用原生的吗」「Windows 的关闭 icon 都乱了」
// 「为哈破坏这么大」。破坏的源头不视图里，在 settings_window.go 建窗那一行：
// 它照抄了主窗口的 TitleBarHidden，却没抄另一半 —— macOS 的
// TrafficLightPosition、Windows 的 Frameless。
//
// 少抄的后果按平台分开：
//
//   - macOS：darwin 的 layoutTrafficLights 只在 TrafficLightPosition 非 nil 时
//     动手（internal/darwin/window.go:944）。不设就按 AppKit 默认位置画三颗
//     —— 紧贴窗口左上角，正压在「设置」两个字上。
//   - Windows：不设 Frameless，系统那三颗按钮照画（internal/windows/window.go:172
//     的 newCaptionBar），而视图又在卡片顶栏画一颗自己的关闭按钮 —— 两颗挤在
//     一处，就是「关闭 icon 乱了」。
//
// 所以这条同时看两边：macOS 上必须 HiddenInset + 灯的位置，Windows / Linux 上
// 必须 Frameless。漏任何一边，另一边的用户就在看坏掉的 chrome。
//
// 两个平台都在这儿验：平台是参数，不是本机 GOOS（isMacPlatform 测试改不了）。
// 在 macOS 上开发，Windows 那一半也得有人看着。
func TestSettingsWindowKeepsTheNativeControls(t *testing.T) {
	// 从一套和主窗口一样的基础 opts 出发，只看标题栏那一套填成什么样。
	// 不碰 *app：isDarkMode 会 post 到主线程并阻塞等着回来，无头环境里没有
	// 主循环，一个 Tester 建起来就永远卡住（见 settings.go 顶上关于 isDark(c)
	// 的说明）。
	t.Run("macOS", func(t *testing.T) {
		opts := mygo.WindowOptions{TitleBarStyle: mygo.TitleBarHidden}
		settingsTitleBarOptions(&opts, true)

		if opts.TitleBarStyle != mygo.TitleBarHiddenInset {
			t.Errorf("TitleBarStyle = %q，该是 %q —— "+
				"只写 Hidden 的话 AppKit 按默认位置画红绿灯，压在「设置」标题上",
				opts.TitleBarStyle, mygo.TitleBarHiddenInset)
		}
		if opts.TrafficLightPosition == nil {
			t.Fatal("没设 TrafficLightPosition —— 红绿灯会画在 AppKit 的" +
				"默认位置（紧贴窗口左上角），压在内容上（用户报的「布局乱了」）")
		}
		if opts.TrafficLightPosition.X != trafficLightX || opts.TrafficLightPosition.Y != trafficLightY {
			t.Errorf("红绿灯位置 = (%v, %v)，该和主窗口一致 (%d, %d)",
				opts.TrafficLightPosition.X, opts.TrafficLightPosition.Y,
				trafficLightX, trafficLightY)
		}
		if opts.Frameless {
			t.Error("macOS 上不该设 Frameless：那会把系统红绿灯一并收走" +
				"（darwin 的 Frameless 把三颗标准按钮 setHidden:1）")
		}
	})

	t.Run("Windows/Linux", func(t *testing.T) {
		opts := mygo.WindowOptions{TitleBarStyle: mygo.TitleBarHidden}
		settingsTitleBarOptions(&opts, false)

		if opts.TrafficLightPosition != nil {
			t.Error("不该设 TrafficLightPosition：那儿没有红绿灯")
		}
		if !opts.Frameless {
			t.Error("没设 Frameless —— 系统那三颗按钮会照画，" +
				"和卡片顶栏里自绘的关闭按钮挤在一处（用户报的「关闭 icon 乱了」）")
		}
	})

	// 生产里走的那一支必须就是上面某一支：改坏了接线，两个子测试再绿也没用。
	if !isMacPlatform() {
		return
	}
	opts := mygo.WindowOptions{TitleBarStyle: mygo.TitleBarHidden}
	settingsTitleBarOptions(&opts, isMacPlatform())
	if opts.TitleBarStyle != mygo.TitleBarHiddenInset {
		t.Errorf("生产分支走了 macOS 那一支，但 TitleBarStyle = %q", opts.TitleBarStyle)
	}
}

// dragHandleLines 找出 file 里 funcs 中那些函数体（含嵌套闭包）的
// .DragWindow() 调用点，按行号返回。
func dragHandleLines(t *testing.T, file string, funcs ...string) []int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", file, err)
	}
	want := map[string]bool{}
	for _, name := range funcs {
		want[name] = true
	}
	var lines []int
	ast.Inspect(f, func(n ast.Node) bool {
		// 只在指定的函数体里找。
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Body != nil {
			if !want[fn.Name.Name] {
				return false
			}
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "DragWindow" {
			return true
		}
		lines = append(lines, fset.Position(call.Pos()).Line)
		return true
	})
	return lines
}

// TestSettingsFooterFitsLargeText 底栏那一排必须在大字号档下也装得下那句
// 本地存储说明。
//
// 这一条不是用户报的那件事（那句在真实窗口里没折行，见文件顶上的量法），
// 留作回归防线：那一排的高度是写死的 settingsFooterH = 52，而字是会变的 ——
// 大一档那句话就从 286px 宽涨到 428px，再涨下去就得折行、顶出卡片。
func TestSettingsFooterFitsLargeText(t *testing.T) {
	const note = "设置存储于本地配置文件，所有请求由本机直连服务端"

	// 一档一档地看，大档是最紧的那一档。
	old := setUIFontSizeStep(uiFontSizeMax)
	t.Cleanup(func() { setUIFontSizeStep(old) })

	a := richApp(t)
	st := settingsTitleBarTester(t, a)

	r, ok := st.Find(note)
	if !ok {
		t.Fatal("底栏里没有本地存储说明")
	}
	// 一行放下的话，这段字的高度就是一行（大字号档下 21 上下）。折成两行
	// 会翻倍 —— 而那一排只有 settingsFooterH = 52 高，多出来的就顶出卡片。
	line := uiFontSize(textBodySM) * 1.4
	if r.H > line*1.6 {
		t.Errorf("本地存储说明折行了：高 %.1f，一行只有 %.1f —— "+
			"大字号档下它从一排放不下了", r.H, line)
	}
	// 整句话必须完整在窗口里：窗口是 940 宽，右端留 18 给「取消 / 保存」。
	if r.X+r.W > settingsWindowW-18 {
		t.Errorf("本地存储说明右端顶到 %.0f，窗口只有 %d 宽", r.X+r.W, settingsWindowW)
	}
}

// TestSettingsFooterButtonsStayInTheWindow 「取消 / 保存」在大字号档下必须
// 还待在窗口里，不被挤出去。
//
// 和上一条是同一边的事：那句话越长，两颗按钮被推得越靠右。
func TestSettingsFooterButtonsStayInTheWindow(t *testing.T) {
	old := setUIFontSizeStep(uiFontSizeMax)
	t.Cleanup(func() { setUIFontSizeStep(old) })

	a := richApp(t)
	st := settingsTitleBarTester(t, a)

	for _, s := range []string{"取消", "保存"} {
		r, ok := st.Find(s)
		if !ok {
			t.Fatalf("底栏里没有 %q", s)
		}
		if r.X+r.W > settingsWindowW {
			t.Errorf("%q 被挤出窗口：右端 %.0f，窗口只有 %d 宽", s, r.X+r.W, settingsWindowW)
		}
	}
}
