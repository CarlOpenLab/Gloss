// 「AI 生成」右边那颗 ⌘I 快捷键提示看不清（用户原话：变更预检右侧的快捷键
// 还是看不清）。
//
// 它是全界面最小的一段文字：11px，坐在一颗 4px 圆角的洗色小片子上。而 DESIGN.md
// §2.3 正好把**最暗的那一档墨**（Ink Tertiary）派给了「快捷键提示文本」，
// §2.1 又把**最淡的那一档洗色**（kbd，深 6% 白 / 浅 4% 黑）派给了「快捷键提示」
// —— 两个最虚的 token 叠在一起，就是用户读到的「看不清」。
//
// 无头渲染下量出来的实测值（笔画核心对自己那片底子的 WCAG 对比度）：
//
//	深色 1.72:1    浅色 2.08:1
//
// 同一颗按钮里其余三段文字（两颗图标 + 「AI 生成」）是 6.31 / 4.58 ——
// 芯片这一段是唯一的异类。深色下更糟：它的笔画核心**一个像素都没有**，
// 整段字全是抗锯齿摊出来的灰，从来没能落成一个实心像素。
//
// 为什么量笔画核心而不是整段字的平均色：WCAG 的对比度是定义在两种**实色**
// 之间的，抗锯齿那一圈灰既不是字也不是底。按平均色算会把这圈灰算进字里，
// 每一段都被拉低一截（同一屏上按钮自己的标签按均值算只有 3.16，按核心算是
// 4.58）—— 那是量法的偏差，不是设计的问题。但核心同时得要求「够多的像素」：
// 只按核心色算的话，一段全糊的字也能靠一两个侥幸像素过关。

package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// rgbpx 是一个 8bit 的 RGB 像素。测试里量的是渲染出来的东西，给它一个名字，
// 免得每个函数签名都拖一长串匿名结构体。
type rgbpx struct{ r, g, b uint32 }

const (
	// inkGate 是「这个像素算不算墨」的门槛：和底子差过这么多个色阶才算字。
	// 24 是抗锯齿和这一档洗色够不到、笔画核心一定过得了的一档 —— 按钮底和
	// 芯片底之间只差 4% 的一层洗色，不设这么高就会把「哪一片是芯片」也读成字。
	inkGate = 24
	// coreGate 是「这个像素算笔画核心」的门槛。抗锯齿的边角过不了，只有
	// 真正落到实处的像素过得了。
	coreGate = 60
)

// kbdChipContrast 把「AI 生成」那颗按钮真的画出来，量 ⌘I 那段字的笔画核心
// 对它自己底子的 WCAG 对比度，以及够得上核心的像素数。
//
// 为什么从右往左找最后一段墨：那颗芯片是按钮里最靠右的东西（工具栏排布是
// 变更预检 → AI 生成 → 提交，芯片在 AI 生成内部的最右端）。不按坐标写死
// 位置 —— 字号、文案、语言一改它就漂了，按「最右一段够宽的墨」找则换语言
// 也成立。
func kbdChipContrast(t *testing.T, dark bool) (ratio float64, corePx int) {
	t.Helper()
	th := themeFor(dark, ui.Preferences{})
	pal := paletteFor(th, laneSet{})

	a := richApp(t)
	// 不暂存的话按钮是禁用态，芯片那一块根本不画。
	a.stageAll()
	settle(t, a)

	const w, h = 400, 26
	view := func(c *ui.Context) {
		ui.Box(c).Size(w, h).Shrink(0).Children(func() {
			a.aiCommitButton(c, th, pal)
		})
	}
	tt := ui.NewTester(view, w, h)
	tt.SetDark(dark)
	still(tt)
	tt.Frame()
	img := tt.Image()
	at := func(x, y int) rgbpx {
		r, g, b, _ := img.At(x, y).RGBA()
		return rgbpx{r >> 8, g >> 8, b >> 8}
	}

	// 按钮自己那道 1px 发丝边也是「墨」—— 不剃掉的话会把边框当成要找的字。
	// 内缩 3px：1px 边 + 2px 抗锯齿，够走到填充里去。
	const inset = 3
	ground := modalColorIn(inset, w-inset, inset, h-inset, at)
	ink := func(x, y int) bool { return delta(at(x, y), ground) > inkGate }

	col := make([]int, w)
	for x := inset; x < w-inset; x++ {
		for y := inset; y < h-inset; y++ {
			if ink(x, y) {
				col[x]++
			}
		}
	}
	// 一列有墨就连成段，中间空窗 <=3 列算同一段（字的笔画之间本来就有缝）。
	type span struct{ x0, x1 int }
	var spans []span
	for x := inset; x < w-inset; {
		if col[x] == 0 {
			x++
			continue
		}
		s := span{x, x}
		for x < w-inset {
			if col[x] > 0 {
				s.x1 = x
				x++
				continue
			}
			gap := 0
			for x+gap < w-inset && col[x+gap] == 0 {
				gap++
			}
			if gap <= 3 && x+gap < w-inset {
				x += gap
				continue
			}
			break
		}
		spans = append(spans, s)
	}
	// 最右一段、宽度够一个字号的 —— 就是 ⌘I。
	var chip span
	for _, s := range spans {
		if s.x1-s.x0+1 >= 4 {
			chip = s
		}
	}
	if chip.x1 <= chip.x0 {
		t.Fatalf("dark=%v: 按钮里找不到快捷键那段字 —— 按钮被禁用还是排版变了？", dark)
	}

	y0, y1 := h, 0
	for x := chip.x0; x <= chip.x1; x++ {
		for y := inset; y < h-inset; y++ {
			if ink(x, y) {
				y0 = min(y0, y)
				y1 = max(y1, y)
			}
		}
	}
	// 那段字自己的底子：这一小片里的模态色。
	chipBg := modalColorIn(chip.x0, chip.x1+1, y0, y1+1, at)
	if chipBg == ground {
		t.Logf("dark=%v: 芯片底和按钮底同色（%v）—— 洗色没生效，量到的还是按钮底", dark, chipBg)
	}

	// 笔画核心 = 偏离底子够远的那些像素的模态色。
	//
	// 按最大色道的值取最值，不按 map 迭代顺序：11px 的字只有十来粒核心像素，
	// 众数常常并列，按 map 顺序取值的话同一个用例两次跑会给出不同的数。
	core := map[rgbpx]int{}
	for x := chip.x0; x <= chip.x1; x++ {
		for y := y0; y <= y1; y++ {
			if p := at(x, y); delta(p, chipBg) >= coreGate {
				core[p]++
			}
		}
	}
	var peak rgbpx
	for c, n := range core {
		if n > corePx || (n == corePx && peak.rgb() < c.rgb()) {
			peak, corePx = c, n
		}
	}
	if corePx == 0 {
		// 一个核心像素都没有 —— 整段字全是抗azakiash摊出来的灰。这时候
		// 「核心色」无从谈起，按最亮的那一粒算也说明不了问题，直接交 0
		// 让调用方知道这段字根本没落成实心。
		return 0, 0
	}
	return contrastRGB(peak, chipBg), corePx
}

