package main

import (
	"strings"
	"testing"

	"diff-explainer/internal/diff"
	"diff-explainer/internal/gitrepo"
)

func fileFromPatch(t *testing.T, patch string) gitrepo.File {
	t.Helper()
	parsed := diff.Parse(patch)
	if len(parsed) == 0 {
		t.Fatalf("解析不出文件：%q", patch)
	}
	return gitrepo.File{FileDiff: parsed[0], Status: gitrepo.StatusModified}
}

func kinds(rows []row) []rowKind {
	out := make([]rowKind, len(rows))
	for i, r := range rows {
		out[i] = r.kind
	}
	return out
}

func shape(rows []row) string {
	var b []string
	for _, r := range rows {
		switch r.kind {
		case rowFileHeader:
			b = append(b, "H")
		case rowHunkHeader:
			b = append(b, "@")
		case rowLine:
			b = append(b, "L")
		case rowGap:
			b = append(b, "G")
		case rowNote:
			b = append(b, "N")
		}
	}
	return strings.Join(b, "")
}

const simplePatch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 package main
+// added
 func main() {}
`

func TestBuildRowsSimple(t *testing.T) {
	f := fileFromPatch(t, simplePatch)
	rows := buildRows([]gitrepo.File{f}, nil, nil, false)

	if got := shape(rows); got != "H@LLL" {
		t.Errorf("行结构 = %s, want H@LLL", got)
	}
}

// hunk 从第 40 行开始，前面 39 行未改动 —— 应该折成一条而不是铺 39 行。
func TestBuildRowsLeadingGapCollapses(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -40,3 +40,4 @@
 line40
+added
 line41
 line42
`)
	rows := buildRows([]gitrepo.File{f}, nil, nil, false)

	if got := shape(rows); got != "HG@LLLL" {
		t.Fatalf("行结构 = %s, want HG@LLLL", got)
	}
	if rows[1].gapN != 39 {
		t.Errorf("折叠行数 = %d, want 39", rows[1].gapN)
	}
	if rows[1].gapKey == "" {
		t.Error("折叠条应该有 key，否则展开状态没法记住")
	}
}

// 不到 12 行的未改动区间直接摊开，不折成条。
func TestBuildRowsShortGapIsInlined(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -5,2 +5,3 @@
 x
+y
 z
`)
	rows := buildRows([]gitrepo.File{f}, nil, nil, false)

	// 前 4 行被摊开，没有折叠条
	if got := shape(rows); got != "HLLLL@LLL" {
		t.Errorf("行结构 = %s, want HLLLL@LLL", got)
	}
	if rows[1].oldNo != 1 || rows[1].newNo != 1 {
		t.Errorf("第一条展开行的行号 = old %d new %d, want 1/1", rows[1].oldNo, rows[1].newNo)
	}
	if rows[4].oldNo != 4 || rows[4].newNo != 4 {
		t.Errorf("第四条展开行的行号 = old %d new %d, want 4/4", rows[4].oldNo, rows[4].newNo)
	}
}

// 展开状态要能被 buildRows 认出来，并把对应行数摊出来。
func TestBuildRowsHonorsExpanded(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -40,3 +40,4 @@
 line40
+added
 line41
 line42
`)
	files := []gitrepo.File{f}

	// 先拿到 gapKey
	base := buildRows(files, nil, nil, false)
	var key string
	for _, r := range base {
		if r.kind == rowGap {
			key = r.gapKey
		}
	}
	if key == "" {
		t.Fatal("没有折叠条")
	}

	// 展开 100 行：整个 39 行的区间都该摊开，折叠条消失
	rows := buildRows(files, nil, map[string]int{key: expandStep}, false)
	want := "H" + strings.Repeat("L", 39) + "@LLLL"
	if got := shape(rows); got != want {
		t.Errorf("全部展开后 = %s, want %s", got, want)
	}
	for _, r := range rows {
		if r.kind == rowGap {
			t.Error("全部展开后不该还有折叠条")
		}
	}
	// 摊开的行行号必须是 1..39
	n := 0
	for _, r := range rows {
		if r.kind == rowLine && r.fromGap {
			n++
			if r.newNo != n {
				t.Errorf("第 %d 条展开行的新行号 = %d, want %d", n, r.newNo, n)
			}
		}
	}
	if n != 39 {
		t.Errorf("展开行数 = %d, want 39", n)
	}
}

// 展开一部分时：摊开一部分，剩下的仍然折成条。
func TestBuildRowsPartialExpand(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -40,3 +40,4 @@
 line40
+added
 line41
 line42
`)
	files := []gitrepo.File{f}
	base := buildRows(files, nil, nil, false)
	key := ""
	for _, r := range base {
		if r.kind == rowGap {
			key = r.gapKey
		}
	}

	rows := buildRows(files, nil, map[string]int{key: 10}, false)
	gaps := 0
	lines := 0
	for _, r := range rows {
		if r.kind == rowGap {
			gaps++
			if r.gapN != 29 {
				t.Errorf("剩余折叠行数 = %d, want 29", r.gapN)
			}
		}
		if r.kind == rowLine && r.fromGap {
			lines++
		}
	}
	if gaps != 1 || lines != 10 {
		t.Errorf("展开 10 行后：折叠条 %d 个、展开行 %d 行，want 1 / 10", gaps, lines)
	}
}

// 知道文件总行数时，末尾那段未改动区间也要折出来。
func TestBuildRowsTrailingGap(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 package main
+// added
 func main() {}
`)
	// 文件共 30 行
	content := &fileContent{lines: make([]string, 30)}
	rows := buildRows([]gitrepo.File{f}, map[string]*fileContent{"a.go": content}, nil, false)

	if got := shape(rows); got != "H@LLLG" {
		t.Fatalf("行结构 = %s, want H@LLLG", got)
	}
	last := rows[len(rows)-1]
	// hunk 覆盖到新侧第 3 行，所以后面还有 27 行
	if last.gapN != 27 {
		t.Errorf("末尾折叠行数 = %d, want 27", last.gapN)
	}
}

