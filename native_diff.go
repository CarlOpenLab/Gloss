package main

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// nativeDiffState is a per-file native DiffView preview. It deliberately stays
// separate from the existing virtualized renderer: the latter owns staged/hunk
// actions and file navigation, while this view adds MyGo's word-level inline
// diff, folding, keyboard navigation and optional editing without changing Git
// state until the user explicitly saves.
type nativeDiffState struct {
	on       bool
	loading  bool
	path     string
	err      string
	editable bool
	old, new *editor.Buffer
	diff     *editor.DiffState
}

func (a *app) openNativeDiff(f gitrepo.File) {
	if a.repoPath == "" || a.op != gitrepo.OpNone {
		return
	}
	path := pathOf(f)
	a.nativeDiff = nativeDiffState{on: true, loading: true, path: path, editable: false}
	section := f.Section
	root := a.repoPath
	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var old, nw []byte
		if err == nil {
			old, nw, err = nativeDiffContents(ctx, repo, path, section)
		}
		if err == nil && (!utf8.Valid(old) || !utf8.Valid(nw)) {
			err = fmt.Errorf("%s 不是可编辑的文本差异", path)
		}
		a.dispatch(func() {
			if !a.nativeDiff.on || a.nativeDiff.path != path {
				return
			}
			if err != nil {
				a.nativeDiff.loading = false
				a.nativeDiff.err = err.Error()
				return
			}
			lang := editor.LanguageFor(path)
			makeBuffer := func(data []byte) *editor.Buffer {
				b := editor.NewBuffer(string(data))
				b.SetLanguage(lang)
				return b
			}
			a.nativeDiff.old = makeBuffer(old)
			a.nativeDiff.new = makeBuffer(nw)
			a.nativeDiff.diff = &editor.DiffState{
				Old: a.nativeDiff.old, New: a.nativeDiff.new,
				Layout: editor.DiffSplit, Inline: editor.DiffInlineWordAlt,
				Indicators: editor.DiffBars, ContextLines: 3,
				Editable: false,
			}
			a.nativeDiff.loading = false
		})
	})
}

func nativeDiffContents(ctx context.Context, repo *gitrepo.Repo, path string, section gitrepo.Section) (old, nw []byte, err error) {
	switch section {
	case gitrepo.SectionStaged:
		old, err = repo.ContentsAt(ctx, "HEAD", path)
		if err != nil {
			// New files have an empty old side.
			old = nil
		}
		nw, err = repo.ContentsAt(ctx, ":0", path)
	case gitrepo.SectionUnstaged:
		old, err = repo.ContentsAt(ctx, ":0", path)
		if err != nil {
			old = nil
		}
		nw, err = repo.Contents(path)
	default:
		// Historical/PR entries are read-only here; the existing content cache
		// remains the authoritative source for their exact selected revision.
		return nil, nil, fmt.Errorf("该文件不支持内置 Diff 预览")
	}
	return old, nw, err
}

func (a *app) closeNativeDiff() { a.nativeDiff = nativeDiffState{} }

func (a *app) nativeDiffDialog(c *ui.Context, pal palette) {
	if !a.nativeDiff.on {
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
			t := c.Theme()
			s := &a.nativeDiff
			ui.Column(c).Fill().Children(func() {
				ui.Row(c).Height(48).Shrink(0).Padding(0, 14).Gap(8).AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, "内置 Diff 预览").FontSize(textHeadline).FontWeight(weightSemibold).
							TextColor(pal.ink)
						ui.Text(c, s.path).Font("monospace").FontSize(textCaption).
							SingleLine().TextColor(pal.inkMuted)
					})
					if xButton(c, pal, "关闭") {
						a.closeNativeDiff()
					}
				})
				if s.loading {
					ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
						ui.Spinner(c)
						ui.Text(c, "正在读取文件两侧…").TextColor(t.TextMuted)
					})
					return
				}
				if s.err != "" || s.diff == nil {
					ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
						headline(c, pal, "无法打开内置 Diff")
						ui.Text(c, s.err).Font("monospace").FontSize(textBodySM).TextColor(t.TextMuted)
						if secondaryAction(c, pal, "关闭", "", false).Clicked() {
							a.closeNativeDiff()
						}
					})
					return
				}
				ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
					th := editor.DefaultTheme(t)
					th.Background = pal.codeBg
					th.Text = pal.code
					s.diff.Theme = th
					s.diff.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
					editor.DiffView(c, s.diff).Fill()
				})
				ui.Row(c).Height(44).Shrink(0).Padding(0, 12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "MyGo DiffView：行内变更高亮、折叠未改动区间、双栏联动滚动").
						FontSize(textCaption).TextColor(pal.inkMuted)
					ui.Spacer(c)
					if secondaryAction(c, pal, "关闭", "", false).Clicked() {
						a.closeNativeDiff()
					}
				})
			})
		})
	})
	if !open {
		a.closeNativeDiff()
	}
}
