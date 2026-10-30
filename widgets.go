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
func fadeColors(e ui.Element) ui.Element {
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

// specChip 起一颗「规范芯片」：chip 洗色底 + 1px 发丝边 + Ink Muted 文字。
//
// 语义 —— 本地分支 / 远程 / 标签 / 严重度 / 状态 —— 全靠 dot 那颗 6px 圆点带
// 出来，而不是把整块芯片涂成琥珀、亮蓝、翠绿。这不是洁癖：提交行一行能挂四枚
// 引用芯片，逐个上色就是 §5 点名的「彩色药丸大爆炸」，一行扫过去全是色块，
// 「这是哪条分支」反而更难认。dot 传 ui.Transparent 就得到一颗纯文字芯片。
//
// 高度和字号由调用方给：芯片出现在 24px 的树行、30px 的提交行和对话框里，
// 尺寸不一样，但底、边、圆点这三样必须一样 —— 那是「它是什么」的部分。
//
// 返回元素：调用方可能要再往上挂东西（提交行的分支芯片挂「双击检出」）。
// 其余调用处照旧当语句用，返回值直接丢掉就行。
func specChip(c *ui.Context, pal palette, label string, dot ui.Color, height, fontSize float32) ui.Element {
	chip := ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).Gap(4).
		Padding(0, 6).Radius(radiusXS).
		// 半透明底：芯片多落在工作卡里（文件状态、HEAD、历史徽章），
		// surface2 不透明，会把卡片的亚克力挡掉一块。
		Background(pal.chip).Border(1, pal.hairline)
	chip.Children(func() {
		if dot.A > 0 {
			ui.Box(c).Size(chipDot, chipDot).Shrink(0).
				Radius(chipDot / 2).Background(dot)
		}
		ui.Text(c, label).FontSize(fontSize).FontWeight(weightMedium).
			SingleLine().TextColor(pal.inkMuted)
	})
	return chip
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

// shieldIcon 是一个盾牌图标方盒，用来摆在安全预检按钮或对话框标题中。
func shieldIcon(c *ui.Context, size float32, color ui.Color) {
	drawIcon(c, iconShield, size, color)
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
func searchField(c *ui.Context, pal palette, value *string, placeholder string) ui.Element {
	t := c.Theme()
	var in ui.Element

	// 搜索栏在 §3 里明确归到 md(8)，和文本输入框同一档 —— 它就是个输入框。
	//
	// 底色走半透明的 chip：surface1 和卡片同色却是不透明的，铺在亚克力卡
	// 上等于抠出一块挡板；chip 只是在卡片上叠一层薄纱（深色略抬、浅色略沉），
	// 框的形状仍由 hairlineStrong 那条边定。
	box := ui.Row(c).Height(searchH).Shrink(0).AlignItems(ui.Center).
		Gap(6).Padding(0, 8).Radius(radiusMD).Background(pal.chip).
		Border(1, pal.hairline).
		Label(placeholder).Children(func() {
		searchIcon(c, 12, pal.inkSubtle)
		in = ui.TextInputBase(c, value).Grow(1).MinWidth(0).
			Placeholder(placeholder).Label(placeholder)
		if value != nil && *value != "" {
			clearBtn := ui.ButtonBase(c).Size(16, 16).Radius(radiusPill).Center().
				Cursor(ui.CursorPointer).Label("清空").Tooltip("清空筛选")
			if clearBtn.Hovered() {
				clearBtn.Background(pal.rowSelected)
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
	case in.Valid() && in.Focused():
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
func toolbarSegmentedText(c *ui.Context, pal palette, selected *int, off bool, labels ...string) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(labels))
	g := parts.Track
	g.Height(28).AlignItems(ui.Center).Padding(2).Gap(2).Radius(radiusMD).
		Background(pal.chip).Border(1, pal.hairline).Disabled(off)

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
				bg = pal.rowSelected
				fg = pal.ink
				s.Border(1, pal.hairlineStrong)
			case s.Hovered():
				bg = pal.rowHover
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
	// off 让这一格单独变灰、点不动。分段里几格各有各的前提时用它 ——
	// 「改动 / 历史」要有仓库而「终端 / 编辑器」不必，整条控件关掉就走
	// toolbarSegmentedIcons 的 off 参数。
	off bool
}

// toolbarSegmentedIcons 是工具栏上的图标分段控件（如「Unified / Split」切换）。
//
// 轨道自己不画底、不描边：工具栏上已经有一条卡片边线，再给这一组套一圈
// 圆角边框，屏上就多出一个「框里的框」，和旁边的幽灵图标键也不在同一档。
// 只留每一格自己的悬停 / 选中底 —— 选中态是 rowSelected 洗色的小胶囊，未选中
// 的图标直接坐在工具栏上。单元 sm(6)、图标用 Lucide 标准矢量。
//
// 两档都是洗色、不是 Surface：这一条长在亚克力工作卡上，不透明的胶囊会在
// 磨砂上抠出一块实心（DESIGN.md §2.1「洗色阶梯」）。
func toolbarSegmentedIcons(c *ui.Context, pal palette, selected *int, off bool, items ...segmentedIconItem) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(items))
	g := parts.Track
	g.Height(28).AlignItems(ui.Center).Padding(2).Gap(2).Disabled(off)

	g.Children(func() {
		for i, it := range items {
			s := parts.Segment(i).Size(26, 24).Radius(radiusSM).
				Cursor(ui.CursorPointer).Label(it.label).Tooltip(it.tip)
			if it.off {
				s.Disabled(true)
			}
			sel := i == *selected && !it.off
			bg := ui.Transparent
			fg := pal.inkMuted
			switch {
			case off || it.off:
				fg = pal.inkTertiary
			case sel:
				bg = pal.rowSelected
				fg = pal.ink
				s.Border(1, pal.hairlineStrong)
			case s.Hovered():
				bg = pal.rowHover
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
	// MyGo 0.4 controls apply pending input through Changed; without this call
	// the local mode value is still the previous frame when the caller reads it.
	g.Changed()
	return g
}

// toolButton 是工具栏上的一颗动作按钮：矢量图标 + 文字标签。
//
// 照 §4.1 的 Ghost / Tool Button 走：透明底、悬停 rowHover、按下 rowSelected、
// 圆角 sm(6)、图标由 Ink Subtle 提到 Ink。它和主按钮不是一档东西 —— 圆角也
// 小一号。底色只走洗色阶梯：它多半长在亚克力工作卡上，也会落进弹窗，洗色
// 在两种底上都是「提一档」，不透明的 Surface 只在弹窗里成立。
//
// 支持 loading 态：正在加载时内部 15x15 图标平滑切换为 Spinner 原地旋转，
// 保持按钮总尺寸与布局毫厘不差，彻底杜绝在按钮外侧横向弹出额外 icon 造成的剧烈布局偏移。
func toolButton(c *ui.Context, pal palette, label, tip string, off, loading bool, icon func(p *ui.Painter, r ui.Rect, col ui.Color)) bool {
	disabled := off || loading
	btn := ui.ButtonBase(c).Height(28).Shrink(0).Radius(radiusSM).
		Padding(0, 9).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Disabled(disabled)
	if disabled && tip != "" {
		btn.Tooltip(tip)
	} else {
		btn.Tooltip(tip)
	}

	bg := ui.Transparent
	fg := pal.inkSubtle
	switch {
	case disabled:
		fg = pal.inkTertiary
		btn.Cursor(ui.CursorDefault)
	case btn.Pressed():
		bg = pal.rowSelected
		fg = pal.ink
	case btn.Hovered():
		bg = pal.rowHover
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
// 同样是 §4.1 的幽灵键规格：28x28 方形、sm(6) 圆角、悬停 rowHover、按下
// rowSelected，操作提示交给 Tooltip。
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
		bg = pal.rowSelected
		fg = pal.ink
	case btn.Hovered():
		bg = pal.rowHover
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

// drawRefreshIcon 画「刷新」（Lucide rotate-cw）。
func drawRefreshIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconRefresh, r, col)
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

// drawPlusIcon 画「新建 / 加号」（Lucide plus）。
func drawPlusIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconPlus, r, col)
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
		bg, color = pal.rowHover, pal.ink
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

// washCheckbox 是落在亚克力工作卡上的勾选框（提交栏的 Amend、diff 栏头的
// 「已 review」）。
//
// 不用 ui.Checkbox：框架那颗方框未勾选时铺 t.Background（深色 #232328、浅色
// 纯白）—— 和卡片同色，却是不透明的。卡片一透，这颗 16px 的方块就成了磨砂
// 上抠出来的一块实心。这里只换方框的皮：未勾选是 chip 洗色 + hairlineStrong
// 边，悬停升 rowSelected；勾上是实心强调色，那是它应得的（§5）。点击、Space、
// 无障碍角色都还是 CheckboxBase 的，尺寸和间距照框架那颗取，换上去不挪位置。
func washCheckbox(c *ui.Context, pal palette, checked *bool, label string) ui.Element {
	t := c.Theme()
	row := ui.CheckboxBase(c, checked).Gap(t.Space(2)).AlignItems(ui.Center).
		FocusRing(false).Label(label)
	on, hovered := *checked, row.Hovered()
	row.Children(func() {
		box := ui.Box(c).Size(t.Space(4), t.Space(4)).Radius(radiusXS).Shrink(0)
		switch {
		case on:
			box.Background(t.Accent).Border(1, t.Accent)
		case hovered:
			box.Background(pal.rowSelected).Border(1, pal.hairlineTertiary)
		default:
			box.Background(pal.chip).Border(1, pal.hairlineStrong)
		}
		fadeColors(box)
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if on {
				// 勾画在内缩 3px 的方块里，和 stageBox 那颗同一副比例。
				drawCheckMark(p, ui.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}, t.AccentText)
			}
			if row.FocusVisible() {
				p.FocusRing(r, [4]float32{radiusXS, radiusXS, radiusXS, radiusXS})
			}
		})
		if label != "" {
			ui.Text(c, label)
		}
	})
	return row
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
func primaryAction(c *ui.Context, pal palette, label, tip string, disabled bool) ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Shrink(0).Height(actionBtnHeight).Radius(radiusMD).
		Padding(0, 12).Gap(5).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Disabled(disabled).Cursor(ui.CursorPointer)

	var bg, border, fg ui.Color
	switch {
	case disabled:
		bg, border, fg = pal.chip, pal.hairline, pal.inkTertiary
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

// secondaryAction 是对话框上的次要键（§4.1 Secondary Button）：chip 洗色底 +
// 1px 发丝边 + Ink Muted 文字，悬停 / 按下升到 rowSelected、边升 hairlineStrong。
// 「取消」「关闭」「重试」都走这里 —— 它既落在弹窗里，也落在亚克力工作卡上
// （冲突条的「全部采用」、空状态页），所以底色只能是洗色。
//
// 它和 primaryAction 的差别不只在颜色：两者同高同宽，谁主谁次由**有没有实心底**
// 说清楚 —— 只靠色相区分的话，红绿色盲的用户看不出哪颗是「就这么办」。
func secondaryAction(c *ui.Context, pal palette, label, tip string, disabled bool) ui.Element {
	btn := ui.ButtonBase(c).Shrink(0).Height(actionBtnHeight).Radius(radiusMD).
		Padding(0, 12).Gap(5).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Disabled(disabled).Cursor(ui.CursorPointer)

	var bg, border, fg ui.Color
	switch {
	case disabled:
		bg, border, fg = ui.Transparent, pal.hairline, pal.inkTertiary
	case btn.Pressed(), btn.Hovered():
		bg, border, fg = pal.rowSelected, pal.hairlineStrong, pal.ink
	default:
		bg, border, fg = pal.chip, pal.hairline, pal.inkMuted
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
	specModalOpts(c, pal, open, false, fn)
}

// specModalSticky 是 specModal 的变体：点压暗的背景不关对话框（Esc 和
// 「取消」照旧关）。
//
// 给表单类对话框用 —— 新建分支打了一半名字，手一抖点在下边的背景上，内容和
// 「取消」问都不问就一起没了。目前走它的：新建分支、新建标签、储藏、重命名
// 分支、建 PR、PR 评审、对比分支、模型设置、代码编辑器。剩下三种照旧点背景
// 即关：确认框（点背景本来就是「取消」）、文件历史与风险预检（纯查看，没有
// 会丢的东西）、重置（挑的是模式，不是打字）。
//
// back.Clicked() 在 DialogBase 判过之后还是 true（点击数记在元素自己身上，
// 一帧之后才清零），所以在这儿能把 *open 拨回去；点在面板里的点击记在面板
// 上，到不了背景，拨不错。自己搭 DialogBase 的对话框（设置、代码编辑器、
// 对比分支）在里面照抄这两行。
func specModalSticky(c *ui.Context, pal palette, open *bool, fn func()) {
	specModalOpts(c, pal, open, true, fn)
}

func specModalOpts(c *ui.Context, pal palette, open *bool, keepOnBackdrop bool, fn func()) {
	t := c.Theme()
	w, h := c.Size()
	ui.DialogBase(c, open, func(back, panel ui.Element) {
		if keepOnBackdrop && back.Clicked() {
			*open = true
		}
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Padding(t.Space(5)).Gap(t.Space(3)).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			MaxWidth(w-t.Space(10)).MaxHeight(h-t.Space(10)).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24))
		fn()
	})
}
