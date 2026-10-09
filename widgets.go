package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// hoverMotion 是悬停、选中这类状态底色的过渡时长。
//
// 短到几乎察觉不到「动画」这回事，但底色不会再啪地跳过去：鼠标从一列行上
// 扫过时，那一串底色是连着亮起来的。再长就拖沓了 —— 指针早走了，颜色还在追。
const hoverMotion = 120 * time.Millisecond

// fadeColors 给一个「平时没脸、悬停或选中才上色」的元素挂上颜色过渡。
//
// 只开 Colors：这些元素的尺寸和位置跟着布局走，不该有动画 —— 列表滚动时
// 每一行都在动，再给位置加过渡就成了一片黏糊糊的挪动。
func fadeColors(e *ui.Element) *ui.Element {
	return e.Transition(ui.ElementTransition{Colors: true, Duration: hoverMotion})
}

// emptyNote 是列表、侧栏空着时的一行浅色提示。msg 为空就什么都不画 ——
// 有些「为什么空」是主区该说的事，侧栏跟着重复一遍反而乱。
func emptyNote(c *ui.Context, t *ui.Theme, msg string) {
	if msg == "" {
		return
	}
	ui.Box(c).Grow(1).Padding(16).Children(func() {
		ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted)
	})
}

// xBtnSize 是叉号按钮的边长（关闭、删除）。
//
// 不用 ui.Button(c, "×")：按钮宽度跟着字形走，一个 × 撑出来是又扁又小的
// 一条，跟旁边正常尺寸的按钮排在一起显得局促，点击区域也窄。这里把尺寸
// 定死成正方形。
const xBtnSize = 26

// closeButton 是右上角的关闭按钮（解释卡片、对话框）。
func closeButton(c *ui.Context) bool { return xButton(c, "关闭") }

// removeButton 是列表行右边的删除按钮（模型设置里的供应商列表）。
func removeButton(c *ui.Context) bool { return xButton(c, "删除") }

// sparkleSize 是 ✨ 图标的默认边长。
const sparkleSize = 11

// drawSparkle 画一颗四角星。
//
// 不打死 ✨：这类符号会落到彩色 emoji 字体上，各机器上的形状、颜色和基线
// 都不一样 —— 在 12px 上是一团糊的橙黄，而且没法跟着主题变色（浅色下和深色下
// drawSparkle 画一颗火花星（Lucide sparkle）。
func drawSparkle(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Icon(iconSparkle, r, c)
}

// sparkleIcon 是一个只有火花的方盒，用来摆在按钮或标题里。
func sparkleIcon(c *ui.Context, size float32, color ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		drawSparkle(p, r, color)
	})
}

// sparkleCluster 是星群（Lucide sparkles），用在卡片标题上。
func sparkleCluster(c *ui.Context, size float32, color ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Icon(iconSparkles, r, color)
	})
}

// shieldIcon 是一个盾牌图标方盒，用来摆在安全预检按钮或对话框标题中。
func shieldIcon(c *ui.Context, size float32, color ui.Color) {
	drawIcon(c, iconShield, size, color)
}

// explainBtnH 是 hunk 行上那颗小按钮的高度。比 hunk 行（26）矮一圈：上下
// 各留 3px。填满整行的按钮看着是「贴」在色带上的，四周留出缝才像放上去的
// 控件。
const explainBtnH = 20

// hunkButton 起一颗 hunk 行上的幽灵按钮。
//
// 不用 ui.Button：它自带的面 + 描边在蓝色 hunk 带上是一块突兀的白板，而
// 这类按钮每条 hunk 都有，是界面上出现次数最多的控件，值得按它所在的底色
// 单独调。
//
// 三态：常态是描边幽灵键（清楚可点，但不和代码抢视线）；悬停时底色提亮；
// on 时填成强调色（✨ 解释用它表示「这一段正在讲」）。
//
// ✨ 解释和暂存这一段共用这一套：两处各画各的话，一排按钮里一眼就能看出
// 是两种东西拼起来的。content 拿到的是这一档该用的前景色。
func hunkButton(c *ui.Context, pal palette, label, tip string, on bool, content func(fg ui.Color)) *ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Height(explainBtnH).Shrink(0).Radius(5).
		Padding(0, 7).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip)

	hovered := btn.Hovered()
	var bg, border, fg ui.Color
	switch {
	case on:
		bg, border, fg = t.Accent, ui.Transparent, t.AccentText
	case hovered:
		bg, border, fg = pal.explainHover, pal.hunkText.Alpha(0.7), pal.hunkText
	default:
		bg, border, fg = ui.Transparent, pal.hunkText.Alpha(0.38), pal.hunkText
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() { content(fg) })
	return btn
}

