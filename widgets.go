package main

import "github.com/egoist/mygo/ui"

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
