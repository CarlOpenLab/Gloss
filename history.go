package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/gitrepo"
)

// 历史 tab：侧栏列提交，主区显示选中提交的 diff。
//
// 之所以把提交列表放侧栏、diff 放主区，是因为这样一来 diff 视图、语法高亮、
// 折叠未改动区间、✨ 解释全都能原样复用 —— 主区拿到的还是一个 []gitrepo.File。

// logLimit 一次读多少个提交。够翻很久了，真要看更早的再往下拉。
const logLimit = 200

// setTab 切换「改动 / 历史」。只改状态，真正的加载在后台做。
func (a *app) setTab(t int) {
	if t == a.tab {
		return
	}
	a.tab = t
	a.closeExplain()
	a.filter = ""
	a.treeFilter = ""

	switch t {
	case 0:
		if a.workFiles != nil {
			a.showFiles(a.workFiles)
		} else {
			a.allFiles = nil
			a.applyFilter()
		}
	case 1:
		a.allFiles = nil
		a.applyFilter()
		a.loadCommits()
		if a.commitSel >= 0 && a.commitSel < len(a.commits) {
			a.loadCommit(a.commitSel)
		}
	}
}

// loadCommits 在后台读提交列表。
func (a *app) loadCommits() {
	if a.repoPath == "" {
		return
	}
	a.hist = loadLoading
	a.histErr = ""

	a.logSeq++
	seq := a.logSeq
	dir := a.repoPath
	a.run(func() {
		repo, err := gitrepo.Open(context.Background(), dir)
		var commits []gitrepo.Commit
		if err == nil {
			commits, err = repo.Log(context.Background(), logLimit, 0)
		}
		a.dispatch(func() {
			if a.logSeq != seq {
				return
			}
			if err != nil {
				a.hist = loadFailed
				a.histErr = err.Error()
				return
			}
			a.setCommits(commits)
			a.hist = loadReady
			// 进来先选中最近那个提交，省得用户还要再点一下
			if a.commitSel < 0 && len(commits) > 0 {
				a.loadCommit(0)
			}
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
				return
			}
			if err != nil {
				a.histDiff = loadFailed
				a.histDiffErr = err.Error()
				return
			}
			a.histDiff = loadReady
			a.showFiles(files)
		})
	})
}

// historyArea 是历史 tab 的主区：左边一栏提交列表（带拓扑图），右边是选中
// 那个提交的 diff。
//
// 列表从侧栏搬到主区，是因为拓扑图要横向铺开 —— 侧栏 280 宽，铺四条泳道就
// 把提交信息挤没了。侧栏这时让给文件树。
func (a *app) historyArea(c *ui.Context, pal palette) {
	t := c.Theme()

	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
		a.commitListPane(c, pal)
		ui.Box(c).Width(1).Shrink(0).FillHeight().Background(pal.border)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if a.histDiff == loadReady && len(a.files) > 0 {
				a.diffView(c, pal)
				return
			}
			ui.Box(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
					a.historyEmpty(c, t)
				})
			})
		})
	})
}

// 提交列表那一栏的宽度。
//
// 定宽而不是按比例：diff 那边一行代码有多宽是大概知道的，跟着窗口按比例缩
// 的话，窗口一窄先挤坏的是 diff。开 split 时 diff 要并排摆两栏、各得一半，
// 所以列表收窄一点让给它（1280 的窗口下，split 关着时 diff 有 579，开着时
// 收窄到 949 里的一半 —— 都还看得下去）。
const (
	commitListW      = 420
	commitListWSplit = 330
)

func (a *app) commitListWidth() float32 {
	if a.split {
		return commitListWSplit
	}
	return commitListW
}

