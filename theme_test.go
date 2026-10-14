package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

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

// TestAccentIsAchromatic 断言强调色不带色相 —— 黑与白说话，彩度让给语义。
//
// 这是照着「界面整体还是紫色」这条反馈加的闸：强调色一旦又混进某个品牌彩，
// 主按钮、焦点环、导航指示条、当前分支圆点会同时跟着变，而那几处散在
// theme.go、palette.go、sidebar.go、history.go 四个文件里，评审时逐处盯着
// 色值看不出问题，只有把「不许有色相」写成断言才拦得住。
func TestAccentIsAchromatic(t *testing.T) {
	// 闸的自查：它得真的拦得住上一版那几个紫。一道对 #5e6ad2 放行的闸拦不住
	// 任何东西，而 isViolet 的判据不是显眼的「色相区间」，日后更容易被改松 ——
	// 把反例钉在这里，改判据的人会立刻看到自己放跑了什么。
	for _, old := range []string{"#5e6ad2", "#828fff", "#5e69d1", "#9d8df1", "#7e6cd8"} {
		if !isViolet(ui.Hex(old)) {
			t.Errorf("自查失败：%s 是上一版的紫，这道闸却放行了", old)
		}
	}

	for _, dark := range []bool{false, true} {
		th := themeFor(dark, ui.Preferences{})
		pal := paletteFor(th, laneSet{})

		for _, tc := range []struct {
			name string
			c    ui.Color
		}{
			{"Accent", th.Accent},
			{"AccentHover", th.AccentHover},
			{"AccentPressed", th.AccentPressed},
			{"AccentText", th.AccentText},
			{"Focus", th.Focus},
			{"Selection", th.Selection},
			{"pal.selection", pal.selection},
		} {
			if spread(tc.c) > 16 {
				t.Errorf("dark=%v: %s 带色相（RGB 极差 %d，彩度 %.2f）—— 强调色只有黑白两极",
					dark, tc.name, spread(tc.c), saturation(tc.c))
			}
		}

		// 深浅互为反相：深色下是白键黑字，浅色下是黑键白字。反了的话，
		// 主按钮在自己那套底色上要么糊成一块、要么刺得没法看。
		lum := luma(th.Accent)
		if dark && lum < 0.7 {
			t.Errorf("深色模式的强调色应当是白的，当前亮度 %.2f", lum)
		}
		if !dark && lum > 0.3 {
			t.Errorf("浅色模式的强调色应当是黑的，当前亮度 %.2f", lum)
		}
		if lumText := luma(th.AccentText); abs(lum-lumText) < 0.5 {
			t.Errorf("dark=%v: 强调键与键上的字亮度太近（%.2f / %.2f），读不出来", dark, lum, lumText)
		}
	}
}

// TestGraphLanesHaveNoPurple 断言拓扑图的支线里没有紫 / 靛。
//
// 八条线同时出现在一屏里，任何一条是紫，整张图就会被读成「紫色的那个软件」——
// 主干已经改走中性 Ink，剩下的七条也必须是明确的非紫。
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

// TestGraphMainLaneIsInk 断言 0 号色（主干 / HEAD）是中性 Ink，且是整张图上
// 最亮（浅色下最重）的一条。
//
// 主干同时是「我在哪条分支」那颗圆点、提交行里本地分支芯片上那颗 6px 点、
// 提交栏状态胶囊的颜色（见 history.go 的 badgeDot、sidebar.go 的 refRow）。
// 它一旦换成某个彩色 —— 上一版是 `#58a6ff` 经典亮蓝 —— 主线立刻降级成「又一
// 条支线」，跟着那几处圆点一起变色，整屏就多出一串蓝点。这条闸是按那次回退加的。
func TestGraphMainLaneIsInk(t *testing.T) {
	for _, dark := range []bool{false, true} {
		pal := paletteFor(themeFor(dark, ui.Preferences{}), laneSet{})
		main := pal.lanes[0]
		if spread(main) > 16 {
			t.Errorf("dark=%v: 主干色 %v 带色相（RGB 极差 %d）—— 主干走中性 Ink", dark, main, spread(main))
		}
		for i := 1; i < len(pal.lanes); i++ {
			lm, li := luma(main), luma(pal.lanes[i])
			if dark && lm <= li {
				t.Errorf("深色下主干 %.2f 不比 %d 号线 %.2f 亮 —— 主线要一眼认得出来", lm, i, li)
			}
			if !dark && lm >= li {
				t.Errorf("浅色下主干 %.2f 不比 %d 号线 %.2f 暗 —— 主线要一眼认得出来", lm, i, li)
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

// 内置的拓扑图方案都得守住 §2.6 那三条：0 号中性 Ink、8 支两两不同、
// 全图只有一支蓝。方案是给用户挑的，挑完照样得能读图 —— 这条就是那道闸。
func TestLanePresetsKeepDesignRules(t *testing.T) {
	for _, p := range lanePresets {
		for _, dark := range []bool{false, true} {
			cols := p.lanes(dark)
			if got := spread(cols[0]); got > 16 {
				t.Errorf("%s(dark=%v)：0 号是主干，得是中性 Ink，实际 %v（极差 %d）",
					p.id, dark, cols[0], got)
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
func TestClassicLanesMatchDesignSpec(t *testing.T) {
	want := map[bool][]string{
		true: {
			"#e6e6e6", "#3eb489", "#e09f3e", "#4c86e8",
			"#e5484d", "#d977a8", "#38b2ac", "#9aab45",
		},
		false: {
			"#0f1115", "#2d9d74", "#c9822b", "#2f6fd0",
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