// rgb 把像素摊成一个可比的整数。用来在众数并列时定一个确定的次序：
// 11px 的字只有十来粒核心像素，并列是常事，按 map 迭代顺序取值的话同一个
// 用例两次跑会给出不同的数。
func (p rgbpx) rgb() uint64 {
	return uint64(p.r)<<32 | uint64(p.g)<<16 | uint64(p.b)
}

// modalColorIn 取这一片里出现次数最多的颜色 —— 那就是底子。
func modalColorIn(x0, x1, y0, y1 int, at func(x, y int) rgbpx) rgbpx {
	counts := map[rgbpx]int{}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			counts[at(x, y)]++
		}
	}
	var best rgbpx
	n := 0
	for c, k := range counts {
		if k > n {
			best, n = c, k
		}
	}
	return best
}

// delta 是两个像素在最大色道上的差。两个颜色都偏灰（这套灰阶就是灰的），
// 所以单看一个道就够了；取最大是为了不让某一道上的噪声把「是不是墨」判错。
func delta(a, b rgbpx) int {
	d := func(x, y uint32) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return max(d(a.r, b.r), max(d(a.g, b.g), d(a.b, b.b)))
}

// contrastRGB 是 WCAG 的对比度（1~21）。和 theme_test.go 的 contrastRatio 同一
// 套公式，只是入口是渲染出来的像素而不是 ui.Color。
func contrastRGB(a, b rgbpx) float64 {
	lum := func(p rgbpx) float64 {
		ch := func(v uint32) float64 {
			s := float64(v) / 255
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(p.r) + 0.7152*ch(p.g) + 0.0722*ch(p.b)
	}
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestKbdHintIsLegible 断言快捷键提示够得着 WCAG 给正文定的 4.5:1。
//
// 阈值不挑 3:1：3:1 是给「大字号文字」和「UI 组件的边界」定的，而这是 11px
// 的正文级小字，4.5:1 才是它的档。界面上其余正文都在 4.5:1 以上
// （theme_test.go 的 TestShellGroundKeepsDarkThemeDark 就按这条盯次要文字），
// 没道理唯独「告诉用户快捷键是什么」的那一小段是例外 —— 它恰恰是给「还不
// 知道这个快捷键的人」看的。同一角色的另一半在终端空态页上（tabs.go 的
// 「快捷键 ⌘⇧T」）走 t.TextMuted 直接画在卡片上，那一屏是够 4.5:1 的。
func TestKbdHintIsLegible(t *testing.T) {
	for _, dark := range []bool{true, false} {
		ratio, core := kbdChipContrast(t, dark)
		if core == 0 {
			t.Errorf("dark=%v: 快捷键那段字一个实心像素都没有 —— 整段全是抗锯齿摊出来的灰。"+
				" 11px 的字落到这个地步，笔画从来没落成过一个像素。", dark)
			continue
		}
		if ratio < 4.5 {
			t.Errorf("dark=%v: 快捷键提示的对比度 %.2f:1，低于 4.5:1 —— 字糊在自己的底子上。"+
				" 这是全界面最小的一段字（11px），它最需要够得着。", dark, ratio)
		}
	}
}
