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

// chipDot 是规范芯片上那颗语义圆点的直径（§2.5）。
const chipDot = 6

// 行内芯片的尺寸：提交行（30 高）和树行（24 高）里那些引用芯片共用一档。
//
// 比正文小一号、比对话框里的芯片紧一点：一行里可能并排四枚，一屏要读几十枚，
// 芯片抢过分支名的注意力就没意义了。字号 10 不在规范的六档里 —— 这是既有的
// 「密集元信息」档，按角色收敛时保留了它。
const (
	rowChipH    = 16.0
	rowChipText = 10.0
)

// 提交栏与对话框里的芯片：比行内芯片大一档，和旁边的正文、按钮排在一起不失衡。
const (
	statusChipH    = 20.0
	statusChipText = textCaption
)

// specChip 起一颗「规范芯片」：Surface 2 底 + 1px 发丝边 + Ink Muted 文字。
//
// 语义 —— 本地分支 / 远程 / 标签 / 严重度 / 状态 —— 全靠 dot 那颗 6px 圆点带
// 出来，而不是把整块芯片涂成琥珀、亮蓝、翠绿。这不是洁癖：提交行一行能挂四枚
// 引用芯片，逐个上色就是 §5 点名的「彩色药丸大爆炸」，一行扫过去全是色块，
// 「这是哪条分支」反而更难认。dot 传 ui.Transparent 就得到一颗纯文字芯片。
//
// 高度和字号由调用方给：芯片出现在 24px 的树行、30px 的提交行和对话框里，
// 尺寸不一样，但底、边、圆点这三样必须一样 —— 那是「它是什么」的部分。
func specChip(c *ui.Context, pal palette, label string, dot ui.Color, height, fontSize float32) {
	ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).Gap(4).
		Padding(0, 6).Radius(radiusXS).
		Background(pal.surface2).Border(1, pal.hairline).Children(func() {
		if dot.A > 0 {
			ui.Box(c).Size(chipDot, chipDot).Shrink(0).
				Radius(chipDot / 2).Background(dot)
		}
		ui.Text(c, label).FontSize(fontSize).FontWeight(weightMedium).
			SingleLine().TextColor(pal.inkMuted)
	})
}

// headline 是空状态、出错态那张大标题（§typography 的 display 档）。
//
// 20 / 600。原来十一处空状态写的都是 `FontSize(18).Bold()`：18 不在规范的
// 六档里，700 也比规范留给标题的 600 重一档。单看一处看不出问题，十一处
// 摆在一起就是另一套排版了。降到 600 之后字号虽涨了一档，在深色底上反而
// 更稳 —— 这一档是给「一屏只有一句话」的场景用的，不是给列表行用的。
func headline(c *ui.Context, pal palette, msg string) {
	ui.Text(c, msg).FontSize(textDisplay).FontWeight(weightSemibold).TextColor(pal.ink)
}

// emptyNote 是列表、侧栏空着时的一行浅色提示。msg 为空就什么都不画 ——
// 有些「为什么空」是主区该说的事，侧栏跟着重复一遍反而乱。
func emptyNote(c *ui.Context, t *ui.Theme, msg string) {
	if msg == "" {
		return
	}
	ui.Box(c).Grow(1).Padding(16).Children(func() {
		ui.Text(c, msg).FontSize(textBodySM).TextColor(t.TextMuted)
	})
}

// xBtnSize 是叉号按钮的边长（关闭、删除）。
//
// 不用 ui.Button(c, "×")：按钮宽度跟着字形走，一个 × 撑出来是又扁又小的
// 一条，跟旁边正常尺寸的按钮排在一起显得局促，点击区域也窄。这里把尺寸
// 定死成正方形。
const xBtnSize = 26

// closeButton 是右上角的关闭按钮（对话框）。
func closeButton(c *ui.Context, pal palette) bool { return xButton(c, pal, "关闭") }