// explainButton 是 hunk 行右边那颗「✨ 解释」。
//
// 卡片开着的时候，按钮自己就是「哪一段在讲」的指示灯（on 时填成强调色）。
func explainButton(c *ui.Context, pal palette, on bool) *ui.Element {
	return hunkButton(c, pal, "解释", "让 AI 讲讲这段改了什么", on, func(fg ui.Color) {
		sparkleIcon(c, sparkleSize, fg)
		ui.Text(c, "解释").FontSize(11).SingleLine().TextColor(fg)
	})
}

// lineExplainButton 是代码行右侧悬停或激活时出现的「✨ 解释」小按钮。
func lineExplainButton(c *ui.Context, pal palette, on bool) *ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Height(18).Shrink(0).Radius(4).
		Padding(0, 5).Gap(3).AlignItems(ui.Center).Margin(0, 6, 0, 0).
		Cursor(ui.CursorPointer).Label("AI 解释这一行").Tooltip("让 AI 解释这一行代码")

	hovered := btn.Hovered()
	var bg, border, fg ui.Color
	switch {
	case on:
		bg, border, fg = t.Accent, ui.Transparent, t.AccentText
	case hovered:
		bg, border, fg = pal.explainHover, pal.hunkText.Alpha(0.7), pal.hunkText
	default:
		bg, border, fg = ui.Transparent, pal.hunkText.Alpha(0.38), pal.hunkText
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		sparkleIcon(c, 11, fg)
		ui.Text(c, "解释").FontSize(10).SingleLine().TextColor(fg)
	})
	return btn
}

// stageHunkButton 是「暂存 / 取消暂存这一段」。
//
// 按下的方向和文件行那颗勾选框一致：填色 = 已经在暂存区里，点它拿出来。
// 可见的字只有「暂存」两个字，而 Label 写成「暂存这一段」—— 无障碍名字和
// 测试找它的凭据都得说清「这一段的」，屏幕上的字则要短。
func stageHunkButton(c *ui.Context, pal palette, staged, lock bool) *ui.Element {
	label, text, tip := "暂存这一段", "暂存", "把这一段改动放进暂存区"
	if staged {
		label, text, tip = "取消暂存这一段", "取消暂存", "把这一段改动移出暂存区"
	}
	btn := hunkButton(c, pal, label, tip, false, func(fg ui.Color) {
		ui.Text(c, text).FontSize(11).SingleLine().TextColor(fg)
	})
	return btn.Disabled(lock)
}

// checkSize 是状态栏里那个小勾的边长。
const checkSize = 11

// drawCheckMark 画一个勾：两笔折线，和折叠箭头同一个画法。
//
// 不打「✓」字符，理由和叉号、箭头一样：这类几何符号会落到后备字体上，
// drawCheckMark 画一个勾（Lucide check）。
func drawCheckMark(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Icon(iconCheck, r, c)
}

// searchH 是筛选框的高度。比框架默认的输入框（边距 1.5+1.5 个间距单位）
// 略矮一点：它是个「随手筛一下」的框，不该和表单里那些正经输入框一样重。
const searchH = 28

