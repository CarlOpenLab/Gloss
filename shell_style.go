package main

import (
	"runtime"

	"github.com/egoist/mygo/ui"
)

// The shell follows a soft, layered desktop rhythm: the window chrome, the
// sidebar controls and each card header share one 30px center line.
const (
	shellBarMid  = 30
	shellTopBar  = 2 * shellBarMid
	shellHeaderH = 2 * (shellBarMid - shellCardGap)
	shellCardGap = 8

	// Sidebar controls keep a fixed title-bar slot whether the sidebar is
	// expanded or folded. The folded card leaves this whole cluster clear.
	shellIconButtonW    = float32(28)
	shellIconButtonH    = float32(26)
	shellIconButtonGap  = float32(2)
	sidebarControlInset = float32(20)
	sidebarControlsW    = 3*shellIconButtonW + 2*shellIconButtonGap
)

func titleBarLead(bar ui.TitleBar) float32 {
	if bar.Left > 0 {
		return bar.Left + shellCardGap
	}
	return 12
}

func sidebarControlsLeft(bar ui.TitleBar) float32 {
	return titleBarLead(bar) + sidebarControlInset
}

// shellWindowRadiusFor 是窗口自己那一圈圆角的半径。它不归我们画 —— macOS /
// Windows 由系统画，Linux 交给桌面主题 —— 但工作卡贴着它内缩 shellCardGap，
// 所以这个数必须写下来：卡片不知道窗口有多圆，就只能各圆各的（见
// shellCardRadiusFor）。
//
// 这三个数属于操作系统，不是 §3 那六级设计圆角，改不了，只能跟。
func shellWindowRadiusFor(goos string) float32 {
	switch goos {
	case "darwin":
		// macOS 26（Tahoe）把窗口角做得比上一代明显更圆。按截图量，窗口那圈
		// 弧的半径是工作卡（14）的 2.1 倍，28 / 13.5 正好对得上；macOS 15
		// 及更早是 10。
		return 28
	case "windows":
		// Windows 11 的 DWM 圆角。窗口是无边框的，main.go 会在建窗后把原生
		// 标题栏（连同 DWM 画在材质上的三颗按钮）收掉，同时显式要回这一档
		// 圆角（见 applyNativeChrome）。
		return 8
	default:
		// Linux 各家 CSD 常见的那一档（GNOME 的 Adwaita）。
		return 12
	}
}

// shellCardRadiusFor 是工作卡的圆角：内圆 = 外圆 − 间距，但不低于卡片自己的
// 那一档（radiusLG），也不能比外圆更圆。
//
// 同心是这条留白唯一的读法。写死一个不跟外圈走的数（原来是 14），在 macOS 26
// 那种 28 的窗口角里，8px 的灰带会在角上挤薄到 5px —— 眼睛看见的是两条曲率
// 差一倍的弧，像两张不相干的卡片叠在一起，而不是一张卡片浮在窗口里。
//
// 窗口角小到装不下 rounded.lg 时（Windows 11 的 8、间距也是 8），同心会算出 0：
// 一张没有圆角的卡片读起来不是「跟着窗口走」，是「圆角丢了」。这时守住
// radiusLG，但以内圆不超过外圆为界 —— Windows 上取 8，内圈和外圈是同一套弧，
// 不再出现旧规矩里「内圆比外圆更圆、角上一块楔形灰带」的样子（2026-10-30
// 实机反馈）；Linux 的外圆是 12，取 12 正好落在两条边界之间。
func shellCardRadiusFor(goos string) float32 {
	win := shellWindowRadiusFor(goos)
	return min(max(radiusLG, win-shellCardGap), win)
}

func shellCardRadius() float32 { return shellCardRadiusFor(runtime.GOOS) }

// 系统红绿灯的位置：关闭按钮的左上角离窗口左上角 18，灯心落在 30px 中线上
// （mygo 会把灯放在这个位置上方 1 像素，按钮 14 高，于是顶栏正好 60 高，
// 和侧栏、卡片头同一套节奏）。
const (
	trafficLightX = 18
	trafficLightY = shellBarMid - 7
)

// 窗口右侧三颗按钮（Windows / Linux）：28x26、间隔 2，右沿 18px（卡片右边
// 留白 8 + 内缩 10），和标题栏、侧栏共用同一条 30px 中线。
const (
	windowButtonW     = float32(28)
	windowButtonH     = float32(26)
	windowButtonGap   = float32(2)
	windowButtonRight = shellCardGap + 10
)

func windowButtonsWidth() float32 { return 3*windowButtonW + 2*windowButtonGap }

// windowButtonsReserved 是卡片工具栏右端要让给窗口按钮的宽度（从卡片右沿
// 算起）：按钮本身，加上与内容之间的 8px，再加上按钮离卡片边的 10px。
func windowButtonsReserved() float32 { return windowButtonsWidth() + 18 }

// windowControlsAtRight 报告窗口按钮画在右上角（Windows / Linux）还是左上角
// （macOS 的红绿灯）。做成变量是为了让测试能画另一边平台的样子。
var windowControlsAtRight = !isMacPlatform()

