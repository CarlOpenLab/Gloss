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
		"internal/server/handler.go", // 文件头（列表的 header）
		"line 3: CHANGED",            // 旧侧被删掉的那行
		"line 3: CHANGED AGAIN",      // 新侧加上的那行
		"line 52: CHANGED TOO",       // 第二个 hunk，隔着一段未改动区间
		"line 52: 也改了",
		"line 6: 新加的一行 with 中文",
	} {
		if !tt.HasText(want) {
			t.Errorf("diff 里找不到 %q", want)
		}
	}
}

// git 把函数名直接接在 @@ 头后面，我们把它单独画在右边 ——
// 不切掉的话同一句话会连着出现两遍。
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

	// 没有函数名时原样返回
	bare := diff.Hunk{Header: "@@ -1,3 +1,4 @@"}
	if got := hunkHeaderText(bare); got != "@@ -1,3 +1,4 @@" {
		t.Errorf("无函数名时 = %q", got)
	}
}

// 未改动区间默认折成一条；点一下摊开，摊开的行要带上真实行号和内容。
func TestGapExpandsOnClick(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	const bar = "⋯  展开 39 行未改动"
	if !tt.HasText(bar) {
		t.Fatalf("没有看到折叠条，实际文本：%q", tt.Texts())
	}
	// 折叠着的第 20 行不该在界面上
	if tt.HasText("line 20: some code here") {
		t.Fatal("还没展开就看到了区间里的行")
	}

	if err := tt.Click(bar); err != nil {
		t.Fatalf("点折叠条：%v", err)
	}
	tt.Frame()

	// 区间 key 里带的是 fileKey（含段落前缀），这里照界面自己的口径取那一份 ——
	// 钉的是「点了一下展开状态确实落到那个区间上」，不是 key 长什么样。
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

// 内容还没读到时也要能折叠 —— 行号靠 hunk 的位置算得出来，不依赖文件内容。
func TestGapShowsWithoutContents(t *testing.T) {
	a := richApp(t)
	a.contents = nil
	a.rowsDirty = true

	tt := ui.NewTester(a.view, 1280, 860)
	if !tt.HasText("⋯  展开 39 行未改动") {
		t.Errorf("没有文件内容时也该显示折叠条，实际：%q", tt.Texts())
	}
}

// 末尾那段未改动区间要靠文件总行数才算得出来，所以内容必须读到。
func TestTrailingGapAfterContentsLoad(t *testing.T) {
	a := richApp(t)
	if a.contents[fileKey(a.files[0])] == nil {
		t.Fatal("文件内容应该已经读进来了")
	}

	// handler.go 的最后一个 hunk 结束在第 60 行，正好是文件末尾，没有尾部区间；
	// README.md 只有 4 行且 hunk 覆盖到第 4 行，也没有。这里只钉住「内容
	// 到了之后行模型是对的」这个不变量。
	a.rowsDirty = true
	a.rebuildRows()
	for _, r := range a.rows {
		if r.kind == rowGap && r.gapN <= 0 {
			t.Errorf("折叠条的行数 = %d，应该是正数", r.gapN)
		}
	}
}

// 筛选会同时收窄侧栏和 diff —— 界面上下对不上会让人以为漏看了文件。
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

// 勾上「已 review」要写进 viewed，再勾一次取消。
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
	// 不含 tab 时不该重新拼接字符串（省一次分配，也避免意外改动内容）
	const s = "line with 中文 and spaces"
	if got := expandTabs(s); got != s {
		t.Errorf("expandTabs 改动了不含 tab 的文本：%q", got)
	}
}

func TestDisplayPath(t *testing.T) {
	cases := []struct {
		old, new, want string
	}{
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

// bigApp 建一个改动量很大的仓库：一个两万行的文件全改了，外加一个
// 大段未改动区间 —— 用来验证虚拟化真的在起作用。
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

// 列表必须只构建可见的那些行。几万行的 diff 要是整份建出来，
// 光是构建元素就会把界面卡死。
func TestDiffViewOnlyBuildsVisibleRows(t *testing.T) {
	a := bigApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if len(a.rows) < 30000 {
		t.Fatalf("行模型的规模不对：%d 行", len(a.rows))
	}
	// 860 高的窗口能放下的行数在 50 上下，加上列表的前后预取也就百来行
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

// 滚动到别处时构建的行数不应该跟着涨 —— 那是虚拟化最容易退化的方式。
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
