// Package highlight 用 chroma 把源码切成带类型的片段，供界面按行上色。
//
// 只做「哪一段是什么 token」，不决定颜色：同一份切分结果在浅色和深色
// 主题下都能用，颜色在渲染时按当前主题查。这样切换主题不需要重新切分。
package highlight

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Seg 是行内一段同类型的文本，区间是该行里的字节下标。
type Seg struct {
	Start, End int
	Type       chroma.TokenType
}

// File 是一个文件切分好的结果，按行（从 1 开始）取用。
type File struct {
	lines [][]Seg
	// plain 表示这个文件没被任何词法器认出来，全是普通文本。
	// 界面据此走「整行一个颜色」的快路径，省掉每行拼 span 的开销。
	plain bool
}

// Plain 报告文件是否按纯文本处理。
func (f *File) Plain() bool { return f == nil || f.plain }

// Line 返回第 n 行（从 1 开始）的片段。超出范围或没有内容时返回 nil，
// 调用方按整行一个颜色处理即可。
func (f *File) Line(n int) []Seg {
	if f == nil || n < 1 || n > len(f.lines) {
		return nil
	}
	return f.lines[n-1]
}

// NeedHighlight 判断这个文件名值不值得高亮。
//
// 词法器认不出来的一律走纯文本。chroma 会把 .txt 之类也认成 plaintext
// 词法器，那种「高亮」只是把每行标成 Text，白跑一遍切分，所以按名字挡掉。
func NeedHighlight(name string) bool {
	l := lexers.Match(filepath.Base(name))
	if l == nil {
		return false
	}
	switch l.Config().Name {
	case "plaintext", "fallback", "":
		return false
	}
	return true
}

// NewLines 切分文件内容。lines 是按行切好的内容（不含换行符）。
//
// 必须整份文件一起切：多行字符串、块注释这类结构单看一行是判断不出来的，
// 而 diff 恰好经常出现在这些结构附近。
func NewLines(name string, lines []string) *File {
	if !NeedHighlight(name) {
		return &File{plain: true}
	}
	lexer := chroma.Coalesce(lexers.Match(filepath.Base(name)))

	f := &File{lines: make([][]Seg, len(lines))}
	it, err := lexer.Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return f
	}

	line, col := 0, 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		text := tok.Value
		for len(text) > 0 {
			nl := strings.IndexByte(text, '\n')
			if nl < 0 {
				f.add(line, col, col+len(text), tok.Type)
				col += len(text)
				break
			}
			f.add(line, col, col+nl, tok.Type)
			line++
			col = 0
			text = text[nl+1:]
			if line >= len(lines) {
				// 词法器多吐了换行：行数以传入的 lines 为准，多的丢掉
				return f
			}
		}
	}
	return f
}

// add 往某一行追加一个片段。相邻且同类型的片段会合并 ——
// coalesce 之后这种情况不多，但换行切分本身也会制造出不少。
func (f *File) add(line, start, end int, t chroma.TokenType) {
	if end <= start || line < 0 || line >= len(f.lines) {
		return
	}
	segs := f.lines[line]
	if n := len(segs); n > 0 && segs[n-1].End == start && segs[n-1].Type == t {
		segs[n-1].End = end
		return
	}
	f.lines[line] = append(segs, Seg{Start: start, End: end, Type: t})
}

// Style 按明暗取配色方案。名字不存在时 chroma 会退回默认方案，
// 不会返回 nil。
func Style(dark bool) *chroma.Style {
	if dark {
		return styles.Get("github-dark")
	}
	return styles.Get("github")
}

// Entry 是渲染一段文字需要的样式。
type Entry struct {
	// Hex 是 "#rrggbb"；空串表示这套配色没规定颜色，该用默认前景色。
	Hex    string
	Bold   bool
	Italic bool
}

// Palette 是一套配色方案，缓存每个 token 类型解析好的样式。
//
// 切分结果和明暗无关，颜色却随主题变 —— 所以颜色在渲染时按当前主题
// 解析，解析一次就缓存下来，不必每帧对每一段重新做一遍样式继承。
type Palette struct {
	style *chroma.Style
	cache map[chroma.TokenType]Entry
}

// NewPalette 按明暗取一套配色。
func NewPalette(dark bool) *Palette {
	return &Palette{style: Style(dark), cache: map[chroma.TokenType]Entry{}}
}

// Entry 返回某个片段该用什么样式。不认识的片段类型返回零值，
// 调用方用默认前景色即可。
func (p *Palette) Entry(s Seg) Entry {
	if e, ok := p.cache[s.Type]; ok {
		return e
	}
	st := p.style.Get(s.Type)
	e := Entry{Bold: st.Bold == chroma.Yes, Italic: st.Italic == chroma.Yes}
	if st.Colour != 0 {
		e.Hex = st.Colour.String()
	}
	p.cache[s.Type] = e
	return e
}

// Resolve 查 token 类型在给定配色下的样式。切分本身不看样式，
// 所以这个函数只给测试和 Palette 用。
func Resolve(style *chroma.Style, t chroma.TokenType) Entry {
	st := style.Get(t)
	e := Entry{Bold: st.Bold == chroma.Yes, Italic: st.Italic == chroma.Yes}
	if st.Colour != 0 {
		e.Hex = st.Colour.String()
	}
	return e
}
