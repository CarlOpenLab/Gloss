package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 命中的字节区间必须落在原文上，大小写不敏感也不能改变这件事。
func TestFindHitsCaseInsensitive(t *testing.T) {
	got := findHits("Handler.Handler", compileFindQuery("handler"))
	want := [][2]int{{0, 7}, {8, 15}}
	if len(got) != len(want) {
		t.Fatalf("命中 %d 条，want %d：%+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].start != w[0] || got[i].end != w[1] {
			t.Errorf("第 %d 条 = [%d,%d)，want [%d,%d)", i, got[i].start, got[i].end, w[0], w[1])
		}
	}
}

// 命中之间不重叠：查 "aa" 在 "aaa" 里只有一条，第二条不能从已经用掉的字符里
// 再借一个。
func TestFindHitsDoNotOverlap(t *testing.T) {
	if got := findHits("aaa", compileFindQuery("aa")); len(got) != 1 {
		t.Errorf("命中 %d 条，want 1：%+v", len(got), got)
	}
}

// İ（U+0130）的小写是两个码点、三个字节 —— 拿 strings.ToLower 整串转换再按
// 转换后的下标去切原文，后面的偏移全错位。按 rune 取等长片段比较的写法不会。
func TestFindHitsByteOffsetsSurviveUnicodeCaseFolding(t *testing.T) {
	const text = "aİb" // 1 + 2 + 1 = 4 字节

	// 先确认这个坑是真实存在的：这版 Go 的 ToLower 确实把 İ 展开了。
	if len(strings.ToLower(text)) == len(text) {
		t.Fatalf("前提不成立：ToLower(%q) 没有改变长度，对照失去意义", text)
	}

	got := findHits(text, compileFindQuery("b"))
	if len(got) != 1 {
		t.Fatalf("命中 %d 条，want 1：%+v", len(got), got)
	}
	if got[0].start != 3 || got[0].end != 4 {
		t.Fatalf("区间 = [%d,%d)，want [3,4)", got[0].start, got[0].end)
	}
	if text[got[0].start:got[0].end] != "b" {
		t.Errorf("按区间切出来的是 %q，want %q", text[got[0].start:got[0].end], "b")
	}
}

func TestFindHitsCJK(t *testing.T) {
	const text = "line 6: 新加的一行 with 中文"
	got := findHits(text, compileFindQuery("新加"))
	if len(got) != 1 {
		t.Fatalf("命中 %d 条，want 1：%+v", len(got), got)
	}
	if text[got[0].start:got[0].end] != "新加" {
		t.Errorf("按区间切出来的是 %q，want 新加", text[got[0].start:got[0].end])
	}
}

const findPatch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -3,2 +3,3 @@
 line3
+needle added
 line4
