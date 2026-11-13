package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// editorTabState 是「编辑器」视图里的一个文件 tab。
//
// 一个 tab 一条完整的编辑器状态：缓冲区、git 基线、语言、待兑现的搜索命中
// 都在它自己身上。切 tab 只换 active，不动这些；异步读盘写回的是 tab 对象
// 本身（seq + dead 认领），所以切走项目再回来不会永远转圈。
type editorTabState struct {
	path    string
	section gitrepo.Section // 打开时的段落，用于选 git 基线（index 还是 HEAD）
	loading bool
	err     string
	buffer  *editor.Buffer
	base    *editor.Buffer
	code    *editor.State
	// img 是图片文件的预览（见 imagereview.go）。图片进不了编辑器：
	// 没有文本缓冲区，这里摆的是解好的位图，保存/还原都不适用。
	img     *ui.Bitmap
	imgMeta string
	// pendingHit 是「打开后兑现」的一次性跳转请求（编辑器栏搜索结果的跳转
	// 用）：缓冲还在读盘时先记下点中的那条命中，dispatch 就位后选中它 ——
	// Line 为 0 表示没有待兑现的。
	pendingHit wsSearchHit
	// seq 给每次读盘编号；dead 标记这个 tab 已经被关掉/所属项目被清空。
	// 迟到的回调两者对不上就什么都不碰。
	seq  int
	dead bool
}

// editorPaneState 是「编辑器」视图的一列文件 tab 加当前激活下标。
//
// active 为 -1 表示没有打开的文件。它和 codeEdit（文件右键「编辑代码」弹的
// 对话框）是两份独立状态：那边是一次性的，这边跟着项目走。
type editorPaneState struct {
	tabs   []*editorTabState
	active int
}

// activeEditorTab 返回当前激活的 tab，没有就 nil。
func (a *app) activeEditorTab() *editorTabState {
	s := a.editorPane
	if s.active < 0 || s.active >= len(s.tabs) {
		return nil
	}
	return s.tabs[s.active]
}

// editorTabIndex 按路径找已打开的 tab，没有返回 -1。
func (a *app) editorTabIndex(path string) int {
	for i, t := range a.editorPane.tabs {
		if t.path == path {
			return i
		}
	}
	return -1
}

// editorTabIndexPtr 按对象找 tab 现在的下标（确认框关 tab 时用：确认期间
// 列表可能已经变过，不能拿旧下标去删）。
func (a *app) editorTabIndexPtr(target *editorTabState) int {
	for i, t := range a.editorPane.tabs {
		if t == target {
			return i
		}
	}
	return -1
}

// openEditorTab 打开或激活一个文件。已在某个 tab 里打开的直接切过去，不
// 重复读盘；新打开追加到末尾并激活。
func (a *app) openEditorTab(f gitrepo.File) {
	if a.viewMode != viewModeEditor {
		return
	}
	path := pathOf(f)
	if i := a.editorTabIndex(path); i >= 0 {
		a.activateEditorTab(i)
		return
	}
	t := &editorTabState{path: path, section: f.Section, loading: true}
	a.editorPane.tabs = append(a.editorPane.tabs, t)
	a.activateEditorTab(len(a.editorPane.tabs) - 1)
	a.loadEditorTab(t, f)
}

// activateEditorTab 切到第 i 个 tab。路径能在左栏找到时同步选中，避免
// 「tab 是 A、左栏高亮 B」。
func (a *app) activateEditorTab(i int) {
	if i < 0 || i >= len(a.editorPane.tabs) {
		return
	}
	a.editorPane.active = i
	a.editorReveal = true
	if a.viewMode != viewModeEditor {
		return
	}
	path := a.editorPane.tabs[i].path
	for j := range a.wsFiles {
		if pathOf(a.wsFiles[j]) == path {
			a.selectWorkspaceKey(path)
			break
		}
	}
}

