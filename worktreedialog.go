package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 新建 / 移除 Worktree。新建表单照 newbranchdialog.go 的形状（一个输入框、
// 一句预检、两颗按钮），移除走两次确认：第一次说清会丢什么，普通移除被
// 未提交内容挡住时再问一次「强制移除」。

// worktreeState 是「在新 Worktree 中检出」对话框的状态。
type worktreeState struct {
	on        bool
	branch    string
	path      string
	focusPath bool
	err       gitrepo.Failure
}

// openNewWorktreeDialog 打开新建对话框，路径先填一个照着分支名生成的建议。
func (a *app) openNewWorktreeDialog(branch string) {
	if !a.repoReady() || a.op != gitrepo.OpNone || branch == "" {
		return
	}
	a.worktree = worktreeState{
		on:        true,
		branch:    branch,
		path:      a.defaultWorktreePath(branch),
		focusPath: true,
	}
}

// defaultWorktreePath 是默认落在仓库旁边的目录名：<仓库父目录>/<仓库名>-<分支>。
// 分支里的斜杠换成横杠 —— 目录名里再套一层目录，用户找起来反而费劲。
func (a *app) defaultWorktreePath(branch string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	repo := a.repoName
	if repo == "" {
		repo = filepath.Base(a.repoPath)
	}
	return filepath.Join(filepath.Dir(a.repoPath), repo+"-"+name)
}

// resolveWorktreePath 把输入框里的路径归一化成绝对路径。
//
// 相对路径按**仓库根目录**拼 —— 和 `git worktree add` 在仓库根跑时的理解
// 一致。空输入返回空串，交给 worktreeBlocker 说「还没填」。
func (a *app) resolveWorktreePath() string {
	p := strings.TrimSpace(a.worktree.path)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.repoPath, p)
	}
	return filepath.Clean(p)
}

// worktreeBlocker 是「还差什么才能创建」，空串表示可以点创建。
func (a *app) worktreeBlocker() string {
	if strings.TrimSpace(a.worktree.path) == "" {
		return T("先选一个工作区路径", "Choose a worktree path first")
	}
	if _, err := os.Stat(a.resolveWorktreePath()); err == nil {
		return T("这个目录已经存在", "This directory already exists")
	}
	return ""
}

// worktreeDialog 是对话框入口，画在 view 的对话框那一层。
func (a *app) worktreeDialog(c *ui.Context, pal palette) {
	if !a.worktree.on {
		return
	}
	open := true
	specModalSticky(c, pal, &open, func() { a.worktreeBody(c, pal) })
	if !open {
		a.worktree = worktreeState{}
	}
}

func (a *app) worktreeBody(c *ui.Context, pal palette) {
	t := c.Theme()
	ws := &a.worktree

	ui.Column(c).Width(480).Gap(12).Children(func() {
		ui.Text(c, T("在新 Worktree 中检出", "Check Out in New Worktree")).
			FontSize(textHeadline).FontWeight(weightSemibold)
		ui.Textf(c, T("分支 %s 将在一个独立工作目录里检出", "Branch %s will be checked out in a separate working directory"), ws.branch).
			FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, T("路径", "Path")).FontSize(11).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				in := ui.TextInput(c, &ws.path).Placeholder(T("工作目录路径", "Working directory path")).
					Label(T("Worktree 路径", "Worktree Path"))
				if ws.focusPath {
					in.Focus()
					ws.focusPath = false
				}
				if in.Submitted() {
					a.createWorktree()
				}
				if secondaryAction(c, pal, T("选择…", "Choose…"), T("选择工作区目录", "Choose a working directory"), false).Clicked() {
					a.chooseWorktreeFolder()
				}
			})
		})

		ui.Text(c, T("创建后会作为新项目打开。", "It opens as a new project after creation.")).
			FontSize(11).TextColor(t.TextMuted)

		if msg := a.worktreeBlocker(); msg != "" {
			ui.Text(c, msg).FontSize(11).TextColor(t.TextMuted)
		}
		failureBox(c, pal, t, ws.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, T("取消", "Cancel"), "", false).Clicked() {
				a.worktree = worktreeState{}
				return
			}
			btn := primaryAction(c, pal, T("创建 Worktree", "Create Worktree"),
				T("在所选目录检出这条分支", "Check out the branch in the chosen directory"),
				a.worktreeBlocker() != "" || a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.createWorktree()
			}
		})
	})
}

// chooseWorktreeFolder 弹原生目录选择框。阻塞调用必须放到后台（同
// chooseFolder）。
func (a *app) chooseWorktreeFolder() {
	a.run(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:                     a.win,
			Title:                      T("选择 Worktree 目录", "Select Worktree Directory"),
			Directory:                  true,
			TreatPackagesAsDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.dispatch(func() {
			if a.worktree.on {
				a.worktree.path = paths[0]
			}
		})
	})
}

// createWorktree 把选中的分支检出到输入框里的目录。
//
// 成功后关掉对话框、把原项目的 refs 标脏并重读，再把新目录作为新项目打开
// （openInNewTab 不替换当前项目）。loadRefs 放在 openInNewTab 之前发起是
// 故意的：结果迟到时会按发起时的目录写回原项目快照，不会串到新项目上。
func (a *app) createWorktree() {
	ws := &a.worktree
	if !ws.on || a.worktreeBlocker() != "" || a.op != gitrepo.OpNone {
		return
	}
	path := a.resolveWorktreePath()
	branch := ws.branch
	root := a.repoPath
	ws.err = gitrepo.Failure{}

	a.startOp(gitrepo.OpWorktree, T("正在检出 Worktree ", "Checking out worktree ")+branch,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return T("已在新 Worktree 中检出 ", "Checked out ") + branch, repo.AddWorktree(ctx, path, branch)
		}, func(ok bool) {
			if !ok {
				// 失败留在对话框里：路径或分支还能改（目录已存在、分支被占用
				// 都是改一改就能过的）。
				if a.opErr != nil {
					ws.err = *a.opErr
					a.opErr = nil
				}
				return
			}
			a.worktree = worktreeState{}
			a.markRefsDirtyForRepo(root)
			a.loadRefs()
			a.openInNewTab(path)
		})
}

