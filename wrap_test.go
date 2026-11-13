package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/store"
)

// 代码视图的自动换行是一份共享设置：主界面的编辑器 tab 和整文件代码编辑器
// 看的是同一个开关，默认关，切换后落盘。
func TestCodeWrapSharedByBothCodeEditors(t *testing.T) {
	dir := t.TempDir()
	a := richApp(t)
	a.attachStore(store.Open(dir))
	a.setViewMode(viewModeEditor)
	settle(t, a)

	if a.wrapCode {
		t.Fatal("代码自动换行默认该是关的")
	}

	tab := a.activeEditorTab()
	if tab == nil || tab.code == nil {
		t.Fatal("编辑器 tab 该已经装好")
	}
	a.openCodeEditor(listedFile(t, a, "internal/server/handler.go"))
	settle(t, a)
	s := a.codeEdit
	if s.code == nil {
		t.Fatalf("代码编辑器没打开：%q", s.err)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	if tab.code.SoftWrap || s.code.SoftWrap {
		t.Error("默认关的时候两处编辑器都不该软换行")
	}

	a.setCodeWrap(true)
	tt.Frame()
	if !a.wrapCode {
		t.Error("setCodeWrap(true) 之后 wrapCode 该是 true")
	}
	if !tab.code.SoftWrap {
		t.Error("编辑器 tab 没跟上代码换行开关")
	}
	if !s.code.SoftWrap {
		t.Error("整文件代码编辑器没跟上代码换行开关")
	}
	settle(t, a)
	if !store.Open(dir).Config().WrapCode {
		t.Error("代码换行没存进配置文件")
	}

	a.setCodeWrap(false)
	tt.Frame()
	if a.wrapCode || tab.code.SoftWrap || s.code.SoftWrap {
		t.Errorf("再关一次没生效：wrapCode=%v tab=%v modal=%v",
			a.wrapCode, tab.code.SoftWrap, s.code.SoftWrap)
	}
	// 落盘是后台任务；等它写完，TempDir 的清理才不会和它抢目录。
	settle(t, a)
}

// 整文件代码编辑器底栏的换行按钮：点一下开、再点一下关，两处代码视图跟着变。
func TestCodeEditorWrapButtonTogglesSharedSetting(t *testing.T) {
	a := richApp(t)
	a.attachStore(store.Open(t.TempDir()))
	a.setViewMode(viewModeEditor)
	settle(t, a)

	a.openCodeEditor(listedFile(t, a, "internal/server/handler.go"))
	settle(t, a)
	if a.codeEdit.code == nil {
		t.Fatalf("代码编辑器没打开：%q", a.codeEdit.err)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("点代码编辑器的换行按钮：%v", err)
	}
	tt.Frame()
	if !a.wrapCode || !a.codeEdit.code.SoftWrap {
		t.Errorf("点过之后该开启并即时生效：wrapCode=%v editor=%v",
			a.wrapCode, a.codeEdit.code.SoftWrap)
	}

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("再点一次换行按钮：%v", err)
	}
	tt.Frame()
	if a.wrapCode || a.codeEdit.code.SoftWrap {
		t.Errorf("再点一次该关回去：wrapCode=%v editor=%v",
			a.wrapCode, a.codeEdit.code.SoftWrap)
	}
	settle(t, a)
}

// 设置页「Diff 与代码视图」里的两个换行开关：和四处按钮同一份状态，
// 打开后落盘。
func TestWrapSettingSwitchesPersist(t *testing.T) {
	dir := t.TempDir()
	a := richApp(t)
	a.attachStore(store.Open(dir))
	settle(t, a)
	a.setLocale("zh")

	st := settingsTester(t, a, settingsWindowW, settingsWindowH)
	a.settingsTab = settingsTabGeneral
	st.Frame()

	scrollTo(t, st, T("代码自动换行", "Wrap Code"), settingsWindowH)
	if err := st.Click(T("代码自动换行开关", "Wrap Code switch")); err != nil {
		t.Fatalf("点「代码自动换行」开关：%v", err)
	}
	settle(t, a)
	st.Frame()
	if !a.wrapCode {
		t.Error("点过之后 wrapCode 该是 true")
	}

	scrollTo(t, st, T("Diff 自动换行", "Wrap Diff"), settingsWindowH)
	if err := st.Click(T("Diff 自动换行开关", "Wrap Diff switch")); err != nil {
		t.Fatalf("点「Diff 自动换行」开关：%v", err)
	}
	settle(t, a)
	st.Frame()
	if !a.wrapDiff {
		t.Error("点过之后 wrapDiff 该是 true")
	}

	cfg := store.Open(dir).Config()
	if !cfg.WrapCode || !cfg.WrapDiff {
		t.Errorf("两个开关都该落盘：WrapCode=%v WrapDiff=%v", cfg.WrapCode, cfg.WrapDiff)
	}
}

