package highlight

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

const goSrc = `package main

import "fmt"

// 一个函数
func main() {
	s := "字符串"
	fmt.Println(s, 42)
}
`

func goFile(t *testing.T) (*File, []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(goSrc, "\n"), "\n")
	return NewLines("main.go", lines), lines
}

// 片段必须严丝合缝地铺满每一行：漏一段就会丢字符，重叠就会重复渲染。
// 这是整份高亮里最要紧的一条不变量。
func TestSegmentsTileEveryLine(t *testing.T) {
	f, lines := goFile(t)
	for i, line := range lines {
		segs := f.Line(i + 1)
		col := 0
		for _, s := range segs {
			if s.Start != col {
				t.Fatalf("第 %d 行第 %d 段起点 = %d, want %d（行内容 %q）", i+1, s.Start, col, s.Start, line)
			}
			col = s.End
		}
		if col != len(line) {
			t.Errorf("第 %d 行片段覆盖到 %d, want %d（行内容 %q）", i+1, col, len(line), line)
		}
	}
}

// 关键词要真的被认出来，否则高亮等于没做。
func TestKeywordsAreTokenized(t *testing.T) {
	f, _ := goFile(t)
	segs := f.Line(1) // "package main"
	if len(segs) < 2 {
		t.Fatalf("第 1 行只切出 %d 段：%+v", len(segs), segs)
	}
	if segs[0].Type != chroma.KeywordNamespace {
		t.Errorf("package 的 token 类型 = %v, want KeywordNamespace", segs[0].Type)
	}
}

// 多行结构（这里是块注释）要跨行成立：单看一行判断不出来，
// 所以必须整份文件一起切。
func TestMultiLineConstructSpansLines(t *testing.T) {
	src := "package main\n\n/*\n这是块注释\n*/\nvar x = 1\n"
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	f := NewLines("a.go", lines)

	// 第 4 行完全在块注释里（具体是 Comment 下哪个子类不重要）
	if segs := f.Line(4); len(segs) != 1 || !segs[0].Type.InCategory(chroma.Comment) {
		t.Errorf("块注释中间那行 = %+v, want 单个注释片段", segs)
	}
}

// 认不出后缀的文件按纯文本处理，省掉整份切分的开销。
func TestPlainFilesSkipTokenizing(t *testing.T) {
	f := NewLines("notes.txt", []string{"随便写点什么", "第二行"})
	if !f.Plain() {
		t.Error("txt 应该按纯文本处理")
	}
	if f.Line(1) != nil {
		t.Errorf("纯文本文件不该有片段，实际 %+v", f.Line(1))
	}
}

func TestNeedHighlight(t *testing.T) {
	cases := map[string]bool{
		"main.go":         true,
		"handler.py":      true,
		"index.ts":        true,
		"Makefile":        true,
		"notes.txt":       false,
		"LICENSE":         false,
		"logo.png":        false,
		"deep/dir/app.rb": true,
	}
	for name, want := range cases {
		if got := NeedHighlight(name); got != want {
			t.Errorf("NeedHighlight(%q) = %v, want %v", name, got, want)
		}
	}
}

// 行号越界不该 panic —— 界面按行号取内容，diff 和文件内容对不上的瞬间会问到。
func TestLineOutOfRange(t *testing.T) {
	f, _ := goFile(t)
	if f.Line(0) != nil || f.Line(-1) != nil || f.Line(9999) != nil {
		t.Error("越界应该返回 nil")
	}
	var nilFile *File
	if nilFile.Line(1) != nil || !nilFile.Plain() {
		t.Error("nil 文件应该安全地当纯文本处理")
	}
}

// 空行、空文件都不能把行号搞乱。
func TestBlankLinesAndEmptyInput(t *testing.T) {
	f := NewLines("a.go", []string{"package main", "", "var x = 1"})
	if segs := f.Line(2); len(segs) != 0 {
		t.Errorf("空行不该有片段，实际 %+v", segs)
	}
	if segs := f.Line(3); len(segs) == 0 {
		t.Error("第 3 行应该有片段")
	}

	empty := NewLines("a.go", nil)
	if empty.Line(1) != nil {
		t.Error("空文件不该有片段")
	}
}

func TestStyleResolvesColors(t *testing.T) {
	for _, dark := range []bool{false, true} {
		st := Style(dark)
		if st == nil {
			t.Fatalf("dark=%v 时配色为空", dark)
		}
		kw := Resolve(st, chroma.Keyword)
		if !strings.HasPrefix(kw.Hex, "#") || len(kw.Hex) != 7 {
			t.Errorf("dark=%v 时关键词颜色格式不对：%q", dark, kw.Hex)
		}

		// Palette 缓存同一个 token 类型的结果，两次问必须给一样的答案
		p := NewPalette(dark)
		seg := Seg{Type: chroma.Keyword}
		if p.Entry(seg) != p.Entry(seg) {
			t.Errorf("dark=%v 时 Palette 两次解析结果不一致", dark)
		}
		if p.Entry(seg).Hex == "" {
			t.Errorf("dark=%v 时 Palette 没解析出关键词颜色", dark)
		}
	}
}
