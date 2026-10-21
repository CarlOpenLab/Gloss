package main

// 窗口按钮和设置浮层的两条规矩：
//
//   - Windows / Linux 的窗口按钮在右上角，macOS 的红绿灯才在左上；
//   - 设置是浮在页面上的窗口，不压暗整页，按住标题就能拖开。

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

// TestSettingsLeavesThePageInSight 从像素上读：设置打开时，对话框外面那一点
// 和没开时一模一样。原来那层压暗会把桌面（以及正在预览的终端）盖掉。
func TestSettingsLeavesThePageInSight(t *testing.T) {
	a := richApp(t)
	const width, height = 1280, 860
	// 窗口左下角，离居中的对话框（980 宽，左边从 150 开始）很远。
	const probeX, probeY = 60, 800

	tt := ui.NewTester(a.view, width, height)
	still(tt)
	tt.Frame()
	r1, g1, b1, _ := tt.Image().At(probeX, probeY).RGBA()

	a.openSettings()
	a.settingsTab = settingsTabGeneral
	tt.Frame()
	tt.Frame()
	r2, g2, b2, _ := tt.Image().At(probeX, probeY).RGBA()

	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("设置打开后压暗了页面：关闭 (%d,%d,%d)，打开 (%d,%d,%d)",
			r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
	}
}

// TestTheSettingsDialogFollowsItsHeader 按住标题栏拖，对话框跟着走，窗口不动。
func TestTheSettingsDialogFollowsItsHeader(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	a.openSettings()
	a.settingsTab = settingsTabGeneral
	tt.Frame()
	tt.Frame()

	closeBtn, ok := tt.Find(T("关闭设置", "Close Settings"))
	if !ok {
		t.Fatalf("设置窗口的关闭按钮不在屏幕上；texts = %v", tt.Texts())
	}
	// 关闭按钮左边 30px 是卡片顶栏的空白：按住那里拖。
	x, y := closeBtn.X-30, closeBtn.Y+closeBtn.H/2
	tt.Press(x, y)
	tt.Frame()
	tt.Move(x+60, y+20)
	tt.Frame()
	tt.Release(x+60, y+20)
	tt.Frame()

	if !a.settingsOn {
		t.Fatal("拖动把设置关掉了")
	}
	if a.settingsDrag.X < 40 || a.settingsDrag.X > 80 || a.settingsDrag.Y < 10 || a.settingsDrag.Y > 30 {
		t.Fatalf("拖了 60,20 之后对话框在 %+v，应在 60,20 上下", a.settingsDrag)
	}
}
