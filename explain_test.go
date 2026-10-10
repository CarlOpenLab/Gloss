package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/llm"
	"github.com/CarlOpenLab/Gloss/internal/store"
)

// fakeLLM 起一个假的 OpenAI 兼容接口，按片返回一段解释。
// 返回的 *int 记录被调用了几次 —— 用来验证缓存真的生效了。
func fakeLLM(t *testing.T, chunks ...string) (*httptest.Server, *int) {
	t.Helper()
	calls := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

// withProvider 给 app 换上一个指向假接口的 store。
func withProvider(t *testing.T, a *app, srv *httptest.Server) {
	t.Helper()
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{Providers: []llm.Provider{{
		Name: "假接口", Type: "openai", BaseURL: srv.URL, APIKey: "k", Model: "test-model",
	}}})
	a.attachStore(s)
}

// explainFixtures 找一个有 hunk 的文件，返回它和第一个 hunk 的下标。
func explainFixtures(t *testing.T, a *app) (int, int) {
	t.Helper()
	for i, f := range a.files {
		if len(f.Hunks) > 0 {
			return i, 0
		}
	}
	t.Fatal("仓库里没有带 hunk 的文件")
	return 0, 0
}

// 点 ✨ 打开、再点一次收起。这是卡片最基本的状态机。
func TestExplainTogglesFromButton(t *testing.T) {
	srv, _ := fakeLLM(t, "这是一段解释。")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()
	if !a.explainOpen() {
		t.Fatal("openExplain 之后卡片应该是开着的")
	}

	// 按钮点一次收起 —— 用真实点击而不是直接调函数，顺便验证按钮接上了。
	// 标签是「解释」：✨ 是自绘的矢量图形，不在文字里。
	if err := tt.Click("解释"); err != nil {
		t.Fatalf("点解释按钮：%v", err)
	}
	tt.Frame()
	if a.explainOpen() {
		t.Error("再点一次 ✨ 应该收起卡片")
	}
}

// 点 ✨ 能打开卡片，并把流式内容显示出来。
func TestExplainButtonOpensCard(t *testing.T) {
	srv, _ := fakeLLM(t, "先看这一句", "，再看这一句。")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	if err := tt.Click("解释"); err != nil {
		t.Fatalf("点解释按钮：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if !a.explainOpen() {
		t.Fatal("点 ✨ 之后卡片没打开")
	}
	if !tt.HasText("AI 解释") {
		t.Errorf("卡片标题没画出来：%q", tt.Texts())
	}
	if !tt.HasText("先看这一句，再看这一句。") {
		t.Errorf("流式内容没显示全：%q", tt.Texts())
	}
}

// Esc 关掉卡片。
func TestExplainEscapeCloses(t *testing.T) {
	srv, _ := fakeLLM(t, "解释")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.explainOpen() {
		t.Error("Esc 应该关掉卡片")
	}
}

// 第二次打开同一个 hunk 直接用缓存，不再问模型 —— 缓存的意义就在这里。
func TestExplainSecondOpenHitsCache(t *testing.T) {
	srv, calls := fakeLLM(t, "缓存下来的解释")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	if *calls != 1 {
		t.Fatalf("第一次应该请求一次，实际 %d 次", *calls)
	}

	a.closeExplain()
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()

	if *calls != 1 {
		t.Errorf("第二次打开不该再请求，实际请求了 %d 次", *calls)
	}
	if !a.ex.cached {
		t.Error("第二次打开应该标记为命中缓存")
	}
	if a.ex.loading {
		t.Error("命中缓存时不该处于加载中")
	}
	if !tt.HasText("缓存下来的解释") {
		t.Errorf("缓存内容没显示：%q", tt.Texts())
	}
	if !tt.HasText("缓存") {
		t.Errorf("卡片上应该有缓存标记：%q", tt.Texts())
	}
}

// 解释结果要落进 store，下次启动还能直接命中。
func TestExplainSavesToStore(t *testing.T) {
	srv, _ := fakeLLM(t, "存起来的解释")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)

	key := a.explainKey(a.files[fi], hi)
	got, ok := a.store.Lookup(key)
	if !ok {
		t.Fatal("解释结果没有写进 store")
	}
	if got != "存起来的解释" {
		t.Errorf("缓存内容 = %q", got)
	}
}

