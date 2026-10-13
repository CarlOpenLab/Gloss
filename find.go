package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 这个文件是 diff 查找：匹配、索引、高亮，以及查找栏本身。
//
// 命中一律用「源坐标」记录 —— 文件下标 + hunk 下标 + 行下标，而不是行模型的
// 下标。行模型是派生数据，折叠文件、展开未改动区间、切 unified/split 都会让
// 它整体重建；用行下标的话每做一次这些动作命中就全废一次，计数还会跟着跳。
// 源坐标只跟 (files, contents, 查询词) 有关，所以命中集合和计数都是稳的。

// findHit 是一行里的一段命中。
type findHit struct {
	// start/end 相对的是「expandTabs 之后」的显示文本 —— 渲染时喂给
	// lineSpans 的就是那一份，两边必须同源，否则高亮会整体错位。
	start, end int
	// match 是在 findState.matches 里的下标。findHits 不管它（恒 0），
	// 由 buildFindIndex 填 —— 「哪一条是当前命中」就是拿它和 cur 比。
	match int
}

// findMatch 是一条命中在源坐标里的位置。
//
// hunk >= 0 时 line 是 file.Hunks[hunk].Lines 的下标；hunk == -1 时 line 是
// 新侧行号（未改动区间里的行，内容来自 a.contents 里读到的那一份）。
type findMatch struct {
	file, hunk, line int
	start, end       int
}

// findKey 是「一行」的身份，渲染时拿它反查这一行上的命中。
//
// 未改动行必须用 hunk = -1 记：那些行在行模型里的 hunk/lineIdx 是零值 0
// （见 rows.go 的 appendGapLines），按 (hunk, lineIdx) 查会和该文件第一条
// hunk 行撞上，把底色涂到别的行去。
type findKey struct{ file, hunk, line int }

// findLine 是渲染某一行（或 split 的某一半）要涂的命中。
// cur 是当前命中在 matches 里的下标，用来把「当前这一条」画得更重。
type findLine struct {
	hits []findHit
	cur  int
}

// findQuery 是编译好的查询词。
//
// 大小写不敏感的比较按 rune 对齐（取与查询词等量 rune 的片段再比），所以
// rune 数得提前算好，不能每行重数。
type findQuery struct {
	text  string
	runes int
}

func compileFindQuery(s string) findQuery {
	return findQuery{text: s, runes: utf8.RuneCountInString(s)}
}

// findHits 在一行文本里找出所有命中，从左到右、互不重叠。
//
// 大小写不敏感走的是「按 rune 取等长片段 + EqualFold」，不是 strings.ToLower
// 整串转换：Unicode 的大小写映射会改变字节长度（İ 这类字符），转完偏移就对
// 不上原文了 —— 而这里的偏移要直接拿去切显示文本。
func findHits(text string, q findQuery) []findHit {
	if q.runes == 0 || len(text) == 0 {
		return nil
	}
	var out []findHit
	for i := 0; i < len(text); {
		end, n := i, 0
		for n < q.runes && end < len(text) {
			_, sz := utf8.DecodeRuneInString(text[end:])
			end += sz
			n++
		}
		if n < q.runes {
			break // 剩下的字不够一个查询词了
		}
		if strings.EqualFold(text[i:end], q.text) {
			out = append(out, findHit{start: i, end: end})
			i = end
			continue
		}
		_, sz := utf8.DecodeRuneInString(text[i:])
		i += sz
	}
	return out
}

