package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 历史 tab：上半是提交列表（带拓扑图），下半是选中提交的「文件列表 + diff」。
//
// 列表占满主区全宽、diff 压到下面 —— 这是 Fork 的样子。之前列表挤在主区左
// 半边时只有 420px，拓扑图被压在最左边一条 78px 的窄栏里，铺不开。

// logBatchSize 一次读多少个提交。默认首批读 500 个，向下滚动或键盘导航到底部时自动增量加载。
const logBatchSize = 500

// defaultCommitH 是提交列表那一栏的高度，占主区高度的四成半。
//
// 没写死成一个常数：窗口 860 高和 420 高（最小尺寸）差一倍，同一个数字
// 在矮窗口里会把 diff 挤没。这里是比例的分子，真正夹的时候还要看可用高度。
const defaultCommitH = 0.45

// setTab 切换「改动 / 历史」。只改状态，真正的加载在后台做。
//
// 仓库没开成（或者还没开完）时哪一路都读不出东西，直接不切：工具栏上那两格
// 这时是灰的，⌘1 ⌘2、菜单和命令栏却都通到这里 —— 在入口上把住，
// 几处就不会各说各话。切过去还会真发一轮注定失败的 git 读，然后在历史里
// 报「读不到历史」，而其实根本是仓库没开。
func (a *app) setTab(t int) {
	if !a.repoReady() || t == a.tab {
		return
	}
	oldTab := a.tab
	a.tab = t
	// 两个 tab 的文件集完全不同，留着筛选词多半一个文件都看不到。
	// 侧栏的 refFilter 不清：那是分支树，两个 tab 共用同一棵。
	a.fileFilter = ""
	a.fileTreeFilter = ""

	switch t {
	case 0:
		if oldTab == 1 {
			a.histFiles = a.allFiles
			a.histListID = a.listID
		}
		if a.workFiles != nil {
			a.showFiles(a.workFiles, workListID(a.repoPath, a.workBranch()))
		} else {
			a.allFiles = nil
			a.applyFilter()
		}
	case 1:
		if a.histFiles != nil {
			a.showFiles(a.histFiles, a.histListID)
		} else {
			a.allFiles = nil
			a.applyFilter()
		}
		a.loadCommits()
		// 刚才在看储藏就把储藏接回来。切去改动 tab 时 allFiles 被清空了，
		// 不接回来的话下半区是空的，而上半区因为 stashRev 还在、不会自动
		// 选中任何提交 —— 两头都空着。提交的选中是恢复的，储藏一样。
		if a.stashRev != "" {
			if ref, ok := a.refByFull[a.stashRev]; ok {
				a.selectStash(*ref)
				return
			}
			a.stashRev = "" // 储藏已经没了（被 pop 掉），退回正常历史
		}
		if a.commitSel >= 0 && a.commitSel < len(a.commits) {
			if a.histFiles == nil || a.histListID != "c:"+a.commits[a.commitSel].Hash {
				a.loadCommit(a.commitSel)
			}
		}
	}
}

// loadCommits 在后台读提交列表。
func (a *app) loadCommits() {
	if a.repoPath == "" {
		return
	}
	// 已经有提交时不刷白屏幕，保持当前列表显示并在后台静默重读，
	// 避免切 tab 时闪烁「正在读提交…」。
	if len(a.commits) == 0 {
		a.hist = loadLoading
		a.histErr = ""
	}

	a.logSeq++
	seq := a.logSeq
	dir := a.repoPath
	rev := a.histRev
	a.histHasMore = true
	a.histLoadingMore = false

	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var commits []gitrepo.Commit
		if err == nil {
			commits, err = repo.Log(context.Background(), rev, logBatchSize, 0)
		}
		a.dispatch(func() {
			if a.logSeq != seq {
				return
			}
			if err != nil {
				if len(a.commits) == 0 {
					a.hist = loadFailed
					a.histErr = err.Error()
				}
				return
			}
			oldHead := ""
			if len(a.commits) > 0 {
				oldHead = a.commits[0].Hash
			}
			a.histHasMore = len(commits) == logBatchSize
			a.setCommits(commits)
			a.hist = loadReady
			// 进来先选中最近那个提交，省得用户还要再点一下。
			//
			// 正在看储藏的时候不能选：那会把下半区从储藏的改动换成某个
			// 提交的，用户点的是储藏，屏幕上是别的东西。
			if a.commitSel < 0 && a.stashRev == "" && len(commits) > 0 {
				a.loadCommit(0)
			} else if a.commitSel == 0 && a.stashRev == "" && len(commits) > 0 && commits[0].Hash != oldHead {
				a.loadCommit(0)
			}
		})
	})
}

