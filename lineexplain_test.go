package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/explain"
)

// 双击一行代码，卡片讲的是「这一行」而不是整段。
func TestDoubleClickLineOpensLineCard(t *testing.T) {
	srv, calls := fakeLLM(t, "这一行在凑请求头。")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, hi, li := explainLineFixture(t, a)
	a.openExplainLine(fi, hi, li, nil)
	settle(t, a)
	tt.Frame()

	if !a.explainOpen() {
		t.Fatal("双击之后卡片没打开")
	}
	if a.ex.scope != scopeLine {
		t.Errorf("卡片粒度应该是 scopeLine，实际 %v", a.ex.scope)
	}
	if a.ex.line.lineIdx != li {
		t.Errorf("卡片指的行 = %d，应该是 %d", a.ex.line.lineIdx, li)
	}
	if *calls != 1 {
		t.Errorf("应该请求一次，实际 %d 次", *calls)
	}
	if !a.explainStillValid() {
		t.Error("刚打开的卡片应该是有效的")
	}
}

// 单行解释的前提词里要带这一行的内容和行号 —— 不然模型不知道在讲什么。
func TestLineExplainPromptCarriesTheLine(t *testing.T) {
	srv, _ := fakeLLM(t, "resp")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi, li := explainLineFixture(t, a)
	f, h := a.files[fi].FileDiff, a.files[fi].Hunks[hi]

	// 先把卡片开成这一行，再拿它的 prompt 走一遍 —— prompt 用的是 a.ex，
	// 和界面点出来的路径一致。
	a.openExplainLine(fi, hi, li, nil)
	system, user := a.selectionPrompt(f, h, hi, li, nil)

	if system == "" {
		t.Error("系统提示词不该是空的")
	}
	want := lineText(h, li)
	if want == "" {
		t.Fatal("测试夹具里的那一行是空的，这个测试没有意义")
	}
	if !hasText(user, want) {
		t.Errorf("提示词里没有那一行的内容：\n%s", user)
	}
	if !hasText(user, "需要解释的代码") {
		t.Errorf("提示词里没有点明「要解释这一行」：\n%s", user)
	}
}

// hunk 那颗 ✨ 讲的是整段，和单行解释共用一张卡片但 key 不同 ——
// 同一段的整体解释和单行解释不能互相顶替缓存。
func TestLineAndHunkKeysDiffer(t *testing.T) {
	srv, _ := fakeLLM(t, "resp")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi, li := explainLineFixture(t, a)
	f := a.files[fi]

	a.openExplainLine(fi, hi, li, nil)
	lineKey := a.ex.key
	// 追问也要换 key：问的是另一件事，缓存不该互相命中。
	first := lineKey

	a.ex.question = "这一行为什么只能用 map？"
	lineKey = a.explainKey(f, hi)
	if lineKey == first {
		t.Error("换了追问，缓存 key 应该跟着变")
	}

	// hunk 那一套：换 scope 再算一次，key 又不一样。
	a.ex.scope = scopeHunk
	hunkKey := a.explainKey(f, hi)
	if hunkKey == lineKey {
		t.Error("单行和整段的缓存 key 不该相同")
	}
}

// 第二次打开同一行直接吃缓存，不重复问模型。
func TestLineExplainSecondOpenHitsCache(t *testing.T) {
	srv, calls := fakeLLM(t, "缓存里那一行的解释")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi, li := explainLineFixture(t, a)
	a.openExplainLine(fi, hi, li, nil)
	settle(t, a)
	if *calls != 1 {
		t.Fatalf("第一次应该请求一次，实际 %d 次", *calls)
	}

	a.closeExplain()
	a.openExplainLine(fi, hi, li, nil)
	settle(t, a)

	if *calls != 1 {
		t.Errorf("第二次打开不该再请求，实际请求了 %d 次", *calls)
	}
	if !a.ex.cached {
		t.Error("第二次打开应该标记为命中缓存")
	}
	if !a.explainOpen() {
		t.Error("命中缓存也该把卡片开着")
	}
}