// removeButton 是列表行右边的删除按钮（模型设置里的供应商列表）。
func removeButton(c *ui.Context, pal palette) bool { return xButton(c, pal, "删除") }

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

// hunkBtnH 是 hunk 行上那颗小按钮的高度。比 hunk 行（26）矮一圈：上下
// 各留 3px。填满整行的按钮看着是「贴」在色带上的，四周留出缝才像放上去的
// 控件。
const hunkBtnH = 20

// hunkButton 起一颗 hunk 行上的幽灵按钮。
//
// 不用 ui.Button：它自带的面 + 描边在 hunk 带上是一块突兀的白板，而
// 这类按钮每条 hunk 都有，是界面上出现次数最多的控件，值得按它所在的底色
// 单独调。
//
// 两态：常态是描边幽灵键（清楚可点，但不和代码抢视线）；悬停时底色提亮。
// content 拿到的是这一档该用的前景色。
//
// 圆角取 sm(6) 而不是通用按钮的 md(8)：§4.1 把幽灵键单列一档，它比按钮轻，
// 圆角也跟着收一点。字号 11 同样比通用按钮小 —— 这几个字夹在代码行之间，
// 和行号一个量级，不该比代码还抢眼。
func hunkButton(c *ui.Context, pal palette, label, tip string, content func(fg ui.Color)) *ui.Element {
	btn := ui.ButtonBase(c).Height(hunkBtnH).Shrink(0).Radius(radiusSM).
		Padding(0, 7).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip)

	bg, border, fg := ui.Transparent, pal.hunkText.Alpha(0.38), pal.hunkText
	if btn.Hovered() {
		bg, border, fg = pal.hunkHover, pal.hunkText.Alpha(0.7), pal.hunkText
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() { content(fg) })
	return btn
}

