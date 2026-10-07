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

// renderMD 把块拼回文字，用于断言「内容没丢」。
func renderMD(blocks []mdBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.kind == mdCode {
			b.WriteString(blk.code)
			continue
		}
		b.WriteString(blk.marker)
		for _, s := range blk.spans {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

// spanFor 在所有块里找第一个包含 sub 的片段。
func spanFor(blocks []mdBlock, sub string) (ui.Span, bool) {
	for _, blk := range blocks {
		for _, s := range blk.spans {
			if strings.Contains(s.Text, sub) {
				return s, true
			}
		}
	}
	return ui.Span{}, false
}

// 渲染不能吃掉内容：所有非语法字符都要原样出现在片段里。
func TestMarkdownKeepsAllText(t *testing.T) {
	src := "第一行普通文字\n\n- 要点一\n- 要点二\n\n**加粗** 和 `代码`\n"
	got := renderMD(mdBlocks(src, testMD))

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
	s, ok := spanFor(mdBlocks("这是**重点**内容", testMD), "重点")
	if !ok {
		t.Fatalf("找不到加粗片段：%+v", mdBlocks("这是**重点**内容", testMD))
	}
	if s.Weight != 700 {
		t.Errorf("加粗片段的 Weight = %d, want 700", s.Weight)
	}
	if s.Text != "重点" {
		t.Errorf("加粗片段多了星号：%q", s.Text)
	}
}

func TestMarkdownInlineCode(t *testing.T) {
	s, ok := spanFor(mdBlocks("调用 `expandTabs()` 展开", testMD), "expandTabs")
	if !ok {
		t.Fatalf("找不到行内代码片段：%+v", mdBlocks("调用 `expandTabs()` 展开", testMD))
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
	blocks := mdBlocks(src, testMD)

	var code *mdBlock
	for i := range blocks {
		if blocks[i].kind == mdCode {
			code = &blocks[i]
		}
	}
	if code == nil {
		t.Fatalf("没有切出代码块：%+v", blocks)
	}
	if !strings.Contains(code.code, "x := a * b") {
		t.Fatalf("代码块内容丢了：%q", code.code)
	}
	if strings.Contains(renderMD(blocks), "```") {
		t.Errorf("围栏记号应该被吃掉：%q", renderMD(blocks))
	}
	if !strings.Contains(code.code, `fmt.Println("`+"`"+`")`) {
		t.Errorf("代码块里的 * 和引号被当成格式了：%q", code.code)
	}
	// 代码块前后的正文各归各的，不能被卷进代码里
	if blocks[0].kind != mdPara || !strings.Contains(renderMD(blocks[:1]), "看这段") {
		t.Errorf("代码块前面的段落不对：%+v", blocks[0])
	}
}

// 代码块里的空行和缩进是代码的一部分，不能当分块的空行处理。
func TestMarkdownCodeKeepsBlankLines(t *testing.T) {
	src := "```\nfunc a() {\n\n\treturn\n}\n```"
	blocks := mdBlocks(src, testMD)
	if len(blocks) != 1 || blocks[0].kind != mdCode {
		t.Fatalf("空行把代码块切开了：%+v", blocks)
	}
	if want := "func a() {\n\n\treturn\n}"; blocks[0].code != want {
		t.Errorf("代码块 = %q, want %q", blocks[0].code, want)
	}
}

// 围栏没闭合时（模型忘了收尾），攒下的代码也要放出来。
func TestMarkdownUnclosedFence(t *testing.T) {
	blocks := mdBlocks("看：\n```\nx := 1", testMD)
	got := renderMD(blocks)
	if !strings.Contains(got, "x := 1") {
		t.Errorf("没闭合的代码块内容丢了：%q", got)
	}
}

func TestMarkdownList(t *testing.T) {
	for _, src := range []string{"- 第一点", "* 第一点", "+ 第一点", "1. 第一点", "2) 第一点"} {
		blocks := mdBlocks(src, testMD)
		if len(blocks) != 1 || blocks[0].kind != mdBullet {
			t.Fatalf("%q 没被认成列表项：%+v", src, blocks)
		}
		got := renderMD(blocks)
		if !strings.Contains(got, "第一点") {
			t.Errorf("%q 的内容丢了：%q", src, got)
		}
		if m := blocks[0].marker; m != "•" && !strings.HasPrefix(m, "1") && !strings.HasPrefix(m, "2") {
			t.Errorf("%q 的记号不对：%q", src, m)
		}
	}
	// 有序列号保留原编号：正文里常说「第 2 点」，重排就对不上了
	if got := renderMD(mdBlocks("3. 第三点", testMD)); !strings.Contains(got, "3.") {
		t.Errorf("有序列表应该保留原编号：%q", got)
	}
}

// 连续的列表项各自成块，中间的空行不算数。
func TestMarkdownListItemsAreSeparateBlocks(t *testing.T) {
	blocks := mdBlocks("- 甲\n- 乙\n- 丙", testMD)
	if len(blocks) != 3 {
		t.Fatalf("三个列表项应该切出三块，得到 %d：%+v", len(blocks), blocks)
	}
	for i, b := range blocks {
		if b.kind != mdBullet {
			t.Errorf("第 %d 块不是列表项：%+v", i, b)
		}
	}
}

func TestMarkdownHeading(t *testing.T) {
	blocks := mdBlocks("## 变更目的", testMD)
	if len(blocks) != 1 || blocks[0].kind != mdHead {
		t.Fatalf("没认成标题：%+v", blocks)
	}
	got := renderMD(blocks)
	if !strings.Contains(got, "变更目的") {
		t.Errorf("标题文字丢了：%q", got)
	}
	if strings.Contains(got, "#") {
		t.Errorf("标题的 # 应该去掉：%q", got)
	}
	if s, _ := spanFor(blocks, "变更目的"); s.Weight != 700 {
		t.Errorf("标题应该是粗体：%+v", s)
	}
}

// 引用的内容整体压暗一档，和正文分得开。
func TestMarkdownQuote(t *testing.T) {
	blocks := mdBlocks("> 别人的注释", testMD)
	if len(blocks) != 1 || blocks[0].kind != mdQuote {
		t.Fatalf("没认成引用：%+v", blocks)
	}
	s, ok := spanFor(blocks, "别人的注释")
	if !ok {
		t.Fatal("引用内容丢了")
	}
	if s.Color == testMD.text {
		t.Error("引用应该压暗，不该和正文同色")
	}
}

// 语法用错（星号不成对）时按字面显示，不能把后面的内容吃掉。
func TestMarkdownUnpairedMarkers(t *testing.T) {
	for _, src := range []string{"这是 * 号", "**没闭合", "`没闭合", "a ** b * c"} {
		got := renderMD(mdBlocks(src, testMD))
		for _, part := range strings.Fields(strings.NewReplacer("*", " ", "`", " ").Replace(src)) {
			if !strings.Contains(got, part) {
				t.Errorf("%q 渲染后丢了 %q：%q", src, part, got)
			}
		}
	}
}

// 首尾的空行不留块，卡片不会顶着一个空白开头；段落之间要分成两块。
func TestMarkdownTrimsBlankEdges(t *testing.T) {
	blocks := mdBlocks("\n\n正文\n\n\n", testMD)
	if len(blocks) == 0 {
		t.Fatal("内容全被当成空行了")
	}
	if got := strings.TrimSpace(renderMD(blocks)); got != "正文" {
		t.Errorf("首尾空行没去干净：%q", got)
	}
	if n := len(mdBlocks("甲\n\n乙", testMD)); n != 2 {
		t.Errorf("空行应该把段落分成两块，得到 %d 块", n)
	}
}

// 空输入不能崩，也不能吐出垃圾块。
func TestMarkdownEmpty(t *testing.T) {
	if blocks := mdBlocks("", testMD); len(blocks) != 0 {
		t.Errorf("空输入应该没有块：%+v", blocks)
	}
	if blocks := mdBlocks("\n\n", testMD); len(blocks) != 0 {
		t.Errorf("只有空行时应该没有块：%+v", blocks)
	}
}