// loadMoreCommits 在后台加载下一批更早的提交。
func (a *app) loadMoreCommits() {
	if !a.histHasMore || a.histLoadingMore || a.repoPath == "" {
		return
	}
	a.histLoadingMore = true
	seq := a.logSeq
	dir := a.repoPath
	rev := a.histRev
	offset := len(a.commits)

	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var more []gitrepo.Commit
		if err == nil {
			more, err = repo.Log(context.Background(), rev, logBatchSize, offset)
		}
		a.dispatch(func() {
			if a.logSeq != seq {
				return
			}
			a.histLoadingMore = false
			if err != nil || len(more) == 0 {
				a.histHasMore = false
				return
			}
			if len(more) < logBatchSize {
				a.histHasMore = false
			}
			a.setCommits(append(a.commits, more...))
		})
	})
}

// setCommits 换掉提交列表，顺便把拓扑图的布局算出来。
//
// 两者必须一起换：图的第 i 行就是列表的第 i 行，分开赋值迟早会对不上。
func (a *app) setCommits(commits []gitrepo.Commit) {
	a.commits = commits
	a.graph, a.maxLanes = buildGraph(commits)
}

// loadCommit 把某个提交的 diff 读出来，作为主区显示的内容。
func (a *app) loadCommit(i int) {
	if i < 0 || i >= len(a.commits) {
		return
	}
	a.commitSel = i
	a.fileSel = -1
	a.stashName = ""
	// 看提交了，储藏那个视图就到此为止（和上面清 commitSel 是一回事）
	a.stashRev = ""
	a.histDiff = loadLoading
	a.histDiffErr = ""

	a.diffSeq++
	seq := a.diffSeq
	dir, hash := a.repoPath, a.commits[i].Hash
	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var files []gitrepo.File
		if err == nil {
			files, err = repo.CommitDiff(context.Background(), hash)
		}
		a.dispatch(func() {
			if a.diffSeq != seq {
				return
			}
			// 等结果的时候用户可能已经切回改动 tab 了，这份 diff 是给
			// 历史 tab 的，不该顶掉工作区的文件列表 —— diffSeq 挡不住
			// 这一手，切 tab 不会动它。
			if a.tab != 1 {
				if err == nil {
					a.histDiff = loadReady
					a.histFiles = files
					a.histListID = "c:" + hash
				}
				return
			}
			if err != nil {
				a.histDiff = loadFailed
				a.histDiffErr = err.Error()
				return
			}
			a.histDiff = loadReady
			a.histFiles = files
			a.histListID = "c:" + hash
			a.showFiles(files, "c:"+hash)
		})
	})
}

// navCommit 在历史提交列表中按方向导航（dir > 0 往下/向旧提交，dir < 0 往上/向新提交）。
func (a *app) navCommit(dir int) {
	if len(a.commits) == 0 {
		return
	}
	var newIdx int
	if a.commitSel < 0 {
		newIdx = 0
	} else if a.commitSel >= len(a.commits) {
		newIdx = len(a.commits) - 1
	} else {
		newIdx = clampSel(a.commitSel+dir, len(a.commits))
	}
	if newIdx != a.commitSel {
		a.loadCommit(newIdx)
	}
	a.commitList.ScrollIntoView(newIdx)
	if newIdx >= len(a.commits)-15 && a.histHasMore && !a.histLoadingMore {
		a.loadMoreCommits()
	}
}

