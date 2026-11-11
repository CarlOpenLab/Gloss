package main

// 窗口按钮和设置窗口的两条规矩：
//
//   - Windows / Linux 的窗口按钮在右上角，macOS 的红绿灯才在左上；
//   - 设置是它自己的窗口，主窗口上一处都不画（issue #7 问题四）。

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestWindowButtonsSitAtTheTopRight 读的是布局：Windows / Linux 上最小化、
// 最大化、关闭三颗按钮在窗口右上角，不在左上。
func TestWindowButtonsSitAtTheTopRight(t *testing.T) {
	previous := windowControlsAtRight
	windowControlsAtRight = true
	defer func() { windowControlsAtRight = previous }()

	a := richApp(t)
	const width, height = 1280, 860
	tt := ui.NewTester(a.view, width, height)
	still(tt)
	tt.Frame()

	for _, label := range []string{T("关闭窗口", "Close Window"), T("最小化窗口", "Minimize Window")} {
		rect, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%q 不在屏幕上；texts = %v", label, tt.Texts())
		}
		if rect.X <= width/2 {
			t.Errorf("%q 在 x=%v，没在窗口右上角", label, rect.X)
		}
		if center := rect.Y + rect.H/2; center < shellBarMid-2 || center > shellBarMid+2 {
			t.Errorf("%q 的中线在 y=%v，不在顶栏的 %v 上", label, center, float32(shellBarMid))
		}
	}
}

// TestSettingsLeavesThePageInSight 从像素上读：设置打开时，主窗口的画面上
// 一点都没变 —— 它搬去自己的窗口了（issue #7 问题四）。
//
// 原来这一条守的是「设置不压暗整页」：面板坐在主窗口上，底色必须透明，
// 否则桌面和正在预览的终端都被盖掉。现在面板不在主窗口里了，这条更要紧
// 的是它**整个不见了** —— 一个字都不该多。
func TestSettingsLeavesThePageInSight(t *testing.T) {
	a := richApp(t)
	const width, height = 1280, 860

	tt := ui.NewTester(a.view, width, height)
	still(tt)
	tt.Frame()
	r1, g1, b1, _ := tt.Image().At(60, 800).RGBA()
	before := len(tt.Texts())

	a.openSettings()
	a.settingsTab = settingsTabGeneral
	tt.Frame()
	tt.Frame()
	r2, g2, b2, _ := tt.Image().At(60, 800).RGBA()

	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("设置打开后动了主窗口的画面：关闭 (%d,%d,%d)，打开 (%d,%d,%d)",
			r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
	}
	if after := len(tt.Texts()); after != before {
		t.Errorf("设置打开后主窗口多了 %d 条文字：%q", after-before, tt.Texts())
	}
}

// TestSettingsWindowIsAChildOfTheMainWindow 设置窗口是主窗口的子窗口。
//
// 用户报的是「设置弹框无法拖出主界面区域」（issue #7 问题四）。以前面板坐在
// 主窗口的视图里，bounds 是拿主窗口尺寸算的，于是再拖也拖不出去；改字号时
// 想看看效果，得先把设置关掉 —— 面板底下就是它唯一的可见区域。
//
// 现在设置是自己的窗口，由系统拖动。但**子窗口**这一条不能丢：不设 Parent
// 的话，点回主窗口时设置窗口会被压到底下去，而用户以为它还在那儿 —— 于是
// 「设置不见了」。
func TestSettingsWindowIsAChildOfTheMainWindow(t *testing.T) {
	a := richApp(t)

	old := settingsWindowNew
	t.Cleanup(func() { settingsWindowNew = old })
	var got settingsWindowOptions
	settingsWindowNew = func(a *app, o settingsWindowOptions) settingsWindow {
		got = o
		return &stubSettingsWindow{}
	}
	a.openSettings()

	if got.View == nil {
		t.Fatal("建设置窗口时没给视图 —— 系统那边只会是一个空窗口")
	}
	if got.OnClosed == nil {
		t.Fatal("建设置窗口时没给 OnClosed —— 用户点窗口的 × 时没人收尾")
	}
	// 交出去的必须是主窗口那一个，不是另开的一扇、也不是 nil。
	//
	// 无头测试里 a.win 是 nil（它只在 main.go 建窗那一刻赋值），所以这里
	// 判不了「非空」；判的是「原样转发」—— 把 Parent 改成别的值、或者漏掉
	// 这一行，这条就红。生产里 a.win 就是主窗口，转发它就是设成子窗口。
	if got.Parent != a.win {
		t.Fatalf("设置窗口的 Parent = %p, 该是主窗口 %p", got.Parent, a.win)
	}
}
