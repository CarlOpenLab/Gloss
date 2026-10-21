// Command Gloss 是一个跨平台的 Git GUI（macOS / Linux / Windows），
// 主线是让 review 大段改动变轻松：逐 hunk 阅读、按块暂存、AI 风险预检。
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/store"
)

func main() {
	a := newApp()

	// 自动更新：后台每天检查一次 GitHub Release，有新版本时弹出
	// Sparkle 风格窗口（release notes + 安装/稍后/跳过）。dev 构建
	// 和包管理器安装（.deb 等）不会触发。
	mygo.Use(updater.Plugin)

	mygo.App.WhenReady(func() {
		// 设置和状态都在应用数据目录里。拿不到目录（极少见）
		// 就退回内存里的一份空 store —— 设置存不下，但应用照常能用。
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Printf("拿不到数据目录，这次的设置不会被保存：%v", err)
		}
		s := store.Open(dir)
		a.attachStore(s)

		// 菜单栏。自己搭一份的理由写在 menu.go 顶上：框架默认那份会把
		// ⌘R / ⌘+ / ⌘− / ⌘0 当成网页的「重新载入 / 缩放」占掉，而这几个
		// 键在 Gloss 里另有主人。
		mygo.App.SetMenu(buildAppMenu(a))

		opts := mygo.WindowOptions{
			Title:     "Gloss",
			Width:     1280,
			Height:    860,
			MinWidth:  720,
			MinHeight: 420,
			StateKey:  "main",
			// 标题栏不画系统的那条，改由 view 自己画（见 app.go 的 toolbar）。
			TitleBarStyle: mygo.TitleBarHidden,
			Vibrancy:      mygo.VibrancySidebar,
			Content:       ui.View(a.view),
		}
		if isMacPlatform() {
			// macOS：连系统红绿灯一起藏掉，由 view 自绘（见 shell_style.go
			// 的 windowTrafficLights）。Frameless 只收走标题栏与那三颗按钮，
			// 窗口仍是可缩放、可拖动、有阴影的普通窗口。
			opts.Frameless = true
		} else {
			// 其他平台保留系统按钮，位置照旧贴着标题栏中线。
			//
			// 把红绿灯从系统默认的位置（贴着窗口顶边那条 32 高的带子）挪到
			// 工具栏的正中间。系统默认位置是给「32 高的标题栏」留的，而工具栏
			// 把红绿灯对齐到 38 高的顶栏正中间（关闭按钮高 14，12 + 14 + 12 = 38）。
			opts.TrafficLightPosition = &mygo.Point{X: 18, Y: 12}
		}
		win := mygo.NewWindow(opts)
		a.win = win
		a.syncAppIcon()
		// 后台 goroutine 改状态必须走这里：它把闭包投到主线程执行，
		// 并请求重绘。直接改字段不会触发重绘。
		a.dispatch = win.Update
		// git 状态自动刷新（每次打开仓库后启动，改动/历史/侧栏自己跟上）。
		a.startAutoRefresh()
		// 冲突卡片上的「在编辑器打开」：交给系统默认应用。
		a.openPath = func(p string) { _ = mygo.Shell.OpenPath(p) }
		// 文件行右键的「在访达中显示」：在文件管理器里亮出这个文件。
		a.revealPath = func(p string) { mygo.Shell.ShowItemInFolder(p) }
		// 关于界面的「访问官方网站」等：交给系统默认浏览器。
		a.openURL = func(u string) { _ = mygo.Shell.OpenExternal(u) }

		// 关窗前把提交框里还没提交的消息存成草稿，并优雅关闭所有终端会话。
		win.OnClose(func(*mygo.CloseEvent) {
			a.stopAutoRefresh()
			a.saveCommitDraft()
			a.closeAllTerminals()
		})

		// 接着上次打开的标签页（或最后打开的仓库），省得每次启动都要重新选一遍
		if len(a.tabs) > 0 {
			a.dispatch(func() {
				a.activeTab = 0
				a.restoreActiveTab()
				a.openRepo(a.tabs[0].path)
			})
		} else if p := s.State().LastRepo; p != "" {
			a.dispatch(func() { a.openRepo(p) })
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
