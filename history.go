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
	all := a.histAll
	a.histHasMore = true
	a.histLoadingMore = false

	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var commits []gitrepo.Commit
		if err == nil {
			if all {
				commits, err = repo.LogAll(context.Background(), logBatchSize, 0)
			} else {
				commits, err = repo.Log(context.Background(), rev, logBatchSize, 0)
			}
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
			// 提交的，用户点的是储藏，屏幕上是别的东西。选中按**筛完之后**
			// 那一份算（a.commits）：筛选词把最近那个提交藏起来时，该选中的
			// 是屏幕上第一个，不是被藏起来的那个。
			if a.stashRev != "" || len(a.commits) == 0 {
				return
			}
			if a.commitSel < 0 {
				a.loadCommit(0)
			} else if a.commitSel == 0 && a.commits[0].Hash != oldHead {
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
	all := a.histAll
	// offset 按**没筛过**的那份算：筛选只是把已经读回来的提交藏起来，
	// 下一页该从哪儿接着读和筛选词无关。按筛过的 len(a.commits) 算的话，
	// 筛掉一半就会把同一批提交读第二遍。
	offset := len(a.allCommits)

	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var more []gitrepo.Commit
		if err == nil {
			if all {
				more, err = repo.LogAll(context.Background(), logBatchSize, offset)
			} else {
				more, err = repo.Log(context.Background(), rev, logBatchSize, offset)
			}
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
			a.setCommits(append(a.allCommits, more...))
		})
	})
}

// setCommits 换掉提交列表，顺便把拓扑图的布局算出来。
//
// 两者必须一起换：图的第 i 行就是列表的第 i 行，分开赋值迟早会对不上。
// 进来的这一份是**没筛过**的全部提交，筛完之后显示的那份由 applyCommitFilter
// 算 —— 筛选词改一下只重算视图，不用重新读盘。
func (a *app) setCommits(commits []gitrepo.Commit) {
	a.allCommits = commits
	a.applyCommitFilter()
}

// applyCommitFilter 按 commitFilter 算要显示的提交，并重建拓扑图。
//
// 匹配大小写不敏感，范围是提交信息、完整/短哈希、作者和指向它的引用
// （分支、标签）。引用一起进来是因为「按分支名找提交」是最常见的找法 ——
// 提交信息里往往不写分支名。
func (a *app) applyCommitFilter() {
	q := strings.ToLower(strings.TrimSpace(a.commitFilter))
	if q == "" {
		a.commits = a.allCommits
	} else {
		kept := make([]gitrepo.Commit, 0, len(a.allCommits))
		for _, cm := range a.allCommits {
			if commitMatches(cm, q) {
				kept = append(kept, cm)
			}
		}
		a.commits = kept
	}
	a.commitFilterBuilt = a.commitFilter
	a.graph, a.maxLanes = buildGraph(a.commits)
	// 选中可能已经被筛掉了：下标越界时清掉，别让下半区显示别处的 diff
	// （和 selectRef 清 commitSel 是同一个道理）。
	if a.commitSel >= len(a.commits) {
		a.commitSel = -1
		a.histFiles = nil
		a.histListID = ""
		a.stashRev = ""
		a.histDiff = loadIdle
	}
}

