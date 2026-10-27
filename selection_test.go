package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 改动区里是原生可选文本：DiffView 是一个真的编辑器，拖动选中、⌘C 复制、
// 撤销都是它自己的行为 —— Gloss 不再画 Notion 风格的选区浮层和复制按钮。
func TestDiffHasNoSelectionOverlay(t *testing.T) {
	srv, _ := fakeLLM(t, "选区解释结果")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := settleReview(t, a)

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

// 代码行是原生可选择的文本：全选之后 ⌘C 能把内容复制出去。
//
// 代码是 DiffView 自己画的（不是 ui.Text 元素），所以这里按位置点进去、用
// 键盘全选，而不是去找某一段文字的元素。
func TestDiffCodeTextIsNativelySelectable(t *testing.T) {
	a := richApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "README.md")))
	tt := settleReview(t, a)

	// 点进正文（右侧那一栏的中段），焦点归 DiffView 里的编辑器。
	focusDiffBody(t, tt, a)

	tt.Key(ui.Cmd, ui.KeyA)
	tt.Frame()
	tt.Command("copy")
	tt.Frame()
	if got := tt.Clipboard(); !strings.Contains(got, "diff explainer") {
		t.Errorf("全选之后 ⌘C 该把内容复制出来，实际 %q", got)
	}
}