// selectStash 是点了一条储藏。
//
// 储藏不进提交列表：它不是任何一条分支历史的一部分，硬塞进去会打断拓扑图，
// 而且 Log 也拿不出「这个储藏的改动」—— 那是 stash show 的活。
func (a *app) selectStash(ref gitrepo.Ref) {
	a.commitSel = -1
	a.fileSel = -1
	a.stashRev = ref.Full
	a.stashName = ref.Name
	a.histDiff = loadLoading
	a.histDiffErr = ""
	a.tab = 1

	// 用户可能是在改动 tab 上直接点的储藏，提交列表还没读过。不补这一下，
	// 上半区会停在 loadIdle，显示「还没有提交」—— 仓库里明明有一堆。
	// 已经读过（loadReady/loadLoading）就别再读一遍，那会把列表闪一下。
	if a.hist == loadIdle {
		a.loadCommits()
	}

	a.diffSeq++
	seq := a.diffSeq
	dir, name := a.repoPath, ref.Name
	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var files []gitrepo.File
		if err == nil {
			files, err = repo.StashDiff(context.Background(), name)
		}
		a.dispatch(func() {
			if a.diffSeq != seq || a.tab != 1 {
				if a.diffSeq == seq && err == nil {
					a.histDiff = loadReady
					a.histFiles = files
					a.histListID = "k:" + ref.Full
				}
				return
			}
			if err != nil {
				a.histDiff = loadFailed
				a.histDiffErr = err.Error()
				return
			}
			a.histDiff = loadReady
			a.histFiles = files
			a.histListID = "k:" + ref.Full
			a.showFiles(files, "k:"+ref.Full)
		})
	})
}

// historyArea 是历史 tab 的主区：上半提交列表，下半「文件列表 + diff」，
// 中间一条可以拖的分隔条。
func (a *app) historyArea(c *ui.Context, pal palette) {
	a.clampHistSplit(c)

	ui.SplitVertical(c, &a.histSplit,
		func() { a.commitListPane(c, pal) },
		func() { a.detailPane(c, pal) },
	).Grow(1)
}

// clampHistSplit 把提交列表的高度收进当前窗口。
//
// 为什么不能只靠 ui.Split 自己夹：它在 set() 里用的是**上一帧实测的**
// 可用高度，而且只在拖动/按方向键那一刻生效 —— 第一帧（还没量过）夹不了，
// 窗口被拖矮也不会重新夹一次。结果就是上栏顶到窗口外面、下栏（diff）
// 被压成 0，看上去像 diff 没了。
//
// 下限用框架自己那个 minPane（t.Space(10)，主题默认 Spacing=4 时是 40），
// 不另立一个更紧的数：比它大就会和框架的夹逼打架 —— 用户拖到底，框架先
// 停在 40，下一帧又被这里拉回去，闪一下。
func (a *app) clampHistSplit(c *ui.Context) {
	_, h := c.Size()
	avail := h - a.toolbarHeight(c)
	if avail <= 0 {
		return
	}
	minPane := c.Theme().Space(10)
	hi := max(minPane, avail-1-minPane)

	if a.histSplit <= 0 { // 还没定过：给 diff 留大头
		a.histSplit = clamp(avail*defaultCommitH, minPane, hi)
		return
	}
	a.histSplit = clamp(a.histSplit, minPane, hi)
}

// commitListPane 是提交列表那一栏：栏头 + 列头 + 列表。
func (a *app) commitListPane(c *ui.Context, pal palette) {
	t := c.Theme()

	ui.Column(c).Grow(1).FillHeight().Background(pal.surface1).Children(func() {
		// 栏头。提交条数原来挂在侧栏底部，侧栏让给分支树之后搬到这里
		// —— 它说的就是这份列表，放在列表头上比放在屏幕另一头合适。
		ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			n := T("提交", "Commits")
			if a.hist == loadReady {
				if a.histHasMore {
					n = Tf("已加载 %d 个提交…", "Loaded %d commits…", len(a.commits))
				} else {
					n = Tf("%d 个提交", "%d commits", len(a.commits))
				}
			}
			ui.Text(c, n).FontSize(11).Font("monospace").FontFeatures("tnum").
				TextColor(t.TextMuted)
			if a.histRev != "" {
				// 点了分支之后得有个说法，不然用户不知道自己在看哪条历史
				ui.Text(c, a.histRev).FontSize(11).Bold().SingleLine().
					TextColor(pal.ref)
			}
			ui.Spacer(c)
			if a.histDiff == loadReady && len(a.files) > 0 {
				ui.Textf(c, "+%d −%d", a.totalAdd(), a.totalDel()).
					FontSize(11).Font("monospace").FontFeatures("tnum").
					TextColor(pal.ref)
			}
		})

		// 这里只给个说法，重试按钮留在大区那边 —— 同一个动作摆两个
		// 按钮，屏幕上看是重复的。
		switch a.hist {
		case loadLoading:
			if len(a.commits) == 0 {
				emptyNote(c, t, T("正在读提交…", "Loading commits…"))
				return
			}
		case loadFailed:
			if len(a.commits) == 0 {
				emptyNote(c, t, T("读不到历史", "Unable to load history"))
				return
			}
		}
		if len(a.commits) == 0 {
			emptyNote(c, t, T("还没有提交", "No commits yet"))
			return
		}

		a.commitHeaderRow(c, t, pal)

		totalRows := len(a.commits)
		if a.histHasMore || a.histLoadingMore {
			totalRows++
		}
		a.commitList.Key = func(i int) any {
			if i < len(a.commits) {
				return "c:" + a.commits[i].Hash
			}
			return "load_more_commits"
		}
		ui.List(c, &a.commitList, totalRows, func(i int) {
			if i < len(a.commits) {
				a.commitRow(c, pal, i)
				// 预加载：当滚动接近列表末尾（倒数 20 行以内）时，自动触发加载更早提交
				if i >= len(a.commits)-20 && a.histHasMore && !a.histLoadingMore {
					a.loadMoreCommits()
				}
			} else {
				a.loadMoreRow(c, t, pal)
			}
		}).Grow(1)
	})
}

