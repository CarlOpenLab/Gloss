// 「AI 生成」旁边那颗 ✨ 图标在按钮里看着发虚（用户原话：有点看不清）。
//
// 它不是颜色不够 —— 静息态 inkMuted 在 chip 底上是够的，悬停还会提到 ink。
// 虚的是**笔画**：iconSparkle 是从 Lucide 的 24×24 viewBox 描出来的，
// stroke-width=2，画到 11px 的按钮里只剩 2×11/24 ≈ 0.92 个像素宽。
// 不到一个设备像素，笔画被抗锯齿摊成半透明的一层灰，四只角就糊在一起。
//
// 所以这里量的是「实心像素」：把图标画在深色底上数有多少个像素真的亮起来。
// 修好之前 11px 上只有 52 个，加到 3.2 之后翻倍到 104 —— 原来糊掉的那一层
// 就是被摊薄的那半个像素。
//
// 顺手把 16px 那一档也盯住：prdialog 里那颗画得大，笔画加重之后不能糊成
// 一坨实心，所以两头都断言「变重了，但没变成色块」。

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// iconInk 把 svg 画到深色底上，量它有多少个像素真的亮起来（≥0.85 算实心）
// 以及总墨量。用 R 通道而不是 alpha：无头窗口自己的底是不透明的，
// 读 alpha 读到的是窗口背景，不是图标。
func iconInk(t *testing.T, svg *ui.SVG, size float32) (solid int, weight float64) {
	t.Helper()
	n := int(size) + 8
	view := func(c *ui.Context) {
		ui.Box(c).Size(float32(n), float32(n)).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Icon(svg, r, ui.Hex("#ffffff"))
		})
	}
	tt := ui.NewTester(view, n, n)
	tt.SetDark(true)
	still(tt)
	tt.Frame()
	img := tt.Image()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			v := float64(r) / 65535
			weight += v
			if v >= 0.85 {
				solid++
			}
		}
	}
	return solid, weight
}

// TestSparkleIconIsLegibleAtButtonSize 断言火花在按钮里那个尺寸下笔画够重。
//
// 阈值取 11px 下实测值的一半多一点：现在 52，修好要 ≥ 80。不是拍一个好看
// 的数 —— 是「不到一个像素宽的那层灰被填实」这件事本身。16px 下反而设上限：
// 那颗画得大，笔画一起加重会糊成色块。
func TestSparkleIconIsLegibleAtButtonSize(t *testing.T) {
	solid, weight := iconInk(t, iconSparkle, 11)
	if solid < 80 {
		t.Errorf("11px 上火花的实心像素只有 %d —— 笔画被摊薄到半个像素以下，四只角糊在一起。", solid)
	}
	if weight < 110 {
		t.Errorf("11px 上火花的墨量只有 %.1f —— 图标整体发虚。", weight)
	}

	big, bigWeight := iconInk(t, iconSparkle, 16)
	if big <= solid {
		t.Errorf("16px 的火花（%d）不该比 11px 的（%d）还淡 —— 大的那颗画在 PR 对话框里，同样的虚。", big, solid)
	}
	// 加重到 3.2 时 16px 是 168。留一档余量，但明确不许糊成实心块：
	// 一颗描边图标如果变成色块，就换来了另一个问题。
	if big > 240 {
		t.Errorf("16px 的火花有 %d 个实心像素 —— 描边图标糊成色块了。", big)
	}
	_ = bigWeight
}

// TestSparkleIconStaysAnOutline 断言火花始终是描边图标，不是实心块。
//
// 上一管「够重」，这一管「别过重」：最省事的修法是把 fill 打开 —— 11px 上它
// 的实心像素是 57，看着比描边的 52 还多。但那是另一回事：四角星变成一团实心，
// 浅色下一团黑、深色下一团白，在按钮里比虚更糟，而且和旁边所有 Lucide 图标
// 不再是一套画法。
//
// 拿中心量：描边的四角星中心是空的，填充的是一条贯通的实心带。这正好是
// 「描边」和「填充」的区别本身，比数像素数更靠得住 —— 两头都能数出几十个
// 实心像素，只有中心的空与不空分得开。
func TestSparkleIconStaysAnOutline(t *testing.T) {
	const size = 11
	n := int(size) + 4
	view := func(c *ui.Context) {
		ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Icon(iconSparkle, r, ui.Hex("#ffffff"))
		})
	}
	tt := ui.NewTester(view, n, n)
	tt.SetDark(true)
	still(tt)
	tt.Frame()
	img := tt.Image()
	lit := func(x, y int) float64 {
		r, _, _, _ := img.At(x, y).RGBA()
		return float64(r) / 65535
	}

	// 图标的中心：盒子左上角在 (0,0)，size×size，所以中心在 size/2。
	cx, cy := int(size)/2, int(size)/2
	hollow := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if lit(cx+dx, cy+dy) < 0.12 {
				hollow++
			}
		}
	}
	if hollow == 0 {
		t.Errorf("火花中心 %d 个像素全是实的 —— 它变成填充块了。"+
			" 描边的四角星中心该是空的（fill=none）。", 9)
	}
}