// promptRemoveWorktree 是移除 worktree 的第一次确认：写明路径、会丢什么，
// 以及这个项目在 Gloss 里开着时终端和未保存编辑会怎样。
func (a *app) promptRemoveWorktree(path string, main bool) {
	if path == "" || main || a.op != gitrepo.OpNone {
		return
	}
	body := Tf("目录 %s 会被删除；里面未提交的改动会被 git 拦住，强制移除时会一起消失。",
		"Directory %s will be deleted; uncommitted changes make git refuse, and a forced removal discards them.", path)
	if a.worktreeOpenAsProject(path) {
		body += "\n" + T("它在 Gloss 里作为项目打开着：终端会话会被关闭。",
			" It is open as a project in Gloss: its terminal sessions will be closed.")
	}
	if a.worktreeHasUnsavedEdits(path) {
		body += "\n" + T("这个项目还有未保存的编辑，移除后会一起丢失。",
			"This project also has unsaved edits; removing it discards them.")
	}
	a.askConfirm(confirmState{
		title: T("移除这个 Worktree？", "Remove this worktree?"),
		body:  body,
		yes:   T("移除", "Remove"),
		onYes: func() { a.removeWorktreeNow(path, false) },
	})
}

// worktreeOpenAsProject 说这个路径是不是正作为一个项目开着。
func (a *app) worktreeOpenAsProject(path string) bool {
	for _, tab := range a.tabs {
		if sameRepo(tab.path, path) {
			return true
		}
	}
	return false
}

// worktreeHasUnsavedEdits 说这个路径对应的项目里有没有没保存的编辑。
func (a *app) worktreeHasUnsavedEdits(path string) bool {
	for i, tab := range a.tabs {
		if sameRepo(tab.path, path) {
			return a.tabHasUnsavedEdits(i)
		}
	}
	return false
}

// askForceRemoveWorktree 是脏 worktree 的第二次确认。
func (a *app) askForceRemoveWorktree(path string) {
	a.askConfirm(confirmState{
		title: T("强制移除这个 Worktree？", "Force remove this worktree?"),
		body: T("里面有未提交的改动，强制移除会把它们一起丢掉。",
			"It has uncommitted changes; forcing the removal discards them."),
		yes:   T("强制移除", "Force Remove"),
		onYes: func() { a.removeWorktreeNow(path, true) },
	})
}

// removeWorktreeNow 真正动手移除。
//
// 先关掉这个 worktree 的终端（Windows 上打开的文件句柄会让目录删不掉），
// 再跑 git；成功后关掉对应项目 tab 并重读引用。git 的「脏目录」拒绝被
// Failure.Code 认出来，转成第二次「强制移除」确认。
func (a *app) removeWorktreeNow(path string, force bool) {
	for _, tab := range a.tabs {
		if sameRepo(tab.path, path) {
			releaseTermSessions(tab)
		}
	}
	// anchor 用来事后找「同一仓库的所有项目」：当前项目可能就是被删的那个，
	// 删完再 Open 它就打不开了。主工作区的路径在删之前记下来。
	anchor := a.repoPath
	a.startOp(gitrepo.OpWorktree, T("正在移除 Worktree", "Removing worktree"),
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			if wts, err := repo.Worktrees(ctx); err == nil {
				for _, w := range wts {
					if w.Main {
						anchor = w.Path
					}
				}
			}
			return T("已移除 Worktree ", "Removed worktree ") + path, repo.RemoveWorktree(ctx, path, force)
		}, func(ok bool) {
			if !ok {
				if !force && a.opErr != nil && a.opErr.Code == gitrepo.FailureCodeWorktreeDirty {
					a.opErr = nil
					a.askForceRemoveWorktree(path)
				}
				return
			}
			a.markRefsDirtyForRepo(anchor)
			a.closeProjectTabByPath(path)
			if a.repoPath == "" {
				return
			}
			a.loadRefs()
		})
}

// closeProjectTabByPath 强制关掉路径对应的项目 tab（不再问未保存内容）：
// 调用它的确认框已经把「未保存的编辑会丢」写在正文里了。
func (a *app) closeProjectTabByPath(path string) bool {
	idx := -1
	for i, tab := range a.tabs {
		if sameRepo(tab.path, path) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	if idx == a.activeTab {
		a.closeTransientEditors()
		a.discardEditorTabs(&a.editorPane)
		a.discardEditorTabs(&a.tabs[idx].editorPane)
	} else {
		a.discardEditorTabs(&a.tabs[idx].editorPane)
		a.tabs[idx].review = reviewState{}
	}
	a.closeTab(idx)
	return true
}

// pruneWorktrees 清掉目录已经不在了的 worktree 记录。
func (a *app) pruneWorktrees() {
	root := a.repoPath
	a.startOp(gitrepo.OpWorktree, T("正在清理失效 Worktree 记录", "Pruning stale worktree records"),
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return T("已清理失效 Worktree 记录", "Pruned stale worktree records"), repo.PruneWorktrees(ctx)
		}, func(ok bool) {
			if !ok {
				return
			}
			a.markRefsDirtyForRepo(root)
			a.loadRefs()
		})
}
