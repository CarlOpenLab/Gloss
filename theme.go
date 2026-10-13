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
	// 表面阶梯与发丝线、灰阶（§2.1~2.3）。
	//
	// 这四档以前是拿 t.Surface / t.Background 凑的：mygo 的主题只有「控件面」
	// 和「浮层底」两个语义，而 Linear 这套要的是四级明度阶梯，还得能指名道姓
	// 地说「这是 Surface 3」。凑出来的后果是层级错位 —— popover 用 Surface 1、
	// 模态和卡片都挤在 Surface 2，深度全靠 24~36px 的大投影撑，正是 §1.3
	// 「明度阶梯代替阴影」要消除的做法。
	//
	// 有了这四个字段，组件直接写「我是第几级」，深度由明度表达。
	surface1 ui.Color // 侧栏、文件列表、提交消息面板、主要容器
	surface2 ui.Color // 卡片、hunk 栏、列表悬停态、徽章底
	surface3 ui.Color // 浮层、下拉菜单、右键菜单
	surface4 ui.Color // 模态弹窗、设置窗口顶层面板

	hairline         ui.Color // 基础 1px 细线（§2.2）
	hairlineStrong   ui.Color // 输入框外框、激活态卡片
	hairlineTertiary ui.Color // 悬停态边框、弹窗外围

	ink         ui.Color // 高亮标题、正文基准（§2.3）
	inkMuted    ui.Color // 常规正文、文件名、按钮文字
	inkSubtle   ui.Color // 行号、辅助元信息、未激活图标
	inkTertiary ui.Color // 快捷键提示、占位符、禁用态

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
	// hunkHover 是 hunk 行上那颗幽灵按钮悬停时的底色。它落在 hunk 带上，
	// 所以不能直接用 codeBg：深色下那是比带子更暗的一档，鼠标一指上去
	// 按钮反而沉下去。浅色下要的是提亮（白），深色下要的是抬高一档的蓝灰。
	hunkHover ui.Color
	sideBg    ui.Color // 删除侧留白（split 模式下对面没有对应行）
	ref       ui.Color // 分支名、commit hash 的强调色
	viewed    ui.Color // 已 review 的勾与文字

	// —— 查找（find.go）——
	// 命中底色用半透明琥珀：它要叠在 codeBg / 新增行 / 删除行三种底上，
	// 半透明才能混出「这一行还是绿的，但这段字被框住了」——不透明的色块会把
	// 增删信息整个盖掉。当前命中更重一档，另加一道下划线（只靠深浅不够快，
	// 连按「下一个」时视线得能立刻钉住新落点）。
	findBg        ui.Color // 全部命中的底色
	findBgActive  ui.Color // 当前命中的底色
	findUnderline ui.Color // 当前命中的下划线

	// selection 是列表与树的选中底带。
	//
	// 取中性灰阶而不是拿 Accent 淡染：Accent 是「白」（见 glossTheme），
	// 拿它淡染出来的底带是一层雾，把行糊住不说，§5 的强调预算也立刻超支 ——
	// 侧栏三个列表（refs 树、文件树、提交记录）各有一条选中带，再把主按钮
	// 和导航指示条算上，全屏同时亮着的实心强调就有五六处。选中位置改由
	// 中性底带表达之后，两极的 Ink 只落在真正要引导视线的那几处。
	selection ui.Color

	// lanes 是提交拓扑图里每条线的颜色。
	//
	// 用定长数组而不是切片：调色板每帧都要重建，切片意味着每帧一次分配。
	// 色号由 buildGraph 指给「线」（graphSeg.Color），不是泳道号 —— 一条线
	// 会换泳道，按泳道号定色的话主线走一半就变色了。
	lanes [laneCount]ui.Color

	// hl 是代码上色用的配色，按明暗共享同一份缓存。
	hl *highlight.Palette
}

