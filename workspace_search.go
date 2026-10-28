package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// 编辑器栏的「搜索」页签：在工作区全部文件的内容里找一个词。
//
// 它和文件树是同一栏的两个页签 —— 树那颗管「有哪些文件」，搜索这颗管「内容
// 里有什么」，栏头上两颗图标切换（VS Code 的侧栏同款布局）。结果按文件分组，
// 点一条就在右边的编辑器里打开那一行。
//
// 搜索在后台扫（a.run），逐文件读、命中封顶就停：不求一次扫全仓库，求的是
// 「敲下词的下一拍就有结果」。上限都写在下面的常量里，到了就如实说截断了。

const (
	// 编辑器栏的两个页签。
	wsTabTree   = 0
	wsTabSearch = 1

	wsSearchMaxHits  = 200     // 命中条数封顶
	wsSearchMaxFiles = 50      // 有命中的文件数封顶
	wsSearchMaxScan  = 5000    // 最多扫多少个文件（路径按字母序，扫不完如实说）
	wsSearchMaxFile  = 1 << 20 // 每个文件最多读进来 1MB
	wsSearchCtx      = 32      // 命中行左右各留多少个字符做窗口
)

// wsSearchHit 是一条命中：哪个文件第几行，窗口文本里命中段的字节区间
// （高亮用）。区间都落在字符边界上（建窗口时对齐过），可以放心切片。
type wsSearchHit struct {
	Path string
	Line int // 1 起算
	Text string
	Off  int // 命中段在 Text 里的字节偏移
	Len  int // 命中段的字节长度
}

// wsSearchState 是搜索页签的活状态。root 记着结果属于哪个仓库：换了仓库，
// 这些结果就是别家的事，作废重跑。
type wsSearchState struct {
	root      string
	query     string // 已经跑起来（或跑完）的查询词，trim 过
	running   bool
	seq       int // 一次查询一个号，过期结果作废
	results   []wsSearchHit
	fileCount int
	truncated bool
}

// wsSearchRow 是结果列表的一行：文件组头，或者组下的一条命中。两种行一样高
// （22px）—— 列表是虚拟化的，行高不齐首帧会把行摞在一起（treeview 同款教训）。
type wsSearchRow struct {
	path  string
	head  bool
	count int // 组头用：该文件命中几处
	hit   *wsSearchHit
}

func (r wsSearchRow) key() any {
	if r.head {
		return "head/" + r.path
	}
	return fmt.Sprintf("hit/%s/%d/%d", r.hit.Path, r.hit.Line, r.hit.Off)
}

// wsPaneTabButton 是编辑器栏头上的页签图标（文件树 / 搜索两颗）。选中的那颗
// 带选中底色，和树行的选中带是同一支色。返回这一帧有没有被点。
func (a *app) wsPaneTabButton(c *ui.Context, pal palette, label, tip string, icon *ui.SVG, on bool) bool {
	btn := ui.ButtonBase(c).Size(22, 22).Shrink(0).Radius(radiusSM).Center().
		Cursor(ui.CursorPointer).Label(label).Tooltip(tip)
	col := pal.inkSubtle
	switch {
	case on:
		btn.Background(pal.selection)
		col = pal.ink
	case btn.Hovered():
		btn.Background(pal.surface2)
	}
	fadeColors(btn)
	btn.Children(func() {
		drawIcon(c, icon, 13, col)
	})
	return btn.Clicked()
}

// startWsSearch 发起一次后台搜索。主线程调用：仓库路径、文件路径表和查询词
// 先捕获成值，后台只碰局部变量，结果经 dispatch 送回来，号对不上（期间又
// 发起了新查询）就作废 —— 和编辑器读文件是同一套纪律。
func (a *app) startWsSearch(q string) {
	s := &a.wsSearch
	s.seq++
	s.query = q
	s.root = a.repoPath
	if q == "" || a.repoPath == "" {
		s.running, s.results, s.fileCount, s.truncated = false, nil, 0, false
		return
	}
	paths := make([]string, 0, len(a.wsAll))
	for _, f := range a.wsAll {
		paths = append(paths, pathOf(f))
	}
	seq := s.seq
	root := s.root
	s.running = true
	a.run(func() {
		hits, files, truncated := scanWsFiles(root, paths, q)
		a.dispatch(func() {
			s := &a.wsSearch
			if s.seq != seq {
				return // 期间又敲了字，这份结果过期了
			}
			s.running = false
			s.results, s.fileCount, s.truncated = hits, files, truncated
		})
	})
}

