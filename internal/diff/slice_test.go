package diff

import (
	"strings"
	"testing"
)

// firstHunkID 用 Parse 自己算出 raw 里 path 首个 hunk 的 ID —— 切片的对错
// 以「和解析模型对得上」为准，而不是写死哈希。
func firstHunkID(t *testing.T, raw, path string) string {
	t.Helper()
	for _, f := range Parse(raw) {
		if f.Path() == path && len(f.Hunks) > 0 {
			return f.Hunks[0].ID
		}
	}
	t.Fatalf("Parse 结果里找不到 %s 的 hunk", path)
	return ""
}

// 每个 hunk 切出来都能单独解析回同一个 ID —— 这是 git apply 能吃的形式。
func assertSliceRoundTrips(t *testing.T, raw, path, want, id string) {
	t.Helper()
	got, ok := SliceHunk(raw, path, id)
	if !ok {
		t.Fatalf("SliceHunk(%s) 没找到", path)
	}
	if got != want {
		t.Errorf("切片不是字节原样：\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	files := Parse(got)
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("切出来的 patch 应该只有一个文件一个 hunk，实际 %d 个文件", len(files))
	}
	if gotID := files[0].Hunks[0].ID; gotID != id {
		t.Errorf("切出来的 hunk ID = %s, want %s", gotID, id)
	}
}

// 多 hunk 的文件：切第二个，文件头要带上、第一个 hunk 不能混进来。
func TestSliceHunkSecondOfTwo(t *testing.T) {
	raw := `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
@@ -10,3 +10,4 @@ func main() {
 ten
 eleven
+twelve
 twelve
`
	want := `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -10,3 +10,4 @@ func main() {
 ten
 eleven
+twelve
 twelve
`
	ids := Parse(raw)[0].Hunks
	if len(ids) != 2 {
		t.Fatalf("测试夹具该有两个 hunk，实际 %d", len(ids))
	}
	assertSliceRoundTrips(t, raw, "f.txt", want, ids[1].ID)
}

// "\ No newline at end of file" 必须原样带上 —— Parse 会丢它，拼 Raw() 就会
// 少了这两行，git apply 对缺尾换行的文件会处理错。
func TestSliceHunkKeepsNoNewlineMarkers(t *testing.T) {
	raw := "diff --git a/f.txt b/f.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" one\n" +
		"-two\n" +
		"\\ No newline at end of file\n" +
		"+two!\n" +
		"\\ No newline at end of file\n"
	id := firstHunkID(t, raw, "f.txt")
	got, ok := SliceHunk(raw, "f.txt", id)
	if !ok {
		t.Fatal("没切出来")
	}
	if n := strings.Count(got, `\ No newline at end of file`); n != 2 {
		t.Errorf("切出来的 patch 里标记出现 %d 次，want 2:\n%s", n, got)
	}
	assertSliceRoundTrips(t, raw, "f.txt", raw, id)
}

// 两个文件：切 b 的时候必须从 b 的文件头开始，a 的头和 hunk 都不能混进来。
func TestSliceHunkSecondFile(t *testing.T) {
	raw := `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
diff --git a/b.txt b/b.txt
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/b.txt
@@ -0,0 +1,2 @@
+hello
+world
`
	want := `diff --git a/b.txt b/b.txt
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/b.txt
@@ -0,0 +1,2 @@
+hello
+world
`
	id := firstHunkID(t, raw, "b.txt")
	assertSliceRoundTrips(t, raw, "b.txt", want, id)
}

// hunk 内容里出现「顶格像是文件头」的文本不能把块切断：内容行永远带前缀，
// 真正顶格的 diff --git 才是新块。
func TestSliceHunkContentLooksLikeHeader(t *testing.T) {
	raw := `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,3 @@
 keep
+diff --git a/evil b/evil
 tail
`
	id := firstHunkID(t, raw, "f.txt")
	got, ok := SliceHunk(raw, "f.txt", id)
	if !ok {
		t.Fatal("没切出来")
	}
	if !strings.Contains(got, "+diff --git a/evil b/evil") {
		t.Errorf("内容行被切丢了：\n%s", got)
	}
	assertSliceRoundTrips(t, raw, "f.txt", raw, id)
}

// CRLF 内容的 \r 在行内，切片要按字节保留。
func TestSliceHunkCRLFKept(t *testing.T) {
	raw := "diff --git a/f.txt b/f.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" one\r\n" +
		"-two\r\n" +
		"+TWO\r\n"
	id := firstHunkID(t, raw, "f.txt")
	assertSliceRoundTrips(t, raw, "f.txt", raw, id)
}

func TestSliceHunkNotFound(t *testing.T) {
	raw := `diff --git a/f.txt b/f.txt
index 1111111..2222222 100644
--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
`
	id := firstHunkID(t, raw, "f.txt")
	if _, ok := SliceHunk(raw, "f.txt", "deadbeefdeadbeef"); ok {
		t.Error("hunkID 不存在时该返回 false")
	}
	if _, ok := SliceHunk(raw, "other.txt", id); ok {
		t.Error("路径不存在时该返回 false")
	}

	binary := "diff --git a/img.png b/img.png\nindex 1111111..2222222 100644\nBinary files a/img.png and b/img.png differ\n"
	if _, ok := SliceHunk(binary, "img.png", "whatever"); ok {
		t.Error("二进制文件没有 hunk，该返回 false")
	}
}

// 反过来的性质：对一份 patch 里的每个 hunk 逐一切片，都能切到、都不串。
func TestSliceHunkEveryHunkOfEveryFile(t *testing.T) {
	raw := `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
@@ -20,3 +20,4 @@
 twenty
+twenty-one
 twenty-two
 twenty-three
diff --git a/b.txt b/b.txt
index 4444444..5555555 100644
--- a/b.txt
+++ b/b.txt
@@ -5,2 +5,2 @@
 five
-six
+SIX
`
	for _, f := range Parse(raw) {
		for _, h := range f.Hunks {
			got, ok := SliceHunk(raw, f.Path(), h.ID)
			if !ok {
				t.Fatalf("%s 的 hunk %s 没切到", f.Path(), h.ID)
			}
			one := Parse(got)
			if len(one) != 1 || len(one[0].Hunks) != 1 || one[0].Hunks[0].ID != h.ID {
				t.Errorf("%s/%s 切出来的不是同一段:\n%s", f.Path(), h.ID, got)
			}
			if one[0].Path() != f.Path() {
				t.Errorf("切出来的文件是 %s, want %s", one[0].Path(), f.Path())
			}
		}
	}
}