// windowControlsReservedRight 是卡片右上角留给自绘窗口按钮的宽度：macOS 的
// 按钮在左上，返回 0；全屏时按钮隐藏，也不留。
func (a *app) windowControlsReservedRight() float32 {
	if !windowControlsAtRight {
		return 0
	}
	if a.win != nil && a.win.IsFullScreen() {
		return 0
	}
	return windowButtonsReserved()
}

// windowControls 画窗口自己的按钮，只在 Windows / Linux 上：macOS 的三颗
// 红绿灯交给系统（main.go 用 TitleBarHiddenInset + TrafficLightPosition），
// 视图不画，只按 c.TitleBar() 报出来的宽度让位。
//
// 画在 overlay 里：窗口按钮和系统的一样浮在内容（包括设置这类浮层）之上，
// 不跟着侧栏、卡片或对话框一起动。
func (a *app) windowControls(c *ui.Context, pal palette) {
	if !windowControlsAtRight {
		return
	}
	ui.Overlay(c, func() {
		a.windowButtons(c, pal)
	})
}

// windowButtons 画 Windows 11 那三颗窗口按钮：最小化、最大化或还原、关闭，
// 平铺在右上角，关闭按钮悬停时变红。系统那三颗在 main.go 里由 Frameless
// 收走 —— 它们会压在卡片右上角的圆角和控件上。
func (a *app) windowButtons(c *ui.Context, pal palette) {
	if a.win != nil && a.win.IsFullScreen() {
		return
	}
	row := ui.Row(c.Key("window-controls")).Absolute().
		Right(windowButtonRight).Top((shellTopBar - windowButtonH) / 2).
		Height(windowButtonH).Gap(windowButtonGap).AlignItems(ui.Center)
	row.Children(func() {
		a.windowButton(c, pal, "window-minimize", iconMinimize,
			T("最小化窗口", "Minimize Window"), false, func() {
				if a.win != nil {
					a.win.Minimize()
				}
			})

		maximized := a.win != nil && a.win.IsMaximized()
		svg, label := iconMaximize, T("最大化窗口", "Maximize Window")
		if maximized {
			svg, label = iconRestore, T("还原窗口", "Restore Window")
		}
		a.windowButton(c, pal, "window-maximize", svg, label, false, func() {
			if a.win == nil {
				return
			}
			if maximized {
				a.win.Unmaximize()
			} else {
				a.win.Maximize()
			}
		})

		a.windowButton(c, pal, "window-close", iconX,
			T("关闭窗口", "Close Window"), true, func() {
				if a.win != nil {
					a.win.Close()
				}
			})
	})
}

// windowButton 是其中一颗：平时只是一枚安静的图标，悬停铺一层底色；关闭
// 那颗悬停变红底白字，和 Windows 11 一致。
func (a *app) windowButton(c *ui.Context, pal palette, key string, svg *ui.SVG,
	label string, dangerous bool, run func()) {
	b := ui.ButtonBase(c.Key(key)).Size(windowButtonW, windowButtonH).Radius(radiusSM).
		Center().Cursor(ui.CursorPointer).Label(label).Tooltip(label)
	col := pal.textMuted
	switch {
	case dangerous && (b.Hovered() || b.Pressed()):
		b.Background(pal.danger)
		col = ui.RGB(255, 255, 255)
	case b.Pressed():
		b.Background(pal.rowHover.Alpha(0.16))
		col = pal.text
	case b.Hovered():
		b.Background(pal.rowHover)
		col = pal.text
	}
	b.Children(func() {
		drawIcon(c, svg, 16, col)
	})
	if b.Clicked() {
		run()
	}
}

// shellBackdrop paints the window ground under the cards and the sidebar.
//
// 亚克力模式：有系统材质时（macOS、Windows 11 22H2+）窗口底是主题色的一层
// 薄纱盖在磨砂材质上 —— 材质给纹理，主题色决定这块窗口读起来是深还是浅。
// 不能把底色整个交给材质：材质跟的是系统外观，不是应用里固定的深 / 浅色。
// 深色应用碰上浅色系统时，侧栏和卡片四周会亮成一片，和深色的卡片、提交栏
// 拼在一起像没上完色（2026-10-30 实机截图）。剩下的部分留给材质透出背后
// 的内容，模糊仍然看得见。
//
// 没有材质时（Linux、旧 Windows）直接铺实色 sideBg：透明底不模糊，透出来
// 的桌面只会让文字没法读。
//
// 这里不再画深色那条 Oklab 渐变和紫辉光：那是「纯色洗底」时代的底，
// 现在深色的颜色由这支 sideBg 确定，叠一条渐变只会把材质糊掉。
func shellBackdrop(c *ui.Context, pal palette) {
	c.Root().Background(shellGround(pal, c.Vibrancy()))
}

// shellTint 是窗口底那层主题色的不透明度：材质最亮（正午的白墙）也要把
// 侧栏的次要文字压到 4.5:1 以上（见 TestShellGroundKeepsDarkThemeDark）。
const shellTint = 0.65