// 接口报错时卡片要给出错误和重试，而不是一直转圈。
//
// 卡片上摆的是「出了什么事」那句中文，供应商那句原话收在下面当证据 ——
// 反过来的话，用户在中文界面里只会看到一句英文报错（401 的原文甚至不止
// 一句：SDK 会把请求方法、完整 URL、Request-ID 和一整坨 JSON 拼成一行）。
func TestExplainShowsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)
	before := countText(tt.Texts(), "模型设置…")

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()

	if a.ex.loading {
		t.Error("出错之后不该还在加载")
	}
	if !a.ex.fail.Auth {
		t.Errorf("401 该认成钥匙的问题：%+v", a.ex.fail)
	}
	if !tt.HasText("模型没认这个密钥") {
		t.Errorf("卡片上该有一句说清是什么事的中文：%q", tt.Texts())
	}
	if !tt.HasText("Invalid API key") {
		t.Errorf("供应商的原话该留着当证据：%q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Errorf("卡片上没有重试：%q", tt.Texts())
	}
	// 钥匙不对 —— 该有一条通向设置的路
	if n := countText(tt.Texts(), "模型设置…"); n <= before {
		t.Errorf("该给一个通向模型设置的按钮（「模型设置…」从 %d 个变成 %d 个）", before, n)
	}
}

// 点重试：重新发一次请求，把新的结果拿出来 —— 而不是把上一次的错误
// 钉在卡片上。
func TestExplainRetryAfterError(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"重试问出来的解释\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()
	if !tt.HasText("重试") {
		t.Fatalf("第一趟就该失败：%q", tt.Texts())
	}

	if err := tt.Click("重试"); err != nil {
		t.Fatalf("点重试：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.ex.fail.Title != "" {
		t.Errorf("重试成功之后卡片上不该还挂着错误：%+v", a.ex.fail)
	}
	if !tt.HasText("重试问出来的解释") {
		t.Errorf("重试的结果没显示：%q", tt.Texts())
	}
}

// 上一趟断线前已经吐出来的半截话，重试时得先清掉。
//
// 断线时那半句是要留着显示的（收到多少算多少），但它已经不是这次的答案
// 了 —— 流式输出是往后拼的，不清的话新答案会从这半句后面长出来，用户
// 看到的是一句拼接起来的怪话。
func TestExplainRetryClearsPartialText(t *testing.T) {
	srv, _ := fakeLLM(t, "新的一份")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi := explainFixtures(t, a)
	a.ex = explainState{
		fi: fi, hi: hi,
		key:  a.explainKey(a.files[fi], hi),
		head: a.files[fi].Hunks[hi].Header,
		text: "上一趟的半截话",
		fail: llm.Failure{Title: "连不上模型服务"},
	}

	a.retryExplain()

	// 请求刚发出去、一个片段都还没回来 —— 就看这一刻
	if a.ex.text != "" {
		t.Errorf("重试该先把半截话清掉，实际还留着 %q", a.ex.text)
	}
	settle(t, a)
	if a.ex.text != "新的一份" {
		t.Errorf("重试的答案 = %q，该只有新的一份", a.ex.text)
	}
}

// 一个供应商都没配、环境里也没有 key 时点 ✨：这一趟不发出去（发出去必然
// 失败），卡片直接说清楚该去哪儿配，并给一个通向模型设置的按钮。
//
// 这条曾经是「首屏体验最差的一处」：真实的输出是 SDK 回的一大段英文，
// 讲它按顺序找过哪些凭证来源、建议你去跑 `ant auth login` —— 在一个
// 中文界面里，用户只知道「我点了一下，冒出来一大段看不懂的话」。
func TestExplainWithoutProvider(t *testing.T) {
	// 本机可能真配着 ANTHROPIC_API_KEY，清掉才能测「什么都没有」
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 先数一遍：工具栏上那颗按钮也是「模型设置…」这个名字，卡片上应该
	// 再多出一个来。
	before := countText(tt.Texts(), "模型设置…")

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()

	if a.ex.loading {
		t.Error("没有凭证就不该假装在请求")
	}
	if !tt.HasText("还没有配置模型") {
		t.Fatalf("卡片该说清楚是没配模型：%q", tt.Texts())
	}
	if n := countText(tt.Texts(), "模型设置…"); n <= before {
		t.Errorf("卡片上该给一个通向模型设置的按钮（「模型设置…」从 %d 个变成 %d 个）", before, n)
	}

	// 点它（工具栏那颗文案一样、做的是同一件事，点到哪颗都算数）
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置：%v", err)
	}
	tt.Frame()
	if !a.settingsOn {
		t.Error("点那个按钮应该把模型设置打开")
	}
}