// commitMatches 看一个提交是否匹配已经转成小写的筛选词。
func commitMatches(cm gitrepo.Commit, q string) bool {
	hay := strings.ToLower(cm.Subject + " " + cm.Hash + " " + cm.Short + " " + cm.Author + " " + cm.Refs)
	return strings.Contains(hay, q)
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
	a.enterHistory()

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
//
// 分隔条自己画，不用 ui.SplitVertical：框架那条线取主题的 t.Border，而
// Gloss 把 t.Border 设成了透明（框架自绘的控件一律不描边）—— 能拖的缝在
// 屏幕上一根线都没有，用户只能猜。线用发丝线（§2.2），悬停提亮一档，
// 和文件栏那两条竖分隔条（paneSplitter）同一副面孔。
func (a *app) historyArea(c *ui.Context, pal palette) {
	minPane, hi := a.clampHistSplit(c)
	grip := float32(6)

	ui.Column(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Box(c).FillWidth().Shrink(0).Height(a.histSplit).MinHeight(0).Clip().Children(func() {
			a.commitListPane(c, pal)
		})

		// 1px 的槽：线本身由下面的把手画，这里只占位、把两栏分开。
		ui.Box(c).FillWidth().Height(paneSplitterW).Shrink(0)

		ui.Box(c).Grow(1).Shrink(1).FillWidth().MinHeight(0).Clip().Children(func() {
			a.detailPane(c, pal)
		})

		// 把手最后建：它盖在上下两栏的交界上，命中区比那 1px 各探出 3px
		// —— 一条发丝线是世界上最难点中的东西。放最后才压得住两栏里的内容，
		// 指针落在缝上时不会先被列表或 diff 接走。
		handle := ui.Box(c).Absolute().Left(0).Right(0).
			Top(a.histSplit - (grip-1)/2).Height(grip).
			Cursor(ui.CursorResizeNS).Role(ui.RoleNone)
		if handle.DoubleClicked() {
			a.histSplit = 0 // 下一帧由 clampHistSplit 重新给默认值
		}
		if _, dy, held := handle.Dragged(); held && dy != 0 {
			a.histSplit = clamp(a.histSplit+dy, minPane, hi)
		}
		handle.Draw(func(p *ui.Painter, r ui.Rect) {
			// 悬停 / 按下只把发丝线提亮一档，不用强调蓝：它是两栏的边界，
			// 不是可点击的主 CTA（§5 的强调预算），和 paneSplitter 一致。
			line := pal.hairline
			if handle.Hovered() || handle.Pressed() {
				line = pal.hairlineStrong
			}
			p.Fill(ui.Rect{X: r.X, Y: r.Y + (grip-1)/2, W: r.W, H: 1}, line, 0)
		})
	})
}

// clampHistSplit 把提交列表的高度收进当前窗口，并给出能拖的范围
// （下限、上限）。上限给下半区至少留一个 minPane 和那条 1px 的线。
//
// 这个夹逼不能只靠分隔条自己：拖动时它拿到的位移是相对的，窗口被拖矮
// 时也没有人来把 histSplit 拉回来 —— 不在这里夹一次，上栏会顶到窗口
// 外面、下栏（diff）被压成 0，看上去像 diff 没了。
//
// 下限用框架自己那个 minPane（t.Space(10)，主题默认 Spacing=4 时是 40），
// 不另立一个更紧的数：和框架其它分隔条的下限保持同一处，用户拖到底时
// 停在他熟悉的位置。
func (a *app) clampHistSplit(c *ui.Context) (minPane, hi float32) {
	minPane = c.Theme().Space(10)
	_, h := c.Size()
	avail := h - a.toolbarHeight(c)
	if avail <= 0 {
		if a.histSplit <= 0 {
			a.histSplit = minPane
		}
		return minPane, minPane
	}
	hi = max(minPane, avail-1-minPane)

	if a.histSplit <= 0 { // 还没定过：给 diff 留大头
		a.histSplit = clamp(avail*defaultCommitH, minPane, hi)
	} else {
		a.histSplit = clamp(a.histSplit, minPane, hi)
	}
	return minPane, hi
}

