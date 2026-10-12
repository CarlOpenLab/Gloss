package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/highlight"
)

// diff 视图的尺寸常量集中在这里，改字号时一起调。
const (
	barW     = 4    // 行左侧的增删色条宽度
	gutterW  = 46   // 单侧行号栏宽度
	gutterPx = 8    // 行号栏左右内边距
	codeGap  = 12   // 行号栏和代码之间的间距
	lineH    = 20   // 代码行高
	hunkH    = 26   // @@ 行高度
	fileH    = 36   // 文件头高度
	noteH    = 40   // 二进制/过大提示行的高度
	defSize  = 12.5 // 默认代码字号
)

// diffView 渲染整份 diff。
//
// 列表是虚拟化的：只构建可见的那些行，所以几万行的 diff 和几百行一样快，
// 前提是行高别做成动态的（同一帧里量到的高度会被复用）。
func (a *app) diffView(c *ui.Context, pal palette) {
	if a.rowsDirty {
		a.rebuildRows()
	}

	// 点了文件栏的那一下：行模型这时候才是新的，在这里滚才对得上。
	// 放在重建之后而不是点击处，是为了不管文件栏和 diff 谁先构建都对。
	if a.pendingScrollFile >= 0 {
		if i := a.fileHeaderRowOf(a.pendingScrollFile); i >= 0 {
			a.diffList.ScrollTo(i, ui.Start)
			// 手动跳走了，J/K 的出发点作废（和 hunkAnchor 一个道理）
			a.hunkCur = ""
		}
		a.pendingScrollFile = -1
	}

	// Key 用行的稳定标识而不是下标：展开一个未改动区间会让它后面所有行的
	// 下标整体后移，没有 Key 的话列表的滚动位置和行状态会跟着乱跳。
	a.diffList.Key = func(i int) any { return a.rows[i].key }
	// 文件头滚动时钉在顶部，一眼知道现在看的是哪个文件。
	a.diffList.Header = func(i int) bool { return a.rows[i].kind == rowFileHeader }

	// 只统计本帧构建的行。虚拟化生效时这个数和窗口高度成正比，
	// 和 diff 总行数无关。
	a.built = 0
	ui.List(c, &a.diffList, len(a.rows), func(i int) {
		a.built++
		a.diffRow(c, pal, a.rows[i])
	}).Grow(1)
}

// rebuildRows 重建扁平行模型。只在主线程、且不在列表构建过程中调用。
func (a *app) rebuildRows() {
	var collapsed map[string]bool
	for i := range a.files {
		if a.isFolded(a.files[i]) {
			if collapsed == nil {
				collapsed = map[string]bool{}
			}
			collapsed[fileKey(a.files[i])] = true
		}
	}
	a.rows = buildRows(a.files, a.contents, a.expanded, a.split, collapsed)
	if a.explainOpen() {
		a.rows = a.injectExplainRow(a.rows)
	}
	a.rowsDirty = false
}

// injectExplainRow 将当前处于打开状态的 AI 解释卡片作为 GitHub 风格的行间内嵌行（rowExplain）插入到行模型中。
func (a *app) injectExplainRow(rows []row) []row {
	if !a.explainOpen() {
		return rows
	}
	expRow := row{
		kind: rowExplain,
		file: a.ex.fi,
		hunk: a.ex.hi,
		key:  "ex:" + a.ex.key,
	}

	// 单行解释：卡片插在被解释那一行的正下方。
	//
	// 认 lineIdx 而不是行号：同一个 hunk 里同一行只出现一次，而行号会随刷新、
	// 切提交漂走。「那一行此刻不在行模型里」（被折叠的区间吃了、diff 换了）
	// 就退到这一段的 hunk header 底下 —— 仍然贴着正在讲的那一段。
	if a.ex.scope == scopeLine {
		for i := range rows {
			r := rows[i]
			if r.file == a.ex.fi && r.hunk == a.ex.hi &&
				r.kind == rowLine && r.lineIdx == a.ex.line.lineIdx {
				return injectRowAt(rows, i, expRow)
			}
		}
		if i := a.explainAnchorRow(rows); i >= 0 {
			return injectRowAt(rows, i, expRow)
		}
		return append(rows, expRow)
	}

	targetIdx := -1
	// Hunk 解释：插入在对应 hunk header 的紧下方
	for i, r := range rows {
		if r.file == a.ex.fi && r.kind == rowHunkHeader && r.hunk == a.ex.hi {
			targetIdx = i
			break
		}
	}

	if targetIdx >= 0 {
		return injectRowAt(rows, targetIdx, expRow)
	}

	// 兜底：插入在对应文件头后
	for i, r := range rows {
		if r.file == a.ex.fi && r.kind == rowFileHeader {
			return injectRowAt(rows, i, expRow)
		}
	}
	return append(rows, expRow)
}

