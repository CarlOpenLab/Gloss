package main

import "github.com/egoist/mygo/ui"

// lucide 把 Lucide 图标的路径字符串包装成带标准 24x24 viewBox 的 SVG 对象。
func lucide(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	// 刷新（rotate-cw）
	iconRefresh = lucide(`<path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/>`)

	// 设置（sliders-horizontal）
	iconSliders = lucide(`<path d="M10 5H3"/><path d="M12 19H3"/><path d="M14 3v4"/><path d="M16 17v4"/><path d="M21 12h-9"/><path d="M21 19h-5"/><path d="M21 5h-7"/><path d="M8 10v4"/><path d="M8 12H3"/>`)

	// 打开仓库（folder）
	iconFolder = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)

	// 抓取（cloud-download）
	iconFetch = lucide(`<path d="M12 13v8l-4-4"/><path d="m12 21 4-4"/><path d="M4.393 15.269A7 7 0 1 1 15.71 8h1.79a4.5 4.5 0 0 1 2.436 8.284"/>`)

	// 拉取（arrow-down-to-line）
	iconPull = lucide(`<path d="M12 17V3"/><path d="m6 11 6 6 6-6"/><path d="M19 21H5"/>`)

	// 推送（arrow-up-from-line）
	iconPush = lucide(`<path d="m18 9-6-6-6 6"/><path d="M12 3v14"/><path d="M5 21h14"/>`)

	// 搜索 / 筛选（search）
	iconSearch = lucide(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.34-4.34"/>`)

	// 单颗火花（sparkle，用于解释按钮）
	iconSparkle = lucide(`<path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/>`)

	// 星群（sparkles，用于解释卡片标题）
	iconSparkles = lucide(`<path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/><path d="M20 2v4"/><path d="M22 4h-4"/><circle cx="4" cy="20" r="2"/>`)

	// 勾选（check）
	iconCheck = lucide(`<path d="M20 6 9 17l-5-5"/>`)

	// 折叠箭头（chevron-right / chevron-down）
	iconChevronRight = lucide(`<path d="m9 18 6-6-6-6"/>`)
	iconChevronDown  = lucide(`<path d="m6 9 6 6 6-6"/>`)

	// 视图切换（columns-2 / rows-3）
	iconSplit   = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M12 3v18"/>`)
	iconUnified = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M21 9H3"/><path d="M21 15H3"/>`)

	// 分支对比（git-compare）
	iconGitCompare = lucide(`<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><path d="M11 18H8a2 2 0 0 1-2-2V9"/>`)

	// 分支（git-branch）
	iconGitBranch = lucide(`<line x1="6" x2="6" y1="3" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>`)

	// 交换分支（arrow-left-right）
	iconSwap = lucide(`<path d="m16 3 4 4-4 4"/><path d="M20 7H4"/><path d="m8 21-4-4 4-4"/><path d="M4 17h16"/>`)

	// Pull Request（git-pull-request）
	iconGitPullRequest = lucide(`<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><line x1="6" x2="6" y1="9" y2="21"/>`)

	// 盾牌 / 安全预检（shield）
	iconShield = lucide(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>`)
)

// drawIcon 是绘制指定尺寸与颜色的矢量图标的通用控件。
func drawIcon(c *ui.Context, svg *ui.SVG, size float32, color ui.Color) *ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Icon(svg, r, color)
	})
}
