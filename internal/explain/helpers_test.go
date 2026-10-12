package explain

import (
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
