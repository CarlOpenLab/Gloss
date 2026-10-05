package main

import "github.com/egoist/mygo/ui"

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
	gapText   ui.Color // 未改动区间条的文字
	sideBg    ui.Color // 删除侧留白（split 模式下对面没有对应行）
	border    ui.Color
	ref       ui.Color // 分支名、commit hash 的强调色
	viewed    ui.Color // 已 review 的勾与文字
}

func paletteFor(t *ui.Theme) palette {
	if t.Dark {
		return palette{
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
			gapText:   ui.Hex("#8b949e"),
			sideBg:    ui.Hex("#11161d"),
			border:    ui.Hex("#30363d"),
			ref:       ui.Hex("#d29922"),
			viewed:    ui.Hex("#3fb950"),
		}
	}
	return palette{
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
		gapText:   ui.Hex("#6e7781"),
		sideBg:    ui.Hex("#fafbfc"),
		border:    ui.Hex("#d0d7de"),
		ref:       ui.Hex("#bc4c00"),
		viewed:    ui.Hex("#1a7f37"),
	}
}
