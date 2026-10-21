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

// 自绘红绿灯的尺寸：三颗 12px 的圆点，各带 16px 的点击区、4px 间隔。
// 左起 12px，右沿 68px —— 工具栏与侧栏顶栏都按这个右沿让位。
const (
	trafficDot  = 12
	trafficHit  = 16
	trafficGap  = 4
	trafficLeft = 12
)

func trafficLightsRight() float32 {
	return trafficLeft + 3*trafficHit + 2*trafficGap
}

// windowControlsReserved 是窗口左上角留给自绘红绿灯的宽度（含与内容的
// 间距）。非 macOS 上原生按钮不在这里，返回 0；全屏时红绿灯隐藏，也不留。
func (a *app) windowControlsReserved() float32 {
	if !isMacPlatform() {
		return 0
	}
	if a.win != nil && a.win.IsFullScreen() {
		return 0
	}
	return trafficLightsRight() + shellCardGap
}

// windowTrafficLights 画 macOS 的红黄绿三颗窗口按钮。
//
// 系统那三颗在 main.go 里由 Frameless 藏掉，改由视图自己画：它们和侧栏、
// 卡片共用同一条 30px 中线，颜色、悬停符号、失焦时的灰阶都照 AppKit 的
// 那一套来。点红关窗（走 OnClose，提交草稿和终端照常收尾），点黄最小化，
// 点绿缩放。
func (a *app) windowTrafficLights(c *ui.Context, pal palette) {
	if !isMacPlatform() {
		return
	}
	if a.win != nil && a.win.IsFullScreen() {
		return
	}

	focused := true
	if a.win != nil {
		focused = a.win.IsFocused()
	}

	// 整行是一个透明的容器：指针落在三颗之间的空隙上也算「悬停红绿灯」，
	// 三颗一起露出符号，和 macOS 一样。
	row := ui.Row(c).Key("window-traffic-lights").Absolute().
		Left(trafficLeft).Top(0).Height(shellTopBar).
		Gap(trafficGap).AlignItems(ui.Center).PassThrough()
	row.Children(func() {
		closeDot := ui.Hex("#ff5f57")
		minDot := ui.Hex("#febc2e")
		zoomDot := ui.Hex("#28c840")
		if !focused {
			closeDot, minDot, zoomDot = ui.Hex("#c8c8cc"), ui.Hex("#c8c8cc"), ui.Hex("#c8c8cc")
			if pal.dark {
				closeDot, minDot, zoomDot = ui.Hex("#4a4a50"), ui.Hex("#4a4a50"), ui.Hex("#4a4a50")
			}
		}

		const (
			trafficClose = iota
			trafficMinimize
			trafficZoom
		)
		trafficButton := func(dot ui.Color, label string, glyphKind int, action func()) {
			btn := ui.ButtonBase(c).Size(trafficHit, trafficHit).Radius(radiusPill).
				Center().Cursor(ui.CursorPointer).Label(label).FocusRing(false)
			btn.Children(func() {
				ui.Box(c).Size(trafficHit, trafficHit).Draw(func(p *ui.Painter, r ui.Rect) {
					d := float32(trafficDot)
					cx, cy := r.X+r.W/2, r.Y+r.H/2
					dotRect := ui.Rect{X: cx - d/2, Y: cy - d/2, W: d, H: d}
					p.Fill(dotRect, dot, d/2)
					if !row.Hovered() {
						return
					}
					glyph := ui.RGBA(0, 0, 0, 0.62)
					switch glyphKind {
					case trafficClose:
						p.Line(cx-2.6, cy-2.6, cx+2.6, cy+2.6, 1.25, glyph)
						p.Line(cx+2.6, cy-2.6, cx-2.6, cy+2.6, 1.25, glyph)
					case trafficMinimize:
						p.Line(cx-3, cy, cx+3, cy, 1.25, glyph)
					default:
						// 缩放：两个相对的三角，AppKit 的样子。
						p.Line(cx-3, cy, cx, cy-3, 1.25, glyph)
						p.Line(cx, cy-3, cx, cy, 1.25, glyph)
						p.Line(cx, cy, cx+3, cy+3, 1.25, glyph)
						p.Line(cx+3, cy+3, cx, cy+3, 1.25, glyph)
					}
				})
			})
			if btn.Clicked() && a.win != nil {
				action()
			}
		}

		trafficButton(closeDot, T("关闭窗口", "Close Window"), trafficClose, func() { a.win.Close() })
		trafficButton(minDot, T("最小化窗口", "Minimize Window"), trafficMinimize, func() { a.win.Minimize() })
		trafficButton(zoomDot, T("缩放窗口", "Zoom Window"), trafficZoom, func() { a.win.ToggleMaximize() })
	})
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
