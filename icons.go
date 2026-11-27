package main

import "github.com/egoist/mygo/ui"

// lucideStrokeWidth 是全局 Lucide 图标的笔画宽（基于标准 24×24 viewBox）。
//
// Lucide 官方默认是 2，画在桌面端 14~16px 的工具栏与侧栏里略显粗钝，尤其是
// settings、file-diff、git-branch-plus 这类细节多的图标。收细到 1.8 后，
// 14px 上落到 1.05px（Retina 2.1px）、15px 上落到 1.125px（Retina 2.25px），
// 在 1x 与 2x 屏上都保持清晰利落、不过重。
const lucideStrokeWidth = "1.8"

// lucide 把 Lucide（https://lucide.dev/，v1.82.0）图标的路径字符串包装成带标准
// 24x24 viewBox 的 SVG 对象。
func lucide(shapes string) *ui.SVG {
	return lucideStroke(shapes, lucideStrokeWidth)
}

// sparkleStroke 是 iconSparkle 自己的笔画宽。
//
// 图标是从 24×24 的 viewBox 缩到实际尺寸的，笔画跟着一起缩：常规的 1.8~2
// 画到 11px 的按钮里不到 1 个像素宽，四只角会被抗锯齿摊成半透明的一层灰。
// 2.8~3.2 缩下来才落得实（icon_legibility_test.go 两头都盯着）。只给火花
// 自己加，不动 lucide()，免得整层工具栏一起变粗。
const sparkleStroke = "2.9"

