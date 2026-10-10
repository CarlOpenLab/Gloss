package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Diff 文本用原生可选文本：代码行可以自由拖动选中并 ⌘C 复制；
// 不再有 Notion 风格的选区浮层与复制按钮。
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

// 代码行是原生可选择的文本：拖动选中后 Cmd+C 能把选中的字复制出去。
func TestDiffCodeTextIsNativelySelectable(t *testing.T) {
	a := richApp(t)
	a.rebuildRows()
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	const probe = "line 1: some code here"
	r, ok := tt.Find(probe)
	if !ok {
		t.Fatalf("找不到代码行 %q：%q", probe, tt.Texts())
	}

	// 在行内从左往右拖一段，选中开头几个字符。
	tt.Press(r.X+2, r.Y+r.H/2)
	tt.Move(r.X+40, r.Y+r.H/2)
	tt.Release(r.X+40, r.Y+r.H/2)
	tt.Frame()

	tt.Command("copy")
	tt.Frame()
	if got := tt.Clipboard(); got == "" {
		t.Fatal("原生选中代码后 Cmd+C 应该能复制")
	}
}