// scanWsFiles 在 root 下的 paths 里找含 query（不分大小写）的行。
// 纯函数：只碰参数和文件系统，后台扫描与测试共用这一份实现。
//
// 上限三道：命中条数、命中文件数、扫描文件数。到了就停，truncated 如实
// 报告 —— 不求扫全，求快。路径按字母序来，扫不到的都在字母表后面。
func scanWsFiles(root string, paths []string, query string) (hits []wsSearchHit, fileCount int, truncated bool) {
	q := strings.ToLower(query)
	if q == "" {
		return nil, 0, false
	}
	scanned := 0
	for _, p := range paths {
		if len(hits) >= wsSearchMaxHits || fileCount >= wsSearchMaxFiles || scanned >= wsSearchMaxScan {
			truncated = true
			return hits, fileCount, truncated
		}
		data, err := readHead(filepath.Join(root, filepath.FromSlash(p)), wsSearchMaxFile)
		if err != nil || looksBinary(data) {
			continue
		}
		scanned++
		before := len(hits)
		for i, line := range strings.Split(string(data), "\n") {
			if len(hits) >= wsSearchMaxHits {
				truncated = true
				break
			}
			off, ok := indexLine(line, q)
			if !ok {
				continue
			}
			hits = append(hits, wsHitWindow(p, i+1, line, off, len(q)))
		}
		if len(hits) > before {
			fileCount++
		}
	}
	return hits, fileCount, truncated
}

// indexLine 在 line 里找 q（不分大小写），返回命中的字节偏移。
//
// 先在小写副本里找（快），再用 EqualFold 对回原文确认起点：个别字符的
// 大小写映射会改字节长度（如 İ 转小写是两个字符），小写副本里的偏移对不上
// 原文，这时在原文里用 EqualFold 补一次线性找 —— 只有真命中的行才付这笔钱。
func indexLine(line, q string) (int, bool) {
	if q == "" {
		return 0, false
	}
	low := strings.ToLower(line)
	off := strings.Index(low, q)
	if off < 0 {
		return 0, false
	}
	if off+len(q) <= len(line) && strings.EqualFold(line[off:off+len(q)], q) {
		return off, true
	}
	for i := 0; i+len(q) <= len(line); i++ {
		if strings.EqualFold(line[i:i+len(q)], q) {
			return i, true
		}
	}
	return 0, false
}

// wsHitWindow 把命中行裁成前后各留若干字符的窗口，截断侧补省略号；返回窗口
// 文本和命中段在窗口里的偏移。两头都按字符边界对齐，中文不会被切成半截。
func wsHitWindow(path string, lineNo int, line string, off, n int) wsSearchHit {
	line = strings.TrimSuffix(line, "\r")
	if off < 0 || off+n > len(line) {
		return wsSearchHit{Path: path, Line: lineNo, Text: line}
	}
	start, end := off, off+n
	for i := 0; i < wsSearchCtx && start > 0; i++ {
		_, sz := utf8.DecodeLastRuneInString(line[:start])
		start -= sz
	}
	for i := 0; i < wsSearchCtx && end < len(line); i++ {
		_, sz := utf8.DecodeRuneInString(line[end:])
		end += sz
	}
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(line) {
		suffix = "…"
	}
	text := prefix + line[start:end] + suffix
	return wsSearchHit{
		Path: path, Line: lineNo, Text: text,
		Off: len(prefix) + off - start, Len: n,
	}
}

