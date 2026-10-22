package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// editorPaneState 是「编辑器」视图里那个整文件编辑器的状态。
//
// 它和 codeEdit（文件右键「编辑代码」弹的对话框）是两份独立状态。那边是一次性
// 的：打开一个文件、改完保存、关掉。这边跟着左侧文件栏的选中共进退 —— 换文件就
// 换缓冲区，切仓库整份丢掉，只要还停在编辑器视图就一直活着。分成两份是因为收尾
// 动作不同（对话框存完就关，这里存完要继续留着改），混在一个 state 里迟早有一边
// 做错。
type editorPaneState struct {
	loading bool
	path    string // 已加载（或正在加载）的文件路径
	err     string
	buffer  *editor.Buffer
	base    *editor.Buffer
	code    *editor.State
	// img 是图片文件的预览（见 imagereview.go）。图片进不了编辑器：
	// 没有文本缓冲区，这里摆的是解好的位图，保存/还原都不适用。
	img     *ui.Bitmap
	imgMeta string
}

// editorTarget 取编辑器该显示哪个文件：左栏当前选中的那个，没选就退到第一个。
//
// 编辑器视图的左栏是整个工作区（wsFiles），别的视图里那份是改动（files）——
// 跟着视图走，两边各自记着自己的选中下标。
func (a *app) editorTarget() *gitrepo.File {
	list, sel := a.files, a.fileSel
	if a.viewMode == viewModeEditor {
		list, sel = a.wsFiles, a.wsSel
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

// syncEditorPane 让编辑器对齐到左栏当前选中的文件。
//
// 只在「编辑器」视图下动作 —— 别的视图里没必要读盘。切文件、切视图、仓库重读
// 之后都调它一次；目标路径没变就直接返回，所以重复调用是安全的。
func (a *app) syncEditorPane() {
	if a.viewMode != viewModeEditor || a.editorPane.loading {
		return
	}
	f := a.editorTarget()
	if f == nil {
		a.editorPane = editorPaneState{}
		return
	}
	if a.editorPane.path == pathOf(*f) && a.editorPane.code != nil {
		return
	}
	a.loadEditorPane(*f)
}

// loadEditorPane 读工作区里那一份进来，并配上和「编辑代码」同一套的 git 基线
// （基线决定行号槽的增删标记，也决定「相对哪一版」）。
func (a *app) loadEditorPane(f gitrepo.File) {
	path, section := pathOf(f), f.Section
	root := a.repoPath
	image := isImagePreview(f)
	a.editorPane.loading = true
	a.editorPane.path = path
	a.editorPane.err = ""
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
			base = codeEditorBaseline(ctx, repo, path, section)
		}
		a.dispatch(func() {
			if a.editorPane.path != path {
				return // 读盘期间又切走了，这份结果作废
			}
			s := &a.editorPane
			s.loading = false
			if err != nil {
				s.err = err.Error()
				s.code, s.buffer, s.base, s.img = nil, nil, nil, nil
				return
			}
			if image {
				if side.err != "" {
					s.err = side.err
					s.code, s.buffer, s.base, s.img = nil, nil, nil, nil
					return
				}
				s.img, s.imgMeta = side.bitmap, side.meta
				s.code, s.buffer, s.base = nil, nil, nil
				s.err = ""
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
				s.base = b
			}
			st.Bindings = []editor.Binding{
				{Mods: ui.Cmd, Key: ui.KeyS, Command: editor.NoCommand},
			}
			s.buffer = buf
			s.code = st
			s.img, s.imgMeta = nil, ""
			s.err = ""
		})
	})
}

// saveEditorPane 把缓冲区写回工作区：只写盘、不动 index，和「编辑代码」一致
// （暂不暂存由用户在文件栏里自己决定）。
func (a *app) saveEditorPane() {
	s := &a.editorPane
	if s.buffer == nil || s.err != "" {
		return
	}
	path := s.path
	text := s.buffer.String()
	buf := s.buffer
	a.startOp(gitrepo.OpWrite, Tf("正在保存 %s", "Saving %s", path),
		func(_ context.Context, repo *gitrepo.Repo) (string, error) {
			if err := repo.WriteContents(path, []byte(text)); err != nil {
				return "", err
			}
			return Tf("已保存 %s", "Saved %s", path), nil
		}, func(ok bool) {
			if ok && a.editorPane.buffer == buf {
				buf.MarkSaved()
			}
			a.reloadAfterOp()
		})
}