// closeEditorTab 关闭第 i 个 tab；有未保存改动时先问一句。
func (a *app) closeEditorTab(i int) {
	if i < 0 || i >= len(a.editorPane.tabs) {
		return
	}
	target := a.editorPane.tabs[i]
	if target.buffer != nil && target.buffer.Modified() {
		a.askConfirm(confirmState{
			title: T("关闭这个文件标签页？", "Close this file tab?"),
			body: T("这个文件有还没保存的改动，关掉就丢了。先保存（⌘S），或者点「取消」。",
				"This file has unsaved changes; closing it discards them. Save (⌘S) first, or cancel."),
			yes: T("仍然关闭", "Close anyway"),
			onYes: func() {
				if idx := a.editorTabIndexPtr(target); idx >= 0 {
					a.closeEditorTabNow(idx)
				}
			},
		})
		return
	}
	a.closeEditorTabNow(i)
}

// closeEditorTabNow 立刻关掉第 i 个 tab。关闭激活 tab 后激活右邻（没有则
// 左邻）；全部关掉后回到空状态。
func (a *app) closeEditorTabNow(i int) {
	if i < 0 || i >= len(a.editorPane.tabs) {
		return
	}
	t := a.editorPane.tabs[i]
	t.dead = true
	t.seq++ // 迟到的读盘结果对不上号

	a.editorPane.tabs = append(a.editorPane.tabs[:i], a.editorPane.tabs[i+1:]...)
	switch {
	case len(a.editorPane.tabs) == 0:
		a.editorPane.active = -1
	case i < a.editorPane.active:
		a.editorPane.active--
	case i == a.editorPane.active:
		if i >= len(a.editorPane.tabs) {
			a.editorPane.active = len(a.editorPane.tabs) - 1
		}
	}
	a.editorReveal = true
}

// discardEditorTabs 把一个项目里的所有 tab 标死并清空。换仓库、关项目时用：
// 缓冲区属于旧仓库，留着下一次保存就会写进新仓库。
func (a *app) discardEditorTabs(s *editorPaneState) {
	for _, t := range s.tabs {
		t.dead = true
		t.seq++
	}
	s.tabs = nil
	s.active = -1
}

// editorTabsDirty 报告这一列 tab 里有没有未保存的改动。
func editorTabsDirty(s *editorPaneState) bool {
	for _, t := range s.tabs {
		if t.buffer != nil && t.buffer.Modified() {
			return true
		}
	}
	return false
}

// editorTarget 取编辑器该显示哪个文件：左栏当前选中的那个，没选就退到第一个。
//
// 编辑器视图的左栏是整个工作区（wsFiles），别的视图里那份是改动（files）——
// 跟着视图走，两边各自记着自己的选中下标。树筛选把选中的文件筛出列表时
// （下标落空），搜索跳转钉住的文件（wsPinPath）顶上。
func (a *app) editorTarget() *gitrepo.File {
	list, sel := a.files, a.fileSel
	if a.viewMode == viewModeEditor {
		list, sel = a.wsFiles, a.wsSel
		if a.wsPinPath != "" && (sel < 0 || sel >= len(list)) {
			if f := wsFileByPath(a.wsAll, a.wsPinPath); f != nil {
				return f
			}
		}
	}
	if len(list) == 0 {
		return nil
	}
	i := sel
	if i < 0 || i >= len(list) {
		i = 0
	}
	return &list[i]
}

// syncEditorPane 是「进入编辑器视图时该显示什么」：一个 tab 都没有才把左栏
// 当前选中的文件打开；已经有 tab 就保持原样。
//
// 仓库刷新（applyModel）不调它 —— 刷新是更新文件列表，不是换一个文件来看。
// 方向键导航也不调它：移动选中和打开文件是两件事，一路按 ↓ 不该长出一串
// tab；要打开请点一下或者从搜索里跳。
func (a *app) syncEditorPane() {
	if a.viewMode != viewModeEditor {
		return
	}
	if len(a.editorPane.tabs) > 0 {
		return
	}
	if f := a.editorTarget(); f != nil {
		a.openEditorTab(*f)
	}
}