// loadMoreRow 是提交列表底部的加载行，支持点击或自动滚动加载更早的提交。
func (a *app) loadMoreRow(c *ui.Context, t *ui.Theme, pal palette) {
	row := ui.Row(c).Height(commitH).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).
		Cursor(ui.CursorPointer).Background(pal.fileBg)
	if row.Hovered() {
		row.Background(pal.surface2)
	}
	fadeColors(row)
	row.Children(func() {
		if a.histLoadingMore {
			ui.Text(c, T("正在加载更早的提交…", "Loading earlier commits…")).
				FontSize(11).TextColor(t.TextMuted)
		} else {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				drawIcon(c, iconChevronDown, 12, pal.inkSubtle)
				ui.Text(c, Tf("加载更早的提交 (已显示 %d 个)", "Load earlier commits (%d shown)", len(a.commits))).
					FontSize(11).TextColor(pal.inkSubtle)
			})
		}
	})
	if row.Clicked() && !a.histLoadingMore && a.histHasMore {
		a.loadMoreCommits()
	}
	if a.histHasMore && !a.histLoadingMore {
		a.loadMoreCommits()
	}
}

// commitH 是提交列表一行的高度。
//
// 写死是必须的：ui.List 是虚拟化的，它按「已经量到的高度」估算没构建的那
// 些行，行高忽大忽小会让首帧的行摞在一起。拓扑图那边也靠它是整数（引擎按
// 「路径相对自身包围盒的形状」缓存蒙版，行高和泳道 x 都是整数时，每一行的
// 线形状完全一样，栅格化只做一次）。
const commitH = 30

// 提交行的三列固定宽度。列头（commitHeaderRow）和数据行（commitRow）共用，
// 两处各写一遍的话改一个忘一个就对不齐了。
const (
	commitAuthorW = 110
	commitWhenW   = 76
	commitHashW   = 64
)

// commitHeaderRow 是提交列表的列头，像 Fork 那样把每列的名字摆在列上。
//
// 结构必须和 commitRow 一模一样（图栏 + 带内边距的 Row），否则列对不齐 ——
// 两处的宽度都取自上面那组常量。
func (a *app) commitHeaderRow(c *ui.Context, t *ui.Theme, pal palette) {
	ui.Row(c).Height(24).Shrink(0).AlignItems(ui.Center).
		Background(pal.fileBg).Children(func() {
		ui.Box(c).Width(graphWidth(a.maxLanes)).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, T("描述", "Description")).FontSize(11).Grow(1).MinWidth(0).TextColor(t.TextMuted)
			ui.Text(c, T("作者", "Author")).FontSize(11).Width(commitAuthorW).Shrink(0).TextColor(t.TextMuted)
			ui.Text(c, T("时间", "Date")).FontSize(11).Width(commitWhenW).Shrink(0).
				TextAlign(ui.End).TextColor(t.TextMuted)
			ui.Text(c, T("哈希", "Commit")).FontSize(11).Width(commitHashW).Shrink(0).TextColor(t.TextMuted)
		})
	})
}

