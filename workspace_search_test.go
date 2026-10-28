package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// wsWrite 往 dir 下写一个文件（路径用正斜杠，建目录一步到位）。
func wsWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanWsFilesFindsMatches(t *testing.T) {
	dir := t.TempDir()
	wsWrite(t, dir, "a.go", "package main\n\nfunc hello() {}\n// hello again\n")
	wsWrite(t, dir, "sub/b.txt", "nothing here\nHELLO there\n")
	wsWrite(t, dir, "c.md", "no match in this one\n")

	hits, files, truncated := scanWsFiles(dir, []string{"a.go", "sub/b.txt", "c.md"}, "hello")
	if truncated {
		t.Fatal("远没到上限，不该报截断")
	}
	if files != 2 {
		t.Errorf("fileCount = %d, want 2", files)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3（a.go 两处 + b.txt 一处）", len(hits))
	}
	// 路径序 + 行序：a.go 的两行在前，sub/b.txt 在后
	if hits[0].Path != "a.go" || hits[0].Line != 3 {
		t.Errorf("hits[0] = %+v, want a.go:3", hits[0])
	}
	if hits[1].Line != 4 {
		t.Errorf("hits[1].Line = %d, want 4", hits[1].Line)
	}
	if hits[2].Path != "sub/b.txt" || hits[2].Line != 2 {
		t.Errorf("hits[2] = %+v, want sub/b.txt:2（不分大小写）", hits[2])
	}
	// 高亮区间对回原文必须是查询词本身
	for i, h := range hits {
		if got := h.Text[h.Off : h.Off+h.Len]; !strings.EqualFold(got, "hello") {
			t.Errorf("hits[%d] 高亮段 = %q, want hello", i, got)
		}
	}
}

func TestScanWsFilesTrimsCarriageReturn(t *testing.T) {
	dir := t.TempDir()
	wsWrite(t, dir, "win.txt", "find me\r\nsecond line\r\n")

	hits, _, _ := scanWsFiles(dir, []string{"win.txt"}, "find me")
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if strings.HasSuffix(hits[0].Text, "\r") {
		t.Errorf("行尾的 \\r 该剪掉：%q", hits[0].Text)
	}
}

func TestScanWsFilesSkipsBinaryAndMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{'P', 'N', 'G', 0x00, 0x01, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	// 文本里带 NUL 也算二进制（looksBinary 的判据），不该扫出「内容」来
	hits, _, _ := scanWsFiles(dir, []string{"bin.dat", "gone/missing.txt"}, "x")
	if len(hits) != 0 {
		t.Errorf("二进制与不存在的文件不该有命中：%+v", hits)
	}
}

func TestScanWsFilesCJKWindowIsRuneAligned(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("长", 40) +
		"关键词夹在正中间" +
		strings.Repeat("后", 40)
	wsWrite(t, dir, "cjk.txt", long+"\n")

	hits, _, _ := scanWsFiles(dir, []string{"cjk.txt"}, "关键词")
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	h := hits[0]
	// 窗口两头按字符对齐：偏移切出来的三段拼回去必须还是合法文本，
	// 高亮段就是查询词
	if got := h.Text[h.Off : h.Off+h.Len]; got != "关键词" {
		t.Errorf("高亮段 = %q, want 关键词", got)
	}
	if !strings.HasPrefix(h.Text, "…") || !strings.HasSuffix(h.Text, "…") {
		t.Errorf("两头都截断了该带省略号：%q", h.Text)
	}
	if !utf8ValidString(h.Text) {
		t.Errorf("窗口文本不是合法 UTF-8：%q", h.Text)
	}
}

func TestScanWsFilesTruncatesAtHitCap(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < wsSearchMaxHits+50; i++ {
		b.WriteString("hit me\n")
	}
	wsWrite(t, dir, "many.txt", b.String())

	hits, files, truncated := scanWsFiles(dir, []string{"many.txt"}, "hit me")
	if !truncated {
		t.Error("超过命中上限该如实报截断")
	}
	if len(hits) != wsSearchMaxHits {
		t.Errorf("hits = %d, want %d", len(hits), wsSearchMaxHits)
	}
	if files != 1 {
		t.Errorf("fileCount = %d, want 1", files)
	}
}

func TestScanWsFilesEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	wsWrite(t, dir, "a.txt", "anything\n")
	hits, _, truncated := scanWsFiles(dir, []string{"a.txt"}, "")
	if hits != nil || truncated {
		t.Errorf("空查询该直接返回空：%+v", hits)
	}
}

func TestWsSearchRowsGroupsByPath(t *testing.T) {
	hits := []wsSearchHit{
		{Path: "a.go", Line: 3},
		{Path: "a.go", Line: 7},
		{Path: "sub/b.txt", Line: 2},
	}
	rows := wsSearchRows(hits)
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5（两个组头 + 三条命中）", len(rows))
	}
	if !rows[0].head || rows[0].path != "a.go" || rows[0].count != 2 {
		t.Errorf("rows[0] = %+v, want a.go 组头 count=2", rows[0])
	}
	if rows[1].head || rows[1].hit != &hits[0] {
		t.Errorf("rows[1] = %+v, want a.go:3 的命中", rows[1])
	}
	if !rows[3].head || rows[3].path != "sub/b.txt" || rows[3].count != 1 {
		t.Errorf("rows[3] = %+v, want sub/b.txt 组头 count=1", rows[3])
	}
}

// utf8ValidString 报告 s 是不是合法 UTF-8。测试里直接用 unicode/utf8 的
// ValidString 就行，这里包一层是为了让用意（窗口必须字符对齐）读得出来。
func utf8ValidString(s string) bool { return utf8.ValidString(s) }