// lucideStroke 和 lucide() 一样，但笔画宽由调用方给。
func lucideStroke(shapes, strokeWidth string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="` + strokeWidth + `" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	// 刷新（rotate-cw）
	iconRefresh = lucide(`<path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/>`)

	// 偏好设置 / 滑块（sliders-horizontal）
	iconSliders = lucide(`<path d="M10 5H3"/><path d="M12 19H3"/><path d="M14 3v4"/><path d="M16 17v4"/><path d="M21 12h-9"/><path d="M21 19h-5"/><path d="M21 5h-7"/><path d="M8 10v4"/><path d="M8 12H3"/>`)

	// 齿轮设置（settings）
	iconSettings = lucide(`<path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/>`)

	// 调色板 / 外观（palette）
	iconPalette = lucide(`<path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z"/><circle cx="13.5" cy="6.5" r=".5" fill="currentColor"/><circle cx="17.5" cy="10.5" r=".5" fill="currentColor"/><circle cx="6.5" cy="12.5" r=".5" fill="currentColor"/><circle cx="8.5" cy="7.5" r=".5" fill="currentColor"/>`)

	// 打开仓库 / 文件夹（folder）
	iconFolder = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)

	// 远程同步三件套（download / arrow-down / arrow-up）：
	// 抓取=下载远程引用到本地仓库（download），拉取=向下合入当前分支（arrow-down），
	// 推送=向上推到远程（arrow-up），与角标数字排在一起干净利落。
	iconFetch = lucide(`<path d="M12 15V3"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/>`)
	iconPull  = lucide(`<path d="M12 5v14"/><path d="m19 12-7 7-7-7"/>`)
	iconPush  = lucide(`<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>`)

	// 搜索 / 筛选（search）
	iconSearch = lucide(`<path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/>`)

	// 单颗火花（sparkle，用于 AI 按钮）
	//
	// 笔画比 lucide() 默认的重（sparkleStroke）：这颗在紧凑按钮里最小会画到
	// 11px，标准描边缩下来不到 1px 容易发虚。别顺手改成 fill：那会变成一团
	// 实心块，比虚更糟（icon_legibility_test.go 两头都盯着）。
	iconSparkle = lucideStroke(`<path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/>`, sparkleStroke)

	// 星群（sparkles，用于 AI 结果标题与入口）
	iconSparkles = lucide(`<path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/><path d="M20 2v4"/><path d="M22 4h-4"/><circle cx="4" cy="20" r="2"/>`)

	// 勾选（check）
	iconCheck = lucide(`<path d="M20 6 9 17l-5-5"/>`)

	// 减号 / 部分勾选（minus）
	iconMinus = lucide(`<path d="M5 12h14"/>`)

	// 折叠箭头（chevron-right / chevron-down）
	iconChevronRight = lucide(`<path d="m9 18 6-6-6-6"/>`)
	iconChevronDown  = lucide(`<path d="m6 9 6 6 6-6"/>`)

	// 视图切换（columns-2 / square-menu）：两者共用同一套 18×18 圆角外框，
	// 左右分栏与单栏行文一眼分明。
	iconSplit   = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M12 3v18"/>`)
	iconUnified = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M7 8h10"/><path d="M7 12h10"/><path d="M7 16h10"/>`)

	// 自动换行（wrap-text）
	iconWrap = lucide(`<path d="m16 16-3 3 3 3"/><path d="M3 12h14.5a1 1 0 0 1 0 7H13"/><path d="M3 19h6"/><path d="M3 5h18"/>`)

	// 分支对比（git-compare）
	iconGitCompare = lucide(`<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><path d="M11 18H8a2 2 0 0 1-2-2V9"/>`)

	// 分支（git-branch）
	iconGitBranch = lucide(`<path d="M15 6a9 9 0 0 0-9 9V3"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/>`)

	// 新建分支（git-branch-plus）
	iconGitBranchPlus = lucide(`<path d="M6 3v12"/><path d="M18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"/><path d="M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"/><path d="M15 6a9 9 0 0 0-9 9"/><path d="M18 15v6"/><path d="M21 18h-6"/>`)

	// 交换分支（arrow-left-right）
	iconSwap = lucide(`<path d="M8 3 4 7l4 4"/><path d="M4 7h16"/><path d="m16 21 4-4-4-4"/><path d="M20 17H4"/>`)

	// Pull Request（git-pull-request）
	iconGitPullRequest = lucide(`<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><line x1="6" x2="6" y1="9" y2="21"/>`)

	// 盾牌 / 安全预检（shield / shield-check / shield-alert）
	iconShield      = lucide(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>`)
	iconShieldCheck = lucide(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>`)
	iconShieldAlert = lucide(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="M12 8v4"/><path d="M12 16h.01"/>`)

	// 状态提示（circle-alert / triangle-alert / circle-check / info）
	iconCircleAlert   = lucide(`<circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/>`)
	iconTriangleAlert = lucide(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>`)
	iconCircleCheck   = lucide(`<circle cx="12" cy="12" r="10"/><path d="m16 9-5.5 5.5L8 12"/>`)
	iconInfo          = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>`)

	// 提交（git-commit-vertical，纵向线条与提交树方向一致，小尺寸下比横向清晰）
	iconGitCommit = lucide(`<path d="M12 3v6"/><circle cx="12" cy="12" r="3"/><path d="M12 15v6"/>`)

	// 提交历史（history）
	iconHistory = lucide(`<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l4 2"/>`)

	// 时钟 / 最近项目（clock）
	iconClock = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>`)

	// 垃圾桶 / 删除（trash-2）
	iconTrash = lucide(`<path d="M10 11v6"/><path d="M14 11v6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)

	// 新建/打开文件夹（folder-plus）
	iconFolderPlus = lucide(`<path d="M12 10v6"/><path d="M9 13h6"/><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)

	// 眼睛（eye / eye-off）
	iconEye    = lucide(`<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/>`)
	iconEyeOff = lucide(`<path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49"/><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242"/><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143"/><path d="m2 2 20 20"/>`)

	// 闪电（zap）
	iconZap = lucide(`<path d="M15.914 4a1.5 1.5 0 00-2.474-1.561l-9 9A1.5 1.5 0 005.5 14h4.002a.5.5 0 01.471.666L8.086 20a1.5 1.5 0 002.475 1.56l9-9A1.5 1.5 0 0018.5 10h-3.997a.5.5 0 01-.472-.667z"/>`)

	// 关闭/清空（x）
	iconX = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)

	// 窗口按钮（Windows / Linux）：最小化（minus）、最大化（square）、还原（copy）。
	iconMinimize = lucide(`<path d="M5 12h14"/>`)
	iconMaximize = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/>`)
	iconRestore  = lucide(`<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>`)

	// 终端（terminal）
	iconTerminal = lucide(`<path d="M12 19h8"/><path d="m4 17 6-6-6-6"/>`)

	// 加号 / 新建（plus）
	iconPlus = lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)

	// 改动 / 工作区文件（file-diff）
	iconFileDiff = lucide(`<path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"/><path d="M9 10h6"/><path d="M12 13V7"/><path d="M9 17h6"/>`)

	// 代码编辑器（code-xml）
	iconCode = lucide(`<path d="m18 16 4-4-4-4"/><path d="m6 8-4 4 4 4"/><path d="m14.5 4-5 16"/>`)

	// 浅色模式（sun）
	iconSun = lucide(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>`)

	// 暗黑模式（moon）
	iconMoon = lucide(`<path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/>`)

	// 跟随系统外观（monitor）
	iconMonitor = lucide(`<rect width="20" height="14" x="2" y="3" rx="2"/><line x1="8" x2="16" y1="21" y2="21"/><line x1="12" x2="12" y1="17" y2="21"/>`)

	// 拖动排序手柄（grip-vertical）。六个点各自填色：描边版的 r=1 圆在
	// 11px 的尺寸下只剩一圈糊边的轮廓，填实了才看得清是「抓手」。
	iconGrip = lucide(`<circle cx="9" cy="12" r="1" fill="currentColor"/><circle cx="9" cy="5" r="1" fill="currentColor"/><circle cx="9" cy="19" r="1" fill="currentColor"/><circle cx="15" cy="12" r="1" fill="currentColor"/><circle cx="15" cy="5" r="1" fill="currentColor"/><circle cx="15" cy="19" r="1" fill="currentColor"/>`)

	// 语言切换 / 全局配置（globe）
	iconGlobe = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>`)

	// 窗口 / 应用图标（app-window）
	iconAppWindow = lucide(`<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M10 4v4"/><path d="M2 8h20"/><path d="M6 4v4"/>`)

	// 侧栏开关（panel-left）
	iconSidebar = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)

	// 作者头像里的那个人形（user）。提交详情卡用 —— 没有真人照片可放，
	// 也不能拿用户名首字母硬凑（中文名取首字母是个字母都不是的怪东西）。
	iconUser = lucide(`<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>`)

	// 邮箱（mail）
	iconMail = lucide(`<path d="m22 7-8.991 5.727a2 2 0 0 1-2.009 0L2 7"/><rect x="2" y="4" width="20" height="16" rx="2"/>`)

	// 网页链接 / 外部链接（link / external-link）
	iconLink         = lucide(`<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>`)
	iconExternalLink = lucide(`<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`)

	// 哈希（hash，用于提交详情页的 commit hash）
	iconHash = lucide(`<line x1="4" x2="20" y1="9" y2="9"/><line x1="4" x2="20" y1="15" y2="15"/><line x1="10" x2="8" y1="3" y2="21"/><line x1="16" x2="14" y1="3" y2="21"/>`)

	// 提示词（message-square-text）
	iconMessageSquareText = lucide(`<path d="M22 17a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 21.286V5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2z"/><path d="M7 11h10"/><path d="M7 15h6"/><path d="M7 7h8"/>`)

	// 软件更新（circle-arrow-up）
	iconCircleArrowUp = lucide(`<circle cx="12" cy="12" r="10"/><path d="m16 12-4-4-4 4"/><path d="M12 16V8"/>`)
)

// drawIcon 是绘制指定尺寸与颜色的矢量图标的通用控件。
func drawIcon(c *ui.Context, svg *ui.SVG, size float32, color ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Icon(svg, r, color)
	})
}