// injectRowAt 把卡片插在 rows[i] 后面。
func injectRowAt(rows []row, i int, expRow row) []row {
	result := make([]row, 0, len(rows)+1)
	result = append(result, rows[:i+1]...)
	result = append(result, expRow)
	return append(result, rows[i+1:]...)
}

// explainAnchorRow 找单行解释的兜底落点：这一段 hunk header 的下标。
func (a *app) explainAnchorRow(rows []row) int {
	for i, r := range rows {
		if r.file == a.ex.fi && r.kind == rowHunkHeader && r.hunk == a.ex.hi {
			return i
		}
	}
	return -1
}

// fileHeaderRowOf 找某个文件的文件头在行模型里的下标，没有返回 -1。
//
// 每次点击扫一遍而不是维护一张对照表：行模型一变（展开未改动区间、折叠、
// 切视图）表就得跟着重建，多一份要保持同步的派生状态。扫描本身很快 ——
// 文件头按 file 下标单调递增，扫到更大的就能停。
func (a *app) fileHeaderRowOf(fi int) int {
	for i := range a.rows {
		if a.rows[i].kind != rowFileHeader {
			continue
		}
		if a.rows[i].file == fi {
			return i
		}
		if a.rows[i].file > fi {
			break
		}
	}
	return -1
}

// fileAt 取某个行所指向的文件。
func (a *app) fileAt(i int) *gitrepo.File {
	if i < 0 || i >= len(a.files) {
		return nil
	}
	return &a.files[i]
}

func (a *app) diffRow(c *ui.Context, pal palette, r row) {
	switch r.kind {
	case rowFileHeader:
		a.fileHeaderRow(c, pal, r)
	case rowSection:
		a.sectionRow(c, pal, r)
	case rowHunkHeader:
		a.hunkHeaderRow(c, pal, r)
	case rowGap:
		a.gapRow(c, pal, r)
	case rowNote:
		a.noteRow(c, pal, r)
	case rowConflict:
		a.conflictRow(c, pal, r)
	case rowExplain:
		a.explainInlineRow(c, pal, r)
	default:
		a.codeRow(c, pal, r)
	}
}

// sectionH 是「已暂存 / 未暂存」分隔行的高度。
const sectionH = 28

// sectionRow 是两段之间的分隔行。
//
// 字号、字重、颜色照抄侧栏的分组行（本地分支 / 远程 / 标签）：两处都是
// 「下面这一摞属于这里」，长得一样才不用重新认。右边跟上这一段有几个文件 ——
// 文件栏的栏头报的是总数，分段之后「各有多少」只有这里能看到。
//
// 两边都有内容时才画（buildRows 决定），只有一段的时候它不提供任何信息。
func (a *app) sectionRow(c *ui.Context, pal palette, r row) {
	t := c.Theme()
	n := 0
	sec := gitrepo.SectionNone
	if f := a.fileAt(r.file); f != nil {
		sec = f.Section
		for i := range a.files {
			if a.files[i].Section == sec {
				n++
			}
		}
	}

	ui.Row(c).Height(sectionH).Shrink(0).FillWidth().
		Padding(0, codeGap).Gap(8).AlignItems(ui.Center).
		Background(pal.gapBg).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		ui.Text(c, r.note).FontSize(11).Bold().SingleLine().TextColor(t.TextMuted)
		ui.Spacer(c)
		ui.Textf(c, "%d 个文件", n).FontSize(11).FontFeatures("tnum").
			TextColor(t.TextMuted)
	})
}