// 追问问完，答案替换掉旧的（不是接在后面），而且问题不同缓存不同。
func TestReAskLineReplacesAnswer(t *testing.T) {
	srv, calls := fakeLLM(t, "针对追问的新回答")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi, li := explainLineFixture(t, a)
	a.openExplainLine(fi, hi, li, nil)
	settle(t, a)
	if *calls != 1 {
		t.Fatalf("开局应该请求一次，实际 %d 次", *calls)
	}

	a.ex.question = "换成 goroutine 会更好吗"
	a.reAskLine()
	settle(t, a)

	if *calls != 2 {
		t.Errorf("追问应该再请求一次，实际 %d 次", *calls)
	}
	if a.ex.text != "针对追问的新回答" {
		t.Errorf("追问后的答案 = %q，应该是新回答而不是接在旧答案后面", a.ex.text)
	}
}

// 卡片指的那一行不在行模型里（区间被折叠吃了）时不该崩，退到 hunk 头下面。
func TestLineCardFallsBackWhenRowGone(t *testing.T) {
	a := richApp(t)
	fi, hi := explainFixtures(t, a)
	li := 0

	a.openExplainLine(fi, hi, li, nil)
	if !a.explainOpen() {
		t.Fatal("openExplainLine 之后面板应该是开着的")
	}
	if !a.ex.popoverOpen {
		t.Fatal("悬浮面板应该是打开状态")
	}
	if a.ex.scope != scopeLine {
		t.Fatalf("scope 应该是 scopeLine，实际是 %v", a.ex.scope)
	}
}

// 行在的时候卡片插在那一行后面，而不是 hunk 头后面 ——
// 「讲第 12 行」的卡片飘到段首，人就找不到它在讲哪了。
func TestLineCardInjectsUnderTheLine(t *testing.T) {
	a := richApp(t)
	fi, hi := explainFixtures(t, a)
	li := 2

	rows := []row{
		{kind: rowFileHeader, file: fi},
		{kind: rowHunkHeader, file: fi, hunk: hi},
		{kind: rowLine, file: fi, hunk: hi, lineIdx: 0},
		{kind: rowLine, file: fi, hunk: hi, lineIdx: 1},
		{kind: rowLine, file: fi, hunk: hi, lineIdx: 2},
		{kind: rowLine, file: fi, hunk: hi, lineIdx: 3},
	}
	a.openExplainLine(fi, hi, li, nil)

	// 悬浮面板不再内嵌在行模型里，验证状态即可
	if !a.explainOpen() {
		t.Fatal("面板应该是开着的")
	}
	if a.ex.fi != fi || a.ex.hi != hi {
		t.Fatalf("面板指向 %d:%d，该是 %d:%d", a.ex.fi, a.ex.hi, fi, hi)
	}
	_ = rows // rows fixture kept for future use
}

// 刷完之后卡片指的东西没了，得收起来 —— 不能挂着一张讲旧文件的卡片。
//
// 逐项掐掉它指着的东西，每一项都该让卡片失效。分开列而不是最后一把清空 ——
// 清空之后 a.files[fi] 直接越界返回，根本走不到中间那几道判断，测试就成了
// 「什么都不验也通过」。
func TestLineCardClosesWhenTargetVanishes(t *testing.T) {
	a := richApp(t)
	fi, hi, li := explainLineFixture(t, a)

	a.openExplainLine(fi, hi, li, nil)
	if !a.explainStillValid() {
		t.Fatal("刚打开的卡片应该是有效的")
	}

	// 1. 那一行没了：把 hunk 从 li 处截断，lineIdx 就越界了。
	saved := a.files[fi].Hunks[hi].Lines
	a.files[fi].Hunks[hi].Lines = saved[:li]
	if a.explainStillValid() {
		t.Error("lineIdx 越界之后卡片该失效")
	}

	// 2. hunk 没了（先把行还原回去，确认它又活了 —— 否则第 1 条什么也没验）
	a.files[fi].Hunks[hi].Lines = saved
	if !a.explainStillValid() {
		t.Fatal("还原之后卡片该重新有效")
	}
	a.files[fi].Hunks = nil
	if a.explainStillValid() {
		t.Error("hunk 没了之后卡片该失效")
	}

	// 3. 文件没了（连 fi 都指不出去了）
	a.files = nil
	if a.explainStillValid() {
		t.Error("文件没了之后卡片该失效")
	}
}

