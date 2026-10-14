package diff

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// rangeLines 造 n 行、每行名字可辨认的内容（第 i 行 = l<i>，1 基），
// 期望值里能一眼看出上下文取到了哪几行。
func rangeLines(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = fmt.Sprintf("l%d", i+1)
	}
	return out
}

// rangeText 把行拼回一份「每行都有末尾换行」的原文。
func rangeText(lines []string) string { return strings.Join(lines, "\n") + "\n" }

// 两处相距很远的改动，只生成第二处的补丁：patch 里必须只有第二处，
// 上下文只能取未改动的那几行。
func TestRangePatchMiddleSegmentOnly(t *testing.T) {
	old := rangeLines(20)
	mod := slices.Clone(old)
	mod[2] = "X3"
	mod[14] = "X15"

	got := RangePatch("f.txt", rangeText(old), rangeText(mod), HunkRange{OldStart: 14, OldEnd: 15, NewStart: 14, NewEnd: 15})
	want := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -12,7 +12,7 @@
 l12
 l13
 l14
-l15
+X15
 l16
 l17
 l18
`
	if got != want {
		t.Errorf("第二段补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// 反过来只生成第一处：尾部的上下文同样不能把后面的改动卷进来。
func TestRangePatchFirstSegmentOnly(t *testing.T) {
	old := rangeLines(20)
	mod := slices.Clone(old)
	mod[2] = "X3"
	mod[14] = "X15"

	got := RangePatch("f.txt", rangeText(old), rangeText(mod), HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3})
	want := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,6 +1,6 @@
 l1
 l2
-l3
+X3
 l4
 l5
 l6
`
	if got != want {
		t.Errorf("第一段补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// 纯新增：旧侧是空区间，头写 -<start>,0。
func TestRangePatchPureInsertion(t *testing.T) {
	old := rangeLines(20)
	ins := slices.Insert(slices.Clone(old), 5, "N1", "N2")

	got := RangePatch("f.txt", rangeText(old), rangeText(ins), HunkRange{OldStart: 5, OldEnd: 5, NewStart: 5, NewEnd: 7})
	want := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -3,6 +3,8 @@
 l3
 l4
 l5
+N1
+N2
 l6
 l7
 l8
`
	if got != want {
		t.Errorf("纯新增补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// 纯删除：新侧是空区间，头写 +<start>,0。
func TestRangePatchPureDeletion(t *testing.T) {
	old := rangeLines(20)
	del := slices.Concat(old[:5], old[7:])

	got := RangePatch("f.txt", rangeText(old), rangeText(del), HunkRange{OldStart: 5, OldEnd: 7, NewStart: 5, NewEnd: 5})
	want := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -3,8 +3,6 @@
 l3
 l4
 l5
-l6
-l7
 l8
 l9
 l10
`
	if got != want {
		t.Errorf("纯删除补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// 文件开头、结尾的改动：另一侧没有行可取时上下文只有 1、2 行，不能取成负数，
// 头里的起点也得跟着缩。
func TestRangePatchContextClippedAtEdges(t *testing.T) {
	old := rangeLines(4) // l1..l4
	first := slices.Clone(old)
	first[0] = "H1"
	last := slices.Clone(old)
	last[3] = "T4"

	gotHead := RangePatch("f.txt", rangeText(old), rangeText(first), HunkRange{OldStart: 0, OldEnd: 1, NewStart: 0, NewEnd: 1})
	wantHead := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,4 +1,4 @@
-l1
+H1
 l2
 l3
 l4
`
	if gotHead != wantHead {
		t.Errorf("开头改动补丁不对：\n--- got ---\n%s--- want ---\n%s", gotHead, wantHead)
	}

	gotTail := RangePatch("f.txt", rangeText(old), rangeText(last), HunkRange{OldStart: 3, OldEnd: 4, NewStart: 3, NewEnd: 4})
	wantTail := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,4 +1,4 @@
 l1
 l2
 l3
-l4
+T4
`
	if gotTail != wantTail {
		t.Errorf("结尾改动补丁不对：\n--- got ---\n%s--- want ---\n%s", gotTail, wantTail)
	}
}

// 末尾缺换行：缺失的那一侧，它最后一条补丁行后面要跟标记，两侧各自判断。
func TestRangePatchNoFinalNewline(t *testing.T) {
	cases := []struct {
		name     string
		old, new string
		hr       HunkRange
		want     string
	}{
		{
			name: "两侧都缺（改的正好是最后一行）",
			old:  "a\nb\nc",
			new:  "a\nb\nC",
			hr:   HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3},
			want: `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,3 @@
 a
 b
-c
` + noNewlineMark + `
+C
` + noNewlineMark + `
`,
		},
		{
			name: "只有新侧缺（旧侧那份末尾有换行）",
			old:  "a\nb\nc\n",
			new:  "a\nb\nC",
			hr:   HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3},
			want: `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,3 @@
 a
 b
-c
+C
` + noNewlineMark + `
`,
		},
		{
			name: "只有旧侧缺（新侧那份补回了换行）",
			old:  "a\nb\nc",
			new:  "a\nb\nC\n",
			hr:   HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3},
			want: `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,3 @@
 a
 b
-c
` + noNewlineMark + `
+C
`,
		},
		{
			name: "两侧都缺、缺的是共用的上下文尾行（标记只写一次）",
			old:  "a\nb",
			new:  "A\nb",
			hr:   HunkRange{OldStart: 0, OldEnd: 1, NewStart: 0, NewEnd: 1},
			want: `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,2 @@
-a
+A
 b
` + noNewlineMark + `
`,
		},
		{
			name: "在缺换行的尾行前面插入",
			old:  "a\nb",
			new:  "a\nN\nb",
			hr:   HunkRange{OldStart: 1, OldEnd: 1, NewStart: 1, NewEnd: 2},
			want: `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,3 @@
 a
+N
 b
` + noNewlineMark + `
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RangePatch("f.txt", tc.old, tc.new, tc.hr); got != tc.want {
				t.Errorf("补丁不对：\n--- got ---\n%s--- want ---\n%s", got, tc.want)
			}
		})
	}
}

// 新文件：旧侧整份原文为空，按 git 的规矩写 /dev/null + new file mode
// （少了 mode 行 git 会把 /dev/null 当成一个真路径，apply 直接报
// "dev/null: does not exist in index"）。
func TestRangePatchNewFile(t *testing.T) {
	got := RangePatch("n.txt", "", "x\ny\n", HunkRange{OldStart: 0, OldEnd: 0, NewStart: 0, NewEnd: 2})
	want := `diff --git a/n.txt b/n.txt
new file mode 100644
--- /dev/null
+++ b/n.txt
@@ -0,0 +1,2 @@
+x
+y
`
	if got != want {
		t.Errorf("新文件补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}

	// 新文件末尾缺换行：标记跟在 +y 后面。
	got = RangePatch("n.txt", "", "x\ny", HunkRange{OldStart: 0, OldEnd: 0, NewStart: 0, NewEnd: 2})
	want = `diff --git a/n.txt b/n.txt
new file mode 100644
--- /dev/null
+++ b/n.txt
@@ -0,0 +1,2 @@
+x
+y
` + noNewlineMark + `
`
	if got != want {
		t.Errorf("缺换行的新文件补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// 整个文件被删掉：新侧写 /dev/null。
func TestRangePatchDeletedFile(t *testing.T) {
	got := RangePatch("d.txt", "x\ny\n", "", HunkRange{OldStart: 0, OldEnd: 2, NewStart: 0, NewEnd: 0})
	want := `diff --git a/d.txt b/d.txt
deleted file mode 100644
--- a/d.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-x
-y
`
	if got != want {
		t.Errorf("删除文件补丁不对：\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// 越界、区间反了、两侧都空、路径空：一律空串，不能 panic —— 上层靠它发现
// 「点下去那一刻的行号已经对不上了」。
func TestRangePatchRejectsBadRanges(t *testing.T) {
	old := rangeText(rangeLines(5)) // 5 行
	cases := []struct {
		name string
		path string
		old  string
		nw   string
		hr   HunkRange
	}{
		{"旧侧越界", "f.txt", old, old, HunkRange{OldStart: 4, OldEnd: 6, NewStart: 4, NewEnd: 6}},
		{"新侧越界", "f.txt", old, old, HunkRange{OldStart: 4, OldEnd: 5, NewStart: 4, NewEnd: 9}},
		{"负数起点", "f.txt", old, old, HunkRange{OldStart: -1, OldEnd: 1, NewStart: -1, NewEnd: 1}},
		{"终点在起点前", "f.txt", old, old, HunkRange{OldStart: 3, OldEnd: 2, NewStart: 3, NewEnd: 2}},
		{"两侧都是空区间（没有改动）", "f.txt", old, old, HunkRange{OldStart: 2, OldEnd: 2, NewStart: 2, NewEnd: 2}},
		{"路径为空", "", old, old, HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3}},
		{"新文件但区间在文件外", "f.txt", "", "x\n", HunkRange{OldStart: 0, OldEnd: 0, NewStart: 0, NewEnd: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RangePatch(tc.path, tc.old, tc.nw, tc.hr); got != "" {
				t.Errorf("应该返回空串，得到：\n%s", got)
			}
		})
	}
}

// 空文件里插入第一行：头写 -0,0。
func TestRangePatchEmptyOldFile(t *testing.T) {
	got := RangePatch("f.txt", "", "x\n", HunkRange{OldStart: 0, OldEnd: 0, NewStart: 0, NewEnd: 1})
	if !strings.Contains(got, "@@ -0,0 +1,1 @@") {
		t.Errorf("空文件插入的头不对：\n%s", got)
	}
}

// 生成的补丁要能被自家解析器读回一段「和拿到的区间一致」的改动：
// git apply 吃的就是这种形式，Parse 读不回来意味着某处行数对不上。
func TestRangePatchParsesBack(t *testing.T) {
	old := rangeLines(20)
	mod := slices.Clone(old)
	mod[14] = "X15"

	patch := RangePatch("f.txt", rangeText(old), rangeText(mod), HunkRange{OldStart: 14, OldEnd: 15, NewStart: 14, NewEnd: 15})
	files := Parse(patch)
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("补丁应该只有一个文件一个 hunk，实际 %d 个文件", len(files))
	}
	f, h := files[0], files[0].Hunks[0]
	if f.Path() != "f.txt" || f.Status != "modified" {
		t.Errorf("文件 = %q status = %q，want f.txt/modified", f.Path(), f.Status)
	}
	if h.OldStart != 12 || h.OldLines != 7 || h.NewStart != 12 || h.NewLines != 7 {
		t.Errorf("hunk 头 = -%d,%d +%d,%d，want -12,7 +12,7", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	}
	if f.DelCount != 1 || f.AddCount != 1 {
		t.Errorf("增删计数 = +%d/-%d，want +1/-1", f.AddCount, f.DelCount)
	}
}

// 末端换行的原文不该多出空行：最后一行是 l20，不是空串。
func TestSplitLinesKeepsFinalNewlineHonest(t *testing.T) {
	cases := []struct {
		in    string
		lines int
		nl    bool
	}{
		{"", 0, true},
		{"a\n", 1, true},
		{"a", 1, false},
		{"a\nb\n", 2, true},
		{"a\nb", 2, false},
		{"a\n\n", 2, true},
	}
	for _, tc := range cases {
		lines, nl := splitLines(tc.in)
		if len(lines) != tc.lines || nl != tc.nl {
			t.Errorf("splitLines(%q) = %q,%v; want %d 行, 末尾换行 %v", tc.in, lines, nl, tc.lines, tc.nl)
		}
	}
}

// 相邻的另一处改动不能当成上下文：DiffView 的两段可以只隔一两行，那几行
// 对其中一段是上下文、对另一段却是改动本身。拿旧侧的那几行当上下文写进
// 补丁，反向 apply（撤出 index）会对不上 —— 目标文件上它已经不是这个内容。
func TestRangePatchContextStopsAtNeighbourChange(t *testing.T) {
	oldText := "a\nb\nc\nd\ne\nf\ng\n"
	newText := "a\nB\nc\nd\ne\nF\ng\n" // 第 2 行与第 6 行各改了一处
	// 前一处：旧侧 [1,2) → 新侧 [1,2)（0 基）
	p := RangePatch("f.txt", oldText, newText, HunkRange{OldStart: 1, OldEnd: 2, NewStart: 1, NewEnd: 2})
	if p == "" {
		t.Fatal("该生成一段补丁")
	}
	if strings.Contains(p, " f") {
		t.Errorf("f 那一行在新侧是改过的，不能当上下文写进补丁：\n%s", p)
	}
	if !strings.Contains(p, "@@ -1,5 +1,5 @@") {
		t.Errorf("上下文该只取到第 5 行（e），头是 @@ -1,5 +1,5 @@：\n%s", p)
	}

	// 后一处反过来：它前面那一行（e）两侧相同、可以当上下文，再往前
	// （d、c）也相同，但 b/B 那一行不同，绝不能越过它往更前取。
	p = RangePatch("f.txt", oldText, newText, HunkRange{OldStart: 5, OldEnd: 6, NewStart: 5, NewEnd: 6})
	if p == "" {
		t.Fatal("该生成一段补丁")
	}
	if strings.Contains(p, " B") || strings.Contains(p, "-b") {
		t.Errorf("b 那一行是另一处改动，不该出现在这一段补丁里：\n%s", p)
	}
	if !strings.Contains(p, "@@ -3,5 +3,5 @@") {
		t.Errorf("往前的上下文该停在 b 之前（第 3 行 c），头是 @@ -3,5 +3,5 @@：\n%s", p)
	}
}