// ensureEditorTabOnEnter 在第一次进入编辑器视图时打开左栏选中的文件。
func (a *app) ensureEditorTabOnEnter() {
	a.syncEditorPane()
}

// openEditorSelection 是「用户点了左栏某个文件」：打开或激活它。已有 tab
// 时也照常动作 —— 点击本来就是明确的打开意图。
func (a *app) openEditorSelection() {
	if a.viewMode != viewModeEditor {
		return
	}
	if f := a.editorTarget(); f != nil {
		a.openEditorTab(*f)
	}
}

// loadEditorTab 读工作区里那一份进来，并配上和「编辑代码」同一套的 git 基线
// （基线决定行号槽的增删标记，也决定「相对哪一版」）。结果写回 t 本身，
// 不要求 t 还在当前项目的列表里。
func (a *app) loadEditorTab(t *editorTabState, f gitrepo.File) {
	t.seq++
	seq := t.seq
	t.dead = false
	t.section = f.Section
	t.loading = true
	t.err = ""
	path := t.path
	root := a.repoPath
	image := isImagePreview(f)
	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var data, base []byte
		var side imageSide
		if err == nil {
			data, err = repo.Contents(path)
		}
		if err == nil && image {
			// 图片不进编辑器：解成位图摆出来。超过上限的连解都不解，
			// 直接把说明当错误显示（见 decodeImageSide）。
			side = decodeImageSide(data)
		}
		if err == nil && !image && looksBinary(data) {
			err = fmt.Errorf("%s 看起来不是文本文件", path)
		}
		if err == nil && !image {
			base = codeEditorBaseline(ctx, repo, path, t.section)
		}
		a.dispatch(func() {
			if t.seq != seq || t.dead {
				return // 读盘期间被关掉或被新的一次读盘顶掉了
			}
			t.loading = false
			if err != nil {
				t.err = err.Error()
				t.code, t.buffer, t.base, t.img = nil, nil, nil, nil
				t.pendingHit = wsSearchHit{} // 没有缓冲可兑现，别让请求残留到下一个文件头上
				return
			}
			if image {
				if side.err != "" {
					t.err = side.err
					t.code, t.buffer, t.base, t.img = nil, nil, nil, nil
					t.pendingHit = wsSearchHit{}
					return
				}
				t.img, t.imgMeta = side.bitmap, side.meta
				t.code, t.buffer, t.base = nil, nil, nil
				t.err = ""
				t.pendingHit = wsSearchHit{}
				return
			}
			lang := editor.LanguageFor(path)
			buf := editor.NewBuffer(string(data))
			buf.SetLanguage(lang)
			st := &editor.State{Buffer: buf, Label: path}
			if len(base) > 0 {
				b := editor.NewBuffer(string(base))
				b.SetLanguage(lang)
				st.DiffBase = b
				t.base = b
			}
			st.Bindings = []editor.Binding{
				{Mods: ui.Cmd, Key: ui.KeyS, Command: editor.NoCommand},
			}
			t.buffer = buf
			t.code = st
			t.img, t.imgMeta = nil, ""
			t.err = ""
			if t.pendingHit.Line > 0 {
				revealWsHit(st, t.pendingHit)
				t.pendingHit = wsSearchHit{}
			}
		})
	})
}

