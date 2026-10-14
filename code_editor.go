package main

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// codeEditorState 是内置代码编辑器（整文件编辑）的短生命周期状态。
//
// 缓冲区放在这里而不是每帧新建：MyGo 把光标、选区、折叠、撤销历史和滚动
// 位置都存在 buffer/state 里，跨帧复用才留得住 —— 和冲突编辑器一个道理。
type codeEditorState struct {
	on      bool
	loading bool
	path    string
	err     string

	buffer *editor.Buffer
	base   *editor.Buffer // 行号槽里画改动标记的参照版本，没有基线时为 nil
	code   *editor.State
}

// openCodeEditor 打开工作区里的一个文件，用 MyGo 的 editor.View 整文件编辑。
//
// 和冲突编辑器分开：那边编的是「三方版本 + 冲突标记 + 保存即 git add」，这里
// 编的是「磁盘上这一份」，保存只写工作区、不动 index。两件事的收尾动作不同，
// 混在一个 state 里迟早会有一边做错。
func (a *app) openCodeEditor(f gitrepo.File) {
	if a.repoPath == "" || a.op != gitrepo.OpNone {
		return
	}
	path := pathOf(f)
	section := f.Section
	a.codeEdit = codeEditorState{on: true, loading: true, path: path}
	root := a.repoPath
	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var data, base []byte
		if err == nil {
			data, err = repo.Contents(path)
		}
		if err == nil && !utf8.Valid(data) {
			err = fmt.Errorf("%s 不是可编辑的文本文件", path)
		}
		if err == nil {
			base = codeEditorBaseline(ctx, repo, path, section)
		}
		a.dispatch(func() {
			if !a.codeEdit.on || a.codeEdit.path != path {
				return
			}
			if err != nil {
				a.codeEdit.loading = false
				a.codeEdit.err = err.Error()
				return
			}
			lang := editor.LanguageFor(path)
			buf := editor.NewBuffer(string(data))
			buf.SetLanguage(lang)
			st := &editor.State{Buffer: buf, Label: "代码编辑器"}
			if len(base) > 0 {
				b := editor.NewBuffer(string(base))
				b.SetLanguage(lang)
				a.codeEdit.base = b
				st.DiffBase = b
			}
			// Cmd+S 交给 Gloss 自己处理：编辑器把它认走就没机会保存了。
			st.Bindings = []editor.Binding{
				{Mods: ui.Cmd, Key: ui.KeyS, Command: editor.NoCommand},
			}
			a.codeEdit.buffer = buf
			a.codeEdit.code = st
			a.codeEdit.loading = false
		})
	})
}

// codeEditorBaseline 取行号槽里画改动标记的参照版本。
//
// 取法和内置 Diff 预览一致：未暂存段对 index（:0），已暂存段对 HEAD。两处
// 用同一套「跟谁比」的规矩，同一个文件在编辑器里和在 Diff 里的增删才对得上。
// 都取不到就没有基线（未跟踪、新建的文件本来没有可比的版本）。
func codeEditorBaseline(ctx context.Context, repo *gitrepo.Repo, path string, section gitrepo.Section) []byte {
	revs := []string{":0", "HEAD"}
	if section == gitrepo.SectionStaged {
		revs = []string{"HEAD"}
	}
	for _, rev := range revs {
		if data, err := repo.ContentsAt(ctx, rev, path); err == nil {
			return data
		}
	}
	return nil
}

func (a *app) closeCodeEditor() { a.codeEdit = codeEditorState{} }

// closeEmbeddedEditors 收掉内置编辑器（整文件代码对话框 / 编辑器视图 /
// 改动区的 DiffView）。
//
// 换仓库时必须调：它们的缓冲区属于上一个仓库，而「保存」写的是 a.repoPath
// 底下的相对路径 —— 留着它，下一次保存就会把 A 仓的内容写进 B 仓。
func (a *app) closeEmbeddedEditors() {
	a.codeEdit = codeEditorState{}
	a.editorPane = editorPaneState{}
	a.review = reviewState{}
	// 草稿里的缓冲区属于上一个仓库，而「保存」写的是 a.repoPath 底下的
	// 相对路径 —— 留着它，下一次保存就会把 A 仓的内容写进 B 仓。
	a.reviewDrafts = nil
}

