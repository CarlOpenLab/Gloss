package main

// 设置窗口该是它自己的窗口，不是主窗口里的一块浮层。
//
// 用户报的是（issue #7 问题四）：
//
//	「设置弹框或者其他弹框无法拖出主界面区域，按理设置界面是一个单独的窗口
//	  比较好，特别是我看修改字号的时候，已经生效了，但是被遮挡。」
//
// 两个症状是同一件事的两面：
//
//  1. 设置拖不出主窗口 —— bounds 是拿主窗口的尺寸算的（settings.go 的
//     settingsMargin），面板再拖也还在窗口里；
//  2. 改字号时「已经生效了，但是被遮挡」—— 面板盖在主窗口上，而主窗口是
//     设置唯一的可见区域，想看看改动效果就得先把它关掉。
//
// 单独一个窗口两个症状一起解：面板搬到自己的窗口里，窗口由系统拖动，想放到
// 哪放到哪；主窗口整个露出来，字号改动当场就能在终端里看见。
//
// 这里的测试分两层，因为这两层的失败方式不一样：
//
//   - 主视图这一层：设置不再画进 a.view。Tester 里直接看得到 —— 打开设置
//     之后主窗口的画面上不该再有设置的内容。
//   - 设置视图这一层：它自己是一棵能点的树。Tester 一次驱动一个 view 函数，
//     所以这一层用**第二个** Tester 驱动 a.settingsView。两个 Tester 共享
//     同一个 *app，所以在设置视图里点的「保存」照样落到主窗口的状态上。
//
// 系统那条 NewWindow 的路在测试里走不到：mygo.NewWindow 在应用没就绪时会
// panic，而无头环境里没有应用。所以建窗这件事抽成一个包级变量
// （settingsWindowNew），测试换上替身，只验「设置视图是不是自己的视图」
// 和「主视图里是不是干净了」，不去验系统的窗口管理器。

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/llm"
	"github.com/CarlOpenLab/Gloss/internal/store"
)

// stubSettingsWindow 是测试替身：记录调用，什么都不做。
//
// 它替的是 *mygo.Window —— 无头环境里没有窗口系统，而真窗口的三个方法
// 在没有 native 时也全是空转。
type stubSettingsWindow struct {
	closed      int
	invalidated int
}

func (w *stubSettingsWindow) Close()      { w.closed++ }
func (w *stubSettingsWindow) Invalidate() { w.invalidated++ }

// settingsTester 打开设置，按测试替身的方式驱动设置视图，返回它的 Tester。
func settingsTester(t *testing.T, a *app, w, h int) *ui.Tester {
	t.Helper()
	installSettingsWindowStub(t)
	a.openSettings()
	if !a.settingsOn {
		t.Fatal("打开设置之后 settingsOn 应为 true")
	}
	return newSettingsViewTester(t, a, w, h)
}

// installSettingsWindowStub 换上建窗替身，并在测试结束时还原。
//
// 替身把设置窗口的视图留在 a.settingsView 上 —— 真实窗口会自己留着它，而
// 替身开不出窗口，只能由测试这边拿着驱动。settingsViewTester 用它。
func installSettingsWindowStub(t *testing.T) {
	t.Helper()
	old := settingsWindowNew
	t.Cleanup(func() { settingsWindowNew = old })
	settingsWindowNew = func(a *app, o settingsWindowOptions) settingsWindow {
		a.settingsView = o.View
		return &stubSettingsWindow{}
	}
}

// settingsViewTester 驱动已经开着的那个设置视图。
//
// 给「先从主窗口点开、再去面板里操作」的测试用：主窗口那一个 Tester 点
// 「模型设置…」，这一个点面板里的「保存」。
func settingsViewTester(t *testing.T, a *app) *ui.Tester {
	t.Helper()
	return newSettingsViewTester(t, a, settingsWindowW, settingsWindowH)
}

func newSettingsViewTester(t *testing.T, a *app, w, h int) *ui.Tester {
	t.Helper()
	if !a.settingsOn {
		t.Fatal("设置没开：settingsOn 是 false")
	}
	if a.settingsView == nil {
		t.Fatal("设置没有通过 settingsWindowNew 交出它的视图 —— " +
			"没有视图，系统那边就只是一个空窗口")
	}
	tt := ui.NewTester(a.settingsView, w, h)
	still(tt)
	tt.Frame()
	return tt
}

// TestSettingsLeavesTheMainView 打开设置之后，主窗口的画面上不该再有设置的
// 内容 —— 它搬去自己的窗口了。
//
// 这一条是问题四的直接断言：以前设置面板画在 a.view 里，所以主窗口的
// Texts() 里能翻出「保存」「取消」这些字。现在这些字只该出现在设置
// 视图那一边。
func TestSettingsLeavesTheMainView(t *testing.T) {
	a := richApp(t)
	main := ui.NewTester(a.view, 1280, 860)
	still(main)
	main.Frame()
	before := len(main.Texts())

	old := settingsWindowNew
	t.Cleanup(func() { settingsWindowNew = old })
	settingsWindowNew = func(a *app, o settingsWindowOptions) settingsWindow {
		return &stubSettingsWindow{}
	}
	a.openSettings()
	main.Frame()
	main.Frame()

	if !a.settingsOn {
		t.Fatal("打开设置之后 settingsOn 应为 true")
	}
	// 主窗口的文字一条都不该多：设置整块搬走了，不是「少了一半」。
	if after := len(main.Texts()); after != before {
		t.Errorf("设置打开后主窗口多了 %d 条文字：%q", after-before, main.Texts())
	}
	// 只挑设置窗口独有的那几句：主窗口自己的工具栏上有一颗「模型设置…」，
	// HasText 是子串匹配，拿「模型设置」去问会永远命中它自己。
	for _, s := range []string{"设置存储于本地配置文件，所有请求由本机直连服务端",
		"搜索供应商", "提示词设置", "关于 Gloss", "软件更新", "远程同步", "亚克力模糊"} {
		if main.HasText(s) {
			t.Errorf("主窗口里还有设置的内容 %q —— 设置该在自己的窗口里", s)
		}
	}
}

