package main

import (
	"math"
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
// 该是同一个亮度，emoji 的写死色做不到）。这是这个应用的动作图标，值得自己画。
//
// 形状：四个尖在正上/正右/正下/正左，每个象限用一条二次曲线连，控制点落在
// 中心 —— 边因此朝里凹，是「闪」的形状。控制点写在中心是这类星形的老做法，
// 比调贝塞尔手柄省事，改边长也不用重算比例。
func drawSparkle(p *ui.Painter, r ui.Rect, c ui.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rx, ry := r.W/2, r.H/2
	var path ui.Path
	path.MoveTo(cx, cy-ry)
	path.QuadTo(cx, cy, cx+rx, cy)
	path.QuadTo(cx, cy, cx, cy+ry)
	path.QuadTo(cx, cy, cx-rx, cy)
	path.QuadTo(cx, cy, cx, cy-ry)
	path.Close()
	p.FillPath(&path, c)
}

// sparkleIcon 是一个只有火花的方盒，用来摆在按钮或标题里。
func sparkleIcon(c *ui.Context, size float32, color ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		drawSparkle(p, r, color)
	})
}

// sparkleCluster 是「一大两小」的星群，用在卡片标题上。
//
// 和按钮里那颗单星分开：12px 上三颗星会糊成一团，而卡片标题那块有 16px 的
// 空间，一大两小才认得出是 ✨ 而不是一个菱形。小的两颗压暗一档，星群才有
// 主次，不然三个等重的形状挤在一起。
func sparkleCluster(c *ui.Context, size float32, color ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		big, small := size*0.72, size*0.34
		at := func(x, y, s float32) ui.Rect {
			return ui.Rect{X: r.X + x, Y: r.Y + y, W: s, H: s}
		}
		drawSparkle(p, at(0, 0, big), color)
		drawSparkle(p, at(size-small, size*0.04, small), color.Alpha(0.85))
		drawSparkle(p, at(size*0.06, size-small, small), color.Alpha(0.6))
	})
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
// 大小、粗细、基线各机器都不一样，而这个勾要和旁边的绿色数字对齐。
func drawCheckMark(p *ui.Painter, r ui.Rect, c ui.Color) {
	var path ui.Path
	path.MoveTo(r.X+r.W*0.14, r.Y+r.H*0.54)
	path.LineTo(r.X+r.W*0.40, r.Y+r.H*0.80)
	path.LineTo(r.X+r.W*0.88, r.Y+r.H*0.20)
	p.StrokePath(&path, 1.6, c)
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

// drawMagnifier 画放大镜：一个镜面加一根斜柄。
//
// 柄从镜面的边缘接出去，不是从圆心 —— 从圆心画的话，镜面那条圆弧会横在
// 柄的中间，看着像一根棒棒糖。
func drawMagnifier(p *ui.Painter, r ui.Rect, col ui.Color) {
	rad := min(r.W, r.H) * 0.30
	cx, cy := r.X+r.W*0.42, r.Y+r.H*0.42

	var lens ui.Path
	lens.Circle(cx, cy, rad)
	p.StrokePath(&lens, 1.4, col)

	k := rad * 0.707 // 45° 方向上的余弦
	var handle ui.Path
	handle.MoveTo(cx+k, cy+k).LineTo(r.X+r.W*0.92, r.Y+r.H*0.92)
	p.StrokePath(&handle, 1.6, col)
}

// toolIconSize 是工具栏图标方盒的边长。比 13px 的文字略大一圈 —— 图标和
// 文字视觉等重时，笔画密的图标看着会小一号。
const toolIconSize = 15

// toolButton 是工具栏上的一颗动作按钮：矢量图标 + 文字标签。
//
// 用 ui.Button 而不是 ui.ButtonBase：它落在 ui.Toolbar 里时自带「平时没有脸、
// 指上去才浮出底色」的样式（框架按 c.buttons == toolbarButtons 分派），和
// 系统工具栏一个脾气。描边留给分段控件那种「带着状态」的控件，工具栏才不会
// 是一排等重的灰盒子 —— 改动/历史 是「现在在看哪个」，刷新是「做一件事」，
// 两者的分量本来就不同。
//
// Label 给的是无障碍名字，也是测试找它的凭据；可见的文字是同一个词，
// 两者都写上，谁也替代不了谁。
// off 为真时画成灰的（框架压到半透明）、点也不响应：没有对象可作用时，
// 一颗点下去什么都没发生的按钮比一颗灰掉的按钮更让人困惑。
func toolButton(c *ui.Context, label, tip string, off bool, icon func(p *ui.Painter, r ui.Rect, col ui.Color)) bool {
	btn := ui.Button(c, "").Label(label).Tooltip(tip).Padding(0, 8).Disabled(off)
	fadeColors(btn)
	btn.Children(func() {
		col := c.Theme().Text
		ui.Box(c).Size(toolIconSize, toolIconSize).Shrink(0).
			Draw(func(p *ui.Painter, r ui.Rect) { icon(p, r, col) })
		ui.Text(c, label).FontSize(12).SingleLine()
	})
	return btn.Clicked()
}

// iconPt 把 0..1 的归一化坐标落进矩形里。
//
// 图标按比例画，不写死像素：同一段代码既能画 15px 的工具栏图标，也不会
// 在别的尺寸上变形。
func iconPt(r ui.Rect, x, y float32) (float32, float32) {
	return r.X + r.W*x, r.Y + r.H*y
}

// arcPoint 是圆上角度 deg（度）处的点。
//
// 0 度在正右方，角度**增大**的方向在屏幕上就是顺时针 —— 屏幕坐标 y 朝下，
// 和数学课本里的逆时针正好相反。整个文件里的角度都按这一套来。
func arcPoint(cx, cy, rad, deg float32) (float32, float32) {
	a := float64(deg) * math.Pi / 180
	return cx + rad*float32(math.Cos(a)), cy + rad*float32(math.Sin(a))
}

// arcTo 把一段顺时针圆弧加进 path。
//
// ui.Path 只有直线和贝塞尔，圆得自己拼。每段不超过 90°，控制点长度取
// 4/3·tan(Δ/4)：这是拿三次贝塞尔逼近圆弧的经典取法，90° 一段的误差在
// 千分之一以内，这个尺寸上完全看不出来。
func arcTo(path *ui.Path, cx, cy, rad, from, to float32) {
	sweep := to - from
	for sweep <= 0 {
		sweep += 360
	}
	segs := int(math.Ceil(float64(sweep) / 90))
	step := sweep / float32(segs)
	k := 4.0 / 3.0 * math.Tan(float64(step)*math.Pi/720)

	x0, y0 := arcPoint(cx, cy, rad, from)
	path.MoveTo(x0, y0)
	for i := 0; i < segs; i++ {
		a0 := from + step*float32(i)
		a1 := a0 + step
		// 切线方向：对 (cos a, sin a) 求导得 (-sin a, cos a)
		t0x, t0y := -math.Sin(float64(a0)*math.Pi/180), math.Cos(float64(a0)*math.Pi/180)
		t1x, t1y := -math.Sin(float64(a1)*math.Pi/180), math.Cos(float64(a1)*math.Pi/180)
		c1x, c1y := x0+rad*float32(k)*float32(t0x), y0+rad*float32(k)*float32(t0y)
		x1, y1 := arcPoint(cx, cy, rad, a1)
		c2x, c2y := x1-rad*float32(k)*float32(t1x), y1-rad*float32(k)*float32(t1y)
		path.CubeTo(c1x, c1y, c2x, c2y, x1, y1)
		x0, y0 = x1, y1
	}
}

// drawRefreshIcon 画「刷新」：一段留缺口的圆弧加一个箭头。
//
// 缺口开在右上，箭头停在缺口边上、指着顺时针方向 —— 这是刷新符号通行的
// 样子，少一段弧加一个头，一眼就知道是「转一圈重来」，而不是一个问号。
func drawRefreshIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rad := min(r.W, r.H) * 0.36

	// 缺口 80° 开在正上方（偏右一点），箭头长在**收笔的那一头**（左上），
	// 指着顺时针方向。
	//
	// 画在收笔那头而不是起笔那头：起笔那头的箭头会和弧线叠在一起，15px 上
	// 两个形状一糊就成了个疙瘩。收笔这头的箭头正好落在缺口里，底边和弧的
	// 端点接上，是一支看得清的箭头。
	//
	// 缺口的角度是量出来的：
	//
	//   - 60°（弧走 300°）时，头再小也和弧粘成一坨 —— 缺口里挤不下一个
	//     三角形的轮廓，整个图标读作「一个带尖的实心圆」；
	//   - 95° 往上，头是清楚了，可圈本身断成了一牙「C」；
	//   - 80° 是两头都成立的那一档：圈还闭得起来，头也还立得住。
	var path ui.Path
	arcTo(&path, cx, cy, rad, 325, 245)
	p.StrokePath(&path, 1.3, col)

	px, py := arcPoint(cx, cy, rad, 245)
	a := float32(245) * math.Pi / 180
	tx, ty := -float32(math.Sin(float64(a))), float32(math.Cos(float64(a))) // 切线（画弧的方向）
	nx, ny := -ty, tx                                                       // 法线

	// 箭头是个实心三角：底边横在弧的端点上（和切线垂直），尖顺着切线往
	// 前探 0.85 个半径。
	//
	// 试过把倒钩画成两笔描边（羽毛那一路）：在这个尺寸上笔画太细，反倒
	// 认不出是个头。实心三角靠的是「比弧粗」——底宽是弧线的三倍多，眼睛
	// 一眼就分得出哪段是圈、哪个是箭头。
	var head ui.Path
	l, wd := rad*0.85, rad*0.46
	head.MoveTo(px+tx*l, py+ty*l)
	head.LineTo(px+nx*wd, py+ny*wd)
	head.LineTo(px-nx*wd, py-ny*wd)
	head.Close()
	p.FillPath(&head, col)
}

