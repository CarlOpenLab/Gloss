package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// palette 是 diff 视图专用的语义色。
//
// mygo 的 ui.Theme 已经管了通用控件（按钮、输入框、边框、文字），
// 但 diff 需要的是它没有的一层：行背景、行号栏底色、增删色条、未改动区间。
// 深浅两套跟随系统，取值贴近 Web 版那套 GitHub 配色，观感能对上。
type palette struct {
	// Shell materials. These are the values the card, sidebar rows and menus
	// read directly; the surface ladder below maps the older components onto
	// the same set so both layers stay visually consistent.
	dark        bool
	text        ui.Color
	textMuted   ui.Color
	textFaint   ui.Color
	rowHover    ui.Color
	rowSelected ui.Color
	card        ui.Color
	cardBorder  ui.Color
	cardShadow  ui.Color
	cardShadow2 ui.Color
	floatShadow ui.Color
	popover     ui.Color
	accent      ui.Color

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
	// 设置分组面板与表单控件的表面（agent-desktop-ui §2.1 / §5）。
	// chip 是分组容器的底、rowDivider 是行与行之间那一道发丝线；
	// control / controlHover 是次要按钮与分段轨道赖以站起来的填充。
	chip         ui.Color
	rowDivider   ui.Color
	control      ui.Color
	controlHover ui.Color
	// kbd 是快捷键提示的底色（§2.1）。
	kbd ui.Color

	ink         ui.Color // 高亮标题、正文基准（§2.3）
	inkMuted    ui.Color // 常规正文、文件名、按钮文字
	inkSubtle   ui.Color // 行号、辅助元信息、未激活图标
	inkTertiary ui.Color // 快捷键提示、占位符、禁用态

	// appBg 是卡片里挖出来的那条槽（终端芯片带）的底色：浅色下比卡片白
	// 暗一档，深色下比卡片沉一档，靠一道发丝线和正文分开。
	appBg  ui.Color
	codeBg ui.Color // 普通代码行背景
	code   ui.Color // 代码文字
	addBar ui.Color // 新增行左侧色条
	delBar ui.Color // 删除行左侧色条
	gapBg  ui.Color // 未改动区间条的背景
	fileBg ui.Color // 文件头的背景
	// hunkHover 是 hunk 行上那颗幽灵按钮悬停时的底色。它落在 hunk 带上，
	// 所以不能直接用 codeBg：深色下那是比带子更暗的一档，鼠标一指上去
	// 按钮反而沉下去。浅色下要的是提亮（白），深色下要的是抬高一档的蓝灰。
	// sideBg 是「窗口」那一层的中性底：主窗口铺在侧栏和卡片下面（§1），
	// 设置窗口的外壳也用它，浅色取值与 term-fleet 的 plain 同一支冷灰。
	// 拆分 diff 里对面没有对应行的留白也读它 —— 那里要的是同一档「不是内容」。
	sideBg ui.Color
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

	// —— 语义三色 ——
	//
	// 成功 / 告警 / 危险：值仍然由 glossTheme 定义（框架自绘的控件、toast、
	// 工具提示也读那一份），这里只是把同一份搬进调色板，让主包里所有语义着色
	// 都从这一个入口拿 —— 以前是「有的地方写 t.Danger，有的地方写 pal.delBar」，
	// 同一种红在两个名字底下，谁也没法一眼看出它们是不是同一支。
	success ui.Color
	warning ui.Color
	danger  ui.Color

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

// lanePreset 是一套内置的拓扑图配色：深浅两份，各 laneCount 个色号。
//
// 色号是「线的身份」，不是泳道号 —— 见 buildGraph 那段注释：一条线会换泳道，
// 按泳道号定色的话主线走一半就变色了。所以一份色板就是一份身份表。
type lanePreset struct {
	id    string
	dark  [laneCount]ui.Color
	light [laneCount]ui.Color
}

// lanePresets 是设置里可选的几套方案，第一套是出厂默认。
//
// 每套都守着 §2.6 那两条：0 号是主干（classic / vivid 里是一支彩色，
// mono 那一套整份灰阶，0 号仍是最亮/最重的一阶）、全图只有一支蓝、不含紫。
// 方案之间的差别是彩度与明度策略，不是「多给几种颜色」—— 颜色本身不是目的，
// 让并排的线一眼分得开才是；两支蓝并排会把图读成一条蓝渐变，那正是 §2.6
// 当初要拆掉的东西。
var lanePresets = []lanePreset{
	{
		// classic 就是 §2.6 写死的那一份：明度与彩度均衡的莫兰迪 8 色。
		//
		// 0 号是钢青蓝而不是中性 Ink：主干是整张图上最该被认出的那条线，
		// 黑白在那儿不如一支彩色（大多数 git 图形工具的主干也都是彩色）。
		// 这一改是明确选定过的取舍 —— 0 号色同时是侧栏当前分支圆点、
		// HEAD 芯片点、「当前」芯片、提交栏状态胶囊的颜色，那些地方跟着
		// 一起变蓝。旧 3 号那支蓝因此让位，换成芥末黄：8 个色相里再挤一支，
		// 它和琥珀差 10° 色相但一个纯黄一个偏橙，图最多铺 6 条泳道
		// （graphMaxCol），7/8 号极少同时出现。
		id: "classic",
		dark: [laneCount]ui.Color{
			ui.Hex("#4c86e8"), // 0 主干 / HEAD：钢青蓝（全图唯一的蓝）
			ui.Hex("#3eb489"), // 1 鼠尾草绿
			ui.Hex("#e09f3e"), // 2 暖琥珀
			ui.Hex("#c9a227"), // 3 芥末黄
			ui.Hex("#e5484d"), // 4 珊瑚红
			ui.Hex("#d977a8"), // 5 暮灰粉
			ui.Hex("#38b2ac"), // 6 暗青
			ui.Hex("#9aab45"), // 7 苔绿
		},
		light: [laneCount]ui.Color{
			ui.Hex("#2f6fd0"), // 0 主干 / HEAD：钢青蓝（全图唯一的蓝）
			ui.Hex("#2d9d74"), // 1 鼠尾草绿
			ui.Hex("#c9822b"), // 2 暖琥珀
			ui.Hex("#a8860b"), // 3 芥末黄
			ui.Hex("#cf3b40"), // 4 珊瑚红
			ui.Hex("#b85487"), // 5 暮灰粉
			ui.Hex("#288c87"), // 6 暗青
			ui.Hex("#6f7a1f"), // 7 苔绿
		},
	},
	{
		// vivid 是同一套色相、彩度拉高一档的一版：投影、外接屏、或者
		// 反光的房间里更容易看清是哪条线。
		id: "vivid",
		dark: [laneCount]ui.Color{
			ui.Hex("#4c86e8"), // 0 主干 / HEAD：钢青蓝（唯一的蓝）
			ui.Hex("#17c964"), // 1 绿
			ui.Hex("#f5a524"), // 2 琥珀
			ui.Hex("#eab308"), // 3 亮黄
			ui.Hex("#f31260"), // 4 洋红
			ui.Hex("#ff8fb1"), // 5 粉
			ui.Hex("#14b8a6"), // 6 青
			ui.Hex("#a3e635"), // 7 黄绿
		},
		light: [laneCount]ui.Color{
			ui.Hex("#2f6fd0"), // 0 主干 / HEAD：钢青蓝（唯一的蓝）
			ui.Hex("#0f9d58"), // 1 绿
			ui.Hex("#c47a12"), // 2 琥珀
			ui.Hex("#ca8a04"), // 3 亮黄
			ui.Hex("#d62b5b"), // 4 洋红
			ui.Hex("#c2618b"), // 5 粉
			ui.Hex("#0e8f8a"), // 6 青
			ui.Hex("#5f8f1f"), // 7 黄绿
		},
	},
	{
		// mono 是灰阶：不靠色相分辨，打印、色觉辅助、以及「只想看结构」时用。
		// 八阶明度顺着排，相邻两阶在各自底色上都拉得开。它是唯一 0 号不走
		// 彩色的方案 —— 那一套的全部意义就是「不靠色相」。
		id: "mono",
		dark: [laneCount]ui.Color{
			ui.Hex("#e6e6e6"), // 0 主干 / HEAD：最亮的一阶 = 主线
			ui.Hex("#d2d6da"),
			ui.Hex("#b9bfc5"),
			ui.Hex("#a1a8af"),
			ui.Hex("#8a9199"),
			ui.Hex("#737a83"),
			ui.Hex("#5d646d"),
			ui.Hex("#4a5158"),
		},
		light: [laneCount]ui.Color{
			ui.Hex("#0f1115"), // 0 主干 / HEAD：最重的一阶 = 主线
			ui.Hex("#262b31"),
			ui.Hex("#3d434a"),
			ui.Hex("#545b63"),
			ui.Hex("#6b737c"),
			ui.Hex("#828b95"),
			ui.Hex("#99a3ad"),
			ui.Hex("#b0bac5"),
		},
	},
}

// lanes 按明暗取这套方案自带的一份色号（不含用户覆盖）。
func (p lanePreset) lanes(dark bool) [laneCount]ui.Color {
	if dark {
		return p.dark
	}
	return p.light
}

// lanePresetLabel 是方案在界面上的名字与说明。id 存进配置，名字只给人看，
// 所以它跟着界面语言走（见 T）。
func lanePresetLabel(id string) (name, desc string) {
	switch id {
	case "vivid":
		return T("鲜艳", "Vivid"), T("彩度更高 · 投影与强光环境", "Higher chroma · Projectors and glare")
	case "mono":
		return T("灰阶", "Grayscale"), T("不靠色相 · 打印与色觉辅助", "No hue · Print and color-vision friendly")
	default:
		return T("经典", "Classic"), T("莫兰迪 8 色 · 出厂默认", "Morandi 8 · Shipped default")
	}
}

// laneSet 是设置里选出来的「方案 + 覆盖」，和 store.Config 的三个字段一一对应。
type laneSet struct {
	preset string
	dark   []string
	light  []string
}

// lanes 算出这一帧要用的 8 个色号：先取方案的底，再逐号套覆盖。
//
// 解析不出来的覆盖条目（空串、写错的 hex、多出来的第 9 号）一律跳过，保留
// 方案自带的颜色 —— 配置是用户能手改的文件，一条坏值该只影响它自己那一号，
// 而不是让整张图失色；ui.Hex 对非法输入是 panic，所以这层必须拦在前面。
func (ls laneSet) lanes(dark bool) [laneCount]ui.Color {
	p := lanePresetByID(ls.preset)
	out, ov := p.lanes(false), ls.light
	if dark {
		out, ov = p.lanes(true), ls.dark
	}
	for i, s := range ov {
		if i >= laneCount {
			break
		}
		if c, ok := hexColor(s); ok {
			out[i] = c
		}
	}
	return out
}

// lanePresetByID 找方案。名字空着、拼错，或者配置文件来自更老的版本，都回落到
// 第一套 —— 这里的失败模式必须是「长得像出厂设置」，而不是「图没有颜色」。
func lanePresetByID(id string) lanePreset {
	for _, p := range lanePresets {
		if p.id == id {
			return p
		}
	}
	return lanePresets[0]
}

// hexColor 解析 "#rgb" / "#rgba" / "#rrggbb" / "#rrggbbaa"，其余一律 false。
//
// 自己先挡一道，而不是直接交给 ui.Hex：ui.Hex 的契约是「非法输入 panic，
// 那是程序写错了」，而这里的输入来自磁盘上可以手改的 JSON。
func hexColor(s string) (ui.Color, bool) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(h) {
	case 3, 4, 6, 8:
	default:
		return ui.Color{}, false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return ui.Color{}, false
	}
	return ui.Hex("#" + h), true
}

