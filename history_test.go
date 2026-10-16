package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
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
	waitReview(t, a, tt)

	// 最近的提交加了 b.txt
	if !tt.HasText("b.txt") {
		t.Errorf("主区没有显示这个提交改的文件：%q", tt.Texts())
	}
	if got := reviewText(a); !strings.Contains(got, "hello") {
		t.Errorf("主区没有显示新增的内容：\n%s", got)
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
	waitReview(t, a, tt)
	if !tt.HasText("a.txt") {
		t.Errorf("主区没有换成第二条提交的 diff：%q", tt.Texts())
	}
	if got := reviewText(a); !strings.Contains(got, "two") {
		t.Errorf("主区没有显示这一条提交加的内容：\n%s", got)
	}
	if got := reviewText(a); strings.Contains(got, "hello") {
		t.Errorf("不该还留着上一条提交的内容：\n%s", got)
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
	a.fileFilter = "a.txt"

	a.setTab(1)
	settle(t, a)
	if a.fileFilter != "" {
		t.Errorf("切 tab 之后筛选词应该清掉，实际 %q", a.fileFilter)
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
		{"HEAD -> main", "HEAD → main"},
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

// 提交行应该能同时解析出 HEAD 本地分支、远程分支（如 origin/beta）和标签，
// 并自动跳过 origin/HEAD 这种符号引用。
func TestParseRefBadges(t *testing.T) {
	// 场景 1：用户最关心的分支同步场景 —— 同时包含本地 HEAD 与远程 origin
	t.Run("HEAD and remote branch", func(t *testing.T) {
		got := parseRefBadges("HEAD -> beta, origin/beta", []string{"origin"})
		if len(got) != 2 {
			t.Fatalf("want 2 badges, got %d: %+v", len(got), got)
		}
		if got[0].Label != "HEAD → beta" || got[0].Kind != refBadgeHead {
			t.Errorf("badge[0] = %+v, want HEAD → beta (head)", got[0])
		}
		if got[1].Label != "origin/beta" || got[1].Kind != refBadgeRemote {
			t.Errorf("badge[1] = %+v, want origin/beta (remote)", got[1])
		}
	})

	// 场景 2：过滤 origin/HEAD 符号引用
	t.Run("filters origin/HEAD symref", func(t *testing.T) {
		got := parseRefBadges("HEAD -> main, origin/main, origin/HEAD", []string{"origin"})
		if len(got) != 2 {
			t.Fatalf("want 2 badges, got %d: %+v", len(got), got)
		}
		if got[0].Label != "HEAD → main" || got[1].Label != "origin/main" {
			t.Errorf("got %+v, want [HEAD → main, origin/main]", got)
		}
	})

	// 场景 3：排序保证 本地分支 > 远程分支 > 标签
	t.Run("orders local > remote > tag", func(t *testing.T) {
		got := parseRefBadges("tag: v1.0, origin/main, main", []string{"origin"})
		if len(got) != 3 {
			t.Fatalf("want 3 badges, got %d", len(got))
		}
		if got[0].Label != "main" || got[0].Kind != refBadgeLocal {
			t.Errorf("badge[0] = %+v, want main", got[0])
		}
		if got[1].Label != "origin/main" || got[1].Kind != refBadgeRemote {
			t.Errorf("badge[1] = %+v, want origin/main", got[1])
		}
		if got[2].Label != "tag: v1.0" || got[2].Kind != refBadgeTag {
			t.Errorf("badge[2] = %+v, want tag: v1.0", got[2])
		}
	})

	// 场景 4：分离 HEAD
	t.Run("detached HEAD", func(t *testing.T) {
		got := parseRefBadges("HEAD", nil)
		if len(got) != 1 || got[0].Label != "HEAD" || got[0].Kind != refBadgeHead {
			t.Errorf("got %+v, want [HEAD]", got)
		}
	})

	// 场景 5：空引用
	t.Run("empty refs", func(t *testing.T) {
		if got := parseRefBadges("", nil); len(got) != 0 {
			t.Errorf("want empty, got %+v", got)
		}
		if got := parseRefBadges("   ", nil); len(got) != 0 {
			t.Errorf("want empty, got %+v", got)
		}
	})

	// 场景 6：多个远程
	t.Run("multiple remotes", func(t *testing.T) {
		got := parseRefBadges("upstream/main, origin/main, HEAD -> main", []string{"origin", "upstream"})
		if len(got) != 3 {
			t.Fatalf("want 3 badges, got %d", len(got))
		}
		if got[0].Label != "HEAD → main" {
			t.Errorf("badge[0] = %+v, want HEAD → main", got[0])
		}
		if got[1].Kind != refBadgeRemote || got[2].Kind != refBadgeRemote {
			t.Errorf("want both remotes to be refBadgeRemote: %+v", got)
		}
	})
}

// 测试提交列表 UI 渲染中能够同时看到 HEAD 分支和 origin 分支徽章
func TestCommitRowShowsRemoteBadge(t *testing.T) {
	a := histApp(t)
	a.remotes = []string{"origin"}
	a.setTab(1)
	settle(t, a)

	// 模拟最新提交已同步到 origin/main
	if len(a.commits) > 0 {
		a.commits[0].Refs = "HEAD -> main, origin/main"
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("HEAD → main") {
		t.Errorf("没有在历史列表中找到 HEAD → main 徽章：%q", tt.Texts())
	}
	if !tt.HasText("origin/main") {
		t.Errorf("没有在历史列表中找到 origin/main 徽章：%q", tt.Texts())
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

// 从改动切回历史时立即恢复展示，不闪烁加载占位提示。
func TestSwitchingTabsDoesNotFlicker(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	// 第一次进历史并就绪
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	if !tt.HasText("第三条：加个文件") {
		t.Fatalf("第一次进历史应该显示提交：%q", tt.Texts())
	}

	// 切到改动
	a.setTab(0)
	tt.Frame()

	// 再次切到历史：甚至在后台 settle 还没跑完的第一帧，
	// 界面也应该立刻恢复提交和 diff，不能闪出「正在读提交…」或「正在读这个提交…」
	a.setTab(1)
	tt.Frame()

	if tt.HasText("正在读提交…") {
		t.Errorf("切回历史时不应闪烁「正在读提交…」：%q", tt.Texts())
	}
	if tt.HasText("正在读这个提交…") {
		t.Errorf("切回历史时不应闪烁「正在读这个提交…」：%q", tt.Texts())
	}
	if !tt.HasText("第三条：加个文件") {
		t.Errorf("切回历史应该立即保留展示提交列表：%q", tt.Texts())
	}
	if len(a.files) == 0 {
		t.Errorf("切回历史应该立即恢复文件列表，实际为空")
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
	// 一条泳道的图必须比两条的窄。这里原来写的是 got != graphWidth(2) && got > 8+1*14
	// —— 后半句是旧常量的字面量副本，泳道宽从 14 改到 20 之后就变成「30 > 22」，
	// 测试直接红了，而它想说的根本不是那个。
	if graphWidth(1) >= graphWidth(2) {
		t.Errorf("一条泳道的图不该比两条的宽：%v vs %v", graphWidth(1), graphWidth(2))
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

// TestLabelAccentsAreDistinct 断言小标签亮色真的分得开（§2.5）。
//
// 这一组色只落在 10px 的状态字母和芯片上那颗 6px 的圆点上，而它存在的唯一理由
// 就是「一眼能分开」：M 和 R 曾经一起落在 pal.ref 的灰上（改动列表于是得逐字读），
// 本地分支的圆点曾经和 HEAD 一样是 Ink（一列芯片全是白点）。所以这里不看具体色值，
// 只钉住「两两不同」+「该带色相的确实带」—— 谁再把这几个色统一成一支灰，这条就红。
func TestLabelAccentsAreDistinct(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := themeFor(dark, ui.Preferences{})
		pal := paletteFor(th, laneSet{})

		letters := map[string]ui.Color{
			"A": statusColor(gitrepo.StatusAdded, pal),
			"D": statusColor(gitrepo.StatusDeleted, pal),
			"M": statusColor(gitrepo.StatusModified, pal),
			"R": statusColor(gitrepo.StatusRenamed, pal),
		}
		seen := map[ui.Color]string{}
		for _, k := range []string{"A", "D", "M", "R"} {
			if prev, dup := seen[letters[k]]; dup {
				t.Errorf("dark=%v: 状态字母 %s 与 %s 同色 %v —— 状态字母得一眼分得开",
					dark, k, prev, letters[k])
			}
			seen[letters[k]] = k
		}
		// M 与 R 是补上的那两支亮色，不许退化成灰。
		for _, k := range []string{"M", "R"} {
			if spread(letters[k]) <= 16 {
				t.Errorf("dark=%v: 状态字母 %s 是灰的（RGB 极差 %d）—— 灰阶在 10px 的字上分不出层级",
					dark, k, spread(letters[k]))
			}
		}

		dots := map[string]ui.Color{
			"HEAD": badgeDot(pal, refBadgeHead),
			"本地分支": badgeDot(pal, refBadgeLocal),
			"远程分支": badgeDot(pal, refBadgeRemote),
			"标签":   badgeDot(pal, refBadgeTag),
		}
		seenDot := map[ui.Color]string{}
		for _, k := range []string{"HEAD", "本地分支", "远程分支", "标签"} {
			if prev, dup := seenDot[dots[k]]; dup {
				t.Errorf("dark=%v: %s 与 %s 的圆点同色 %v —— 一列芯片上都是同色的点，谁也认不出谁",
					dark, k, prev, dots[k])
			}
			seenDot[dots[k]] = k
		}
		if spread(dots["HEAD"]) > 16 {
			t.Errorf("dark=%v: HEAD 的圆点 %v 带色相 —— 它跟主干、「我在哪」共用 Ink", dark, dots["HEAD"])
		}

		// 语义三色只有 glossTheme 一处定义，调色板只是把同一份搬过来：
		// 两边一旦分家，对话框里的红和框架自绘控件的红就会是两种红。
		if pal.success != th.Success || pal.warning != th.Warning || pal.danger != th.Danger {
			t.Errorf("dark=%v: 调色板的语义三色与主题不一致（%v/%v/%v vs %v/%v/%v）",
				dark, pal.success, pal.warning, pal.danger, th.Success, th.Warning, th.Danger)
		}
		// 标签琥珀就是告警琥珀：M 的字母、标签芯片的点、告警圆点说的是
		// 同一件「留神看一眼」的事，一支色就够了。
		if pal.accentAmber != pal.warning {
			t.Errorf("dark=%v: 标签琥珀 %v 与告警琥珀 %v 不是同一支", dark, pal.accentAmber, pal.warning)
		}
	}
}

// 一条线从头顶到底部必须是同一个颜色。
//
// 这是「色号跟着线走」的端到端证明。上面 TestGraphLineKeepsColor 只盯着
// buildGraph 的数据；真正出问题的是画的那一步 —— 它原来用 pal.laneColor(row.Lane)
// 取色，主线一旦挪泳道（主线三→主线二→主线一 一路往右挪）颜色就跟着变，
// 屏幕上一条主线看着像三条不同的分支，而数据侧一切正常。
func TestHistoryGraphLineColorIsContinuous(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 1100) // 窗口开高一点，11 行提交要全看得见

	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()

	img := tt.Image()
	// 提交主题 → 它在列表里的行号
	idx := map[string]int{}
	for i, cm := range a.commits {
		idx[cm.Subject] = i
	}

	// 主线上那几条。它们在 a.graph 里的 Lane 各不相同，这正是要点。
	chain := []string{"合并支线二", "主线四", "合并支线一", "主线三", "主线二", "主线一"}
	type sample struct {
		name string
		lane int
		r    uint32
		g    uint32
		b    uint32
	}
	var got []sample
	for _, name := range chain {
		i, ok := idx[name]
		if !ok {
			t.Fatalf("测试仓库里没有 %q", name)
		}
		box, ok := tt.Find(name)
		if !ok {
			t.Fatalf("屏幕上找不到 %q（窗口不够高？）", name)
		}
		// 行里的文字是垂直居中的，所以文字框的中线就是节点中心那条水平线
		y := int(box.Y + box.H/2)
		x := sidebarWidth + int(laneX(a.graph[i].Lane, a.maxLanes))
		r, g, b, _ := img.At(x, y).RGBA()
		got = append(got, sample{name, a.graph[i].Lane, r, g, b})
	}

	// 先把「这条链真的换了泳道」钉死，否则这个测试可能什么也没验证
	lanes := map[int]bool{}
	for _, s := range got {
		lanes[s.lane] = true
	}
	if len(lanes) < 2 {
		t.Fatalf("主线全程都在同一泳道上（%v），这个测试就白测了", lanes)
	}

	first := got[0]
	for _, s := range got[1:] {
		if s.r != first.r || s.g != first.g || s.b != first.b {
			t.Errorf("%s（lane %d）的节点是 %v，%s（lane %d）是 %v —— 同一条线换了颜色",
				s.name, s.lane, []uint32{s.r, s.g, s.b},
				first.name, first.lane, []uint32{first.r, first.g, first.b})
		}
	}

	// 反面：支线得是另一个颜色，否则「同色」也可能是「全图一个颜色」
	if i, ok := idx["支线二之二"]; ok {
		if box, ok := tt.Find("支线二之二"); ok {
			y := int(box.Y + box.H/2)
			x := sidebarWidth + int(laneX(a.graph[i].Lane, a.maxLanes))
			r, g, b, _ := img.At(x, y).RGBA()
			if r == first.r && g == first.g && b == first.b {
				t.Errorf("支线二之二和主线同色 —— 两条线得分得开")
			}
		}
	}
}

// 点侧栏的分支，历史就切到那条分支上。
//
// 这是侧栏从文件树换成 refs 树之后新接的那条链路：点一下 → histRev 指到
// 完整引用名 → 提交列表重读。判据不能只看「列表变短了」—— 还要看 main 上
// 独有的提交确实不见了，否则「什么都没发生」也能蒙混过关。
func TestSidebarBranchNarrowsHistory(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if len(a.refs) == 0 {
		t.Fatalf("侧栏没读到引用：%v（%s）", a.refsState, a.refErr)
	}
	if a.histRev != "" {
		t.Fatalf("一开始该看 HEAD，实际 histRev = %q", a.histRev)
	}

	// 用坐标点，不用 tt.Click("feat1")：提交列表里那个提交还挂着 feat1 的
	// 徽章，同名文本有两处，按名字点会点中哪一个全看遍历顺序。
	box, ok := tt.Find("feat1")
	if !ok {
		t.Fatalf("侧栏里找不到 feat1：%q", tt.Texts())
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	settle(t, a)
	tt.Frame()

	if a.histRev != "refs/heads/feat1" {
		t.Fatalf("点了 feat1 之后 histRev = %q, want refs/heads/feat1", a.histRev)
	}
	if a.tab != 1 {
		t.Errorf("点分支该切到历史 tab，实际 tab = %d", a.tab)
	}
	// feat1 从「主线一」分出去：支线一之一、支线一之二，加上主线上到
	// 「初始提交」为止 —— 一共四条
	if len(a.commits) != 4 {
		t.Errorf("feat1 的历史有 %d 条，want 4：%v", len(a.commits), subjects(a.commits))
	}
	if !tt.HasText("支线一之二") {
		t.Errorf("feat1 的提交没列出来：%q", tt.Texts())
	}
	if tt.HasText("主线四") {
		t.Errorf("主线四不在 feat1 上，不该出现在列表里：%q", tt.Texts())
	}
}

// 点侧栏「所有分支」：提交列表换成所有引用合起来的历史。
//
// 这是「本地 / 远程两棵树分开列、点哪条看哪条」之外的那一屏：用户问的是
// 「这个仓库总共在干什么」。判据是 HEAD 走不到的支线提交也出现 —— 只比
// 条数的话，一条恰好同样长的历史也能蒙混过关。
func TestAllBranchesRowShowsEveryBranch(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 夹具里两条支线都被合并进了 main，HEAD 走得到每一条。补一条没合并的
	// 分支 —— 「所有分支」要比 HEAD 多的那几条正是它。
	runGit(t, a.repoPath, "checkout", "-q", "-b", "unmerged")
	writeFile(t, filepath.Join(a.repoPath, "u.txt"), "u\n")
	runGit(t, a.repoPath, "add", ".")
	runGit(t, a.repoPath, "commit", "-m", "没人要的分支上的提交")
	runGit(t, a.repoPath, "checkout", "-q", "main")
	a.reload()
	settle(t, a)

	a.setTab(1)
	settle(t, a)
	tt.Frame()

	headOnly := len(a.commits)
	if headOnly == 0 {
		t.Fatal("HEAD 的历史是空的")
	}

	// 用坐标点，不用 tt.Click("所有分支")：栏头也会写这四个字，而那一处
	// 在点了之后才出现 —— 之前侧栏里只有这一处，Find 拿到的就是它。
	box, ok := tt.Find("所有分支")
	if !ok {
		t.Fatalf("侧栏里找不到「所有分支」：%q", tt.Texts())
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	settle(t, a)
	tt.Frame()

	if !a.histAll {
		t.Error("点了「所有分支」之后 histAll 该是 true")
	}
	if a.histRev != "" {
		t.Errorf("「所有分支」不是一条引用，histRev 该是空串，实际 %q", a.histRev)
	}
	if a.tab != 1 {
		t.Errorf("该留在历史 tab，实际 tab = %d", a.tab)
	}
	if len(a.commits) <= headOnly {
		t.Errorf("所有分支的历史该比 HEAD 那条长：%d vs %d（%v）",
			len(a.commits), headOnly, subjects(a.commits))
	}
	// 没合并的那条分支上的提交，HEAD 走不到
	for _, want := range []string{"没人要的分支上的提交"} {
		if !tt.HasText(want) {
			t.Errorf("%s 不在所有分支的历史里：%q", want, tt.Texts())
		}
	}
	// 栏头得说出在看的是哪一份，不然用户不知道自己在看什么
	if !tt.HasText("所有分支") {
		t.Errorf("栏头没写「所有分支」：%q", tt.Texts())
	}
}

// 提交筛选框：打字即筛，只筛已经读回来的那一批。
func TestCommitFilterNarrowsHistory(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	// / 把焦点放进提交筛选框
	tt.Key(0, ui.KeySlash)
	tt.Frame()
	tt.Type("支线二")
	settle(t, a)
	tt.Frame()

	if a.commitFilter != "支线二" {
		t.Fatalf("输入没进提交筛选框，commitFilter = %q", a.commitFilter)
	}
	// feat2 上两个提交；合并支线二那条是合并提交，信息里带「支线二」三个字
	if len(a.commits) != 3 {
		t.Errorf("筛「支线二」该留下 3 条，实际 %d：%v", len(a.commits), subjects(a.commits))
	}
	if !tt.HasText("支线二之一") {
		t.Errorf("筛出来的提交没列出来：%q", tt.Texts())
	}
	if tt.HasText("主线一") {
		t.Errorf("主线一不该匹配「支线二」：%q", tt.Texts())
	}
	// 栏头说得出范围：屏幕上几个、这一趟一共几个
	if !tt.HasText("筛选出") {
		t.Errorf("筛选时栏头该写「筛选出 X / Y」：%q", tt.Texts())
	}

	// 空结果是「没有匹配」，不该是「还没有提交」—— 那说的是空仓库
	tt.Type("肯定没有这样的提交信息")
	settle(t, a)
	tt.Frame()
	if len(a.commits) != 0 {
		t.Errorf("筛一个不存在的词该空，实际 %d 条", len(a.commits))
	}
	if !tt.HasText("没有匹配") {
		t.Errorf("空筛选该提示没有匹配：%q", tt.Texts())
	}

	// 清空回到全量：筛选只是视图，读回来的提交一条都没丢。
	// （框里那个 × 是画出来的图标、没有文字可点，这里直接清词再走一遍
	// 重算 —— 和点 × 落到同一处。）
	a.commitFilter = ""
	a.applyCommitFilter()
	tt.Frame()
	if a.commitFilter != "" || len(a.commits) != len(a.allCommits) {
		t.Errorf("清空之后该回到全量：filter=%q commits=%d all=%d",
			a.commitFilter, len(a.commits), len(a.allCommits))
	}
}

// 筛选把选中的那个提交筛掉时，选中必须作废。
//
// 不清的话下标往往还合法（列表短了一截，旧下标恰好还在范围内），于是下半区
// 显示的是别处的 diff —— 界面上没有任何异常，只是内容和选中对不上。
func TestCommitFilterDropsStaleSelection(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.commitSel < 0 {
		t.Fatal("进来该自动选中最近那个提交")
	}
	sel := a.commits[a.commitSel]

	// 筛一个选中的提交匹配不上的词
	a.commitFilter = "肯定匹配不上" + sel.Subject
	a.applyCommitFilter()
	tt.Frame()

	if a.commitSel != -1 {
		t.Errorf("选中的提交被筛掉之后 commitSel 该是 -1，实际 %d", a.commitSel)
	}
	if a.histDiff == loadReady && len(a.files) > 0 && a.histListID == "c:"+sel.Hash {
		t.Error("下半区还显示着已经被筛掉的那个提交")
	}
}

// 筛选态下「加载更多」接着没筛过的那份往下读。
//
// offset 按 a.allCommits 算，不是按筛完的 a.commits：筛掉一半还按后者算的
// 话，下一页会把同一批提交读第二遍 —— 屏幕上看着「筛出来的还是那些」，
// 其实已经重复了，而 allCommits 越长重复越多。
func TestLoadMoreCommitsWhileFiltering(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	// 505 个提交：首批 500，还剩 5 个在下一页
	var fi strings.Builder
	for i := 1; i <= 505; i++ {
		fmt.Fprintf(&fi, "commit refs/heads/main\ncommitter Test <test@example.com> %d +0000\ndata %d\ncommit %d\nM 644 inline file.txt\ndata 5\nhello\n\n", 1700000000+i, len(fmt.Sprintf("commit %d", i)), i)
	}
	cmd := exec.Command("git", "fast-import")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(fi.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fast-import failed: %v: %s", err, string(out))
	}
	runGit(t, dir, "reset", "--hard", "main")

	a := loadApp(t, dir)
	a.setTab(1)
	settle(t, a)

	if len(a.allCommits) != 500 || !a.histHasMore {
		t.Fatalf("首批该读到 500 个且还有更多：all=%d hasMore=%v", len(a.allCommits), a.histHasMore)
	}

	// 只留一个提交在屏幕上
	a.commitFilter = "commit 3 "
	a.applyCommitFilter()
	if len(a.commits) == 0 || len(a.commits) >= len(a.allCommits) {
		t.Fatalf("筛选该只留下个位数条，实际 %d / %d", len(a.commits), len(a.allCommits))
	}
	shown := subjects(a.commits)

	a.loadMoreCommits()
	settle(t, a)

	if len(a.allCommits) != 505 {
		t.Errorf("加载更多之后该有 505 个未筛提交，实际 %d", len(a.allCommits))
	}
	// 读回来的提交一条都不该重复：按筛过的条数算 offset 就会把整批读第二遍
	seen := map[string]bool{}
	for _, cm := range a.allCommits {
		if seen[cm.Hash] {
			t.Fatalf("提交 %s 被读了两遍", cm.Short)
		}
		seen[cm.Hash] = true
	}
	// 筛选结果跟着新读回来的那批走：原来筛中的还在（它不会被读第二遍改变）
	for _, s := range shown {
		found := false
		for _, cm := range a.commits {
			if cm.Subject == s {
				found = true
			}
		}
		if !found {
			t.Errorf("筛出来的 %s 在加载更多之后丢了", s)
		}
	}
}

// 在编辑器 / 终端视图里点侧栏的分支，界面要回到 Git 面板的历史那一栏。
//
// tab 只在 Git 面板里活着（主区画哪一栏归 viewMode 管）：光把 a.tab 置成 1，
// 屏幕上还是编辑器那一栏 —— 用户看到的就是「点了没反应」。从终端视图点过来
// 是同一条路，一起钉住。
func TestSidebarBranchClickLeavesOtherViews(t *testing.T) {
	for _, mode := range []struct {
		name string
		mode int
	}{{"编辑器", viewModeEditor}, {"终端", viewModeTerm}} {
		t.Run(mode.name, func(t *testing.T) {
			a := graphApp(t)
			tt := ui.NewTester(a.view, 1280, 1100)
			tt.Frame()
			settle(t, a)
			tt.Frame()

			a.setViewMode(mode.mode)
			settle(t, a)
			tt.Frame()
			if a.viewMode != mode.mode {
				t.Fatalf("没切进那一视图：viewMode = %d", a.viewMode)
			}

			// 用坐标点，不用 tt.Click("feat1")：提交列表里那个提交还挂着
			// feat1 的徽章，同名文本有两处，按名字点会点中哪一个全看遍历顺序。
			box, ok := tt.Find("feat1")
			if !ok {
				t.Fatalf("侧栏里找不到 feat1：%q", tt.Texts())
			}
			tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
			settle(t, a)
			tt.Frame()

			if a.viewMode != viewModeGit {
				t.Errorf("点了分支该回到 Git 面板，实际 viewMode = %d", a.viewMode)
			}
			if a.tab != 1 {
				t.Errorf("点了分支该切到历史 tab，实际 tab = %d", a.tab)
			}
			if a.histRev != "refs/heads/feat1" {
				t.Errorf("点了 feat1 之后 histRev = %q, want refs/heads/feat1", a.histRev)
			}
			if !tt.HasText("支线一之二") {
				t.Errorf("feat1 的提交没列出来：%q", tt.Texts())
			}
		})
	}
}

// 历史 tab 栏头的增删计数上色：绿 / 红，和文件树行尾、栏脚「总计」同一个
// 口径（addBar / delBar）。
//
// 曾经这里两个数字共用一支中性的 ref 色（分支名、哈希那支）—— 同样两个数字
// 在屏幕三处三个脸色，读的人得分别记。
func TestHistoryHeaderCountsAreColored(t *testing.T) {
	a := richApp(t)
	a.setTab(1)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if a.histDiff != loadReady || len(a.files) == 0 {
		t.Fatalf("选中提交的 diff 没读出来：hist=%v files=%d err=%s",
			a.histDiff, len(a.files), a.histDiffErr)
	}

	// 栏脚「总计」那一对数字和栏头一模一样（同一个 totalAdd/totalDel），
	// Find 取先遍历到的那个 —— 栏头建在栏脚前面，拿到的就是栏头。
	add := "+" + strconv.Itoa(a.totalAdd())
	del := "−" + strconv.Itoa(a.totalDel())

	box, ok := tt.Find(add)
	if !ok {
		t.Fatalf("栏头找不到 %q：%q", add, tt.Texts())
	}
	if box.Y > 200 {
		t.Errorf("拿到的是栏脚那一对（Y=%.0f），不是栏头", box.Y)
	}
	if green, red := countHue(tt, box); !green || red {
		t.Errorf("栏头的 %s 该是绿的（不红），实际 green=%v red=%v", add, green, red)
	}

	box, ok = tt.Find(del)
	if !ok {
		t.Fatalf("栏头找不到 %q：%q", del, tt.Texts())
	}
	if green, red := countHue(tt, box); green || !red {
		t.Errorf("栏头的 %s 该是红的（不绿），实际 green=%v red=%v", del, green, red)
	}
}

// countHue 在这一行字里横向扫一遍，看有没有绿像素、有没有红像素。
//
// 抗锯齿让多数字素停在灰阶上，字与字之间全是背景色，所以扫一片而不是取一个
// 点。比较前转 int：uint8 上加 20 会绕回（237 > 219 变成 237 > 3）。
func countHue(tt *ui.Tester, box ui.Rect) (green, red bool) {
	y := int(box.Y + box.H/2)
	for x := int(box.X); x < int(box.X+box.W); x++ {
		p := pixelAt(tt, x, y)
		if int(p[1]) > int(p[0])+20 && int(p[1]) > int(p[2])+20 {
			green = true
		}
		if int(p[0]) > 150 && int(p[1]) < 120 && int(p[2]) < 120 {
			red = true
		}
	}
	return green, red
}

// subjects 是给报错信息用的提交主题列表。
func subjects(commits []gitrepo.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.Subject)
	}
	return out
}

// scrollApp 建一个「长文件夹着目标文件」的仓库。
//
// 目标文件前面要有很长的内容（否则一开始就在屏幕上，测不出滚动），后面也
// 要有（否则列表滚到底也够不着它，它到不了顶 —— 这不算滚失败，但断言就得
// 写得绕）。三个文件各放在自己的目录里：文件树显示短名（target.txt），
// diff 的文件头显示完整路径（m/target.txt），两者对不上，按名字找不会找错。
func scrollApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(dir, "seed.txt"), "seed\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "初始提交")

	bulk := func(prefix string) string {
		var lines []string
		for i := 1; i <= 200; i++ {
			lines = append(lines, fmt.Sprintf("%s line %d", prefix, i))
		}
		return strings.Join(lines, "\n") + "\n"
	}
	writeFile(t, filepath.Join(dir, "a/huge1.txt"), bulk("before"))
	writeFile(t, filepath.Join(dir, "m/target.txt"), "target\n")
	writeFile(t, filepath.Join(dir, "z/huge2.txt"), bulk("after"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "三个文件")

	return loadApp(t, dir)
}

// 点文件列表里的文件，改动区就换成那个文件。
//
// 一份改动里几十个文件时，点文件名如果只是高亮一下、右边还画着上一个文件，
// 点了看着就像没反应。
func TestFileClickShowsThatFileDiff(t *testing.T) {
	a := scrollApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	const want = "m/target.txt"
	target := -1
	for i, f := range a.files {
		if f.NewPath == want {
			target = i
		}
	}
	if target < 0 {
		t.Fatalf("测试仓库的提交里没有 %s", want)
	}

	// 文件树里只叫 target.txt，和别的路径对不上，所以这个名字在屏幕上是唯一的
	if err := tt.Click("target.txt"); err != nil {
		t.Fatalf("点不到文件列表里的 target.txt：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.fileSel != target {
		t.Errorf("点了之后 fileSel = %d, want %d", a.fileSel, target)
	}
	if wantKey := fileKey(a.files[target]); a.review.key != wantKey {
		t.Errorf("改动区该换成 %s，实际 %s", wantKey, a.review.key)
	}
	if !tt.HasText(want) {
		t.Errorf("改动区栏头该显示完整路径 %s：%q", want, tt.Texts())
	}
}

// stashApp 建一个有储藏的仓库：一个已跟踪文件改了、外加一个未跟踪的新文件，
// 一起 stash 起来。两种改动都在，才看得出 `stash show -p --include-untracked`
// 的产物是不是真能当 diff 显示。
func stashApp(t *testing.T) *app {
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
	runGit(t, dir, "commit", "-m", "初始提交")

	writeFile(t, filepath.Join(dir, "a.txt"), "one\n已经提交的一行\n")
	runGit(t, dir, "commit", "-am", "第二条")

	writeFile(t, filepath.Join(dir, "a.txt"), "one\n已经提交的一行\n攒了一半的一行\n")
	writeFile(t, filepath.Join(dir, "b.txt"), "还没加进索引的新文件\n")
	runGit(t, dir, "stash", "push", "-q", "-u", "-m", "半成品")

	return loadApp(t, dir)
}

// 点侧栏的储藏，下半区显示它的改动。
//
// 储藏不走 Log —— 它不是任何一条分支历史的一部分，硬塞进提交列表会打断
// 拓扑图，而且 Log 也拿不出「这个储藏的改动」。走的是 git stash show。
// 计划里把这一步标成「可以对不上就先放弃」，所以更得有一条测试钉住它。
func TestStashClickShowsDiff(t *testing.T) {
	a := stashApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if !tt.HasText("储藏") {
		t.Fatalf("侧栏没有储藏分组：%q", tt.Texts())
	}
	if !tt.HasText("stash@{0}") {
		t.Fatalf("侧栏没列出储藏：%q", tt.Texts())
	}

	if err := tt.Click("stash@{0}"); err != nil {
		t.Fatalf("点不到储藏：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.histDiff != loadReady {
		t.Fatalf("储藏没读出 diff：state=%v err=%s", a.histDiff, a.histDiffErr)
	}
	if len(a.files) != 2 {
		t.Fatalf("储藏的改动有 %d 个文件，want 2（改过的 + 未跟踪的）：%v", len(a.files), a.files)
	}
	if a.commitSel != -1 {
		t.Errorf("看储藏的时候不该还选中着某个提交：commitSel = %d", a.commitSel)
	}
	// 储藏不进提交列表：列表还是 HEAD 那份，拓扑图不该被它打断
	if a.hist != loadReady || len(a.commits) != 2 {
		t.Errorf("提交列表被储藏影响了：hist=%v 共 %d 条", a.hist, len(a.commits))
	}

	// 屏幕上真画出来了：两个文件都在文件列表里，改动区也装上了其中一个。
	// （不断言具体内容：本机的 git 在储藏路径上会给出带引号的转义形式，
	// 「读某一边的原文」因此拿不到内容 —— 那是既有的环境问题，见 gitrepo
	// 里那条 TestStashDiff。这条只管界面有没有把储藏的改动摆上去。）
	waitReview(t, a, tt)
	if len(a.files) != 2 {
		t.Errorf("文件列表里该有两个文件：%v", a.files)
	}
	if a.review.key == "" || a.review.err != "" {
		t.Errorf("储藏的改动没摆进改动区：key=%q err=%q", a.review.key, a.review.err)
	}
}

// 看过储藏之后切去改动 tab 再切回来，储藏的改动还得在。
//
// 切去改动 tab 会把 allFiles 清掉，而 stashRev 还在、提交列表因此不会自动
// 选中任何提交 —— 不在 setTab 里把储藏接回来的话，上半区没有选中的行、
// 下半区一片空白，两头都空着。
func TestStashSurvivesTabSwitch(t *testing.T) {
	a := stashApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if err := tt.Click("stash@{0}"); err != nil {
		t.Fatalf("点不到储藏：%v", err)
	}
	settle(t, a)
	tt.Frame()

	a.setTab(0)
	settle(t, a)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if a.stashRev == "" {
		t.Error("切回来之后储藏被忘了")
	}
	if a.histDiff != loadReady || len(a.files) != 2 {
		t.Fatalf("切回来之后下半区是空的：state=%v 共 %d 个文件", a.histDiff, len(a.files))
	}
	waitReview(t, a, tt)
	if a.review.key == "" {
		t.Errorf("储藏的改动没摆进改动区：err=%q", a.review.err)
	}

	// 换个分支看，储藏就该让位了
	box, ok := tt.Find("main")
	if !ok {
		t.Fatalf("侧栏找不到 main：%q", tt.Texts())
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	settle(t, a)
	tt.Frame()
	if a.stashRev != "" {
		t.Errorf("点了分支之后 stashRev 还留着：%q", a.stashRev)
	}
	if a.commitSel != 0 {
		t.Errorf("点了分支该自动选中最近那个提交，实际 commitSel = %d", a.commitSel)
	}
}

// 验证提交列表突破 200 条限制，首屏读取 500 条且支持增量加载更多历史提交。
func TestHistoryLoadMoreCommits(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	// 通过 fast-import 极速生成 550 个提交（耗时 < 0.1s）
	var fi strings.Builder
	for i := 1; i <= 550; i++ {
		fmt.Fprintf(&fi, "commit refs/heads/main\ncommitter Test <test@example.com> %d +0000\ndata %d\ncommit %d\nM 644 inline file.txt\ndata 5\nhello\n\n", 1700000000+i, len(fmt.Sprintf("commit %d", i)), i)
	}
	cmd := exec.Command("git", "fast-import")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(fi.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fast-import failed: %v: %s", err, string(out))
	}
	runGit(t, dir, "reset", "--hard", "main")

	a := loadApp(t, dir)
	a.setTab(1)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 首批读取 500 个提交（突破原本 200 个的限制）
	if len(a.commits) != 500 {
		t.Fatalf("首批应读取 500 个提交，实际为 %d", len(a.commits))
	}
	if !a.histHasMore {
		t.Fatal("应该还有未加载的提交 (histHasMore 应为 true)")
	}
	if !tt.HasText("已加载 500 个提交…") {
		t.Errorf("应该显示已加载 500 个提交提示，实际文本: %q", tt.Texts())
	}

	// 触发加载更多
	a.loadMoreCommits()
	settle(t, a)
	tt.Frame()

	// 全部 550 个提交加载完毕
	if len(a.commits) != 550 {
		t.Fatalf("加载更多后应为 550 个提交，实际为 %d", len(a.commits))
	}
	if a.histHasMore {
		t.Fatal("全部加载完毕后 histHasMore 应为 false")
	}
	if !tt.HasText("550 个提交") {
		t.Errorf("应该显示 550 个提交提示，实际文本: %q", tt.Texts())
	}
}
