package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 亚克力外观的四条：档位名是配置里的定值、未知档位收敛回柔和、材质映射和
// beacon 同一张表、不透明度有默认值与上下限。任何一条坏了，用户改设置之后
// 窗口要么没有材质，要么变成一个不由他决定的透明度。
func TestBlurNormalizesToKnownChoices(t *testing.T) {
	for _, id := range blurChoices {
		if got := normalizeBlur(id); got != id {
			t.Errorf("normalizeBlur(%q) = %q，有效档位不该被改写", id, got)
		}
	}
	for _, id := range []string{"", "bogus", "Deep", "acrylic"} {
		if got := normalizeBlur(id); got != blurSubtle {
			t.Errorf("normalizeBlur(%q) = %q，未知档位应回落到 %q", id, got, blurSubtle)
		}
	}
	if got := blurIndex("bogus"); got != 1 {
		t.Errorf("blurIndex(bogus) = %d，应落到柔和那一格（1）", got)
	}
	for i, id := range blurChoices {
		if got := blurIndex(id); got != i {
			t.Errorf("blurIndex(%q) = %d，应为 %d", id, got, i)
		}
	}
}

func TestVibrancyFollowsBlurChoice(t *testing.T) {
	cases := map[string]mygo.Vibrancy{
		blurNone:   mygo.VibrancyNone,
		blurSubtle: mygo.VibrancySidebar,
		blurMedium: mygo.VibrancyPopover,
		blurDeep:   mygo.VibrancyAcrylic,
		"bogus":    mygo.VibrancySidebar,
	}
	for blur, want := range cases {
		if got := vibrancyFor(blur); got != want {
			t.Errorf("vibrancyFor(%q) = %q，应为 %q", blur, got, want)
		}
	}
}

func TestOpacityDefaultAndClamp(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, bgOpacityDefault},  // 没设过 / 旧配置
		{-1, bgOpacityDefault}, // 坏值当没设过
		{0.1, bgOpacityMin},    // 低于下限
		{1.5, bgOpacityMax},    // 高于上限
		{0.65, 0.65},           // 区间内原样
	}
	for _, tc := range cases {
		if got := normalizeOpacity(tc.in); got != tc.want {
			t.Errorf("normalizeOpacity(%v) = %v，应为 %v", tc.in, got, tc.want)
		}
	}
}

// 工作卡的底色跟着设置走：默认 80% 的 pal.card，拧到 100% 就和实色卡完全一样。
func TestCardFillFollowsBackgroundOpacity(t *testing.T) {
	a := newApp()
	pal := paletteFor(themeFor(false, ui.Preferences{}), laneSet{})
	if got, want := a.cardFill(pal), pal.card.Alpha(0.80); got != want {
		t.Errorf("默认 cardFill = %v，应为 %v", got, want)
	}
	a.bgOpacity = 1
	if got := a.cardFill(pal); got != pal.card {
		t.Errorf("100%% 时 cardFill = %v，应与 pal.card 相同（%v）", got, pal.card)
	}
}

// 根这一层：有材质时是主题色的薄纱，没有材质时是实色 sideBg。材质跟的是
// 系统外观，不是应用里固定的深浅 —— 窗口底必须由主题色兜底，不能整个交给
// 材质（浅色系统 + 深色应用时，侧栏和卡片四周会亮成一片）。
func TestShellGroundTintsMaterialToTheme(t *testing.T) {
	pal := paletteFor(themeFor(false, ui.Preferences{}), laneSet{})
	if got, want := shellGround(pal, true), pal.sideBg.Alpha(shellTint); got != want {
		t.Errorf("有材质时根 = %v，应为 sideBg 的 %.0f%% 薄纱 %v", got, shellTint*100, want)
	}
	if got := shellGround(pal, false); got != pal.sideBg {
		t.Errorf("无材质时根应为实色 sideBg（%v），实际 %v", pal.sideBg, got)
	}
}

