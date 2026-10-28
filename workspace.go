package main

import (
	"sort"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/egoist/mygo/ui"
)

// 编辑器视图左边那一栏列的是**整个工作区**，不是改动。
//
// 「编辑器」这个视图要的是「打开仓库里某个文件看一眼、改两笔」，所以进来看
// 到的应该是仓库里有哪些代码；只有改动过的那些，是「改动」视图的事。两份
// 列表因此各记一份状态：筛选词、选中下标都分开，切视图来回不会互相踩 ——
// 在改动栏打过的筛选词不该跟到编辑器栏来。

// workspaceFiles 把「工作区里有哪些文件」（git ls-files 那份）和「哪些文件
// 改了」（状态那份）合成编辑器栏要的列表。
//
// 顺序按路径排：建树走的是插入顺序（filetree.BuildAt），输入有序，树里同层
// 才有序。改动过的文件直接沿用状态里那一份 —— 增删行数、状态字母、暂存区
// 信息都在里头，所以编辑器栏里也认得哪些文件改了、也能暂存；没改的只带一
// 个路径，状态字母为空，行尾不画增删行数。
//
// 同一个路径在状态里可能出现两次（暂存之后又改了）：那一栏一个文件只该占
// 一行，取能反映工作区内容的那一份 —— 也就是没暂存的那次。
func workspaceFiles(m *gitrepo.Model) []gitrepo.File {
	if m == nil {
		return nil
	}
	changed := make(map[string]gitrepo.File, len(m.Files))
	for _, f := range m.Files {
		p := pathOf(f)
		if prev, ok := changed[p]; ok && prev.Unstaged {
			continue // 手上这份已经更能反映工作区了，别用暂存那份盖掉
		}
		changed[p] = f
	}

	files := make([]gitrepo.File, 0, len(m.Worktree))
	seen := make(map[string]bool, len(m.Worktree))
	for _, w := range m.Worktree {
		seen[w.Path] = true
		if f, ok := changed[w.Path]; ok {
			files = append(files, f)
			continue
		}
		files = append(files, gitrepo.File{FileDiff: diff.FileDiff{NewPath: w.Path}})
	}
	// 状态里有、ls-files 里没有的：索引里已经不存在的删除（工作区里的文件
	// 不再被跟踪）。它们仍然是「改动」，得留在列表里，否则删掉的文件在编辑器
	// 栏里就找不着了。
	for _, f := range m.Files {
		if p := pathOf(f); !seen[p] {
			seen[p] = true
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool { return pathOf(files[i]) < pathOf(files[j]) })
	return files
}

// setWorktree 换掉工作区列表 —— 每次读到仓库都会来一遍。
//
// 筛选词留着（刷新不该清掉用户敲的筛选），选中的文件按路径找回来：列表重建
// 之后下标全变了，记着下标只会指到别的文件上去。
func (a *app) setWorktree(m *gitrepo.Model) {
	key := a.selectedWorkspaceKey()
	a.wsAll = workspaceFiles(m)
	a.applyWsFilter()
	a.selectWorkspaceKey(key)
	// 搜索页签开着且敲了词的话，跟着新的一份文件列表重跑一遍：文件可能
	// 增删了，结果不该停在上一份快照上。startWsSearch 会记下新的 root，
	// 换了仓库旧结果同样作废。
	if a.wsTab == wsTabSearch && strings.TrimSpace(a.wsSearchQuery) != "" {
		a.startWsSearch(strings.TrimSpace(a.wsSearchQuery))
	}
}

// applyWsFilter 按当前的筛选词重算显示的工作区文件与文件树。
func (a *app) applyWsFilter() {
	key := a.selectedWorkspaceKey()
	a.wsFiles = filterFiles(a.wsAll, a.wsFilter)
	a.wsTreeFilter = a.wsFilter
	a.wsTree = filetree.Filter(filetree.Build(a.wsFiles), a.wsFilter)
	a.selectWorkspaceKey(key)
}

// selectedWorkspaceKey 是编辑器栏里选中的那个文件的路径，没选中时为空。
func (a *app) selectedWorkspaceKey() string {
	if a.wsSel < 0 || a.wsSel >= len(a.wsFiles) {
		return ""
	}
	return pathOf(a.wsFiles[a.wsSel])
}

// selectWorkspaceKey 按路径选中文件；路径不在当前列表里就变成没选中。
func (a *app) selectWorkspaceKey(key string) {
	a.wsSel = -1
	if key == "" {
		return
	}
	for i := range a.wsFiles {
		if pathOf(a.wsFiles[i]) == key {
			a.wsSel = i
			return
		}
	}
}

// workspaceFile 是编辑器该显示的那个文件。没选中时返回 false，编辑器显示
// 空态。
func (a *app) workspaceFile() (gitrepo.File, bool) {
	if a.wsSel < 0 || a.wsSel >= len(a.wsFiles) {
		return gitrepo.File{}, false
	}
	return a.wsFiles[a.wsSel], true
}

// changedWorkspaceCount 是工作区里改过的文件数（那一栏脚上的「已改动」）。
func (a *app) changedWorkspaceCount() int {
	n := 0
	for _, f := range a.wsAll {
		if f.Status != "" {
			n++
		}
	}
	return n
}

// workspaceFilePane 是编辑器视图左边那一栏。
//
// 栏头两颗页签图标（VS Code 侧栏同款）：文件树管「有哪些文件」，搜索管
// 「内容里有什么」，各自换掉下面那一块。
func (a *app) workspaceFilePane(c *ui.Context, pal palette, t *ui.Theme) {
	ui.Column(c).Width(a.filePaneWidth(c)).Shrink(0).FillHeight().
		Background(pal.surface1).Children(func() {
		ui.Row(c).Shrink(0).Padding(5, 8, 5, 12).Gap(4).AlignItems(ui.Center).Children(func() {
			if a.wsPaneTabButton(c, pal, T("文件树", "Files"),
				T("浏览工作区的文件", "Browse the workspace files"),
				iconFolder, a.wsTab == wsTabTree) {
				a.wsTab = wsTabTree
			}
			if a.wsPaneTabButton(c, pal, T("搜索", "Search"),
				T("搜索工作区文件的内容", "Search file contents across the workspace"),
				iconSearch, a.wsTab == wsTabSearch) {
				a.wsTab = wsTabSearch
				a.focusWsSearch = true
			}
			ui.Spacer(c)
			if a.wsTab == wsTabTree {
				label := T("文件", "Files")
				if a.state == loadReady || a.state == loadFailed {
					label = Tf("%d 个文件", "%d files", len(a.wsFiles))
				}
				ui.Text(c, label).FontSize(textCaption).TextColor(pal.inkSubtle)
			}
		})
		if a.wsTab == wsTabSearch {
			a.workspaceSearchView(c, pal, t)
		} else {
			ui.Box(c).Key("workspace-file-filter-wrap").Shrink(0).Padding(0, 12, 8, 12).Children(func() {
				a.workspaceFilterInput(c, pal)
			})
			ui.Box(c).Key("workspace-file-tree-wrap").Grow(1).FillWidth().Children(func() {
				a.workspaceTreeView(c, pal, t)
			})
		}
		a.workspaceFooter(c, pal)
	})
}

// workspaceFilterInput 是编辑器栏的筛选框。
//
// Label 必须和另外两个筛选框（「筛选文件」「筛选分支」）分得开：筛的是不同的
// 东西，名字一样的话测试与无障碍工具都认不出找的是哪一个。
func (a *app) workspaceFilterInput(c *ui.Context, pal palette) {
	in := searchField(c, pal, &a.wsFilter, T("筛选工作区文件", "Filter workspace files"))
	if a.focusFilter {
		in.Focus()
		a.focusFilter = false
	}
}

// wsCollapsed 供 filetree.FlattenFunc 问「工作区树的这个目录收起了吗」。
// 与改动栏相反：编辑器栏默认全收起，只有用户点开过的目录才展开。
func (a *app) wsCollapsed(key string) bool {
	return !a.dirOpen(treeWorkspace, key)
}

// workspaceTreeView 画编辑器栏的文件树。筛选词变了就重算显示的文件 ——
// 与改动栏同一套做法：直接比对值，不依赖控件的 Changed()。
func (a *app) workspaceTreeView(c *ui.Context, pal palette, t *ui.Theme) {
	if a.wsFilter != a.wsTreeFilter {
		a.applyWsFilter()
	}
	// 筛选态一律展开：搜出来的文件藏在收起的目录里，等于没搜到。
	// 平时按编辑器栏自己的折叠记录来，默认全收起（见 wsCollapsed）。
	rows := filetree.FlattenFunc(a.wsTree, a.wsCollapsed, a.wsFilter != "")
	if len(rows) == 0 {
		msg := T("这个仓库里没有文件", "No files in this repository")
		switch {
		case a.state == loadIdle:
			msg = T("还没有打开仓库", "No repository opened")
		case a.state == loadFailed:
			msg = T("读取仓库失败", "Failed to read the repository")
		case a.wsFilter != "" && len(a.wsAll) > 0:
			msg = T("没有匹配的文件", "No matching files")
		}
		emptyNote(c, t, msg)
		return
	}
	a.wsList.Key = func(i int) any { return rows[i].Node.Key }
	ui.List(c, &a.wsList, len(rows), func(i int) {
		a.treeRowIn(c, pal, rows[i], rowSpec{
			list: a.wsFiles, sel: &a.wsSel, onClick: a.syncEditorPane,
			tree: treeWorkspace, plain: true,
		})
	}).Grow(1)
}

// navWorkspaceFile 用方向键在编辑器栏的工作区列表里移动选中。
//
// 与 navFile 同一套路：走扁平行、跳过目录行（FileIdx 为 -1），并让列表把新
// 选中滚进视野。列表是树形扁平出来的，直接对 wsFiles 加一减一会和眼睛看到
// 的顺序对不上。
func (a *app) navWorkspaceFile(dir int) {
	if len(a.wsFiles) == 0 {
		return
	}
	rows := filetree.FlattenFunc(a.wsTree, a.wsCollapsed, a.wsFilter != "")

	cur := -1
	if a.wsSel >= 0 {
		for i, r := range rows {
			if r.Node.FileIdx == a.wsSel {
				cur = i
				break
			}
		}
	}
	next := -1
	nextFrom := cur + dir
	if cur < 0 {
		if dir < 0 {
			nextFrom = len(rows) - 1
		} else {
			nextFrom = 0
		}
	}
	for i := nextFrom; i >= 0 && i < len(rows); i += dir {
		if rows[i].Node.FileIdx >= 0 {
			next = i
			break
		}
	}
	if next < 0 {
		return // 已经到边界了
	}

	idx := rows[next].Node.FileIdx
	if idx < 0 || idx >= len(a.wsFiles) {
		return
	}
	a.wsSel = idx
	a.syncEditorPane()
	a.wsList.ScrollIntoView(next)
}

// workspaceFooter 是编辑器栏底下那条栏脚。
//
// 它只报一件事：工作区里改过几个文件。整份改动的增删行数、已 review 的进度
// 是「改动」视图的问题，摆在浏览整个仓库的地方没有意义；但「有几个文件动过」
// 恰恰是从改动列表切过来时丢掉的那条信息，所以留在这儿。
func (a *app) workspaceFooter(c *ui.Context, pal palette) {
	n := a.changedWorkspaceCount()
	ui.Row(c).Shrink(0).Padding(7, 12).Gap(6).AlignItems(ui.Center).
		Background(pal.surface2).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
		if n == 0 {
			ui.Text(c, T("工作区无改动", "Working tree clean")).FontSize(textCaption).
				TextColor(pal.inkSubtle)
			return
		}
		ui.Text(c, T("已改动", "Changed")).FontSize(textCaption).TextColor(pal.inkSubtle)
		ui.Textf(c, T("%d 个文件", "%d files"), n).FontSize(11).
			FontFeatures("tnum").TextColor(pal.inkMuted)
	})
}
