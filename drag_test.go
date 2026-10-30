package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarDragResize(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Frame()

	initW := a.sidebarWidth(nil)
	if initW != sidebarDefaultW {
		t.Fatalf("侧栏初始宽度应该是 %.0f，实际 %.0f", sidebarDefaultW, initW)
	}

	// 抓手的中心在主卡片那条白色边线上（侧栏宽度 + 卡片留白），不是浮在
	// 侧栏与卡片之间的粉色留白里 —— 眼睛看见的边界就是能拖的那条线。
	sepX := a.sidebarWidth(nil) + shellCardGap
	tt.Move(sepX, 300)
	tt.Frame()
	if c := tt.Cursor(); c != ui.CursorResizeEW {
		t.Errorf("悬停在卡片左沿光标应该是 CursorResizeEW，实际是 %v", c)
	}
	// 侧栏自己那条边（卡片边线往左 8px）已经不是抓手的中心了。
	tt.Move(a.sidebarWidth(nil)-2, 300)
	tt.Frame()
	if c := tt.Cursor(); c == ui.CursorResizeEW {
		t.Errorf("侧栏自己的边缘不该再是抓手中心（它现在压在卡片边线上）")
	}

	// 从卡片边线向右拉 50px
	tt.Press(sepX, 300)
	tt.Move(sepX+50, 300)
	tt.Frame()
	tt.Release(sepX+50, 300)
	tt.Frame()

	newW := a.sidebarWidth(nil)
	if newW <= initW {
		t.Errorf("拖动后侧栏宽度应变大：before=%.0f, after=%.0f", initW, newW)
	}

	// 验证已持久化到 store
	if cfg := a.store.Config(); cfg.SidebarWidth != newW {
		t.Errorf("侧栏宽度应持久化到 store：want %.0f, got %.0f", newW, cfg.SidebarWidth)
	}

	// 拖拽手柄向左拉到超出下限，应被 clamp 保护在 sidebarMinW
	curX := a.sidebarWidth(nil) + shellCardGap
	tt.Press(curX, 300)
	tt.Move(curX-300, 300)
	tt.Frame()
	tt.Release(curX-300, 300)
	tt.Frame()
	if minClamped := a.sidebarWidth(nil); minClamped != sidebarMinW {
		t.Errorf("侧栏宽度向左拖动应受 clamp 保护在 %.0f，实际 %.0f", sidebarMinW, minClamped)
	}

	// 双击复位
	a.resetSidebarWidth()
	tt.Frame()
	if got := a.sidebarWidth(nil); got != sidebarDefaultW {
		t.Errorf("复位后侧栏宽度应为 %.0f，实际 %.0f", sidebarDefaultW, got)
	}
}

func TestFilePaneDragResize(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	initW := a.filePaneWidth(nil)
	if initW <= 0 {
		t.Fatalf("文件列表栏初始宽度不应为空")
	}

	// 主卡与侧栏之间留了 shellCardGap，文件栏从卡片内沿开始。
	sepX := a.sidebarWidth(nil) + shellCardGap + initW

	// 悬停在文件列表分隔条上，光标应变为东西双向箭头
	tt.Move(sepX, 500)
	tt.Frame()
	if c := tt.Cursor(); c != ui.CursorResizeEW {
		t.Errorf("悬停在文件列表边缘光标应该是 CursorResizeEW，实际是 %v", c)
	}

	// 拖拽手柄向右拉 40px
	tt.Press(sepX, 500)
	tt.Move(sepX+40, 500)
	tt.Frame()
	tt.Release(sepX+40, 500)
	tt.Frame()

	newW := a.filePaneWidth(nil)
	if newW <= initW {
		t.Errorf("拖动后文件栏宽度应变大：before=%.0f, after=%.0f", initW, newW)
	}

	// 验证已持久化到 store
	if cfg := a.store.Config(); cfg.FilePaneWidth != newW {
		t.Errorf("文件列表栏宽度应持久化到 store：want %.0f, got %.0f", newW, cfg.FilePaneWidth)
	}

	// 拖拽向左拉到超出下限，应被 clamp 保护在 filePaneMinW
	curW := a.filePaneWidth(nil)
	curSep := a.sidebarWidth(nil) + shellCardGap + curW
	tt.Press(curSep, 500)
	tt.Move(curSep-300, 500)
	tt.Frame()
	tt.Release(curSep-300, 500)
	tt.Frame()
	if minClamped := a.filePaneWidth(nil); minClamped != filePaneMinW {
		t.Errorf("文件栏宽度向左拖动应受 clamp 保护在 %.0f，实际 %.0f", filePaneMinW, minClamped)
	}

	// 复位测试
	a.resetFilePaneWidth()
	tt.Frame()
	if got := a.filePaneWidth(nil); got != a.filePaneDefaultW() {
		t.Errorf("复位后文件栏宽度应为默认值 %.0f，实际 %.0f", a.filePaneDefaultW(), got)
	}
}