// buildFindIndex 扫出全部命中，并建一张「某一行上有哪些命中」的反查表。
//
// 生成顺序：文件顺序 → 文件内「未改动区间 / hunk」交错（gap0, hunk0, gap1, …）
// → 行号递增。这个顺序和界面从上往下看一致，「下一个」按它走就是一路向下。
//
// 未改动行读的是 a.contents 里已经缓存的新侧内容（渲染那些行的 contentLine
// 读的是同一份），所以「界面上看得见的正文都能搜到」；内容没读到的文件，
// 那些行在界面上本来就是空白，搜不到是自洽的。
func buildFindIndex(files []gitrepo.File, contents map[string]*fileContent, q findQuery) ([]findMatch, map[findKey][]findHit) {
	if q.runes == 0 {
		return nil, nil
	}
	var matches []findMatch
	index := map[findKey][]findHit{}

	add := func(key findKey, text string) {
		for _, h := range findHits(text, q) {
			h.match = len(matches)
			index[key] = append(index[key], h)
			matches = append(matches, findMatch{
				file: key.file, hunk: key.hunk, line: key.line,
				start: h.start, end: h.end,
			})
		}
	}

	for fi := range files {
		f := &files[fi]
		// 冲突文件在界面上是一张「挑一边」的卡片，行模型里没有它的代码行；
		// 二进制文件根本没有正文。索引它们只会让 cur 停在一条跳不到的命中上。
		if f.Status == gitrepo.StatusConflict || f.Binary {
			continue
		}

		var lines []string
		if cc := contents[fileKey(*f)]; cc != nil {
			lines = cc.lines
		}
		gaps := fileGaps(*f, len(lines))
		addGap := func(g gapSpan) {
			for i := 0; i < g.n; i++ {
				n := g.from + i
				if n > len(lines) {
					break
				}
				add(findKey{fi, -1, n}, expandTabs(lines[n-1]))
			}
		}

		// 交错着走：每个 hunk 之前先放它前面那段未改动区间。生成顺序因此
		// 和界面从上往下看一致 —— 顺序错了「下一个」会在文件里来回跳。
		gi := 0
		for hi := range f.Hunks {
			h := &f.Hunks[hi]
			for gi < len(gaps) && gaps[gi].from < h.NewStartLine() {
				addGap(gaps[gi])
				gi++
			}
			for li := range h.Lines {
				add(findKey{fi, hi, li}, expandTabs(h.Lines[li].Content))
			}
		}
		for ; gi < len(gaps); gi++ {
			addGap(gaps[gi])
		}
	}
	return matches, index
}

// applyFind 把命中底色叠到已经切好的一行片段上（lineSpans 的出口都过它一遍）。
//
// 只动 Background（当前命中再加一道下划线），Color/Weight/Italic 一律原样带着
// 走 —— 查找不能把语法高亮毁掉。输入输出必须铺满同一个 text 的每一字节：
// RichText 按片段依次排版，多一段少一段整行就错位（和 lineSpans 同一条约束）。
func applyFind(spans []ui.Span, text string, fl findLine, pal palette) []ui.Span {
	hits := cleanHits(fl.hits, len(text))
	if len(hits) == 0 {
		return spans
	}

	out := make([]ui.Span, 0, len(spans)+2*len(hits))
	pos, hi := 0, 0
	for _, sp := range spans {
		segEnd := pos + len(sp.Text)
		// 整条落在这一段之前的命中先跳过。每个片段开头都要做一次 ——
		// 一个片段可能消化掉好几条命中。
		for hi < len(hits) && hits[hi].end <= pos {
			hi++
		}
		cur := pos
		for hi < len(hits) && hits[hi].start < segEnd {
			h := hits[hi]
			if s := max(h.start, cur); s > cur {
				out = append(out, reslice(sp, text[cur:s]))
				cur = s
			}
			end := min(h.end, segEnd)
			out = append(out, highlightSpan(sp, text[cur:end], h.match == fl.cur, pal))
			cur = end
			if h.end > segEnd {
				// 这条命中跨到了下一个片段（查 "func main" 落在两个语法
				// token 上就是这样），留着继续涂。
				break
			}
			hi++
		}
		if cur < segEnd {
			out = append(out, reslice(sp, text[cur:segEnd]))
		}
		pos = segEnd
	}
	if pos != len(text) {
		// 片段没铺满整行（正常不该发生）：宁可不好看，也别把颜色涂到别的字上
		// —— 和 lineSpans 里那条退回纯文本的哲学一致。
		return spans
	}
	return out
}

// cleanHits 把命中区间洗一遍：夹到 text 内、丢掉空区间、把与前一条重叠的部分
// 裁掉（留下的尾巴照常涂，整条丢掉会让那段字凭空失去高亮）。
//
// 这是防御性的：索引和文本万一不一致（内容刚变、这一帧还没重算），也不让
// 它涂到别的字上。不改传入的切片 —— 那份是索引里的，多个调用点共用。
func cleanHits(hits []findHit, n int) []findHit {
	var out []findHit
	prev := 0
	for _, h := range hits {
		s, e := max(h.start, prev), min(h.end, n)
		if s >= e {
			continue
		}
		out = append(out, findHit{start: s, end: e, match: h.match})
		prev = e
	}
	return out
}

