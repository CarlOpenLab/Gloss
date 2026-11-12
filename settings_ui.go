package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// 这一份是设置窗口的「积木」，照 agent-desktop-ui 的设计规范搭：
//
//   - 分组是一块 12px 圆角的软面板（pal.chip），不是带 1px 黑边的卡片；
//   - 组内的行用 11px 的上下留白撑开，行与行之间只有一道 7% 的发丝线；
//   - 行标题 13.5px 正文色，说明 12px 次要色，右侧控件 24~30px 高；
//   - 控件（开关、分段、输入框、按钮）照 §5 的尺寸与状态来。
//
// 设置里所有页面共用这里的函数，样子才不会一页一个说法。

const (
	settingsNavW     = 212
	settingsCardGap  = 8
	settingsBarH     = 44
	settingsFooterH  = 52
	settingsPadX     = 28
	settingsPadTop   = 18
	settingsPadBot   = 28
	settingsGroupR   = 12
	settingsRowV     = 11
	settingsControlH = 26

	// settingsCardRadius 是设置窗口里那张卡的圆角。它不再跟着主窗口的
	// shellCardRadius 走：这个浮窗的外框是 radiusXL(16)，按「内 = 外 − 间距」
	// 算，内卡该是 8 —— 那张卡片会比现在方得多，是另一次版式决定。眼下先
	// 留住 14，和主工作卡分开取值。
	settingsCardRadius = 14

	// settingsNavPad 是导航栏自己的左右留白（没有系统窗口控件时）；
	// settingsBarLeadGap 是内容让开系统窗口控件之后与它之间再留的一道间距。
	//
	// 设置窗口的标题栏是 TitleBarHidden（settings_window.go），系统于是把
	// 窗口控件画在窗口的顶上：macOS 是三颗红绿灯（左上），Windows / Linux
	// 是三颗窗口按钮（右上）。视图不按 c.TitleBar() 让位的话，它们就压在
	// 内容上 —— 用户报的「布局乱了」正是这一件（截图里「设置」两个字横在
	// 红灯和黄灯中间）。
	settingsNavPad     = float32(12)
	settingsBarLeadGap = float32(12)
)

// settingsGroupRows counts the rows of the group being built so that the
// hairline goes between them, never at the outer edges (as macOS does).
// The build is single-threaded, so one counter is enough.
var settingsGroupRows int

// settingsGroupTitle is a group's heading: 12px, semibold, secondary.
func settingsGroupTitle(c *ui.Context, pal palette, title string) {
	ui.Text(c, title).FontSize(uiFontSize(textBodySM)).FontWeight(weightSemibold).
		TextColor(pal.textMuted).Padding(0, 4)
}

// settingsGroupNote is the quiet line under a group.
func settingsGroupNote(c *ui.Context, pal palette, note string) {
	ui.Text(c, note).FontSize(uiFontSize(textBodySM)).TextColor(pal.textFaint).
		LineHeight(1.4).Padding(0, 4)
}

// settingsGroup is a titled set of rows on a soft panel.
func settingsGroup(c *ui.Context, pal palette, title, note string, fn func()) {
	ui.Column(c).Gap(8).FillWidth().Children(func() {
		if title != "" {
			settingsGroupTitle(c, pal, title)
		}
		if fn != nil {
			settingsGroupRows = 0
			ui.Column(c).Radius(settingsGroupR).Background(pal.chip).
				FillWidth().Padding(4, 16).Children(fn)
		}
		if note != "" {
			settingsGroupNote(c, pal, note)
		}
	})
}

// settingsRow is one row of a group: label, optional detail, and its
// control on the right. Consecutive rows get a hairline between them.
func settingsRow(c *ui.Context, pal palette, label, detail string, control func()) {
	if settingsGroupRows++; settingsGroupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.rowDivider)
	}
	ui.Row(c).Padding(settingsRowV, 0).Gap(16).AlignItems(ui.Center).FillWidth().
		Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, label).FontSize(uiFontSize(textBody)).TextColor(pal.text).SingleLine()
				if detail != "" {
					ui.Text(c, detail).FontSize(uiFontSize(textBodySM)).TextColor(pal.textMuted).
						LineHeight(1.35)
				}
			})
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
		})
}

// settingsMiniButton is a compact control for steppers and secondary
// actions inside a row: 24px high, 6px radius, a quiet control fill.
//
// 皮和界面上其余控件同一套：洗色底 + 1px 发丝边 + inkMuted 文字，悬停 /
// 按下沿洗色阶梯升（widgets.go 的 secondaryAction）。这里以前走的是拟态那一套
// —— darkSurfaceShadow(litSurfaceEdge(…))，深色下三层外投影加一道内高光，
// 于是同一个设置页里，−/＋ 是拟态、旁边的分段控件和开关不是（issue #7 问题三
// 问的就是这个）。层级靠明度阶梯和发丝边表达，控件不投投影（DESIGN.md §4.1）。
func settingsMiniButton(c *ui.Context, pal palette, label, text string, disabled bool) ui.Element {
	btn := ui.ButtonBase(c).Height(24).Shrink(0).Padding(0, 8).Radius(radiusSM).
		Center().Label(label).Disabled(disabled).Cursor(ui.CursorPointer)
	var bg, border, fg ui.Color
	switch {
	case disabled:
		bg, border, fg = ui.Transparent, pal.hairline, pal.textFaint
	case btn.Pressed():
		bg, border, fg = pal.rowSelected, pal.hairlineStrong, pal.ink
	case btn.Hovered():
		bg, border, fg = pal.rowSelected, pal.hairlineStrong, pal.ink
	default:
		bg, border, fg = pal.control, pal.hairline, pal.text
	}
	btn.Background(btn.AnimateColor("bg", bg, ui.Animation{Duration: 90 * time.Millisecond})).
		Border(1, border).TextColor(fg)
	fadeColors(btn)
	btn.Children(func() {
		ui.Text(c, text).FontSize(uiFontSize(textBodySM)).FontWeight(weightMedium).SingleLine()
	})
	return btn
}

