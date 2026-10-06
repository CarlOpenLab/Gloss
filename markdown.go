package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// 迷你 Markdown 渲染：把模型输出的文字变成一串带样式的 ui.Span。
//
// 为什么不用现成的库：解释卡片只要几个最简单的语法（粗体、行内代码、
// 代码块、标题、列表），而 mygo 的 RichText 本来就是一个「span 数组」
// 模型 —— 引一个 AST 库再翻译回 span 反而更绕。
//
// 认不出来的一律当纯文本，宁可少认几种语法，也不要把内容吃掉。

// mdStyle 是渲染一套 Markdown 需要的颜色与字体，由调用方从主题里取。
type mdStyle struct {
	text   ui.Color // 正文
	muted  ui.Color // 引用、次要文字
	code   ui.Color // 行内代码与代码块的文字
	codeBg ui.Color // 代码的背景
	head   ui.Color // 标题、粗体
	bullet ui.Color // 列表符号
	mono   string   // 等宽字体名
}

// mdSpans 把一段 Markdown 转成 spans。
func mdSpans(src string, st mdStyle) []ui.Span {
	var out []ui.Span
	inFence := false

	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			// 围栏行本身不显示，它只是个开关
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, ui.Span{Text: line + "\n", Font: st.mono, Color: st.code, Background: st.codeBg})
			continue
		}
		out = append(out, mdLineSpans(line, st)...)
	}
	return trimSpans(out)
}

// mdLineSpans 渲染一整行：先按块级语法（标题、列表、引用）决定前缀的样式，
// 再把剩下的正文按行内语法切分。
func mdLineSpans(line string, st mdStyle) []ui.Span {
	trimmed := strings.TrimLeft(line, " \t")
	indent := len(line) - len(trimmed)

	var spans []ui.Span
	switch {
	case strings.HasPrefix(trimmed, "#"):
		// 标题：去掉 # 号，只留加粗的字。层级不做区分 ——
		// 卡片里的标题只用来分段，字号分级反而显得乱。
		text := strings.TrimLeft(trimmed, "#")
		spans = append(spans, ui.Span{Text: strings.TrimSpace(text) + "\n", Weight: 700, Color: st.head})
		return spans

	case isBullet(trimmed):
		// 列表：缩进保留，符号换成圆点，其余的照常做行内解析
		marker, rest := bulletParts(trimmed)
		spans = append(spans,
			ui.Span{Text: strings.Repeat(" ", indent) + marker + " ", Color: st.bullet},
		)
		spans = append(spans, mdInlineSpans(rest, st)...)
		spans = append(spans, ui.Span{Text: "\n"})
		return spans

	case strings.HasPrefix(trimmed, "> "):
		spans = append(spans, ui.Span{Text: strings.Repeat(" ", indent) + "▏ ", Color: st.bullet})
		inner := mdInlineSpans(trimmed[2:], st)
		for i := range inner {
			// 引用整体压暗一档，和正文分得开
			if inner[i].Color == st.text || inner[i].Color == (ui.Color{}) {
				inner[i].Color = st.muted
			}
		}
		spans = append(spans, inner...)
		spans = append(spans, ui.Span{Text: "\n"})
		return spans
	}

	spans = append(spans, mdInlineSpans(line, st)...)
	spans = append(spans, ui.Span{Text: "\n"})
	return spans
}

// mdInlineSpans 处理行内语法：**粗体**、`代码`、*斜体*。
func mdInlineSpans(s string, st mdStyle) []ui.Span {
	var out []ui.Span
	for len(s) > 0 {
		i := strings.IndexAny(s, "*`")
		if i < 0 {
			out = append(out, ui.Span{Text: s, Color: st.text})
			break
		}
		if i > 0 {
			out = append(out, ui.Span{Text: s[:i], Color: st.text})
			s = s[i:]
		}

		switch s[0] {
		case '`':
			// 行内代码：成对的反引号，找不到配对就按普通字符处理
			if end := strings.IndexByte(s[1:], '`'); end >= 0 {
				out = append(out, ui.Span{
					Text:       " " + s[1:1+end] + " ",
					Font:       st.mono,
					Color:      st.code,
					Background: st.codeBg,
				})
				s = s[end+2:]
				continue
			}
			out = append(out, ui.Span{Text: "`", Color: st.text})
			s = s[1:]

		case '*':
			// 粗体优先；** 不成对时退而求其次找单个 *
			if strings.HasPrefix(s, "**") {
				if end := strings.Index(s[2:], "**"); end >= 0 {
					out = append(out, ui.Span{Text: s[2 : 2+end], Weight: 700, Color: st.head})
					s = s[end+4:]
					continue
				}
			}
			if end := strings.IndexByte(s[1:], '*'); end >= 0 && end > 0 {
				out = append(out, ui.Span{Text: s[1 : 1+end], Italic: true, Color: st.text})
				s = s[end+2:]
				continue
			}
			out = append(out, ui.Span{Text: "*", Color: st.text})
			s = s[1:]
		}
	}
	return out
}

// isBullet 判断是不是列表项：- / * / + 开头，或者 1. 这样的序号。
func isBullet(s string) bool {
	if len(s) >= 2 && (s[0] == '-' || s[0] == '+' || s[0] == '*') && s[1] == ' ' {
		return true
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(s) && (s[i] == '.' || s[i] == ')') && s[i+1] == ' '
}

// bulletParts 把列表项拆成「符号」和「内容」。有序列表保留原编号，
// 重新编号反而会和正文里「第 2 点」这样的说法对不上。
func bulletParts(s string) (marker, rest string) {
	if strings.ContainsAny(s[:min(3, len(s))], "0123456789") {
		i := strings.IndexByte(s, ' ')
		return s[:i], s[i+1:]
	}
	return "•", s[2:]
}

// trimSpans 去掉首尾的空行 —— 模型经常在开头结尾多给一两行。
func trimSpans(spans []ui.Span) []ui.Span {
	for len(spans) > 0 && strings.TrimSpace(spans[len(spans)-1].Text) == "" {
		spans = spans[:len(spans)-1]
	}
	for len(spans) > 0 && strings.TrimSpace(spans[0].Text) == "" {
		spans = spans[1:]
	}
	return spans
}