// 两条栏分隔线悬停 / 拖动时只提亮中性发丝线，不铺强调蓝；侧栏那条干脆
// 不画线（见 sidebarResizer）。
//
// 蓝线是「可点击的主 CTA」那一档的强调（DESIGN §5）；调栏宽时它跟着指针
// 满屏走，一次普通的拖动就变成在卡片边上划一道彩色高亮。这条测试盯像素：
// 分隔线附近任何一个像素都不该偏蓝；文件栏那条线本身还得看得见（不能干脆
// 不画）—— 侧栏那条不画，卡片左沿已有自己的 1px 硬边，再描一条就是按下
// / 松手时闪一下的白杠。
func TestSplitterLineIsNeutral(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	check := func(name string, x float32, mustShow bool) {
		t.Helper()
		tt.Move(x, 420)
		tt.Frame()
		img := tt.Image()
		darkest := 255
		for dx := -3; dx <= 3; dx++ {
			r, g, b, _ := img.At(int(x)+dx, 420).RGBA()
			r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
			if b8 > r8+20 && b8 > g8+20 {
				t.Errorf("%s分隔线在 x=%d 处偏蓝：rgb(%d,%d,%d)", name, int(x)+dx, r8, g8, b8)
			}
			darkest = min(darkest, r8)
		}
		if mustShow && darkest >= 250 {
			t.Errorf("%s分隔线悬停时该可见（扫到的像素全是背景）", name)
		}
	}

	check("侧栏", a.sidebarWidth(nil)+shellCardGap, false)
	check("文件栏", a.sidebarWidth(nil)+shellCardGap+a.filePaneWidth(nil), true)
}

// 拖动时整条分隔线收掉：栏宽自己跟着指针在变，那条位移就是反馈，
// 再画一条跟着指针满屏跑的线只是噪音。
//
// 侧栏那条更进一步 —— 干脆不画：悬停、按下、拖动、松手（指针还压在抓手
// 上）四种状态的边界像素必须和静止时逐个一样。深色下另有
// TestSidebarEdgeNeverLightsUp 盯同一件事（用户报的那条白杠出在深色主题）。
func TestSplitterLineHiddenWhileDragging(t *testing.T) {
	a := richApp(t)
	// 这条量的是「线在不在」的绝对像素值，按实色底调的：把亚克力不透明度
	// 关回 100%，别让默认 30% 的半透明卡片把整块画布的读数都带偏。亚克力
	// 本身由 theme_test / settings_test 盯。
	a.bgOpacity = 1
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	near := func(v, want int) bool { d := v - want; return d >= -2 && d <= 2 }
	// lineAt 说这一带有没有一条中性线（r≈g≈b，且明显暗于白底）。
	lineAt := func(x float32, y int) bool {
		img := tt.Image()
		for dx := -2; dx <= 2; dx++ {
			r, g, b, _ := img.At(int(x)+dx, y).RGBA()
			r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
			if r8 < 245 && near(r8, g8) && near(g8, b8) {
				return true
			}
		}
		return false
	}
	// darkest 是这一带最暗的中性像素。文件栏那条是两个同色栏之间的发丝线，
	// 只能问「有没有线」；侧栏那条边界上本来就有卡片自己的 1px 硬边（见
	// raisedSurface），而拖拽带不再往上描线，所以侧栏问的是「亮度没变」。
	darkest := func(x float32, y int) int {
		img := tt.Image()
		out := 255
		for dx := -2; dx <= 2; dx++ {
			r, g, b, _ := img.At(int(x)+dx, y).RGBA()
			r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
			if r8 < out && near(r8, g8) && near(g8, b8) {
				out = r8
			}
		}
		return out
	}

	// 侧栏：一个像素都不许多出来。
	sep := a.sidebarWidth(nil) + shellCardGap
	rest := darkest(sep, 420)
	same := func(stage string, at float32) {
		t.Helper()
		if got := darkest(at, 420); got != rest {
			t.Errorf("%s时侧栏边界多出了线：最暗 %d，静止时 %d", stage, got, rest)
		}
	}
	tt.Move(sep, 420)
	tt.Frame()
	same("悬停", sep)
	tt.Press(sep, 420)
	tt.Frame()
	same("按下", sep)
	tt.Move(sep+50, 420)
	tt.Frame()
	same("拖动", a.sidebarWidth(nil)+shellCardGap)
	tt.Release(sep+50, 420)
	tt.Frame()
	same("松手", a.sidebarWidth(nil)+shellCardGap)

	// 文件栏：同一套。
	paneSep := a.sidebarWidth(nil) + shellCardGap + a.filePaneWidth(nil)
	tt.Move(paneSep, 500)
	tt.Frame()
	if !lineAt(paneSep, 500) {
		t.Fatal("文件栏悬停时该有分隔线")
	}
	tt.Press(paneSep, 500)
	tt.Move(paneSep+40, 500)
	tt.Frame()
	if lineAt(a.sidebarWidth(nil)+shellCardGap+a.filePaneWidth(nil), 500) {
		t.Error("拖动文件栏时线该完全消失")
	}
	tt.Release(paneSep+40, 500)
	tt.Frame()
}