// commitRow 是提交列表的一行：图 + 描述（主题和 refs 徽章）+ 作者 + 时间 + 哈希。
func (a *app) commitRow(c *ui.Context, pal palette, i int) {
	t := c.Theme()
	cm := a.commits[i]
	sel := i == a.commitSel

	bg := ui.Transparent
	if sel {
		bg = pal.selection
	}

	row := ui.Row(c).Height(commitH).Shrink(0).AlignItems(ui.Center).
		Background(bg).Cursor(ui.CursorPointer)
	if row.Hovered() && !sel {
		row.Background(pal.surface2)
	}
	fadeColors(row)
	row.Children(func() {
		// 图形栏：本行的线画在这块的 Draw 里，r 就是它这一行的矩形。
		// 宽度按整张图的最大泳道数定，所以每行的图对齐，文字也齐。
		if i < len(a.graph) {
			ui.Box(c).Width(graphWidth(a.maxLanes)).Height(commitH).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) {
					drawGraphRow(p, r, pal, a.graph[i], a.maxLanes)
				})
		}

		ui.Row(c).Grow(1).MinWidth(0).Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			// 主题和徽章挤在同一行：主题可以压到很小（MinWidth(0) 是关键，
			// 中文主题没有空格可断，不给它就会被自己撑住、把徽章挤出去），
			// 徽章不压，宁可截主题也不能让「这是哪条分支」看不全。
			ui.Text(c, cm.Subject).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
			badges := parseRefBadges(cm.Refs, a.remotes)
			visible := badges
			overflow := 0
			if len(badges) > maxVisibleBadges {
				visible = badges[:maxVisibleBadges-1]
				overflow = len(badges) - (maxVisibleBadges - 1)
			}
			for _, b := range visible {
				// 四枚芯片共用同一副面孔，靠圆点分本地 / 远程 / 标签。
				// 逐个染色的版本见 git 历史：一行里蓝、绿、灰三色并排，
				// 扫过去只看见色块，认不出分支名。
				specChip(c, pal, b.Label, badgeDot(pal, b.Kind), rowChipH, rowChipText)
			}
			if overflow > 0 {
				ui.Text(c, fmt.Sprintf("+%d", overflow)).FontSize(rowChipText).
					FontWeight(weightMedium).SingleLine().Shrink(0).
					Padding(1, 5).Radius(radiusXS).
					TextColor(pal.inkSubtle).Background(pal.surface2).
					Border(1, pal.hairline)
			}
			ui.Spacer(c)

			ui.Text(c, cm.Author).FontSize(11).SingleLine().
				Width(commitAuthorW).Shrink(0).TextColor(t.TextMuted)
			ui.Text(c, shortWhen(cm.When, time.Now())).FontSize(11).
				Width(commitWhenW).Shrink(0).TextAlign(ui.End).TextColor(t.TextMuted)
			ui.Text(c, cm.Short).Font("monospace").FontSize(10).
				Width(commitHashW).Shrink(0).TextColor(t.TextMuted)
		})
	})
	// 右键菜单挂在整行上（含图形栏那一块）：一行 30px 高，能按的面积越大
	// 越好。挂在行上之后行内不再有别的菜单，右键一定落在这里。
	row.ContextMenu(func(m *ui.Menu) { a.commitMenu(m, cm) })

	if row.Clicked() {
		a.loadCommit(i)
	}
}

// commitMenu 在 commitops.go（提交行右键点开之后的那些事）。

// maxVisibleBadges 是一行提交最多渲染的徽章数，超过的折叠成「+N」。
const maxVisibleBadges = 4

// refBadgeKind 是提交行徽章的类型：本地分支、远程跟踪分支、标签。
type refBadgeKind int

const (
	refBadgeHead   refBadgeKind = iota // HEAD（当前检出的那条），如 "HEAD → beta"、"HEAD"
	refBadgeLocal                      // 其他本地分支，如 "beta"
	refBadgeRemote                     // 远程跟踪分支，如 "origin/beta"
	refBadgeTag                        // 标签，如 "tag: v1.0"
)

type refBadge struct {
	Label string
	Kind  refBadgeKind
}