// saveEditorPane 把当前激活 tab 的缓冲区写回工作区：只写盘、不动 index，
// 和「编辑代码」一致（暂不暂存由用户在文件栏里自己决定）。
func (a *app) saveEditorPane() {
	t := a.activeEditorTab()
	if t == nil || t.buffer == nil || t.err != "" {
		return
	}
	path := t.path
	text := t.buffer.String()
	buf := t.buffer
	dir := a.repoPath
	a.startOp(gitrepo.OpWrite, Tf("正在保存 %s", "Saving %s", path),
		func(_ context.Context, repo *gitrepo.Repo) (string, error) {
			if err := repo.WriteContents(path, []byte(text)); err != nil {
				return "", err
			}
			return Tf("已保存 %s", "Saved %s", path), nil
		}, func(ok bool) {
			if ok {
				buf.MarkSaved()
			}
			if a.repoPath == dir {
				a.reloadAfterOp()
				return
			}
			// 保存期间切到了别的项目：别刷新眼前这个，把发起保存的那个项目
			// 置回 loadIdle，下次切回去时整体重读。
			for _, tab := range a.tabs {
				if sameRepo(tab.path, dir) {
					tab.state = loadIdle
				}
			}
		})
}

// editorArea 是「编辑器」视图下的整块主区：左边仍是那栏文件列表，右边换成一个
// 可编辑的整文件编辑器 —— 「看哪几个文件变了」和「改文件」在同一屏里。
func (a *app) editorArea(c *ui.Context, pal palette) {
	t := c.Theme()

	if a.viewMode != viewModeEditor && !a.hasDiff() && (a.state == loadIdle || a.state == loadFailed) {
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
				a.detailEmpty(c, t, pal)
			})
		})
		return
	}

	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
		// 编辑器视图左边列整个工作区：工作区没改动也照列 —— 「改了什么」是
		// 改动视图要回答的问题，这里回答的是「仓库里有什么」。
		if a.viewMode == viewModeEditor {
			a.workspaceFilePane(c, pal, t)
		} else {
			a.fileListPane(c, pal, t)
		}
		a.filePaneSplitter(c, pal)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.editorPaneView(c, pal)
		})
	})
}

// editorPaneView 画右边那一栏：文件 tab 栏加当前 tab 的编辑器。
func (a *app) editorPaneView(c *ui.Context, pal palette) {
	t := c.Theme()
	a.editorTabBar(c, pal)

	s := a.activeEditorTab()
	switch {
	case s == nil:
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(8).Padding(24).Children(func() {
				ui.Text(c, T("从左侧选择一个文件开始编辑", "Select a file on the left to start editing")).
					FontSize(uiFontSize(textBodySM)).TextColor(t.TextMuted)
			})
		})
		return
	case s.loading || (s.code == nil && s.img == nil && s.err == ""):
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Spinner(c).Size(18, 18)
				ui.Text(c, T("正在读取文件…", "Loading file…")).FontSize(uiFontSize(textBodySM)).TextColor(pal.inkMuted)
			})
		})
		return
	case s.err != "":
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
				headline(c, pal, T("打不开这个文件", "Unable to Open File"))
				ui.Text(c, s.err).Font("monospace").FontSize(uiFontSize(textCaption)).
					SingleLine().TextColor(pal.inkMuted)
			})
		})
		return
	case s.img != nil:
		imagePanel(c, pal, "", imageSide{bitmap: s.img, meta: s.imgMeta})
		return
	}

	th := a.codeEditorTheme(c, pal)
	s.code.Theme = th
	s.code.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
	s.code.SoftWrap = a.wrapCode
	editor.View(c, s.code).Fill().AutoFocus()
}