`

// 索引的顺序必须和界面从上往下看一致（未改动区间 / hunk 交错），否则连按
// 「下一个」会在文件里来回跳。key 也要分得开：未改动行是 hunk = -1。
func TestBuildFindIndexOrderAndKeys(t *testing.T) {
	f := fileFromPatch(t, findPatch)
	// 新侧共 6 行；hunk 覆盖第 3~5 行，所以区间是 1~2 和 6。
	var lines []string
	for i := 1; i <= 6; i++ {
		lines = append(lines, "line"+strconv.Itoa(i))
	}
	contents := map[string]*fileContent{fileKey(f): {lines: lines}}

	matches, index := buildFindIndex([]gitrepo.File{f}, contents, compileFindQuery("line"))

	want := []findKey{
		{0, -1, 1}, // 开头的未改动行
		{0, -1, 2},
		{0, 0, 0},  // hunk 里的上下文行 line3
		{0, 0, 2},  // hunk 里的上下文行 line4
		{0, -1, 6}, // 末尾的未改动行
	}
	if len(matches) != len(want) {
		t.Fatalf("命中 %d 条，want %d：%+v", len(matches), len(want), matches)
	}
	for i, w := range want {
		m := matches[i]
		key := findKey{m.file, m.hunk, m.line}
		if key != w {
			t.Errorf("第 %d 条命中在 %+v，want %+v", i, key, w)
		}
	}

	// 上下文行 line3 既在 hunk 里也在文件内容里，只能算一次。
	if hits := index[findKey{0, 0, 0}]; len(hits) != 1 {
		t.Errorf("line3 上有 %d 条命中，want 1（上下文行不能重复计数）", len(hits))
	}

	// 反查表里的下标要能指回 matches，且就是这一行的；而且逐条唯一 ——
	// 「当前命中」就是拿这个下标比的。
	seen := map[int]bool{}
	for key, hits := range index {
		for _, h := range hits {
			if h.match < 0 || h.match >= len(matches) {
				t.Fatalf("%+v 里记的下标 %d 越界", key, h.match)
			}
			m := matches[h.match]
			if got := (findKey{m.file, m.hunk, m.line}); got != key {
				t.Errorf("%+v 里记的下标 %d 指向了另一行 %+v", key, h.match, got)
			}
			if seen[h.match] {
				t.Errorf("matches 下标 %d 被两行共用", h.match)
			}
			seen[h.match] = true
		}
	}
}

// 冲突文件在界面上是卡片，行模型里没有它的代码行 —— 索引它等于让 cur 停在
// 一条永远跳不到的命中上。
func TestBuildFindIndexSkipsConflict(t *testing.T) {
	f := fileFromPatch(t, findPatch)
	f.Status = gitrepo.StatusConflict
	matches, _ := buildFindIndex([]gitrepo.File{f}, nil, compileFindQuery("needle"))
	if len(matches) != 0 {
		t.Errorf("冲突文件不该产生命中：%+v", matches)
	}
}

// 未改动行的高亮靠 hunk = -1 的 key 和 hunk 行分开 —— 行模型里那些行的
// hunk/lineIdx 是零值 0，key 不分开的话第一条 hunk 行的命中会涂到未改动行上。
func TestFindLineOfSeparatesGapKeys(t *testing.T) {
	a := &app{}
	a.find.on = true
	a.find.cur = 1
	a.find.byLine = map[findKey][]findHit{
		{0, -1, 7}: {{start: 0, end: 3, match: 0}},
		{0, 0, 0}:  {{start: 4, end: 7, match: 1}},
	}

	if got := a.findLineOf(0, 0, 0); len(got.hits) != 1 || got.hits[0].start != 4 {
		t.Errorf("hunk 行取到了 %+v", got.hits)
	}
	if got := a.findLineOf(0, -1, 7); len(got.hits) != 1 || got.hits[0].start != 0 {
		t.Errorf("未改动行取到了 %+v", got.hits)
	}
	// split 的空半行是 -1，取不到东西而不是取到第 0 行。
	if got := a.findLineOf(0, 0, -1); len(got.hits) != 0 {
		t.Errorf("空半行取到了 %+v", got.hits)
	}
	if got := a.findLineOf(0, 5, 5); len(got.hits) != 0 {
		t.Errorf("没命中的行取到了 %+v", got.hits)
	}

	a.find.on = false
	if got := a.findLineOf(0, 0, 0); len(got.hits) != 0 {
		t.Errorf("关着的时候还给出了命中：%+v", got.hits)
	}
}

// fileGaps 的区间是查找跳转的路标：跳转拿它把「第 N 行」映射回 gapKeyOf，
// 所以它必须和 buildRows 折出来的折叠条完全一致。
func TestFileGapsMatchesBuildRows(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -40,3 +40,4 @@
 line40
+added
 line41
 line42
@@ -80,2 +81,3 @@
 line80
+added2
 line81
`)

	// 新侧 100 行：hunk 覆盖 40~43 和 81~83，区间是 1~39、44~80、84~100。
	const total = 100
	got := fileGaps(f, total)
	want := []gapSpan{{from: 1, n: 39}, {from: 44, n: 37}, {from: 84, n: 17}}
	if len(got) != len(want) {
		t.Fatalf("区间 %d 段，want %d：%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 段 = %+v，want %+v", i, got[i], want[i])
		}
	}

	// 和行模型对一遍：三段都超过 inlineGap，都会折成折叠条。
	lines := make([]string, total)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i+1)
	}
	contents := map[string]*fileContent{fileKey(f): {lines: lines}}
	rows := buildRows([]gitrepo.File{f}, contents, nil, false, nil)

	var folded []gapSpan
	for _, r := range rows {
		if r.kind != rowGap {
			continue
		}
		from, err := strconv.Atoi(strings.TrimPrefix(r.gapKey, "g:a.go:"))
		if err != nil {
			t.Fatalf("折叠条的 key 认不出来：%q", r.gapKey)
		}
		folded = append(folded, gapSpan{from: from, n: r.gapN})
	}
	if len(folded) != len(got) {
		t.Fatalf("行模型折出 %d 段，fileGaps 给出 %d 段：%+v vs %+v", len(folded), len(got), folded, got)
	}
	for i := range got {
		if folded[i] != got[i] {
			t.Errorf("第 %d 段对不上：行模型 %+v，fileGaps %+v", i, folded[i], got[i])
		}
	}
}