// 对比弹窗工具条上的换行按钮：和改动区共享同一个 diff 换行开关。
func TestCompareWrapButtonTogglesSharedSetting(t *testing.T) {
	a, _ := compareApp(t)
	a.attachStore(store.Open(t.TempDir()))
	a.startCompare("main", "feature")
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	// 工具条要等 diff 装好才画（加载中那一帧整个身子是 Spinner）。
	for range 6 {
		if a.compare.review.diff != nil {
			break
		}
		settle(t, a)
		tt.Frame()
	}
	if a.compare.review.diff == nil {
		t.Fatalf("对比弹窗的 diff 没装好：%q", a.compare.review.err)
	}

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("点对比弹窗的换行按钮：%v", err)
	}
	tt.Frame()
	if !a.wrapDiff {
		t.Error("点过之后 wrapDiff 该是 true")
	}
	if !a.compare.review.diff.Wrap {
		t.Error("对比弹窗的 diff 没跟上换行开关")
	}

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("再点一次换行按钮：%v", err)
	}
	tt.Frame()
	if a.wrapDiff || a.compare.review.diff.Wrap {
		t.Errorf("再点一次该关回去：wrapDiff=%v diff=%v", a.wrapDiff, a.compare.review.diff.Wrap)
	}
	settle(t, a)
}

// 改动区头栏的换行按钮：点一下 diff 长行折行，改动区与对比弹窗共享开关。
func TestReviewWrapButtonTogglesSharedSetting(t *testing.T) {
	a, _ := compareApp(t)
	a.attachStore(store.Open(t.TempDir()))
	settle(t, a)

	tt := settleReview(t, a)
	waitReview(t, a, tt)
	if a.review.diff == nil {
		t.Fatalf("改动区没装好：%q", a.review.err)
	}

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("点改动区的换行按钮：%v", err)
	}
	tt.Frame()
	if !a.wrapDiff || !a.review.diff.Wrap {
		t.Errorf("点过之后该开启并即时生效：wrapDiff=%v diff=%v", a.wrapDiff, a.review.diff.Wrap)
	}

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("再点一次换行按钮：%v", err)
	}
	tt.Frame()
	if a.wrapDiff || a.review.diff.Wrap {
		t.Errorf("再点一次该关回去：wrapDiff=%v diff=%v", a.wrapDiff, a.review.diff.Wrap)
	}
	settle(t, a)
}

// 主界面编辑器 tab 栏右端的换行按钮：点一下切换共享设置，当前 tab 立刻折行。
func TestEditorTabBarWrapButtonTogglesSharedSetting(t *testing.T) {
	a := richApp(t)
	a.attachStore(store.Open(t.TempDir()))
	a.setViewMode(viewModeEditor)
	settle(t, a)
	tab := openEditorFile(t, a, "README.md")

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("点编辑器 tab 栏的换行按钮：%v", err)
	}
	tt.Frame()
	if !a.wrapCode || !tab.code.SoftWrap {
		t.Errorf("点过之后该开启并即时生效：wrapCode=%v tab=%v", a.wrapCode, tab.code.SoftWrap)
	}

	if err := tt.Click(T("自动换行", "Soft Wrap")); err != nil {
		t.Fatalf("再点一次换行按钮：%v", err)
	}
	tt.Frame()
	if a.wrapCode || tab.code.SoftWrap {
		t.Errorf("再点一次该关回去：wrapCode=%v tab=%v", a.wrapCode, tab.code.SoftWrap)
	}
	settle(t, a)
}

// diff 视图的自动换行也是一份共享设置：改动区和对比弹窗共用，默认关，
// 切换后落盘。
func TestDiffWrapSharedByReviewAndCompare(t *testing.T) {
	dir := t.TempDir()
	a, _ := compareApp(t)
	a.attachStore(store.Open(dir))
	settle(t, a)

	if a.wrapDiff {
		t.Fatal("diff 自动换行默认该是关的")
	}

	tt := settleReview(t, a)
	waitReview(t, a, tt)
	if a.review.diff == nil {
		t.Fatalf("改动区没装好：%q", a.review.err)
	}
	if a.review.diff.Wrap {
		t.Error("默认关的时候改动区不该软换行")
	}

	a.setDiffWrap(true)
	tt.Frame()
	if !a.wrapDiff {
		t.Error("setDiffWrap(true) 之后 wrapDiff 该是 true")
	}
	if !a.review.diff.Wrap {
		t.Error("改动区没跟上 diff 换行开关")
	}

	a.startCompare("main", "feature")
	settle(t, a)
	tt.Frame()
	for range 6 {
		if a.compare.review.diff != nil {
			break
		}
		settle(t, a)
		tt.Frame()
	}
	if a.compare.review.diff == nil {
		t.Fatalf("对比弹窗的 diff 没装好：%q", a.compare.review.err)
	}
	if !a.compare.review.diff.Wrap {
		t.Error("对比弹窗没跟上 diff 换行开关")
	}
	settle(t, a)
	if !store.Open(dir).Config().WrapDiff {
		t.Error("diff 换行没存进配置文件")
	}

	a.setDiffWrap(false)
	tt.Frame()
	if a.wrapDiff || a.review.diff.Wrap || a.compare.review.diff.Wrap {
		t.Errorf("再关一次没生效：wrapDiff=%v review=%v compare=%v",
			a.wrapDiff, a.review.diff.Wrap, a.compare.review.diff.Wrap)
	}
	settle(t, a)
}
