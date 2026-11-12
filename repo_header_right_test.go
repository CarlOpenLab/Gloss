package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 侧栏收起时，「最近打开的项目」那个仓库名下拉该挪到工具栏最右端。
//
// 用户报的是：「collapse 的时候 最近最近打开的项目这个最好移动到最右边。」
//
// 原先它摆在 x≈331 —— 紧跟四格视图切换右边，夹在刷新 / 远程 / 对照那一串
// 动作中间。收起来的那一屏左边依次是 [命令面板][设置][显示侧栏]
// [改动|历史|终端|编辑器] 001 [刷新][抓取…][Unified|Split]，项目名夹在
// 两组互不相干的控件当中，读起来像动作链的一环。而它是这一排里唯一指向
// 「我现在在哪个仓库、怎么换一个」的控件，语义上属于窗口那一头。
//
// 挪到最右端之后：左边是一串纯动作，右边一端是身份 + 窗口控件，读法分开。
func TestFoldedRepoHeaderMovesToTheFarRight(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false

	tt := ui.NewTester(a.view, 1280, 860)
	// 侧栏宽度是 260ms 的动画，这里量的是摆定之后的位置。无头渲染每帧
	// 耗时不稳定，画两帧等不到终点 —— 关掉动效。
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	repo, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("侧栏收起时工具栏上找不到仓库名 %q：%q", a.repoName, tt.Texts())
	}

	// 左半屏最右的常驻控件是视图显示方式那一组（Unified / Split）。仓库名该
	// 排在它右边 —— Split 现在最靠右，仓库名还在它左边就是没挪。
	split, ok := tt.Find("Split")
	if !ok {
		t.Fatalf("工具栏上找不到显示方式切换：%q", tt.Texts())
	}
	if repo.X <= split.X {
		t.Errorf("仓库名 %q 左边 x=%.0f 该在显示方式切换（Split x=%.0f）右边 —— "+
			"现在夹在动作组中间", a.repoName, repo.X, split.X)
	}

	// 并且靠右到窗口那一头去，而不是停在窗口中间。
	if right := repo.X + repo.W; right < float32(1280)*0.6 {
		t.Errorf("仓库名右端只到 x=%.0f，1280 宽里连六成都没占到 —— 还没挪到最右边", right)
	}
}

// 挪到最右端之后，仓库名不能压住右上角那三颗窗口按钮（Windows / Linux）。
//
// 右边这一头现在有两个住户：自绘的窗口按钮（windowButtons，浮在内容之上的
// overlay）和仓库名。工具栏右端给窗口按钮留的位是 windowControlsReservedRight
// —— 仓库名排在它左边，两颗才不至于叠在一处。
func TestFoldedRepoHeaderClearsTheWindowButtons(t *testing.T) {
	previous := windowControlsAtRight
	windowControlsAtRight = true
	defer func() { windowControlsAtRight = previous }()

	a := richApp(t)
	a.sidebarOn = false
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.Frame()
	tt.Frame()

	repo, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("侧栏收起时工具栏上找不到仓库名 %q：%q", a.repoName, tt.Texts())
	}
	for _, label := range []string{T("关闭窗口", "Close Window"), T("最小化窗口", "Minimize Window")} {
		btn, ok := tt.Find(label)
		if !ok {
			t.Fatalf("找不到窗口按钮 %q：%q", label, tt.Texts())
		}
		if repo.X+repo.W > btn.X {
			t.Errorf("仓库名右端 x=%.0f 压住窗口按钮 %q（左边 x=%.0f）了 —— "+
				"工具栏该按 windowControlsReservedRight 让位",
				repo.X+repo.W, label, btn.X)
		}
	}
}

// 侧栏开着时项目名归侧栏顶栏的标签，工具栏右端不该再摆一个仓库名。
func TestExpandedSidebarKeepsRepoNameOutOfTheToolbar(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.Frame()

	// 侧栏开着时工具栏上出现的那个名字该在侧栏那一列里（x < 侧栏宽），
	// 而不是在工具栏右端。
	repo, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("找不到项目名 %q：%q", a.repoName, tt.Texts())
	}
	if repo.X > float32(sidebarWidth) {
		t.Errorf("侧栏开着时项目名 %q 出现在 x=%.0f —— 它该在侧栏那一列里",
			a.repoName, repo.X)
	}
}
