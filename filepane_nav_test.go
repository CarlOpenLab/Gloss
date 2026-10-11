package main

import (
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/egoist/mygo/ui"
)

// 方向键在「改动」文件列表里上下移选中，跳过中间的目录行。
func TestNavFileMovesSelection(t *testing.T) {
	a := richApp(t)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// richApp 造的几个文件分属两段树，中间夹着目录行（FileIdx = -1）。
	// 用和应用同一套 Flatten，不自己造一份行模型。
	rows := filetree.Flatten(a.tree, a.filesClosed, false)
	files := realFiles(rows)
	if len(files) < 2 {
		t.Fatalf("至少要有两个文件才能测导航，实际 %d 个", len(files))
	}

	// 没选中过时按 ↓：从第一个文件开始，不会是「按了没反应」。
	a.fileSel = -1
	a.navFile(1)
	if a.fileSel < 0 {
		t.Fatal("没选中时按 ↓ 应选中第一个文件")
	}
	first := a.fileSel

	// 再按一下 ↓：落到另一个文件上（目录行要被跳过）。
	a.navFile(1)
	if a.fileSel == first {
		t.Errorf("第二次按 ↓ 还停在同一个文件 %d", first)
	}

	// 请 diff 跟着滚：pendingScrollFile 是点击走的那条通道，键盘也该用。
	if a.pendingScrollFile != a.fileSel {
		t.Errorf("导航后 diff 该滚到新文件：pendingScrollFile=%d fileSel=%d",
			a.pendingScrollFile, a.fileSel)
	}

	// 按回 ↑：回到刚才那个文件。
	a.navFile(-1)
	if a.fileSel != first {
		t.Errorf("按 ↑ 该回到上一个文件 %d，实际 %d", first, a.fileSel)
	}
}

// 连续按到边界不能跑出列表，也不能卡死。
func TestNavFileStopsAtEdges(t *testing.T) {
	a := richApp(t)
	settle(t, a)

	rows := filetree.Flatten(a.tree, a.filesClosed, false)
	files := realFiles(rows)
	if len(files) == 0 {
		t.Fatal("没有文件可测")
	}

	last := files[len(files)-1]
	a.fileSel = last
	for i := 0; i < len(files)+3; i++ {
		a.navFile(1)
	}
	if a.fileSel != last {
		t.Errorf("一路按 ↓ 应停在最后一个文件 %d，实际 %d", last, a.fileSel)
	}

	for i := 0; i < len(files)+3; i++ {
		a.navFile(-1)
	}
	if a.fileSel != files[0] {
		t.Errorf("一路按 ↑ 该停在第一个文件 %d，实际 %d", files[0], a.fileSel)
	}
}

// 空列表（没有改动）时按方向键不能崩。
func TestNavFileEmpty(t *testing.T) {
	a := richApp(t)
	a.files = nil
	a.tree = nil
	a.fileSel = 0
	a.navFile(1)
	a.navFile(-1)
}

// realFiles 从扁平行里挑出真正的文件行（跳过目录），按显示顺序给 FileIdx。
func realFiles(rows []filetree.Row) []int {
	var out []int
	for _, r := range rows {
		if r.Node != nil && r.Node.FileIdx >= 0 {
			out = append(out, r.Node.FileIdx)
		}
	}
	return out
}

// 真的按下箭头要能选中文件 —— 上面几条直接调 navFile，这条盯的是 keys.go
// 里那把键接没接上。
func TestArrowKeySelectsFile(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = true
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// 输入框没焦点、没有模态时按 ↓，文件栏该动起来。
	if !a.modalOn() {
		a.fileSel = -1
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		settle(t, a)
		if a.fileSel < 0 {
			t.Fatal("按 ↓ 后文件栏应该有选中的文件")
		}
	}
}
