package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
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

	hits, files, truncated := scanWsFiles(dir, []string{"a.go", "sub/b.txt", "c.md"}, "hello", nil, nil)
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

	hits, _, _ := scanWsFiles(dir, []string{"win.txt"}, "find me", nil, nil)
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
	hits, _, _ := scanWsFiles(dir, []string{"bin.dat", "gone/missing.txt"}, "x", nil, nil)
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

	hits, _, _ := scanWsFiles(dir, []string{"cjk.txt"}, "关键词", nil, nil)
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

	hits, files, truncated := scanWsFiles(dir, []string{"many.txt"}, "hit me", nil, nil)
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
	hits, _, truncated := scanWsFiles(dir, []string{"a.txt"}, "", nil, nil)
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

func TestScanWsFilesRecordsHitOffsets(t *testing.T) {
	dir := t.TempDir()
	// CRLF 文件：行起始字节按原始字节算（\r\n 占两个），LfPos 再把命中行
	// 之前每个 CRLF 行尾扣回去 —— 那正是编辑器缓冲里归一成 LF 之后的偏移。
	wsWrite(t, dir, "win.txt", "one\r\ntwo find\r\nthree find\r\n")
	wsWrite(t, dir, "unix.txt", "one\ntwo find\nthree find\n")

	hits, _, _ := scanWsFiles(dir, []string{"win.txt", "unix.txt"}, "find", nil, nil)
	if len(hits) != 4 {
		t.Fatalf("hits = %d, want 4", len(hits))
	}
	for _, h := range hits {
		var pos, lfPos int
		switch {
		case h.Path == "win.txt" && h.Line == 2:
			pos, lfPos = 9, 8 // "one\r\n" 5 字节 + 行内偏移 4；LF 文本少一个 \r
		case h.Path == "win.txt" && h.Line == 3:
			pos, lfPos = 21, 19 // 两个 CRLF 行尾都已被走过
		case h.Path == "unix.txt":
			pos, lfPos = h.Pos, h.Pos // 没有 \r，两套偏移一致
		default:
			t.Fatalf("意外的命中：%+v", h)
		}
		if h.Pos != pos || h.LfPos != lfPos {
			t.Errorf("%s:%d Pos/LfPos = %d/%d, want %d/%d", h.Path, h.Line, h.Pos, h.LfPos, pos, lfPos)
		}
	}
}

func TestRevealWsHitSelectsMatch(t *testing.T) {
	dir := t.TempDir()
	raw := "one\r\ntwo find\r\nthree find\r\n"
	wsWrite(t, dir, "win.txt", raw)

	hits, _, _ := scanWsFiles(dir, []string{"win.txt"}, "find", nil, nil)
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	st := &editor.State{Buffer: editor.NewBuffer(raw)}
	revealWsHit(st, hits[0])
	// 选中底色就是跳转的高亮：选中的文字必须正是命中的那段
	if sel := st.SelectedText(); !strings.EqualFold(sel, "find") {
		t.Errorf("SelectedText = %q, want find", sel)
	}
	// 光标跟着落在匹配段上，即命中那一行（0 起算的第 1 行）
	if pos := st.CursorPos(); pos.Line != 1 {
		t.Errorf("CursorPos.Line = %d, want 1", pos.Line)
	}
}

func TestRevealWsHitFallsBackWhenFileChanged(t *testing.T) {
	// 搜索之后文件变过：缓冲里的字节对不上扫到的，就不选（会点错地方），
	// 退回把光标放到命中行行首，活动行高亮顶上。
	st := &editor.State{Buffer: editor.NewBuffer("aaa\nbbb\nccc find me\nddd\n")}
	revealWsHit(st, wsSearchHit{Path: "x.go", Line: 3, Text: "…find me…", Off: 1, Len: 7})
	if sel := st.SelectedText(); sel != "" {
		t.Errorf("SelectedText = %q, want 空（内容对不上不该选中）", sel)
	}
	if pos := st.CursorPos(); pos.Line != 2 {
		t.Errorf("CursorPos.Line = %d, want 2（光标退到命中行行首）", pos.Line)
	}
}

