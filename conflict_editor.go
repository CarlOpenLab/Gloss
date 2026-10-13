package main

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// conflictEditorState is the short-lived state of the embedded conflict editor.
// The editor buffers live here, not in the frame builder: MyGo keeps selections,
// undo history and scroll position in the buffers/state across frames.
type conflictEditorState struct {
	on      bool
	loading bool
	path    string
	err     string

	base, ours, theirs *editor.Buffer
	diff               *editor.DiffState
	code               *editor.State
	side               conflictEditorSide
	plain              bool
}

type conflictEditorSide uint8

const (
	conflictSideOurs conflictEditorSide = iota
	conflictSideTheirs
	conflictSideBase
)

// openConflictEditor opens a real diff even when git's combined conflict patch
// cannot be parsed by Gloss's unified-diff parser. The working-tree text is the
// editable New side; the selected index stage is the Old side.
func (a *app) openConflictEditor(f gitrepo.File) {
	if a.op != gitrepo.OpNone || a.repoPath == "" {
		return
	}
	path := pathOf(f)
	a.conflictEdit = conflictEditorState{on: true, loading: true, path: path, side: conflictSideOurs}
	root := a.repoPath
	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var base, ours, theirs, worktree []byte
		if err == nil {
			base, ours, theirs, worktree, err = repo.ConflictContents(ctx, path)
		}
		if err == nil && !utf8.Valid(worktree) {
			err = fmt.Errorf("%s 不是可编辑的文本文件", path)
		}
		a.dispatch(func() {
			if !a.conflictEdit.on || a.conflictEdit.path != path {
				return
			}
			if err != nil {
				a.conflictEdit.loading = false
				a.conflictEdit.err = err.Error()
				return
			}
			lang := editor.LanguageFor(path)
			makeBuffer := func(data []byte) *editor.Buffer {
				b := editor.NewBuffer(string(data))
				b.SetLanguage(lang)
				return b
			}
			a.conflictEdit.base = makeBuffer(base)
			a.conflictEdit.ours = makeBuffer(ours)
			a.conflictEdit.theirs = makeBuffer(theirs)
			work := makeBuffer(worktree)
			a.conflictEdit.diff = &editor.DiffState{
				Old:          a.conflictEdit.ours,
				New:          work,
				Layout:       editor.DiffSplit,
				Editable:     true,
				Inline:       editor.DiffInlineWordAlt,
				Indicators:   editor.DiffBars,
				ContextLines: 3,
			}
			a.conflictEdit.code = &editor.State{
				Buffer: work,
				Label:  "冲突解决编辑器",
				// 整文件模式没有左边那一栏，但基线还是同一份：行号槽里照样能
				// 看出相对选中那一侧改了什么，切基线时两边一起跟着换。
				DiffBase: a.conflictEdit.ours,
			}
			a.conflictEdit.loading = false
		})
	})
}

func (a *app) closeConflictEditor() {
	a.conflictEdit = conflictEditorState{}
}

// conflictEditor renders the embedded resolver modal. DiffView supplies the
// editor behavior (selection, undo, find, folding and keyboard commands) while
// Gloss keeps the Git write/stage operation behind startOp.
func (a *app) conflictEditor(c *ui.Context, pal palette) {
	if !a.conflictEdit.on {
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
			a.conflictEditorBody(c, pal)
		})
	})
	if !open {
		a.closeConflictEditor()
	}
}

