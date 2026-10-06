package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// histApp 建一个有三条提交的仓库，用来测历史 tab。
func histApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(dir, "a.txt"), "one\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "第一条")

	writeFile(t, filepath.Join(dir, "a.txt"), "one\ntwo\n")
	runGit(t, dir, "commit", "-am", "第二条")

	writeFile(t, filepath.Join(dir, "b.txt"), "hello\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "第三条：加个文件")

	return loadApp(t, dir)
}

// 切到历史 tab：提交列表出来，并自动选中最近那个提交。
func TestHistoryTabListsCommits(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.hist != loadReady {
		t.Fatalf("历史加载状态 = %v, want ready", a.hist)
	}
	if len(a.commits) != 3 {
		t.Fatalf("读到 %d 条提交，want 3", len(a.commits))
	}
	if a.commitSel != 0 {
		t.Errorf("应该默认选中最近的提交，实际 commitSel = %d", a.commitSel)
	}
	for _, want := range []string{"第三条：加个文件", "第二条", "第一条", "3 个提交"} {
		if !tt.HasText(want) {
			t.Errorf("侧栏找不到 %q：%q", want, tt.Texts())
		}
	}
}

// 历史 tab 主区显示选中提交的 diff —— 复用同一套 diff 视图。
func TestHistoryShowsSelectedCommitDiff(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	// 最近的提交加了 b.txt
	if !tt.HasText("b.txt") {
		t.Errorf("主区没有显示这个提交改的文件：%q", tt.Texts())
	}
	if !tt.HasText("hello") {
		t.Errorf("主区没有显示新增的内容：%q", tt.Texts())
	}

	// ⌘1 切回改动：工作区是干净的
	a.setTab(0)
	settle(t, a)
	tt.Frame()
	if !tt.HasText("没有未提交的改动") {
		t.Errorf("切回改动 tab 应该显示工作区干净：%q", tt.Texts())
	}
}

// 点另一条提交，主区跟着换。
func TestHistoryClickCommitSwitchesDiff(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if err := tt.Click("第二条"); err != nil {
		t.Fatalf("点提交：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.commitSel != 1 {
		t.Fatalf("commitSel = %d, want 1", a.commitSel)
	}
	if !tt.HasText("a.txt") {
		t.Errorf("主区没有换成第二条提交的 diff：%q", tt.Texts())
	}
	if !tt.HasText("two") {
		t.Errorf("主区没有显示这一条提交加的内容：%q", tt.Texts())
	}
	if tt.HasText("hello") {
		t.Errorf("不该还留着上一条提交的内容：%q", tt.Texts())
	}
}

// 空仓库（一次提交都没有）单独给提示，不能报错。
func TestHistoryEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.hist != loadReady {
		t.Fatalf("空仓库不该算加载失败，实际状态 = %v（%s）", a.hist, a.histErr)
	}
	if !tt.HasText("还没有提交") {
		t.Errorf("空仓库应该给提示：%q", tt.Texts())
	}
}

// 切 tab 会把筛选词清掉：两个列表的文件集完全不同，留着筛选词多半是
// 一个文件都看不到。
func TestSetTabClearsFilter(t *testing.T) {
	a := histApp(t)
	a.filter = "a.txt"

	a.setTab(1)
	settle(t, a)
	if a.filter != "" {
		t.Errorf("切 tab 之后筛选词应该清掉，实际 %q", a.filter)
	}
}

// 历史上一个提交的 diff 也能点 ✨ 解释 —— 主区拿到的是同样的文件列表。
func TestHistoryCommitDiffIsExplainable(t *testing.T) {
	srv, _ := fakeLLM(t, "这段提交的解释")
	a := histApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if err := tt.Click("✨ 解释"); err != nil {
		t.Fatalf("点 ✨ 解释：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if !a.explainOpen() {
		t.Fatal("历史里的 diff 也该能点开解释")
	}
	if !tt.HasText("这段提交的解释") {
		t.Errorf("解释内容没出来：%q", tt.Texts())
	}
}

func TestShortWhen(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	cases := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "刚刚"},
		{now.Add(-5 * time.Minute), "5 分钟前"},
		{now.Add(-3 * time.Hour), "3 小时前"},
		{now.Add(-50 * time.Hour), "2 天前"},
		{now.Add(-24 * 45 * time.Hour), "1 个月前"},
		{now.Add(-24 * 400 * time.Hour), "1 年前"},
		{time.Time{}, ""},
	}
	for _, c := range cases {
		if got := shortWhen(c.at, now); got != c.want {
			t.Errorf("shortWhen(%v) = %q, want %q", c.at, got, c.want)
		}
	}
	// 未来时间当成刚发生，不能出现「-3 小时前」这种
	if got := shortWhen(now.Add(time.Hour), now); got != "刚刚" {
		t.Errorf("未来时间 = %q, want 刚刚", got)
	}
}

// git 给的 ref 是一串，可能带 "HEAD -> " 前缀，也可能有好几个。侧栏只放得下
// 一个，取第一个 —— 它总是最关键的那个。
func TestRefLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HEAD -> main", "HEAD -> main"},
		{"", ""},
		{"   ", ""},
		{"main, origin/main, tag: v1", "main"},
		{"tag: v1, main", "tag: v1"},
	}
	for _, c := range cases {
		if got := refLabel(c.in); got != c.want {
			t.Errorf("refLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
