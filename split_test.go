package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

func pairsOf(t *testing.T, patch string) []pair {
	t.Helper()
	f := diff.Parse(patch)
	if len(f) == 0 {
		t.Fatalf("解析不出文件：%q", patch)
	}
	return pairHunk(f[0].Hunks[0])
}

func pairShape(ps []pair) string {
	var b []string
	for _, p := range ps {
		b = append(b, strings.Join([]string{itoaOrDash(p.left), itoaOrDash(p.right)}, "/"))
	}
	return strings.Join(b, " ")
}

func itoaOrDash(n int) string {
	if n < 0 {
		return "-"
	}
	return string(rune('0' + n))
}

// 改一行显示成一行：删除和紧跟其后的新增配对。
func TestPairHunkMatchesDelWithAdd(t *testing.T) {
	got := pairsOf(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
`)
	if pairShape(got) != "0/1" {
		t.Errorf("配对结果 = %s, want 0/1", pairShape(got))
	}
}

// 删得多、加得少：多出来的删除行在右侧留空。
func TestPairHunkLeavesExtraSidesEmpty(t *testing.T) {
	got := pairsOf(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,2 @@
-gone1
-gone2
+new
 keep
`)
	// keep 是未改动行，两侧都占；前面两删一增配成两行，第二行右侧空着
	if pairShape(got) != "0/2 1/- 3/3" {
		t.Errorf("配对结果 = %s, want 0/2 1/- 3/3", pairShape(got))
	}
}

// 未改动行把前后两段增删隔开，不能跨过去配对 ——
// 否则「删掉的这行」和「很远之后加的那行」会被摆成同一行，读起来全错。
func TestPairHunkDoesNotPairAcrossContext(t *testing.T) {
	got := pairsOf(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
-gone
 keep
+added
`)
	if pairShape(got) != "0/- 1/1 -/2" {
		t.Errorf("配对结果 = %s, want 0/- 1/1 -/2", pairShape(got))
	}
}

// 全是新增（新文件）：每一行右边有内容、左边留空。
func TestPairHunkAllAdds(t *testing.T) {
	got := pairsOf(t, `diff --git a/a.go b/a.go
--- /dev/null
+++ b/a.go
@@ -0,0 +1,3 @@
+one
+two
+three
`)
	if pairShape(got) != "-/0 -/1 -/2" {
		t.Errorf("配对结果 = %s, want -/0 -/1 -/2", pairShape(got))
	}
}

// split 模式下每条行的 key 仍然必须唯一（ListState 靠它认行）。
// 而且 key 刻意取自 unified 那几个 key 之一：切换视图时列表按 key 找回
// 原来的位置，滚动位置才不会跳。
func TestSplitRowsKeysAreUnique(t *testing.T) {
	files := []gitrepo.File{
		fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
-gone1
-gone2
+new
 keep
`),
	}
	content := &fileContent{lines: make([]string, 40)}
	contents := map[string]*fileContent{"a.go": content}

	unified := map[string]bool{}
	for _, r := range buildRows(files, contents, nil, false, nil) {
		unified[r.key] = true
	}

	seen := map[string]bool{}
	for _, r := range buildRows(files, contents, nil, true, nil) {
		if r.key == "" {
			t.Errorf("有行没有 key（kind=%d）", r.kind)
			continue
		}
		if seen[r.key] {
			t.Errorf("key 重复：%s", r.key)
		}
		seen[r.key] = true
		if !unified[r.key] {
			t.Errorf("split 的 key 在 unified 里找不到，切换视图时位置会跳：%s", r.key)
		}
	}
}

// split 视图里左右两边的文字都要真的画出来。
func TestSplitViewShowsBothSides(t *testing.T) {
	a := richApp(t)
	a.split = true
	tt := ui.NewTester(a.view, 1280, 860)

	for _, want := range []string{
		"line 52: CHANGED TOO", // 旧侧
		"line 52: 也改了",         // 新侧
	} {
		if !tt.HasText(want) {
			t.Errorf("split 视图里找不到 %q", want)
		}
	}
}

// 工具栏上的 Unified / Split 能切换，而且切完行模型要重建 ——
// 不重建的话界面上还是老样子，看起来就像按钮坏了。
func TestSplitToggleRebuildsRows(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if a.split {
		t.Fatal("默认应该是 unified")
	}
	if err := tt.Click("Split"); err != nil {
		t.Fatalf("点 Split：%v", err)
	}
	tt.Frame()

	if !a.split {
		t.Fatal("点了 Split 之后 a.split 还是 false")
	}
	if a.rowsDirty {
		t.Error("切完视图应该已经重建了行模型")
	}
	// split 下同一行只画一次：旧侧的删除行和新侧的新增行并成一条，
	// 所以总行数比 unified 少
	unified := buildRows(a.files, a.contents, a.expanded, false, nil)
	if len(a.rows) >= len(unified) {
		t.Errorf("split 的行数 = %d，unified = %d，配对没生效", len(a.rows), len(unified))
	}
}