// selAlpha 是「选中行」底色的不透明度 —— 现在只剩高对比度模式下叠一层用。
//
// 常规的选中底带走 palette.selection（中性灰阶），不再拿 Accent 淡染，
// 理由见那个字段的注释。
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
// 1px Hairline 发丝边框（#23252a），强调色是黑白两极的 Ink —— 深色下白键黑字，
// 浅色下黑键白字。整套界面不含色相，彩色只留给语义。
func glossTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}

	if dark {
		// Linear 规范深色系统：
		// 画布底色由 pal.appBg (#010102) 铺陈；
		// t.Background 是浮层/弹窗底，即 Surface 3 (#18191a) —— 规范把
		// 「浮层、下拉菜单、右键菜单」都放在这一级（§2.1），而框架自绘的
		// 右键菜单、tooltip、toast 读的正是它，改这一处就等于把三样一起摆正。
		// Surface 阶梯：Surface 1 (#0f1011)，SurfaceHover (#141516)，SurfacePressed (#18191a)。
		// 边框采用 Linear 1px Hairline (#23252a)。
		// 文本采用 Ink 灰阶：#f7f8f8 (正文), #8a8f98 (次要)。
		//
		// 强调色是 **白**（Ink #f7f8f8），不是某个品牌彩 —— 主按钮就是一颗
		// 白键、上面写黑字，和 Linear 现版一致。深浅两侧互为反相：深色下白键
		// 黑字，浅色下黑键白字。彩度全部让给语义（diff 红绿、告警琥珀），
		// 界面本身不含色相。
		//
		// t.Radius 是框架**全部**自绘控件的圆角（按钮、输入框、TextArea、
		// 下拉框、列表），§3 把它们统一归到 md(8)。规范另外点名要给 6 的
		// （工具/幽灵按钮、芯片、分段单元）在调用点显式写 radiusSM。
		t.Background = ui.Hex("#18191a")
		t.Surface = ui.Hex("#0f1011")
		t.SurfaceHover = ui.Hex("#141516")
		t.SurfacePressed = ui.Hex("#18191a")
		t.Border = ui.Hex("#23252a")
		t.Text = ui.Hex("#f7f8f8")
		t.TextMuted = ui.Hex("#8a8f98")
		t.Accent = ui.Hex("#f7f8f8")
		t.AccentHover = ui.Hex("#ffffff")
		t.AccentPressed = ui.Hex("#d0d6e0")
		t.AccentText = ui.Hex("#08090a")
		t.Danger = ui.Hex("#e5484d")
		t.Warning = ui.Hex("#e09f3e")
		t.Success = ui.Hex("#27a644")
		t.Selection = ui.RGBA(255, 255, 255, 0.20)
		t.Focus = ui.RGBA(247, 248, 248, 0.60)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.16)
		t.Radius = radiusMD
		return t
	}

	// 浅色模式对齐 Linear Inverse 规范。
	//
	// 有一处刻意不照搬：规范给的 inverse-surface-3 (#e4e7eb) 比画布和 Surface 1
	// 都暗，拿它铺浮层会让菜单和模态看着是**凹**下去的。浅色下的「浮起来」靠的是
	// 往白走（对比深色下的往亮走），所以浮层与模态用 inverse-canvas 的白，
	// 卡片与容器才用那三级灰。
	//
	// 强调色同理取反相：近黑的 Ink 实心键 + 白字。
	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#f7f8f9")
	t.SurfaceHover = ui.Hex("#f0f2f5")
	t.SurfacePressed = ui.Hex("#e4e7eb")
	t.Border = ui.Hex("#e1e4e8")
	t.Text = ui.Hex("#0f1115")
	t.TextMuted = ui.Hex("#6e7781")
	t.Accent = ui.Hex("#0f1115")
	t.AccentHover = ui.Hex("#2b2f36")
	t.AccentPressed = ui.Hex("#000000")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#e5484d")
	t.Warning = ui.Hex("#d97706")
	t.Success = ui.Hex("#27a644")
	t.Selection = ui.RGBA(15, 17, 21, 0.12)
	t.Focus = ui.RGBA(15, 17, 21, 0.40)
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.18)
	t.Radius = radiusMD
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
			hl: hl,
			// 表面阶梯（§2.1）
			surface1: ui.Hex("#0f1011"),
			surface2: ui.Hex("#141516"),
			surface3: ui.Hex("#18191a"),
			surface4: ui.Hex("#191a1b"),
			// 发丝线（§2.2）
			hairline:         ui.Hex("#23252a"),
			hairlineStrong:   ui.Hex("#34343a"),
			hairlineTertiary: ui.Hex("#3e3e44"),
			// 灰阶（§2.3）
			ink:         ui.Hex("#f7f8f8"),
			inkMuted:    ui.Hex("#d0d6e0"),
			inkSubtle:   ui.Hex("#8a8f98"),
			inkTertiary: ui.Hex("#62666d"),
			// 选中底带：Surface 2 与 3 之间的一档灰，比悬停的 SurfaceHover
			// 重一档，才压得住「选中的是这一行」。
			selection: ui.Hex("#1f2023"),
			appBg:     ui.Hex("#010102"), // Linear Canvas: 极致纯净深黑
			codeBg:    ui.Hex("#010102"),
			code:      ui.Hex("#f7f8f8"), // Linear Ink
			lineNo:    ui.Hex("#62666d"), // Linear Ink Tertiary
			gutterBg:  ui.Hex("#010102"),
			addBg:     ui.RGBA(39, 166, 68, 0.09), // 极淡半透明暗苔绿
			addGutter: ui.RGBA(39, 166, 68, 0.14),
			addBar:    ui.Hex("#27a644"),          // Linear 语义绿发丝条
			delBg:     ui.RGBA(229, 72, 77, 0.09), // 极淡半透明暗绯红
			delGutter: ui.RGBA(229, 72, 77, 0.14),
			delBar:    ui.Hex("#e5484d"), // 校准红发丝条
			gapBg:     ui.Hex("#0f1011"), // Surface 1
			gapHover:  ui.Hex("#141516"), // Surface 2
			gapText:   ui.Hex("#8a8f98"), // Ink Subtle
			fileBg:    ui.Hex("#0f1011"), // Surface 1
			hunkBg:    ui.Hex("#141516"), // Surface 2: 告别旧版刺眼亮蓝！
			hunkText:  ui.Hex("#8a8f98"), // Ink Subtle 等宽中性文字
			hunkHover: ui.Hex("#18191a"), // Surface 3
			sideBg:    ui.Hex("#08090a"), // 介于 Canvas 与 Surface 1 之间
			ref:       ui.Hex("#d0d6e0"), // 中性 Ink Muted
			viewed:    ui.Hex("#27a644"),
			// 查找命中：深色下取拓扑图那颗暖琥珀（#e09f3e），压在近黑底上
			// 读得清，又和增删的绿/红拉开色相。透明度比浅色下高一档 ——
			// 白底上 0.20 已经很明显，同样的比例压在 #010102 上几乎看不见。
			findBg:        ui.RGBA(224, 159, 62, 0.40),
			findBgActive:  ui.RGBA(224, 159, 62, 0.78),
			findUnderline: ui.Hex("#e09f3e"),
			// 拓扑图精密仪器 8 色谱系（明度与彩度均衡）：
			// 0 号为主干，多颜色色彩方案，其余 7 条为支线，不含紫。
			lanes: [8]ui.Color{
				ui.Hex("#58a6ff"), // 主干/HEAD: 经典亮蓝
				ui.Hex("#3eb489"), // 鼠尾草绿
				ui.Hex("#e09f3e"), // 暖琥珀
				ui.Hex("#4c86e8"), // 钢青蓝
				ui.Hex("#e5484d"), // 珊瑚红
				ui.Hex("#45b6fe"), // 天青蓝
				ui.Hex("#d977a8"), // 暮灰粉
				ui.Hex("#38b2ac"), // 暗青
			},
		}
	}
	return palette{
		hl: hl,
		// 浅色下的表面阶梯：容器用灰、浮层往白走（理由见 glossTheme 里那段）。
		surface1:         ui.Hex("#f7f8f9"),
		surface2:         ui.Hex("#f0f2f5"),
		surface3:         ui.Hex("#ffffff"),
		surface4:         ui.Hex("#ffffff"),
		hairline:         ui.Hex("#e1e4e8"),
		hairlineStrong:   ui.Hex("#c9ced6"),
		hairlineTertiary: ui.Hex("#b8bec7"),
		ink:              ui.Hex("#0f1115"),
		inkMuted:         ui.Hex("#4b535d"),
		inkSubtle:        ui.Hex("#6e7781"),
		inkTertiary:      ui.Hex("#8c959f"),
		selection:        ui.Hex("#e4e7eb"),
		appBg:            ui.Hex("#ffffff"),
		codeBg:           ui.Hex("#ffffff"),
		code:             ui.Hex("#0f1115"),
		lineNo:           ui.Hex("#8c959f"),
		gutterBg:         ui.Hex("#ffffff"),
		addBg:            ui.RGBA(39, 166, 68, 0.10),
		addGutter:        ui.RGBA(39, 166, 68, 0.16),
		addBar:           ui.Hex("#27a644"),
		delBg:            ui.RGBA(229, 72, 77, 0.10),
		delGutter:        ui.RGBA(229, 72, 77, 0.16),
		delBar:           ui.Hex("#e5484d"),
		gapBg:            ui.Hex("#f7f8f9"),
		gapHover:         ui.Hex("#f0f2f5"),
		gapText:          ui.Hex("#6e7781"),
		fileBg:           ui.Hex("#f7f8f9"),
		hunkBg:           ui.Hex("#f0f2f5"),
		hunkText:         ui.Hex("#4b535d"),
		hunkHover:        ui.Hex("#ffffff"),
		sideBg:           ui.Hex("#f7f8f9"),
		ref:              ui.Hex("#4b535d"),
		viewed:           ui.Hex("#27a644"),
		// 查找命中：浅色下压深一档到琥珀棕（#d97706），白底上才有足够对比度。
		findBg:        ui.RGBA(217, 119, 6, 0.20),
		findBgActive:  ui.RGBA(217, 119, 6, 0.42),
		findUnderline: ui.Hex("#b45309"),
		lanes: [8]ui.Color{
			ui.Hex("#0969da"), // 主干/HEAD: 经典亮蓝
			ui.Hex("#2d9d74"),
			ui.Hex("#c9822b"),
			ui.Hex("#2f6fd0"),
			ui.Hex("#cf3b40"),
			ui.Hex("#2693db"),
			ui.Hex("#b85487"),
			ui.Hex("#288c87"),
		},
	}
}
