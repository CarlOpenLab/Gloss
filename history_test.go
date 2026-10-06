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

// graphApp 建一个带分支和合并的仓库，用来看拓扑图对不对。
//
//	init ─ a1 ─┬── a2 ─┬── a3 ── 合并支线一 ── a4 ── 合并支线二
//	           └ f1 ─ f2 │        └ g1 ─ g2 ─┘
//	                     └ (feat2 从 a2 分出去)
//
// 两条支线同时开着，"合并支线一" 那阵子图上会并排三条泳道 —— 只有一条直线
// 的仓库看不出分叉画得对不对。
func graphApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	// 每次写一个新文件名，保证一定有东西可提交
	commit := func(name, msg string) {
		t.Helper()
		writeFile(t, filepath.Join(dir, name), msg+"\n")
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", msg)
	}

	commit("init.txt", "初始提交")
	commit("a1.txt", "主线一")

	runGit(t, dir, "checkout", "-q", "-b", "feat1")
	commit("f1.txt", "支线一之一")
	commit("f2.txt", "支线一之二")

	runGit(t, dir, "checkout", "-q", "main")
	commit("a2.txt", "主线二")
	runGit(t, dir, "checkout", "-q", "-b", "feat2")
	commit("g1.txt", "支线二之一")
	commit("g2.txt", "支线二之二")

	runGit(t, dir, "checkout", "-q", "main")
	commit("a3.txt", "主线三")
	runGit(t, dir, "merge", "-q", "--no-ff", "-m", "合并支线一", "feat1")
	commit("a4.txt", "主线四")
	runGit(t, dir, "merge", "-q", "--no-ff", "-m", "合并支线二", "feat2")

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