func (a *app) conflictEditorBody(c *ui.Context, pal palette) {
	t := c.Theme()
	s := &a.conflictEdit
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(48).Shrink(0).Padding(0, 14).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "解决冲突").FontSize(textHeadline).FontWeight(weightSemibold).
					TextColor(pal.ink)
				ui.Text(c, s.path).Font("monospace").FontSize(textCaption).
					SingleLine().TextColor(pal.inkMuted)
			})
			if s.diff != nil {
				added, deleted := s.diff.Stats()
				ui.Textf(c, "+%d −%d", added, deleted).Font("monospace").
					FontSize(textCaption).TextColor(pal.inkMuted)
			}
			if xButton(c, pal, "关闭") {
				a.closeConflictEditor()
			}
		})

		if s.loading {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在读取冲突双方…").TextColor(t.TextMuted)
			})
			return
		}
		if s.err != "" || s.diff == nil {
			ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
				headline(c, pal, "无法打开内置冲突编辑器")
				ui.Text(c, s.err).Font("monospace").FontSize(textBodySM).
					TextColor(t.TextMuted)
				if secondaryAction(c, pal, "关闭", "", false).Clicked() {
					a.closeConflictEditor()
				}
			})
			return
		}

		ui.Row(c).Height(38).Shrink(0).Padding(0, 12).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
			ui.Text(c, "对比基线").FontSize(textCaption).TextColor(pal.inkMuted)
			a.conflictSideButton(c, pal, "当前分支", conflictSideOurs, s.side == conflictSideOurs)
			a.conflictSideButton(c, pal, "对方分支", conflictSideTheirs, s.side == conflictSideTheirs)
			a.conflictSideButton(c, pal, "共同祖先", conflictSideBase, s.side == conflictSideBase)
			if s.plain {
				ui.Text(c, "内置编辑器").FontSize(textCaption).TextColor(pal.inkMuted)
			} else {
				ui.Text(c, "Diff 编辑器").FontSize(textCaption).TextColor(pal.inkMuted)
			}
			ui.Spacer(c)
			mode := "切换到内置编辑器"
			if s.plain {
				mode = "切换到 Diff 视图"
			}
			if secondaryAction(c, pal, mode, "切换编辑模式", false).Clicked() {
				s.plain = !s.plain
			}
			if s.diff.New.Modified() {
				ui.Text(c, "已编辑").FontSize(textCaption).TextColor(t.Warning)
			} else {
				ui.Text(c, "未编辑").FontSize(textCaption).TextColor(pal.inkSubtle)
			}
		})

		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			th := editor.DefaultTheme(t)
			th.Background = pal.codeBg
			th.Text = pal.code
			if s.plain {
				s.code.Theme = th
				s.code.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
				editor.View(c, s.code).Fill()
			} else {
				s.diff.Theme = th
				s.diff.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
				editor.DiffView(c, s.diff).Fill()
			}
		})

		ui.Row(c).Height(48).Shrink(0).Padding(0, 12).Gap(8).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
			ui.Text(c, "保存后会写入工作区并执行 git add，冲突才算解决").
				FontSize(textCaption).TextColor(pal.inkMuted).Grow(1).MinWidth(0)
			busy := a.op != gitrepo.OpNone
			if secondaryAction(c, pal, "取消", "放弃本次编辑", busy).Clicked() {
				a.closeConflictEditor()
			}
			if primaryAction(c, pal, "保存并标记已解决", "写入工作区并暂存该文件", busy).Clicked() {
				a.saveConflictEditor()
			}
		})
	})
}

func (a *app) conflictSideButton(c *ui.Context, pal palette, label string, side conflictEditorSide, selected bool) {
	btn := ui.ButtonBase(c).Height(26).Shrink(0).Padding(0, 8).Radius(radiusSM).
		Cursor(ui.CursorPointer).Label(label).Tooltip("用这份内容作为左侧对比基线")
	if selected {
		btn.Background(pal.selection).Border(1, pal.hairlineStrong)
	} else if btn.Hovered() {
		btn.Background(pal.surface2).Border(1, pal.hairlineStrong)
	} else {
		btn.Background(ui.Transparent).Border(1, pal.hairline)
	}
	btn.Children(func() { ui.Text(c, label).FontSize(textCaption).TextColor(pal.inkMuted) })
	if btn.Clicked() && !selected {
		a.setConflictSide(side)
	}
}

// setConflictSide 换对比基线：Diff 的左侧和整文件编辑器的基线必须永远指同一
// 份缓冲区 —— 两种模式说的是同一件事，切了基线只有一边变就是自相矛盾。
func (a *app) setConflictSide(side conflictEditorSide) {
	s := &a.conflictEdit
	s.side = side
	old := s.sideBuffer(side)
	if s.diff != nil {
		s.diff.Old = old
	}
	if s.code != nil {
		s.code.DiffBase = old
	}
}

// sideBuffer 返回某一边对应的缓冲区。三方版本都在这一个 state 里，散在几处
// switch 里迟早会有一处漏改（Diff 的左侧和编辑器的基线必须永远指同一份）。
func (s *conflictEditorState) sideBuffer(side conflictEditorSide) *editor.Buffer {
	switch side {
	case conflictSideTheirs:
		return s.theirs
	case conflictSideBase:
		return s.base
	default:
		return s.ours
	}
}

func (a *app) saveConflictEditor() {
	s := &a.conflictEdit
	if !s.on || s.diff == nil || s.diff.New == nil || a.op != gitrepo.OpNone {
		return
	}
	path := s.path
	text := s.diff.New.String()
	if s.plain && s.code != nil && s.code.Buffer != nil {
		text = s.code.Buffer.String()
	}
	a.startOp(gitrepo.OpConflict, "正在保存并解决 "+path, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		if err := repo.ResolveManual(ctx, path, []byte(text)); err != nil {
			return "", err
		}
		return "已保存并解决 " + path, nil
	}, func(ok bool) {
		if ok {
			a.closeConflictEditor()
		}
		a.reloadAfterOp()
	})
}