// codeEditorDialog 画整文件编辑器。和冲突编辑器同一副骨架（标题条 / 正文 /
// 底栏），只是正文永远是 editor.View，而不是分栏 diff。
func (a *app) codeEditorDialog(c *ui.Context, pal palette) {
	if !a.codeEdit.on {
		return
	}
	open := true
	w, h := c.Size()
	panelW := clamp(w-32, 640, 1180)
	panelH := clamp(h-32, 420, 780)
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.28))
		panel.Width(panelW).Height(panelH).Padding(0).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24)).Children(func() {
			a.codeEditorBody(c, pal)
		})
	})
	if !open {
		a.closeCodeEditor()
	}
}

func (a *app) codeEditorBody(c *ui.Context, pal palette) {
	t := c.Theme()
	s := &a.codeEdit
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(48).Shrink(0).Padding(0, 14).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "编辑代码").FontSize(textHeadline).FontWeight(weightSemibold).
					TextColor(pal.ink)
				ui.Text(c, s.path).Font("monospace").FontSize(textCaption).
					SingleLine().TextColor(pal.inkMuted)
			})
			if s.buffer != nil && s.buffer.Modified() {
				ui.Text(c, "已修改").FontSize(textCaption).TextColor(t.Warning)
			}
			// Cmd+S 只有落在对话框自己身上才有效：handleKeys 在 view 一开头
			// 跑，而那时 c 还在对话框外面，全局快捷键在有弹窗时一律让位。
			if s.code != nil && c.Shortcut(ui.Cmd, ui.KeyS) {
				a.saveCodeEditor()
			}
			if xButton(c, pal, "关闭") {
				a.closeCodeEditor()
			}
		})

		if s.loading {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在读取文件…").TextColor(t.TextMuted)
			})
			return
		}
		if s.err != "" || s.code == nil {
			ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
				headline(c, pal, "无法打开内置编辑器")
				ui.Text(c, s.err).Font("monospace").FontSize(textBodySM).
					TextColor(t.TextMuted)
				if secondaryAction(c, pal, "关闭", "", false).Clicked() {
					a.closeCodeEditor()
				}
			})
			return
		}

		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			th := editor.DefaultTheme(t)
			th.Background = pal.codeBg
			th.Text = pal.code
			s.code.Theme = th
			s.code.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
			editor.View(c, s.code).Fill().AutoFocus()
		})

		ui.Row(c).Height(48).Shrink(0).Padding(0, 12).Gap(8).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
			hint := "保存只写工作区，不会替你暂存 —— 改动会出现在「未暂存」里"
			if s.base != nil {
				hint += "；行号槽的标记是相对暂存区/HEAD 的改动"
			}
			ui.Text(c, hint).FontSize(textCaption).TextColor(pal.inkMuted).
				Grow(1).MinWidth(0)
			busy := a.op != gitrepo.OpNone
			if secondaryAction(c, pal, "取消", "放弃本次编辑", busy).Clicked() {
				a.closeCodeEditor()
			}
			if primaryAction(c, pal, "保存", "写入工作区（⌘S）", busy).Clicked() {
				a.saveCodeEditor()
			}
		})
	})
}

// saveCodeEditor 把缓冲区写回工作区。
//
// 只走 WriteContents：不 git add、不碰 index。用户按下「保存」想要的是磁盘上
// 那一份变了，暂存与否是下一步自己的决定（暂存区的勾选框就在文件行左边）。
func (a *app) saveCodeEditor() {
	s := &a.codeEdit
	if !s.on || s.buffer == nil || a.op != gitrepo.OpNone {
		return
	}
	path := s.path
	text := s.buffer.String()
	buf := s.buffer
	a.startOp(gitrepo.OpWrite, "正在保存 "+path, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		if err := repo.WriteContents(path, []byte(text)); err != nil {
			return "", err
		}
		return "已保存 " + path, nil
	}, func(ok bool) {
		if ok && a.codeEdit.on && a.codeEdit.buffer == buf {
			// 行号槽的基线也跟着走：刚写进去的这份就是新的工作区内容，
			// 而基线上一步已经在 reloadAfterOp 里重读过了。
			a.closeCodeEditor()
		}
		a.reloadAfterOp()
	})
}
