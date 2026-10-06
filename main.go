// Command Gloss 是一个原生 macOS 的 Git GUI，
// 核心是逐 hunk 的 AI 解释 —— review 大段代码时哪里不懂点哪里。
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/store"
)

func main() {
	a := newApp()

	mygo.App.WhenReady(func() {
		// 设置、状态和解释缓存都在应用数据目录里。拿不到目录（极少见）
		// 就退回内存里的一份空 store —— 设置存不下，但应用照常能用。
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Printf("拿不到数据目录，这次的设置不会被保存：%v", err)
		}
		s := store.Open(dir)
		a.attachStore(s)

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Gloss",
			Width:     1280,
			Height:    860,
			MinWidth:  720,
			MinHeight: 420,
			StateKey:  "main",
			// 标题栏不画系统的那条，改由 view 自己画（见 app.go 的 toolbar），
			// 这样红绿灯能和工具栏内容在同一行里对齐。
			TitleBarStyle: mygo.TitleBarHidden,
			// 把红绿灯从系统默认的位置（贴着窗口顶边那条 32 高的带子）挪到
			// 工具栏的正中间。系统默认位置是给「32 高的标题栏」留的，而工具栏
			// 有 52 高，照默认放会偏高，和右边的按钮不在一条水平线上。
			// Y 是关闭按钮顶边到窗口顶边的距离，工具栏高度会跟着它算出来。
			TrafficLightPosition: &mygo.Point{X: 20, Y: 19},
			Vibrancy:             mygo.VibrancySidebar,
			Content:              ui.View(a.view),
		})
		a.win = win
		// 后台 goroutine 改状态必须走这里：它把闭包投到主线程执行，
		// 并请求重绘。直接改字段不会触发重绘。
		a.dispatch = win.Update

		// 接着上次的仓库打开，省得每次启动都要重新选一遍
		if p := s.State().LastRepo; p != "" {
			a.dispatch(func() { a.openRepo(p) })
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
