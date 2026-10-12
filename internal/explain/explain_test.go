package explain

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

func parseOne(t *testing.T, patch string) diff.FileDiff {
	t.Helper()
	fs := diff.Parse(patch)
	if len(fs) != 1 {
		t.Fatalf("解析出 %d 个文件，want 1", len(fs))
	}
	return fs[0]
}

const twoHunks = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 one
+inserted
 two
 three
@@ -20,3 +21,3 @@ func main() {
 old
-gone
+new
 tail
`

// 提示词里必须点名「要解释的是第几个」，否则同文件多个变更块时模型会
// 把整个文件讲一遍。
func TestBuildPromptNamesTheHunk(t *testing.T) {
	f := parseOne(t, twoHunks)
	_, user := BuildPrompt(Request{File: f, Hunk: f.Hunks[1], HunkIdx: 1})

	for _, want := range []string{
		"该文件共有 2 个变更块",
		"要给解释的是第 2 个",
		"@@ -1,3 +1,4 @@", // 另一个块的位置也要列出来
		"a.go",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("提示词里缺 %q：\n%s", want, user)
		}
	}
	if strings.Contains(user, "第 1 个") {
		t.Errorf("把当前块说成了第 1 个：\n%s", user)
	}
}

// 只解释一个变更块：正文里不能混进同文件其它块的内容。
func TestBuildPromptOnlyIncludesTheGivenHunk(t *testing.T) {
	f := parseOne(t, twoHunks)
	_, user := BuildPrompt(Request{File: f, Hunk: f.Hunks[0], HunkIdx: 0})

	body := user[strings.Index(user, "## 需要解释的变更块"):]
	if !strings.Contains(body, "+inserted") {
		t.Errorf("正文里没有这个块的增行：\n%s", body)
	}
	if strings.Contains(body, "+new") {
		t.Errorf("正文里混进了另一个块的增行：\n%s", body)
	}
}

// 重命名的文件两个路径都要写出来，否则模型看到的是个陌生的路径。
func TestBuildPromptShowsRename(t *testing.T) {
	f := parseOne(t, `diff --git a/old.go b/new.go
similarity index 90%
rename from old.go
rename to new.go
--- a/old.go
+++ b/new.go
@@ -1 +1 @@
-a
+b
`)
	_, user := BuildPrompt(Request{File: f, Hunk: f.Hunks[0], HunkIdx: 0})
	if !strings.Contains(user, "old.go → new.go") {
		t.Errorf("提示词里没有重命名：\n%s", user)
	}
	if !strings.Contains(user, "重命名") {
		t.Errorf("提示词里没有说是重命名：\n%s", user)
	}
}

// Context 非空时要带上，并且标出这是带行号的上下文。
func TestBuildPromptIncludesContext(t *testing.T) {
	f := parseOne(t, twoHunks)
	_, user := BuildPrompt(Request{
		File:    f,
		Hunk:    f.Hunks[0],
		HunkIdx: 0,
		Context: "1| one\n2| two",
	})
	if !strings.Contains(user, "1| one") || !strings.Contains(user, "真实行号") {
		t.Errorf("上下文没带上或没说清行号：\n%s", user)
	}
}

// 缓存 key 必须跟着内容走：换个位置、换个模型都要变。
func TestCacheKeyTracksContentAndModel(t *testing.T) {
	f := parseOne(t, twoHunks)
	a := CacheKey(f, f.Hunks[0], "m1")

	if a != CacheKey(f, f.Hunks[0], "m1") {
		t.Error("同一个块同一个模型，key 应该稳定")
	}
	if a == CacheKey(f, f.Hunks[1], "m1") {
		t.Error("同一文件的不同块不该共用缓存")
	}
	if a == CacheKey(f, f.Hunks[0], "m2") {
		t.Error("不同模型不该共用缓存")
	}
}

// key 覆盖的是模型看到的那份文本，@@ 行里的行号也在其中 ——
// 同一处改动挪到别的行号就是另一份提示词，缓存不该混用。
func TestCacheKeyIncludesHunkHeader(t *testing.T) {
	first := parseOne(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
`)
	later := parseOne(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -900 +900 @@
-old
+new
`)
	if CacheKey(first, first.Hunks[0], "m") == CacheKey(later, later.Hunks[0], "m") {
		t.Error("行号不同的两个块撞了同一个 key，会把行号写进提示词的缓存混在一起")
	}
}

func TestWindowAroundHunk(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "L" + string(rune('0'+i%10))
	}
	// hunk 在新侧第 50 行起、改 3 行
	h := diff.Hunk{OldStart: 50, OldLines: 3, NewStart: 50, NewLines: 3}

	got := Window(lines, h, 2, 1<<20)
	if !strings.Contains(got, "48| ") || !strings.Contains(got, "54| ") {
		t.Errorf("上下文范围不对：\n%s", got)
	}
	if strings.Contains(got, "47| ") {
		t.Errorf("多截了一行：\n%s", got)
	}
	if !strings.Contains(got, "省略 47 行") || !strings.Contains(got, "省略 45 行") {
		t.Errorf("截断提示不对：\n%s", got)
	}
}

// 纯删除的 hunk 在新侧没有行号，得按旧侧算窗口，否则上下文会跑到文件头上去。
func TestWindowPureDeletionUsesOldSide(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "x"
	}
	// 删掉旧文件第 30-31 行，新侧插入点是第 29 行之后
	h := diff.Hunk{OldStart: 30, OldLines: 2, NewStart: 29, NewLines: 0}

	got := Window(lines, h, 2, 1<<20)
	if !strings.Contains(got, "28| ") || !strings.Contains(got, "31| ") {
		t.Errorf("窗口没落在删除位置附近：\n%s", got)
	}
}

// 上下文超过上限时按行截断，不能切出半行。
func TestWindowTruncatesAtLineBoundary(t *testing.T) {
	lines := make([]string, 500)
	for i := range lines {
		lines[i] = strings.Repeat("y", 40)
	}
	h := diff.Hunk{OldStart: 250, OldLines: 1, NewStart: 250, NewLines: 1}

	got := Window(lines, h, 200, 300)
	if len(got) > 400 {
		t.Errorf("截断后还是太长（%d 字节）：\n%s", len(got), got)
	}
	if !strings.Contains(got, "已截断") {
		t.Error("没有说明被截断了")
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "y") && len(line) < 40 && !strings.Contains(got, "已截断") {
			t.Errorf("切出了半行：%q", line)
		}
	}
}

func TestLangOf(t *testing.T) {
	cases := map[string]string{
		"main.go":         "go",
		"web/App.vue":     "vue",
		"a/b/c.rs":        "rust",
		"Makefile":        "",
		"README":          "",
		"x.PY":            "python",
		"dir.with.dots/f": "",
	}
	for path, want := range cases {
		if got := Lang(path); got != want {
			t.Errorf("Lang(%q) = %q, want %q", path, got, want)
		}
	}
}

// 系统提示是模型行为的唯一约束，改它多半是有意的；这里钉住几条关键要求，
// 免得手滑删掉。
func TestSystemPromptKeepsKeyConstraints(t *testing.T) {
	for _, want := range []string{"Only explain the single hunk", "Chinese", "250 words"} {
		if !strings.Contains(SystemPrompt, want) {
			t.Errorf("System prompt missing %q", want)
		}
	}
}