// searchField 是列表上方那个筛选框：灰底、圆角、左边一颗放大镜，
// 没有描边 —— 焦点进来时才描一圈强调色。
//
// 不用 ui.SearchField：它的 placeholder 在框架里写死成英文 "Search"，
// 从外面盖不掉，而这个界面是中文的。形状照着它的样子自己拼：外面一个
// 有底色的行，里面一颗放大镜加一个无脸的输入框。输入框要的是
// TextInputBase —— 带脸的 TextInput 会在灰底上再叠一层自己的底和描边。
//
// 名字同时挂在框和输入框上：框是给测试和无障碍工具找的（返回的是整块
// 的位置，和上面那行文字左对齐），输入框是给读屏软件的（它才是可输入的
// 那个控件）。Find 取先遍历到的，也就是外面这块。
func searchField(c *ui.Context, value *string, placeholder string) *ui.Element {
	t := c.Theme()
	var in *ui.Element

	box := ui.Row(c).Height(searchH).Shrink(0).AlignItems(ui.Center).
		Gap(6).Padding(0, 8).Radius(t.Radius).Background(t.Surface).
		Label(placeholder).Children(func() {
		searchIcon(c, 12, t.TextMuted)
		in = ui.TextInputBase(c, value).Grow(1).MinWidth(0).
			Placeholder(placeholder).Label(placeholder)
	})

	// 描边一直占着 1px，只换颜色 —— 焦点来的那一帧才加边框的话，
	// 框会往里缩一圈，光标跟着跳一下。
	border := ui.Transparent
	switch {
	case in != nil && in.Focused():
		border = t.Accent
	case box.Hovered():
		border = t.Border
	}
	box.Border(1, border)
	return in
}

// searchIcon 是筛选框里那颗放大镜。
func searchIcon(c *ui.Context, size float32, color ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		drawMagnifier(p, r, color)
	})
}

// drawMagnifier 画放大镜（Lucide search）。
func drawMagnifier(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconSearch, r, col)
}

// toolIconSize 是工具栏图标方盒的边长。比 13px 的文字略大一圈 —— 图标和
// 文字视觉等重时，笔画密的图标看着会小一号。
const toolIconSize = 15

// toolbarDivider 画工具栏上的细分割线。
func toolbarDivider(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Width(1).Height(16).Shrink(0).Background(t.Border.Alpha(0.6))
}

// toolbarSegmentedText 是工具栏上的文字分段控件（如「改动 / 历史」）。
// 严格对齐 28px 高度基准网格，提供细腻的微投影与悬停平滑过渡。
func toolbarSegmentedText(c *ui.Context, selected *int, off bool, labels ...string) *ui.Element {
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(labels))
	g := parts.Track
	g.Height(28).AlignItems(ui.Center).Padding(2).Gap(2).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Disabled(off)

	g.Children(func() {
		for i, label := range labels {
			s := parts.Segment(i).Height(24).Padding(0, 10).Radius(t.Radius - 2).
				Cursor(ui.CursorPointer).Label(label)
			sel := i == *selected
			bg := ui.Transparent
			fg := t.TextMuted
			switch {
			case off:
				fg = t.TextMuted.Alpha(0.4)
			case sel:
				bg = t.Background
				fg = t.Text
				s.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.08))
				if t.Dark {
					s.Border(1, t.Border.Alpha(0.6))
				}
			case s.Hovered():
				bg = t.SurfaceHover
				fg = t.Text
			}
			s.Background(bg)
			fadeColors(s)
			s.Children(func() {
				ui.Text(c, label).FontSize(12).TextColor(fg).SingleLine()
			})
		}
	})
	return g
}

type segmentedIconItem struct {
	label string
	tip   string
	icon  *ui.SVG
}

// toolbarSegmentedIcons 是工具栏上的图标分段控件（如「Unified / Split」切换）。
// 紧凑、精确、高度一致（28px），两颗图标均采用 Lucide 标准矢量。
func toolbarSegmentedIcons(c *ui.Context, selected *int, off bool, items ...segmentedIconItem) *ui.Element {
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(items))
	g := parts.Track
	g.Height(28).AlignItems(ui.Center).Padding(2).Gap(2).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Disabled(off)

	g.Children(func() {
		for i, it := range items {
			s := parts.Segment(i).Size(26, 24).Radius(t.Radius - 2).
				Cursor(ui.CursorPointer).Label(it.label).Tooltip(it.tip)
			sel := i == *selected
			bg := ui.Transparent
			fg := t.TextMuted
			switch {
			case off:
				fg = t.TextMuted.Alpha(0.4)
			case sel:
				bg = t.Background
				fg = t.Text
				s.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.08))
				if t.Dark {
					s.Border(1, t.Border.Alpha(0.6))
				}
			case s.Hovered():
				bg = t.SurfaceHover
				fg = t.Text
			}
			s.Background(bg)
			fadeColors(s)
			s.Children(func() {
				ui.Box(c).Size(14, 14).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Icon(it.icon, r, fg)
				})
			})
		}
	})
	return g
}

