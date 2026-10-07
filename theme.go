package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/highlight"
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
	// explainHover 是「✨ 解释」按钮悬停时的底色。它落在 hunk 带上，
	// 所以不能直接用 codeBg：深色下那是比带子更暗的一档，鼠标一指上去
	// 按钮反而沉下去。浅色下要的是提亮（白），深色下要的是抬高一档的蓝灰。
	explainHover ui.Color
	sideBg       ui.Color // 删除侧留白（split 模式下对面没有对应行）
	border       ui.Color
	ref          ui.Color // 分支名、commit hash 的强调色
	viewed       ui.Color // 已 review 的勾与文字

	// lanes 是提交拓扑图里每条线的颜色。
	//
	// 用定长数组而不是切片：调色板每帧都要重建，切片意味着每帧一次分配。
	// 色号由 buildGraph 指给「线」（graphSeg.Color），不是泳道号 —— 一条线
	// 会换泳道，按泳道号定色的话主线走一半就变色了。
	lanes [laneCount]ui.Color

	// hl 是代码上色用的配色，按明暗共享同一份缓存。
	hl *highlight.Palette
}

// selAlpha 是「选中行」底色的不透明度。
//
// 侧栏的引用树、下半区的文件树、提交列表共用这一个值 —— 三处的选中态是同一
// 件事，不该有三种浓度。选中底色都是叠在自己那层的底色上，深浅主题一个值够用。
const selAlpha = 0.22

// laneCount 是拓扑图的色号数量，palette.lanes 的长度。graph.go 挑色号时按它
// 取模/找空位，两处必须一致。
const laneCount = 8

// laneColor 是第 i 号色。
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

// glossTheme 把 mygo 的通用控件配色换成 Gloss 自己那一套。
//
// 不设的话框架用的是 LightTheme/DarkTheme 的默认值：zinc 灰 + #2563eb 蓝，
// 和下面 palette 里这套 GitHub 色系是两种语言 —— 同一个窗口里，diff 是
// GitHub 的灰蓝，按钮和对话框却是另一套，边界上一眼看得出来「控件是别人家的」。
//
// 取值直接对齐 palette 里的语义色：Surface 就是 fileBg、SurfaceHover 就是
// gapHover、Border 就是 border。于是 diff 的文件头、面板底、按钮、输入框
// 全落在同一组灰阶上，深浅两套各自自洽。
//
// 每个值为什么是它：
//   - Accent 用 GitHub 的链接蓝而不是品牌的橙色：橙是 diff 里「元信息」
//     （引用名、状态字母）的颜色，选中态再用橙会和那些标记抢注意力。
//   - Focus 是半透明的：输入框的聚焦描边落在浅灰底上，实心蓝太抢眼，
//     半透明的一圈能指认焦点又不盖住内容。
//   - Scrollbar 比框架默认淡一档（0.32 → 0.22）：滚动条是随时在的常驻件，
//     默认那档在浅色长列表里像一条脏边。
func glossTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}

	if dark {
		// Background 是深色下的「浮层底」而不是画布底：画布由 pal.appBg
		// （#0d1117）自己铺，框架拿 Background 画的是对话框面板、弹出层、
		// 提示气泡这些**浮在内容之上**的东西。取 #161b22（GitHub 的 overlay
		// 色）：原先跟画布同色，深色对话框的面板边缘就消失了 —— 内容浮在
		// 一片被压暗的画布上，看不出卡片在哪儿。浅色那边 Background 是纯白，
		// 和画布同色反而正好（白卡片压在深色遮罩上）。
		t.Background = ui.Hex("#161b22")
		t.Surface = ui.Hex("#161b22")
		t.SurfaceHover = ui.Hex("#21262d")
		t.SurfacePressed = ui.Hex("#282e37")
		t.Border = ui.Hex("#30363d")
		t.Text = ui.Hex("#e6edf3")
		t.TextMuted = ui.Hex("#8b949e")
		t.Accent = ui.Hex("#1f6feb")
		t.AccentHover = ui.Hex("#388bfd")
		t.AccentPressed = ui.Hex("#1158c7")
		t.AccentText = ui.Hex("#ffffff")
		t.Danger = ui.Hex("#f85149")
		t.Warning = ui.Hex("#d29922")
		t.Success = ui.Hex("#3fb950")
		t.Selection = ui.RGBA(31, 111, 235, 0.45)
		t.Focus = ui.RGBA(88, 166, 255, 0.65)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.24)
		return t
	}

	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#f6f8fa")
	t.SurfaceHover = ui.Hex("#eaeef2")
	t.SurfacePressed = ui.Hex("#dfe3e8")
	t.Border = ui.Hex("#d0d7de")
	t.Text = ui.Hex("#1f2328")
	t.TextMuted = ui.Hex("#59636e")
	t.Accent = ui.Hex("#0969da")
	t.AccentHover = ui.Hex("#0860c4")
	t.AccentPressed = ui.Hex("#0550ae")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#cf222e")
	t.Warning = ui.Hex("#9a6700")
	t.Success = ui.Hex("#1a7f37")
	t.Selection = ui.RGBA(9, 105, 218, 0.25)
	t.Focus = ui.RGBA(9, 105, 218, 0.55)
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.22)
	return t
}

