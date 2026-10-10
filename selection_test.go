package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Diff 文本改为浏览器式的原生选择：代码行是可选文本，用户自己拖动选中、
// 自己复制；不再有 Notion 风格的悬浮工具条。
func TestDiffHasNoSelectionOverlay(t *testing.T) {
	srv, _ := fakeLLM(t, "选区解释结果")
	a := richApp(t)
	withProvider(t, a, srv)
	a.rebuildRows()
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	for _, s := range []string{
		"复制所选代码", "Copy Selected Code",
		"AI 解析所选代码", "Explain selected code with AI",
		"取消选区", "Clear selection",
	} {
		if tt.HasText(s) {
			t.Errorf("diff 不该再出现选区浮层文案 %q：%q", s, tt.Texts())
		}
	}
}

// unified 单栏模式下同样可以拖选一段并复制。
func TestDiffDragSelectsLineRangeUnified(t *testing.T) {
	a := richApp(t)
	a.split = false
	a.rebuildRows()
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	const probe = "line 1: some code here"
	r, ok := tt.Find(probe)
	if !ok {
		t.Fatalf("找不到代码行 %q：%q", probe, tt.Texts())
	}
	tt.Press(r.X+2, r.Y+3)
	tt.Move(r.X+2, r.Y+43)
	tt.Release(r.X+2, r.Y+43)
	tt.Frame()

	if !a.diffSel.active || a.diffSel.startLine != 1 || a.diffSel.endLine != 3 {
		t.Fatalf("选区间应为 1..3，实际 active=%v %d..%d", a.diffSel.active, a.diffSel.startLine, a.diffSel.endLine)
	}
	tt.Key(ui.Cmd, ui.KeyC)
	tt.Frame()
	tt.Frame()
	if tt.Clipboard() == "" {
		t.Fatal("⌘C 后剪贴板应有内容")
	}
}

// diff 代码行支持拖选一段：按住左键从一行拖到另一行，整段高亮，⌘C 复制。
func TestDiffDragSelectsLineRangeAndCopies(t *testing.T) {
	a := richApp(t)
	a.rebuildRows()
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	const probe = "line 1: some code here"
	r, ok := tt.Find(probe)
	if !ok {
		t.Fatalf("找不到代码行 %q：%q", probe, tt.Texts())
	}

	// 从第 1 行按住往下拖两行，选中 1..3 行。
	tt.Press(r.X+2, r.Y+3)
	tt.Move(r.X+2, r.Y+43)
	tt.Release(r.X+2, r.Y+43)
	tt.Frame()

	if !a.diffSel.active {
		t.Fatal("拖选后应该有行区间")
	}
	if a.diffSel.startLine != 1 || a.diffSel.endLine != 3 {
		t.Fatalf("选区间应为 1..3，实际 %d..%d", a.diffSel.startLine, a.diffSel.endLine)
	}

	tt.Key(ui.Cmd, ui.KeyC)
	tt.Frame()
	tt.Frame()
	got := tt.Clipboard()
	if got == "" {
		t.Fatal("⌘C 后剪贴板应有内容")
	}
	if got != a.diffSel.code || !strings.Contains(got, "\n") {
		t.Fatalf("剪贴板应为多行选区文本，实际 %q", got)
	}

	// 单击一行（没有拖）清掉选区。
	tt.ClickAt(r.X+2, r.Y+3)
	tt.Frame()
	if a.diffSel.active {
		t.Error("单击应该清掉选区")
	}
}
