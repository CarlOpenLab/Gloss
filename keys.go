package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 窗口级快捷键集中在 handleKeys 里，view 一开头就调用。
//
// c.Shortcut 报的是「本帧有没有按下这个键」，注册即认领 —— 一个键只能被
// 认领一次，所以每处只问一次。它在对话框打开时会自动让位（ui 内部的
// insideModal），输入框正在编辑时也会先把键吃掉（焦点元素先收到 keyDown），
// 所以「在筛选框里打 j」不会跳 hunk。

// codeSizeMin/Max/Step 是 ⌘+/⌘− 的范围。小于 9 看不清，大于 22 一行放不下
// 几个字。
const (
	codeSizeMin  = 9
	codeSizeMax  = 22
	codeSizeStep = 1
)

func (a *app) handleKeys(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		a.reload()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyB) {
		a.toggleSidebar()
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.chooseFolder()
	}
	if c.Shortcut(ui.Cmd, ui.KeyT) {
		a.chooseFolder()
	}
	if c.Shortcut(ui.Cmd, ui.KeyW) {
		// 终端视图里关当前会话，别处关项目标签页（见 closeActive）。
		a.closeActive()
	}
	// 终端会话的 ⌘⇧T。⌘T 已经是「新建项目标签页」、⌘W 是「关闭标签页」、
	// Ctrl+~ / ⌘J 是视图切换 —— 这几个都动不得（它们在别的应用里也是那个
	// 意思）。⌘⇧T 是同一条规律的上一层：浏览器和 iTerm 里「⇧ 加在 T 上」
	// 就是「再开一个（同类的）标签」，不用另学一套。
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyT) {
		a.newTerminalTab()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyBackquote) || c.Shortcut(ui.Cmd, ui.KeyJ) {
		a.toggleViewMode()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft) || c.Shortcut(ui.Cmd|ui.Alt, ui.KeyLeft) {
		a.prevTab()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight) || c.Shortcut(ui.Cmd|ui.Alt, ui.KeyRight) {
		a.nextTab()
	}
	// 提交栏那一套（暂存全部 / 提交 / AI 起名）只在 Git 面板里认 —— 判据和
	// 菜单、命令栏共用一份（gitPanelLive）：终端视图里屏幕上没有提交栏，
	// 按下去是看不见地改仓库。
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyA) {
		if a.gitPanelLive() && a.tab == 0 && a.repoReady() && a.op == gitrepo.OpNone {
			if a.unstagedCount() > 0 {
				a.stageAll()
			} else if a.stagedCount() > 0 {
				a.unstageAll()
			}
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		a.newBranchAtHead()
	}
	// ⌘P 只在 Git 面板里认：终端模式下 diff 根本不在屏幕上，把 focusFilter
	// 挂在那儿，等用户切回 Git 面板，一个不知道什么时候按下的 ⌘P 会突然把
	// 焦点抢走。（⌘F/⌘G 归 DiffView 自己的查找：焦点在它里面时由编辑器
	// 先收键，那份查找跟着这一栏活着。）
	if a.viewMode == viewModeGit {
		if c.Shortcut(ui.Cmd, ui.KeyP) {
			// 焦点给主区那个文件筛选框，不是侧栏的分支筛选框 —— review 时
			// 想找的是文件。也不再把侧栏拽开：那个框现在筛的是分支，开不开
			// 和「找文件」没关系。
			//
			// ⌘F = 找内容（改动区自己的查找栏）、⌘P = 找文件，是编辑器和
			// Git 客户端的通行分法。
			a.focusFilter = true
		}
	}
	// `/` 聚焦提交筛选框。只在历史 tab 认：改动 tab 里没有这个东西，
	// 挂上去按下去没反应更糊。焦点已经在框里的时候框架先把键给输入框
	// （焦点元素先收到 keyDown），所以打 `/` 不会自己抢自己。
	if a.viewMode == viewModeGit && a.tab == 1 && !a.modalOn() {
		if c.Shortcut(0, ui.KeySlash) {
			a.focusCommitFilter = true
		}
	}
	// ⌘K：Git 面板里是命令栏，终端视图里是清屏。
	//
	// 终端视图里一个会话都没有（从没开过、或最后一个被 × / ⌘W 关掉）时没有
	// 东西可清 —— 那一下退回命令栏，不让 ⌘K 在空终端页上变成死键。
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		s := a.activeSession()
		switch {
		case a.viewMode != viewModeTerm:
			a.openPalette()
		case s != nil && s.term != nil:
			a.clearCurrentTerminal()
		default:
			a.openPalette()
		}
	}
	// ⌘1~⌘4 依次是工具栏上那四格：改动 / 历史 / 终端 / 编辑器。
	//
	// ⌘1 ⌘2 以前按「当前是不是 Git 面板」有两种意思（在 Git 面板里切
	// 改动/历史，在别处切回 Git 面板），得先看一眼自己在哪儿才知道按下去
	// 会去哪儿；四格并成一条之后一个键就一种意思。
	if c.Shortcut(ui.Cmd, ui.Key1) {
		a.setSection(sectionChanges)
	}
	if c.Shortcut(ui.Cmd, ui.Key2) {
		a.setSection(sectionHistory)
	}
	if c.Shortcut(ui.Cmd, ui.Key3) {
		a.setSection(sectionTerminal)
	}
	if c.Shortcut(ui.Cmd, ui.Key4) {
		a.setSection(sectionEditor)
	}
	// 「编辑器」视图里的 ⌘S 存回工作区。编辑器自身把 ⌘S 让了出来
	// （State.Bindings 里绑成 NoCommand），所以这里能接住。
	if a.viewMode == viewModeEditor && c.Shortcut(ui.Cmd, ui.KeyS) {
		a.saveEditorPane()
	}
	// 改动区里的 ⌘S：右侧改过就存下来（冲突文件顺手 git add）。同样是因为
	// DiffView 把 ⌘S 让给了应用（见 review.go 里那份 Bindings）。
	if a.viewMode == viewModeGit && c.Shortcut(ui.Cmd, ui.KeyS) {
		if a.review.editable && a.review.new != nil && a.review.new.Modified() {
			a.saveReview()
		}
	}
	// 字号从「当前生效的字号」上加减，而不是从 a.codeSize 上 ——
	// 后者 0 表示「用默认值」，直接 +1 会被夹到下限，第一次按就跳到底。
	//
	// 放大要认两个键：⌘= 和 ⌘⇧=。键盘上的加号本来是 ⇧=，可框架配对
	// 修饰键是全等的（Cmd 不等于 Cmd|Shift），只写一个的话，用户照着
	// 菜单里的「⌘+」按下去什么都不会发生。
	if c.Shortcut(ui.Cmd, ui.KeyEqual) || c.Shortcut(ui.Cmd|ui.Shift, ui.KeyEqual) {
		a.setCodeSize(a.codeSizeValue() + codeSizeStep)
	}
	if c.Shortcut(ui.Cmd, ui.KeyMinus) {
		a.setCodeSize(a.codeSizeValue() - codeSizeStep)
	}
	if c.Shortcut(ui.Cmd, ui.Key0) {
		a.setCodeSize(0)
	}
	// ⌘, 开设置 —— macOS 上这个键是「偏好设置」的意思，按遍所有 Mac 应用
	// 都是它。原来这个键被接去放大字号了（顺手把 ⌘= 那一路多认了一个键），
	// 于是用户想开设置，得到的是代码变大一号。
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.openSettings()
	}

	// ⌘I 触发 AI 提交信息生成。和 ⌘⇧A、⌘⏎ 一样只在 Git 面板里认
	// （gitPanelLive）：终端视图里发一次请求、把消息填进一个看不见的
	// 提交框，不如不认。
	if c.Shortcut(ui.Cmd, ui.KeyI) {
		if a.gitPanelLive() && a.stagedCount() > 0 && !a.aiCommitLoading {
			a.startAICommit()
		}
	}

	// ⌘⏎ 提交。
	//
	// 用 ⌘ 而不是裸回车：输入框里回车是换行，提交信息本来就常常是多行的。
	// 框架的编辑器对 KeyEnter 只在「没有修饰键」时才认（ui/editor.go 的
	// wants），⌘⏎ 因此会漏给窗口这一层 —— 正合惯例。
	//
	// 同样只在 Git 面板里认：终端视图里屏幕上没有提交栏，这一下会静默提交
	// （opBanner / Toast 也不在那一视图画），用户看到的是「什么都没发生」。
	if c.Shortcut(ui.Cmd, ui.KeyEnter) && a.gitPanelLive() {
		a.doCommit()
	}

	// 远程三条。可用性判断在 do* 里面（没有远程时不能跑 —— 那种情况下按钮
	// 和菜单都是灰的，快捷键不该有第二条暗路）。
	//
	// 键位和 menu.go 里「远程」菜单那三条一致：真实 macOS 应用里菜单先把键
	// 吃掉，这条路走不到；测试和没有菜单的环境走的才是这里。
	//
	// 和提交那一套同一个理由只认 Git 面板（gitPanelLive）：远程操作跑起来
	// 动辄几秒，而它的横幅、toast、进度条全在 Git 面板里画 —— 终端视图里
	// 按下去是干等一个没人汇报的结果。⌘⇧D 不设闸：它开的是模态框，任何
	// 视图下都看得见。
	if a.gitPanelLive() {
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
			a.doFetch()
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
			a.doPull()
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyU) {
			a.doPush()
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		a.openCompareDialog("")
	}

	// J / K 与方向键导航。
	// 历史 tab（tab == 1）：浏览历史提交列表时，J / K 与 Down / Up 移动选中的提交。
	// 改动 tab（tab == 0）：J / K 跳上下一段改动（hunk），↑ / ↓ 在改动文件
	// 列表里上下移 —— 一层管 diff 正文、一层管文件栏，两个键不冲突。
	// 在模态对话框或输入框获取焦点时不响应。
	//
	// 在模态对话框、输入框获取焦点、或「编辑器」视图时不响应 —— 后者的键位
	// 归给编辑器里的光标（DiffView 里的光标也算：它有自己的一套键）。
	if !a.modalOn() && a.viewMode == viewModeGit {
		if a.tab == 1 && len(a.commits) > 0 {
			if c.Shortcut(0, ui.KeyJ) || c.Shortcut(0, ui.KeyDown) {
				a.navCommit(1)
			} else if c.Shortcut(0, ui.KeyK) || c.Shortcut(0, ui.KeyUp) {
				a.navCommit(-1)
			}
		} else if a.tab == 0 && a.state == loadReady && len(a.files) > 0 {
			// 方向键给文件栏：没有选中过任何文件时这一下就是「选中第一个」，
			// 有了选中就在列表里上下移。J / K 与它分开，在这一个文件里跳段。
			if c.Shortcut(0, ui.KeyDown) {
				a.navFile(1)
			} else if c.Shortcut(0, ui.KeyUp) {
				a.navFile(-1)
			} else if c.Shortcut(0, ui.KeyJ) {
				a.jumpHunk(1)
			} else if c.Shortcut(0, ui.KeyK) {
				a.jumpHunk(-1)
			}
		}
	}
}