// themeBase 是两套（浅/深）没被改过的基准主题，themeLive 是每帧给出去的那份。
//
// 分成两份是因为 SetTheme 换掉默认主题之后，框架那套「跟着系统外观和偏好走」
// 也一并失效了（见 Theme.follow）：系统字号放大、提高对比度这些无障碍设置
// 得自己接过来。做法是每帧从基准拷一份、application 一遍 —— 幂等，
// 偏好变了下一帧就跟上，也不会每帧新建对象（控件的主题指针得稳住）。
var (
	themeBase [2]*ui.Theme
	themeLive [2]*ui.Theme
)

// themeFor 取这一帧要用的主题。
func themeFor(dark bool, p ui.Preferences) *ui.Theme {
	i := 0
	if dark {
		i = 1
	}
	if themeBase[i] == nil {
		themeBase[i] = glossTheme(dark)
		t := *themeBase[i]
		themeLive[i] = &t
	}
	b, t := themeBase[i], themeLive[i]
	*t = *b

	// 系统字号缩放。TextScale 为 1（默认）时等于基准值。
	if s := p.TextScale; s > 0 && s != 1 {
		t.FontSize = b.FontSize * s
	}
	// 提高对比度：和框架默认主题同一套做法 —— 边框和次要文字往正文色靠，
	// 焦点圈拉满不透明。只动这三样，diff 的语义色不碰。
	if p.HighContrast {
		t.Border = b.Border.Mix(b.Text, 0.45)
		t.TextMuted = b.TextMuted.Mix(b.Text, 0.4)
		t.Focus.A = 255
	}
	return t
}

func paletteFor(t *ui.Theme) palette {
	hl := hlPalette(t.Dark)
	if t.Dark {
		return palette{
			hl:           hl,
			appBg:        ui.Hex("#0d1117"),
			codeBg:       ui.Hex("#0d1117"),
			code:         ui.Hex("#e6edf3"),
			lineNo:       ui.Hex("#6e7681"),
			gutterBg:     ui.Hex("#0d1117"),
			addBg:        ui.Hex("#12261e"),
			addGutter:    ui.Hex("#1b4721"),
			addBar:       ui.Hex("#3fb950"),
			delBg:        ui.Hex("#25171c"),
			delGutter:    ui.Hex("#4c1f24"),
			delBar:       ui.Hex("#f85149"),
			gapBg:        ui.Hex("#161b22"),
			gapHover:     ui.Hex("#21262d"),
			gapText:      ui.Hex("#8b949e"),
			fileBg:       ui.Hex("#161b22"),
			hunkBg:       ui.Hex("#121d2f"),
			hunkText:     ui.Hex("#79c0ff"),
			explainHover: ui.Hex("#1b2b45"),
			sideBg:       ui.Hex("#11161d"),
			border:       ui.Hex("#30363d"),
			ref:          ui.Hex("#d29922"),
			viewed:       ui.Hex("#3fb950"),
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
		hl:           hl,
		appBg:        ui.Hex("#ffffff"),
		codeBg:       ui.Hex("#ffffff"),
		code:         ui.Hex("#1f2328"),
		lineNo:       ui.Hex("#6e7781"),
		gutterBg:     ui.Hex("#ffffff"),
		addBg:        ui.Hex("#dafbe1"),
		addGutter:    ui.Hex("#aceebb"),
		addBar:       ui.Hex("#1a7f37"),
		delBg:        ui.Hex("#ffebe9"),
		delGutter:    ui.Hex("#ffd7d5"),
		delBar:       ui.Hex("#cf222e"),
		gapBg:        ui.Hex("#f6f8fa"),
		gapHover:     ui.Hex("#eaeef2"),
		gapText:      ui.Hex("#6e7781"),
		fileBg:       ui.Hex("#f6f8fa"),
		hunkBg:       ui.Hex("#ddf4ff"),
		hunkText:     ui.Hex("#0550ae"),
		explainHover: ui.Hex("#ffffff"),
		sideBg:       ui.Hex("#fafbfc"),
		border:       ui.Hex("#d0d7de"),
		ref:          ui.Hex("#bc4c00"),
		viewed:       ui.Hex("#1a7f37"),
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