// settingsFieldRow is a group row with its label above its control, for
// text fields that need the full width of a narrow panel.
func settingsFieldRow(c *ui.Context, pal palette, label, detail string, field func()) {
	if settingsGroupRows++; settingsGroupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.rowDivider)
	}
	ui.Column(c).Padding(settingsRowV, 0).Gap(6).FillWidth().Children(func() {
		ui.Text(c, label).FontSize(uiFontSize(textBody)).TextColor(pal.text).SingleLine()
		if detail != "" {
			ui.Text(c, detail).FontSize(uiFontSize(textBodySM)).TextColor(pal.textMuted).LineHeight(1.35)
		}
		field()
	})
}

// settingsCardHeader is the 44px bar at the top of the floating card.
// The title is the page's name; the right side holds the window's own
// controls (the close button here).
//
// 右边按 c.TitleBar() 让位：设置窗口是 TitleBarHidden（settings_window.go），
// Windows / Linux 于是把三颗窗口按钮画在窗口右上角，正压在这一排的关闭按钮上
// —— 主窗口的工具栏是同一条算式（app.go 的 padRight）。macOS 的红绿灯在左上，
// Right 是 0，这时只留卡片自己那 12。
func settingsCardHeader(c *ui.Context, pal palette, title string, tools func()) {
	bar := c.TitleBar()
	padRight := settingsNavPad
	if bar.Right > 0 {
		padRight = bar.Right + settingsBarLeadGap
	}
	ui.Row(c).Height(settingsBarH).Shrink(0).FillWidth().
		Padding(0, padRight, 0, 16).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 0, 0).Children(func() {
		ui.Text(c, title).FontSize(uiFontSize(textHeadline)).FontWeight(weightSemibold).
			TextColor(pal.text).SingleLine()
		ui.Spacer(c)
		if tools != nil {
			tools()
		}
	})
}

// settingsIconButton is a quiet square button for the card header.
func settingsIconButton(c *ui.Context, pal palette, label, tip string, icon *ui.SVG, action func()) bool {
	btn := ui.ButtonBase(c).Size(26, 26).Radius(radiusSM).Center().
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip).FocusRing(false)
	if btn.Hovered() {
		btn.Background(pal.rowHover)
	}
	fadeColors(btn)
	btn.Children(func() {
		drawIcon(c, icon, 14, pal.textMuted)
	})
	if btn.Clicked() {
		action()
		return true
	}
	return false
}

// settingsButton is the settings window's button: the shared wash control
// face, or the accent's for the primary action.
//
// 和 primaryAction / secondaryAction 同一套（widgets.go）：静息洗色底 + 1px 发丝
// 边，悬停 / 按下沿洗色阶梯升。以前深色走拟态（三层外投影 + 内高光）、浅色走
// 一层内阴影加一层外投影 —— 同一设置页里这颗是拟态、顶栏「关闭设置」不是，
// 用户读到的是两三套互相不认识的语言（issue #7 问题三）。
//
// 主按钮和 primaryAction 一样靠**有没有实心底**分主次，不靠投影：实心强调色
// 的是「就这么办」，洗色的是「再想想」。
func settingsButton(c *ui.Context, pal palette, label string, primary, disabled bool) ui.Element {
	btn := ui.ButtonBase(c).Height(30).Shrink(0).Padding(0, 14).Gap(8).
		Radius(radiusMD).AlignItems(ui.Center).Center().
		Label(label).Disabled(disabled).Cursor(ui.CursorPointer)
	var bg, border, fg ui.Color
	switch {
	case disabled:
		bg, border, fg = pal.chip, pal.hairline, pal.inkTertiary
	case primary:
		bg, border, fg = pal.accent, pal.accent, ui.Hex("#ffffff")
		if btn.Hovered() {
			bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
		}
		if btn.Pressed() {
			bg = bg.Mix(ui.Hex("#000000"), 0.08)
		}
	case btn.Pressed(), btn.Hovered():
		bg, border, fg = pal.rowSelected, pal.hairlineStrong, pal.ink
	default:
		bg, border, fg = pal.chip, pal.hairline, pal.inkMuted
	}
	btn.Background(btn.AnimateColor("bg", bg, ui.Animation{Duration: 90 * time.Millisecond})).
		Border(1, border).TextColor(fg)
	fadeColors(btn)
	btn.Children(func() {
		ui.Text(c, label).FontSize(uiFontSize(textBody)).FontWeight(weightMedium).SingleLine()
	})
	return btn
}
