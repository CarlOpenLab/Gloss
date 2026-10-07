package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// 迷你 Markdown 渲染：把模型输出的文字变成一块块可以各自排版的元素。
//
// 为什么不直接摊成一片 span 交给 RichText：那样的换行只认 \n，段落一折行
// 就顶到最左边 —— 列表项的第二行会和符号对齐（「• 写了很多字 / 接下去」
// 看着成了两段）；代码块只能靠 span 的文字底色，出不来内边距和圆角，
// 一片灰底紧贴着字。切成块之后，每一块交给布局系统：
//
//   - 列表是「符号列 + 文字列」两栏，折行自然对齐文字（悬挂缩进）；
//   - 代码块是一个有底色、内边距和圆角的盒子；
//   - 引用左边是一条竖线，而不是一个「▏」字符；
//   - 块与块的间距由布局给，不再靠空行。
//
// 行内语法（粗体、行内代码、斜体）仍然解析成 span —— 那些是字内部的样式，
// 本来就该由 RichText 画。
//
// 认不出来的一律当纯文本，宁可少认几种语法，也不要把内容吃掉。

// mdKind 是一块的类型。
type mdKind uint8

const (
	mdPara   mdKind = iota // 普通段落
	mdBullet               // 列表项
	mdCode                 // 代码块
	mdHead                 // 标题
	mdQuote                // 引用
)

// mdBlock 是一块排好版的 Markdown。
type mdBlock struct {
	kind   mdKind
	marker string    // 列表项的符号：无序是 •，有序保留原编号
	spans  []ui.Span // 行内解析后的内容（代码块不用）
	code   string    // 代码块原文，围栏行已去掉
}

// mdStyle 是渲染一套 Markdown 需要的颜色与字体，由调用方从主题里取。
type mdStyle struct {
	text   ui.Color // 正文
	muted  ui.Color // 引用、次要文字
	code   ui.Color // 行内代码与代码块的文字
	codeBg ui.Color // 代码的背景
	head   ui.Color // 标题、粗体
	bullet ui.Color // 列表符号、引用条
	mono   string   // 等宽字体名
}

// mdBlocks 把一段 Markdown 切成块。
//
// 空行只用来分块，本身不留 —— 块间距交给布局，两个来源管一件事迟早会打架
// （模型多给一行空行，屏幕上就多一段空白）。首尾的空块也去掉，卡片不顶着
// 一个空白开头。
func mdBlocks(src string, st mdStyle) []mdBlock {
	var out []mdBlock
	var para []string    // 攒着的段落行
	var bullet []string  // 攒着的列表项（裸文本）
	var code []string    // 攒着的代码块行
	var markers []string // 和 bullet 一一对应的符号
	inFence := false

	flushPara := func() {
		if len(para) > 0 {
			out = append(out, mdBlock{kind: mdPara, spans: mdInlineSpans(strings.Join(para, "\n"), st)})
			para = para[:0]
		}
	}
	flushBullet := func() {
		for i, item := range bullet {
			out = append(out, mdBlock{kind: mdBullet, marker: markers[i], spans: mdInlineSpans(item, st)})
		}
		bullet, markers = bullet[:0], markers[:0]
	}
	flushCode := func() {
		if len(code) > 0 {
			out = append(out, mdBlock{kind: mdCode, code: strings.Join(code, "\n")})
			code = code[:0]
		}
	}
	flush := func() { flushPara(); flushBullet(); flushCode() }

	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			// 围栏行本身不显示，它只是个开关。开关两边的块各归各的。
			if inFence {
				flushCode()
			} else {
				flushPara()
				flushBullet()
			}
			inFence = !inFence
			continue
		}
		if inFence {
			// 代码里原样保留缩进和空行
			code = append(code, line)
			continue
		}

		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case strings.TrimSpace(line) == "":
			flush()
		case strings.HasPrefix(trimmed, "#"):
			flush()
			text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			out = append(out, mdBlock{kind: mdHead, spans: []ui.Span{{Text: text, Weight: 700, Color: st.head}}})
		case isBullet(trimmed):
			flushPara()
			flushCode()
			marker, rest := bulletParts(trimmed)
			markers = append(markers, marker)
			bullet = append(bullet, rest)
		case strings.HasPrefix(trimmed, "> "):
			flushPara()
			flushBullet()
			flushCode()
			inner := mdInlineSpans(trimmed[2:], st)
			for i := range inner {
				// 引用整体压暗一档，和正文分得开
				if inner[i].Color == st.text || inner[i].Color == (ui.Color{}) {
					inner[i].Color = st.muted
				}
			}
			out = append(out, mdBlock{kind: mdQuote, spans: inner})
		default:
			flushBullet()
			flushCode()
			para = append(para, line)
		}
	}
	// 围栏没闭合时，攒下的代码也要放出来 —— 模型经常忘了收尾
	flush()
	return out
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
