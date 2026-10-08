package main

import (
	"github.com/egoist/mygo/ui"
)

// 两棵树 —— 侧栏的分支/标签/储藏树、主区下半的文件树 —— 共用这套行画法。
//
// 这两处原来都用 ui.Outline。它把「缩进 + 折叠箭头」画在行内容**外面**，
// 行元素碰不到那一块，于是有三件事一直别扭：
//
//   - 选中底色只能从缩进之后开始画，每深一层往右挪一格，是块浮着的小胶囊，
//     而不是「这一行被选中了」；
//   - 名字前面白白多出一个箭头框，侧栏里名字从 x=64 才开始，可分组名在
//     x=28 —— 差的这 36px 里什么都没有，看着不像一棵树；
//   - 行高由内容撑，量出来是 24、25、24 这样参差的，竖着扫一列眼睛看得出来。
//
// 所以改成「filetree.Flatten 摊平 + ui.List 画」：缩进、箭头、行高全在自己
// 手里，底色铺满整行，各层的名字落在同一条竖线上。
const (
	treeRowH   = 24 // 定死：ui.List 是虚拟化的，行高不齐首帧会把行摞在一起
	treeIndent = 14 // 每深一层缩进多少
	treeArrowW = 12 // 折叠箭头的槽宽；叶子也留出来，各层名字才对得齐
	treePadX   = 8  // 行内左右留白，也是底色圆角块的缩进

	// 选中/悬停的底色块上下各往里收多少，以及它的圆角。
	//
	// 收这 2px 是为了让每行是一颗独立的胶囊，而不是首尾相连的一整块板子 ——
	// 一列扫下来，行与行之间有没有缝，观感差别很大。
	//
	// 高度必须写死，不能靠 Margin：底色块原来用 FillHeight（height:100%），
	// 百分比高度会把上下 margin 顶掉，写 Margin(1) 量出来还是整整 24。
	treeSelInset  = 2
	treeSelRadius = 5
)

// treeRowOpts 是一行的画法。
//
// 用结构体而不是一串参数：这里好几个 bool 挨在一起，调用处
// treeRow(c, t, 1, false, true, false, ...) 谁也看不出哪个是哪个。
type treeRowOpts struct {
	Depth    int  // 缩进层级
	Selected bool // 选中的行画强调色底色
	HasKids  bool // 有子节点 —— 折叠箭头画得出来、点得动
	Open     bool // 展开着（只在 HasKids 时有意义）
	// Dot 不为 nil 时，在折叠箭头那个槽里点一个实心圆点。
	//
	// 侧栏用它标「当前检出的分支」。放在箭头槽里而不是另起一槽，是为了不占
	// 名字的位置：名字前面单独留一个圆点槽的话，整列分支名会白往右挪 12px，
	// 和上面那个「名字被顶到右边」的毛病是同一类。
	Dot *ui.Color
}

// treeRow 起一行树节点，返回整行和折叠箭头两个元素。
//
// 交互分两层：整行负责命中（悬停、点击都算在整行上，缩进那块空白也点得动），
// 里面那个往里缩 8px 的圆角块负责好看 —— 底色直接铺到窗口边上会显得很生硬。
//
// 箭头画在行里面，点它会同时命中整行，所以调用方要先问 Arrow 再问 Row。
func treeRow(c *ui.Context, t *ui.Theme, o treeRowOpts, content func()) (row, arrow *ui.Element) {
	row = ui.Row(c).Height(treeRowH).Shrink(0).FillWidth().Cursor(ui.CursorPointer)
	// 悬停要在建行的当口就问：这时读的是上一帧的命中结果，等子元素都建完
	// 再问也行，但那时已经过了画底色的地方。
	hovered := row.Hovered()

	row.Children(func() {
		band := ui.Row(c).Grow(1).Height(treeRowH-2*treeSelInset).
			Margin(treeSelInset, treePadX).
			Padding(0, 6).Radius(treeSelRadius).Gap(6).AlignItems(ui.Center)
		switch {
		case o.Selected:
			band.Background(t.Accent.Alpha(selAlpha))
		case hovered:
			band.Background(t.Surface)
		}
		fadeColors(band)

		band.Children(func() {
			if o.Depth > 0 {
				ui.Box(c).Width(float32(o.Depth) * treeIndent).Shrink(0)
			}

			arrow = ui.Box(c).Size(treeArrowW, treeArrowW).Shrink(0)
			switch {
			case o.HasKids:
				arrow.Cursor(ui.CursorPointer)
				arrow.Draw(func(p *ui.Painter, r ui.Rect) {
					drawChevron(p, r, t.TextMuted, o.Open)
				})
			case o.Dot != nil:
				dot := *o.Dot
				arrow.Draw(func(p *ui.Painter, r ui.Rect) {
					var path ui.Path
					path.Circle(r.X+r.W/2, r.Y+r.H/2, 3)
					p.FillPath(&path, dot)
				})
			}

			content()
		})
	})
	return row, arrow
}

// drawChevron 画折叠箭头（Lucide chevron-down / chevron-right）。
func drawChevron(p *ui.Painter, r ui.Rect, c ui.Color, open bool) {
	if open {
		p.Icon(iconChevronDown, r, c)
	} else {
		p.Icon(iconChevronRight, r, c)
	}
}