// 深色主题压在最亮的材质上也还得是暗的：薄纱太薄的话，侧栏和卡片四周会
// 亮成一片（2026-10-30 的截图里，材质约 #898c8d，窗口底就直接是它自己，
// 深色文字全糊了）。按那支亮度合成后，窗口底要和次要文字拉开 4.5:1。
func TestShellGroundKeepsDarkThemeDark(t *testing.T) {
	dark := paletteFor(themeFor(true, ui.Preferences{}), laneSet{})
	const material = 0x898c8d // 浅色系统上主窗口材质实测的灰度
	blend := func(fg ui.Color, bg uint32) ui.Color {
		a := float64(fg.A) / 255
		ch := func(v uint8, shift uint) uint8 {
			return uint8(math.Round(float64(v)*a + float64((bg>>shift)&0xff)*(1-a)))
		}
		return ui.RGB(ch(fg.R, 16), ch(fg.G, 8), ch(fg.B, 0))
	}
	ground := blend(shellGround(dark, true), material)
	if luma(ground) > 0.35 {
		t.Errorf("深色窗口底压在材质上亮度 %.2f，偏亮", luma(ground))
	}
	if got := contrastRatio(ground, dark.inkMuted); got < 4.5 {
		t.Errorf("次要文字在窗口底上对比度 %.1f:1，低于 4.5:1", got)
	}
}

