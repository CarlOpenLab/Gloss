package main

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 文件历史：文件行右键 → 「查看文件历史…」，列的是这个文件自己的提交。
//
// 和历史 tab 那份提交列表不是一回事：那是整条分支的历史，一个文件在里面
// 出现几次得靠翻。这里只问 git「这个文件被改过几次」（git log --follow），
// 点其中一次就跳到历史 tab 去看那一次的 diff —— 弹窗只负责挑，看 diff 的
// 活还是历史 tab 干。

// fileHistLimit 一次读多少条文件历史。
//
// 500 和 logBatchSize 对齐：够长（一个文件被改 500 次的仓库不多）又不会
// 为了一个弹窗把整个历史读完。真超过这个数，最后一条会注明还有更早的，
// 用户改用历史 tab 里的筛选看。
const fileHistLimit = 500

// fileHistState 是「文件历史」对话框的状态。
type fileHistState struct {
	on   bool
	path string // 相对仓库的路径（标题下面那行）
	// seq 挡过期结果：换了文件、或者用户已经把弹窗关了，先前那次读回来的
	// 东西就不能再往上摆（和 loadCommits 的 logSeq 一个用法）。
	seq     int
	state   loadState
	err     string
	commits []gitrepo.Commit
	scroll  ui.ListState
	// jump 是点中的那次提交，等下一帧开头再跳（见 openCommitInHistory）。
	//
	// 不能在行回调里当场跳：那会儿列表正在自己建行，跳到一半把列表状态换掉
	// 或者清掉，框架下一行就撞空指针（ListState 里嵌着这一帧的 frame）。
	jump string
}

// openFileHistory 打开文件历史对话框，并去后台读这个文件的提交。
func (a *app) openFileHistory(f gitrepo.File) {
	if a.repoPath == "" {
		return
	}
	p := pathOf(f)
	a.fileHist.on = true
	a.fileHist.path = p
	a.fileHist.commits = nil
	a.fileHist.err = ""
	a.fileHist.state = loadLoading
	a.fileHist.seq++
	seq := a.fileHist.seq
	dir := a.repoPath

	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var commits []gitrepo.Commit
		if err == nil {
			commits, err = repo.FileLog(context.Background(), p, fileHistLimit)
		}
		a.dispatch(func() {
			if a.fileHist.seq != seq || !a.fileHist.on {
				return // 已经换文件或者关掉了，这份结果作废
			}
			if err != nil {
				a.fileHist.state = loadFailed
				a.fileHist.err = err.Error()
				return
			}
			a.fileHist.commits = commits
			a.fileHist.state = loadReady
		})
	})
}

// closeFileHistory 关掉弹窗并把它带的状态清干净：下次为另一个文件打开时
// 不该先闪一眼上一个文件的提交。
//
// scroll（那个 ui.ListState）**不清**：它是列表自己那一帧的记账本，清掉
// 正在建行的列表会当场崩 —— 而 Esc 关弹窗正好发生在建完内容之后、列表
// 惰性建行之前。留着它没有任何副作用，框架下一帧自己会按新的行数对齐。
func (a *app) closeFileHistory() {
	a.fileHist.on = false
	a.fileHist.commits = nil
	a.fileHist.err = ""
	a.fileHist.state = loadIdle
	a.fileHist.jump = ""
}

// fileHistDialog 是对话框入口（画在 view 的对话框那一层）。
func (a *app) fileHistDialog(c *ui.Context, pal palette) {
	if !a.fileHist.on {
		return
	}
	open := true
	specModal(c, pal, &open, func() { a.fileHistBody(c, pal) })
	if !open {
		a.closeFileHistory()
	}
}

func (a *app) fileHistBody(c *ui.Context, pal palette) {
	t := c.Theme()
	fh := &a.fileHist

	ui.Column(c).Width(560).Gap(10).Children(func() {
		ui.Text(c, T("文件历史", "File History")).FontSize(textHeadline).FontWeight(weightSemibold)
		// 路径写在脸上：同一棵树里几个文件名字可能只差一个目录，而弹窗
		// 打开之后用户未必还记得自己右键的是哪一个。
		ui.Text(c, fh.path).Font("monospace").FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Height(360).Radius(radiusMD).Background(pal.surface2).
			Border(1, pal.hairline).Children(func() {
			switch {
			case fh.state == loadLoading:
				ui.Box(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Text(c, T("正在读取文件历史…", "Loading file history…")).FontSize(12).TextColor(t.TextMuted)
				})
			case fh.state == loadFailed:
				ui.Box(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(16).Children(func() {
					ui.Text(c, T("读不出这个文件的历史", "Could not read this file's history")).
						FontSize(12).TextColor(t.TextMuted)
				})
			case len(fh.commits) == 0:
				ui.Box(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(16).Children(func() {
					// 空 = 这个文件还没有任何一次提交碰过它（新加的文件就是
					// 这样）。说清楚是「还没有」，不是「读失败了」。
					ui.Text(c, T("Git 里还没有这个文件的历史", "No history for this file yet")).
						FontSize(12).TextColor(t.TextMuted)
				})
			default:
				fh.scroll.Key = func(i int) any {
					if i < len(fh.commits) {
						return "fh:" + fh.commits[i].Hash
					}
					return "fh"
				}
				ui.List(c, &fh.scroll, len(fh.commits), func(i int) {
					a.fileHistRow(c, pal, i)
				}).Grow(1)
			}
		})

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, T("关闭", "Close"), "", false).Clicked() {
				a.closeFileHistory()
			}
		})
	})
}