func TestRevealWsHitScrollsForGroupHead(t *testing.T) {
	// 文件组头的跳转没有匹配段可亮：只滚到那一行，别动光标。
	st := &editor.State{Buffer: editor.NewBuffer("l1\nl2\nl3\n")}
	revealWsHit(st, wsSearchHit{Path: "x.txt", Line: 3})
	if st.SelectedText() != "" {
		t.Errorf("SelectedText = %q, want 空（组头跳转不选中）", st.SelectedText())
	}
	if pos := st.CursorPos(); pos.Line != 0 {
		t.Errorf("CursorPos.Line = %d, want 0（光标不该被移动）", pos.Line)
	}
}

func TestWsGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		// 不带 / 的模式对任意一层的名字匹配
		{"*.go", "main.go", true},
		{"*.go", "internal/gitrepo/repo.go", true},
		{"*.go", "main.ts", false},
		{"node_modules", "node_modules/x.js", true},       // 排除目录：目录名这一层对上就整棵出局
		{"node_modules", "a/b/node_modules/c.js", true},   // 任意深度同理
		{"node_modules", "src/node_modules2/x.js", false}, // 名字要整段对上，不是子串
		// 带 / 的模式按根相对的全路径走，** 跨任意层
		{"src/**", "src/a/b.go", true},
		{"src/**", "src/main.go", true},
		{"src/**", "libs/src/a.go", false},
		{"**/vendor/**", "a/vendor/b/c.go", true},
		{"**/vendor/**", "a/b/c.go", false},
		{"internal/store/*.go", "internal/store/store.go", true},
		{"internal/store/*.go", "internal/store/sub/store.go", false}, // 单 * 不跨层
		{"test?.go", "test1.go", true},
		{"test?.go", "test12.go", false},
		{"", "anything", false},
		{"*.GO", "a.go", true}, // 大小写不敏感
	}
	for _, c := range cases {
		if got := wsGlobMatch(c.pattern, c.path); got != c.want {
			t.Errorf("wsGlobMatch(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestWsGlobPatterns(t *testing.T) {
	if got := wsGlobPatterns("*.go, src/** ，node_modules  *.md"); len(got) != 4 {
		t.Errorf("wsGlobPatterns = %q, want 4 条（中英文逗号和空格都算分隔）", got)
	}
	if got := wsGlobPatterns("  ,, ， "); len(got) != 0 {
		t.Errorf("纯分隔符该拆出空表：%q", got)
	}
}

func TestWsPathAllowed(t *testing.T) {
	// 都空 = 全放行
	if !wsPathAllowed("a/b.go", nil, nil) {
		t.Error("两框都空该全放行")
	}
	// 包含：至少命中一个
	if wsPathAllowed("a/b.ts", []string{"*.go"}, nil) {
		t.Error("没命中包含模式该出局")
	}
	// 排除比包含重：包含放进来的，排除能捞出去
	if wsPathAllowed("vendor/x.go", []string{"*.go"}, []string{"vendor"}) {
		t.Error("排除命中的该出局，哪怕包含也命中")
	}
}

func TestScanWsFilesRespectsPathFilters(t *testing.T) {
	dir := t.TempDir()
	wsWrite(t, dir, "a.go", "find me here\n")
	wsWrite(t, dir, "sub/b.ts", "find me too\n")
	wsWrite(t, dir, "vendor/x.go", "find me not\n")
	paths := []string{"a.go", "sub/b.ts", "vendor/x.go"}

	// 不带 / 的 *.go 对任意深度的 Go 文件都算，vendor 里的也在内
	hits, _, _ := scanWsFiles(dir, paths, "find", []string{"*.go"}, nil)
	if len(hits) != 2 {
		t.Errorf("包含 *.go 该剩 a.go 和 vendor/x.go 两处：%+v", hits)
	}
	// 再叠一个排除，vendor 就捞出去了 —— 排除比包含重
	hits, _, _ = scanWsFiles(dir, paths, "find", []string{"*.go"}, []string{"vendor"})
	if len(hits) != 1 || hits[0].Path != "a.go" {
		t.Errorf("排除 vendor 后该只剩 a.go：%+v", hits)
	}

	hits, _, _ = scanWsFiles(dir, paths, "find", nil, []string{"vendor"})
	if len(hits) != 2 {
		t.Errorf("排除 vendor 后该剩两处：%+v", hits)
	}

	// 被挡在门外的文件一文不值：连 walked 都不算，扫描上限不会被它们吃掉
	hits, _, truncated := scanWsFiles(dir, paths, "find", []string{"a.go"}, nil)
	if truncated || len(hits) != 1 {
		t.Errorf("glob 过滤不该算进扫描量：%+v truncated=%v", hits, truncated)
	}
}

func TestEditorTargetPinnedThroughTreeFilter(t *testing.T) {
	var a app
	a.viewMode = viewModeEditor
	a.wsAll = []gitrepo.File{
		{}, {}, {},
	}
	a.wsAll[0].NewPath = "keep.go"
	a.wsAll[1].NewPath = "filtered.go"
	a.wsAll[2].NewPath = "other.go"
	// 树筛走了 filtered.go（不在 wsFiles 里），选中落空
	a.wsFiles = []gitrepo.File{{NewPath: "keep.go"}}
	a.wsSel = -1
	a.wsPinPath = "filtered.go"

	f := a.editorTarget()
	if f == nil || pathOf(*f) != "filtered.go" {
		t.Errorf("editorTarget = %v, want 钉住的 filtered.go", f)
	}

	// 点树里可见的任何一行，钉住就作废，回到正常选中
	a.selectWorkspaceKey("keep.go")
	if a.wsPinPath != "" {
		t.Error("selectWorkspaceKey 该清掉钉住")
	}
	if f := a.editorTarget(); f == nil || pathOf(*f) != "keep.go" {
		t.Errorf("editorTarget = %v, want keep.go", f)
	}

	// 钉住的文件连 wsAll 里都没有了（搜索之后被删）—— 退回列表第一个
	a.wsSel = -1
	a.wsPinPath = "deleted.go"
	if f := a.editorTarget(); f == nil || pathOf(*f) != "keep.go" {
		t.Errorf("editorTarget = %v, want 退回 keep.go", f)
	}
}

func TestWsSearchSuggestionsAndAppend(t *testing.T) {
	files := []gitrepo.File{
		{NewPath: "internal/store/store.go"},
		{NewPath: "internal/gitrepo/repo.go"},
		{NewPath: "ui/view.go"},
		{NewPath: "ui/list.ts"},
		{NewPath: "readme.md"},
	}
	sug := wsSearchSuggestions(files)
	found := map[string]bool{}
	for _, s := range sug {
		found[s] = true
	}
	if !found["internal"] || !found["ui"] {
		t.Errorf("建议里该有顶层目录 internal 和 ui：%q", sug)
	}
	if !found["*.go"] {
		t.Errorf("建议里该有 *.go：%q", sug)
	}

	// 珠子并进输入框：去重、逗号续写
	if got := wsAppendPattern("*.md, internal", "internal"); got != "*.md, internal" {
		t.Errorf("重复的珠不该再并：%q", got)
	}
	if got := wsAppendPattern("*.md", "ui"); got != "*.md, ui" {
		t.Errorf("并珠 = %q, want *.md, ui", got)
	}
	if got := wsAppendPattern("", "*.go"); got != "*.go" {
		t.Errorf("空框并珠 = %q, want *.go", got)
	}
}