// chevronSize 是折叠三角的边长。和侧栏树里的那个一样大。
const chevronSize = 10

// foldChevron 画文件头前面的折叠三角。
//
// 用矢量画而不是打一个 ▾ 字符：这类几何符号很容易落到后备字体上，
// 各机器上大小和位置都不一样，还可能画成缺字方框。
//
// 走向由 drawChevron 统一给（侧栏树里的折叠箭头也是它），全窗口一个样子。
func foldChevron(c *ui.Context, open bool, color ui.Color) {
	ui.Box(c).Size(chevronSize, chevronSize).Shrink(0).
		Draw(func(p *ui.Painter, r ui.Rect) {
			drawChevron(p, r, color, open)
		})
}

// fileHeaderRow 是每个文件顶上的那条：路径 + 增删统计 + 已 review 勾选。
// 它是列表的 header，会钉在顶部。
//
// 点文件头空白处可以折叠/展开这个文件。已 review 的文件默认折起来，
// 一屏能扫完整个改动清单。
func (a *app) fileHeaderRow(c *ui.Context, pal palette, r row) {
	t := c.Theme()
	f := a.fileAt(r.file)
	if f == nil {
		return
	}
	viewed := a.isViewed(*f)
	folded := a.isFolded(*f)

	// 文件头用 surface2 底 + 上下两道边，和 hunk 带（surface2 但只有一道边）
	// 拉开层次。多个文件连续时，上下边线划出明确边界，一眼看出哪些 hunk 属于
	// 哪个文件。原来的 fileBg 和 codeBg 几乎同色，文件之间没有视觉分隔。
	row := ui.Row(c).Height(fileH).Shrink(0).FillWidth().
		Padding(0, 12).Gap(10).AlignItems(ui.Center).
		Background(pal.surface2).Cursor(ui.CursorPointer).
		Border(1, pal.hairline)
	row.Children(func() {
		foldChevron(c, !folded, t.TextMuted)

		name := t.Text
		if viewed {
			name = t.TextMuted
		}
		ui.Text(c, displayPath(*f)).Font("monospace").FontSize(12).Grow(1).
			SingleLine().TextColor(name)

		if folded {
			ui.Text(c, T("已折叠", "Folded")).FontSize(11).TextColor(t.TextMuted)
		}
		if f.AddCount > 0 {
			ui.Textf(c, "+%d", f.AddCount).Font("monospace").FontSize(11).Bold().
				FontFeatures("tnum").TextColor(pal.addBar)
		}
		if f.DelCount > 0 {
			ui.Textf(c, "−%d", f.DelCount).Font("monospace").FontSize(11).Bold().
				FontFeatures("tnum").TextColor(pal.delBar)
		}

		if f.Section == gitrepo.SectionStaged {
			ui.Text(c, T("已暂存", "Staged")).FontSize(11).TextColor(t.TextMuted)
		}

		// 整文件暂存/取消暂存：带文字的按钮样式，和 hunk 级的 stageHunkButton
		// 保持一致。裸 checkbox 在 diff 里没有上下文，用户不知道它是干嘛的。
		if a.tab == 0 && !a.compare.on && f.Section != gitrepo.SectionNone {
			stageBtn := stageFileButton(c, pal, stageStateOf(*f), a.op != gitrepo.OpNone)
			if stageBtn.Clicked() {
				a.toggleStageFile(*f)
			}
		}

		// Checkbox 要的是 *bool，而 viewed 是按指纹存在 map 里的。
		// 传一份副本进去，变了再回写 —— 中间不碰真实状态。
		v := viewed
		ui.Checkbox(c, &v, T("已 review", "Reviewed"))
		if v != viewed {
			a.toggleViewed(*f)
		}
	})
	// Checkbox 是行内的可点元素，命中的是它自己 —— 点勾选框不会连带折叠。
	if row.Clicked() {
		a.toggleFold(*f)
	}
}