// countText 数一段文字在界面上一共出现了几次。
func countText(texts []string, want string) int {
	n := 0
	for _, s := range texts {
		if s == want {
			n++
		}
	}
	return n
}

// 关掉卡片要掐掉进行中的请求：不然用户走了，token 还在烧。
func TestExplainCloseCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done() // 一直挂着，直到客户端取消
	}))
	defer srv.Close()

	a := richApp(t)
	withProvider(t, a, srv)
	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)

	<-started
	a.closeExplain()
	settle(t, a)

	if a.explainOpen() {
		t.Error("关闭之后卡片不该还开着")
	}
	if a.ex.cancel != nil {
		t.Error("关闭时应该把 cancel 用掉并清空")
	}
}

// 打开卡片期间 diff 列表照常滚动：内嵌卡片跟着行模型走，不该挡住底下的内容。
func TestExplainCardDoesNotBlockScrolling(t *testing.T) {
	srv, _ := fakeLLM(t, "解释")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)
	tt.Frame()

	// 挑一行在这一屏里看得见的代码，滚一段之后它应该往上走了
	const probe = "line 52: 也改了"
	before, ok := tt.Find(probe)
	if !ok {
		t.Fatalf("滚动前找不到 %q：%q", probe, tt.Texts())
	}

	tt.Scroll(640, 400, 0, 300)
	tt.Frame()

	after, ok := tt.Find(probe)
	if !ok {
		// 滚出可视区也算滚动生效了
		return
	}
	if after.Y >= before.Y {
		t.Errorf("卡片打开时 diff 滚不动：%q 的 y 从 %v 变成了 %v", probe, before.Y, after.Y)
	}
}

// 切换仓库/刷新之后，卡片指向的 hunk 可能已经不存在了 ——
// 不能按下标去取，那样会串到别的文件上。
func TestExplainInvalidatedOnReload(t *testing.T) {
	srv, _ := fakeLLM(t, "解释")
	a := richApp(t)
	withProvider(t, a, srv)

	fi, hi := explainFixtures(t, a)
	a.openExplain(fi, hi)
	settle(t, a)

	if !a.isExplaining(fi, hi) {
		t.Fatal("刚打开时应该认得这个 hunk")
	}
	// 文件列表换了，下标指向别处
	a.files = nil
	if a.isExplaining(fi, hi) {
		t.Error("文件没了之后不该还认为是同一个 hunk")
	}
}

// 设置对话框能开能关，并且填的东西能存下来。
func TestSettingsDialogSavesProviders(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置：%v", err)
	}
	tt.Frame()
	if !a.settingsOn {
		t.Fatal("设置对话框没打开")
	}
	if !tt.HasText("模型设置") {
		t.Errorf("对话框内容没画出来：%q", tt.Texts())
	}

	// 加一个 DeepSeek，改掉 API key，保存
	if err := tt.Click("DeepSeek"); err != nil {
		t.Fatalf("点预设：%v", err)
	}
	tt.Frame()
	if i := a.editCfg.Active; a.editCfg.Providers[i].BaseURL != "https://api.deepseek.com" {
		t.Errorf("预设没填进 Base URL：%+v", a.editCfg.Providers[i])
	}

	a.editCfg.Providers[a.editCfg.Active].APIKey = "sk-test"
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存：%v", err)
	}
	tt.Frame()

	if a.settingsOn {
		t.Error("保存之后对话框应该关掉")
	}
	p := a.store.Provider()
	if p == nil || p.APIKey != "sk-test" {
		t.Errorf("设置没写进 store：%+v", p)
	}
}

// 视图默认左右对照（split）模式。
func TestSplitSettingDefault(t *testing.T) {
	a := richApp(t)
	settle(t, a)
	if !a.split {
		t.Error("默认应该开启 split 模式")
	}
}