// commitListPane 是提交列表那一栏：栏头 + 列头 + 列表。
func (a *app) commitListPane(c *ui.Context, pal palette) {
	t := c.Theme()

	// 不铺底：和下面的文件列表同一个卡片、同一层亚克力（见 fileListPane
	// 的注释）。
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		// 栏头。提交条数原来挂在侧栏底部，侧栏让给分支树之后搬到这里
		// —— 它说的就是这份列表，放在列表头上比放在屏幕另一头合适。
		ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			n := T("提交", "Commits")
			if a.hist == loadReady {
				if a.commitFilter != "" {
					// 筛选的时候两个数都得说：屏幕上这几个、这一趟读回来
					// 一共几个。只说前者的话，「我这个分支明明有更多提交」
					// 和「筛选把提交藏起来了」分不清。
					n = Tf("筛选出 %d / %d 个提交", "Filtered %d of %d commits", len(a.commits), len(a.allCommits))
				} else if a.histHasMore {
					n = Tf("已加载 %d 个提交…", "Loaded %d commits…", len(a.commits))
				} else {
					n = Tf("%d 个提交", "%d commits", len(a.commits))
				}
			}
			ui.Text(c, n).FontSize(11).Font("monospace").FontFeatures("tnum").
				TextColor(t.TextMuted)
			if name := a.histName(); name != "" {
				// 点了分支（或「所有分支」）之后得有个说法，不然用户不知道
				// 自己在看哪条历史
				ui.Text(c, name).FontSize(11).Bold().SingleLine().
					TextColor(pal.ref)
			}
			ui.Spacer(c)
			if a.histDiff == loadReady && len(a.files) > 0 {
				// 增删各上一支色（addBar / delBar），和文件树行尾、栏脚
				// 「总计」同一个口径：一处用中性 ref 色，同样两个数字在
				// 屏幕三处三个脸色，读的人得分别记。
				ui.Textf(c, "+%d", a.totalAdd()).FontSize(11).Bold().
					Font("monospace").FontFeatures("tnum").TextColor(pal.addBar)
				ui.Textf(c, "−%d", a.totalDel()).FontSize(11).Bold().
					Font("monospace").FontFeatures("tnum").TextColor(pal.delBar)
			}
		})

		// 提交筛选框。和侧栏筛分支、主区筛文件是同一副样子：常驻、随手打
		// 字即筛。它筛的是**已经读回来**的那批（见 commitFilter 的注释），
		// 想翻更早的提交还有列表底部那一行「加载更多」。
		//
		// 筛完的那一份提交是用哪个词算的，记在 commitFilterBuilt 里：不一致
		// 就重算视图（和 refsFilter / fileTreeFilter 同一个套路）。不重新读盘，
		// 也不清选中 —— 选中的提交被筛掉时 applyCommitFilter 自己会收尾。
		if a.commitFilter != a.commitFilterBuilt {
			a.applyCommitFilter()
		}
		ui.Box(c).Key("commit-filter-wrap").Shrink(0).Padding(0, 12, 6, 12).Children(func() {
			in := searchField(c, pal, &a.commitFilter, T("筛选提交", "Filter commits"))
			// / 键、命令栏置位的一次性焦点请求（见 focusCommitFilter）。
			if a.focusCommitFilter {
				in.Focus()
				a.focusCommitFilter = false
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
			// 筛选出一个都没有、而这一趟还没读完时，别让用户对着空列表猜：
			// 接着往下读，找到为止（读完了就还是空的，栏头那个「筛选出
			// 0 / N」会说明范围）。
			if a.commitFilter != "" && a.histHasMore && !a.histLoadingMore {
				a.loadMoreCommits()
				emptyNote(c, t, T("正在更多提交里筛选…", "Searching earlier commits…"))
				return
			}
			if a.commitFilter != "" {
				emptyNote(c, t, Tf("没有匹配「%s」的提交", "No commits matching \"%s\"", a.commitFilter))
				return
			}
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
				// 预加载：滚动到接近列表末尾（倒数 20 行以内）时自动读更早的提交。
				// 筛选态不在这里预载 —— 那时下标数的是筛过的那份，列表一短
				// 就贴着阈值，会一路读到仓库尽头；筛选要往下找走上面那条
				// 「一个都没筛出来」的路。
				if a.commitFilter == "" && i >= len(a.commits)-20 && a.histHasMore && !a.histLoadingMore {
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
		Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(pal.rowHover)
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
	ui.Row(c).Height(24).Shrink(0).AlignItems(ui.Center).Children(func() {
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
		row.Background(pal.rowHover)
	}
	fadeColors(row)
	// 徽章上那一下双击也要拦住整行的单击：芯片是行里的子元素，指针事件一
	// 路冒泡到行，不拦的话双击检出的同时还会把选中切回这个提交（紧接着
	// 检出又把界面带去改动 tab，白读一份 diff）。
	chipDouble := false
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
				chip := specChip(c, pal, b.Label, badgeDot(pal, b.Kind), rowChipH, rowChipText)
				// 双击一枚分支芯片 = 检出那条分支（和侧栏里双击一行同一个
				// 动作）。提交行上一枚分支名是「这条线上有哪个分支」，看见
				// 了想切过去的人第一反应就是点它 —— 以前点了什么都不会发生。
				//
				// 单击不接：那一交由整行处理（选中这个提交），和侧栏
				// 「单击看历史、双击才检出」是同一个分工。
				if ref, ok := badgeRef(b, a.refByFull); ok {
					chip.Tooltip(Tf("双击检出 %s", "Double-click to check out %s", ref.Name)).
						Cursor(ui.CursorPointer)
					if chip.DoubleClicked() {
						chipDouble = true
						a.checkoutRef(ref)
					}
				}
			}
			if overflow > 0 {
				ui.Text(c, fmt.Sprintf("+%d", overflow)).FontSize(rowChipText).
					FontWeight(weightMedium).SingleLine().Shrink(0).
					Padding(1, 5).Radius(radiusXS).
					TextColor(pal.inkSubtle).Background(pal.chip).
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

	if chipDouble {
		return
	}
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
// 四种引用各一支色，而且只落在那颗 6px 的点上：HEAD 跟「我在哪」共用 Ink
// —— 它是全屏少数几处实心强调之一，不能变成又一颗彩点；本地分支、远程、
// 标签则各拿一支，颜色都借自拓扑图那套色板，
// 于是「这条线是哪条分支」和「这枚芯片是哪类引用」是同一套颜色语言。
// 这里原来本地分支与 HEAD 同色（都是 Ink），一列芯片上全是白点，谁也认不出谁。
//
// HEAD **不跟主干走**：主干占走钢青蓝之后，它和本地分支那支 accentBlue 是同一
// 支 hex，跟着走就等于 HEAD 芯片和本地分支芯片共用一颗点 —— 一列芯片上两种
// 引用分不开，正是上面那句要防的事。「我在哪」因此保持 Ink，图里那条蓝线是
// 「这是主干」，两件事各说各话。
func badgeDot(pal palette, kind refBadgeKind) ui.Color {
	switch kind {
	case refBadgeHead:
		return pal.ink
	case refBadgeRemote:
		return pal.accentTeal
	case refBadgeTag:
		return pal.accentAmber
	default: // refBadgeLocal
		return pal.accentBlue
	}
}

// badgeRef 把提交行上的一枚徽章对回侧栏里那条引用。
//
// 徽章上写的是给人看的简称（beta、origin/beta、tag: v1.0），对回引用要的是
// 全名。对不上就返回 false：远程分支还没读进来、或者引用刚被删掉，那一枚
// 芯片点不出东西 —— 比点出一个错的强。HEAD 那枚也不对（它指的是「现在
// 就在这儿」，没有可检出的目标）。
func badgeRef(b refBadge, byFull map[string]*gitrepo.Ref) (gitrepo.Ref, bool) {
	var full string
	switch b.Kind {
	case refBadgeLocal:
		full = "refs/heads/" + b.Label
	case refBadgeRemote:
		full = "refs/remotes/" + b.Label
	case refBadgeTag:
		full = "refs/tags/" + strings.TrimPrefix(b.Label, "tag: ")
	default:
		return gitrepo.Ref{}, false
	}
	ref, ok := byFull[full]
	if !ok || ref == nil {
		return gitrepo.Ref{}, false
	}
	return *ref, true
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