// hunkHeaderRow 是 @@ 那一行。左边是原始的 hunk 头，中间跟着 git 抓出来的
// 函数名（Section），最右边是「暂存这一段」和「✨ 解释」。
func (a *app) hunkHeaderRow(c *ui.Context, pal palette, r row) {
	f := a.fileAt(r.file)
	if f == nil || r.hunk >= len(f.Hunks) {
		return
	}
	h := f.Hunks[r.hunk]

	// hunk 带按 §2.5 来：Surface 2 底 + 1px hairline 边 + Ink Subtle 等宽文字。
	// 原来只有底色没有边 —— 带子上下都挨着代码行，缺这道线时它在视觉上和
	// 上下文行糊在一起，看不出「这一段的头在这儿」。
	//
	// 文字用 mono(12) 而不是 11：hunk 头是给人读行号的（`@@ -12,7 +12,9 @@`），
	// 10 和 11 那两档是给元信息用的字号，读一串数字偏挤。
	ui.Row(c).Height(hunkH).Shrink(0).FillWidth().Padding(0, codeGap).Gap(12).
		AlignItems(ui.Center).Background(pal.hunkBg).
		Border(1, pal.hairline).Children(func() {
		ui.Text(c, hunkHeaderText(h)).Font("monospace").FontSize(textMono).SingleLine().
			Shrink(3).TextColor(pal.hunkText)
		if h.Section != "" {
			ui.Text(c, h.Section).Font("monospace").FontSize(textMono).SingleLine().
				Grow(1).MinWidth(0).TextColor(pal.hunkText).Opacity(0.7)
		} else {
			ui.Spacer(c)
		}

		// 按块暂存。不画在不能按块暂存的文件上（未跟踪、冲突、重命名、
		// 二进制）：那些文件只有「整个暂存」一条路，一颗永远灰着的按钮
		// 只会让人猜它为什么点不动。文件栏里的整文件勾选框管着它们。
		if a.hunkStageable(*f) {
			staged := f.Section == gitrepo.SectionStaged
			if stageHunkButton(c, pal, staged, a.op != gitrepo.OpNone).Clicked() {
				a.toggleStageHunk(r.file, r.hunk)
			}
		}

		// 按钮的选中态取点击**之前**的状态：刚点开的那一帧，按钮下一刻
		// 才该变成「正在讲」，否则点下去的同时按钮先变色、卡片才跟上，
		// 两件事分在两帧上闪。
		//
		// 卡片讲的是单行时这颗不亮：亮着会让人以为整段都在讲，而这一趟
		// 其实只讲了其中一行。真要讲整段，再点一下 ✨ 就是。
		btn := explainButton(c, pal, a.isExplaining(r.file, r.hunk) && a.ex.scope != scopeLine)
		if btn.Clicked() {
			a.toggleExplain(r.file, r.hunk)
		}
		// 但锚点必须问点击**之后**的状态：这一帧刚点开时，卡片要靠同一个
		// 按钮的位置来定位，顺序反了第一帧就会摆在左上角。
		if a.isExplaining(r.file, r.hunk) {
			a.ex.anchor = btn.Bounds()
		}
	})
}

