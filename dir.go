package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 目录行的右键菜单：一个文件夹底下的改动一起处理。
//
// 单文件的行已经有 fileMenu，目录这边一直是空白 —— 而「这半个仓库的改动
// 先都收进 index」恰好是最常有的诉求：拆了一个大功能，改动散在十来个文件
// 里，逐个勾一遍不如对着文件夹来一下。
//
// 菜单和文件行共用一个入口（treeRowIn 里的 ContextMenu），条目按「有多常用、
// 有多危险」排，和 fileMenu 同一个次序：批量暂存在最上面，纯读取的在中间，
// 放弃变更压在下半段 —— 它是这一片里唯一不可逆的动作。
func (a *app) dirMenu(m *ui.Menu, n *filetree.Node, o rowSpec) {
	files := dirFiles(n, o.list)
	busy := a.op != gitrepo.OpNone

	// 分组行（g:s / g:u）不是路径，复制路径那一组对它没有意义
	group := strings.HasPrefix(n.Key, "g:")

	staged := 0
	unstaged := 0
	for _, f := range files {
		switch f.Section {
		case gitrepo.SectionStaged:
			staged++
		case gitrepo.SectionUnstaged:
			unstaged++
		}
	}
	// 历史提交、储藏的 diff 是只读的（SectionNone）：那边没有暂存/丢弃
	// 可言 —— 那些动作改的是工作区，而屏幕上是别的版本的文件。
	live := staged > 0 || unstaged > 0

	if live {
		// 计数写在条目上：一次动多少个文件是用户该在下手前看到的，
		//尤其在目录是收起着的时候。
		//
		// 计数是 0 的条目直接不摆，而不是灰着：「全部取消暂存（0 个文件）」
		// 摆在一列菜单里只是在说一件刚才已经说完的事。灰留给「有这事但此刻
		// 点不了」（有写操作在飞，见 busy）。
		if unstaged > 0 {
			if m.Item(Tf("全部暂存（%d 个文件）", "Stage All (%d files)", unstaged)).
				Disabled(busy).Chosen() {
				a.stageDirFiles(files)
			}
		}
		if staged > 0 {
			if m.Item(Tf("全部取消暂存（%d 个文件）", "Unstage All (%d files)", staged)).
				Disabled(busy).Chosen() {
				a.unstageDirFiles(files)
			}
		}
		if unstaged > 0 {
			m.Separator()
			if m.Item(Tf("全部放弃变更…（%d 个文件）", "Discard All Changes… (%d files)", unstaged)).
				Disabled(busy).Chosen() {
				a.promptDiscardDir(n.Label, files)
			}
		}
	}

	if group {
		return
	}
	// 分割线只画在「前面真有东西」的时候：只读的那几栏（历史、储藏）菜单里
	// 没有写操作，开头一道线看着像被谁咬了一口。
	if live {
		m.Separator()
	}
	if m.Item(T("在访达中显示", "Reveal in Finder")).Chosen() {
		a.revealDir(n.Path)
	}
	p := filepath.FromSlash(n.Path)
	if m.Item(T("复制路径", "Copy Path")).Chosen() {
		a.copyText = filepath.Join(a.repoPath, p)
	}
	if m.Item(T("复制相对路径", "Copy Relative Path")).Chosen() {
		a.copyText = p
	}
}

// dirFiles 收集一个目录节点底下列出来的全部文件，含子目录里的。
//
// 走的是**树上挂着的 FileIdx**，不是磁盘上的路径：批量操作要动的就是用户
// 眼前这一份（筛选之后树里只剩匹配的文件），照着磁盘重新走一遍会把筛掉的
// 文件也卷进去 —— 用户看着三个文件，动的却是三十个。
//
// 分组行（g:s / g:u）也走这条：它们的子节点就是那一整段，收集出来正好是
// 「已暂存的全部」/「未暂存的全部」，不必为它另写一套。
func dirFiles(n *filetree.Node, list []gitrepo.File) []gitrepo.File {
	var out []gitrepo.File
	var walk func(*filetree.Node)
	walk = func(n *filetree.Node) {
		if n.FileIdx >= 0 && n.FileIdx < len(list) {
			out = append(out, list[n.FileIdx])
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	return out
}

// stageDirFiles 把一个目录底下的文件全部放进 index。
func (a *app) stageDirFiles(files []gitrepo.File) {
	paths := dirStagePaths(files)
	if len(paths) == 0 {
		return
	}
	label := Tf("正在暂存 %d 个文件", "Staging %d files", len(paths))
	a.startOp(gitrepo.OpStage, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "", repo.StageFile(ctx, paths...)
	}, func(bool) { a.reloadAfterOp() })
}

// unstageDirFiles 把一个目录底下已暂存的文件全部撤出 index。
func (a *app) unstageDirFiles(files []gitrepo.File) {
	paths := dirStagePaths(files)
	if len(paths) == 0 {
		return
	}
	label := Tf("正在取消暂存 %d 个文件", "Unstaging %d files", len(paths))
	a.startOp(gitrepo.OpUnstage, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "", repo.UnstageFile(ctx, paths...)
	}, func(bool) { a.reloadAfterOp() })
}

