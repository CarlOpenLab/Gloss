package main

import (
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
		base := paletteFor(themeFor(dark, ui.Preferences{}))
		got := paletteFor(themeFor(dark, prefs))
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
		pal := paletteFor(th)

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
		pal := paletteFor(themeFor(dark, ui.Preferences{}))
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
		pal := paletteFor(themeFor(dark, ui.Preferences{}))
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