// reslice 复制一个片段并换掉文字，其余字段（字体、颜色、字重……）原样保留。
func reslice(sp ui.Span, text string) ui.Span {
	sp.Text = text
	return sp
}

// highlightSpan 是命中那一段的样子。当前命中更重，并且多一道下划线：
// 连按「下一个」时视线要能立刻钉住新落点，只靠底色深浅不够快。
func highlightSpan(sp ui.Span, text string, cur bool, pal palette) ui.Span {
	sp.Text = text
	sp.Background = pal.findBg
	if cur {
		sp.Background = pal.findBgActive
		sp.Underline = true
		sp.DecorationColor = pal.findUnderline
		sp.DecorationThickness = 2
	}
	return sp
}

// findState 是查找的全部状态。
//
// builtQuery/builtGen 是「这批 matches 是按什么算出来的」：查询词，加一个
// 内容版本号（app.findGen）。两者都没变就一个字节都不用重扫 —— 触发重算的
// 是「输入变了」和「文件列表/内容变了」，不是「行模型脏了」（折叠、展开、
// 切视图都会弄脏行模型，但和命中集合毫无关系）。
type findState struct {
	on      bool
	query   string // 绑定给查找栏的输入框
	matches []findMatch
	// byLine 是「某一行上有哪些命中」，渲染时按 findKey 反查。
	byLine map[findKey][]findHit
	cur    int
	// pending 是本帧要执行的跳转。不在这里直接算行下标：发起跳转的时候行
	// 模型可能还是旧的（折叠文件还没展开、区间还没放行），按旧下标滚会落到
	// 别的行上 —— 和 pendingScrollFile 是同一个道理，等重建完再解析。
	pending *findMatch
	// focus 是一次性的「把焦点放进输入框」请求。和 filepane 的 focusFilter
	// 一个套路：置位晚一帧才轮到输入框，那时没有谁跟它抢。
	focus bool
	// focused 记的是上一帧输入框有没有焦点。裸方向键在编辑框里会不会被吃掉
	// 没法从代码里断定，所以给 J/K/↑/↓ 那段导航留一道闸（和提交框的
	// commitFocused 同款做法）。
	focused    bool
	builtQuery string
	builtGen   int
	// builtFiles 是建索引时那份文件列表的 key（按 a.files 的顺序）。重扫
	// 之后要把「当前是第几条」找回来，而旧命中里的 file 是**旧列表**的下标
	// —— 列表换过之后那个下标指到谁身上已经无从查起，只能建索引时先记下来。
	builtFiles []string
}

// —— 开关 ——

// openFind 打开查找栏（⌘F）。
//
// 没有 diff 可搜时什么都不做：开一条搜不到东西的栏只会让人以为功能坏了。
func (a *app) openFind() {
	if a.viewMode != viewModeGit || !a.hasDiff() {
		return
	}
	// 已经开着就是「把焦点送回来」，查询词留着 —— 手滑按了 Esc 之后
	// ⌘F 应该能接着刚才那次查找，而不是从头再来。
	a.find.on = true
	a.find.focus = true
}

// closeFind 收栏（Esc）。命中当场清掉、高亮立刻消失；查询词留着，
// 下一次 ⌘F 打开时还是它。
func (a *app) closeFind() {
	a.find.on = false
	a.find.focus = false
	// 焦点闸也要松开：栏一收，findBar 就不再跑了，没人再去更新它 ——
	// 留着 true 会把 J/K/↑/↓ 的导航永久锁死。
	a.find.focused = false
	a.clearFindMatches()
	// builtQuery 也清掉：下次打开哪怕查询词没变也要重扫 —— 中间文件内容
	// 可能已经变了，而 findGen 从我们停止监听的那一刻起就不动了。
	a.find.builtQuery = ""
	a.find.builtFiles = nil
}

