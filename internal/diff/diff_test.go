package diff

import "testing"

const sample = `commit 9f8e7d6c5b4a3a2a1a0a9a8a7a6a5a4a3a2a1a0a
Author: dev <dev@example.com>
Date:   Thu Oct 2 10:00:00 2025 +0800

    feat: add greeting

diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,7 @@ package main
 import "fmt"

-func main() {
-	fmt.Println("hello")
+func greet(name string) string {
+	return "hello " + name
 }
+
+func main() { fmt.Println(greet("world")) }
diff --git a/util.go b/util.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/util.go
@@ -0,0 +1,3 @@
+package main
+
+func add(a, b int) int { return a + b }
diff --git a/old.go b/old.go
deleted file mode 100644
index 4444444..0000000
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package main
--- this line starts with three dashes, must stay in hunk
`

func TestParse(t *testing.T) {
	files := Parse(sample)
	if len(files) != 3 {
		t.Fatalf("expect 3 files, got %d", len(files))
	}

	m := files[0]
	if m.NewPath != "main.go" || m.Status != "modified" {
		t.Errorf("file0: got %s %s", m.NewPath, m.Status)
	}
	if m.AddCount != 4 || m.DelCount != 2 {
		t.Errorf("file0 counts: +%d -%d", m.AddCount, m.DelCount)
	}
	h := m.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 5 || h.NewStart != 1 || h.NewLines != 7 {
		t.Errorf("hunk range: %+v", h)
	}
	if h.Section != "package main" {
		t.Errorf("section: %q", h.Section)
	}
	// 行号检查
	last := h.Lines[len(h.Lines)-1]
	if last.Type != LineAdd || last.NewNum != 7 {
		t.Errorf("last line: %+v", last)
	}

	a := files[1]
	if a.Status != "added" || a.OldPath != "" || a.NewPath != "util.go" || a.AddCount != 3 {
		t.Errorf("added file: %+v", a)
	}

	d := files[2]
	if d.Status != "deleted" || d.NewPath != "" || d.OldPath != "old.go" {
		t.Errorf("deleted file: %+v", d)
	}
	// hunk 内部以 "--- " 开头的删除行不能被误判成新文件头
	if len(d.Hunks) != 1 || len(d.Hunks[0].Lines) != 2 {
		t.Fatalf("deleted file hunks: %+v", d.Hunks)
	}
	if d.Hunks[0].Lines[1].Content != "-- this line starts with three dashes, must stay in hunk" {
		t.Errorf("tricky del line: %q", d.Hunks[0].Lines[1].Content)
	}
	if d.Hunks[0].Lines[1].Type != LineDel {
		t.Errorf("tricky line type: %s", d.Hunks[0].Lines[1].Type)
	}
}

func TestHunkIDStable(t *testing.T) {
	a := Parse(sample)
	b := Parse(sample)
	if a[0].Hunks[0].ID != b[0].Hunks[0].ID {
		t.Error("hunk ID should be deterministic")
	}
	if a[0].Hunks[0].ID == a[1].Hunks[0].ID {
		t.Error("different hunks should have different IDs")
	}
}