// hunkHeaderText 取 @@ 行里不含函数名的那一段。
//
// git 把函数名直接接在第二个 @@ 后面，而我们把函数名单独画在右边 ——
// 不切掉的话同一句话会连着出现两遍。
func hunkHeaderText(h diff.Hunk) string {
	s := h.Header
	if h.Section != "" {
		s = strings.TrimSuffix(s, h.Section)
	}
	return strings.TrimRight(s, " \t")
}

// gapRow 是「⋯ N 行未改动」那条，点一下展开一段。
func (a *app) gapRow(c *ui.Context, pal palette, r row) {
	bg := pal.gapBg
	e := ui.Box(c).Height(lineH).Shrink(0).FillWidth().
		Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if e.Hovered() {
		bg = pal.gapHover
	}
	if e.Clicked() {
		a.expandGap(r)
	}
	e.Background(bg)
	fadeColors(e)
	e.Children(func() {
		ui.Textf(c, "⋯  展开 %d 行未改动", r.gapN).FontSize(11).TextColor(pal.gapText)
	})
}

// noteRow 是没有 hunk 的文件（二进制、过大）的说明行。
func (a *app) noteRow(c *ui.Context, pal palette, r row) {
	t := c.Theme()
	ui.Box(c).Height(noteH).Shrink(0).FillWidth().Padding(0, codeGap+barW).
		Justify(ui.Center).Children(func() {
		ui.Text(c, r.note).FontSize(12).TextColor(t.TextMuted)
	})
}

// codeRow 画一行代码：unified 是一条行占满宽度，split 是左右对照。
func (a *app) codeRow(c *ui.Context, pal palette, r row) {
	if a.split {
		a.splitRow(c, pal, r)
		return
	}
	a.unifiedRow(c, pal, r)
}

func (a *app) unifiedRow(c *ui.Context, pal palette, r row) {
	f := a.fileAt(r.file)
	if f == nil {
		return
	}

	var (
		text   string
		oldNo  int
		newNo  int
		lineBg = pal.codeBg
		gutBg  = pal.gutterBg
		bar    = ui.Transparent
		// 这一行该去哪份高亮里查：新侧的行看新文件，删除的行看还原出来的旧文件。
		hlFile *highlight.File
		old    bool
	)
	switch {
	case r.fromGap:
		// 展开出来的未改动行：内容不在 patch 里，按行号去文件内容里取。
		oldNo, newNo = r.oldNo, r.newNo
		text = a.contentLine(*f, r.newNo)
	case r.hunk < len(f.Hunks) && r.lineIdx < len(f.Hunks[r.hunk].Lines):
		l := f.Hunks[r.hunk].Lines[r.lineIdx]
		oldNo, newNo, text = l.OldNum, l.NewNum, l.Content
		switch l.Type {
		case diff.LineAdd:
			lineBg, gutBg, bar = pal.addBg, pal.addGutter, pal.addBar
		case diff.LineDel:
			lineBg, gutBg, bar = pal.delBg, pal.delGutter, pal.delBar
			old = true
		}
	default:
		// 文件列表换了但 rows 还没重建，这一帧先不画。
		return
	}
	newHL, oldHL := a.highlightFor(*f)
	if old {
		// 删除行在新侧没有行号，行号栏右边留空，高亮改查旧文件
		hlFile, newNo = oldHL, 0
	} else {
		hlFile = newHL
	}

	// 这一行正在被单独解释：给它描一道边。只描边不改底色 —— 增删行本来
	// 就各有一条色带，再叠底色就看不出是增还是删了（我的记忆里那条
	// 「同一信息不摆两遍」在这里也适用：边线已经说清「在讲这一行」）。
	marked := a.isExplainingLine(r.file, r.hunk, r.lineIdx)

	size := a.codeSizeValue()
	// 高亮是按展开 tab 之后的文本切的，这里必须做同样的展开，
	// 否则字节下标对不上，颜色会整段错位。
	text = expandTabs(text)
	targetLineNo := lineNoOf(r, oldNo, newNo)

	rowEl := ui.Row(c).Height(lineH).Shrink(0).FillWidth().Background(lineBg).Cursor(ui.CursorText)
	if marked {
		rowEl.Border(1, pal.explainHover)
	}
	a.lineExplainHit(rowEl, r)
	rowEl.Children(func() {
		ui.Box(c).Width(barW).AlignSelf(ui.Stretch).Background(bar)
		a.gutter(c, pal, oldNo, gutBg)
		a.gutter(c, pal, newNo, gutBg)
		a.code(c, pal, hlFile, targetLineNo, text, size)
	})
}