// TestSettingsWindowDrawsItsOwnPanel 设置窗口自己那棵树是完整的：导航栏、
// 页面、底栏的取消与保存都在。
//
// 它防的是「视图交出去了，但里头是空的」——那种情况下系统会开一个空白窗口，
// 比浮层更糟。
func TestSettingsWindowDrawsItsOwnPanel(t *testing.T) {
	a := richApp(t)
	tt := settingsTester(t, a, 900, 700)

	for _, s := range []string{"模型设置", "通用偏好", "保存", "取消"} {
		if !tt.HasText(s) {
			t.Errorf("设置窗口里没有 %q；texts = %q", s, tt.Texts())
		}
	}
}

// TestSettingsWindowSavesFromItsOwnView 在设置窗口里点「保存」，改动落到
// 主窗口那一侧的状态上。
//
// 两个 Tester 共享同一个 *app，所以这条路验的是真行为：设置不再是主视图的
// 一个分支，但它改的还是同一份配置。
func TestSettingsWindowSavesFromItsOwnView(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{
		Providers: []llm.Provider{
			{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel},
		},
		Active: 0,
	})
	a.attachStore(s)

	tt := settingsTester(t, a, 900, 700)

	a.editCfg.Providers[0].Model = "claude-sonnet-4-5"
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("设置窗口里点保存: %v", err)
	}
	tt.Frame()

	if a.settingsOn {
		t.Error("保存之后设置窗口应该关上")
	}
	if got := a.store.Config().Providers[0].Model; got != "claude-sonnet-4-5" {
		t.Errorf("保存没落到配置里：model = %q", got)
	}
}

// TestSettingsWindowCancelKeepsTheOldConfig 在设置窗口里点「取消」，配置原样
// 不动。
//
// 另一半的安全绳：保存那一条过了、这一条没过的话，说明改动是无条件落盘的，
// 取消只是个摆设。
func TestSettingsWindowCancelKeepsTheOldConfig(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{
		Providers: []llm.Provider{
			{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel},
		},
		Active: 0,
	})
	a.attachStore(s)

	tt := settingsTester(t, a, 900, 700)

	a.editCfg.Providers[0].Model = "claude-sonnet-4-5"
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("设置窗口里点取消: %v", err)
	}
	tt.Frame()

	if a.settingsOn {
		t.Error("取消之后设置窗口应该关上")
	}
	if got := a.store.Config().Providers[0].Model; got != llm.DefaultModel {
		t.Errorf("取消之后配置被改了：model = %q", got)
	}
}

// TestSettingsWindowClosesWhenTheSystemClosesIt 系统那边关窗（用户点了窗口
// 右上角的 ×，或者 ⌘W）时，设置的状态跟着关上，而且再来一次也不慌。
//
// 「保存 / 取消」和系统的 × 是两条不同的路，都汇到同一个收尾上。没有这个
// 汇合点的话，一次用户操作会关两次窗 —— macOS 上表现为窗口没了但应用还占着
// 菜单，Windows 上表现为托盘里多一个幽灵。
func TestSettingsWindowClosesWhenTheSystemClosesIt(t *testing.T) {
	a := richApp(t)

	old := settingsWindowNew
	t.Cleanup(func() { settingsWindowNew = old })
	var onClosed func()
	settingsWindowNew = func(a *app, o settingsWindowOptions) settingsWindow {
		onClosed = o.OnClosed
		return &stubSettingsWindow{}
	}
	a.openSettings()
	if !a.settingsOn {
		t.Fatal("打开设置之后 settingsOn 应为 true")
	}
	if onClosed == nil {
		t.Fatal("建窗时没有交出 OnClosed —— 系统关窗时没人收尾")
	}

	onClosed()
	if a.settingsOn {
		t.Error("系统关窗之后 settingsOn 应为 false")
	}
	onClosed() // 再来一次：不该慌
	if a.settingsOn {
		t.Error("重复关窗之后 settingsOn 应为 false")
	}
}

// TestSettingsWindowSurvivesNoWindow 没有真窗口（无头测试，或者系统拒绝开窗）
// 时，设置照样能打开和关闭。
//
// 这一条守的是 nil 窗口的每一条调用路径：替身可以返回 nil，而 nil 窗口上的
// Close / Invalidate / Update 必须都是空操作。
func TestSettingsWindowSurvivesNoWindow(t *testing.T) {
	a := richApp(t)

	old := settingsWindowNew
	t.Cleanup(func() { settingsWindowNew = old })
	settingsWindowNew = func(a *app, o settingsWindowOptions) settingsWindow { return nil }

	a.openSettings()
	if !a.settingsOn {
		t.Fatal("拿不到窗口也该让设置开着")
	}
	a.closeSettings()
	if a.settingsOn {
		t.Error("拿不到窗口也该能关上设置")
	}
}
