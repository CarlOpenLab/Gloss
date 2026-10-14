package main

import (
	"context"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// fileMenu 是文件行的右键菜单。改动/历史/储藏三处共用一个：同一条路径在
// 三种视图里能做的事差不多，分开写迟早会各处漏一项。
//
// 条目按「有多常用、有多危险」排：暂存/丢弃在最上面（review 时最常点），
// 复制路径这类纯读取的在中间，标记已查看在下面，加入 .gitignore 压在最后
// —— 它写的是用户自己的 .gitignore 文件，不该在手滑的半径里。
func (a *app) fileMenu(m *ui.Menu, f gitrepo.File) {
	busy := a.op != gitrepo.OpNone
	// 已经在看的这份 diff 是只读的（历史提交、储藏）：那里没有暂存/丢弃
	// 可言 —— 那些动作改的是工作区，而屏幕上是别的版本的文件。
	live := f.Section != gitrepo.SectionNone
	conflict := f.Status == gitrepo.StatusConflict
	untracked := f.Status == gitrepo.StatusUntracked
	unstaged := f.Section == gitrepo.SectionUnstaged

	if live && conflict {
		// 冲突文件的解决动作摆最前面：保留远端统一 fileMenu，同时提供
		// 内置 Diff 编辑器和外部编辑器两条手工解决路径。
		act := a.conflictActionsFor(f)
		if m.Item(act.ours).Disabled(busy).Chosen() {
			a.resolveFile(f, gitrepo.SideOurs, act.ours)
		}
		if m.Item(act.theirs).Disabled(busy).Chosen() {
			a.resolveFile(f, gitrepo.SideTheirs, act.theirs)
		}
		m.Separator()
	} else if live && !conflict {
		// 整文件编辑：改动区看的是 diff，这里直接改盘上那一份。冲突文件
		// 不走这里 —— 它的出入口在改动区顶上那条（挑边 / 标记已解决），
		// 收尾还要 git add。
		if m.Item("编辑代码").Disabled(busy).Chosen() {
			a.openCodeEditor(f)
		}
	}

	if live {
		if m.Item(stageLabel(f)).Disabled(busy).Chosen() {
			a.toggleStageFile(f)
		}
	}
	// 丢弃只在未暂存段：那一段才是「工作区里还没收进 index 的东西」，
	// 丢了就是丢了。已暂存段想反悔走「取消暂存」，一个字都不会丢。
	// 冲突文件也不给：它在 index 里没有 stage 0 的条目，checkout 会直接
	// 报 unmerged，而它该走的是上面那两条「挑一边」。
	if untracked || (unstaged && !conflict) {
		label := T("丢弃更改…", "Discard Changes…")
		if untracked {
			// 未跟踪的文件 git 还不认识，「丢弃」= 从磁盘上删掉。按钮上
			// 直说要删文件，别让人以为只是撤销一下改动。
			label = T("删除这个文件…", "Delete File…")
		}
		if m.Item(label).Disabled(busy).Chosen() {
			a.promptDiscard(f)
		}
	}

	m.Separator()
	if m.Item(T("在编辑器打开", "Open in Editor")).Disabled(busy).Chosen() {
		a.openInEditor(f)
	}
	if m.Item(T("在访达中显示", "Reveal in Finder")).Chosen() {
		a.revealInFinder(f)
	}

	m.Separator()
	p := pathOf(f)
	if m.Item(T("复制路径", "Copy Path")).Chosen() {
		a.copyText = filepath.Join(a.repoPath, p)
	}
	if m.Item(T("复制相对路径", "Copy Relative Path")).Chosen() {
		a.copyText = p
	}
	if m.Item(T("复制文件名", "Copy File Name")).Chosen() {
		a.copyText = filepath.Base(p)
	}

	m.Separator()
	// 未跟踪的文件不在任何一次提交里，历史必然是空的 —— 灰着比点开一个
	// 空弹窗清楚。
	if m.Item(T("查看文件历史…", "View File History…")).Disabled(untracked).Chosen() {
		a.openFileHistory(f)
	}

	m.Separator()
	if viewed := a.isViewed(f); viewed {
		if m.Item(T("取消已查看", "Mark as Unreviewed")).Chosen() {
			a.toggleViewed(f)
		}
	} else {
		if m.Item(T("标记为已查看", "Mark as Reviewed")).Chosen() {
			a.toggleViewed(f)
		}
	}
	if untracked {
		if m.Item(T("加入 .gitignore", "Add to .gitignore")).Disabled(busy).Chosen() {
			a.addToGitignore(f)
		}
	}
}

// promptDiscard 先问一句再丢改动。
//
// 这道闸不能省：丢弃是这一片里唯一不可逆的动作 —— 暂存、复制、标记已查看
// 都能反悔，改坏了的文件内容不能。未跟踪的那个还会连文件一起删掉。
func (a *app) promptDiscard(f gitrepo.File) {
	p := pathOf(f)
	title := Tf("丢弃 %s 的改动？", "Discard changes in %s?", p)
	body := T("这个文件会恢复成上一次暂存的样子，工作区里还没暂存的改动会丢掉，找不回来。",
		"The file goes back to what was last staged. Unstaged changes are lost for good.")
	yes := T("丢弃更改", "Discard Changes")
	if f.Status == gitrepo.StatusUntracked {
		title = Tf("删除 %s？", "Delete %s?", p)
		body = T("这个文件还没被 git 跟踪，删掉就是真的从磁盘上删掉，找不回来。",
			"The file is untracked, so deleting it removes it from disk for good.")
		yes = T("删除文件", "Delete File")
	}
	a.askConfirm(confirmState{
		title: title,
		body:  body,
		yes:   yes,
		onYes: func() { a.discardChanges(f) },
	})
}

// discardChanges 丢弃一个文件工作区里的改动。
//
// 两条路：git 已经认识它，就 checkout 回 index 里那份（改内容的回到原样，
// 删掉的也一起被恢复回来）；不认识它（未跟踪），恢复原状就是从磁盘上删掉。
//
// 工作区里的重命名不是第三种情况 —— 实测 git status 把它报成「旧路径被删」
// 加「新路径是未跟踪」（--porcelain=v2 也是，工作区那一侧不做重命名检测）。
// 两行各按自己的类型丢：旧路径恢复回来，新路径当未跟踪文件删掉，合起来正好
// 是「改名前」。所以这里不需要认重命名。
func (a *app) discardChanges(f gitrepo.File) {
	p := pathOf(f)
	label := Tf("正在丢弃 %s 的改动", "Discarding changes in %s", p)
	a.startOp(gitrepo.OpDiscard, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		if f.Status == gitrepo.StatusUntracked {
			return Tf("已删除 %s", "Deleted %s", p), repo.RemoveFiles(ctx, p)
		}
		return Tf("已丢弃 %s 的改动", "Discarded changes in %s", p), repo.RestoreFile(ctx, stagePaths(f)...)
	}, func(bool) {
		a.reloadAfterOp()
	})
}

// addToGitignore 把这条路径写进仓库根的 .gitignore，下一轮刷新它就会从
// 未跟踪那一堆里消失。
func (a *app) addToGitignore(f gitrepo.File) {
	p := pathOf(f)
	a.startOp(gitrepo.OpDiscard, Tf("正在忽略 %s", "Ignoring %s", p), func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return Tf("已把 %s 加进 .gitignore", "Added %s to .gitignore", p), repo.AddIgnore(ctx, p)
	}, func(bool) {
		// 重读一遍：那条路径从「未跟踪」里消失，而 .gitignore 自己多了一条
		// 改动（仓库里已经提交过它的话）—— 两件事都在这一次刷新里上屏。
		a.reloadAfterOp()
	})
}

// revealInFinder 在系统文件管理器里亮出这个文件。
//
// 和 openInEditor 一个道理：真跑在 goroutine 里，别让一次「打开访达」
// 占住主线程 —— 这一层每毫秒都在画画。
func (a *app) revealInFinder(f gitrepo.File) {
	if a.revealPath == nil || a.repoPath == "" {
		return
	}
	reveal, path := a.revealPath, filepath.Join(a.repoPath, pathOf(f))
	a.run(func() { reveal(path) })
}