// drawSlidersIcon 画「设置」：两条横轨，各挂一个滑钮，位置错开。
//
// 两条而不是三条：三条横线在 15px 里只剩 4px 的间距，笔画一挤就糊成一团。
// 两个错开的滑钮已经足够说出「调参数」这层意思。
func drawSlidersIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	ys := [2]float32{0.3, 0.7}
	knobs := [2]float32{0.66, 0.34}

	var rails ui.Path
	for _, y := range ys {
		x0, py := iconPt(r, 0.07, y)
		x1, _ := iconPt(r, 0.93, y)
		rails.MoveTo(x0, py).LineTo(x1, py)
	}
	p.StrokePath(&rails, 1.3, col)

	var dots ui.Path
	for i, kx := range knobs {
		x, y := iconPt(r, kx, ys[i])
		dots.Circle(x, y, 2.1)
	}
	p.FillPath(&dots, col)
}

// drawFolderIcon 画「打开仓库」：一个文件夹。
//
// 画的是轮廓不是实心块：15px 的实心文件夹是一小坨色块，轮廓才看得出形状。
// 右上角往里收一点的那条斜边是折页，少了它就是个方框。
func drawFolderIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	pt := func(x, y float32) (float32, float32) { return iconPt(r, x, y) }

	var path ui.Path
	x, y := pt(0.08, 0.80)
	path.MoveTo(x, y)
	x, y = pt(0.08, 0.26)
	path.LineTo(x, y)
	x, y = pt(0.40, 0.26)
	path.LineTo(x, y)
	x, y = pt(0.49, 0.38)
	path.LineTo(x, y)
	x, y = pt(0.88, 0.38)
	path.LineTo(x, y)
	x, y = pt(0.92, 0.44)
	path.LineTo(x, y)
	x, y = pt(0.92, 0.74)
	path.LineTo(x, y)
	x, y = pt(0.88, 0.80)
	path.LineTo(x, y)
	path.Close()
	p.StrokePath(&path, 1.4, col)
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