// dirStagePaths 是批量暂存/取消暂存要交给 git 的路径。
//
// 重命名得新旧两个都给（见 stagePaths 的理由），所以这里不能图省事只收
// pathOf —— 只给新路径的话，旧路径在 index 里还挂着「已跟踪」，暂存完是
// 一半重命名一半还没，用户得再点一次。
func dirStagePaths(files []gitrepo.File) []string {
	var paths []string
	for _, f := range files {
		paths = append(paths, stagePaths(f)...)
	}
	return dedupPaths(paths)
}

// dirDiscardPaths 把一个目录底下要丢的改动分成两路：git 认识的（checkout 回
// index 里那份）和未跟踪的（从磁盘上拿走）。
//
// 只收未暂存那一段，冲突文件排除 —— 已暂存的是已经收进 index 的东西（走
// 「取消暂存」，一个字都不丢），冲突文件在 index 里没有 stage 0 的条目，
// checkout 会直接报 unmerged，它该走的是那两条「挑一边」。
//
// 未跟踪的不能交给 checkout —— 它对未跟踪文件报的是
// "pathspec did not match any file(s) known to git"。
func dirDiscardPaths(files []gitrepo.File) (restore, remove []string) {
	for _, f := range files {
		if f.Section != gitrepo.SectionUnstaged || f.Status == gitrepo.StatusConflict {
			continue
		}
		if f.Status == gitrepo.StatusUntracked {
			remove = append(remove, pathOf(f))
			continue
		}
		restore = append(restore, stagePaths(f)...)
	}
	return dedupPaths(restore), dedupPaths(remove)
}

// promptDiscardDir 先问一句再丢一个目录底下的改动。
//
// 这道闸和 promptDiscard 是同一个理由，只是这里一次动一批：丢的是找不回来
// 的东西，而未跟踪的那些还会连文件一起从磁盘上删掉。
//
// 路径在问的时候就定下来：对话框里写的「N 个文件」和执行时动的必须是同一
// 批 —— 分别算两遍，中间列表刷新一次就对不上了。
func (a *app) promptDiscardDir(label string, files []gitrepo.File) {
	restore, remove := dirDiscardPaths(files)
	n := len(restore) + len(remove)
	if n == 0 {
		return
	}
	title := Tf("丢弃 %s 里的改动？", "Discard changes in %s?", label)
	body := Tf("这里面 %d 个文件会恢复成上一次暂存的样子，还没暂存的改动会丢掉，找不回来。未跟踪的文件会连文件一起删掉。",
		"The %d files here go back to what was last staged. Unstaged changes are lost for good; untracked files are deleted from disk.", n)
	a.askConfirm(confirmState{
		title: title,
		body:  body,
		yes:   T("丢弃更改", "Discard Changes"),
		onYes: func() {
			a.discardDirChanges(label, restore, remove)
		},
	})
}

// discardDirChanges 丢掉一批路径上的未暂存改动。
func (a *app) discardDirChanges(label string, restore, remove []string) {
	if len(restore) == 0 && len(remove) == 0 {
		return
	}
	n := len(restore) + len(remove)
	a.startOp(gitrepo.OpDiscard, Tf("正在丢弃 %d 个文件的改动", "Discarding changes in %d files", n),
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			// 先恢复再删：删文件对「已经不在」是宽容的，而 checkout 对一条
			// 不认识的路径报的是和用户做的事不相干的一句话。
			if len(restore) > 0 {
				if err := repo.RestoreFile(ctx, restore...); err != nil {
					return "", err
				}
			}
			if len(remove) > 0 {
				if err := repo.RemoveFiles(ctx, remove...); err != nil {
					return "", err
				}
			}
			return Tf("已丢弃 %s 里 %d 个文件的改动", "Discarded changes in %d files in %s", n, label), nil
		}, func(bool) { a.reloadAfterOp() })
}

// dedupPaths 去掉重复路径、保住原有顺序。
//
// 重命名的同一个文件会在树里占两行（旧路径被删 + 新路径未跟踪），stagePaths
// 各自给出新旧两个路径，合起来就会有重复 —— 重复的路径交给 git 不出错，
// 但「动了几个文件」那个数会虚高。
func dedupPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := paths[:0:0]
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// revealDir 在系统文件管理器里亮出一个目录。
func (a *app) revealDir(path string) {
	if a.revealPath == nil || a.repoPath == "" || path == "" {
		return
	}
	reveal, full := a.revealPath, filepath.Join(a.repoPath, filepath.FromSlash(path))
	a.run(func() { reveal(full) })
}