// toolButton 是工具栏上的一颗动作按钮：矢量图标 + 文字标签。
// 高度严格对齐 28px 基准，悬停与按下提供顺滑微交互。
// 支持 loading 态：正在加载时内部 15x15 图标平滑切换为 Spinner 原地旋转，
// 保持按钮总尺寸与布局毫厘不差，彻底杜绝在按钮外侧横向弹出额外 icon 造成的剧烈布局偏移。
func toolButton(c *ui.Context, label, tip string, off, loading bool, icon func(p *ui.Painter, r ui.Rect, col ui.Color)) bool {
	t := c.Theme()
	disabled := off || loading
	btn := ui.ButtonBase(c).Height(28).Shrink(0).Radius(t.Radius).
		Padding(0, 9).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Disabled(disabled)

	bg := ui.Transparent
	fg := t.TextMuted
	switch {
	case disabled:
		fg = t.TextMuted.Alpha(0.38)
		btn.Cursor(ui.CursorDefault)
	case btn.Pressed():
		bg = t.SurfacePressed
		fg = t.Text
	case btn.Hovered():
		bg = t.SurfaceHover
		fg = t.Text
	}
	btn.Background(bg)
	fadeColors(btn)

	btn.Children(func() {
		if loading {
			ui.Spinner(c).Size(toolIconSize, toolIconSize)
		} else {
			ui.Box(c).Size(toolIconSize, toolIconSize).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) { icon(p, r, fg) })
		}
		ui.Text(c, label).FontSize(12).SingleLine().TextColor(fg)
	})
	return btn.Clicked() && !loading
}

// iconPt 把 0..1 的归一化坐标落进矩形里。
//
// 图标按比例画，不写死像素：同一段代码既能画 15px 的工具栏图标，也不会
// 在别的尺寸上变形。
func iconPt(r ui.Rect, x, y float32) (float32, float32) {
	return r.X + r.W*x, r.Y + r.H*y
}

// drawRefreshIcon 画「刷新」（Lucide rotate-cw）。
func drawRefreshIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconRefresh, r, col)
}

// drawSlidersIcon 画「设置」（Lucide sliders-horizontal）。
func drawSlidersIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconSliders, r, col)
}

// drawFolderIcon 画「打开仓库」（Lucide folder）。
func drawFolderIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconFolder, r, col)
}

// xButton 画一个方形叉号按钮，返回这一帧有没有被点中。
//
// 用 ButtonBase 而不是 Button：Button 自带底和描边，摆在列表行里像另一颗
// 普通按钮；叉号在没指上去之前应该只是淡淡的，hover 才浮出底色。
// ButtonBase 把样子全交出来，焦点、键盘和 role 还是照旧。
//
// 叉号是矢量画的，不打 "×" 字符：这类几何符号很容易落到后备字体上，
// 各机器上的大小和位置都不一样，还可能画成缺字方框 —— 和 diffview.go
// 里 foldChevron 是同一个道理。
func xButton(c *ui.Context, hint string) bool {
	t := c.Theme()

	btn := ui.ButtonBase(c).Size(xBtnSize, xBtnSize).Shrink(0).
		Radius(t.Radius).Cursor(ui.CursorPointer).
		// 按钮上没有文字，Label 是无障碍名字，也是测试找它的凭据；
		// Tooltip 给鼠标用户看一眼。
		Label(hint).Tooltip(hint)

	bg, color := ui.Transparent, t.TextMuted
	if btn.Hovered() {
		bg, color = t.SurfaceHover, t.Text
	}
	btn.Background(bg)
	fadeColors(btn)

	// 叉号放在一个居中的小盒子里，而不是直接画在按钮上：按钮是
	// Center() 的行，它的 Draw 拿到的是整块的矩形，和内容区差着几个像素，
	// 叉号会略微偏出去。让布局系统去居中就准了。
	// 两臂各占按钮边长的四成，四周留出来的就是点击余量。
	arm := float32(xBtnSize * 0.4)
	btn.Children(func() {
		ui.Box(c).Size(arm, arm).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			path.MoveTo(r.X, r.Y).LineTo(r.X+r.W, r.Y+r.H)
			path.MoveTo(r.X+r.W, r.Y).LineTo(r.X, r.Y+r.H)
			p.StrokePath(&path, 1.5, color)
		})
	})
	return btn.Clicked()
}
