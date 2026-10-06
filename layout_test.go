package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 窗口用的是隐藏标题栏，红绿灯直接浮在内容上。侧栏顶到窗口最上边，
// 顶上那条带子得空出来，否则关闭按钮正好压在筛选框上。
//
// 无头渲染默认没有窗口控件（TitleBar 是零值），所以这里显式摆上真实
// 尺寸 —— 在本机量出来的那组数，见 screenshot_test.go 的 realTitleBar。
func TestSidebarClearsTitleBar(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	box, ok := tt.Find("筛选文件")
	if !ok {
		t.Fatalf("找不到筛选框：%q", tt.Texts())
	}
	if box.Y < realTitleBar.Height {
		t.Errorf("筛选框顶边 y=%.0f，红绿灯占到 y=%.0f —— 被压住了",
			box.Y, realTitleBar.Height)
	}
}

// 侧栏收起来的时候，工具栏自己就贴着窗口左边缘了，这时轮到它让位。
func TestToolbarClearsTitleBarWithoutSidebar(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	name, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("找不到仓库名：%q", tt.Texts())
	}
	if name.X < realTitleBar.Left {
		t.Errorf("工具栏文字左边 x=%.0f，红绿灯占到 x=%.0f —— 被压住了",
			name.X, realTitleBar.Left)
	}

	// 反过来：侧栏开着的时候红绿灯被侧栏顶走，工具栏不该再白白缩进去 ——
	// 文字左边贴着侧栏右边缘就行，不该再加上 bar.Left。
	a.sidebarOn = true
	tt.Frame()
	tt.Frame()
	name2, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("侧栏开着时找不到仓库名：%q", tt.Texts())
	}
	if want := float32(sidebarWidth) + 12; name2.X > want+2 {
		t.Errorf("侧栏开着时工具栏文字左边 x=%.0f，该贴着侧栏右边缘的 %.0f",
			name2.X, want)
	}
}

// 卡片和对话框上的 × 要真的点得到。它从 ui.Button(c, "×") 换成了自己画的
// 方形叉号（按钮上不再有文字），这条测试盯着它别变成点不中的装饰。
func TestCloseButtonIsClickable(t *testing.T) {
	a := richApp(t)
	fi, hi := explainFixtures(t, a)
	a.ex = explainState{
		fi:   fi,
		hi:   hi,
		key:  a.explainKey(a.files[fi], hi),
		head: a.files[fi].Hunks[hi].Header,
		text: "这段改动做了什么。",
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if !a.explainOpen() {
		t.Fatal("卡片应该是开着的")
	}

	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("点不到关闭按钮：%v", err)
	}
	tt.Frame()
	if a.explainOpen() {
		t.Error("点了 × 之后卡片该关掉")
	}
}
