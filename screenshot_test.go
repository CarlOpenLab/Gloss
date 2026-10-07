package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// realTitleBar 是真实窗口里红绿灯占的地方（在本机量出来的：窗口用
// TitleBarHidden + TrafficLightPosition{X:20,Y:19}）。无头渲染默认没有
// 窗口控件，不摆上它就看不出侧栏顶上会不会被关闭按钮压住。
var realTitleBar = ui.TitleBar{Height: 52, Left: 80}

// still 关掉这个 tester 上的动效。
//
// 截图要的是「稳下来之后长什么样」，而它在无头环境下不一定稳得下来：
// 淡入、颜色过渡这些都按真实时间走（tester 没有自己的钟），而两张截图
// 之间只隔几微秒 —— 拍到的是动画的第一帧：卡片刚滑出来还压着 diff、
// 底色刚亮起还几乎全透明。接上系统的「减弱动态效果」这一档，框架就会
// 直接摆到终点，拍到的正是用户看惯了的那一帧。
func still(t *ui.Tester) {
	t.SetPreferences(ui.Preferences{ReduceMotion: true})
}

// 这个文件是人工检查工具：把界面渲染成 PNG，用眼看排版对不对。
// 只在 SHOT=1 时跑，不进常规测试。
func TestDumpScreenshot(t *testing.T) {
	if os.Getenv("SHOT") == "" {
		t.Skip("设置 SHOT=1 才跑")
	}
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	// 先空跑一帧让列表把行高量出来，否则首帧的字号/行高还没稳定
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	writePNG(t, "/tmp/de-shot-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-dark.png", tt.Image())

	// 再单独看一眼带语法高亮的那个文件（它在列表靠下的位置，默认看不到）
	a.fileFilter = "router"
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-code-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-code-dark.png", tt.Image())

	// split 视图：左侧旧、右侧新，配不上的那一侧留白
	a.fileFilter = ""
	a.split = true
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-split.png", tt.Image())

	// ✨ 解释卡片：定位、Markdown 排版、头尾两块
	a.split = false
	fi, hi := explainFixtures(t, a)
	a.ex = explainState{
		fi:   fi,
		hi:   hi,
		key:  a.explainKey(a.files[fi], hi),
		head: a.files[fi].Hunks[hi].Header,
		text: sampleExplanation,
	}
	a.ex.anchor = ui.Rect{X: 1120, Y: 180, W: 88, H: 22}
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-card.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-card-dark.png", tt.Image())

	// 卡片翻到 ✨ 按钮上方（按钮贴底时）
	a.ex.anchor = ui.Rect{X: 1120, Y: 800, W: 88, H: 22}
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-card-up.png", tt.Image())

	// 还没有配模型：卡片讲的是该去哪儿配，不是「失败了」
	a.ex = explainState{
		fi: fi, hi: hi,
		key:     a.explainKey(a.files[fi], hi),
		head:    a.files[fi].Hunks[hi].Header,
		needKey: true,
	}
	a.ex.anchor = ui.Rect{X: 1120, Y: 180, W: 88, H: 22}
	tt.Frame()
	writePNG(t, "/tmp/de-shot-card-nokey.png", tt.Image())

	// 请求真的失败了。这里特意走完整条链路 —— 假接口回一个真实的 Anthropic
	// 错误信封，SDK 解析、我们收成 statusError、Describe 翻译成人话 —— 拍
	// 到的才是「密钥填错了的用户」真正会看到的那张卡，而不是我手拼一张。
	badKey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer badKey.Close()
	_, apiErr := llm.StreamExplanation(context.Background(), &llm.Provider{
		Type: "anthropic", BaseURL: badKey.URL, APIKey: "sk-ant-wrong", Model: "claude-sonnet-5-5",
	}, "系统提示", "用户提示", func(string) {})
	if apiErr == nil {
		t.Fatal("假接口该让这一趟失败")
	}
	a.ex = explainState{
		fi: fi, hi: hi,
		key:  a.explainKey(a.files[fi], hi),
		head: a.files[fi].Hunks[hi].Header,
		fail: llm.Describe(apiErr),
	}
	a.ex.anchor = ui.Rect{X: 1120, Y: 180, W: 88, H: 22}
	tt.Frame()
	writePNG(t, "/tmp/de-shot-card-error.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-card-error-dark.png", tt.Image())
	tt.SetDark(false)
	a.ex = explainState{}

	// 模型设置对话框
	a.openSettings()
	a.editCfg.Providers[0].APIKey = "sk-ant-xxxxxxxxxxxxxxxx"
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings.png", tt.Image())
	a.settingsOn = false

	// 折叠：第一个文件标成已 review，它就在 diff 里折起来了。
	// 前面试筛选的时候列表滚下去过，这里先滚回顶上。
	a.toggleViewed(a.files[0])
	a.diffList.ScrollTo(0, ui.Start)
	tt.Frame()
	tt.Frame()
	writePNG(t, "/tmp/de-shot-folded.png", tt.Image())
	a.toggleViewed(a.files[0])

	// ⌘K 命令栏
	a.openPalette()
	tt.Frame()
	writePNG(t, "/tmp/de-shot-palette.png", tt.Image())
	a.closePalette()

	// 空状态：没打开仓库时第一眼看到的东西
	empty := newApp()
	et := ui.NewTester(empty.view, 1280, 860)
	still(et)
	et.Frame()
	et.SetTitleBar(realTitleBar)
	et.Frame()
	writePNG(t, "/tmp/de-shot-empty.png", et.Image())
	et.SetDark(true)
	et.Frame()
	writePNG(t, "/tmp/de-shot-empty-dark.png", et.Image())

	// 打开了一个不是仓库的文件夹：首屏最常踩的那个坑 —— 拖错了文件夹，
	// 看到的是一句 git 的英文。这里看的就是那句话摆在哪儿、多大。
	bad := newApp()
	bad.openRepo(t.TempDir())
	settle(t, bad)
	bt := ui.NewTester(bad.view, 1280, 860)
	still(bt)
	bt.Frame()
	bt.SetTitleBar(realTitleBar)
	bt.Frame()
	writePNG(t, "/tmp/de-shot-open-failed.png", bt.Image())

	// 窄窗口：窗口的最小宽度（main.go 里设的 720）。工具栏摆不下时
	// 右边的动作按钮该收进「»」菜单，而不是被裁掉或者把这一行挤折。
	tt.SetSize(720, 620)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-narrow.png", tt.Image())
	tt.SetSize(1280, 860)
	tt.Frame()

	// 历史 tab：上半是提交列表（带拓扑图），下半是该提交的「文件列表 + diff」。
	// 用带分支的仓库 —— 一条直线的仓库看不出泳道画得对不对，也看不出
	// 主线换泳道之后颜色还连不连续。
	hist := graphApp(t)
	ht := ui.NewTester(hist.view, 1280, 860)
	still(ht)
	ht.Frame()
	ht.SetTitleBar(realTitleBar)
	hist.setTab(1)
	settle(t, hist)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-history.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-history-dark.png", ht.Image())

	// 换个合并提交、切到 Split：上栏那一行连出的线最多，下栏左右对照也出来了
	hist.split = true
	if err := ht.Click("合并支线一"); err != nil {
		t.Fatal(err)
	}
	settle(t, hist)
	ht.SetDark(false)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-history-split.png", ht.Image())
}

// sampleExplanation 是一段有代表性的模型输出：一句话总结、要点、行内代码、
// 代码块、加粗 —— 卡片上会出现的排版都在这。（用双引号写，省得反引号
// 和 Go 的原始字符串字面量打架。）
const sampleExplanation = "这段改动给 `route` 注册了健康检查端点，并新增了 `maxBody` 常量。\n" +
	"\n" +
	"- **做了什么**：把 `/health` 挂到 mux 上；`maxBody` 限制请求体大小\n" +
	"- **为什么**：原来的 mux 是空的，服务起来后没有任何可探活的入口\n" +
	"- **注意**：`maxBody` 现在还没有被真正用上，可能在后续提交里接进 handler\n" +
	"\n" +
	"改动位置在第 133 行附近：\n" +
	"\n" +
	"```go\n" +
	"const maxBody = 1 << 20\n" +
	"```\n" +
	"\n" +
	"总体是一次无风险的增量修改。"

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