// 纯删除的 hunk：git 把新侧起始行号写成插入点之前那一行，
// 行号换算必须照顾到这个约定，否则未改动区间会算错。
func TestBuildRowsPureDeletion(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -5,3 +4,0 @@
 gone1
 gone2
 gone3
`)
	rows := buildRows([]gitrepo.File{f}, nil, nil, false)

	// 新侧第 4 行之后没有内容了，所以折叠区间是第 1-4 行（4 行，走内联）
	if got := shape(rows); got != "HLLLL@LLL" {
		t.Fatalf("行结构 = %s, want HLLLL@LLL", got)
	}
	if rows[1].oldNo != 1 || rows[1].newNo != 1 {
		t.Errorf("首行行号 = %d/%d, want 1/1", rows[1].oldNo, rows[1].newNo)
	}
}

// 删除行让新侧比旧侧少，之后未改动区间的旧行号要跟着偏移。
func TestBuildRowsGapLineNumbersAccountForDelta(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,2 @@
 keep
-gone
 keep2
@@ -8,2 +7,2 @@
 a
+b
`)
	rows := buildRows([]gitrepo.File{f}, nil, nil, false)

	// 两个 hunk 之间的区间是新的 3-6 行（4 行，走内联展开）
	var gapLines []row
	for _, r := range rows {
		if r.kind == rowLine && r.fromGap {
			gapLines = append(gapLines, r)
		}
	}
	if len(gapLines) != 4 {
		t.Fatalf("展开的区间行数 = %d, want 4", len(gapLines))
	}

	// 旧文件删掉了第 2 行，所以从第 4 行起新侧比旧侧少 1：
	//   新 3 行 ↔ 旧 4 行
	if got := gapLines[0]; got.newNo != 3 || got.oldNo != 4 {
		t.Errorf("首行行号 = new %d old %d, want new 3 old 4", got.newNo, got.oldNo)
	}
	if got := gapLines[3]; got.newNo != 6 || got.oldNo != 7 {
		t.Errorf("末行行号 = new %d old %d, want new 6 old 7", got.newNo, got.oldNo)
	}
}

func TestBuildRowsBinaryNote(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/logo.png b/logo.png
index 111..222 100644
Binary files a/logo.png and b/logo.png differ
`)
	f.Binary = true
	rows := buildRows([]gitrepo.File{f}, nil, nil, false)

	if got := shape(rows); got != "HN" {
		t.Fatalf("行结构 = %s, want HN", got)
	}
	if !strings.Contains(rows[1].note, "二进制") {
		t.Errorf("提示文字 = %q", rows[1].note)
	}
}

// ListState.Key 要求全局唯一，重复 key 会让无头测试直接 panic，
// 所以这里也顺手把唯一性钉住。
func TestBuildRowsKeysAreUnique(t *testing.T) {
	files := []gitrepo.File{
		fileFromPatch(t, simplePatch),
		fileFromPatch(t, `diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -40,3 +40,4 @@
 x
+y
 z
 w
`),
	}
	content := &fileContent{lines: make([]string, 200)}
	rows := buildRows(files, map[string]*fileContent{"b.go": content}, map[string]int{"g:b.go:38": 5}, false)

	seen := map[string]bool{}
	for i, r := range rows {
		if r.key == "" {
			t.Errorf("第 %d 行没有 key（kind=%d）", i, r.kind)
			continue
		}
		if seen[r.key] {
			t.Errorf("key 重复：%s", r.key)
		}
		seen[r.key] = true
	}
}

func TestBuildRowsMultipleFiles(t *testing.T) {
	files := []gitrepo.File{
		fileFromPatch(t, simplePatch),
		fileFromPatch(t, `diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,1 +1,2 @@
 package b
+// x
`),
	}
	rows := buildRows(files, nil, nil, false)

	if got := shape(rows); got != "H@LLLH@LL" {
		t.Errorf("行结构 = %s, want H@LLLH@LL", got)
	}
	// 两个文件的头部行 file 下标应该是 0 和 1
	if rows[0].file != 0 || rows[5].file != 1 {
		t.Errorf("文件下标 = %d / %d, want 0 / 1", rows[0].file, rows[5].file)
	}
}

// 行号换算的规则住在 internal/diff 里（gitrepo 还原旧文件时也要用同一套），
// 这里只钉住界面依赖的那几个约定。
func TestHunkNewStartAndEnd(t *testing.T) {
	cases := []struct {
		h       diff.Hunk
		start   int
		end     int
		explain string
	}{
		{diff.Hunk{NewStart: 10, NewLines: 5}, 10, 14, "普通 hunk"},
		{diff.Hunk{NewStart: 4, NewLines: 0}, 5, 4, "纯删除：覆盖起点在插入点之后"},
		{diff.Hunk{NewStart: 1, NewLines: 1}, 1, 1, "单行"},
	}
	for _, c := range cases {
		if got := c.h.NewStartLine(); got != c.start {
			t.Errorf("%s: hunkNewStart = %d, want %d", c.explain, got, c.start)
		}
		if got := c.h.NewEndLine(); got != c.end {
			t.Errorf("%s: hunkNewEnd = %d, want %d", c.explain, got, c.end)
		}
	}
}
