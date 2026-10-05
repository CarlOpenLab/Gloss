// Command desktop 是 diff-explainer 的桌面版：一个 mygo 原生 UI 的 Git GUI，
// 核心是逐 hunk 的 AI 解释。
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Diff Explainer",
			Width:     1280,
			Height:    860,
			MinWidth:  720,
			MinHeight: 420,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// app 是窗口状态的持有者。view 每帧从它构建界面，只在有变化时才跑。
type app struct {
	win *mygo.Window
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "Diff Explainer").FontSize(24).Bold()
		ui.Text(c, "mygo 原生桌面版 —— 骨架就位").TextColor(t.TextMuted)
	})
}
