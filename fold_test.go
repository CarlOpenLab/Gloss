package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// rowsFolded 报告某个文件在行模型里是不是只剩一条文件头。
// 行模型是懒重建的，所以先重建一次再问。
func rowsFolded(a *app, fi int) bool {
	a.rebuildRows()
	for _, r := range a.rows {
		if r.file == fi && r.kind != rowFileHeader {
			return false
		}
	}
	return true
}

// 折起来的文件在行模型里只剩一条文件头 —— 代码行、hunk 头全都不见了。
func TestBuildRowsCollapsedKeepsHeaderOnly(t *testing.T) {
	f := fileFromPatch(t, simplePatch)
	rows := buildRows([]gitrepo.File{f}, nil, nil, false, map[string]bool{"a.go": true})

	if got := shape(rows); got != "H" {
		t.Errorf("行结构 = %s, want H", got)
	}
}

// 折叠只影响被点名的那个文件，别的文件照常展开。
func TestBuildRowsCollapsedOnlyAffectsNamedFile(t *testing.T) {
	a := fileFromPatch(t, simplePatch)
	b := fileFromPatch(t, simplePatch)
	b.NewPath, b.OldPath = "b.go", "b.go"

	rows := buildRows([]gitrepo.File{a, b}, nil, nil, false, map[string]bool{"b.go": true})

	if got := shape(rows); got != "H@LLLH" {
		t.Errorf("行结构 = %s, want H@LLLH", got)
	}
	if rows[0].file != 0 || rows[1].file != 0 {
		t.Error("前四条应该是 a.go 的")
	}
	if rows[len(rows)-1].file != 1 {
		t.Error("最后一条应该是 b.go 的文件头")
	}
}

// 标了「已 review」的文件在 diff 里自动折起来，取消标记又展开回来。
func TestViewedFoldsInDiffView(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 找一个内容够多的文件来折
	fi := -1
	for i, f := range a.files {
		if len(f.Hunks) > 0 {
			fi = i
			break
		}
	}
	if fi < 0 {
		t.Fatal("仓库里没有带 hunk 的文件")
	}
	f := a.files[fi]

	a.toggleViewed(f)
	tt.Frame()
	if !a.isFolded(f) {
		t.Fatal("标了已 review 之后文件应该在 diff 里折起来")
	}
	if !tt.HasText("已折叠") {
		t.Errorf("折叠的文件头应该有提示：%q", tt.Texts())
	}
	// 勾选框本身也要跟着变
	if !tt.HasText("已 review") {
		t.Errorf("文件头应该有已 review 勾选框：%q", tt.Texts())
	}

	if !rowsFolded(a, fi) {
		t.Error("折起来之后这个文件在行模型里不该还有代码行")
	}

	a.toggleViewed(f)
	tt.Frame()
	if a.isFolded(f) {
		t.Error("取消已 review 之后应该展开回来")
	}
	if rowsFolded(a, fi) {
		t.Error("展开之后行模型里应该又有代码行了")
	}
}

// 点文件头手动折叠 / 展开。
func TestClickFileHeaderFolds(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 用带目录的路径：这一段只出现在 diff 的文件头上，侧栏里是拆开的
	const path = "internal/server/handler.go"
	fi := -1
	for i, f := range a.files {
		if displayPath(f) == path {
			fi = i
			break
		}
	}
	if fi < 0 {
		t.Fatalf("找不到 %s", path)
	}
	f := a.files[fi]

	if err := tt.Click(path); err != nil {
		t.Fatalf("点文件头：%v", err)
	}
	tt.Frame()
	if !a.isFolded(f) {
		t.Fatal("点文件头应该折起来")
	}

	if err := tt.Click(path); err != nil {
		t.Fatalf("再点文件头：%v", err)
	}
	tt.Frame()
	if a.isFolded(f) {
		t.Error("再点一次应该展开")
	}
}

// 点「已 review」勾选框不该连带把文件折起来 —— 两个控件各管各的。
func TestClickViewedCheckboxDoesNotFold(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 有多个文件头都带这个勾选框，点第一个（就是列表里第一个文件的）
	if err := tt.Click("已 review"); err != nil {
		t.Fatalf("点已 review：%v", err)
	}
	tt.Frame()

	if a.viewedCount() != 1 {
		t.Fatalf("应该正好标了一个文件，实际 %d", a.viewedCount())
	}
	// 勾上之后按规则是折起来的；这里要验的是「点勾选框只走 toggleViewed 一条路」，
	// 所以看 folded 里没有被写进手动覆盖 —— 一旦写了，取消标记就展不开了。
	for k := range a.folded {
		t.Errorf("点勾选框不该写手动折叠覆盖，却写了 %q", k)
	}
}

// toggleViewed 会清掉手动折叠的覆盖：折叠状态回到「跟着 review 走」。
func TestToggleViewedResetsManualFold(t *testing.T) {
	a := testApp(t)
	f := a.files[0]

	a.toggleFold(f)
	if !a.isFolded(f) {
		t.Fatal("点一下应该折起来")
	}

	a.toggleViewed(f)
	if !a.isFolded(f) {
		t.Error("勾上已 review 之后仍然应该是折着的")
	}

	// 再取消标记：手动覆盖已经清掉，所以跟着 review 走 —— 展开
	a.toggleViewed(f)
	if a.isFolded(f) {
		t.Error("取消已 review 之后应该展开")
	}
	if _, ok := a.folded[fileKey(f)]; ok {
		t.Error("取消标记之后不该还留着手动覆盖")
	}
}

// 折叠状态是纯视图状态，不该被当成「已 review」写进 store。
func TestFoldDoesNotTouchViewedStore(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.toggleFold(a.files[0])
	settle(t, a)

	if a.viewedCount() != 0 {
		t.Error("折叠不该算成已 review")
	}
	if len(a.store.State().Viewed) != 0 {
		t.Error("折叠不该写进 viewed 状态")
	}
}