// clearFindMatches 清掉命中本身（查询词、开关都不动）。
func (a *app) clearFindMatches() {
	a.find.matches = nil
	a.find.byLine = nil
	a.find.cur = 0
	a.find.pending = nil
}

// invalidate 让索引整个作废：清掉命中，并逼下一次 syncFind 重扫。
//
// 用在 a.files 被整份换掉、但没走 applyFilter 的地方（切标签页）——
// 索引里的 file 下标指向的是另一个仓库的列表，留着轻则涂错行、重则越界。
func (a *app) invalidate() {
	a.clearFindMatches()
	a.find.builtQuery = ""
	a.find.builtFiles = nil
}

// —— 索引对账 ——

// syncFind 是每帧开头的对账：查询词或内容变了就重扫命中。
//
// 触发条件是 (查询词, findGen) 这两个版本号，不是 rowsDirty —— 打字不会
// 弄脏行模型（拿 rowsDirty 当信号等于永远不触发），而折叠文件、展开区间、
// 切 unified/split 会弄脏它却和命中集合毫无关系（拿它当信号等于每点一下
// 展开就白扫一遍全文）。
//
// 调用点在 view() 里、handleKeys 之后：框架的输入阶段在这一帧之前就把
// 打的字写进 a.find.query 了，所以这里读到的已经是新值。
func (a *app) syncFind() {
	if !a.find.on || (a.find.query == a.find.builtQuery && a.findGen == a.find.builtGen) {
		return
	}
	queryChanged := a.find.query != a.find.builtQuery
	prev, prevKeys, prevCur := a.find.matches, a.find.builtFiles, a.find.cur

	if a.find.query == "" || !a.hasDiff() {
		a.clearFindMatches()
	} else {
		matches, byLine := buildFindIndex(a.files, a.contents, compileFindQuery(a.find.query))
		a.find.matches, a.find.byLine = matches, byLine
		if queryChanged {
			// 换了词就从第一条开始 —— 旧下标在新结果里没有意义。
			a.find.cur = 0
		} else {
			a.find.cur = reanchorFind(prev, prevKeys, prevCur, a.files, matches)
		}
	}
	a.find.builtFiles = fileKeys(a.files)
	a.find.builtQuery = a.find.query
	a.find.builtGen = a.findGen

	// 换了词就跳到第一条命中：打完字停在原地会让人以为没找到东西。
	// 只有换词才跳 —— 内容分批读回来时 cur 是重新锚定的，那时候再跳一次
	// 就是「视图自己动了」。
	if queryChanged && len(a.find.matches) > 0 {
		a.findRequest(a.find.matches[a.find.cur])
	}
}

// fileKeys 取出文件列表的稳定标识，顺序和列表一致。
func fileKeys(files []gitrepo.File) []string {
	keys := make([]string, len(files))
	for i := range files {
		keys[i] = fileKey(files[i])
	}
	return keys
}

// reanchorFind 在重扫之后找回「当前是第几条命中」。
//
// 认的是 (文件 key, hunk, line, 起点)：行坐标不受重扫影响，文件得用 key ——
// 文件下标会被筛选、被列表刷新整个挪位。找不回来（那条命中没了）就夹到合法
// 范围，**不跳回第一条**：用户指着的位置不该因为别处多了一行就跳走。
func reanchorFind(prev []findMatch, prevKeys []string, cur int, files []gitrepo.File, next []findMatch) int {
	if len(next) == 0 {
		return 0
	}
	cur = min(max(cur, 0), len(next)-1)
	if cur < len(prev) && prev[cur].file < len(prevKeys) {
		m := prev[cur]
		key := prevKeys[m.file]
		for i := range next {
			w := next[i]
			if w.hunk == m.hunk && w.line == m.line && w.start == m.start &&
				w.file < len(files) && fileKey(files[w.file]) == key {
				return i
			}
		}
	}
	return cur
}

// —— 跳转 ——

// findNext 跳上一条/下一条命中，到头绕回另一端（和 J/K 一个脾气）。
func (a *app) findNext(delta int) {
	if !a.find.on || len(a.find.matches) == 0 {
		return
	}
	n := len(a.find.matches)
	a.find.cur = ((a.find.cur+delta)%n + n) % n
	a.findRequest(a.find.matches[a.find.cur])
}