// shellGround 是窗口底这一帧该铺的颜色：有材质时是主题色薄纱，没有材质时
// 是实色 sideBg。拆成纯函数，测试才不用起视图。
func shellGround(pal palette, vibrancy bool) ui.Color {
	if vibrancy {
		return pal.sideBg.Alpha(shellTint)
	}
	return pal.sideBg
}

// raisedSurface lifts the main card and controls off the background.
func raisedSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return darkSurfaceShadow(litSurfaceEdge(e, pal))
	}
	// 浅色卡片也描一圈 1px 硬边。窗口自己那圈是系统画的硬线，卡片只靠阴影的
	// 话，角上是一团软灰、窗口是一根硬线 —— 同一个角两种质感，看着不像一套
	// 东西（§3 的内外同心讲的是弧度，这条讲的是描边）。
	//
	// 用 0 模糊 / 1 扩散的外阴影，不用 Border：Border 会像内边距一样把卡片里的
	// 内容往里挤 1px（文件栏那几条「卡片间距 8 + 内边距 12」就全偏了）；外阴影
	// 只画在卡片外面，贴出一条 1px 的硬边，不动布局。
	return e.Shadow(0, 0, 0, 1, pal.cardBorder).
		Shadow(0, 1, 2, 0, pal.cardShadow).
		Shadow(0, 1, 6, 0, pal.cardShadow2)
}

// selectedSurface is the restrained lift used by selected sidebar rows.
//
// 深色给选中行抬升（描边 + 阴影）；浅色什么都不加 —— rowSelected 的中性
// 灰底本身就是完整的选中态，再叠白胶囊那套投影，画出来就是一颗输入框。
func selectedSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return raisedSurface(e, pal)
	}
	return e
}

// floatingSurface lifts menus and dialogs above the cards.
func floatingSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return darkSurfaceShadow(litSurfaceEdge(e, pal)).
			Shadow(0, 16, 40, 0, pal.floatShadow)
	}
	// 浅色浮层和浅色卡片同一条规矩（见 raisedSurface）：只靠一层软阴影，
	// 边在这片浅灰上就是一团化开的灰 —— 压在亚克力底上尤其糊。补一条
	// 0 模糊 / 1 扩散的硬边，只画在外沿，不动布局。
	return e.Shadow(0, 0, 0, 1, pal.cardBorder).
		Shadow(0, 1, 3, 0, pal.cardShadow).
		Shadow(0, 16, 40, 0, pal.floatShadow)
}

// popoverSurface gives menus a light top edge in dark mode and a quiet warm
// white gradient in light mode.
func popoverSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return e.Background(pal.popover).
			InsetShadow(0, 0, 0, 1, ui.RGBA(255, 255, 255, 0.12)).
			Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.25)).
			Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.40))
	}
	return e.LinearGradient(ui.LinearGradient{
		Angle: 180,
		From:  ui.Hex("#fbfbfb"),
		To:    ui.Hex("#f4f4f5"),
	}).
		InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.90)).
		InsetShadow(0, -1, 0, 0, ui.RGBA(255, 255, 255, 0.90)).
		InsetShadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.30)).
		Shadow(0, 5, 10, 0, ui.RGBA(0, 0, 0, 0.22))
}

// litSurfaceEdge makes a dark surface read as lit from above.
func litSurfaceEdge(e ui.Element, pal palette) ui.Element {
	return e.Border(1, pal.text.Alpha(0.12)).
		InsetShadow(0, 1, 0, 0, pal.text.Alpha(0.18))
}

func darkSurfaceShadow(e ui.Element) ui.Element {
	return e.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.20)).
		Shadow(0, 2, 2, -1, ui.RGBA(0, 0, 0, 0.20)).
		Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
}

func shellIconButton(c *ui.Context, pal palette, label, tip string, icon *ui.SVG, disabled bool, fn func()) bool {
	btn := shellIconElement(c, pal, label, tip, icon, disabled)
	if btn.Clicked() && !disabled {
		fn()
		return true
	}
	return false
}

func shellIconElement(c *ui.Context, pal palette, label, tip string, icon *ui.SVG, disabled bool) ui.Element {
	btn := ui.ButtonBase(c).Size(shellIconButtonW, shellIconButtonH).Radius(radiusSM).Center().
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Disabled(disabled)
	col := pal.textMuted
	switch {
	case disabled:
		col = pal.textFaint.Alpha(0.55)
		btn.Cursor(ui.CursorDefault)
	case btn.Pressed():
		btn.Background(pal.rowHover.Alpha(0.12))
		col = pal.text
	case btn.Hovered():
		btn.Background(pal.rowHover)
		col = pal.text
	}
	fadeColors(btn)
	btn.Children(func() {
		drawIcon(c, icon, 16, col)
	})
	return btn
}

func shellDivider(c *ui.Context, pal palette) {
	ui.Box(c).Height(1).FillWidth().Shrink(0).Background(pal.hairline)
}

func shellSectionLabel(c *ui.Context, pal palette, label string) {
	ui.Text(c, label).FontSize(10.5).FontWeight(weightSemibold).
		TextColor(pal.textFaint).SingleLine().Padding(0, 2)
}