// lineExplainHit 给一行代码接上「双击只看这一行」。
//
// 双击而不是单击、也不是悬停弹小按钮：单击已经被「选中这一行」占着
// （文件栏点一下就读 diff），悬停按钮在每一行上都会出现，整屏都是闪光。
// 双击是显式的「我要细看这一行」，和侧栏分支、Gist 也是同一个套路。
//
// 只接 hunk 里的行（r.hunk >= 0），不接展开出来的未改动行：后者内容不在
// patch 里，没有「这一行改了什么」可讲 —— 它压根没改。
func (a *app) lineExplainHit(el *ui.Element, r row) {
	if r.kind != rowLine || r.hunk < 0 || r.lineIdx < 0 {
		return
	}
	if !el.DoubleClicked() {
		return
	}
	a.toggleExplainLine(r.file, r.hunk, r.lineIdx)
}

// sideLine 是 split 模式下某一侧的半行。empty 表示这一侧空着 ——
// 对面有新增或删除，这里留白。
type sideLine struct {
	empty bool
	text  string
	kind  diff.LineType
	no    int             // 这一侧的行号
	hl    *highlight.File // 该去哪份高亮里查
}

func (a *app) splitRow(c *ui.Context, pal palette, r row) {
	f := a.fileAt(r.file)
	if f == nil {
		return
	}
	left, right := a.splitSides(*f, r)
	if left.empty && right.empty {
		// 文件列表换了但 rows 还没重建，这一帧先不画。
		return
	}

	size := a.codeSizeValue()
	ui.Row(c).Height(lineH).Shrink(0).FillWidth().Children(func() {
		a.splitHalf(c, pal, left, size, r)
		ui.Box(c).Width(1).AlignSelf(ui.Stretch).Background(pal.hairline)
		a.splitHalf(c, pal, right, size, r)
	})
}

// splitHalf 画半行：色条 + 行号 + 代码。左右两半除了内容来源不同，
// 结构完全一样。
func (a *app) splitHalf(c *ui.Context, pal palette, s sideLine, size float32, r row) {
	bg, gutBg, bar := pal.codeBg, pal.gutterBg, ui.Transparent
	switch s.kind {
	case diff.LineAdd:
		bg, gutBg, bar = pal.addBg, pal.addGutter, pal.addBar
	case diff.LineDel:
		bg, gutBg, bar = pal.delBg, pal.delGutter, pal.delBar
	}
	if s.empty {
		// 对面多出来的行：这一侧整片留白，一眼能看出是增还是删
		bg, gutBg = pal.sideBg, pal.sideBg
	}

	half := ui.Row(c).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).Background(bg).Cursor(ui.CursorText)
	if !s.empty {
		// 只有这一侧真有内容时才接双击 —— 空白那半接上去，点它就会
		// 解释对面那一行，而眼睛看的是「这边没东西」。
		a.lineExplainHit(half, r)
	}
	half.Children(func() {
		ui.Box(c).Width(barW).AlignSelf(ui.Stretch).Background(bar)
		a.gutter(c, pal, s.no, gutBg)
		if s.empty {
			ui.Spacer(c)
			return
		}
		a.code(c, pal, s.hl, s.no, s.text, size)
	})
}

