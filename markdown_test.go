package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

var testMD = mdStyle{
	text:   ui.Hex("#111111"),
	muted:  ui.Hex("#666666"),
	code:   ui.Hex("#222222"),
	codeBg: ui.Hex("#eeeeee"),
	head:   ui.Hex("#000000"),
	bullet: ui.Hex("#0000ff"),
	mono:   "monospace",
}

// renderMD 把 spans 拼回文字，用于断言「内容没丢」。
func renderMD(spans []ui.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// spanFor 找第一个包含 sub 的片段。
func spanFor(spans []ui.Span, sub string) (ui.Span, bool) {
	for _, s := range spans {
		if strings.Contains(s.Text, sub) {
			return s, true
		}
	}
	return ui.Span{}, false
}

// 渲染不能吃掉内容：所有非语法字符都要原样出现在片段里。
func TestMarkdownKeepsAllText(t *testing.T) {
	src := "第一行普通文字\n\n- 要点一\n- 要点二\n\n**加粗** 和 `代码`\n"
	got := renderMD(mdSpans(src, testMD))

	for _, want := range []string{"第一行普通文字", "要点一", "要点二", "加粗", "代码"} {
		if !strings.Contains(got, want) {
			t.Errorf("渲染结果里少了 %q：\n%q", want, got)
		}
	}
	if strings.Contains(got, "**") || strings.Contains(got, "`") {
		t.Errorf("语法记号应该被吃掉：\n%q", got)
	}
}

func TestMarkdownBold(t *testing.T) {
	spans := mdSpans("这是**重点**内容", testMD)
	s, ok := spanFor(spans, "重点")
	if !ok {
		t.Fatalf("找不到加粗片段：%+v", spans)
	}
	if s.Weight != 700 {
		t.Errorf("加粗片段的 Weight = %d, want 700", s.Weight)
	}
	if s.Text != "重点" {
		t.Errorf("加粗片段多了星号：%q", s.Text)
	}
}

func TestMarkdownInlineCode(t *testing.T) {
	spans := mdSpans("调用 `expandTabs()` 展开", testMD)
	s, ok := spanFor(spans, "expandTabs")
	if !ok {
		t.Fatalf("找不到行内代码片段：%+v", spans)
	}
	if s.Font != "monospace" {
		t.Errorf("行内代码应该用等宽字体，got %q", s.Font)
	}
	if s.Background == (ui.Color{}) {
		t.Error("行内代码应该有底色")
	}
}

// 代码块整段用等宽字体，而且不再解析里面的语法记号 ——
// 代码里的 * 和 ` 是代码本身，不是格式。
func TestMarkdownCodeFence(t *testing.T) {
	src := "看这段：\n```go\nx := a * b\nfmt.Println(\"`\")\n```\n结束"
	spans := mdSpans(src, testMD)

	if renderMD(spans) == "" || !strings.Contains(renderMD(spans), "x := a * b") {
		t.Fatalf("代码块内容丢了：%q", renderMD(spans))
	}
	s, ok := spanFor(spans, "x := a * b")
	if !ok {
		t.Fatal("找不到代码块那一行")
	}
	if s.Font != "monospace" || s.Background == (ui.Color{}) {
		t.Errorf("代码块应该等宽且有底色：%+v", s)
	}
	if strings.Contains(renderMD(spans), "```") {
		t.Errorf("围栏记号应该被吃掉：%q", renderMD(spans))
	}
	if s.Text != "x := a * b\n" {
		t.Errorf("代码块里的 * 和引号被当成格式了：%q", s.Text)
	}
}

func TestMarkdownList(t *testing.T) {
	for _, src := range []string{"- 第一点", "* 第一点", "+ 第一点", "1. 第一点", "2) 第一点"} {
		spans := mdSpans(src, testMD)
		got := renderMD(spans)
		if !strings.Contains(got, "第一点") {
			t.Errorf("%q 的内容丢了：%q", src, got)
		}
		if strings.HasPrefix(strings.TrimSpace(got), "-") || strings.HasPrefix(strings.TrimSpace(got), "*") {
			t.Errorf("%q 的记号没换掉：%q", src, got)
		}
	}
	// 有序列号保留原编号：正文里常说「第 2 点」，重排就对不上了
	if got := renderMD(mdSpans("3. 第三点", testMD)); !strings.Contains(got, "3.") {
		t.Errorf("有序列表应该保留原编号：%q", got)
	}
}

func TestMarkdownHeading(t *testing.T) {
	spans := mdSpans("## 变更目的", testMD)
	got := renderMD(spans)
	if !strings.Contains(got, "变更目的") {
		t.Errorf("标题文字丢了：%q", got)
	}
	if strings.Contains(got, "#") {
		t.Errorf("标题的 # 应该去掉：%q", got)
	}
	s, _ := spanFor(spans, "变更目的")
	if s.Weight != 700 {
		t.Errorf("标题应该是粗体：%+v", s)
	}
}

// 语法用错（星号不成对）时按字面显示，不能把后面的内容吃掉。
func TestMarkdownUnpairedMarkers(t *testing.T) {
	for _, src := range []string{"这是 * 号", "**没闭合", "`没闭合", "a ** b * c"} {
		got := renderMD(mdSpans(src, testMD))
		for _, part := range strings.Fields(strings.NewReplacer("*", " ", "`", " ").Replace(src)) {
			if !strings.Contains(got, part) {
				t.Errorf("%q 渲染后丢了 %q：%q", src, part, got)
			}
		}
	}
}

// 首尾的空行去掉，卡片不会顶着一个空白开头。
func TestMarkdownTrimsBlankEdges(t *testing.T) {
	spans := mdSpans("\n\n正文\n\n\n", testMD)
	if len(spans) == 0 {
		t.Fatal("内容全被当成空行了")
	}
	if strings.TrimSpace(spans[0].Text) == "" {
		t.Errorf("开头还留着空行：%q", renderMD(spans))
	}
	if last := spans[len(spans)-1]; strings.TrimSpace(last.Text) == "" {
		t.Errorf("结尾还留着空行：%q", renderMD(spans))
	}
	// 正文内部的空行要留着，段落才分得开
	if got := renderMD(mdSpans("甲\n\n乙", testMD)); !strings.Contains(got, "甲\n\n乙") {
		t.Errorf("段落之间的空行被吃掉了：%q", got)
	}
}

// 空输入不能崩，也不能吐出垃圾片段。
func TestMarkdownEmpty(t *testing.T) {
	if spans := mdSpans("", testMD); len(spans) != 0 {
		t.Errorf("空输入应该没有片段：%+v", spans)
	}
	if spans := mdSpans("\n\n", testMD); len(spans) != 0 {
		t.Errorf("只有空行时应该没有片段：%+v", spans)
	}
}
