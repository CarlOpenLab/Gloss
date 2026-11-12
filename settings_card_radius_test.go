package main

import (
	"testing"
)

// 设置窗口里那张内置卡的圆角必须和主窗口的工作卡走同一条算式。
//
// 用户报的是：「设置里面的内置 card 貌似圆角 macos 和 Windows 都没对齐
// 参考 主窗口的 card rounded 是否能优化一下呢？」
//
// 原先 settingsCardRadius 写死 14，而主窗口工作卡走 shellCardRadius()：
// macOS 26 是 20（窗口角 28 − 间距 8），Windows 11 是 8（窗口角也是 8，
// 装不下 radiusLG，守住下限但不能越过外圆）。同一个产品里，主卡和设置卡
// 两套弧度 —— 而设置窗口自己也是一个窗口，它的外圈同样由系统画，卡片贴着
// 它内缩 settingsCardGap，所以这条算式对它一样成立。
//
// 写死 14 在两个平台上都不对齐，且错得不一样：
//
//   - macOS：主卡 20、设置卡 14，角上的 8px 留白带被两种曲率切开，读起来像
//     两张不相干的卡片叠在一起。
//   - Windows：窗口角只有 8，设置卡的 14 **比外圈还圆** —— 角上多出来的曲率
//     留下一块楔形灰带，正是主工作卡在 2026-10-30 实机反馈里见过的那样子。
//
// 所以设置卡的圆角不该自己有一个数，它该问 shellCardRadiusFor：跟着平台走，
// 和主卡永远同一套弧。
func TestSettingsCardRadiusFollowsTheShell(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			if got := settingsCardRadiusFor(goos); got != shellCardRadiusFor(goos) {
				t.Errorf("设置卡圆角 = %v，主工作卡是 %v —— 两处该是同一条算式",
					got, shellCardRadiusFor(goos))
			}
		})
	}
}

// 内圆不能比窗口外圈更圆：多出来的曲率会在角上留一块楔形灰带。
// 写死 14 的时候，Windows 11 的窗口角只有 8，设置卡比它还圆 6px。
func TestSettingsCardRadiusNeverExceedsTheWindow(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			win := shellWindowRadiusFor(goos)
			if got := settingsCardRadiusFor(goos); got > win {
				t.Errorf("%s: 设置卡圆角 %v 比窗口圆角 %v 还大 —— 角上会有楔形灰带",
					goos, got, win)
			}
		})
	}
}

// 卡片不能没有圆角，也不能小到看不出：Windows 11 上同心算出 0，守住
// radiusLG 之后取的是 min(radiusLG, 窗口角) = 8 —— 和主工作卡同一个取舍。
func TestSettingsCardRadiusStaysVisible(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			got := settingsCardRadiusFor(goos)
			if got <= 0 {
				t.Errorf("%s: 设置卡圆角 %v —— 卡片不能没有圆角", goos, got)
			}
			if want := min(radiusLG, shellWindowRadiusFor(goos)); got < want {
				t.Errorf("%s: 设置卡圆角 %v 低于下限 %v", goos, got, want)
			}
		})
	}
}