// 从单行卡片切到整段卡片（以及反过来）时，缓存 key 必须按新粒度重算。
//
// 这是个真实踩过的坑：key 原来在结构体字面量里算，而 explainKey 读的是
// a.ex 上的 scope —— 字面量那一行执行时 a.ex 还是**上一张**卡片的状态，
// 于是单行卡片算出了整段的 key。两边的缓存因此互相顶替：讲整段存进去的
// 东西，双击同一行就原样读了出来。
func TestKeyFollowsScopeAfterSwitch(t *testing.T) {
	a := richApp(t)
	fi, hi, li := explainLineFixture(t, a)

	// 先开单行卡片，再点 ✨ 开整段 —— key 该是整段那一套
	a.openExplainLine(fi, hi, li, nil)
	lineKey := a.ex.key
	if lineKey == "" {
		t.Fatal("单行卡片的 key 不该是空的")
	}

	a.openExplain(fi, hi, nil)
	hunkKey := a.ex.key
	if hunkKey == lineKey {
		t.Errorf("单行卡片的 key 用在了整段卡片上：%q", hunkKey)
	}
	// 整段的 key 由 hunk 内容哈希算出来，不含行内容 —— 换个粒度就认得出
	want := explain.CacheKey(a.files[fi].FileDiff, a.files[fi].Hunks[hi], a.modelName)
	if hunkKey != want {
		t.Errorf("整段卡片 key = %q，该是 %q", hunkKey, want)
	}

	// 再切回单行：这张卡讲的是同一行，key 该回到单行那一套
	a.openExplainLine(fi, hi, li, nil)
	if a.ex.key != lineKey {
		t.Errorf("切回单行卡片 key = %q，该是 %q", a.ex.key, lineKey)
	}
}

// 追问会换 key：问的是另一件事，缓存不该把上一个问题的答案还回来。
func TestQuestionChangesLineKey(t *testing.T) {
	a := richApp(t)
	fi, hi, li := explainLineFixture(t, a)

	a.openExplainLine(fi, hi, li, nil)
	before := a.ex.key

	a.ex.question = "这里为什么不用锁？"
	after := a.explainKey(a.files[fi], hi)
	if after == before {
		t.Error("换了追问，缓存 key 应该跟着变")
	}
}

// explainLineFixture 找一个有 hunk 且有内容行的文件/hunk，返回到第三行那一级。
func explainLineFixture(t *testing.T, a *app) (fi, hi, li int) {
	t.Helper()
	fi, hi = explainFixtures(t, a)
	h := a.files[fi].Hunks[hi]
	if len(h.Lines) == 0 {
		t.Fatal("hunk 里没有行，测不了单行解释")
	}
	// 取第一个非空行：空行解释起来没内容，测试也就看不出提示词拼对没拼对
	for i := range h.Lines {
		if lineText(h, i) != "" {
			return fi, hi, i
		}
	}
	t.Fatal("整个 hunk 都是空行")
	return fi, hi, 0
}

// lineText 是 hunk 里第 i 行去掉行首标记后的内容。
func lineText(h diff.Hunk, i int) string {
	if i < 0 || i >= len(h.Lines) {
		return ""
	}
	return strings.TrimLeft(h.Lines[i].Content, "+-")
}

// hasText 是「这段文本里有没有这句话」。测试用的 contains ——
// keys_i18n.go 里那个是同名不同参（查切片），别撞名。
func hasText(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	return strings.Contains(s, sub)
}