// findRequest 记一个「跳到这条命中」的请求。
//
// 不当场算行下标：发起的时候行模型可能还是旧的（文件还折着、区间还没放行），
// 按旧下标滚会落到别的行上 —— 和 pendingScrollFile 是同一个道理，等行模型
// 重建完再在 diffView 里解析成行下标。
func (a *app) findRequest(m findMatch) {
	if m.file < 0 || m.file >= len(a.files) {
		return
	}
	f := &a.files[m.file]
	if m.hunk < 0 {
		a.findRevealGap(f, m.line)
	}
	// 折着的文件先展开。这里**写死**成 false，而不是 toggle：跳进一个折着的
	// 文件就是要看它的内容，挡路的折叠（已 review 默认折的，和用户手动折的
	// 一样）都得让开。VS Code 同款行为 —— 代价是手动折过的状态回不来。
	if a.isFolded(*f) {
		a.folded[fileKey(*f)] = false
		a.rowsDirty = true
	}
	a.find.pending = &m
}

// findRevealGap 把挡住第 newNo 行的未改动区间整个放出来。
//
// 不是「再放一屏」（+= expandStep）：2000 行的区间里第 1500 行要点十五次
// 「展开」才够得着，而用户刚刚明确说了「我要去那儿」。
func (a *app) findRevealGap(f *gitrepo.File, newNo int) {
	cc := a.contents[fileKey(*f)]
	if cc == nil {
		return // 内容没读到：那些行根本画不出来，也搜不到
	}
	for _, g := range fileGaps(*f, len(cc.lines)) {
		if newNo < g.from || newNo >= g.from+g.n {
			continue
		}
		if g.n <= inlineGap {
			return // 本来就摊在屏幕上，没有折叠条挡路
		}
		key := gapKeyOf(fileKey(*f), g.from)
		if a.expanded == nil {
			a.expanded = map[string]int{}
		}
		if a.expanded[key] < g.n {
			a.expanded[key] = g.n
			a.rowsDirty = true
		}
		return
	}
}

// rowForFind 把源坐标解析成行模型的下标，没有返回 -1。
func (a *app) rowForFind(fi, hunk, line int) int {
	for i := range a.rows {
		r := &a.rows[i]
		if r.file != fi {
			continue
		}
		if hunk < 0 {
			if r.fromGap && r.newNo == line {
				return i
			}
			continue
		}
		// split 下空半行的下标就是 -1：不防的话「只删不增」的每一行都会被
		// 当成目标（那些行的 lineIdx 或 lidx 恰好是 -1）。
		if r.lineIdx >= 0 && r.hunk == hunk && r.lineIdx == line {
			return i
		}
		if r.lidx >= 0 && r.lhunk == hunk && r.lidx == line {
			return i
		}
	}
	return -1
}

// rowFindLine 取 unified 行要涂的命中。
//
// fromGap 必须**先判**：未改动行在行模型里的 hunk/lineIdx 是零值 0
// （见 rows.go 的 appendGapLines），不先判就会和该文件第一条 hunk 行撞 key，
// 把底色涂到别的行去。
func (a *app) rowFindLine(r row) findLine {
	if r.fromGap {
		return a.findLineOf(r.file, -1, r.newNo)
	}
	return a.findLineOf(r.file, r.hunk, r.lineIdx)
}

// findLineOf 取某一行上的命中。hunk < 0 时 line 是新侧行号（未改动行）。
// 关着、行号为空（split 的空半行）、索引里没这一行，都返回零值。
func (a *app) findLineOf(fi, hunk, line int) findLine {
	if !a.find.on || line < 0 {
		return findLine{}
	}
	hits := a.find.byLine[findKey{fi, hunk, line}]
	if len(hits) == 0 {
		return findLine{}
	}
	return findLine{hits: hits, cur: a.find.cur}
}

// —— 查找栏 ——

// findBarH 是查找栏的高度。比里面的搜索框（28）高一条内边距，
// 和文件栏栏头是同一档的厚度 —— 它俩本来就是同一种东西。
const findBarH = 40

