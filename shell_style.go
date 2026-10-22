package main

import "github.com/egoist/mygo/ui"

// The shell follows a soft, layered desktop rhythm: the window chrome, the
// sidebar controls and each card header share one 30px center line.
const (
	shellBarMid     = 30
	shellTopBar     = 2 * shellBarMid
	shellHeaderH    = 2 * (shellBarMid - shellCardGap)
	shellCardGap    = 8
	shellCardRadius = 14
)

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

// shellBackdrop paints the window wash. The cards and sidebar sit directly
// on it, so the gradient gives the shell depth without adding another panel.
func shellBackdrop(c *ui.Context, pal palette, row ui.Element) {
	from, to := ui.Hex("#ebe0f3"), ui.Hex("#f6efcf")
	glow := ui.RGBA(247, 220, 241, 0.75)
	if pal.dark {
		from, to = ui.Hex("#17141f"), ui.Hex("#1b1916")
		glow = ui.RGBA(120, 70, 120, 0.22)
	}

	alpha := float32(1)
	if c.Vibrancy() {
		alpha = 0.95
	}
	c.Root().LinearGradient(ui.LinearGradient{
		Angle: 180,
		Oklab: true,
		From:  from.Alpha(alpha),
		To:    to.Alpha(alpha),
	})
	row.LinearGradient(ui.LinearGradient{
		Angle: 225,
		Oklab: true,
		From:  glow,
		To:    glow.Alpha(0),
	})
}

// raisedSurface lifts the main card and controls off the background.
func raisedSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return darkSurfaceShadow(litSurfaceEdge(e, pal))
	}
	return e.Shadow(0, 1, 2, 0, pal.cardShadow).
		Shadow(0, 1, 6, 0, pal.cardShadow2)
}

// selectedSurface is the restrained lift used by selected sidebar rows.
func selectedSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return raisedSurface(e, pal)
	}
	return e.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06)).
		Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.05))
}

// floatingSurface lifts menus and dialogs above the cards.
func floatingSurface(e ui.Element, pal palette) ui.Element {
	if pal.dark {
		return darkSurfaceShadow(litSurfaceEdge(e, pal)).
			Shadow(0, 16, 40, 0, pal.floatShadow)
	}
	return e.Shadow(0, 1, 3, 0, pal.cardShadow).
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
	btn := ui.ButtonBase(c).Size(28, 26).Radius(radiusSM).Center().
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