// commitListPane 是提交列表那一栏：栏头 + 列表。
func (a *app) commitListPane(c *ui.Context, pal palette) {
	t := c.Theme()

	ui.Column(c).Width(a.commitListWidth()).Shrink(0).FillHeight().
		Background(pal.sideBg.Alpha(0.35)).
		Children(func() {
			// 栏头。提交条数原来挂在侧栏底部，侧栏让给文件树之后搬到这里
			// —— 它说的就是这份列表，放在列表头上比放在屏幕另一头合适。
			ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				n := "提交"
				if a.hist == loadReady {
					n = strconv.Itoa(len(a.commits)) + " 个提交"
				}
				ui.Text(c, n).FontSize(11).Font("monospace").FontFeatures("tnum").
					TextColor(t.TextMuted)
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
				emptyNote(c, t, "正在读提交…")
				return
			case loadFailed:
				emptyNote(c, t, "读不到历史")
				return
			}
			if len(a.commits) == 0 {
				emptyNote(c, t, "还没有提交")
				return
			}

			a.commitList.Key = func(i int) any { return "c:" + a.commits[i].Hash }
			ui.List(c, &a.commitList, len(a.commits), func(i int) {
				a.commitRow(c, pal, i)
			}).Grow(1)
		})
}

// commitH 是提交列表一行的高度。
//
// 写死是必须的：ui.List 是虚拟化的，它按「已经量到的高度」估算没构建的那
// 些行，行高忽大忽小会让首帧的行摞在一起。所以这里固定高度，内容在框里
// 居中对齐 —— 有没有 ref 标签都不影响行高。拓扑图那边也靠它是整数。
const commitH = 56

// commitRow 是提交列表的一行：左边一段拓扑图，右边 ref 标签（可选）、
// subject、短哈希 + 作者 + 时间。
func (a *app) commitRow(c *ui.Context, pal palette, i int) {
	t := c.Theme()
	cm := a.commits[i]
	sel := i == a.commitSel

	bg := ui.Transparent
	if sel {
		bg = t.Accent.Alpha(0.18)
	}

	row := ui.Row(c).Height(commitH).Shrink(0).AlignItems(ui.Center).
		Background(bg).Cursor(ui.CursorPointer)
	if row.Hovered() && !sel {
		row.Background(t.Surface)
	}
	row.Children(func() {
		// 图形栏：本行的线画在这块的 Draw 里，r 就是它这一行的矩形。
		// 宽度按整张图的最大泳道数定，所以每行的图对齐，文字也齐。
		if i < len(a.graph) {
			ui.Box(c).Width(graphWidth(a.maxLanes)).Height(commitH).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) {
					drawGraphRow(p, r, pal, a.graph[i], a.maxLanes)
				})
		}

		ui.Column(c).Grow(1).MinWidth(0).Padding(0, 10).Gap(3).Children(func() {
			if cm.Refs != "" {
				ui.Text(c, refLabel(cm.Refs)).FontSize(10).Bold().SingleLine().
					TextColor(pal.ref)
			}
			// subject 不能 Grow：在 Column 里那是纵向拉伸，会把下面一行顶开
			ui.Text(c, cm.Subject).FontSize(12).SingleLine()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cm.Short).Font("monospace").FontSize(10).TextColor(t.TextMuted)
				ui.Text(c, cm.Author).FontSize(10).SingleLine().Grow(1).TextColor(t.TextMuted)
				ui.Text(c, shortWhen(cm.When, time.Now())).FontSize(10).TextColor(t.TextMuted)
			})
		})
	})
	if row.Clicked() {
		a.loadCommit(i)
	}
}

// refLabel 把 git 的 ref 列表缩短一点。「HEAD -> main, origin/main, tag: v1」
// 全画出来会把侧栏挤爆，只留最关键的那个。
func refLabel(refs string) string {
	for _, part := range strings.Split(refs, ",") {
		if p := strings.TrimSpace(part); p != "" {
			return p
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
		return "刚刚"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " 分钟前"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " 小时前"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + " 天前"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/(24*30))) + " 个月前"
	}
	return strconv.Itoa(int(d.Hours()/(24*365))) + " 年前"
}
