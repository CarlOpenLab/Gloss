package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 这个文件是临时的人工检查工具：把界面渲染成 PNG，用来肉眼确认排版。
func TestDumpScreenshot(t *testing.T) {
	if os.Getenv("SHOT") == "" {
		t.Skip("设置 SHOT=1 才跑")
	}
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	img := tt.Image()
	f, err := os.Create("/tmp/de-shot-light.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}

	tt.SetDark(true)
	tt.Frame()
	f2, err := os.Create("/tmp/de-shot-dark.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	if err := png.Encode(f2, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