// splitSides 返回一行在左右两侧的内容。
func (a *app) splitSides(f gitrepo.File, r row) (left, right sideLine) {
	if r.fromGap {
		// 未改动区间在两侧是同一行，只是行号不同
		newHL, oldHL := a.highlightFor(f)
		text := expandTabs(a.contentLine(f, r.newNo))
		return sideLine{text: text, kind: diff.LineContext, no: r.oldNo, hl: oldHL},
			sideLine{text: text, kind: diff.LineContext, no: r.newNo, hl: newHL}
	}
	return a.sideOf(f, r.lhunk, r.lidx, true), a.sideOf(f, r.hunk, r.lineIdx, false)
}

// sideOf 取 hunk 里第 idx 行在某一侧的样子。old 为 true 是旧侧。
// idx 越界或那一侧本来就没有这一行（新增行在旧侧不存在）时返回空。
func (a *app) sideOf(f gitrepo.File, hunk, idx int, old bool) sideLine {
	if idx < 0 || hunk < 0 || hunk >= len(f.Hunks) || idx >= len(f.Hunks[hunk].Lines) {
		return sideLine{empty: true}
	}
	l := f.Hunks[hunk].Lines[idx]
	newHL, oldHL := a.highlightFor(f)
	s := sideLine{text: l.Content, kind: l.Type, hl: newHL, no: l.NewNum}
	if old {
		s.hl, s.no = oldHL, l.OldNum
	}
	if s.no == 0 {
		return sideLine{empty: true}
	}
	s.text = expandTabs(s.text)
	return s
}

// code 是半行/整行里代码那一块：可伸缩、横向裁掉超出的部分。
// 支持鼠标悬停 Tooltip 显示完整代码，避免单行超长被省略截断时看不到全貌。
func (a *app) code(c *ui.Context, pal palette, hl *highlight.File, lineNo int, text string, size float32) {
	b := ui.Box(c).Grow(1).MinWidth(0).ClipX().Padding(0, 0, 0, codeGap).
		Justify(ui.Center)
	if strings.TrimSpace(text) != "" {
		b.Tooltip(text)
	}
	b.Children(func() {
		ui.RichText(c, lineSpans(pal, hl, lineNo, text)...).
			Font("monospace").FontSize(size).SingleLine().TextColor(pal.code).Selectable()
	})
}

// lineNoOf 是这一行在对应那一侧的行号：高亮就是按这个行号取片段的。
func lineNoOf(r row, oldNo, newNo int) int {
	if r.fromGap {
		return r.newNo
	}
	if newNo > 0 {
		return newNo
	}
	return oldNo
}

// lineSpans 把一行切成带颜色的片段。没有高亮（认不出后缀、文件太大、
// 内容还没读到、行号对不上）时整行一个片段，走默认前景色。
//
// 片段必须严丝合缝地铺满整行 —— RichText 按片段依次排版，多一段少一段
// 都会让画面和实际内容对不上，所以空隙和越界都在这里补齐。
func lineSpans(pal palette, hl *highlight.File, lineNo int, text string) []ui.Span {
	plain := []ui.Span{{Text: text, Color: pal.code}}
	if hl.Plain() {
		return plain
	}
	segs := hl.Line(lineNo)
	if len(segs) == 0 {
		return plain
	}

	out := make([]ui.Span, 0, len(segs)+1)
	col := 0
	rich := false
	for _, s := range segs {
		if s.Start < col || s.End > len(text) {
			// 内容和切分对不上（文件在读到之后又被改了之类）。
			// 整行退回纯文本：宁可不好看，也别把颜色涂到别的字上。
			return plain
		}
		if s.Start > col {
			out = append(out, ui.Span{Text: text[col:s.Start], Color: pal.code})
		}
		e := pal.hl.Entry(s)
		col = s.End
		if e.Hex == "" {
			out = append(out, ui.Span{Text: text[s.Start:s.End], Color: pal.code})
			continue
		}
		rich = true
		sp := ui.Span{
			Text:   text[s.Start:s.End],
			Color:  ui.Hex(e.Hex),
			Italic: e.Italic,
		}
		if e.Bold {
			sp.Weight = 700
		}
		out = append(out, sp)
	}
	if col < len(text) {
		out = append(out, ui.Span{Text: text[col:], Color: pal.code})
	}
	if !rich {
		return plain
	}
	return out
}