// 纯删除的 hunk 没有新侧行数，起止行号是 git 的特殊约定（见 NewStartLine）——
// 区间算术必须跟着它走，否则跳转会展开错的那一段。
func TestFileGapsPureDeletion(t *testing.T) {
	f := fileFromPatch(t, `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -5,3 +4,0 @@
-gone1
-gone2
-gone3
`)
	// 新侧 8 行，hunk 的覆盖点是第 4~5 行之间，所以区间是 1~4 和 5~8。
	got := fileGaps(f, 8)
	want := []gapSpan{{from: 1, n: 4}, {from: 5, n: 4}}
	if len(got) != len(want) {
		t.Fatalf("区间 %d 段，want %d：%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 段 = %+v，want %+v", i, got[i], want[i])
		}
	}
}

// 高亮之后片段必须还是严丝合缝铺满整行 —— RichText 按片段依次排版，多一段
// 少一段整行就错位。
func TestApplyFindSpansCoverText(t *testing.T) {
	pal := palette{
		findBg:        ui.Hex("#111111"),
		findBgActive:  ui.Hex("#222222"),
		findUnderline: ui.Hex("#333333"),
	}
	const text = "func main() {}"

	cases := []struct {
		name    string
		spans   []ui.Span
		hits    []findHit
		cur     int
		covered string // 涂了底色的字节拼起来应该是什么
	}{
		{
			name:  "没有命中",
			spans: []ui.Span{{Text: text, Color: ui.Hex("#ffffff")}},
			cur:   -1,
		},
		{
			name:    "整行一个片段，命中在中间",
			spans:   []ui.Span{{Text: text, Color: ui.Hex("#ffffff")}},
			hits:    []findHit{{start: 5, end: 9, match: 0}},
			cur:     0,
			covered: "main",
		},
		{
			// 查 "func main" 会落在两个语法 token 上：跨片段必须接着涂。
			name: "命中跨越两个片段",
			spans: []ui.Span{
				{Text: "func ", Color: ui.Hex("#ff0000")},
				{Text: "main() {}", Color: ui.Hex("#00ff00")},
			},
			hits:    []findHit{{start: 0, end: 9, match: 0}},
			cur:     0,
			covered: "func main",
		},
		{
			name: "行首和行尾各一条",
			spans: []ui.Span{
				{Text: "func", Color: ui.Hex("#ff0000")},
				{Text: " main() ", Color: ui.Hex("#00ff00")},
				{Text: "{}", Color: ui.Hex("#0000ff")},
			},
			hits:    []findHit{{start: 0, end: 4, match: 0}, {start: 12, end: 14, match: 1}},
			cur:     1,
			covered: "func{}",
		},
		{
			// 索引和文本对不上（内容刚变、这一帧还没重算）：越界的区间夹回
			// 行内，不能涂到别的字上（更不能越界切）。
			name:    "越界的区间被夹回行内",
			spans:   []ui.Span{{Text: text, Color: ui.Hex("#ffffff")}},
			hits:    []findHit{{start: 9, end: 999, match: 0}},
			cur:     0,
			covered: "() {}",
		},
		{
			// 重叠的两条：后一条被裁到前一条的末尾。留下的尾巴（"{}"）还得
			// 涂上 —— 整个丢掉会让这段字凭空失去高亮，涂两遍又会让片段错位。
			name:    "重叠的部分被裁掉",
			spans:   []ui.Span{{Text: text, Color: ui.Hex("#ffffff")}},
			hits:    []findHit{{start: 9, end: 11, match: 0}, {start: 10, end: 14, match: 1}},
			cur:     0,
			covered: "() {}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyFind(tc.spans, text, findLine{hits: tc.hits, cur: tc.cur}, pal)

			var b strings.Builder
			for _, s := range got {
				b.WriteString(s.Text)
			}
			if b.String() != text {
				t.Fatalf("拼回来是 %q，want %q", b.String(), text)
			}

			var covered []string
			for _, s := range got {
				if s.Background != (ui.Color{}) {
					covered = append(covered, s.Text)
				}
			}
			if joined := strings.Join(covered, ""); joined != tc.covered {
				t.Errorf("涂底色的字 = %q，want %q", joined, tc.covered)
			}
		})
	}
}

