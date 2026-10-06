package main

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 这个文件是人工检查工具：把界面渲染成 PNG，用眼看排版对不对。
// 只在 SHOT=1 时跑，不进常规测试。
func TestDumpScreenshot(t *testing.T) {
	if os.Getenv("SHOT") == "" {
		t.Skip("设置 SHOT=1 才跑")
	}
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	// 先空跑一帧让列表把行高量出来，否则首帧的字号/行高还没稳定
	tt.Frame()

	writePNG(t, "/tmp/de-shot-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-dark.png", tt.Image())

	// 再单独看一眼带语法高亮的那个文件（它在列表靠下的位置，默认看不到）
	a.filter = "router"
	tt.Frame()
	writePNG(t, "/tmp/de-shot-code-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-code-dark.png", tt.Image())

	// split 视图：左侧旧、右侧新，配不上的那一侧留白
	a.filter = ""
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
	a.ex = explainState{}

	// 模型设置对话框
	a.openSettings()
	a.editCfg.Providers[0].APIKey = "sk-ant-xxxxxxxxxxxxxxxx"
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings.png", tt.Image())
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
