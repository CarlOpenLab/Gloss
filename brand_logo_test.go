// 站点页头那颗品牌图有两个槽位，按主题各给一张：深色页头上要一张浅色的，
// 浅色页头上要一张深色的。pages/index.html 用
// <source media="(prefers-color-scheme: dark)"> 选 logo-dark.png，浅色选
// logo.png，两张由同一支 SVG 出，只是磁贴的明暗相反（resources/logo.svg 与
// resources/logo-light.svg）。README 的 <picture> 是同一对文件、同一套主题
// 判断（README.md 第 3~5 行）。
//
// 出过的事故：换新 logo 时，assets 里的 PNG 被整批替换成了同一张深色磁贴。
// logo-dark.png 原本是浅色记号，一换之下深色页头上只剩一块和背景几乎同色的
// 方斑 —— 磁贴和页头的对比度只有 2.1，而 WCAG 对图形对象的下限是 3.0。CSS
// 那套 display:none 的主题切换一切正常，两张图的文件名、尺寸、圆角、甚至
// 内容都是同一张，所以读代码发现不了，只有量像素才看得出来。
//
// 所以这里量的就是「这一张在它那个槽位上读得出来」：把 PNG 按 alpha 叠到页头
// 底色上，用现成的 contrastRatio 算 WCAG 对比度。阈值取 3.0（WCAG 对非文字
// 图形的最低要求），事故那一版会红，正常的两张 14.9 / 12.3 离得远远的。

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 页头底色，取自 pages/style.css 的 --bg（深色 #0c0b10，浅色 #f2f3f5）。
// 站点头部是 color-mix(--bg 78%, transparent) 铺在窗口上，取 --bg 是量最
// 不利的那一档：背景越深，浅色记号越显，反过来越浅越显，用纯色比用半透明
// 更严。
var (
	brandHeaderDark  = ui.RGB(0x0c, 0x0b, 0x10)
	brandHeaderLight = ui.RGB(0xf2, 0xf3, 0xf5)
)

// brandMinContrast 是「读得出来」的下限：WCAG 对非文字图形的最低要求。
const brandMinContrast = 3.0

// brandTileOnHeader 把 png 叠到页头底色上，量磁贴内的平均颜色（0~255）。
//
// 只量中间 60%：圆角之外是透明的，叠上去就是页头底色，算进去只会把两个方向
// 的分差一起摊薄；中间那一块稳稳在磁贴里面，记号全在那儿。叠放按浏览器的
// src-over 算法。
func brandTileOnHeader(t *testing.T, path string, header ui.Color) ui.Color {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("打不开 %s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("解不了 %s: %v", path, err)
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		t.Fatalf("%s 不是正方形：%dx%d", path, b.Dx(), b.Dy())
	}

	x0 := b.Min.X + b.Dx()*2/10
	x1 := b.Max.X - b.Dx()*2/10
	y0 := b.Min.Y + b.Dy()*2/10
	y1 := b.Max.Y - b.Dy()*2/10

	var sr, sg, sb, sum float64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			alpha := float64(a) / 65535
			sr += float64(r>>8)*alpha + float64(header.R)*(1-alpha)
			sg += float64(g>>8)*alpha + float64(header.G)*(1-alpha)
			sb += float64(bl>>8)*alpha + float64(header.B)*(1-alpha)
			sum += alpha
		}
	}
	n := float64((x1 - x0) * (y1 - y0))
	if n == 0 {
		t.Fatalf("%s 中间量不到像素", path)
	}
	return ui.RGB(uint8(sr/n+0.5), uint8(sg/n+0.5), uint8(sb/n+0.5))
}

// TestEachBrandLogoSlotReadsAgainstItsHeader 断言每个槽位的图都读得出来。
func TestEachBrandLogoSlotReadsAgainstItsHeader(t *testing.T) {
	root := repoRoot(t)
	slots := []struct {
		path  string
		bg    ui.Color
		where string
	}{
		{"pages/assets/logo-dark.png", brandHeaderDark, "站点深色页头"},
		{"pages/assets/logo.png", brandHeaderLight, "站点浅色页头"},
		{"resources/logo-dark.png", brandHeaderDark, "README 深色"},
		{"resources/logo.png", brandHeaderLight, "README 浅色"},
	}
	for _, s := range slots {
		tile := brandTileOnHeader(t, filepath.Join(root, s.path), s.bg)
		if got := contrastRatio(tile, s.bg); got < brandMinContrast {
			t.Errorf("%s（%s）：磁贴 %v 叠在页头 %v 上对比度只有 %.2f:1 —— "+
				"低于 WCAG 对图形对象的 %.1f:1。这个槽位上的品牌图读不出来，页头上只剩一团和背景同色的方斑。",
				s.path, s.where, tile, s.bg, got, brandMinContrast)
		}
	}
}

// TestTheTwoBrandLogoTilesAreNotTheSameImage 断言两张图真的按主题分了。
//
// 上一管量的是「读得出来」，这一管量的是「方向是对的」：一张不深不浅的中间灰
// 在深色页头上可能勉强过 3.0，在浅色页头上却过不去，反过来也一样。只有两头
// 各自方向对了，才说明这一对是两套配色，而不是同一张图贴了两处。
//
// 拿两张磁贴彼此的对比度比（都叠到同一个中性灰上）：正常的浅色磁贴 0.67、
// 深色磁贴 0.14，彼此相差 4.7 倍；同一张图则完全相等，比值 1.0。阈值取 2，
// 离两头都远 —— 一次把深色槽位换成「没那么浅」的磁贴（比如米白）会掉到 2
// 以下，那就正是这一管要拦的。
func TestTheTwoBrandLogoTilesAreNotTheSameImage(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{"pages/assets", "resources"} {
		dark := brandTileOnHeader(t, filepath.Join(root, dir, "logo-dark.png"), ui.RGB(0, 0, 0))
		light := brandTileOnHeader(t, filepath.Join(root, dir, "logo.png"), ui.RGB(0, 0, 0))
		r := contrastRatio(dark, light)
		if r < 2 {
			t.Errorf("%s：深色槽位磁贴 %v、浅色槽位 %v，彼此只有 %.2f:1 —— "+
				"两张像是同一张图贴了两处。深色槽位该是浅色磁贴，浅色槽位该是深色磁贴。",
				dir, dark, light, r)
		}
	}
}