// badgeDot 是芯片上那颗圆点的颜色（§2.5 的小标签亮色）。
//
// 四种引用各一支色，而且只落在那颗 6px 的点上：HEAD 跟主干、跟侧栏「我在哪」
// 那颗圆点共用 Ink —— 它是全屏少数几处实心强调之一，不能变成又一颗彩点；
// 本地分支、远程、标签则各拿一支，颜色都借自拓扑图那套色板，
// 于是「这条线是哪条分支」和「这枚芯片是哪类引用」是同一套颜色语言。
// 这里原来本地分支与 HEAD 同色（都是 Ink），一列芯片上全是白点，谁也认不出谁。
func badgeDot(pal palette, kind refBadgeKind) ui.Color {
	switch kind {
	case refBadgeHead:
		return pal.laneColor(0)
	case refBadgeRemote:
		return pal.accentTeal
	case refBadgeTag:
		return pal.accentAmber
	default: // refBadgeLocal
		return pal.accentBlue
	}
}

func refPriority(b refBadge) int {
	switch b.Kind {
	case refBadgeHead:
		return 0 // HEAD 优先级最高
	case refBadgeLocal:
		return 1 // 其他本地分支
	case refBadgeRemote:
		return 2 // 远程跟踪分支
	case refBadgeTag:
		return 3 // 标签
	default:
		return 4
	}
}

func isRemoteRef(name string, remotes []string) bool {
	for _, r := range remotes {
		if r != "" && strings.HasPrefix(name, r+"/") {
			return true
		}
	}
	return strings.HasPrefix(name, "origin/") || strings.HasPrefix(name, "upstream/")
}

// parseRefBadges 把 git log 的 %D 输出拆分成结构化的徽章列表，
// 过滤掉无用的符号引用（如 origin/HEAD），并按本地分支、远程分支、标签排序。
func parseRefBadges(refs string, remotes []string) []refBadge {
	if strings.TrimSpace(refs) == "" {
		return nil
	}
	var badges []refBadge
	for _, part := range strings.Split(refs, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		// 过滤远程符号引用：origin/HEAD 或 origin/HEAD -> origin/main 只代表远程默认分支别名
		if strings.HasSuffix(p, "/HEAD") || strings.Contains(p, "/HEAD ->") {
			continue
		}

		var b refBadge
		switch {
		case strings.HasPrefix(p, "HEAD -> "):
			branch := strings.TrimPrefix(p, "HEAD -> ")
			b = refBadge{Label: "HEAD → " + branch, Kind: refBadgeHead}
		case p == "HEAD":
			b = refBadge{Label: "HEAD", Kind: refBadgeHead}
		case strings.HasPrefix(p, "tag: "):
			b = refBadge{Label: p, Kind: refBadgeTag}
		case isRemoteRef(p, remotes):
			b = refBadge{Label: p, Kind: refBadgeRemote}
		default:
			b = refBadge{Label: p, Kind: refBadgeLocal}
		}
		badges = append(badges, b)
	}

	sort.SliceStable(badges, func(i, j int) bool {
		return refPriority(badges[i]) < refPriority(badges[j])
	})
	return badges
}

// refLabel 把 git 的 ref 列表缩短一点。「HEAD -> main, origin/main, tag: v1」
// 全画出来会把一行的宽度吃光，只留最关键的那个。
//
// 箭头换成「→」：git 管道输出里是 ASCII 的「->」，摆在界面上和四周的排版
// 不是一个味道 —— 这是个字，不是图标，用真正的箭头字符没有字形风险。
func refLabel(refs string) string {
	for _, part := range strings.Split(refs, ",") {
		if p := strings.TrimSpace(part); p != "" {
			return strings.Replace(p, " -> ", " → ", 1)
		}
	}
	return ""
}

// shortWhen 给出「3 小时前」这类相对时间。绝对时间在 review 时不好用：
// 想找的是「今天早上那个提交」，不是「14:23」。
func shortWhen(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return T("刚刚", "just now")
	case d < time.Hour:
		return Tf("%d 分钟前", "%d mins ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return Tf("%d 小时前", "%d hours ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return Tf("%d 天前", "%d days ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return Tf("%d 个月前", "%d months ago", int(d.Hours()/(24*30)))
	}
	return Tf("%d 年前", "%d years ago", int(d.Hours()/(24*365)))
}