// stageHunkButton 是「暂存 / 取消暂存这一段」。
//
// 按下的方向和文件行那颗勾选框一致：填色 = 已经在暂存区里，点它拿出来。
// 可见的字只有「暂存」两个字，而 Label 写成「暂存这一段」—— 无障碍名字和
// 测试找它的凭据都得说清「这一段的」，屏幕上的字则要短。
func stageHunkButton(c *ui.Context, pal palette, staged, lock bool) *ui.Element {
	label, text, tip := T("暂存这一段", "Stage hunk"), T("暂存", "Stage"), T("把这一段改动放进暂存区", "Stage this hunk in index")
	if staged {
		label, text, tip = T("取消暂存这一段", "Unstage hunk"), T("取消暂存", "Unstage"), T("把这一段改动移出暂存区", "Unstage this hunk from index")
	}
	btn := hunkButton(c, pal, label, tip, func(fg ui.Color) {
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

// drawMinusMark 画一个减号/横线（Lucide minus，用于半选态）。
func drawMinusMark(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Icon(iconMinus, r, c)
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
func searchField(c *ui.Context, pal palette, value *string, placeholder string) *ui.Element {
	t := c.Theme()
	var in *ui.Element

	// 搜索栏在 §3 里明确归到 md(8)，和文本输入框同一档 —— 它就是个输入框。
	box := ui.Row(c).Height(searchH).Shrink(0).AlignItems(ui.Center).
		Gap(6).Padding(0, 8).Radius(radiusMD).Background(pal.surface1).
		Border(1, pal.hairline).
		Label(placeholder).Children(func() {
		searchIcon(c, 12, pal.inkSubtle)
		in = ui.TextInputBase(c, value).Grow(1).MinWidth(0).
			Placeholder(placeholder).Label(placeholder)
		if value != nil && *value != "" {
			clearBtn := ui.ButtonBase(c).Size(16, 16).Radius(radiusPill).Center().
				Cursor(ui.CursorPointer).Label("清空").Tooltip("清空筛选")
			if clearBtn.Hovered() {
				clearBtn.Background(pal.surface3)
			}
			fadeColors(clearBtn)
			clearBtn.Children(func() {
				drawIcon(c, iconX, 10, pal.inkSubtle)
			})
			if clearBtn.Clicked() {
				*value = ""
			}
		}
	})

	// 描边一直占着 1px，只换颜色 —— 焦点来的那一帧才加边框的话，
	// 框会往里缩一圈，光标跟着跳一下。
	//
	// 三档照 §2.2 取：静息是 hairline-strong（规范里「输入框外框」用的就是它，
	// 比普通分割线重一档，输入框才立得起来），悬停再升一档到 tertiary，
	// 聚焦换成强调色。
	border := pal.hairlineStrong
	switch {
	case in != nil && in.Focused():
		border = t.Accent
	case box.Hovered():
		border = pal.hairlineTertiary
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
func toolbarDivider(c *ui.Context, pal palette) {
	ui.Box(c).Width(1).Height(16).Shrink(0).Background(pal.hairline)
}

// toolbarSegmentedText 是工具栏上的文字分段控件（如「改动 / 历史」）。
//
// 轨道给 md(8)、单元给 sm(6)：单元嵌在轨道里，四周各留着 2px 内边距，
// 8 - 2 = 6 正好同心 —— 两个圆角各自独立取值的话，内圆会顶到外圆的弧上。
func toolbarSegmentedText(c *ui.Context, pal palette, selected *int, off bool, labels ...string) *ui.Element {
	parts := ui.SegmentedBase(c, selected, len(labels))
	g := parts.Track
	g.Height(28).AlignItems(ui.Center).Padding(2).Gap(2).Radius(radiusMD).
		Background(pal.surface1).Border(1, pal.hairline).Disabled(off)

	g.Children(func() {
		for i, label := range labels {
			s := parts.Segment(i).Height(24).Padding(0, 10).Radius(radiusSM).
				Cursor(ui.CursorPointer).Label(label)
			sel := i == *selected
			bg := ui.Transparent
			fg := pal.inkMuted
			switch {
			case off:
				fg = pal.inkTertiary
			case sel:
				bg = pal.surface3
				fg = pal.ink
				s.Border(1, pal.hairlineStrong)
			case s.Hovered():
				bg = pal.surface2
				fg = pal.ink
			}
			s.Background(bg)
			fadeColors(s)
			s.Children(func() {
				ui.Text(c, label).FontSize(textBodySM).FontWeight(weightMedium).
					TextColor(fg).SingleLine()
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
// 与文字版同一套取值：轨道 md(8)、单元 sm(6)、图标用 Lucide 标准矢量。
func toolbarSegmentedIcons(c *ui.Context, pal palette, selected *int, off bool, items ...segmentedIconItem) *ui.Element {
	parts := ui.SegmentedBase(c, selected, len(items))
	g := parts.Track
	g.Height(28).AlignItems(ui.Center).Padding(2).Gap(2).Radius(radiusMD).
		Background(pal.surface1).Border(1, pal.hairline).Disabled(off)

	g.Children(func() {
		for i, it := range items {
			s := parts.Segment(i).Size(26, 24).Radius(radiusSM).
				Cursor(ui.CursorPointer).Label(it.label).Tooltip(it.tip)
			sel := i == *selected
			bg := ui.Transparent
			fg := pal.inkMuted
			switch {
			case off:
				fg = pal.inkTertiary
			case sel:
				bg = pal.surface3
				fg = pal.ink
				s.Border(1, pal.hairlineStrong)
			case s.Hovered():
				bg = pal.surface2
				fg = pal.ink
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
//
// 照 §4.1 的 Ghost / Tool Button 走：透明底、悬停 Surface 2、圆角 sm(6)、
// 图标由 Ink Subtle 提到 Ink。它和主按钮不是一档东西 —— 圆角也小一号。
//
// 支持 loading 态：正在加载时内部 15x15 图标平滑切换为 Spinner 原地旋转，
// 保持按钮总尺寸与布局毫厘不差，彻底杜绝在按钮外侧横向弹出额外 icon 造成的剧烈布局偏移。
func toolButton(c *ui.Context, pal palette, label, tip string, off, loading bool, icon func(p *ui.Painter, r ui.Rect, col ui.Color)) bool {
	disabled := off || loading
	btn := ui.ButtonBase(c).Height(28).Shrink(0).Radius(radiusSM).
		Padding(0, 9).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Disabled(disabled)

	bg := ui.Transparent
	fg := pal.inkSubtle
	switch {
	case disabled:
		fg = pal.inkTertiary
		btn.Cursor(ui.CursorDefault)
	case btn.Pressed():
		bg = pal.surface3
		fg = pal.ink
	case btn.Hovered():
		bg = pal.surface2
		fg = pal.ink
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
		ui.Text(c, label).FontSize(textButton).FontWeight(weightMedium).
			SingleLine().TextColor(fg)
	})
	return btn.Clicked() && !loading
}

// toolIconButton 是工具栏上的纯图标动作按钮（如刷新、设置、打开仓库）。
// 同样是 §4.1 的幽灵键规格：28x28 方形、sm(6) 圆角、悬停 Surface 2，
// 操作提示交给 Tooltip。
func toolIconButton(c *ui.Context, pal palette, label, tip string, off, loading bool, icon func(p *ui.Painter, r ui.Rect, col ui.Color)) bool {
	disabled := off || loading
	btn := ui.ButtonBase(c).Size(28, 28).Shrink(0).Radius(radiusSM).Center().
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Disabled(disabled)

	bg := ui.Transparent
	fg := pal.inkSubtle
	switch {
	case disabled:
		fg = pal.inkTertiary
		btn.Cursor(ui.CursorDefault)
	case btn.Pressed():
		bg = pal.surface3
		fg = pal.ink
	case btn.Hovered():
		bg = pal.surface2
		fg = pal.ink
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

// drawChevronUpIcon / drawChevronDownIcon 是查找栏里「上一条 / 下一条」的箭头
// （Lucide chevron-up / chevron-down）。
func drawChevronUpIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconChevronUp, r, col)
}

func drawChevronDownIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconChevronDown, r, col)
}

// drawSlidersIcon 画「设置」（Lucide sliders-horizontal）。
func drawSlidersIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconSliders, r, col)
}

// drawSettingsIcon 画「设置齿轮」（Lucide settings）。
func drawSettingsIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconSettings, r, col)
}

// drawFolderIcon 画「打开仓库」（Lucide folder）。
func drawFolderIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconFolder, r, col)
}

// drawTrashIcon 画「清除 / 垃圾桶」（Lucide trash-2）。
func drawTrashIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconTrash, r, col)
}

// drawZapIcon 画「闪电 / 重启」（Lucide zap）。
func drawZapIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconZap, r, col)
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
func xButton(c *ui.Context, pal palette, hint string) bool {
	btn := ui.ButtonBase(c).Size(xBtnSize, xBtnSize).Shrink(0).
		Radius(radiusSM).Cursor(ui.CursorPointer).
		// 按钮上没有文字，Label 是无障碍名字，也是测试找它的凭据；
		// Tooltip 给鼠标用户看一眼。
		Label(hint).Tooltip(hint)

	bg, color := ui.Transparent, pal.inkSubtle
	if btn.Hovered() {
		bg, color = pal.surface2, pal.ink
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

// actionBtnHeight 是对话框底部那两颗动作按钮的高度。
//
// 和提交按钮同高：它们是同一个角色的东西 —— 一张卡片或一个对话框上那个
// 「点了就走」的键，只是分别站在底部和右上角。
const actionBtnHeight = 28.0

// primaryAction 是对话框上的主操作键（§4.1 Primary Button）。
//
// 实心强调色是它应得的：§5 给的预算里「核心动作焦点」正是留给它和侧栏导航
// 指示条的。但**一个对话框至多一颗** —— 一屏并排两枚蓝按钮，用户就得先读字
// 才知道哪颗是真正要走的那一步，强调色也就白给了。
//
// tooltip 和 disabled 都收进来而不是留给调用方链：框架的 Tooltip 构建时问一次
// Hovered()，而禁用元素不报 hover —— 调用方一旦把 Disabled 写在 Tooltip 前面，
// 那句「为什么点不了」就永远不会浮出来（resolve.go / pendingbar.go 都踩过）。
// 摆在这里，顺序只对一次。
func primaryAction(c *ui.Context, pal palette, label, tip string, disabled bool) *ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Shrink(0).Height(actionBtnHeight).Radius(radiusMD).
		Padding(0, 12).Gap(5).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Disabled(disabled).Cursor(ui.CursorPointer)

	var bg, border, fg ui.Color
	switch {
	case disabled:
		bg, border, fg = pal.surface3, pal.hairline, pal.inkTertiary
	case btn.Pressed():
		bg, border, fg = t.AccentPressed, t.AccentPressed, t.AccentText
	case btn.Hovered():
		bg, border, fg = t.AccentHover, t.AccentHover, t.AccentText
	default:
		bg, border, fg = t.Accent, t.Accent, t.AccentText
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		ui.Text(c, label).FontSize(textButton).FontWeight(weightMedium).TextColor(fg)
	})
	return btn
}

// secondaryAction 是对话框上的次要键（§4.1 Secondary Button）：Surface 1 底 +
// 1px 发丝边 + Ink Muted 文字。「取消」「关闭」「重试」都走这里。
//
// 它和 primaryAction 的差别不只在颜色：两者同高同宽，谁主谁次由**有没有实心底**
// 说清楚 —— 只靠色相区分的话，红绿色盲的用户看不出哪颗是「就这么办」。
func secondaryAction(c *ui.Context, pal palette, label, tip string, disabled bool) *ui.Element {
	btn := ui.ButtonBase(c).Shrink(0).Height(actionBtnHeight).Radius(radiusMD).
		Padding(0, 12).Gap(5).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Disabled(disabled).Cursor(ui.CursorPointer)

	var bg, border, fg ui.Color
	switch {
	case disabled:
		bg, border, fg = ui.Transparent, pal.hairline, pal.inkTertiary
	case btn.Pressed():
		bg, border, fg = pal.surface3, pal.hairlineStrong, pal.ink
	case btn.Hovered():
		bg, border, fg = pal.surface2, pal.hairlineStrong, pal.ink
	default:
		bg, border, fg = pal.surface1, pal.hairline, pal.inkMuted
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		ui.Text(c, label).FontSize(textButton).FontWeight(weightMedium).TextColor(fg)
	})
	return btn
}

// specModal 是对话框的地基：一层压暗的背景，加一块按 §2.1 / §3 摆好的面板。
//
// 替掉框架的 ui.Modal 只为一件事：它的面板圆角写死成 `t.Space(2.5)`（＝10），
// 而 10 不在规范的六档里，模态该是 xl(16)。其余行为 —— Esc、点背景关闭、
// 焦点锁在对话框内 —— 全在 DialogBase 里，ui.Modal 也只是调它，所以换掉这层
// 皮不影响任何交互。
//
// 内边距和间距跟 ui.Modal 保持一致（20 / 12），各对话框的身子是按这个尺寸摆的。
func specModal(c *ui.Context, pal palette, open *bool, fn func()) {
	t := c.Theme()
	w, h := c.Size()
	ui.DialogBase(c, open, func(back, panel *ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Padding(t.Space(5)).Gap(t.Space(3)).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			MaxWidth(w-t.Space(10)).MaxHeight(h-t.Space(10)).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24))
		fn()
	})
}