// 连着进两次历史 tab 也要正常。setTab 里 loadCommits 和 loadCommit 会
// 前后脚发起，两者曾经抢同一个 loadSeq —— 后发的把先发的撞掉，提交列表
// 永远停在「正在读提交…」。第一次进来 commitSel 还是 -1，所以只有第二次
// 才露馅。
func TestHistoryTabTwice(t *testing.T) {
	a := histApp(t)

	a.setTab(1)
	settle(t, a)
	if a.hist != loadReady || len(a.commits) == 0 {
		t.Fatalf("第一次进历史就不对：hist=%v commits=%d", a.hist, len(a.commits))
	}

	a.setTab(0)
	settle(t, a)
	a.setTab(1)
	settle(t, a)

	if a.hist != loadReady {
		t.Errorf("第二次进历史，列表状态 = %v, want loadReady", a.hist)
	}
	if len(a.commits) == 0 {
		t.Error("第二次进历史，提交列表空了")
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if tt.HasText("正在读提交…") {
		t.Errorf("列表卡在加载中：%q", tt.Texts())
	}
	if !tt.HasText("第三条：加个文件") {
		t.Errorf("提交没列出来：%q", tt.Texts())
	}
}

// 第二次之后进历史 tab，提交列表要重新读过，不能拿着旧的那份。
//
// setTab 里 loadCommits 和 loadCommit 前后脚发起，两者曾经抢同一个
// loadSeq：后发的把先发的撞掉，loadCommits 的结果被丢弃，a.commits
// 一直是第一次进来时那份 —— 新提交看不见。
func TestHistoryRefreshesCommits(t *testing.T) {
	a := histApp(t)
	a.setTab(1)
	settle(t, a) // commitSel 这时才有值，下一次进来才会两个请求打架
	a.setTab(0)
	settle(t, a)

	writeFile(t, filepath.Join(a.repoPath, "c.txt"), "新文件\n")
	runGit(t, a.repoPath, "add", ".")
	runGit(t, a.repoPath, "commit", "-m", "第四条")

	a.setTab(1)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if !tt.HasText("第四条") {
		t.Errorf("新提交没进列表：%q", tt.Texts())
	}
}

// 有分支有合并的仓库：图上要出现多条泳道，行数和列表一一对应，点提交照样
// 能换右边的 diff。
func TestHistoryGraphOnBranchyRepo(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.hist != loadReady || a.histDiff != loadReady {
		t.Fatalf("加载状态 hist=%v histDiff=%v（%s）", a.hist, a.histDiff, a.histDiffErr)
	}
	if len(a.graph) != len(a.commits) {
		t.Fatalf("图 %d 行，提交 %d 条 —— 必须一一对应", len(a.graph), len(a.commits))
	}
	if a.maxLanes < 3 {
		t.Errorf("两条支线同时开着，该并排三条泳道，实际 %d", a.maxLanes)
	}

	// 合并提交那两行要连出两条线
	merges := 0
	for i, cm := range a.commits {
		if len(cm.Parents) == 2 {
			merges++
			if len(a.graph[i].Out) != 2 {
				t.Errorf("合并提交 %q 的 Out = %v，该连出两条", cm.Subject, a.graph[i].Out)
			}
		}
	}
	if merges != 2 {
		t.Fatalf("测试仓库该有两个合并提交，实际 %d", merges)
	}

	for _, want := range []string{"合并支线二", "主线四", "支线二之二", "初始提交", "11 个提交"} {
		if !tt.HasText(want) {
			t.Errorf("列表里找不到 %q：%q", want, tt.Texts())
		}
	}

	// 点支线上的提交，右边换成它的 diff
	if err := tt.Click("支线二之二"); err != nil {
		t.Fatalf("点提交：%v", err)
	}
	settle(t, a)
	tt.Frame()
	if !tt.HasText("g2.txt") {
		t.Errorf("主区没换成那个提交的 diff：%q", tt.Texts())
	}
}

// 图形栏的宽度按整张图的最大泳道数定，不按当前行 —— 否则列表滚动时
// 每一行的文字会跟着图左右横跳。
func TestGraphWidthIsStable(t *testing.T) {
	if got := graphWidth(1); got != graphWidth(2) && got > 8+1*14 {
		t.Errorf("一条泳道的图不该比两条的还宽：%v", got)
	}
	if graphWidth(1) != graphPad+graphLaneW {
		t.Errorf("一条泳道 = %v", graphWidth(1))
	}
	// 超过上限就封顶，不能把提交信息挤没了
	if graphWidth(9) != graphWidth(graphMaxCol) {
		t.Errorf("泳道数超过 %d 之后宽度该封顶：%v vs %v",
			graphMaxCol, graphWidth(9), graphWidth(graphMaxCol))
	}
	// 空列表也留一条泳道的宽度（加载中 vs 就绪之间不闪）
	if graphWidth(0) != graphWidth(1) {
		t.Errorf("空图 = %v, want %v", graphWidth(0), graphWidth(1))
	}
	// 超出上限的泳道都并到最右边那条上
	if laneX(9, 9) != laneX(graphMaxCol-1, graphMaxCol) {
		t.Errorf("超出的泳道该并到最右：%v vs %v", laneX(9, 9), laneX(graphMaxCol-1, graphMaxCol))
	}
	// 泳道在图形栏里面
	for _, l := range []int{0, 3, 7} {
		if x := laneX(l, 8); x < graphPad || x > graphWidth(8) {
			t.Errorf("第 %d 条泳道跑到图形栏外面了：%v（宽 %v）", l, x, graphWidth(8))
		}
	}
}

// 图形栏真的在屏幕上画出了东西。
//
// 只断言 a.graph 里的数据不够：上一版 laneX 算的是「相对图形栏」的 x，画的时候
// 忘了加图形栏自己的偏移，线全画到侧栏底下、还被列表的裁剪区裁掉 —— 数据和
// 单元测试全绿，屏幕上却一根线都没有。所以这里直接数像素。
func TestHistoryGraphDrawsPixels(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()

	img := tt.Image()
	b := img.Bounds()
	if a.maxLanes < 2 {
		t.Fatalf("这个仓库该有多条泳道，实际 %d", a.maxLanes)
	}

	// 图形栏占的那条竖带：从列表左边到文字开始之前
	x0 := sidebarWidth
	x1 := sidebarWidth + int(graphWidth(a.maxLanes))

	// 泳道线是这里唯一的彩色像素 —— 选中行的底色、文字、边框都是灰阶，
	// 所以「鲜艳」就等于「图画出来了」，不用去比具体色值（主题一改就失效）。
	saturated := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := x0; x < x1 && x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			hi, lo := max(r, g, bl), min(r, g, bl)
			if hi-lo > 0x3000 { // 每通道 8 位，差出 48 以上
				saturated++
			}
		}
	}
	if saturated < 200 {
		t.Errorf("图形栏里只有 %d 个彩色像素，线没画出来", saturated)
	}
}
