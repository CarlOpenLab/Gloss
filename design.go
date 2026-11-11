package main

// 设计 token —— DESIGN.md 里那些「规范说是几就是几」的数值。
//
// 之前这些数是散在各处的字面量：圆角出现过 1、3、4、5、6、7、8、10、12、14、99
// 十一个值，字号出现过 9 到 20 之间十四个档位，而规范只认 6 个圆角、6 档字号。
// 散着写的代价不是「不整齐」，是**改不动**：想把按钮圆角统一调一档，得先把
// 「哪些 5 是按钮、哪些 5 是徽章」从二十个调用点里逐个认出来。
//
// 所以这里把规范里的数值收成常量，和 theme.go 的调色板同一层：颜色一处定义、
// 形状一处定义、字号一处定义。调用点写 token 名而不是数字，规范改了改这里。
//
// 配套的三条规则（§2.5 / §5），改界面时照着套：
//
//  1. **芯片 / 徽章**用 `pal.chip` 洗色打底 + 1px `pal.hairline` 边 + `pal.inkMuted`
//     文字，语义靠一颗 6px 圆点或一个微型图标区分。不要整块涂成琥珀、亮蓝、
//     翠绿 —— 分支、远程、标签挤在一行里就是文档点名的「彩色药丸大爆炸」。
//  2. **横幅**（失败、卡住、结果提示）保留语义底色，`semantic-danger/warning`
//     压到 0.10 上下。这是语义色，§5 禁的是**非语义**的装饰性着色。
//  3. **状态**（悬停、选中、按压、禁用）只用洗色阶梯：幽灵控件静息透明、悬停
//     `rowHover`、按下 / 选中 `rowSelected`；实心控件静息 `chip`、悬停与按下
//     `rowSelected`。工作卡和侧栏是亚克力（半透明），不透明的 Surface 1~4 铺上去
//     就是一块实心挡板，只留给浮层、弹窗和代码画布（TestAcrylicLayersUseWashes
//     守这一条）。强调色留给主 CTA、焦点环、当前分支圆点、侧栏指示条，全屏同时
//     出现的实心强调不超过 1~2 处。
const (
	// 圆角（§3）。表格里每个 token 都绑了适用组件，别跨着用：
	// xs 是极小标签，sm 是芯片与分段单元，md 是「全部通用按钮、输入框、
	// 搜索栏」，lg 是卡片与浮层，xl 是模态。
	radiusXS   = 4.0  // 极小代码标签、勾选框、微型图标底板
	radiusSM   = 6.0  // 工具/幽灵按钮、分支芯片、树节点选中底带、分段控件单元
	radiusMD   = 8.0  // 全部通用按钮、文本输入框、下拉框、搜索栏
	radiusLG   = 12.0 // 面板卡片、悬浮卡片、下拉浮层
	radiusXL   = 16.0 // 模态弹窗（设置、PR、分支对话框）
	radiusPill = 9999.0

	// 字号（typography）。六档之外的值不要新引入 —— 密集列表里那些 10 / 10.5
	// 是既有的元信息档，本轮按角色收敛时没动它们，但新写的界面用这六档。
	textDisplay  = 20.0 // 空状态 / hero 标题
	textHeadline = 16.0 // 对话框标题、页面标题
	textBody     = 13.0 // 正文
	textBodySM   = 12.0 // 次正文
	textButton   = 12.5 // 按钮文字
	textCaption  = 11.0 // 辅助元信息、快捷键提示
	textMono     = 12.0 // 等宽（代码、hash、行号）

	// 字重。规范用 600 表标题、500 表按钮，而代码里原先一律用 Bold()（700）——
	// 700 在 11px 的徽章上和 13px 的正文上一样黑，整屏的层级就没了。
	weightRegular  = 400
	weightMedium   = 500
	weightSemibold = 600

	// defSize 是代码的默认字号（diff 与编辑器共用）。⌘+ / ⌘− 在这基础上加减，
	// 用户没调过时就是它。
	defSize = 12.5

	// —— 界面字号（上面那六档 + rowChipText 的整体缩放）——
	//
	// 系统那条 TextScale 路走不通：框架只把它乘到主题自己的 t.FontSize 上
	// （ui/theme.go 的 follow），而 Gloss 界面上三百多处文字写的是上面的
	// token，谁都不看 t.FontSize。用户在 Windows 设置里把文本放大到 125%，
	// Gloss 一个像素都不动 —— 所以界面字号是 Gloss 自己的一个设置，跟着
	// 用户走，而不是跟着系统走（issue #7 问题二）。
	//
	// 存的是**档位**而不是倍数：倍数是个浮点，写进 JSON 之后用户手改一位就
	// 落不到档上，设置页显示的那一格也会对不上。档位是整数，拨过去就停在那。
	uiFontSizeMin     float32 = 0 // 默认（规范定值）
	uiFontSizeDefault float32 = 1 // 小一档
	uiFontSizeMax     float32 = 2 // 大一档
	uiFontSizeGear    float32 = 0.5
)

// uiFontGear 是当前生效的界面字号档位，包级 —— 界面上的字号每帧现算，而算它的
// 地方一大半是没有 *app 的自由函数（settingsButton、riskFindingCard、headline
// 这些，共 46 处），所以档位和 SetLocale/CurrentLocale 一样是包级状态。
var uiFontGear = uiFontSizeDefault

// uiFontSize 把一档字号基准按当前档位缩放。
//
// 缩放**只在这里乘一次**，而且每次都从入参那个基准现算：基准是 design.go 的
// 常量（不会自己变大），结果也不写回去，所以同一个基准取多少次都是同一个数。
// 反过来说，如果某个调用点把 uiFontSize 的结果存进跨帧复用的对象、下一帧又
// 拿它当基准，字号就会一帧涨一截 —— TestUIFontScaleIsNotCompounded 守这一条。
func uiFontSize(base float32) float32 {
	return base * uiFontScaleFactor()
}

// uiFontScaleFactor 是当前档位对应的缩放倍数。
func uiFontScaleFactor() float32 {
	return 1 + (uiFontGear-uiFontSizeDefault)*uiFontSizeGear
}

// setUIFontSizeStep 设置界面字号档位，超出范围的收敛到范围内。
//
// 档位会从配置文件里读回来（用户手改过、旧版本写过别的数），一个 99 落进来，
// 下一帧字号就是 50 倍 —— 界面直接涨出窗口。返回收敛后的档位，调用方不必
// 再读一遍才知道实际落在哪一格。
func setUIFontSizeStep(step float32) float32 {
	uiFontGear = clamp(step, uiFontSizeMin, uiFontSizeMax)
	return uiFontGear
}

// currentUIFontSizeStep 读当前档位（设置页与菜单显示用）。
func currentUIFontSizeStep() float32 { return uiFontGear }

// uiFontGearLabel 是设置页那一格里显示的文字。
//
// 显示「小 / 默认 / 大」而不是「0.5× / 1.0× / 1.5×」：用户调的是界面大小，
// 倍数对他没有意义，而每一档具体是多少像素又随基准不同而不同（正文 13、
// 芯片 10）。档位名是唯一一个三档都说得通的标签。
func uiFontGearLabel(step float32) string {
	switch step {
	case uiFontSizeMin:
		return T("默认", "Default")
	case uiFontSizeMax:
		return T("大", "Large")
	default:
		return T("小", "Small")
	}
}