// 深色主题下侧栏边界那条「白杠」—— 用户报的就是它。
//
// 深色工作卡的左沿本来只有自己那条 1px 硬边（litSurfaceEdge，实测
// rgb(52,52,56)）。抓着侧栏拖拽带的时候，悬停 / 按下 / 松手会在边界上再描
// 一档 hairlineStrong（≈rgb(74,74,78)）：一道白线压在卡片边线上，按下去的
// 那一帧、松手后指针还压在带子上的那一段，一次普通拖动两头各闪一下。
// 现在拖拽带根本不画线 —— 边界附近最亮的像素在静止 / 悬停 / 按下 / 拖动 /
// 松手五种状态下必须一模一样。
func TestSidebarEdgeNeverLightsUp(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.SetDark(true)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 边界附近最亮的像素：卡片左沿那一列（连圆角上的抗锯齿），±4px 覆盖
	// 得了 8px 的留白和卡片内侧前几列 —— 这一带没有文字也没有控件，亮的
	// 只能是边线本身或者多描的那条线。
	edgeMax := func() int {
		img := tt.Image()
		edge := int(a.sidebarWidth(nil) + shellCardGap)
		out := 0
		for dx := -4; dx <= 4; dx++ {
			r, _, _, _ := img.At(edge+dx, 420).RGBA()
			if v := int(r >> 8); v > out {
				out = v
			}
		}
		return out
	}
	rest := edgeMax()
	if rest >= 200 {
		t.Fatalf("深色下侧栏边界不该有亮像素：最亮 %d", rest)
	}
	same := func(stage string) {
		t.Helper()
		if got := edgeMax(); got != rest {
			t.Errorf("%s时侧栏边界多出了一条线：最亮 %d，静止时 %d", stage, got, rest)
		}
	}

	sep := a.sidebarWidth(nil) + shellCardGap
	tt.Move(sep, 420)
	tt.Frame()
	same("悬停")
	tt.Press(sep, 420)
	tt.Frame()
	same("按下")
	tt.Move(sep+50, 420)
	tt.Frame()
	same("拖动")
	tt.Release(sep+50, 420)
	tt.Frame()
	same("松手")
}

// 项目与分支之间那条分割线可以拖：往下拖，项目区变高，分支区让出位置；
// 高度记进配置，重开还在；双击回到「跟着内容」。
func TestProjectsDividerDragAndPersist(t *testing.T) {
	a := newApp()
	a.openInNewTab(filepath.Join(t.TempDir(), "repo1"))
	a.openInNewTab(filepath.Join(t.TempDir(), "repo2"))
	a.switchTab(0)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	grip, ok := tt.Find(T("调整项目与分支的高度", "Resize projects and branches"))
	if !ok {
		t.Fatalf("找不到项目/分支分割线：%q", tt.Texts())
	}
	y := grip.Y + grip.H/2
	tt.Move(100, y)
	tt.Frame()
	if c := tt.Cursor(); c != ui.CursorResizeNS {
		t.Fatalf("悬停在项目/分支分割线上光标应为 CursorResizeNS，实际 %v", c)
	}

	before := a.projectsHeightVal
	if before == 0 {
		before = a.projectsContentHeight()
	}
	tt.Press(100, y)
	// 分几步挪：抓手自己会跟着高度一起走，只有把相对坐标还原成指针的
	// 绝对位置，后几步的增量才不会变成 0（这是之前那个 bug）。
	for i := 1; i <= 5; i++ {
		tt.Move(100, y+float32(9*i))
		tt.Frame()
	}
	tt.Release(100, y+45)
	tt.Frame()

	if a.projectsHeightVal <= before+20 {
		t.Fatalf("拖动之后项目区应该变高：before=%v after=%v", before, a.projectsHeightVal)
	}
	if got := a.store.Config().ProjectsHeight; got != a.projectsHeightVal {
		t.Errorf("高度没落盘：want %v, got %v", a.projectsHeightVal, got)
	}
	if got := a.store.Config().ProjectsHeight; got != a.projectsHeightVal {
		t.Errorf("高度没落盘：want %v, got %v", a.projectsHeightVal, got)
	}

	// 双击复位成「跟着内容」。抓手已经跟着高度挪下去了，重新量它的位置。
	grip, ok = tt.Find(T("调整项目与分支的高度", "Resize projects and branches"))
	if !ok {
		t.Fatalf("拖完之后找不到分割线：%q", tt.Texts())
	}
	gy := grip.Y + grip.H/2
	tt.ClickAt(100, gy)
	tt.Frame()
	tt.ClickAt(100, gy)
	tt.Frame()
	if a.projectsHeightVal != 0 {
		t.Errorf("双击之后项目区该回到跟内容，实际 %v", a.projectsHeightVal)
	}
}