// readHead 读文件的前 limit 个字节。搜索不求读全：超大的文件只看头部，
// 内存和时间都封了顶 —— 与指纹、图片预览是同一套取舍。
func readHead(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, err := io.ReadFull(f, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// openWsSearchHit 在右边的编辑器里打开命中的文件，能把那一行滚到顶上就滚。
func (a *app) openWsSearchHit(h wsSearchHit) {
	a.selectWorkspaceKey(h.Path)
	a.syncEditorPane()
	if a.editorPane.path == h.Path && a.editorPane.code != nil {
		a.editorPane.code.ScrollTo(h.Line)
		a.editorPane.pendingLine = 0
		return
	}
	// 缓冲还在读盘：把目标行记下，等 loadEditorPane 的 dispatch 就位后兑现。
	a.editorPane.pendingLine = h.Line
}

// wsSearchRows 把命中列表摊成显示行：同一路径的连续命中归进一个组，组头
// 一行、命中若干行。扫描器按路径序出结果，这里只需归组，不用再排。
func wsSearchRows(hits []wsSearchHit) []wsSearchRow {
	rows := make([]wsSearchRow, 0, len(hits)+8)
	for i := 0; i < len(hits); {
		j := i
		for j < len(hits) && hits[j].Path == hits[i].Path {
			j++
		}
		rows = append(rows, wsSearchRow{path: hits[i].Path, head: true, count: j - i})
		for k := i; k < j; k++ {
			rows = append(rows, wsSearchRow{path: hits[k].Path, hit: &hits[k]})
		}
		i = j
	}
	return rows
}

// workspaceSearchView 是搜索页签那一栏：输入框、命中汇总、按文件分组的结果。
func (a *app) workspaceSearchView(c *ui.Context, pal palette, t *ui.Theme) {
	ui.Box(c).Key("workspace-search-input-wrap").Shrink(0).Padding(0, 12, 8, 12).Children(func() {
		in := searchField(c, pal, &a.wsSearchQuery, T("搜索整个工作区", "Search across workspace"))
		if a.focusWsSearch {
			in.Focus()
			a.focusWsSearch = false
		}
	})

	// 查询词变了（或换了仓库）就重跑 —— 与文件树筛选同一套做法：逐帧比对
	// 值，不依赖控件的 Changed()。
	q := strings.TrimSpace(a.wsSearchQuery)
	if q != a.wsSearch.query || a.wsSearch.root != a.repoPath {
		a.startWsSearch(q)
	}
	s := &a.wsSearch

	switch {
	case q == "":
		emptyNote(c, t, T("输入关键词，搜索工作区里所有文件的内容", "Type to search the contents of every file in the workspace"))
		return
	case s.running && len(s.results) == 0:
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Spinner(c).Size(16, 16)
				ui.Text(c, T("正在搜索…", "Searching…")).FontSize(textCaption).TextColor(pal.inkSubtle)
			})
		})
		return
	case len(s.results) == 0:
		emptyNote(c, t, T("没有匹配的结果", "No matching results"))
		return
	}

	ui.Row(c).Shrink(0).Padding(2, 12, 6, 12).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, T("在 %d 个文件里命中 %d 处", "%d hits in %d files"), len(s.results), s.fileCount).
			FontSize(textCaption).TextColor(pal.inkSubtle)
		if s.truncated {
			ui.Text(c, T("结果已截断", "Results truncated")).FontSize(textCaption).
				TextColor(pal.inkTertiary)
		}
	})

	rows := wsSearchRows(s.results)
	a.wsSearchList.Key = func(i int) any { return rows[i].key() }
	ui.List(c, &a.wsSearchList, len(rows), func(i int) {
		r := rows[i]
		if r.head {
			a.wsSearchHeadRow(c, pal, r)
		} else {
			a.wsSearchHitRow(c, pal, *r.hit)
		}
	}).Grow(1)
}

// wsSearchHeadRow 是文件组头：路径加命中数，点了打开这个文件。
func (a *app) wsSearchHeadRow(c *ui.Context, pal palette, r wsSearchRow) {
	row := ui.Row(c).Height(22).Shrink(0).FillWidth().Padding(0, 12).Gap(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(pal.rowHover)
	}
	row.Children(func() {
		ui.Text(c, r.path).Font("monospace").FontSize(textCaption).Grow(1).
			MinWidth(0).SingleLine().TextColor(pal.ink)
		ui.Textf(c, "%d", r.count).Shrink(0).FontSize(textCaption).
			FontFeatures("tnum").TextColor(pal.inkSubtle)
	})
	if row.Clicked() {
		a.openWsSearchHit(wsSearchHit{Path: r.path, Line: 1})
	}
}

// wsSearchHitRow 是一条命中：行号加带高亮的片段，点了打开到那一行。
func (a *app) wsSearchHitRow(c *ui.Context, pal palette, h wsSearchHit) {
	row := ui.Row(c).Height(22).Shrink(0).FillWidth().Padding(0, 12, 0, 20).Gap(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(pal.rowHover)
	}
	row.Children(func() {
		ui.Textf(c, "%d", h.Line).Width(28).Shrink(0).TextAlign(ui.End).
			FontSize(11).FontFeatures("tnum").TextColor(pal.inkTertiary)
		// 片段可能比栏宽长：Clip 掉，三段文本在窗口里从左往右排。
		ui.Box(c).Grow(1).MinWidth(0).Clip().Children(func() {
			ui.Row(c).Gap(0).AlignItems(ui.Center).Children(func() {
				ui.Text(c, h.Text[:h.Off]).SingleLine().FontSize(11).TextColor(pal.inkMuted)
				ui.Text(c, h.Text[h.Off:h.Off+h.Len]).SingleLine().FontSize(11).TextColor(pal.accent)
				ui.Text(c, h.Text[h.Off+h.Len:]).SingleLine().FontSize(11).TextColor(pal.inkMuted)
			})
		})
	})
	if row.Clicked() {
		a.openWsSearchHit(h)
	}
}
