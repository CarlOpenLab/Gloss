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

// glossTheme 把 mygo 的通用控件配色换成 Gloss 对齐 Linear 设计体系的那一套。
//
// 参见 DESIGN.md：以 #010102 深邃画布为基准，采用四级表面明度阶梯（Surface 1~4）、
// 1px Hairline 发丝边框（#23252a），以及 Linear 标志性的薰衣草蓝（#5e6ad2）作为唯一主色。
func glossTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}

	if dark {
		// Linear 规范深色系统：
		// 画布底色由 pal.appBg (#010102) 铺陈，
		// t.Background 是浮层/弹窗底（Linear Surface 2 #141516 或 Surface 3 #18191a）。
		// Surface 阶梯：Surface 1 (#0f1011)，SurfaceHover (#141516)，SurfacePressed (#18191a)。
		// 边框采用 Linear 1px Hairline (#23252a)。
		// 文本采用 Ink 灰阶：#f7f8f8 (正文), #8a8f98 (次要)。
		// 单一主色 Accent：Linear 薰衣草蓝 #5e6ad2。
		t.Background = ui.Hex("#141516")
		t.Surface = ui.Hex("#0f1011")
		t.SurfaceHover = ui.Hex("#141516")
		t.SurfacePressed = ui.Hex("#18191a")
		t.Border = ui.Hex("#23252a")
		t.Text = ui.Hex("#f7f8f8")
		t.TextMuted = ui.Hex("#8a8f98")
		t.Accent = ui.Hex("#5e6ad2")
		t.AccentHover = ui.Hex("#828fff")
		t.AccentPressed = ui.Hex("#5e69d1")
		t.AccentText = ui.Hex("#ffffff")
		t.Danger = ui.Hex("#e5484d")
		t.Warning = ui.Hex("#e09f3e")
		t.Success = ui.Hex("#27a644")
		t.Selection = ui.RGBA(94, 106, 210, 0.22)
		t.Focus = ui.RGBA(94, 106, 210, 0.50)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.16)
		return t
	}

	// 浅色模式对齐 Linear Inverse 规范
	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#f7f8f9")
	t.SurfaceHover = ui.Hex("#f0f2f5")
	t.SurfacePressed = ui.Hex("#e4e7eb")
	t.Border = ui.Hex("#e1e4e8")
	t.Text = ui.Hex("#0f1115")
	t.TextMuted = ui.Hex("#6e7781")
	t.Accent = ui.Hex("#5e6ad2")
	t.AccentHover = ui.Hex("#4f5bc4")
	t.AccentPressed = ui.Hex("#414ca8")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#e5484d")
	t.Warning = ui.Hex("#d97706")
	t.Success = ui.Hex("#27a644")
	t.Selection = ui.RGBA(94, 106, 210, 0.18)
	t.Focus = ui.RGBA(94, 106, 210, 0.45)
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.18)
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
			appBg:        ui.Hex("#010102"),           // Linear Canvas: 极致纯净深黑
			codeBg:       ui.Hex("#010102"),
			code:         ui.Hex("#f7f8f8"),           // Linear Ink
			lineNo:       ui.Hex("#62666d"),           // Linear Ink Tertiary
			gutterBg:     ui.Hex("#010102"),
			addBg:        ui.RGBA(39, 166, 68, 0.09),  // 极淡半透明暗苔绿
			addGutter:    ui.RGBA(39, 166, 68, 0.14),
			addBar:       ui.Hex("#27a644"),           // Linear 语义绿发丝条
			delBg:        ui.RGBA(229, 72, 77, 0.09),  // 极淡半透明暗绯红
			delGutter:    ui.RGBA(229, 72, 77, 0.14),
			delBar:       ui.Hex("#e5484d"),           // 校准红发丝条
			gapBg:        ui.Hex("#0f1011"),           // Surface 1
			gapHover:     ui.Hex("#141516"),           // Surface 2
			gapText:      ui.Hex("#8a8f98"),           // Ink Subtle
			fileBg:       ui.Hex("#0f1011"),           // Surface 1
			hunkBg:       ui.Hex("#141516"),           // Surface 2: 告别旧版刺眼亮蓝！
			hunkText:     ui.Hex("#8a8f98"),           // Ink Subtle 等宽中性文字
			explainHover: ui.Hex("#18191a"),           // Surface 3
			sideBg:       ui.Hex("#08090a"),           // 介于 Canvas 与 Surface 1 之间
			border:       ui.Hex("#23252a"),           // Linear 1px Hairline
			ref:          ui.Hex("#d0d6e0"),           // 中性 Ink Muted
			viewed:       ui.Hex("#27a644"),
			// 拓扑图莫兰迪/精密仪器 8 色谱系（明度与彩度均衡）：
			lanes: [8]ui.Color{
				ui.Hex("#5e6ad2"), // 主干/HEAD: Linear 薰衣草蓝
				ui.Hex("#3eb489"), // 鼠尾草绿
				ui.Hex("#e09f3e"), // 暖琥珀
				ui.Hex("#9d8df1"), // 柔鸢尾紫
				ui.Hex("#e5484d"), // 珊瑚红
				ui.Hex("#45b6fe"), // 天青蓝
				ui.Hex("#d977a8"), // 暮灰粉
				ui.Hex("#38b2ac"), // 暗青
			},
		}
	}
	return palette{
		hl:           hl,
		appBg:        ui.Hex("#ffffff"),
		codeBg:       ui.Hex("#ffffff"),
		code:         ui.Hex("#0f1115"),
		lineNo:       ui.Hex("#8c959f"),
		gutterBg:     ui.Hex("#ffffff"),
		addBg:        ui.RGBA(39, 166, 68, 0.10),
		addGutter:    ui.RGBA(39, 166, 68, 0.16),
		addBar:       ui.Hex("#27a644"),
		delBg:        ui.RGBA(229, 72, 77, 0.10),
		delGutter:    ui.RGBA(229, 72, 77, 0.16),
		delBar:       ui.Hex("#e5484d"),
		gapBg:        ui.Hex("#f7f8f9"),
		gapHover:     ui.Hex("#f0f2f5"),
		gapText:      ui.Hex("#6e7781"),
		fileBg:       ui.Hex("#f7f8f9"),
		hunkBg:       ui.Hex("#f0f2f5"),
		hunkText:     ui.Hex("#4b535d"),
		explainHover: ui.Hex("#ffffff"),
		sideBg:       ui.Hex("#f7f8f9"),
		border:       ui.Hex("#e1e4e8"),
		ref:          ui.Hex("#4b535d"),
		viewed:       ui.Hex("#27a644"),
		lanes: [8]ui.Color{
			ui.Hex("#5e6ad2"),
			ui.Hex("#2d9d74"),
			ui.Hex("#c9822b"),
			ui.Hex("#7e6cd8"),
			ui.Hex("#cf3b40"),
			ui.Hex("#2693db"),
			ui.Hex("#b85487"),
			ui.Hex("#288c87"),
		},
	}
}
