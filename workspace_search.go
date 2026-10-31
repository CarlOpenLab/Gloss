package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
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
	// Pos / LfPos 是命中段在文件里的字节偏移，跳转时选中这段文字用：Pos 对
	// 磁盘上的原始字节（CRLF 占两个），LfPos 对编辑器缓冲里的文本（MyGo 把
	// CRLF 归一成 LF）。两者的差就是命中行之前以 CRLF 收尾的行数，扫描时
	// 顺手记下，点结果那一拍就不用再翻文件了。
	Pos   int
	LfPos int
}

// wsSearchState 是搜索页签的活状态。root 记着结果属于哪个仓库：换了仓库，
// 这些结果就是别家的事，作废重跑。include/exclude 记着跑这一轮时的路径
// 过滤原文，变了同样作废重跑。
type wsSearchState struct {
	root      string
	query     string // 已经跑起来（或跑完）的查询词，trim 过
	include   string // 路径过滤原文（包含/排除，glob）
	exclude   string
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
		btn.Background(pal.rowHover)
	}
	fadeColors(btn)
	btn.Children(func() {
		drawIcon(c, icon, 13, col)
	})
	return btn.Clicked()
}

// startWsSearch 发起一次后台搜索。主线程调用：仓库路径、文件路径表、查询词
// 和路径过滤先捕获成值，后台只碰局部变量，结果经 dispatch 送回来，号对不上
// （期间又发起了新查询）就作废 —— 和编辑器读文件是同一套纪律。
func (a *app) startWsSearch(q string) {
	s := &a.wsSearch
	s.seq++
	s.query = q
	s.root = a.repoPath
	s.include, s.exclude = a.wsSearchInclude, a.wsSearchExclude
	include, exclude := wsGlobPatterns(a.wsSearchInclude), wsGlobPatterns(a.wsSearchExclude)
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
		hits, files, truncated := scanWsFiles(root, paths, q, include, exclude)
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

// scanWsFiles 在 root 下的 paths 里找含 query（不分大小写）的行，路径先过
// 包含/排除的 glob（见 wsPathAllowed）。纯函数：只碰参数和文件系统，后台
// 扫描与测试共用这一份实现。
//
// 上限三道：命中条数、命中文件数、走过的文件数。到了就停，truncated 如实
// 报告 —— 不求扫全，求快。路径按字母序来，扫不到的都在字母表后面。走过的
// 数的是「看过的每一个文件」而不只是扫出内容的：二进制仓库不然会一个不漏
// 地读过去，上限就名存实亡了。被 glob 挡在门外的文件一文不值（一次盘都没
// 碰），不计入。
func scanWsFiles(root string, paths []string, query string, include, exclude []string) (hits []wsSearchHit, fileCount int, truncated bool) {
	q := strings.ToLower(query)
	if q == "" {
		return nil, 0, false
	}
	buf := make([]byte, wsSearchMaxFile) // 一份缓冲从头用到尾，不逐文件分配
	walked := 0
	for _, p := range paths {
		if len(hits) >= wsSearchMaxHits || fileCount >= wsSearchMaxFiles || walked >= wsSearchMaxScan {
			truncated = true
			return hits, fileCount, truncated
		}
		if !wsPathAllowed(p, include, exclude) {
			continue
		}
		walked++
		data, err := readHead(filepath.Join(root, filepath.FromSlash(p)), buf)
		if err != nil || looksBinary(data) {
			continue
		}
		before := len(hits)
		lines := strings.Split(string(data), "\n")
		base, crlf := 0, 0 // 本行的起始字节；本行之前以 \r\n 收尾的行数
		for i, line := range lines {
			if i > 0 {
				base += len(lines[i-1]) + 1
				if strings.HasSuffix(lines[i-1], "\r") {
					crlf++
				}
			}
			if len(hits) >= wsSearchMaxHits {
				truncated = true
				break
			}
			off, ok := indexLine(line, q)
			if !ok {
				continue
			}
			hit := wsHitWindow(p, i+1, line, off, len(q))
			hit.Pos, hit.LfPos = base+off, base-crlf+off
			hits = append(hits, hit)
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

// wsGlobPatterns 把用户敲的一串路径模式拆干净：逗号（含中文逗号）、空格都
// 算分隔，空白条目丢掉 —— 和 VS Code 的 include/exclude 框一个脾气。
func wsGlobPatterns(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// wsPathAllowed 报告路径 p 该不该进扫描：有包含模式时至少得命中一个，任何
// 排除模式命中即出局；两框都空就是全部放行。排除的分量更重 —— 包含放进来
// 的，排除能再捞出去（VS Code 同款）。
func wsPathAllowed(p string, include, exclude []string) bool {
	if len(include) > 0 {
		ok := false
		for _, g := range include {
			if wsGlobMatch(g, p) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, g := range exclude {
		if wsGlobMatch(g, p) {
			return false
		}
	}
	return true
}

// wsGlobMatch 报告工作区路径 path（正斜杠、相对仓库根）是否匹配 glob 模式
// pattern。支持 VS Code 那套的常用子集：`**` 跨任意层目录、`*` 段内任意、
// `?` 单字符；`[]` 字符类不支持。不带 `/` 的模式对任意一层的名字匹配 ——
// 所以 `node_modules` 排除得净整棵目录，`*.go` 包含得住所有深度的 Go 文件。
// 比较不分大小写（Windows 的家规，跨平台也不吃亏）。
func wsGlobMatch(pattern, path string) bool {
	pattern, path = strings.ToLower(pattern), strings.ToLower(path)
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		// 不带 /：任意一层的名字对上就算 —— 对「排除目录名」和「包含
		// 扩展名」这两类最常用的写法都刚好。
		for _, seg := range strings.Split(path, "/") {
			if globSeg(pattern, seg) {
				return true
			}
		}
		return false
	}
	return globSegs(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// globSegs 按 `/` 分段对齐匹配，`**` 一段可以吃掉 0 到任意多段。
func globSegs(pp, fp []string) bool {
	for len(pp) > 0 {
		if pp[0] == "**" {
			if len(pp) == 1 {
				return true
			}
			for i := 0; i <= len(fp); i++ {
				if globSegs(pp[1:], fp[i:]) {
					return true
				}
			}
			return false
		}
		if len(fp) == 0 || !globSeg(pp[0], fp[0]) {
			return false
		}
		pp, fp = pp[1:], fp[1:]
	}
	return len(fp) == 0
}

// globSeg 匹配单独一段：`*` 段内任意（不含 /，天然如此）、`?` 单字符。
// 经典的两指针回溯，多一个字节的回溯栈都不用。
func globSeg(pat, s string) bool {
	if !strings.ContainsAny(pat, "*?") {
		return pat == s
	}
	pi, si, star, mark := 0, 0, -1, 0
	for si < len(s) {
		if pi < len(pat) && (pat[pi] == '?' || pat[pi] == s[si]) {
			pi, si = pi+1, si+1
		} else if pi < len(pat) && pat[pi] == '*' {
			star, mark = pi, si
			pi++
		} else if star >= 0 {
			mark++
			si = mark
			pi = star + 1
		} else {
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}

// readHead 读文件的前 len(buf) 个字节，buf 由调用方复用（一次扫描一份，
// 不逐文件分配）。搜索不求读全：超大的文件只看头部，内存和时间都封了顶
// —— 与指纹、图片预览是同一套取舍。
func readHead(path string, buf []byte) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n, err := io.ReadFull(f, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// openWsSearchHit 在右边的编辑器里打开命中的文件，并兑现这一条命中：文件已
// 加载就当场选中匹配的那段文字，还在读盘就先记下，等 loadEditorPane 的
// dispatch 就位后兑现（editorPaneState.pendingHit）。
//
// 文件树把它筛掉了也不碍事（VS Code 同款：树筛选只管树，不管搜索结果）——
// 选中落空时把文件「钉」给编辑器，照样打开；点树里任何一行就回正常选中。
func (a *app) openWsSearchHit(h wsSearchHit) {
	a.selectWorkspaceKey(h.Path)
	if a.selectedWorkspaceKey() != h.Path && wsFileByPath(a.wsAll, h.Path) != nil {
		a.wsPinPath = h.Path
	}
	if a.editorPane.loading && a.editorPane.path != h.Path {
		// 上一份文件还在读盘：不等它了，直接改读命中的这份 —— 旧的那份
		// dispatch 会因路径对不上自己作废。
		if f := a.editorTarget(); f != nil && pathOf(*f) == h.Path {
			a.loadEditorPane(*f)
		}
	} else {
		a.syncEditorPane()
	}
	if a.editorPane.path == h.Path && a.editorPane.code != nil {
		revealWsHit(a.editorPane.code, h)
		a.editorPane.pendingHit = wsSearchHit{}
		return
	}
	a.editorPane.pendingHit = h
}

// revealWsHit 在编辑器里兑现一条搜索命中：选中匹配的那段文字 —— 选中底色就
// 是高亮，活动行跟着落在那一行，编辑器自己会把选区滚进视野，和主流编辑器点
// 搜索结果的做法一致。
//
// 文件在搜索之后可能又变过：先把缓冲里的那段字节和扫到的比对，对不上就不选
// （选会点错地方），退回把光标放到命中行行首 —— 活动行高亮顶上，GoTo 会把
// 它滚到编辑器中间。文件组头的跳转没有匹配段可亮，把那一行滚到顶上就好。
func revealWsHit(st *editor.State, h wsSearchHit) {
	if st == nil {
		return
	}
	if h.Len == 0 {
		if h.Line > 0 {
			st.ScrollTo(h.Line - 1)
		}
		return
	}
	if st.Buffer != nil {
		text := st.Buffer.String() // 缓冲里的文本，CRLF 已归一成 LF，对 LfPos
		if h.LfPos+h.Len <= len(text) && text[h.LfPos:h.LfPos+h.Len] == h.Text[h.Off:h.Off+h.Len] {
			st.Select(h.LfPos, h.LfPos+h.Len)
			return
		}
	}
	st.GoTo(editor.Pos{Line: max(0, h.Line-1), Col: 0})
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

// workspaceSearchView 是搜索页签那一栏：输入框（带路径过滤的开合）、命中
// 汇总、按文件分组的结果。
func (a *app) workspaceSearchView(c *ui.Context, pal palette, t *ui.Theme) {
	ui.Box(c).Key("workspace-search-input-wrap").Shrink(0).Padding(0, 12, 8, 12).Children(func() {
		ui.Column(c).Shrink(0).FillWidth().Gap(6).Children(func() {
			ui.Row(c).Shrink(0).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Grow(1).Children(func() {
					in := searchField(c, pal, &a.wsSearchQuery, T("搜索整个工作区", "Search across workspace"))
					if a.focusWsSearch {
						in.Focus()
						a.focusWsSearch = false
					}
				})
				// 路径过滤的开合：VS Code 搜索框边上那颗「…」的同款。
				if a.wsPaneTabButton(c, pal, "", T("按路径过滤（包含/排除）", "Filter by path (include/exclude)"),
					iconSliders, a.wsSearchPathOpen) {
					a.wsSearchPathOpen = !a.wsSearchPathOpen
				}
			})
			if a.wsSearchPathOpen {
				a.wsSearchPathPanel(c, pal)
			}
		})
	})

	// 查询词或路径过滤变了（或换了仓库）就重跑 —— 与文件树筛选同一套做法：
	// 逐帧比对值，不依赖控件的 Changed()。
	q := strings.TrimSpace(a.wsSearchQuery)
	if q != a.wsSearch.query || a.wsSearch.root != a.repoPath ||
		a.wsSearchInclude != a.wsSearch.include || a.wsSearchExclude != a.wsSearch.exclude {
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
		if s.truncated {
			emptyNote(c, t, T("没有匹配的结果（扫描有上限，没扫完全部文件）", "No matching results (scanning hit its cap; not every file was scanned)"))
		} else {
			emptyNote(c, t, T("没有匹配的结果", "No matching results"))
		}
		return
	}

	ui.Row(c).Shrink(0).Padding(2, 12, 6, 12).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, T("在 %d 个文件里命中 %d 处", "%d hits in %d files"), len(s.results), s.fileCount).
			FontSize(textCaption).TextColor(pal.inkSubtle)
		if len(wsGlobPatterns(s.include)) > 0 || len(wsGlobPatterns(s.exclude)) > 0 {
			ui.Text(c, T("· 路径过滤生效中", "· path filters active")).FontSize(textCaption).
				TextColor(pal.inkTertiary)
		}
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

// wsSearchPathPanel 是路径过滤的展开面板：包含/排除两框加几颗建议珠。
// 框里敲的是 glob（逗号分隔），规则见 wsGlobMatch；珠子从工作区现凑，
// 点一下并进「包含」框 —— 想排除谁，自己敲比点快，就不给排除放珠了。
func (a *app) wsSearchPathPanel(c *ui.Context, pal palette) {
	ui.Column(c).Shrink(0).FillWidth().Gap(6).Children(func() {
		wsSearchPathField(c, pal, iconPlus, &a.wsSearchInclude,
			T("只搜这些：*.go, src/**（逗号分隔，不填=全部）", "Only these: *.go, src/** (comma separated, empty = all)"))
		wsSearchPathField(c, pal, iconX, &a.wsSearchExclude,
			T("不搜这些：node_modules, dist, *.lock", "Not these: node_modules, dist, *.lock"))
		if sug := wsSearchSuggestions(a.wsAll); len(sug) > 0 {
			ui.Row(c).Shrink(0).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, T("试试：", "Try:")).FontSize(textCaption).TextColor(pal.inkTertiary).Shrink(0)
				for _, sg := range sug {
					sg := sg
					chip := ui.ButtonBase(c).Shrink(0).Height(18).Padding(0, 8).Radius(radiusPill).
						Center().Cursor(ui.CursorPointer).Label(sg).Tooltip(Tf("把 %s 加进「只搜这些」", "Add %s to include", sg))
					if chip.Hovered() {
						chip.Background(pal.rowHover)
					}
					fadeColors(chip)
					chip.Children(func() {
						ui.Text(c, sg).Font("monospace").FontSize(10).TextColor(pal.inkMuted)
					})
					if chip.Clicked() {
						a.wsSearchInclude = wsAppendPattern(a.wsSearchInclude, sg)
					}
				}
			})
		}
	})
}

// wsSearchPathField 是路径过滤的一行小输入框：chip 底、左边一枚小图标，
// 和搜索框同一副骨架（见 searchField）。
func wsSearchPathField(c *ui.Context, pal palette, icon *ui.SVG, value *string, placeholder string) {
	ui.Row(c).Height(searchH).Shrink(0).FillWidth().AlignItems(ui.Center).
		Gap(6).Padding(0, 8).Radius(radiusMD).Background(pal.chip).
		Border(1, pal.hairline).Label(placeholder).Children(func() {
		drawIcon(c, icon, 12, pal.inkSubtle)
		ui.TextInputBase(c, value).Grow(1).MinWidth(0).
			Placeholder(placeholder).Label(placeholder)
	})
}

// wsAppendPattern 把一颗建议珠并进「包含」框：去重，逗号加空格续写。
func wsAppendPattern(cur, pat string) string {
	for _, g := range wsGlobPatterns(cur) {
		if strings.EqualFold(g, pat) {
			return cur
		}
	}
	if strings.TrimSpace(cur) == "" {
		return pat
	}
	return strings.TrimRight(cur, "，, ") + ", " + pat
}

// wsSearchSuggestions 从工作区文件表里凑几颗建议珠：出现最多的顶层目录和
// 扩展名各取前三。目录珠是裸名字 —— 不带 / 的模式对任意一层的名字匹配，
// 「只搜这个目录（含子目录）」刚好就是这么写的；扩展名珠是 *.ext。
func wsSearchSuggestions(files []gitrepo.File) []string {
	dirs, exts := map[string]int{}, map[string]int{}
	for _, f := range files {
		p := pathOf(f)
		if i := strings.IndexByte(p, '/'); i > 0 {
			dirs[p[:i]]++
		}
		if ext := strings.TrimPrefix(filepath.Ext(p), "."); ext != "" {
			exts["*."+ext]++
		}
	}
	pick := func(m map[string]int, n int) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if m[keys[i]] != m[keys[j]] {
				return m[keys[i]] > m[keys[j]]
			}
			return keys[i] < keys[j]
		})
		if len(keys) > n {
			keys = keys[:n]
		}
		return keys
	}
	return append(pick(dirs, 3), pick(exts, 3)...)
}