// contrastRatio 是 WCAG 的对比度（1~21），文字可读性的判据。
func contrastRatio(a, b ui.Color) float64 {
	l := func(c ui.Color) float64 {
		ch := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	la, lb := l(a), l(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// themeFor 每帧都要交出一份同样的主题，所以它的几处改动必须是**赋值**而不是
// 累加（`t.FontSize = b.FontSize * s`，不是 `t.FontSize *= s`）。
//
// 写成累加的样子就坏了：主题对象是跨帧复用的，同一个偏好会被逐帧乘进去，
// 字号一秒钟之内指数级涨上去。这种坏法在第一帧的截图里看不出来，得让同一个
// 偏好连来几帧才现形。
func TestThemeForIsIdempotent(t *testing.T) {
	prefs := ui.Preferences{TextScale: 1.5, HighContrast: true}
	for _, dark := range []bool{false, true} {
		first := *themeFor(dark, prefs)
		for i := 0; i < 5; i++ {
			got := *themeFor(dark, prefs)
			if got.FontSize != first.FontSize {
				t.Fatalf("dark=%v: 第 %d 帧字号变成 %.2f，首帧是 %.2f —— 偏好被逐帧累乘了",
					dark, i+2, got.FontSize, first.FontSize)
			}
			if got.Border != first.Border {
				t.Fatalf("dark=%v: 第 %d 帧边框色漂了 —— 高对比混色被逐帧叠加了", dark, i+2)
			}
		}
	}
}

// 偏好撤掉之后要能回到基准。
//
// 这是「每帧从基准重来」那条规则的**唯一**守护：主题对象跨帧复用，若某一帧忘记
// 先拷基准再改，用户把系统里的「增大字号」关掉之后界面会停在放大状态，直到重启
// 才好 —— 而重启之后又复现不了，是最难查的那种。
func TestThemeForRestoresBaseWhenPrefsClear(t *testing.T) {
	for _, dark := range []bool{false, true} {
		base := *themeFor(dark, ui.Preferences{})
		_ = themeFor(dark, ui.Preferences{TextScale: 2, HighContrast: true})
		back := *themeFor(dark, ui.Preferences{})
		if back.FontSize != base.FontSize || back.Border != base.Border {
			t.Errorf("dark=%v: 偏好清空后没回到基准（字号 %.2f→%.2f，边框 %v→%v）",
				dark, base.FontSize, back.FontSize, base.Border, back.Border)
		}
	}
}

// 两条无障碍路径各自真的生效，别只是「不改坏」。
func TestThemeForAppliesPrefs(t *testing.T) {
	for _, dark := range []bool{false, true} {
		base := *themeFor(dark, ui.Preferences{})

		big := *themeFor(dark, ui.Preferences{TextScale: 2})
		if big.FontSize <= base.FontSize {
			t.Errorf("dark=%v: TextScale=2 没放大字号（%.2f）", dark, big.FontSize)
		}

		hc := *themeFor(dark, ui.Preferences{HighContrast: true})
		if hc.Border == base.Border {
			t.Errorf("dark=%v: HighContrast 没把边框色往正文色靠", dark)
		}
		if hc.Focus.A != 255 {
			t.Errorf("dark=%v: HighContrast 下焦点环应当不透明，实际 A=%d", dark, hc.Focus.A)
		}
	}
}

// 设计 token 是规范里的定值，不跟着系统外观或无障碍偏好走 —— 否则「提高对比度」
// 一开，整屏的圆角、发丝线、表面阶梯就全漂了，而规范里写的还是那几个值。
func TestDesignTokensArePreferenceIndependent(t *testing.T) {
	prefs := ui.Preferences{TextScale: 2, HighContrast: true}
	for _, dark := range []bool{false, true} {
		base := paletteFor(themeFor(dark, ui.Preferences{}), laneSet{})
		got := paletteFor(themeFor(dark, prefs), laneSet{})
		for _, tc := range []struct {
			name     string
			a, b     ui.Color
			expected string
		}{
			{"hairline", base.hairline, got.hairline, "发丝线"},
			{"surface1", base.surface1, got.surface1, "表面阶梯"},
			{"surface4", base.surface4, got.surface4, "浮层表面"},
			{"inkMuted", base.inkMuted, got.inkMuted, "次要文字"},
			{"selection", base.selection, got.selection, "选中底带"},
		} {
			if tc.a != tc.b {
				t.Errorf("dark=%v: 开了无障碍偏好之后 %s 变了（%v → %v）—— 设计 token 应当是定值",
					dark, tc.expected, tc.a, tc.b)
			}
		}
	}
}

// TestLightShellGroundIsNeutral 断言浅色「窗口」那一层没有色相。
//
// 窗口底是屏上面积最大的一块颜色，一旦带上粉紫，白卡片旁边就浮着一层洗不掉
// 的染色 —— 之前那版粉紫到奶油的渐变就是这个问题。现在只有一层中性冷灰
// （和 term-fleet 的 plain 同档），和 §2「界面不含色相」一致：颜色留给 diff
// 的增删与拓扑图，底色不参与表达。
//
// 不用 saturation() 判：那是给中间明度的强调色用的，HSL 饱和度在接近白色时
// 会被明度放大 —— #eceef4 这种只差 8 个通道值的冷灰也能算出 0.27。这里看
// 通道间距（max−min），十几级以内才算「灰」。
func TestLightShellGroundIsNeutral(t *testing.T) {
	pal := paletteFor(themeFor(false, ui.Preferences{}), laneSet{})
	for _, tc := range []struct {
		name string
		c    ui.Color
	}{
		{"sideBg", pal.sideBg},
	} {
		spread := int(max(tc.c.R, tc.c.G, tc.c.B)) - int(min(tc.c.R, tc.c.G, tc.c.B))
		if spread > 12 {
			t.Errorf("浅色 %s = %s，通道间距 %d —— 窗口底应当是中性灰，不能带粉紫",
				tc.name, hexOf(tc.c), spread)
		}
	}
}

// isViolet 判断一个颜色是不是落在紫 / 靛 / 鸢尾那一系。
//
// 判据不能是色相角。上一版的主色 #5e6ad2 色相 233°，而一道明确的钢青蓝
// （拓扑图 3 号线的 #4c86e8）是 218° —— 只差 15°，圈一段色相区间的话，
// 要么放过薰衣草，要么误伤蓝。
//
// 真正把紫从蓝里分出来的是**红绿两通道的距离**：蓝里掺红才是紫，纯蓝的红
// 远低于绿。#5e6ad2 的红 94、绿 106（红追到了绿的 89%），#4c86e8 的红 76、
// 绿 134（只有 57%）。判据于是写成「蓝是最大通道，且红追到绿的 85% 以上」，
// 再排掉接近中性的灰 —— 蓝和另一通道差得太小时，谈不上色相。
func isViolet(c ui.Color) bool {
	if c.B <= c.R || c.B <= c.G {
		return false // 蓝不是主导通道
	}
	if int(c.B)-int(max(c.R, c.G)) < 24 {
		return false // 接近中性
	}
	return float64(c.R) >= 0.85*float64(c.G)
}

// TestAccentPalette pins the blue accent shared by both appearances.
//
// The main action, focus ring, selection and navigation cues all read this
// value, so a small drift here changes the whole shell at once.
func TestAccentPalette(t *testing.T) {
	// Keep the violet guard: the blue may change depth, but not drift back.
	for _, old := range []string{"#5e6ad2", "#828fff", "#5e69d1", "#9d8df1", "#7e6cd8"} {
		if !isViolet(ui.Hex(old)) {
			t.Errorf("自查失败：%s 是上一版的紫，这道闸却放行了", old)
		}
	}

	for _, dark := range []bool{false, true} {
		th := themeFor(dark, ui.Preferences{})
		pal := paletteFor(th, laneSet{})

		wantAccent := ui.Hex("#3b6cf6")
		wantHover := ui.Hex("#527ef7")
		wantPressed := ui.Hex("#2f5ddb")
		wantFocus := ui.RGBA(59, 108, 246, 0.55)
		wantSelection := ui.RGBA(59, 108, 246, 0.25)
		if dark {
			wantAccent = ui.Hex("#6b93ff")
			wantHover = ui.Hex("#82a3ff")
			wantPressed = ui.Hex("#5b82ee")
			wantFocus = ui.RGBA(107, 147, 255, 0.65)
			wantSelection = ui.RGBA(107, 147, 255, 0.25)
		}

		for _, tc := range []struct {
			name string
			got  ui.Color
			want ui.Color
		}{
			{"Accent", th.Accent, wantAccent},
			{"AccentHover", th.AccentHover, wantHover},
			{"AccentPressed", th.AccentPressed, wantPressed},
			{"Focus", th.Focus, wantFocus},
			{"Selection", th.Selection, wantSelection},
			{"pal.accent", pal.accent, wantAccent},
		} {
			if tc.got != tc.want {
				t.Errorf("dark=%v: %s = %#v, want %#v", dark, tc.name, tc.got, tc.want)
			}
			if isViolet(tc.got) {
				t.Errorf("dark=%v: %s 漂回紫色（彩度 %.2f）", dark, tc.name, saturation(tc.got))
			}
		}

		if th.AccentText != ui.Hex("#ffffff") {
			t.Errorf("dark=%v: 强调键上的文字应为白色，实际 %#v", dark, th.AccentText)
		}
	}
}

// TestGraphLanesHaveNoPurple 断言拓扑图的支线里没有紫 / 靛。
//
// 八条线同时出现在一屏里，任何一条是紫，整张图就会被读成「紫色的那个软件」——
// 主干是一支蓝，其余的七条也必须是明确的非紫。
func TestGraphLanesHaveNoPurple(t *testing.T) {
	for _, dark := range []bool{false, true} {
		pal := paletteFor(themeFor(dark, ui.Preferences{}), laneSet{})
		for i, c := range pal.lanes {
			if isViolet(c) {
				t.Errorf("dark=%v: 第 %d 条泳道 %v 是紫/靛（彩度 %.2f）—— 拓扑图整份色板不含紫",
					dark, i, c, saturation(c))
			}
		}
	}
}

// TestGraphMainLaneIsTheBlue 断言 0 号色（主干 / HEAD）带色相，而且是全图
// 唯一一支蓝。
//
// 曾经 0 号是中性 Ink（浅色下近黑、深色下近白），那一版回退过：主线降级成
// 「又一条支线」，而 0 号同时是侧栏当前分支圆点、HEAD 芯片点、提交栏状态胶囊
// 的颜色，整屏多出一串同色点。现在主干走一支彩色（钢青蓝），那几处跟着一起
// 变 —— 这是明确选定过的取舍（用户拍板），这里守住的是它的另一半：色相独占。
//
// 两支蓝并排会被读成「一条蓝渐变 + 几个别的色」，支线之间分不出彼此，图就白
// 画了；mono 那一套整份灰阶，是「不靠色相」的选项，0 号保持最亮/最重的一阶。
func TestGraphMainLaneIsTheBlue(t *testing.T) {
	for _, p := range lanePresets {
		for _, dark := range []bool{false, true} {
			cols := p.lanes(dark)
			main := cols[0]
			if p.id == "mono" {
				continue
			}
			if hue(main) < 0 {
				t.Errorf("%s(dark=%v)：主干色 %v 没有色相 —— 主干该是一支彩色", p.id, dark, main)
			}
			if !isBlueFamily(main) {
				t.Errorf("%s(dark=%v)：主干 %v 不是蓝的", p.id, dark, main)
			}
			blues := 0
			for i, c := range cols {
				if isBlueFamily(c) {
					blues++
					if i != 0 {
						t.Errorf("%s(dark=%v)：第 %d 号 %v 也是蓝的 —— 全图只允许一支蓝，且是主干",
							p.id, dark, i, c)
					}
				}
			}
			if blues != 1 {
				t.Errorf("%s(dark=%v)：有 %d 支蓝", p.id, dark, blues)
			}
		}
	}
}

// spread 是 RGB 三通道的极差，用作「有没有色相」的粗判据。
func spread(c ui.Color) int {
	lo, hi := int(c.R), int(c.R)
	for _, v := range []int{int(c.G), int(c.B)} {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return hi - lo
}

// saturation 是 HSL 定义下的彩度（0~1），只用于报错信息里的程度描述。
func saturation(c ui.Color) float64 {
	hi, lo := float64(max(c.R, c.G, c.B))/255, float64(min(c.R, c.G, c.B))/255
	if hi == lo {
		return 0
	}
	if hi+lo > 1 {
		return (hi - lo) / (2 - hi - lo)
	}
	return (hi - lo) / (hi + lo)
}

// luma 是近似感知亮度（0~1），只用来比较「谁比谁亮」，不做色彩科学。
func luma(c ui.Color) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// 内置的拓扑图方案都得守住 §2.6 那几条：0 号是主干（带色相，mono 除外）、
// 8 支两两不同、
// 全图只有一支蓝。方案是给用户挑的，挑完照样得能读图 —— 这条就是那道闸。
func TestLanePresetsKeepDesignRules(t *testing.T) {
	for _, p := range lanePresets {
		for _, dark := range []bool{false, true} {
			cols := p.lanes(dark)
			// 0 号是主干：classic / vivid 里必须是一支彩色（且是唯一的蓝，
			// 下面那条看着），mono 整份灰阶例外。
			if p.id != "mono" && hue(cols[0]) < 0 {
				t.Errorf("%s(dark=%v)：0 号是主干，得带色相，实际 %v", p.id, dark, cols[0])
			}
			seen := map[ui.Color]int{}
			blues := 0
			for i, c := range cols {
				if prev, dup := seen[c]; dup {
					t.Errorf("%s(dark=%v)：第 %d 号与第 %d 号同色 %v —— 两条线同色就分不出彼此",
						p.id, dark, i, prev, c)
				}
				seen[c] = i
				if isBlueFamily(c) {
					blues++
				}
				// Don'ts 里那条「禁止品牌紫 / 薰衣草蓝」：紫、鸢尾那一带
				// （色相 260°~320°）不许出现在拓扑图上。再往蓝偏的靛
				// （#5e6ad2 那一系，色相 230° 上下）由上面那条「只有一支蓝」
				// 顺手挡掉 —— 它本来就判成蓝。
				if h := hue(c); h >= 260 && h <= 320 {
					t.Errorf("%s(dark=%v)：第 %d 号 %v 的色相 %.0f° 落在紫 / 鸢尾 / 靛那一带",
						p.id, dark, i, c, h)
				}
			}
			if blues > 1 {
				t.Errorf("%s(dark=%v)：有 %d 支蓝 —— 并排出现会被读成一条蓝渐变（§2.6）",
					p.id, dark, blues)
			}
		}
	}
}

// hue 是 HSL 定义下的色相角（0~360）。灰阶没有色相，返回 -1。
func hue(c ui.Color) float64 {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := hi - lo
	if d < 0.02 {
		return -1
	}
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h
}

// isBlueFamily 粗判一支色是不是「蓝」：蓝通道最大、且明显压过次大的那个。
//
// 不引色彩科学 —— 这条闸要挡的是「三支蓝并排」，判据只需要像不像蓝。
func isBlueFamily(c ui.Color) bool {
	blue := int(c.B)
	rest := max(int(c.R), int(c.G))
	return blue > rest && blue-rest >= 40
}

// 出厂那一套就是 DESIGN.md §2.6 写死的 8 色。它现在是「可自定义」的默认值，
// 谁改掉它等于改了规范，所以在这里钉一遍：深色、浅色各 8 个。
// 0 号是主干（钢青蓝），3 号是它让出来之后补上的芥末黄。
func TestClassicLanesMatchDesignSpec(t *testing.T) {
	want := map[bool][]string{
		true: {
			"#4c86e8", "#3eb489", "#e09f3e", "#c9a227",
			"#e5484d", "#d977a8", "#38b2ac", "#9aab45",
		},
		false: {
			"#2f6fd0", "#2d9d74", "#c9822b", "#a8860b",
			"#cf3b40", "#b85487", "#288c87", "#6f7a1f",
		},
	}
	got := lanePresetByID("classic").lanes(true)
	for _, dark := range []bool{true, false} {
		got = lanePresetByID("classic").lanes(dark)
		for i, hex := range want[dark] {
			if got[i] != ui.Hex(hex) {
				t.Errorf("classic(dark=%v) 第 %d 号 = %v，规范里是 %s", dark, i, got[i], hex)
			}
		}
	}
}

// 覆盖按号生效：第 N 号的覆盖只动第 N 号，其余仍是方案自带的颜色。
//
// 顺带钉住「色号是线的身份」这件事：覆盖是落在色号上的，不是落在泳道上的
// —— 一条线换了泳道还得是同一个颜色（见 buildGraph 那段注释）。
func TestLaneOverridesApplyPerLane(t *testing.T) {
	ls := laneSet{preset: "", dark: []string{"", "#ff00ff"}, light: []string{"#0000ff"}}
	dark := ls.lanes(true)
	if dark[0] != lanePresets[0].dark[0] {
		t.Errorf("没被覆盖的 0 号不该变：%v", dark[0])
	}
	if dark[1] != ui.Hex("#ff00ff") {
		t.Errorf("1 号该是覆盖的品红，实际 %v", dark[1])
	}
	for i := 2; i < laneCount; i++ {
		if dark[i] != lanePresets[0].dark[i] {
			t.Errorf("第 %d 号没被覆盖，不该变：%v", i, dark[i])
		}
	}
	// 同一份 laneSet 的另一半走浅色方案，深色的覆盖不该串过去。
	light := ls.lanes(false)
	if light[0] != ui.Hex("#0000ff") || light[1] != lanePresets[0].light[1] {
		t.Errorf("浅色只覆盖了 0 号：%v", light)
	}
	if dark[0] == light[0] {
		t.Errorf("深浅两份被串成了一份：%v", dark[0])
	}
}

// 写错的覆盖条目跳过、多出来的条目忽略、不越界 —— 配置是能手改的文件，
// 一条坏值只能影响它自己那一号（ui.Hex 对非法输入是 panic，这层必须拦住）。
func TestLaneOverridesSkipBadValues(t *testing.T) {
	bad := []string{"", "nothex", "#12345", "zzz", "", "", "", "", "#ff0000"}
	ls := laneSet{dark: bad, light: bad}
	for _, dark := range []bool{false, true} {
		got := ls.lanes(dark)
		base := lanePresets[0].lanes(dark)
		if got != base {
			t.Errorf("dark=%v：坏值应该整份跳过，实际 %v，want %v", dark, got, base)
		}
	}
}

// 方案名不认识（旧配置、手写错、空串）时回落到出厂那一套。
func TestUnknownGraphPresetFallsBackToClassic(t *testing.T) {
	for _, id := range []string{"", "nope", "Classic", "vivid "} {
		if got := lanePresetByID(id).id; got != "classic" {
			t.Errorf("方案 %q 落到 %q，该落到 classic", id, got)
		}
	}
	if got := lanePresetByID("vivid").id; got != "vivid" {
		t.Errorf("vivid 该认得出来，实际 %q", got)
	}
}

// hexColor 是配置里的颜色值走进调色板之前的那道门。
func TestHexColorValidationAndRoundTrip(t *testing.T) {
	good := []string{"#ff00ff", "#FF00FF", "ff00ff", " #ff00ff ", "#abc", "#abcd", "#ff00ff80"}
	for _, s := range good {
		if _, ok := hexColor(s); !ok {
			t.Errorf("%q 该解析得出来", s)
		}
	}
	bad := []string{"", "#", "nothex", "#12345", "#1234567", "#gggggg", "red", "rgb(1,2,3)"}
	for _, s := range bad {
		if _, ok := hexColor(s); ok {
			t.Errorf("%q 是非法颜色，不该通过", s)
		}
	}
	for _, c := range []ui.Color{ui.Hex("#4c86e8"), ui.Hex("#000000"), ui.Hex("#ffffff"), ui.RGB(1, 2, 3)} {
		got, ok := hexColor(hexOf(c))
		if !ok || got != c {
			t.Errorf("hexOf → hexColor 往返丢了颜色：%v → %q → %v", c, hexOf(c), got)
		}
	}
}