// 当前命中要和别的命中分得开：深浅不同、还多一道下划线。
func TestApplyFindMarksCurrentHit(t *testing.T) {
	pal := palette{
		findBg:        ui.Hex("#111111"),
		findBgActive:  ui.Hex("#222222"),
		findUnderline: ui.Hex("#333333"),
	}
	const text = "aa bb"
	spans := []ui.Span{{Text: text}}
	hits := []findHit{{start: 0, end: 2, match: 0}, {start: 3, end: 5, match: 1}}

	got := applyFind(spans, text, findLine{hits: hits, cur: 1}, pal)
	var first, second ui.Span
	for _, s := range got {
		switch s.Background {
		case pal.findBgActive:
			second = s
		case pal.findBg:
			first = s
		}
	}
	if first.Text != "aa" {
		t.Errorf("非当前命中涂的是 %q，want aa", first.Text)
	}
	if second.Text != "bb" {
		t.Errorf("当前命中涂的是 %q，want bb", second.Text)
	}
	if !second.Underline || second.DecorationColor != pal.findUnderline {
		t.Error("当前命中应该有下划线，且用 findUnderline 的颜色")
	}
	if first.Underline {
		t.Error("非当前命中不该有下划线")
	}
}

// —— 界面级：走完整的那条路（按键 → 输入 → 建控件）——

// ⌘F 开栏、打字、计数、↵ / ⇧↵ 循环。
//
// richApp 的 handler.go 里 "CHANGED" 出现三次：第 3 行的删除行、第 3 行的
// 新增行（"CHANGED AGAIN"）、第 52 行的删除行。三条都在 hunk 里，顺序就是
// 界面从上往下看的顺序。
func TestFindCountsAndCycles(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	if !a.find.on {
		t.Fatal("⌘F 之后查找栏该是开着的")
	}

	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	if len(a.find.matches) != 3 {
		t.Fatalf("命中 %d 条，want 3：%+v", len(a.find.matches), a.find.matches)
	}
	if !tt.HasText("1/3") {
		t.Errorf("计数该是 1/3：%q", tt.Texts())
	}

	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.find.cur != 1 || !tt.HasText("2/3") {
		t.Errorf("↵ 之后 cur = %d，计数：%q", a.find.cur, tt.Texts())
	}

	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.find.cur != 2 || !tt.HasText("3/3") {
		t.Errorf("再按一次 ↵ 之后 cur = %d，计数：%q", a.find.cur, tt.Texts())
	}

	// 到头绕回第一条：一路按下去不该卡在末尾。
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.find.cur != 0 || !tt.HasText("1/3") {
		t.Errorf("到头该绕回第一条：cur = %d，计数：%q", a.find.cur, tt.Texts())
	}

	// ⇧↵ 是往回走，同样绕圈。
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Frame()
	if a.find.cur != 2 || !tt.HasText("3/3") {
		t.Errorf("⇧↵ 该退到最后一条：cur = %d，计数：%q", a.find.cur, tt.Texts())
	}
}

