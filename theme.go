package main

import (
	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/highlight"
)

// palette 是 diff 视图专用的语义色。
//
// mygo 的 ui.Theme 已经管了通用控件（按钮、输入框、边框、文字），
// 但 diff 需要的是它没有的一层：行背景、行号栏底色、增删色条、未改动区间。
// 深浅两套跟随系统，取值贴近 Web 版那套 GitHub 配色，观感能对上。
type palette struct {
	appBg     ui.Color // 窗口底色
	codeBg    ui.Color // 普通代码行背景
	code      ui.Color // 代码文字
	lineNo    ui.Color // 行号文字
	gutterBg  ui.Color // 行号栏背景（上下文行）
	addBg     ui.Color // 新增行背景
	addGutter ui.Color // 新增行号栏背景
	addBar    ui.Color // 新增行左侧色条
	delBg     ui.Color // 删除行背景
	delGutter ui.Color // 删除行号栏背景
	delBar    ui.Color // 删除行左侧色条
	gapBg     ui.Color // 未改动区间条的背景
	gapHover  ui.Color // 未改动区间条悬停时的背景
	gapText   ui.Color // 未改动区间条的文字
	fileBg    ui.Color // 文件头的背景
	hunkBg    ui.Color // @@ 行背景
	hunkText  ui.Color // @@ 行文字
	sideBg    ui.Color // 删除侧留白（split 模式下对面没有对应行）
	border    ui.Color
	ref       ui.Color // 分支名、commit hash 的强调色
	viewed    ui.Color // 已 review 的勾与文字

	// lanes 是提交拓扑图里每条泳道的颜色。
	//
	// 用定长数组而不是切片：调色板每帧都要重建，切片意味着每帧一次分配。
	// 第 i 条泳道取 lanes[i%8] —— 按泳道号定色，不是按「第几次被占用」，
	// 这样同一列一路同色，而且每行都能独立算出来（列表是虚拟化的，行与行
	// 之间不能有共享状态）。
	lanes [8]ui.Color

	// hl 是代码上色用的配色，按明暗共享同一份缓存。
	hl *highlight.Palette
}

// laneColor 是第 i 条泳道的颜色。
func (p palette) laneColor(i int) ui.Color {
	return p.lanes[i%len(p.lanes)]
}

// hlPalettes 按明暗缓存两套高亮配色。
//
// 解析样式表（样式继承、token 类别合并）不便宜，而配色只跟主题有关，
// 缓存一份就够。只有渲染线程会碰它，所以不加锁。
var hlPalettes [2]*highlight.Palette

func hlPalette(dark bool) *highlight.Palette {
	i := 0
	if dark {
		i = 1
	}
	if hlPalettes[i] == nil {
		hlPalettes[i] = highlight.NewPalette(dark)
	}
	return hlPalettes[i]
}

func paletteFor(t *ui.Theme) palette {
	hl := hlPalette(t.Dark)
	if t.Dark {
		return palette{
			hl:        hl,
			appBg:     ui.Hex("#0d1117"),
			codeBg:    ui.Hex("#0d1117"),
			code:      ui.Hex("#e6edf3"),
			lineNo:    ui.Hex("#6e7681"),
			gutterBg:  ui.Hex("#0d1117"),
			addBg:     ui.Hex("#12261e"),
			addGutter: ui.Hex("#1b4721"),
			addBar:    ui.Hex("#3fb950"),
			delBg:     ui.Hex("#25171c"),
			delGutter: ui.Hex("#4c1f24"),
			delBar:    ui.Hex("#f85149"),
			gapBg:     ui.Hex("#161b22"),
			gapHover:  ui.Hex("#21262d"),
			gapText:   ui.Hex("#8b949e"),
			fileBg:    ui.Hex("#161b22"),
			hunkBg:    ui.Hex("#121d2f"),
			hunkText:  ui.Hex("#79c0ff"),
			sideBg:    ui.Hex("#11161d"),
			border:    ui.Hex("#30363d"),
			ref:       ui.Hex("#d29922"),
			viewed:    ui.Hex("#3fb950"),
			// 泳道色：八条线两两要能分开，也得在 #0d1117 上看得清
			lanes: [8]ui.Color{
				ui.Hex("#58a6ff"), // 蓝
				ui.Hex("#3fb950"), // 绿
				ui.Hex("#d29922"), // 琥珀
				ui.Hex("#bc8cff"), // 紫
				ui.Hex("#f85149"), // 红
				ui.Hex("#39c5cf"), // 青
				ui.Hex("#db61a2"), // 粉
				ui.Hex("#e3b341"), // 黄
			},
		}
	}
	return palette{
		hl:        hl,
		appBg:     ui.Hex("#ffffff"),
		codeBg:    ui.Hex("#ffffff"),
		code:      ui.Hex("#1f2328"),
		lineNo:    ui.Hex("#6e7781"),
		gutterBg:  ui.Hex("#ffffff"),
		addBg:     ui.Hex("#dafbe1"),
		addGutter: ui.Hex("#aceebb"),
		addBar:    ui.Hex("#1a7f37"),
		delBg:     ui.Hex("#ffebe9"),
		delGutter: ui.Hex("#ffd7d5"),
		delBar:    ui.Hex("#cf222e"),
		gapBg:     ui.Hex("#f6f8fa"),
		gapHover:  ui.Hex("#eaeef2"),
		gapText:   ui.Hex("#6e7781"),
		fileBg:    ui.Hex("#f6f8fa"),
		hunkBg:    ui.Hex("#ddf4ff"),
		hunkText:  ui.Hex("#0550ae"),
		sideBg:    ui.Hex("#fafbfc"),
		border:    ui.Hex("#d0d7de"),
		ref:       ui.Hex("#bc4c00"),
		viewed:    ui.Hex("#1a7f37"),
		// 同一套色相，压暗一档，在白底上才够看
		lanes: [8]ui.Color{
			ui.Hex("#0969da"),
			ui.Hex("#1a7f37"),
			ui.Hex("#9a6700"),
			ui.Hex("#8250df"),
			ui.Hex("#cf222e"),
			ui.Hex("#1b7c83"),
			ui.Hex("#bf3989"),
			ui.Hex("#7d4e00"),
		},
	}
}