func (a *app) toggleSidebar() {
	a.setSidebar(!a.sidebarOn)
}

// setCodeSize 调整代码字号。传 0 表示回到默认值。
func (a *app) setCodeSize(size float32) {
	if size == 0 {
		a.codeSize = 0
	} else {
		a.codeSize = clamp(size, codeSizeMin, codeSizeMax)
	}
	a.saveViewSettings()
}

func (a *app) codeSizeValue() float32 {
	if a.codeSize <= 0 {
		return defSize
	}
	return a.codeSize
}

// jumpHunk 跳到下一个/上一个改动块，走到头绕到另一端。
//
// 段是 DiffView 自己算出来的（它按两侧原文切，和 git 的 -U3 段不保证一样），
// 所以这里问它要一份清单，再让它把光标和视口挪过去 —— Gloss 不再自己维护
// 一份行模型，也就没有「行号对不上」这件事。
//
// 还没跳过的时候（hunkIdx < 0）往下落到第一段、往上落到最后一段：
// 「下一段」从头开始数是第一段，不是第二段。
func (a *app) jumpHunk(dir int) {
	s := &a.review
	if s.diff == nil {
		return
	}
	n := len(s.diff.Hunks())
	if n == 0 {
		return
	}
	i := s.hunkIdx
	switch {
	case i < 0 || i >= n:
		if dir > 0 {
			i = 0
		} else {
			i = n - 1
		}
	case dir > 0:
		i = (i + 1) % n
	default:
		i = (i - 1 + n) % n
	}
	s.hunkIdx = i
	s.diff.GoToHunk(i)
}