// highlightFor 取某个文件两侧的高亮切分。内容还没读到就都是 nil，
// 渲染时按纯文本处理。
func (a *app) highlightFor(f gitrepo.File) (newHL, oldHL *highlight.File) {
	cc := a.contents[fileKey(f)]
	if cc == nil {
		return nil, nil
	}
	return cc.hl, cc.oldHL
}

// gutter 画一侧的行号。对面不存在的那一侧留空 —— 空白本身就是信息。
func (a *app) gutter(c *ui.Context, pal palette, no int, bg ui.Color) {
	label := ""
	if no > 0 {
		label = strconv.Itoa(no)
	}
	ui.Box(c).Width(gutterW).AlignSelf(ui.Stretch).Background(bg).
		Justify(ui.Center).Padding(0, gutterPx).Children(func() {
		ui.Text(c, label).Font("monospace").FontSize(11).FontFeatures("tnum").
			SingleLine().Width(gutterW - 2*gutterPx).TextAlign(ui.End).
			TextColor(pal.lineNo)
	})
}

// contentLine 取新侧第 n 行（1 起）的内容。内容没加载到就返回空串，
// 行号仍然照常显示。
func (a *app) contentLine(f gitrepo.File, n int) string {
	cc := a.contents[fileKey(f)]
	if cc == nil || n < 1 || n > len(cc.lines) {
		return ""
	}
	return cc.lines[n-1]
}

// expandGap 把某个未改动区间再放出来一段。
//
// 这里只改状态、把 rows 标脏，不立刻重建 —— 重建会换掉 a.rows，
// 而当前这一帧的列表正拿着它按行号取数据。
func (a *app) expandGap(r row) {
	if a.expanded == nil {
		a.expanded = map[string]int{}
	}
	a.expanded[r.gapKey] += expandStep
	a.rowsDirty = true
}

// displayPath 是文件头上显示的路径。删除的文件没有新路径，退回旧的。
func displayPath(f gitrepo.File) string {
	p := f.NewPath
	if p == "" {
		p = f.OldPath
	}
	if f.OldPath != "" && f.NewPath != "" && f.OldPath != f.NewPath {
		return f.OldPath + " → " + f.NewPath
	}
	return p
}

// tabWidth 是 tab 展开后的列宽。等宽字体下 4 列和大多数编辑器的默认值一致。
const tabWidth = 4

// expandTabs 把 tab 按列对齐展开成空格。
//
// mygo 的文本引擎不处理 \t（会画成缺字方框），而且直接用空格替换不对齐，
// 所以这里按当前列数补齐。传入的字符串可能含中日韩字符，它们的显示宽度是
// 两列 —— 只按 rune 计数会让这类文件的缩进对不齐。
func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += runeCols(r)
	}
	return b.String()
}

// runeCols 是字符占的显示列数：宽字符（CJK、全角标点）占两列。
func runeCols(r rune) int {
	switch {
	case r >= 0x1100 && r <= 0x115F, // 韩文字母
		r >= 0x2E80 && r <= 0xA4CF, // CJK 部首、假名、汉字
		r >= 0xAC00 && r <= 0xD7A3, // 韩文音节
		r >= 0xF900 && r <= 0xFAFF, // CJK 兼容汉字
		r >= 0xFE30 && r <= 0xFE6F, // CJK 兼容形式
		r >= 0xFF00 && r <= 0xFF60, // 全角形式
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x20000 && r <= 0x3FFFD: // 扩展区汉字
		return 2
	}
	return 1
}