// hexOf 把颜色写成 "#rrggbb"：存配置、给取色控件当初始值都用它。
func hexOf(c ui.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// glossTheme gives the framework-drawn controls the same soft, blue-accented
// material as the shell. Surfaces stand apart by fill and a restrained edge;
// semantic colors remain available for diffs, warnings and destructive actions.
func glossTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}

	if dark {
		t.Background = ui.Hex("#232328")
		t.Surface = ui.Hex("#404047")
		t.SurfaceHover = ui.Hex("#4a4a52")
		t.SurfacePressed = ui.Hex("#52525a")
		t.Border = ui.Transparent
		t.Text = ui.Hex("#ececf0")
		t.TextMuted = ui.Hex("#a9a9b4")
		t.Accent = ui.Hex("#6b93ff")
		t.AccentHover = ui.Hex("#82a3ff")
		t.AccentPressed = ui.Hex("#5b82ee")
		t.AccentText = ui.Hex("#ffffff")
		t.Danger = ui.Hex("#ff6961")
		t.Warning = ui.Hex("#f0a33a")
		t.Success = ui.Hex("#27a644")
		t.Selection = ui.RGBA(107, 147, 255, 0.25)
		t.Focus = ui.RGBA(107, 147, 255, 0.65)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.16)
		t.Radius = radiusMD
		return t
	}

	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#f7f7f9")
	t.SurfacePressed = ui.Hex("#eeeef1")
	t.Border = ui.Transparent
	t.Text = ui.Hex("#1d1d1f")
	t.TextMuted = ui.Hex("#6e6e78")
	t.Accent = ui.Hex("#3b6cf6")
	t.AccentHover = ui.Hex("#527ef7")
	t.AccentPressed = ui.Hex("#2f5ddb")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#d93a35")
	t.Warning = ui.Hex("#c27b0f")
	t.Success = ui.Hex("#22863a")
	t.Selection = ui.RGBA(59, 108, 246, 0.25)
	t.Focus = ui.RGBA(59, 108, 246, 0.55)
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