// ⌘G / ⌘⇧G 和 ↵ / ⇧↵ 是同一件事，焦点不在输入框里时也得能用。
func TestFindNextShortcuts(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	// 焦点交给别处：⌘G 是窗口级的键，不该只有输入框拿着焦点时才认。
	tt.Click("关闭查找")
	tt.Frame()
	if a.find.on {
		t.Fatal("关闭查找之后栏该收了")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyG)
	tt.Frame()
	if a.find.cur != 1 {
		t.Errorf("⌘G 之后 cur = %d, want 1", a.find.cur)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyG)
	tt.Frame()
	if a.find.cur != 0 {
		t.Errorf("⌘⇧G 之后 cur = %d, want 0", a.find.cur)
	}
}

// 命中在折叠着的未改动区间里时，跳过去要把它放出来 —— 不然「下一个」会
// 停在一条看不见的命中上。
func TestFindJumpRevealsCollapsedGap(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 第 20 行在 10~48 那段区间里，区间比 inlineGap 长，界面上是一条折叠条。
	const line20 = "line 20: some code here"
	if tt.HasText(line20) {
		t.Fatal("前提不成立：第 20 行本来就看得见，测不到「跳转放行」")
	}

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("line 20:")
	settle(t, a)
	tt.Frame()

	if len(a.find.matches) != 1 {
		t.Fatalf("命中 %d 条，want 1：%+v", len(a.find.matches), a.find.matches)
	}
	// 整段放出来，而不是放一屏 —— 用户点的是「去那一行」。
	// key 用的是 fileKey（带 s:/u: 段落前缀），和 buildRows 里折叠条的 key 同源。
	if got := a.expanded[gapKeyOf(fileKey(a.files[a.find.matches[0].file]), 10)]; got != 39 {
		t.Errorf("区间展开 %d 行，want 39", got)
	}
	// 滚到目标行上：它在可视区里，而不是停在别处。
	i := a.rowForFind(a.find.matches[0].file, -1, 20)
	if i < 0 {
		t.Fatal("行模型里找不到第 20 行 —— 区间没放出来")
	}
	first, last := a.diffList.Visible()
	if i < first || i > last {
		t.Errorf("第 20 行在下标 %d，可视区是 [%d,%d] —— 没滚过去", i, first, last)
	}
}

// 文件折着（已 review）时跳进去要自动展开。
func TestFindJumpUnfoldsFile(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	fi := -1
	for i := range a.files {
		if pathOf(a.files[i]) == "internal/server/handler.go" {
			fi = i
		}
	}
	if fi < 0 {
		t.Fatal("找不到 handler.go")
	}
	a.toggleViewed(a.files[fi])
	tt.Frame()
	if !a.isFolded(a.files[fi]) {
		t.Fatal("标记已 review 之后文件该是折着的")
	}

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED AGAIN")
	settle(t, a)
	tt.Frame()

	if a.isFolded(a.files[fi]) {
		t.Error("跳进去之后文件该展开")
	}
	// 展开还不够 —— 得滚到那一行上。HasText 只说这一帧画过它，
	// 目标在可视区之外时那次「画过」是列表提前构建的虚影。
	i := a.rowForFind(a.find.matches[0].file, a.find.matches[0].hunk, a.find.matches[0].line)
	if i < 0 {
		t.Fatal("行模型里找不到目标行 —— 文件没展开")
	}
	first, last := a.diffList.Visible()
	if i < first || i > last {
		t.Errorf("目标行在下标 %d，可视区是 [%d,%d] —— 没滚过去", i, first, last)
	}
}

// 没有命中时写「无结果」，这时按 ↵ / ⌘G 不能崩、也不能乱跳。
func TestFindNoResults(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("zzz-not-in-this-diff")
	settle(t, a)
	tt.Frame()

	if len(a.find.matches) != 0 {
		t.Fatalf("不该有命中：%+v", a.find.matches)
	}
	if !tt.HasText("无结果") {
		t.Errorf("该写「无结果」：%q", tt.Texts())
	}

	before, _ := a.diffList.Visible()
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Cmd, ui.KeyG)
	tt.Frame()
	if after, _ := a.diffList.Visible(); after != before {
		t.Errorf("没有命中时不该滚动：%d → %d", before, after)
	}
}