// fileHistRow 是文件历史里的一行：短哈希 + 主题 + 作者 + 相对时间。
//
// 列宽口径和历史 tab 的提交列表对齐（作者/时间/哈希那三个常数）：同一个
// 提交在两个地方出现时，扫过去的落点是一样的。
func (a *app) fileHistRow(c *ui.Context, pal palette, i int) {
	t := c.Theme()
	fh := &a.fileHist
	if i < 0 || i >= len(fh.commits) {
		return
	}
	cm := fh.commits[i]

	row := ui.Row(c).Height(commitH).Shrink(0).AlignItems(ui.Center).
		Padding(0, 10).Gap(8).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(pal.surface3)
	}
	fadeColors(row)
	row.Children(func() {
		ui.Text(c, cm.Short).Font("monospace").FontSize(10).
			Width(commitHashW).Shrink(0).TextColor(t.TextMuted)
		ui.Text(c, cm.Subject).FontSize(12).SingleLine().Grow(1).MinWidth(0)
		ui.Text(c, cm.Author).FontSize(11).SingleLine().
			Width(commitAuthorW).Shrink(0).TextColor(t.TextMuted)
		ui.Text(c, shortWhen(cm.When, time.Now())).FontSize(11).
			Width(commitWhenW).Shrink(0).TextAlign(ui.End).TextColor(t.TextMuted)
	})
	if row.Clicked() {
		a.openCommitInHistory(cm.Hash)
	}
}

// openCommitInHistory 记下要跳去看哪次提交，真正的活在下一帧开头做
// （consumeHistoryJump）—— 它多半是在文件历史的行回调里被点起来的，
// 当场跳会动到正在建行的那个列表。
func (a *app) openCommitInHistory(hash string) {
	if hash == "" {
		return
	}
	a.fileHist.jump = hash
}

// consumeHistoryJump 在 view 开头把「从文件历史跳过来」的那次跳转做掉。
func (a *app) consumeHistoryJump() {
	hash := a.fileHist.jump
	if hash == "" {
		return
	}
	a.fileHist.jump = ""
	if !a.repoReady() {
		a.closeFileHistory()
		return
	}
	a.closeFileHistory()
	// 从改动 tab 切走之前先把工作区的文件列表留好（exitCompare/startCompare
	// 同一个口径），否则切回来是空的。
	if a.tab == 0 && a.workFiles == nil {
		a.workFiles = a.allFiles
	}
	// 不走 setTab：它在「已经在这个 tab」时会整个不做事，而从改动 tab 过来
	// 时（或者本来就在历史 tab、只是换一个提交时）要的正是把下半区换掉。
	// enterHistory 顺带把视图切回 Git 面板 —— 从编辑器视图跳过来时，光改
	// tab 字段屏幕还停在编辑器那一栏。
	a.enterHistory()
	a.fileFilter = ""
	a.fileTreeFilter = ""

	// 这一次提交已经在当前这份列表里（大多数情况）就原地选中：用户正在
	// 看的那段历史不跳，上下文的其余提交还在原处。
	for i := range a.commits {
		if a.commits[i].Hash == hash {
			a.loadCommit(i)
			a.commitList.ScrollIntoView(i)
			return
		}
	}
	// 不在：历史长过一屏（首批只读 500 条），或者当前看的是别的分支 ——
	// 把历史切到「从这次提交开始往下看」。loadCommits 会选中第 0 条，
	// 也就是它。
	a.refSel = ""
	a.histRev = hash
	a.histAll = false // 切到某一次提交上了，不再是「所有分支」
	a.commitSel = -1
	a.commits = nil
	a.allCommits = nil
	a.histFiles = nil
	a.histListID = ""
	a.stashRev = ""
	a.histDiff = loadIdle
	a.loadCommits()
}