// paletteFor 把主题算成调色板。ls 是拓扑图配色那三个设置字段 —— 传零值
// 就是出厂的那一套（见 lanePresets）。
func paletteFor(t *ui.Theme, ls laneSet) palette {
	lanes := ls.lanes(t.Dark)
	if t.Dark {
		return palette{
			dark:        true,
			text:        ui.Hex("#ececf0"),
			textMuted:   ui.Hex("#a9a9b4"),
			textFaint:   ui.Hex("#83838f"),
			rowHover:    ui.RGBA(255, 255, 255, 0.05),
			rowSelected: ui.RGBA(255, 255, 255, 0.10),
			card:        ui.Hex("#232328"),
			cardBorder:  ui.RGBA(255, 255, 255, 0.08),
			cardShadow:  ui.RGBA(0, 0, 0, 0.35),
			cardShadow2: ui.RGBA(0, 0, 0, 0.20),
			floatShadow: ui.RGBA(0, 0, 0, 0.45),
			popover:     ui.Hex("#232327"),
			accent:      ui.Hex("#6b93ff"),
			// 表面阶梯（§2.1）
			surface1: ui.Hex("#232328"),
			surface2: ui.Hex("#2c2c32"),
			surface3: ui.Hex("#232327"),
			surface4: ui.Hex("#2b2b31"),
			// 发丝线（§2.2）
			hairline:         ui.RGBA(255, 255, 255, 0.07),
			hairlineStrong:   ui.RGBA(255, 255, 255, 0.11),
			hairlineTertiary: ui.RGBA(255, 255, 255, 0.16),
			chip:             ui.RGBA(255, 255, 255, 0.07),
			rowDivider:       ui.RGBA(255, 255, 255, 0.07),
			control:          ui.Hex("#404047"),
			controlHover:     ui.Hex("#4a4a52"),
			kbd:              ui.RGBA(255, 255, 255, 0.06),
			// 灰阶（§2.3）
			ink:         ui.Hex("#ececf0"),
			inkMuted:    ui.Hex("#a9a9b4"),
			inkSubtle:   ui.Hex("#83838f"),
			inkTertiary: ui.Hex("#656570"),
			// 选中底带：Surface 2 与 3 之间的一档灰，比悬停的 SurfaceHover
			// 重一档，才压得住「选中的是这一行」。
			selection: ui.RGBA(107, 147, 255, 0.14),
			appBg:     ui.Hex("#17141f"), // 窗口柔和底色的最低一档
			codeBg:    ui.Hex("#232328"),
			code:      ui.Hex("#ececf0"),
			addBar:    ui.Hex("#4fd17a"),
			delBar:    ui.Hex("#ff6961"),
			gapBg:     ui.Hex("#232328"),
			sideBg:    ui.Hex("#141418"),
			ref:       ui.Hex("#a9a9b4"),
			viewed:    ui.Hex("#4fd17a"),
			// 小标签亮色（§2.5）：三支都取自下面那 8 色谱系，别单独调，
			// 调了图里和图外的同一种颜色就对不上了。
			accentAmber: ui.Hex("#f0a33a"), // = t.Warning 那一支
			accentBlue:  ui.Hex("#6b93ff"),
			accentTeal:  ui.Hex("#38b2ac"), // = lanes[6]
			// 语义三色从主题取：glossTheme 是唯一定义处，别在这里另写一遍 hex。
			success: t.Success,
			warning: t.Warning,
			danger:  t.Danger,
			// 拓扑图色号：由「方案 + 覆盖」算出来的那一份（见 laneSet.lanes）。
			// 出厂方案就是 §2.6 那一套：0 号中性 Ink 留给主干，全图只有一支蓝。
			// 自定义只动这里；下面三支小标签亮色（accentAmber/Blue/Teal）
			// 不跟着走 —— 它们是状态字母和芯片圆点的固定语义色，见 §2.5。
			lanes: lanes,
		}
	}
	return palette{
		text:        ui.Hex("#1d1d1f"),
		textMuted:   ui.Hex("#6e6e78"),
		textFaint:   ui.Hex("#a3a3ad"),
		rowHover:    ui.RGBA(0, 0, 0, 0.04),
		// 选中行用中性灰底，不用白胶囊：白底加投影压在灰蓝侧栏上活像一颗
		// 输入框（下面「筛选分支」就长那样），到了 settings 的白卡片上又
		// 整个隐形。中性灰在两种底上都成立，比悬停正好重一档 —— 和 dark
		// 侧「比 SurfaceHover 重一档」是同一条规矩。
		rowSelected: ui.RGBA(0, 0, 0, 0.07),
		card:        ui.Hex("#ffffff"),
		cardBorder:  ui.RGBA(0, 0, 0, 0.07),
		cardShadow:  ui.RGBA(0, 0, 0, 0.07),
		cardShadow2: ui.RGBA(0, 0, 0, 0.035),
		floatShadow: ui.RGBA(20, 20, 40, 0.14),
		popover:     ui.Hex("#ffffff"),
		accent:      ui.Hex("#3b6cf6"),
		// 浅色下的表面阶梯：容器用灰、浮层往白走（理由见 glossTheme 里那段）。
		surface1:         ui.Hex("#ffffff"),
		surface2:         ui.Hex("#f7f7f9"),
		surface3:         ui.Hex("#ffffff"),
		surface4:         ui.Hex("#ffffff"),
		hairline:         ui.RGBA(0, 0, 0, 0.06),
		hairlineStrong:   ui.RGBA(0, 0, 0, 0.10),
		hairlineTertiary: ui.RGBA(0, 0, 0, 0.14),
		chip:             ui.RGBA(0, 0, 0, 0.045),
		rowDivider:       ui.RGBA(0, 0, 0, 0.07),
		control:          ui.Hex("#ffffff"),
		controlHover:     ui.Hex("#f7f7f9"),
		kbd:              ui.RGBA(0, 0, 0, 0.04),
		ink:              ui.Hex("#1d1d1f"),
		inkMuted:         ui.Hex("#6e6e78"),
		inkSubtle:        ui.Hex("#8d8d97"),
		inkTertiary:      ui.Hex("#a3a3ad"),
		selection:        ui.RGBA(59, 108, 246, 0.12),
		appBg:            ui.Hex("#f7f7f9"),
		codeBg:           ui.Hex("#ffffff"),
		code:             ui.Hex("#1d1d1f"),
		addBar:           ui.Hex("#22863a"),
		delBar:           ui.Hex("#d93a35"),
		gapBg:            ui.Hex("#ffffff"),
		sideBg:           ui.Hex("#eceef4"),
		ref:              ui.Hex("#6e6e78"),
		viewed:           ui.Hex("#22863a"),
		// 小标签亮色（§2.5）：与深色同源，压深一档对应下面 lanes 的浅色版。
		// 琥珀取的是**告警琥珀**（#d97706，和 t.Warning 同一支），不是下面
		// lanes[2] 那支暖琥珀沙（#c9822b）：标签、改动标记与告警说的是同一件
		// 「留神看一眼」的事，一屏里出现两种琥珀比一种更糟。
		accentAmber: ui.Hex("#c27b0f"), // = t.Warning
		accentBlue:  ui.Hex("#3b6cf6"),
		accentTeal:  ui.Hex("#288c87"), // = lanes[6]
		// 语义三色从主题取，理由见深色那段。
		success: t.Success,
		warning: t.Warning,
		danger:  t.Danger,
		// 浅色下方案自带的那一份压深一档（白底上要更重的色才站得住），
		// 同样是「0 号中性 Ink + 只有一支蓝」。
		lanes: lanes,
	}
}
