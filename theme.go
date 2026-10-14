package main

import (
	"github.com/egoist/mygo/ui"
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

	appBg  ui.Color // 窗口底色
	codeBg ui.Color // 普通代码行背景
	code   ui.Color // 代码文字
	addBar ui.Color // 新增行左侧色条
	delBar ui.Color // 删除行左侧色条
	gapBg  ui.Color // 未改动区间条的背景
	fileBg ui.Color // 文件头的背景
	// hunkHover 是 hunk 行上那颗幽灵按钮悬停时的底色。它落在 hunk 带上，
	// 所以不能直接用 codeBg：深色下那是比带子更暗的一档，鼠标一指上去
	// 按钮反而沉下去。浅色下要的是提亮（白），深色下要的是抬高一档的蓝灰。
	sideBg ui.Color // 删除侧留白（split 模式下对面没有对应行）
	ref    ui.Color // 分支名、commit hash 的强调色
	viewed ui.Color // 已 review 的勾与文字

	// —— 小标签亮色（§2.5）——
	//
	// 界面整体是灰阶 + 黑白两极的强调，这一组是仅有的例外，而且只喂给界面上
	// 最小的两处：文件列表里 10px 的状态字母（M/A/D/R/C）、引用芯片上那颗 6px
	// 的点。这两处必须有颜色 —— 灰阶落在一个 10px 的字上分不出层级，
	// 「M 和 D 都是灰的」等于让读的人把每个字母都读一遍。
	//
	// 取值不是新配的一套：三支全部借自拓扑图那 8 色谱系（§2.6），于是
	// 「图里那条蓝线」和「列表里这个 R」「芯片上这个本地分支点」说的是同一件事、
	// 同一种蓝。A/D 继续用 diff 的绿与红（addBar / delBar）——
	// 一行里出现两个相近的绿比没有绿更糟。
	accentAmber ui.Color // M / T 改动、标签芯片的点：暖琥珀
	accentBlue  ui.Color // R / C 重命名与复制、本地分支芯片的点：钢青蓝
	accentTeal  ui.Color // 远程跟踪分支芯片的点：暗青

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
	if t.Dark {
		return palette{
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
			codeBg:    ui.Hex("#0f1011"), // Surface 1：代码主区从画布上抬一档
			code:      ui.Hex("#f7f8f8"), // Linear Ink
			addBar:    ui.Hex("#27a644"), // Linear 语义绿发丝条
			delBar:    ui.Hex("#e5484d"), // 校准红发丝条
			gapBg:     ui.Hex("#0f1011"), // Surface 1
			sideBg:    ui.Hex("#0f1011"), // Surface 1：侧栏是一块面，不是画布的影子
			ref:       ui.Hex("#d0d6e0"), // 中性 Ink Muted
			viewed:    ui.Hex("#27a644"),
			// 小标签亮色（§2.5）：三支都取自下面那 8 色谱系，别单独调，
			// 调了图里和图外的同一种颜色就对不上了。
			accentAmber: ui.Hex("#e09f3e"), // = lanes[2]
			accentBlue:  ui.Hex("#4c86e8"), // = lanes[3]
			accentTeal:  ui.Hex("#38b2ac"), // = lanes[6]
			// 拓扑图 8 色谱系，明度与彩度均衡，不含紫：
			// 0 号留给主干，取中性 Ink —— 主线是整张图上最亮的一条，一眼就能
			// 顺着它从顶读到底；彩色只落在其余 7 条支线上。
			//
			// 这里同样要「全图只有一支蓝」。原来三条泳道各占一支蓝
			// （#58a6ff / #4c86e8 / #45b6fe），并排出现在一屏里，整张图会被读成
			// 「一条蓝色渐变 + 几个别的色」，而不是「一条线一个颜色」—— 支线之间
			// 分不出彼此，图就白画了。
			lanes: [8]ui.Color{
				ui.Hex("#e6e6e6"), // 0 主干 / HEAD：中性 Ink
				ui.Hex("#3eb489"), // 1 鼠尾草绿
				ui.Hex("#e09f3e"), // 2 暖琥珀
				ui.Hex("#4c86e8"), // 3 钢青蓝（全图唯一的蓝）
				ui.Hex("#e5484d"), // 4 珊瑚红
				ui.Hex("#d977a8"), // 5 暮灰粉
				ui.Hex("#38b2ac"), // 6 暗青
				ui.Hex("#9aab45"), // 7 苔绿
			},
		}
	}
	return palette{
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
		addBar:           ui.Hex("#27a644"),
		delBar:           ui.Hex("#e5484d"),
		gapBg:            ui.Hex("#f7f8f9"),
		sideBg:           ui.Hex("#efeff0"), // 侧栏实色：比主区白底暗一档，是「窗口」不是「内容」
		ref:              ui.Hex("#4b535d"),
		viewed:           ui.Hex("#27a644"),
		// 小标签亮色（§2.5）：与深色同源，压深一档对应下面 lanes 的浅色版。
		accentAmber: ui.Hex("#c9822b"), // = lanes[2]
		accentBlue:  ui.Hex("#2f6fd0"), // = lanes[3]
		accentTeal:  ui.Hex("#288c87"), // = lanes[6]
		// 浅色同一份色板压深一档（白底上要更重的色才站得住），同样是
		// 「0 号中性 Ink + 只有一支蓝」。
		lanes: [8]ui.Color{
			ui.Hex("#0f1115"), // 0 主干 / HEAD：中性 Ink
			ui.Hex("#2d9d74"), // 1 鼠尾草绿
			ui.Hex("#c9822b"), // 2 暖琥珀
			ui.Hex("#2f6fd0"), // 3 钢青蓝（全图唯一的蓝）
			ui.Hex("#cf3b40"), // 4 珊瑚红
			ui.Hex("#b85487"), // 5 暮灰粉
			ui.Hex("#288c87"), // 6 暗青
			ui.Hex("#6f7a1f"), // 7 苔绿
		},
	}
}
