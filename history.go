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
			a.commits = commits
			a.hist = loadReady
			// 进来先选中最近那个提交，省得用户还要再点一下
			if a.commitSel < 0 && len(commits) > 0 {
				a.loadCommit(0)
			}
		})
	})
}

// loadCommit 把某个提交的 diff 读出来，作为主区显示的内容。
func (a *app) loadCommit(i int) {
	if i < 0 || i >= len(a.commits) {
		return
	}
	a.commitSel = i
	a.hist = loadLoading
	a.histErr = ""

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
			if err != nil {
				a.hist = loadFailed
				a.histErr = err.Error()
				return
			}
			a.hist = loadReady
			a.showFiles(files)
		})
	})
}

// commitSidebar 是历史 tab 的侧栏：提交列表。
func (a *app) commitSidebar(c *ui.Context, pal palette) {
	t := c.Theme()

	switch a.hist {
	case loadLoading:
		ui.Box(c).Grow(1).Padding(16).Children(func() {
			ui.Text(c, "正在读提交…").FontSize(12).TextColor(t.TextMuted)
		})
		return
	case loadFailed:
		ui.Box(c).Grow(1).Padding(16).Gap(6).Children(func() {
			ui.Text(c, "读不到历史").FontSize(12).Bold().TextColor(t.Danger)
			ui.Text(c, a.histErr).FontSize(11).TextColor(t.TextMuted)
		})
		return
	case loadReady:
		if len(a.commits) == 0 {
			ui.Box(c).Grow(1).Padding(16).Children(func() {
				ui.Text(c, "还没有提交").FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
	}

	a.commitList.Key = func(i int) any { return "c:" + a.commits[i].Hash }
	ui.List(c, &a.commitList, len(a.commits), func(i int) {
		a.commitRow(c, pal, i)
	}).Grow(1)
}

// commitH 是提交列表一行的高度。
//
// 写死是必须的：ui.List 是虚拟化的，它按「已经量到的高度」估算没构建的那
// 些行，行高忽大忽小会让首帧的行摞在一起。所以这里固定高度，内容在框里
// 居中对齐 —— 有没有 ref 标签都不影响行高。
const commitH = 56

// commitRow 是提交列表的一行：ref 标签（可选）、subject、短哈希 + 作者 + 时间。
func (a *app) commitRow(c *ui.Context, pal palette, i int) {
	t := c.Theme()
	cm := a.commits[i]
	sel := i == a.commitSel

	bg := ui.Transparent
	if sel {
		bg = t.Accent.Alpha(0.18)
	}

	box := ui.Column(c).Height(commitH).Shrink(0).Justify(ui.Center).
		Padding(0, 10).Gap(3).Background(bg).Cursor(ui.CursorPointer)
	if box.Hovered() && !sel {
		box.Background(t.Surface)
	}
	box.Children(func() {
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
	if box.Clicked() {
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
