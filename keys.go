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
		a.closeTab(a.activeTab)
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
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyA) {
		if a.tab == 0 && a.repoReady() && a.op == gitrepo.OpNone {
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
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		// 焦点给主区那个文件筛选框，不是侧栏的分支筛选框 —— review 时
		// 想找的是文件。也不再把侧栏拽开：那个框现在筛的是分支，开不开
		// 和「找文件」没关系。
		a.focusFilter = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		if a.viewMode == viewModeTerm {
			a.clearCurrentTerminal()
		} else {
			a.openPalette()
		}
	}
	if c.Shortcut(ui.Cmd, ui.Key1) {
		if a.viewMode == viewModeGit {
			a.setTab(0)
		} else {
			a.setViewMode(viewModeGit)
		}
	}
	if c.Shortcut(ui.Cmd, ui.Key2) {
		if a.viewMode == viewModeGit {
			a.setTab(1)
		} else {
			a.setViewMode(viewModeTerm)
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

	// ⌘I 触发 AI 提交信息生成。
	if c.Shortcut(ui.Cmd, ui.KeyI) {
		if a.stagedCount() > 0 && !a.aiCommitLoading {
			a.startAICommit()
		}
	}

	// ⌘⏎ 提交。
	//
	// 用 ⌘ 而不是裸回车：输入框里回车是换行，提交信息本来就常常是多行的。
	// 框架的编辑器对 KeyEnter 只在「没有修饰键」时才认（ui/editor.go 的
	// wants），⌘⏎ 因此会漏给窗口这一层 —— 正合惯例。
	if c.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.doCommit()
	}

	// 远程三条。可用性判断在 do* 里面（没有远程时不能跑 —— 那种情况下按钮
	// 和菜单都是灰的，快捷键不该有第二条暗路）。
	//
	// 键位和 menu.go 里「远程」菜单那三条一致：真实 macOS 应用里菜单先把键
	// 吃掉，这条路走不到；测试和没有菜单的环境走的才是这里。
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
		a.doFetch()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		a.doPull()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyU) {
		a.doPush()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		a.openCompareDialog("")
	}

	// J / K 与方向键导航。
	// 历史 tab（tab == 1）：浏览历史提交列表时，J / K 与 Down / Up 移动选中的提交。
	// 改动 tab（tab == 0）：J / K 跳上下一段改动（hunk），↑ / ↓ 在改动文件
	// 列表里上下移 —— 一层管 diff 正文、一层管文件栏，两个键不冲突。
	// 在模态对话框或输入框获取焦点时不响应。
	if !a.modalOn() {
		if a.tab == 1 && len(a.commits) > 0 {
			if c.Shortcut(0, ui.KeyJ) || c.Shortcut(0, ui.KeyDown) {
				a.navCommit(1)
			} else if c.Shortcut(0, ui.KeyK) || c.Shortcut(0, ui.KeyUp) {
				a.navCommit(-1)
			}
		} else if a.tab == 0 && a.state == loadReady && len(a.files) > 0 {
			// 方向键先给文件栏：没有选中过任何文件时这一下就是「选中第一个」，
			// 有了选中就在列表里上下移。J / K 与它分开，仍然跳 hunk。
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

// jumpHunk 把列表滚到上一段/下一段改动。
//
// 出发点优先取上一次跳到的那一段，它看不见了就退回视口顶行 ——
// 用户手动滚动、展开未改动区间、切视图都会让「我记得跳到哪了」过期，
// 而滚动位置永远是真的。细节见 hunkAnchor。
func (a *app) jumpHunk(dir int) {
	if a.rowsDirty {
		a.rebuildRows()
	}
	if len(a.rows) == 0 {
		return
	}

	target := hunkRowNear(a.rows, a.hunkAnchor(), dir)
	if target < 0 {
		return
	}
	a.hunkCur = a.rows[target].key
	a.diffList.ScrollTo(target, ui.Start)
}

// hunkAnchor 是这次跳转的出发点。
//
// 默认取上一次跳到的那一段（它还看得见的话），否则取视口顶行。
//
// 为什么不单纯用视口顶行：视口不一定跟得上。文件头钉在列表顶上时顶行
// 一直是它；diff 滚到底之后视口也不再动。这两种情况都会让「下一段」
// 每次都算出同一个目标，连按 J 就卡在原地，所以要记着跳到哪了。
//
// 反过来，用户要是滚到了别处，那一段就滚出屏幕了（滑出去的会被列表
// 扔掉，下标对不上），于是自然回到「以视口为准」。
func (a *app) hunkAnchor() int {
	first, last := a.diffList.Visible()
	if i := a.rowIndexOf(a.hunkCur); i >= first && i <= last {
		return i
	}
	a.hunkCur = ""
	return a.topRow()
}

// rowIndexOf 找某一行 key 的下标，没有返回 -1。
func (a *app) rowIndexOf(key string) int {
	if key == "" {
		return -1
	}
	for i := range a.rows {
		if a.rows[i].key == key {
			return i
		}
	}
	return -1
}

// topRow 是视口顶上的那一行。
//
// 钉在列表顶部的文件头不算 —— 它下面那一行才是真正顶在眼前的内容。
func (a *app) topRow() int {
	first, last := a.diffList.Visible()
	if first >= 0 && first < last && a.rows[first].kind == rowFileHeader {
		return first + 1
	}
	return first
}

// hunkRowNear 找 from 之后（dir > 0）或之前（dir < 0）最近的一段改动。
// 到头了就绕到另一端：一路按 J 会循环，手不用离开键盘。
func hunkRowNear(rows []row, from, dir int) int {
	if dir > 0 {
		for i := from + 1; i < len(rows); i++ {
			if rows[i].kind == rowHunkHeader {
				return i
			}
		}
		return firstHunkRow(rows, from)
	}
	for i := min(from, len(rows)-1) - 1; i >= 0; i-- {
		if rows[i].kind == rowHunkHeader {
			return i
		}
	}
	return lastHunkRow(rows)
}

// firstHunkRow 是 from 之前的第一段改动。到头了绕回开头时用。
func firstHunkRow(rows []row, before int) int {
	for i := 0; i < len(rows) && i < before; i++ {
		if rows[i].kind == rowHunkHeader {
			return i
		}
	}
	return -1
}

// lastHunkRow 是最后一段改动。
func lastHunkRow(rows []row) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].kind == rowHunkHeader {
			return i
		}
	}
	return -1
}