// editorTabBar 是编辑器上方那条文件 tab 栏。
//
// 左边是横向滚动的 tab：文件名只写 basename，完整相对路径在 tooltip 里；
// 右边再报一次当前文件的相对路径，这样两个同名文件分别在不同目录时也能
// 一眼认出当前是哪一个。tab 自己只报「有没有未保存的改动」（名字前的蓝点），
// 不再摆已暂存/未暂存 chip —— 那是文件列表那边的事。
func (a *app) editorTabBar(c *ui.Context, pal palette) {
	ui.Row(c).Shrink(0).FillWidth().Height(34).Padding(0, 12).Gap(8).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		if len(a.editorPane.tabs) == 0 {
			ui.Text(c, T("未选择文件", "No file selected")).Font("monospace").
				FontSize(uiFontSize(textBodySM)).SingleLine().TextColor(pal.inkMuted)
			ui.Spacer(c)
			return
		}

		var activeEl ui.Element
		var activeOK bool
		closeAt := -1
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Height(28).Gap(2).Children(func() {
			for i, tab := range a.editorPane.tabs {
				el, closeClicked := a.editorTabChip(c, pal, i, tab)
				if i == a.editorPane.active {
					activeEl, activeOK = el, true
				}
				if closeClicked {
					closeAt = i
				}
			}
		})
		if closeAt >= 0 {
			a.closeEditorTab(closeAt)
		}
		if activeOK && a.editorReveal {
			activeEl.ScrollIntoView()
			a.editorReveal = false
		}

		if a.activeEditorTab() == nil {
			return
		}
		// 换行开关常驻 tab 栏右端：它管的是当前这份代码怎么排，长行
		// 用户随时要有处切。
		if toolIconToggle(c, pal, T("自动换行", "Soft Wrap"),
			T("长行折到下一行显示", "Wrap long lines at the view's width"),
			a.wrapCode, drawWrapIcon) {
			a.setCodeWrap(!a.wrapCode)
		}
	})
}

// editorTabChip 画一枚文件 tab，返回元素和这一帧它的关闭 × 有没有被点中。
// 关闭动作由调用方在这一圈 tab 都画完之后再做（与终端芯片同一理由：画到
// 一半从切片里删一个，后面的 tab 会整体前移一格）。
func (a *app) editorTabChip(c *ui.Context, pal palette, idx int, tab *editorTabState) (ui.Element, bool) {
	t := c.Theme()
	active := idx == a.editorPane.active
	name := filepath.Base(tab.path)

	chip := ui.ButtonBase(c).Height(28).Shrink(0).MinWidth(110).MaxWidth(220).
		Radius(radiusSM).Padding(0, 6).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(name).Tooltip(tab.path)

	bg, fg, bd := ui.Transparent, pal.inkMuted, ui.Transparent
	switch {
	case active:
		bg, fg, bd = pal.rowSelected, pal.ink, pal.hairlineStrong
	case chip.Hovered():
		bg = pal.rowHover
	}
	chip.Background(bg).Border(1, bd)
	fadeColors(chip)

	var closeClicked bool
	chip.Children(func() {
		if tab.buffer != nil && tab.buffer.Modified() {
			// 未保存圆点在文件名前面、用强调蓝：它是「这个 tab 里还有东西
			// 没落盘」，和状态色（警告/危险）不是一回事。带 Label 让读屏
			// 和测试都知道它是什么。
			ui.Box(c).Size(6, 6).Shrink(0).Radius(radiusPill).
				Background(t.Accent).Label(T("未保存的改动", "Unsaved changes"))
		}
		ui.Text(c, name).FontSize(uiFontSize(textCaption)).SingleLine().
			Grow(1).MinWidth(0).TextColor(fg)
		closeBtn := ui.ButtonBase(c).Size(16, 16).Shrink(0).Radius(radiusXS).
			Cursor(ui.CursorPointer).
			Label(Tf("关闭文件标签页 %s", "Close File Tab %s", name))
		if closeBtn.Hovered() {
			closeBtn.Background(pal.rowSelected)
		}
		fadeColors(closeBtn)
		closeBtn.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Fill().Children(func() {
				drawIcon(c, iconX, 10, pal.inkSubtle)
			})
		})
		if closeBtn.Clicked() {
			closeClicked = true
		}
	})
	if chip.Clicked() && !closeClicked {
		a.activateEditorTab(idx)
	}
	return chip, closeClicked
}

// 「编辑器」视图原先在顶栏右侧摆「保存 / 还原」两颗按钮，现在不摆了：
// 保存只剩 ⌘S（见 keys.go），还原交给「关掉这个 tab」那条会先问一句的路。
// 顶栏因此只留视图切换与仓库级动作。