// Esc 收栏：高亮跟着消失，J/K 回到「跳 hunk」。
func TestFindEscClosesWhileTyping(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()
	if len(a.find.matches) == 0 {
		t.Fatal("前提不成立：这条查询该有命中")
	}
	if a.find.byLine == nil {
		t.Fatal("前提不成立：索引该建起来了")
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	if a.find.on {
		t.Error("Esc 之后查找栏该收了")
	}
	if tt.HasText("在差异中查找") {
		t.Errorf("栏收了之后输入框不该还在：%q", tt.Texts())
	}
	if a.find.byLine != nil {
		t.Error("栏收了之后索引该清掉 —— 高亮不能再涂")
	}

	// 焦点回到窗口中，J 重新是「下一段改动」。
	a.diffList.ScrollTo(0, ui.Start)
	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if first, _ := a.diffList.Visible(); first == 0 {
		t.Error("Esc 之后 J 该能跳 hunk 了")
	}
}

// 切到历史 tab 之后栏还在，计数对着新的那份 diff。
func TestFindSurvivesDiffSwitch(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("line 1:")
	settle(t, a)
	tt.Frame()

	tt.Key(ui.Cmd, ui.Key2)
	settle(t, a)
	tt.Frame()
	tt.Key(ui.Cmd, ui.Key1)
	settle(t, a)
	tt.Frame()

	if !a.find.on {
		t.Fatal("切 tab 之后查找栏该还在")
	}
	if a.find.cur < 0 || (len(a.find.matches) > 0 && a.find.cur >= len(a.find.matches)) {
		t.Errorf("cur = %d 越出了 %d 条命中", a.find.cur, len(a.find.matches))
	}
	// 索引必须认的是当前这份文件列表：越界的话上面那条就炸了，
	// 这里再钉一下命中确实落在这一次的 diff 上。
	for _, m := range a.find.matches {
		if m.file >= len(a.files) {
			t.Fatalf("命中的文件下标 %d 越界（共 %d 个文件）", m.file, len(a.files))
		}
	}
}

// 两个仓库标签页来回切：索引跟着 a.files 一起换，不许指着上一个仓库的行。
func TestFindTabSwitchInvalidates(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	// 开第二个仓库（内容和第一个不一样），切回去再切回来。
	other := t.TempDir()
	runGit(t, other, "init", "-b", "main")
	runGit(t, other, "config", "user.email", "test@example.com")
	runGit(t, other, "config", "user.name", "Test")
	runGit(t, other, "config", "commit.gpgsign", "false")
	writeFile(t, filepath.Join(other, "only.go"), "package main\n\nvar marker = 1\n")
	runGit(t, other, "add", ".")
	runGit(t, other, "commit", "-m", "init")
	writeFile(t, filepath.Join(other, "only.go"), "package main\n\nvar marker = 2\nvar CHANGED = 3\n")

	a.openInNewTab(other)
	settle(t, a)
	tt.Frame()

	a.switchTab(0)
	settle(t, a)
	tt.Frame()

	if !a.find.on {
		t.Fatal("切回来之后查找栏该还在")
	}
	for _, m := range a.find.matches {
		if m.file >= len(a.files) {
			t.Fatalf("命中的文件下标 %d 越界（共 %d 个文件）", m.file, len(a.files))
		}
	}
	if a.find.cur >= len(a.find.matches) && len(a.find.matches) > 0 {
		t.Fatalf("cur = %d 越出了 %d 条命中", a.find.cur, len(a.find.matches))
	}
}

// 筛掉的文件不参与查找：它们的行根本不在屏幕上，搜出来只会跳到一片空白。
func TestFindSkipsFilteredFiles(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.fileFilter = "readme"
	a.applyFilter()
	settle(t, a)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	if len(a.find.matches) != 0 {
		t.Errorf("筛掉的文件不该有命中：%+v", a.find.matches)
	}
	if !tt.HasText("无结果") {
		t.Errorf("筛掉之后该是「无结果」：%q", tt.Texts())
	}

	// 松开筛选：同一条查询立刻又搜得到（applyFilter 让索引重扫了一遍）。
	a.fileFilter = ""
	a.applyFilter()
	settle(t, a)
	tt.Frame()
	if len(a.find.matches) != 3 {
		t.Errorf("松开筛选之后命中 %d 条，want 3：%+v", len(a.find.matches), a.find.matches)
	}
}

// 终端模式没有 diff 可搜：⌘F 不开栏，Esc 也不归查找管。
func TestFindTerminalMode(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyJ)
	tt.Frame()
	if a.viewMode != viewModeTerm {
		t.Fatal("⌘J 该切到终端模式")
	}

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	if a.find.on {
		t.Error("终端模式下 ⌘F 不该开查找栏")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.find.on {
		t.Error("终端模式下不该有查找栏可关")
	}
}

// 焦点不在输入框里时 Esc 也得能收栏 —— 那是窗口级的那一条（元素级那条
// 只有焦点在输入框里才走得到）。
func TestFindEscClosesFromOutside(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	// 点栏外面（栏脚的总计）：焦点离开查找框。
	if err := tt.Click("总计"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if a.find.focused {
		t.Fatal("点了栏外面之后，焦点闸该松开")
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.find.on {
		t.Error("焦点不在输入框里时 Esc 也该收栏")
	}
}

// 点栏里的按钮之后焦点要还回输入框 —— 点「下一个」的人下一步就是想接着
// 打字，焦点还留在按钮上的话那一下字就丢了。
func TestFindButtonKeepsFocusInField(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	if err := tt.Click("下一个"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if a.find.cur != 1 {
		t.Errorf("点「下一个」之后 cur = %d, want 1", a.find.cur)
	}
	tt.Frame()

	// 焦点回来了的话，这个字进的是查找框。
	tt.Type("X")
	tt.Frame()
	if a.find.query != "CHANGEDX" {
		t.Errorf("点完按钮之后字没进查找框：query = %q", a.find.query)
	}
}

// 模态对话框开着时查找什么都不做：打字不进查找框，Esc 关的是那个对话框。
func TestFindIgnoredByModals(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	if !a.palette.on {
		t.Fatal("⌘K 该打开命令栏")
	}

	// 命令栏占着键盘：⌘F 不该在它底下偷偷把查找栏开出来。
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	if !a.palette.on {
		t.Fatal("⌘F 不该把命令栏关掉")
	}

	tt.Type("zzz")
	tt.Frame()
	if a.find.query != "CHANGED" {
		t.Errorf("命令栏开着时打的字不该进查找框：query = %q", a.find.query)
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.palette.on {
		t.Error("Esc 该关掉命令栏")
	}
	if !a.find.on {
		t.Error("Esc 关的是命令栏，不该顺手把查找栏也收了")
	}
}

// 切 unified / split 不该动命中：命中记的是源坐标，两种视图只是把同一份
// 行摆成不同的形状。
func TestFindSurvivesSplitToggle(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.setSplit(false)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("CHANGED")
	settle(t, a)
	tt.Frame()
	before := len(a.find.matches)
	if before != 3 {
		t.Fatalf("命中 %d 条，want 3", before)
	}

	a.setSplit(true)
	tt.Frame()
	if len(a.find.matches) != before || a.find.cur != 0 {
		t.Errorf("切 split 之后命中变成 %d 条、cur = %d，want %d 条、cur = 0",
			len(a.find.matches), a.find.cur, before)
	}
	// 目标行在两边都解析得出来（split 的空半行不能把查找带到 -1 上去）。
	if i := a.rowForFind(a.find.matches[0].file, a.find.matches[0].hunk, a.find.matches[0].line); i < 0 {
		t.Error("split 下行模型里找不到命中的那一行")
	}

	a.setSplit(false)
	tt.Frame()
	if len(a.find.matches) != before {
		t.Errorf("切回 unified 之后命中变成 %d 条，want %d", len(a.find.matches), before)
	}
}
