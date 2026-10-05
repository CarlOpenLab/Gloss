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
}

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