// revertEditorPane 丢掉未保存的改动，重新读一份。
//
// 走 loadEditorPane 而不是在内存里回滚缓冲区：磁盘上的内容可能已经被外部（别的
// 编辑器、git 命令）改过，重读顺带把「行号槽标记」也重新对齐。
func (a *app) revertEditorPane() {
	f := a.editorTarget()
	if f == nil {
		return
	}
	a.editorPane.path = "" // 让 loadEditorPane 认得这是重新读
	a.loadEditorPane(*f)
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

// editorPaneView 画右边那一栏：栏头（路径 + 已暂存/未暂存 + 已改圆点）加编辑器。
func (a *app) editorPaneView(c *ui.Context, pal palette) {
	s := &a.editorPane

	a.editorPaneHeader(c, pal)

	switch {
	case s.loading || (s.code == nil && s.img == nil && s.err == ""):
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Spinner(c).Size(18, 18)
				ui.Text(c, T("正在读取文件…", "Loading file…")).FontSize(textBodySM).TextColor(pal.inkMuted)
			})
		})
		return
	case s.err != "":
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
				headline(c, pal, T("打不开这个文件", "Unable to Open File"))
				ui.Text(c, s.err).Font("monospace").FontSize(textCaption).
					SingleLine().TextColor(pal.inkMuted)
			})
		})
		return
	case s.img != nil:
		imagePanel(c, pal, "", imageSide{bitmap: s.img, meta: s.imgMeta})
		return
	}

	th := editor.DefaultTheme(c.Theme())
	th.Background = pal.codeBg
	th.Text = pal.code
	s.code.Theme = th
	s.code.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
	editor.View(c, s.code).Fill().AutoFocus()
}

// editorPaneHeader 是编辑器上方那条细栏：左边路径，右边「已暂存/未暂存」和
// 未保存圆点。
func (a *app) editorPaneHeader(c *ui.Context, pal palette) {
	s := &a.editorPane
	t := c.Theme()
	dirty := s.buffer != nil && s.buffer.Modified()

	ui.Row(c).Shrink(0).FillWidth().Height(34).Padding(0, 12).Gap(8).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		label := s.path
		if label == "" {
			label = T("未选择文件", "No file selected")
		}
		ui.Text(c, label).Font("monospace").FontSize(textBodySM).
			SingleLine().TextColor(pal.ink)
		ui.Spacer(c)
		if f := a.editorTarget(); f != nil && (s.code != nil || s.img != nil) {
			if s.img != nil {
				specChip(c, pal, T("图片", "Image"), ui.Transparent, 20, textCaption)
			} else if f.Section == gitrepo.SectionStaged {
				specChip(c, pal, T("已暂存", "Staged"), ui.Transparent, 20, textCaption)
			} else {
				specChip(c, pal, T("未暂存", "Unstaged"), ui.Transparent, 20, textCaption)
			}
		}
		if dirty {
			ui.Box(c).Size(6, 6).Shrink(0).Radius(radiusPill).Background(t.Warning)
		}
	})
}

// editorToolbar 是「编辑器」视图下工具栏右侧的两个动作。
func (a *app) editorToolbar(c *ui.Context, pal palette) {
	s := &a.editorPane
	busy := a.op != gitrepo.OpNone || s.loading
	dirty := s.buffer != nil && s.buffer.Modified()
	ready := s.code != nil && s.err == ""

	save := primaryAction(c, pal, T("保存", "Save"),
		T("写回工作区，不自动暂存", "Write to the working tree, without staging"),
		!ready || !dirty || busy)
	if save.Clicked() {
		a.saveEditorPane()
	}
	revert := secondaryAction(c, pal, T("还原", "Revert"),
		T("丢弃未保存的改动，重新读盘", "Discard unsaved changes and reload from disk"),
		!ready || !dirty || busy)
	if revert.Clicked() {
		a.revertEditorPane()
	}
}
