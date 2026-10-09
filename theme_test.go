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