// findFieldW 是查找输入框的宽度。
//
// 定死而不是让它撑满：撑满的话「计数 + 上/下一个」会被推到窗口另一头，
// 找东西的时候输入框和「找到第几条」得在视线里挨着。
const findFieldW = 240

// findBar 是停在 diff 列顶上的那条查找栏。
//
// 做成停靠条而不是浮层：diff 自己会滚，浮层得跟着算避让；停靠只占一条
// 40px，永远不挡内容，也没有「跑到屏幕外面」这回事。
//
// 不用 ui.FindBar：它的文案（"Find"、"No matches"、"N of M"）在框架里写死
// 成英文，从外面盖不掉 —— 和 searchField 不用 ui.SearchField 是同一条理由。
func (a *app) findBar(c *ui.Context, pal palette) {
	var in *ui.Element
	bar := ui.Row(c).Key("find-bar").Shrink(0).FillWidth().Height(findBarH).
		Padding(0, 12).Gap(6).AlignItems(ui.Center).
		Background(pal.surface1).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline)

	bar.Children(func() {
		ui.Box(c).Key("find-field").Width(findFieldW).Shrink(0).Children(func() {
			in = searchField(c, pal, &a.find.query, T("在差异中查找", "Find in diff"))
		})

		total := len(a.find.matches)
		if a.find.query != "" {
			text := T("无结果", "No results")
			if total > 0 {
				text = fmt.Sprintf("%d/%d", a.find.cur+1, total)
			}
			ui.Text(c, text).FontSize(textCaption).Font("monospace").
				FontFeatures("tnum").TextColor(pal.inkSubtle).SingleLine()
		}

		// 上/下一条。没有命中时是灰的，点了也不会动。
		off := total == 0
		if toolIconButton(c, pal, T("上一个", "Previous"), T("上一个", "Previous")+"  "+K("⇧⏎"), off, false, drawChevronUpIcon) {
			a.findNext(-1)
			a.find.focus = true
		}
		if toolIconButton(c, pal, T("下一个", "Next"), T("下一个", "Next")+"  "+K("⌘G"), off, false, drawChevronDownIcon) {
			a.findNext(1)
			a.find.focus = true
		}

		ui.Spacer(c)
		if xButton(c, pal, T("关闭查找", "Close find")) {
			a.closeFind()
		}
	})

	if a.find.focus {
		in.Focus()
		a.find.focus = false
	}
	// 栏里的按钮（清空、上/下一个、关闭）一按就把焦点夺走。用户点它们是
	// 为了接着打字，所以只要焦点还在这一栏里、却没落在输入框上，就送回来。
	// 焦点在别处（比如去 diff 里选字）时不管 —— 那是用户自己的选择。
	//
	// 与 on 相与：× 就地收栏之后这一栏已经没了，再把焦点交给它的输入框
	// 是交给一个下一帧就不存在的东西。
	if a.find.on && !in.Focused() && bar.FocusWithin() {
		in.Focus()
	}

	// ⇧⏎ 只能在这里拦。框架的编辑器把 Enter 一律当成「提交」，修饰键收在
	// 一个包内的 submitMods 里；Submitted() 对外只说「提交了」，不说是怎么
	// 提交的（框架自己的 FindBar 读的是那个包内字段，我们在包外读不到）。
	// HandleInput 拿到的键**早于**编辑器 —— 只有它看得见 ⇧。
	//
	// 这里不认的键一律返回 false，原样交还给编辑器：打字的路径一个字节
	// 都不经过我们。
	in.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && ev.Mods&ui.Shift != 0 {
			a.findNext(-1)
			return true
		}
		return false
	})
	// 无修饰的 ↵ 由 Submitted 报（框架只对没有修饰键的回车认真）。
	if in.Submitted() {
		a.findNext(1)
	}
	if in.Shortcut(0, ui.KeyEscape) {
		a.closeFind()
	}
	// 记给下一帧的 keys.go：裸 ↑/↓ 单行编辑框不认，会漏给窗口那层的
	// 文件/提交导航 —— 拿这道闸挡住。
	//
	// 与 on 相与：上面那条 Esc 就地收栏之后，这一帧的 in.Focused() 还是
	// true（输入框是这一帧建的），照着它记会把闸门永久锁死。
	a.find.focused = a.find.on && in.Focused()
}
