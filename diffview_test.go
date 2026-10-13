package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

func TestDiffViewShowsCodeLines(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	for _, want := range []string{
		"internal/server/handler.go",
		"line 3: CHANGED",
		"line 3: CHANGED AGAIN",
		"line 52: CHANGED TOO",
		"line 52: 也改了",
		"line 6: 新加的一行 with 中文",
	} {
		if !tt.HasText(want) {
			t.Errorf("diff 里找不到 %q", want)
		}
	}
}

func TestHunkHeaderSectionNotDuplicated(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	const section = "line 48: some code here"
	hits := 0
	for _, s := range tt.Texts() {
		hits += strings.Count(s, section)
	}
	if hits != 1 {
		t.Errorf("hunk 函数名出现了 %d 次，want 1（全文：%q）", hits, tt.Texts())
	}
}

func TestHunkHeaderText(t *testing.T) {
	h := diff.Hunk{
		Header:  "@@ -49,7 +49,7 @@ line 48: some code here",
		Section: "line 48: some code here",
	}
	if got := hunkHeaderText(h); got != "@@ -49,7 +49,7 @@" {
		t.Errorf("hunkHeaderText = %q", got)
	}

	bare := diff.Hunk{Header: "@@ -1,3 +1,4 @@"}
	if got := hunkHeaderText(bare); got != "@@ -1,3 +1,4 @@" {
		t.Errorf("无函数名时 = %q", got)
	}
}

func TestGapExpandsOnClick(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	const bar = "⋯  展开 39 行未改动"
	if !tt.HasText(bar) {
		t.Fatalf("没有看到折叠条，实际文本：%q", tt.Texts())
	}
	if tt.HasText("line 20: some code here") {
		t.Fatal("还没展开就看到了区间里的行")
	}

	if err := tt.Click(bar); err != nil {
		t.Fatalf("点折叠条：%v", err)
	}
	tt.Frame()

	key := ""
	for i := range a.files {
		if pathOf(a.files[i]) == "internal/server/handler.go" {
			key = gapKeyOf(fileKey(a.files[i]), 10)
		}
	}
	if key == "" {
		t.Fatal("文件列表里没有 handler.go")
	}
	if a.expanded[key] != expandStep {
		t.Errorf("展开状态 = %d, want %d", a.expanded[key], expandStep)
	}
	if tt.HasText(bar) {
		t.Error("39 行全摊开后不该还有折叠条")
	}
	if !tt.HasText("line 20: some code here") {
		t.Errorf("展开后应该看到区间里的行，实际：%q", tt.Texts())
	}
}

func TestGapShowsWithoutContents(t *testing.T) {
	a := richApp(t)
	a.contents = nil
	a.rowsDirty = true

	tt := ui.NewTester(a.view, 1280, 860)
	if !tt.HasText("⋯  展开 39 行未改动") {
		t.Errorf("没有文件内容时也该显示折叠条，实际：%q", tt.Texts())
	}
}

func TestTrailingGapAfterContentsLoad(t *testing.T) {
	a := richApp(t)
	if a.contents[fileKey(a.files[0])] == nil {
		t.Fatal("文件内容应该已经读进来了")
	}

	a.rowsDirty = true
	a.rebuildRows()
	for _, r := range a.rows {
		if r.kind == rowGap && r.gapN <= 0 {
			t.Errorf("折叠条的行数 = %d，应该是正数", r.gapN)
		}
	}
}

func TestFilterNarrowsDiffView(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if !tt.HasText("internal/server/handler.go") {
		t.Fatal("筛选前应该能看到 handler.go")
	}

	a.fileFilter = "README"
	tt.Frame()

	if !tt.HasText("README.md") {
		t.Errorf("筛选后应该留下 README.md，实际：%q", tt.Texts())
	}
	if tt.HasText("internal/server/handler.go") {
		t.Errorf("筛选后 handler.go 应该从 diff 里消失，实际：%q", tt.Texts())
	}
}

func TestViewedCheckboxInDiffHeader(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if err := tt.Click("已 review"); err != nil {
		t.Fatalf("点已 review：%v", err)
	}
	tt.Frame()

	if a.viewedCount() != 1 {
		t.Errorf("勾选后 viewedCount = %d, want 1", a.viewedCount())
	}
	if !tt.HasText("1/3") {
		t.Errorf("侧栏底部应显示 1/3（勾是画的，不是字），实际：%q", tt.Texts())
	}
}

func TestExpandTabs(t *testing.T) {
	cases := []struct{ in, want, why string }{
		{"a\tb", "a   b", "从第 1 列跳到第 4 列"},
		{"abc\td", "abc d", "正好对齐在制表位上只补一格"},
		{"abcd\te", "abcd    e", "跨过一个制表位"},
		{"\tx", "    x", "行首的 tab"},
		{"中文\tx", "中文    x", "中文占两列，补齐要考虑宽度"},
		{"没有制表符", "没有制表符", "不含 tab 时原样返回"},
	}
	for _, c := range cases {
		if got := expandTabs(c.in); got != c.want {
			t.Errorf("expandTabs(%q) = %q, want %q（%s）", c.in, got, c.want, c.why)
		}
	}
}

func TestExpandTabsKeepsPlainTextUntouched(t *testing.T) {
	const s = "line with 中文 and spaces"
	if got := expandTabs(s); got != s {
		t.Errorf("expandTabs 改动了不含 tab 的文本：%q", got)
	}
}

func TestDisplayPath(t *testing.T) {
	cases := []struct{ old, new, want string }{
		{new: "a.go", want: "a.go"},
		{old: "gone.go", want: "gone.go"},
		{old: "old.go", new: "new.go", want: "old.go → new.go"},
		{old: "same.go", new: "same.go", want: "same.go"},
	}
	for _, c := range cases {
		f := gitrepo.File{FileDiff: diff.FileDiff{OldPath: c.old, NewPath: c.new}}
		if got := displayPath(f); got != c.want {
			t.Errorf("displayPath(old=%q new=%q) = %q, want %q", c.old, c.new, got, c.want)
		}
	}
}

func bigApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	var before, after []string
	for i := 1; i <= 20000; i++ {
		before = append(before, fmt.Sprintf("old line %d", i))
		after = append(after, fmt.Sprintf("new line %d", i))
	}
	writeFile(t, filepath.Join(dir, "big.go"), strings.Join(before, "\n")+"\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	writeFile(t, filepath.Join(dir, "big.go"), strings.Join(after, "\n")+"\n")

	return loadApp(t, dir)
}

func TestDiffViewOnlyBuildsVisibleRows(t *testing.T) {
	a := bigApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if len(a.rows) < 20000 {
		t.Fatalf("行模型的规模不对：%d 行", len(a.rows))
	}
	if a.built > 200 {
		t.Errorf("一帧构建了 %d 行，虚拟化没生效", a.built)
	}
	if a.built == 0 {
		t.Error("一行都没构建，说明 diff 根本没画出来")
	}
	if !tt.HasText("old line 1") && !tt.HasText("new line 1") {
		t.Errorf("第一行应该可见，实际：%q", tt.Texts())
	}
}

func TestDiffViewScrollingKeepsRowCountFlat(t *testing.T) {
	a := bigApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	first := a.built

	tt.Scroll(640, 400, 0, 20000)
	tt.Frame()
	if a.built > first*3+100 {
		t.Errorf("滚动后构建了 %d 行（滚动前 %d），虚拟化退化了", a.built, first)
	}
}

func TestDiffCodeTooltip(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("line 3: CHANGED AGAIN") {
		t.Fatal("找不到代码行内容")
	}
}
