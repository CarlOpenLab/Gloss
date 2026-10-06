package gitrepo

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// 新侧 8 行，旧侧 8 行；第一个 hunk 删掉一行，第二个 hunk 加了一行。
const oldLinesPatch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,2 @@
 keep
-gone
 keep2
@@ -7,2 +6,3 @@
 f
+NEW
 g
`

func TestOldLinesRebuildsDeletedLine(t *testing.T) {
	f := diff.Parse(oldLinesPatch)[0]
	newLines := []string{"keep", "keep2", "b", "c", "d", "f", "NEW", "g"}

	got := OldLines(f, newLines)
	want := []string{"keep", "gone", "keep2", "b", "c", "d", "f", "g"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("还原出的旧文件 = %q, want %q", got, want)
	}
}

// 没改动的部分必须原样搬过来，长度也要对 —— 高亮按行号取内容，
// 长度错了会把颜色涂到别的行上。
func TestOldLinesLengthMatchesHunks(t *testing.T) {
	f := diff.Parse(oldLinesPatch)[0]
	newLines := []string{"keep", "keep2", "b", "c", "d", "f", "NEW", "g"}

	got := OldLines(f, newLines)
	// 旧侧 = 新侧 − 新增行 + 删除行 = 8 − 1 + 1
	if len(got) != 8 {
		t.Fatalf("旧文件行数 = %d, want 8", len(got))
	}
}

// 纯删除的 hunk：新侧行数为 0，头里的起始行号是插入点之前那一行。
// 头本身取自真实 git 输出（删掉 7 行里的第 3 行，用 -U0 能看到）。
func TestOldLinesPureDeletion(t *testing.T) {
	f := diff.Parse(`diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -3 +2,0 @@
-c
`)[0]
	newLines := []string{"a", "b", "d", "e", "f", "g"}

	got := OldLines(f, newLines)
	want := []string{"a", "b", "c", "d", "e", "f", "g"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("还原出的旧文件 = %q, want %q", got, want)
	}
}

// 纯新增的 hunk：旧侧行数为 0（在旧文件第 1 行之后插入）。
func TestOldLinesPureAddition(t *testing.T) {
	f := diff.Parse(`diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,0 +2 @@
+NEW
`)[0]
	newLines := []string{"a", "NEW", "c", "d", "e", "f", "g"}

	got := OldLines(f, newLines)
	want := []string{"a", "c", "d", "e", "f", "g"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("还原出的旧文件 = %q, want %q", got, want)
	}
}

// 拿真 git 生成的 diff 走一遍：还原出来的必须和提交进去的那份一模一样。
// 这条覆盖 -U3 下的常规 hunk（带上下文行），也是实际会遇到的那种。
func TestOldLinesRoundTrip(t *testing.T) {
	r := tempRepo(t)
	const before = "one\ntwo\nthree\n"
	const after = "one\ntwo changed\nthree\nfour\n"

	write(t, filepath.Join(r.Root, "a.txt"), after)
	m, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var f *diff.FileDiff
	for i := range m.Files {
		if m.Files[i].NewPath == "a.txt" {
			f = &m.Files[i].FileDiff
		}
	}
	if f == nil {
		t.Fatal("没找到 a.txt 的变更")
	}

	got := OldLines(*f, strings.Split(strings.TrimSuffix(after, "\n"), "\n"))
	want := strings.Split(strings.TrimSuffix(before, "\n"), "\n")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("还原出的旧文件 = %q, want %q", got, want)
	}
}

func TestOldLinesGivesUpOnMismatch(t *testing.T) {
	f := diff.Parse(oldLinesPatch)[0]
	// 新侧内容比 hunk 声称的还短，说明我们对文件的理解不对 —— 宁可放弃
	if got := OldLines(f, []string{"keep"}); got != nil {
		t.Errorf("对不齐时应该返回 nil，实际 %q", got)
	}
	if got := OldLines(diff.FileDiff{NewPath: "a.go"}, []string{"x"}); got != nil {
		t.Errorf("没有 hunk 时应该返回 nil，实际 %q", got)
	}
}
