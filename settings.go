package main

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/explain"
	"github.com/CarlOpenLab/Gloss/internal/llm"
)

//go:embed resources/icon-pure-black.png
var iconPureBlackPNG []byte

//go:embed resources/icon-pure-white.png
var iconPureWhitePNG []byte

var (
	iconBlackBitmap *ui.Bitmap
	iconWhiteBitmap *ui.Bitmap
)

func getIconBitmaps() (*ui.Bitmap, *ui.Bitmap) {
	if iconBlackBitmap == nil && len(iconPureBlackPNG) > 0 {
		iconBlackBitmap, _ = ui.DecodeBitmap(iconPureBlackPNG)
	}
	if iconWhiteBitmap == nil && len(iconPureWhitePNG) > 0 {
		iconWhiteBitmap, _ = ui.DecodeBitmap(iconPureWhitePNG)
	}
	return iconBlackBitmap, iconWhiteBitmap
}

type settingsTab int

const (
	settingsTabModels  settingsTab = 0
	settingsTabGeneral settingsTab = 1
	settingsTabAbout   settingsTab = 2
	settingsTabPrompts settingsTab = 3
	settingsTabUpdates settingsTab = 4
	settingsTabGit     settingsTab = 5
)

// modelTagInfo 表示模型及其特性标签（如：视觉、推理、代码）。
type modelTagInfo struct {
	ID   string
	Tags []string
}

// preset 是一个常见供应商的预设：接口类型、Base URL、模型及推荐模型列表。
type preset struct {
	Label  string
	Type   string
	Base   string
	Model  string
	Models []modelTagInfo
}

// providerPresets 覆盖常用服务商及本地网关，按开箱即用性排序。
var providerPresets = []preset{
	{
		Label: "Magpie",
		Type:  "openai",
		Base:  "http://127.0.0.1:3425/v1",
		Model: "opencode-go/deepseek-v4.1-flash",
		Models: []modelTagInfo{
			{"opencode-go/deepseek-v4.1-flash", []string{"视觉", "推理"}},
		},
	},
	{
		Label: "Anthropic",
		Type:  "anthropic",
		Base:  "",
		Model: llm.DefaultModel,
		Models: []modelTagInfo{
			{"claude-sonnet-4.5", []string{"视觉", "推理"}},
			{"claude-3-7-sonnet", []string{"视觉", "推理"}},
			{"claude-3-5-haiku", []string{"快速", "代码"}},
			{"claude-opus-4", []string{"推理"}},
		},
	},
	{
		Label: "DeepSeek",
		Type:  "openai",
		Base:  "https://api.deepseek.com",
		Model: "deepseek-chat",
		Models: []modelTagInfo{
			{"deepseek-chat", []string{"通用", "代码"}},
			{"deepseek-reasoner", []string{"深度推理", "代码"}},
		},
	},
	{
		Label: "Kimi（Moonshot）",
		Type:  "openai",
		Base:  "https://api.moonshot.cn/v1",
		Model: "kimi-k2-0905-preview",
		Models: []modelTagInfo{
			{"kimi-k2-0905-preview", []string{"长文本", "推理"}},
			{"moonshot-v1-32k", []string{"长文本"}},
		},
	},
	{
		Label: "通义千问（DashScope）",
		Type:  "openai",
		Base:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Model: "qwen-plus",
		Models: []modelTagInfo{
			{"qwen-plus", []string{"推理", "代码"}},
			{"qwen-max", []string{"高智商", "推理"}},
			{"qwen2.5-coder-32b", []string{"代码", "推理"}},
		},
	},
	{
		Label: "智谱 GLM",
		Type:  "openai",
		Base:  "https://open.bigmodel.cn/api/paas/v4",
		Model: "glm-4.5",
		Models: []modelTagInfo{
			{"glm-4.5", []string{"视觉", "推理"}},
			{"glm-4-flash", []string{"快速", "免费"}},
			{"glm-4-plus", []string{"高智商", "推理"}},
		},
	},
	{
		Label: "ByteDance / 火山引擎",
		Type:  "openai",
		Base:  "https://ark.cn-beijing.volces.com/api/v3",
		Model: "doubao-pro-32k",
		Models: []modelTagInfo{
			{"doubao-pro-32k", []string{"视觉", "推理"}},
			{"doubao-lite-32k", []string{"快速", "轻量"}},
		},
	},
	{
		Label: "OpenRouter",
		Type:  "openai",
		Base:  "https://openrouter.ai/api/v1",
		Model: "anthropic/claude-sonnet-4.5",
		Models: []modelTagInfo{
			{"anthropic/claude-sonnet-4.5", []string{"视觉", "推理"}},
			{"deepseek/deepseek-r1", []string{"深度推理"}},
			{"meta-llama/llama-3.3-70b-instruct", []string{"开源", "通用"}},
		},
	},
	{
		Label: "Ollama（本地）",
		Type:  "openai",
		Base:  "http://localhost:11434/v1",
		Model: "qwen2.5-coder",
		Models: []modelTagInfo{
			{"qwen2.5-coder", []string{"代码", "本地"}},
			{"deepseek-r1:8b", []string{"推理", "本地"}},
			{"llama3.2", []string{"通用", "轻量"}},
		},
	},
	{
		Label: "302.AI",
		Type:  "openai",
		Base:  "https://api.302.ai/v1",
		Model: "deepseek-chat",
		Models: []modelTagInfo{
			{"deepseek-chat", []string{"推理", "代码"}},
			{"claude-3-5-sonnet", []string{"视觉", "代码"}},
		},
	},
	{
		Label: "自定义 OpenAI 兼容",
		Type:  "openai",
		Base:  "",
		Model: "",
	},
}

// openSettings 打开设置对话框，在配置的副本上编辑 ——
// 点「保存」之前不动真配置。
func (a *app) openSettings() {
	a.editCfg = a.store.Config()
	if len(a.editCfg.Providers) == 0 {
		a.editCfg.Providers = []llm.Provider{{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel}}
	}
	// Active 可以为 -1（用户把当前生效的供应商关了）——那种状态原样保留，
	// 只把「编辑中」钳到一条真实存在的供应商上。
	if a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = len(a.editCfg.Providers) - 1
	}
	a.editIdx = a.editCfg.Active
	if a.editIdx < 0 {
		a.editIdx = 0
	}
	a.editThemeMode = a.themeMode
	a.editLocale = a.locale
	a.showAPIKey = false

	// 初始化提示词编辑状态
	a.editCommitPrompt = a.editCfg.EffectiveCommitPrompt(explain.CommitSystemPrompt)
	a.editBranchPrompt = a.editCfg.EffectiveBranchPrompt(explain.BranchSystemPrompt)
	a.providerSearch = ""
	a.modelSearch = ""
	a.settingsTab = settingsTabModels

	// 「Git 配置」那一页每次打开都重读：那些文件命令行和别的客户端也在改，
	// 留一份上次打开时的旧值摆在面板里，等于骗人。在飞的那一次读作废。
	a.gitCfg.seq++
	a.gitCfg.loaded = false
	a.gitCfg.loading = false
	a.gitCfg.groups = nil
	a.gitCfg.err = ""

	a.settingsOn = true
	a.probeMagpie()
}

// probeMagpie 异步探测本地 Magpie 网关是否在线并拉取可用模型列表。
func (a *app) probeMagpie() {
	if a.magpieProbing {
		return
	}
	a.magpieProbing = true
	a.run(func() {
		models, ok := fetchMagpieModels()
		a.dispatch(func() {
			a.magpieProbing = false
			a.magpieOnline = ok
			a.magpieModels = models
		})
	})
}

// fetchMagpieModels 尝试探测本地 127.0.0.1:3425 的 Magpie 网关服务。
func fetchMagpieModels() ([]string, bool) {
	client := &http.Client{Timeout: 350 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:3425/v1/models")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, false
	}
	var models []string
	for _, m := range res.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, true
}

// connectMagpie 一键连接或切换到本地 Magpie 网关。
func (a *app) connectMagpie() {
	for i, p := range a.editCfg.Providers {
		if isMagpie(p.BaseURL) {
			a.editIdx = i
			a.editCfg.Active = i
			return
		}
	}
	model := "opencode-go/deepseek-v4.1-flash"
	if len(a.magpieModels) > 0 {
		model = a.magpieModels[0]
		for _, m := range a.magpieModels {
			if strings.Contains(strings.ToLower(m), "deepseek") {
				model = m
				break
			}
		}
	}
	a.addProvider(preset{
		Label: "Magpie",
		Type:  "openai",
		Base:  "http://127.0.0.1:3425/v1",
		Model: model,
	})
}

// isMagpie 判断是否为 Magpie 本地网关。
func isMagpie(base string) bool {
	return strings.Contains(base, ":3425") || strings.Contains(strings.ToLower(base), "magpie")
}

// findExistingPreset 检查给定的预设是否已经在列表中，返回对应下标（未找到返回 -1）。
func findExistingPreset(providers []llm.Provider, ps preset) int {
	for i, p := range providers {
		// 1. Magpie 本地网关按地址特征匹配
		if isMagpie(ps.Base) && isMagpie(p.BaseURL) {
			return i
		}
		// 2. 名称完全一致（不区分大小写）
		if strings.EqualFold(p.Name, ps.Label) {
			return i
		}
		// 3. 拥有指定非空 BaseURL 且地址相同
		if ps.Base != "" && strings.TrimRight(strings.ToLower(p.BaseURL), "/") == strings.TrimRight(strings.ToLower(ps.Base), "/") {
			return i
		}
		// 4. Anthropic 官方服务匹配
		if ps.Type == "anthropic" && p.Type == "anthropic" && (p.BaseURL == "" || p.BaseURL == ps.Base) && strings.Contains(strings.ToLower(p.Name), "anthropic") {
			return i
		}
	}
	return -1
}

// settingsDialog 是偏好与模型设置窗口。
//
// 它照 agent-desktop-ui 的窗口结构搭：一整块柔和底（pal.sideBg）上，
// 左边 212px 的导航栏，右边一块与四周都留 8px 的浮起卡片（14px 圆角）。
// 卡片自己带 44px 顶栏和 52px 底栏，中间那页滚动 —— 页与页之间只换内容，
// 不换版式。
// settingsMargin 是拖开的设置窗口离窗口边缘留的缝。
const settingsMargin = float32(12)

func (a *app) settingsDialog(c *ui.Context, pal palette) {
	if !a.settingsOn {
		return
	}
	winW, winH := c.Size()
	panelW := settingsPanelW(winW)
	panelH := clamp(winH-40, 360, 680)
	// 拖动量是从中心（对话框的起始位置）算起的：边界就是让它整块留在
	// 窗口里、离四边留 settingsMargin 的那段余量。
	bounds := ui.Rect{
		X: settingsMargin - (winW-panelW)/2,
		Y: settingsMargin - (winH-panelH)/2,
		W: max(0, winW-panelW-2*settingsMargin),
		H: max(0, winH-panelH-2*settingsMargin),
	}

	open := true
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		// 表单对话框：点背景不关（Esc、标题栏的 × 和底栏的「取消 / 保存」
		// 照旧）—— 供应商表单、API Key 打到一半，被页面上误触的一下丢掉太亏。
		// 见 specModalSticky。
		if back.Clicked() {
			open = true
		}
		// 不再压暗整页：设置是浮在页面上的窗口，不是盖住页面的幕布 ——
		// 改外观、字号、终端配色能当场在底下的终端里看到；按住它的导航栏
		// 或卡片顶栏就能把它挪开。
		back.Background(ui.Transparent)
		window := panel.Width(panelW).Height(panelH).Radius(radiusXL).
			Background(pal.sideBg).Clip()
		floatingSurface(window, pal)
		window.Draggable(&a.settingsDrag, ui.DraggableOptions{Bounds: &bounds, Rigid: true}).
			Children(func() { a.settingsBody(c, pal) })
	})
	if !open {
		a.settingsOn = false
	}
}

// settingsPanelW 是设置窗口的宽度：窗口两侧各留 20，最小 640、最大 980。
//
// 上限从 860 放到 980：模型设置是卡中卡（供应商清单 + 表单），860 时右边
// 只剩 360 上下，「供应商名称」这种标签都会被截成「Provider N...」。
func settingsPanelW(winW float32) float32 { return clamp(winW-40, 640, 980) }

// settingsSidebarItem 是左栏的一条导航。选中态用表面阶梯 + 一点抬升，
// 强调色只落在图标上：全屏同时亮着的「选中」已经够多了。
func settingsSidebarItem(c *ui.Context, pal palette, label string, icon *ui.SVG, selected bool, onClick func()) {
	btn := ui.ButtonBase(c).Height(32).FillWidth().Radius(radiusMD).
		Padding(0, 10).Gap(10).AlignItems(ui.Center).Justify(ui.Start).
		Cursor(ui.CursorPointer).Label(label)

	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.rowSelected
		selectedSurface(btn, pal)
	case btn.Hovered():
		bg = pal.rowHover
	case btn.Pressed():
		bg = pal.rowHover
	}
	btn.Background(btn.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))

	btn.Children(func() {
		col := pal.textMuted
		if selected {
			col = pal.accent
		}
		drawIcon(c, icon, 15, col)
		ui.Text(c, label).FontSize(13.5).TextColor(pal.text).SingleLine()
	})
	if btn.Clicked() {
		onClick()
	}
}

// settingsBody 是设置窗口的内容：导航栏 + 浮起卡片。
func (a *app) settingsBody(c *ui.Context, pal palette) {
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		// —— 左：212px 导航栏，和窗口底同色，不画竖线 ——
		ui.Column(c).Width(settingsNavW).Shrink(0).FillHeight().
			Padding(0, 12, 0, 12).Gap(2).Children(func() {
			ui.Row(c).Height(settingsBarH).Shrink(0).FillWidth().
				Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
				// drawIcon(c, iconSettings, 15, pal.textMuted)
				ui.Text(c, T("设置", "Settings")).FontSize(14).
					FontWeight(weightSemibold).TextColor(pal.text)
			})

			ui.Box(c).Height(6).Shrink(0)
			// 通用偏好在模型设置前头：这一页改的是界面本身（外观、语言、
			// 拓扑图配色、终端字体），打开设置的人十次里有九次是来找它的。
			settingsSidebarItem(c, pal, T("通用偏好", "Preferences"), iconPalette,
				a.settingsTab == settingsTabGeneral, func() { a.settingsTab = settingsTabGeneral })
			settingsSidebarItem(c, pal, T("模型设置", "Model Settings"), iconSparkles,
				a.settingsTab == settingsTabModels, func() { a.settingsTab = settingsTabModels })
			settingsSidebarItem(c, pal, T("提示词设置", "Prompts"), iconSliders,
				a.settingsTab == settingsTabPrompts, func() { a.settingsTab = settingsTabPrompts })
			settingsSidebarItem(c, pal, T("Git 配置", "Git Config"), iconGitBranch,
				a.settingsTab == settingsTabGit, func() { a.settingsTab = settingsTabGit })

			ui.Text(c, T("系统", "System")).FontSize(textCaption).
				FontWeight(weightSemibold).TextColor(pal.textFaint).
				Padding(14, 8, 4, 8)

			settingsSidebarItem(c, pal, T("更新", "Updates"), iconRefresh,
				a.settingsTab == settingsTabUpdates, func() { a.settingsTab = settingsTabUpdates })
			settingsSidebarItem(c, pal, T("关于 Gloss", "About Gloss"), iconEye,
				a.settingsTab == settingsTabAbout, func() { a.settingsTab = settingsTabAbout })

			ui.Spacer(c)

			ui.Row(c).Padding(8, 8).Children(func() {
				ui.Text(c, "Gloss "+appVersionLabel()).FontSize(textCaption).
					TextColor(pal.textFaint)
			})
		})

		// —— 右：浮起的内容卡片，四周留 8px ——
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().
			Padding(settingsCardGap).Children(func() {
			card := ui.Column(c).Grow(1).FillWidth().MinHeight(0).
				Radius(settingsCardRadius).Background(pal.card).Clip()
			raisedSurface(card, pal)
			card.Children(func() {
				settingsCardHeader(c, pal, a.settingsTitle(), func() {
					settingsIconButton(c, pal, T("关闭设置", "Close Settings"), T("关闭设置 Esc", "Close settings Esc"),
						iconX, func() { a.settingsOn = false })
				})

				ui.Box(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
					a.settingsPage(c, pal)
				})

				// 底栏：左边一句本地存储说明，右边取消 / 保存。
				ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.rowDivider)
				ui.Row(c).Height(settingsFooterH).Shrink(0).FillWidth().
					Padding(0, 18).Gap(10).AlignItems(ui.Center).Children(func() {
					drawIcon(c, iconShield, 13, pal.textFaint)
					ui.Text(c, T("设置存储于本地配置文件，所有请求由本机直连服务端",
						"Preferences stored locally; requests connect directly from this device")).
						FontSize(textBodySM).TextColor(pal.textMuted).SingleLine().MinWidth(0).Shrink(1)
					ui.Spacer(c)
					if settingsButton(c, pal, T("取消", "Cancel"), false, false).Clicked() {
						a.settingsOn = false
					}
					if settingsButton(c, pal, T("保存", "Save"), true, false).Clicked() {
						a.applySettings()
					}
				})
			})
		})
	})
}

// settingsTitle 是当前页在卡片顶栏里的标题。
func (a *app) settingsTitle() string {
	switch a.settingsTab {
	case settingsTabGeneral:
		return T("通用偏好", "Preferences")
	case settingsTabPrompts:
		return T("提示词设置", "Prompts")
	case settingsTabUpdates:
		return T("软件更新", "Software Update")
	case settingsTabAbout:
		return T("关于 Gloss", "About Gloss")
	case settingsTabGit:
		return T("Git 配置", "Git Config")
	default:
		return T("模型设置", "Model Settings")
	}
}

// settingsPage 把当前页画进卡片中间那块空间。每页自己管滚动与内边距，
// 因为模型设置是双栏、提示词是长文本，硬套同一套留白反而别扭。
func (a *app) settingsPage(c *ui.Context, pal palette) {
	switch a.settingsTab {
	case settingsTabGeneral:
		a.generalSettingsPane(c, pal)
	case settingsTabPrompts:
		a.promptsSettingsPane(c, pal)
	case settingsTabAbout:
		a.aboutSettingsPane(c, pal)
	case settingsTabUpdates:
		a.updatesSettingsPane(c, pal)
	case settingsTabGit:
		a.gitSettingsPane(c, pal)
	default:
		a.modelsSettingsPane(c, pal)
	}
}

// generalSettingsPane 是「通用偏好」：外观、图标、语言、Diff 与代码、
// 拓扑图配色、终端与远程同步。整页是几块分组面板（settingsGroup），
// 组里一行一件事，右侧放控件 —— 和 macOS 系统设置同一套语法。
func (a *app) generalSettingsPane(c *ui.Context, pal palette) {

	// 行里的小步进器：一个「−」、当前值、一个「＋」，最后的重置是可选。
	stepper := func(value string, decLabel, incLabel string, dec, inc func(), resetLabel string, reset func()) {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if settingsMiniButton(c, pal, decLabel, "−", false).Clicked() {
				dec()
			}
			ui.Text(c, value).FontSize(12.5).FontFeatures("tnum").
				Width(74).TextAlign(ui.Center).TextColor(pal.textMuted).SingleLine()
			if settingsMiniButton(c, pal, incLabel, "+", false).Clicked() {
				inc()
			}
			if resetLabel != "" {
				if settingsMiniButton(c, pal, resetLabel, T("重置", "Reset"), false).Clicked() {
					reset()
				}
			}
		})
	}

	ui.Scroll(c).FillWidth().FillHeight().
		Padding(settingsPadTop, settingsPadX, settingsPadBot, settingsPadX).Children(func() {
		ui.Column(c).Gap(22).FillWidth().MaxWidth(620).Children(func() {

			// —— 外观 ——
			settingsGroup(c, pal, T("外观主题", "Appearance Theme"), "", func() {
				settingsRow(c, pal, T("主题", "Theme"),
					T("跟随系统，或固定为浅色 / 深色。", "Follow the system, or pin light or dark."),
					func() {
						modes := []string{"system", "light", "dark"}
						idx := 0
						for i, m := range modes {
							if a.themeMode == m {
								idx = i
							}
						}
						seg := settingsSegmented(c, pal, &idx,
							T("跟随系统", "System"), T("浅色模式", "Light"), T("深色模式", "Dark"))
						seg.OnChange(func() { a.setThemeMode(modes[idx]) })
					})
			})

			// —— 应用图标（只在 macOS 上摆） ——
			if isMacPlatform() {
				settingsGroup(c, pal, T("应用图标", "Application Icon"), "", func() {
					settingsRow(c, pal, T("图标风格", "Icon Style"),
						T("纯黑曜石或纯白瓷，两套都是中性单色。改的是 Dock 与系统「关于」里的那一枚，免重启即时生效。",
							"Obsidian black or porcelain white, both strictly neutral. Changes the Dock and About icon; applies immediately."),
						func() {
							icons := []string{"black", "white"}
							idx := 0
							if a.appIcon == "white" {
								idx = 1
							}
							seg := settingsSegmented(c, pal, &idx,
								T("纯黑极简", "Pure Black"), T("纯白极简", "Pure White"))
							seg.OnChange(func() { a.setAppIcon(icons[idx]) })
						})
				})
			}

			// —— 界面语言 ——
			settingsGroup(c, pal, T("界面语言", "Display Language"), "", func() {
				settingsRow(c, pal, T("语言", "Language"),
					T("切换后即时生效。", "Applies immediately."),
					func() {
						langs := []string{"zh", "en"}
						idx := 0
						if a.locale == "en" {
							idx = 1
						}
						seg := settingsSegmented(c, pal, &idx, "简体中文", "English")
						seg.OnChange(func() { a.setLocale(langs[idx]) })
					})
			})

			// —— Diff 与代码视图 ——
			settingsGroup(c, pal, T("Diff 与代码视图", "Diff & Code View"), "", func() {
				settingsRow(c, pal, T("代码字体大小", "Code Font Size"), "",
					func() {
						stepper(Tf("%.1f px", "%.1f px", a.codeSizeValue()),
							T("缩小代码字号", "Smaller code font"),
							T("放大代码字号", "Larger code font"),
							func() { a.setCodeSize(a.codeSizeValue() - codeSizeStep) },
							func() { a.setCodeSize(a.codeSizeValue() + codeSizeStep) },
							T("重置代码字号", "Reset code font"),
							func() { a.setCodeSize(0) })
					})
			})

			// —— 拓扑图配色 ——
			settingsGroup(c, pal, T("拓扑图配色", "Commit Graph Colors"),
				T("提交列表左边那几条泳道线的颜色。0 号是主干，也用在「当前分支」的圆点上；自定义只改拓扑图，状态字母和分支芯片的亮色不动。",
					"Colors of the lane lines in the commit list. Lane 0 is the mainline, shared with the current-branch dot; overrides apply to the graph only, never to status letters or ref chips."),
				func() {
					settingsRow(c, pal, T("方案", "Preset"),
						T("经典是莫兰迪 8 色；鲜艳提高一档彩度；灰阶不靠色相分辨。",
							"Classic is the Morandi 8; Vivid raises chroma; Grayscale relies on lightness only."),
						func() {
							presets := []string{"classic", "vivid", "mono"}
							idx := 0
							for i, id := range presets {
								if a.graphPreset == id || (a.graphPreset == "" && i == 0) {
									idx = i
								}
							}
							seg := settingsSegmented(c, pal, &idx,
								T("经典配色", "Classic"), T("鲜艳配色", "Vivid"), T("灰阶配色", "Grayscale"))
							seg.OnChange(func() { a.setGraphPreset(presets[idx]) })
						})

					// 按明暗各摆一排色井：同一支色落在深色底和浅色底上的
					// 可读性差得很远，只显示当前那一排的话，另一份得切一次
					// 外观才能调。
					for _, row := range []struct {
						dark   bool
						label  string
						detail string
					}{
						{true, T("深色外观", "Dark appearance"), T("深色背景上的 8 个色号", "The eight lanes on dark")},
						{false, T("浅色外观", "Light appearance"), T("浅色背景上的 8 个色号", "The eight lanes on light")},
					} {
						row := row
						settingsRow(c, pal, row.label, row.detail, func() {
							laneCols := a.laneSet().lanes(row.dark)
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								for i := 0; i < laneCount; i++ {
									i := i
									col := laneCols[i]
									tip := Tf("第 %d 号泳道线的颜色", "Color of lane %d", i)
									if i == 0 {
										tip = T("主干（0 号）的颜色", "Mainline (lane 0) color")
									}
									ui.Column(c).Gap(2).AlignItems(ui.Center).Children(func() {
										ui.Text(c, Tf("%d", "%d", i)).FontSize(9).
											TextColor(pal.textFaint)
										if ui.ColorWell(c, &col).Label(tip).Tooltip(tip).Changed() {
											a.setLaneOverride(row.dark, i, hexOf(col))
										}
									})
								}
							})
						})
					}

					settingsRow(c, pal, T("自定义", "Overrides"),
						T("把调过的色号恢复成方案自带的 8 色。", "Reset the eight lanes to the preset's own colors."),
						func() {
							btn := settingsButton(c, pal, T("清除自定义颜色", "Clear Custom Colors"), false,
								!a.hasLaneOverrides())
							if btn.Clicked() && a.hasLaneOverrides() {
								a.clearLaneOverrides()
							}
						})
				})

			// —— 内置终端 ——
			settingsGroup(c, pal, T("内置终端", "Built-in Terminal"),
				T("字体族从下拉里挑一个等宽字体：这台机器上装着的排在前面（Nerd Font 最前），没装的留在后面当候选，选了会退到系统等宽。改动对已经开着的会话立即生效。",
					"Pick a monospace family: what this machine has installed comes first (Nerd Fonts on top), uninstalled ones stay below as candidates and fall back to the system monospace. Changes reach open sessions at once."),
				func() {
					// 要用的那一帧才去扫已装字体（见 ensureFontScan）。
					a.ensureFontScan()

					settingsRow(c, pal, T("字体族", "Font Family"),
						T("没装的字体退回系统等宽。", "Uninstalled fonts fall back to the system monospace."),
						func() {
							family := a.termFontFamily
							if family == "" {
								family = termFontGenericMono
							}
							combo := ui.Combobox(c, &family, a.termFontOptions()).MaxWidth(230).
								Label(T("终端字体族", "Terminal font family"))
							if combo.Changed() {
								a.setTermFontFamily(family)
							}
							if !a.fontScanDone {
								ui.Text(c, T("读取字体中…", "Reading fonts…")).
									FontSize(textCaption).TextColor(pal.textFaint)
							}
							if settingsMiniButton(c, pal, T("默认字体", "Default font"),
								T("默认", "Default"), a.termFontFamily == "" || a.termFontFamily == termFontGenericMono).Clicked() {
								a.setTermFontFamily("")
							}
						})

					settingsRow(c, pal, T("字号", "Font Size"), "",
						func() {
							stepper(Tf("%.0f px", "%.0f px", a.termFontSizeValue()),
								T("缩小终端字号", "Smaller terminal font"),
								T("放大终端字号", "Larger terminal font"),
								func() { a.setTermFontSize(a.termFontSizeValue() - termFontStep) },
								func() { a.setTermFontSize(a.termFontSizeValue() + termFontStep) },
								T("重置字号", "Reset size"),
								func() { a.setTermFontSize(0) })
						})
				})

			// —— 远程同步 ——
			settingsGroup(c, pal, T("远程同步", "Remote Sync"), "", func() {
				on := a.autoFetch
				settingsRow(c, pal, T("后台自动抓取", "Fetch in the Background"),
					T("每 5 分钟对所有远程抓取一次，只更新远程分支，不动工作区。",
						"Fetches all remotes every 5 minutes. Updates remote-tracking refs only; never touches the working tree."),
					func() {
						sw := settingsSwitch(c, pal, T("后台自动抓取开关", "Fetch in the Background switch"), &on)
						if sw.Clicked() {
							a.setAutoFetch(on)
						}
					})
			})

			ui.Spacer(c)
		})
	})
}

// aboutSettingsPane 呈现关于 Gloss 的产品信息、版本与外部链接。
func (a *app) aboutSettingsPane(c *ui.Context, pal palette) {
	bmBlack, bmWhite := getIconBitmaps()
	currentBm := bmBlack
	if a.appIcon == "white" {
		currentBm = bmWhite
	}

	ui.Scroll(c).FillWidth().FillHeight().
		Padding(settingsPadTop, settingsPadX, settingsPadBot, settingsPadX).Children(func() {
		ui.Column(c).Gap(22).FillWidth().MaxWidth(620).Children(func() {

			// —— 品牌 ——
			ui.Row(c).AlignItems(ui.Center).Gap(16).Padding(2, 4, 6, 4).Children(func() {
				if currentBm != nil {
					ui.Box(c).Size(56, 56).Radius(radiusLG).Clip().
						Background(pal.chip).Children(func() {
						ui.Image(c, currentBm).Size(56, 56)
					})
				} else {
					ui.Box(c).Size(56, 56).Radius(radiusLG).Background(pal.chip)
				}
				ui.Column(c).Gap(4).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, "Gloss").FontSize(textDisplay).
							FontWeight(weightSemibold).TextColor(pal.text)
						ver := ui.Row(c).Padding(2, 8).Radius(radiusPill).
							Background(pal.chip).AlignItems(ui.Center)
						ver.Children(func() {
							ui.Text(c, appVersionLabel()).FontSize(textCaption).
								FontWeight(weightMedium).TextColor(pal.textMuted).
								FontFeatures("tnum")
						})
					})
					ui.Text(c, T("原生智能 Git 客户端", "Native Git GUI with AI-powered diff review")).
						FontSize(13).TextColor(pal.textMuted)
				})
			})

			// —— 特性 ——
			settingsGroup(c, pal, T("特性", "Highlights"), "", func() {
				features := []struct {
					icon  *ui.SVG
					title string
					desc  string
				}{
					{iconZap, T("极致轻量与性能", "Lightweight & Fast"),
						T("基于 Go 原生图形引擎，瞬间冷启动与顺滑渲染，极低内存开销",
							"Instant startup and smooth rendering powered by native Go engine")},
					{iconSparkles, T("AI 变更审查", "AI Change Review"),
						T("变更块级风险提示与一键智能提交信息生成",
							"Risk analysis on code changes and Conventional Commits generation")},
					{iconPalette, T("克制优雅的视觉设计", "Focused & Distraction-Free"),
						T("深度集成系统外观，深浅色无缝切换，中性单色图标，零多余杂色",
							"Deep system appearance integration, seamless switching, strictly neutral icons")},
				}
				for i, f := range features {
					if i > 0 {
						ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.rowDivider)
					}
					ui.Row(c).Padding(settingsRowV, 0).Gap(12).AlignItems(ui.Center).FillWidth().
						Children(func() {
							ui.Box(c).Size(26, 26).Radius(radiusSM).Shrink(0).
								Background(pal.control).AlignItems(ui.Center).Justify(ui.Center).
								Children(func() { drawIcon(c, f.icon, 14, pal.textMuted) })
							ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
								ui.Text(c, f.title).FontSize(13.5).TextColor(pal.text)
								ui.Text(c, f.desc).FontSize(textBodySM).TextColor(pal.textMuted).
									LineHeight(1.35)
							})
						})
				}
			})

			// —— 链接 ——
			settingsGroup(c, pal, T("链接", "Links"), "", func() {
				settingsRow(c, pal, T("官方网站", "Official Website"),
					"carlopenlab.github.io/Gloss", func() {
						if settingsButton(c, pal, T("打开", "Open"), false, a.openURL == nil).Clicked() && a.openURL != nil {
							a.openURL("https://carlopenlab.github.io/Gloss/")
						}
					})
				settingsRow(c, pal, "GitHub", "github.com/CarlOpenLab/Gloss", func() {
					if settingsButton(c, pal, T("打开", "Open"), false, a.openURL == nil).Clicked() && a.openURL != nil {
						a.openURL("https://github.com/CarlOpenLab/Gloss")
					}
				})
			})

			ui.Text(c, "Copyright © 2026 CarlOpenLab. Released under the MIT License.").
				FontSize(textBodySM).TextColor(pal.textFaint).Padding(0, 4)
		})
	})
}

// updatePrefs 是「更新」设置页需要的自动更新状态快照。
type updatePrefs struct {
	// available 表示当前构建是否支持自动更新；打包发行版为 true，开发构建为 false。
	available bool
	// automaticChecks / automaticDownloads 对应「自动检查更新 / 自动下载更新」两个开关。
	automaticChecks    bool
	automaticDownloads bool
	// lastCheck 是最近一次检查更新的时间，零值表示还没检查过。
	lastCheck time.Time
}

// 自动更新的读写都收在这几个变量里，而不是直接调 updater 包，理由与 menu.go 里的
// updaterMenuItem 相同：updater 插件在 main.go 通过 mygo.Use 注册，并要求存在事件
// 循环；设置面板的单元测试两样都没有，直接调用会 panic（未注册）或卡住主线程。
var (
	loadUpdatePrefs = func() updatePrefs {
		if !mygo.Updater.Enabled() {
			return updatePrefs{}
		}
		return updatePrefs{
			available:          true,
			automaticChecks:    updater.AutomaticChecks(),
			automaticDownloads: updater.AutomaticDownloads(),
			lastCheck:          updater.LastCheck(),
		}
	}
	saveUpdatePrefs = func(automaticChecks, automaticDownloads bool) {
		updater.SetAutomaticChecks(automaticChecks)
		updater.SetAutomaticDownloads(automaticDownloads)
	}
	checkForUpdatesNow = func() { updater.CheckForUpdates() }
)

// appVersion 返回当前构建的版本号；开发构建（未打包）拿不到版本信息时回退为 dev。
func appVersion() string {
	if v := strings.TrimSpace(mygo.App.Version()); v != "" {
		return v
	}
	return "dev"
}

// appVersionLabel 是展示用的版本号，统一带 v 前缀（v0.3.2 / vdev）。
func appVersionLabel() string {
	return "v" + appVersion()
}

// applyAutomaticChecks 处理「自动检查更新」开关：不可用时忽略，值没变时也不写回。
func applyAutomaticChecks(prefs updatePrefs, on bool) {
	if !prefs.available || on == prefs.automaticChecks {
		return
	}
	saveUpdatePrefs(on, prefs.automaticDownloads)
}

// applyAutomaticDownloads 处理「自动下载更新」开关，规则同 applyAutomaticChecks。
func applyAutomaticDownloads(prefs updatePrefs, on bool) {
	if !prefs.available || on == prefs.automaticDownloads {
		return
	}
	saveUpdatePrefs(prefs.automaticChecks, on)
}

// lastCheckLabel 把最近一次检查时间格式化成设置面板里的展示文案。
func lastCheckLabel(when time.Time) string {
	if when.IsZero() {
		return T("尚未检查", "Not checked yet")
	}
	return when.Local().Format("2006-01-02 15:04")
}

// updatesSettingsPane 呈现自动更新设置：当前版本与检查时间、自动检查/下载开关、手动检查入口。
func (a *app) updatesSettingsPane(c *ui.Context, pal palette) {
	prefs := loadUpdatePrefs()

	ui.Scroll(c).FillWidth().FillHeight().
		Padding(settingsPadTop, settingsPadX, settingsPadBot, settingsPadX).Children(func() {
		ui.Column(c).Gap(22).FillWidth().MaxWidth(620).Children(func() {

			settingsGroup(c, pal, T("版本", "Version"),
				T("更新源：github.com/CarlOpenLab/Gloss 的正式发布版。",
					"Update channel: official releases from github.com/CarlOpenLab/Gloss."),
				func() {
					settingsRow(c, pal, T("当前版本", "Current Version"), "", func() {
						ui.Text(c, appVersionLabel()).FontSize(13).FontWeight(weightMedium).
							TextColor(pal.textMuted).FontFeatures("tnum")
					})
					settingsRow(c, pal, T("上次检查", "Last Checked"), "", func() {
						ui.Text(c, lastCheckLabel(prefs.lastCheck)).FontSize(13).
							TextColor(pal.textMuted).FontFeatures("tnum")
					})
				})

			checksDesc := T("启动时及后台定期检查新版本。",
				"Check for new versions at launch and periodically in the background.")
			if !prefs.available {
				checksDesc = T("当前构建未启用自动更新（开发构建，或由包管理器安装）。",
					"Automatic updates are unavailable in this build.")
			}
			settingsGroup(c, pal, T("自动更新", "Automatic Updates"), checksDesc, func() {
				if !prefs.available {
					settingsRow(c, pal, T("自动检查更新", "Check for Updates Automatically"), "", func() {
						ui.Text(c, T("不可用", "Unavailable")).FontSize(13).TextColor(pal.textFaint)
					})
					settingsRow(c, pal, T("自动下载更新", "Download Updates Automatically"), "", func() {
						ui.Text(c, T("不可用", "Unavailable")).FontSize(13).TextColor(pal.textFaint)
					})
					return
				}
				autoCheck := prefs.automaticChecks
				settingsRow(c, pal, T("自动检查更新", "Check for Updates Automatically"), "", func() {
					sw := settingsSwitch(c, pal, T("自动检查更新", "Check for Updates Automatically"), &autoCheck)
					if sw.Clicked() {
						applyAutomaticChecks(prefs, autoCheck)
					}
				})
				autoDownload := prefs.automaticDownloads
				settingsRow(c, pal, T("自动下载更新", "Download Updates Automatically"),
					T("发现新版本后后台下载，下次启动时安装。",
						"Download new versions in the background and install them on the next launch."), func() {
						sw := settingsSwitch(c, pal, T("自动下载更新", "Download Updates Automatically"), &autoDownload)
						if sw.Clicked() {
							applyAutomaticDownloads(prefs, autoDownload)
						}
					})
			})

			ui.Row(c).Children(func() {
				if settingsButton(c, pal, T("检查更新…", "Check for Updates…"), false, !prefs.available).Clicked() && prefs.available {
					checkForUpdatesNow()
				}
			})
		})
	})
}

// settingDivider is the hairline between two custom rows of a group.
func settingDivider(c *ui.Context, pal palette) {
	ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.rowDivider)
}

// modelsForProvider 汇总当前供应商的所有推荐及可用模型，包含特征标签（视觉、推理、代码等）。
func (a *app) modelsForProvider(p *llm.Provider) []modelTagInfo {
	var list []modelTagInfo

	// 1. 若为本地 Magpie 网关且探测到了实时模型
	if isMagpie(p.BaseURL) && len(a.magpieModels) > 0 {
		for _, m := range a.magpieModels {
			tags := []string{"本地"}
			low := strings.ToLower(m)
			if strings.Contains(low, "deepseek") || strings.Contains(low, "coder") || strings.Contains(low, "code") {
				tags = append(tags, "代码")
			}
			if strings.Contains(low, "r1") || strings.Contains(low, "reason") || strings.Contains(low, "thinking") {
				tags = append(tags, "推理")
			}
			if strings.Contains(low, "vision") || strings.Contains(low, "vl") {
				tags = append(tags, "视觉")
			}
			list = append(list, modelTagInfo{ID: m, Tags: tags})
		}
	}

	// 2. 从预设列表中匹配对应供应商的推荐模型
	for _, ps := range providerPresets {
		if findExistingPreset([]llm.Provider{*p}, ps) == 0 && len(ps.Models) > 0 {
			if len(list) == 0 {
				list = append(list, ps.Models...)
			}
			break
		}
	}

	// 3. 若仍为空且为 Anthropic 协议
	if len(list) == 0 && p.Type == "anthropic" {
		list = []modelTagInfo{
			{"claude-sonnet-4.5", []string{"视觉", "推理"}},
			{"claude-3-7-sonnet", []string{"视觉", "推理"}},
			{"claude-3-5-haiku", []string{"代码", "快速"}},
			{"claude-opus-4", []string{"深度推理"}},
		}
	}

	// 4. 若为通用 OpenAI 兼容协议且未匹配到预设
	if len(list) == 0 && p.Type == "openai" {
		list = []modelTagInfo{
			{"deepseek-chat", []string{"通用", "代码"}},
			{"deepseek-reasoner", []string{"推理"}},
			{"gpt-4o", []string{"视觉", "代码"}},
			{"gpt-4o-mini", []string{"轻量", "快速"}},
		}
	}

	// 5. 确保当前选中的 p.Model 始终在模型列表中展示
	if p.Model != "" {
		found := false
		for _, m := range list {
			if m.ID == p.Model {
				found = true
				break
			}
		}
		if !found {
			list = append([]modelTagInfo{{ID: p.Model, Tags: []string{"当前"}}}, list...)
		}
	}

	return list
}

// —— 设置里的表单控件 ——
//
// 框架的 TextInput / Switch / Segmented 读的是 ui.Theme 的 Surface/Border。
// Gloss 的浅色主题把 Surface 设成纯白（卡片与浮层同色）、Border 设成透明，
// 那三样控件落在白色设置面板上就只剩一圈几乎看不见的淡边：输入框分不出
// 边界，开关关闭态和背景糊在一起，分段控件的轨道也读不出来。
//
// 所以设置里的这几种控件改由这里的函数画，颜色从调色板取，深浅两套各自
// 有明确的填充与描边；只影响设置面板，不动全局主题。

const settingsFieldH = settingsControlH

func settingsFieldBg(pal palette) ui.Color {
	return pal.control
}

func settingsFieldBorder(pal palette) ui.Color {
	if pal.dark {
		return ui.RGBA(255, 255, 255, 0.13)
	}
	return ui.RGBA(0, 0, 0, 0.13)
}

func settingsFieldBorderHover(pal palette) ui.Color {
	if pal.dark {
		return ui.RGBA(255, 255, 255, 0.22)
	}
	return ui.RGBA(0, 0, 0, 0.22)
}

// settingsField 是设置里统一外观的单行输入框（§5.5）：26px 高、7px 圆角，
// 一层「白底 + 1px 内描边」，聚焦时描边转强调色，外面套一圈 3.5px 的柔光。
func settingsField(c *ui.Context, pal palette, value *string, placeholder string, password bool) ui.Element {
	var in ui.Element
	box := ui.Row(c).Height(settingsFieldH).AlignItems(ui.Center).Padding(0, 8).
		Radius(radiusSM).Background(settingsFieldBg(pal)).
		Border(1, settingsFieldBorder(pal)).
		Children(func() {
			in = ui.TextInputBase(c, value).Grow(1).MinWidth(0).FontSize(13).
				Placeholder(placeholder).Label(placeholder)
			if password {
				in.Password()
			}
		})
	switch {
	case in.Focused():
		box.Border(1, pal.accent.Alpha(0.75)).
			Shadow(0, 0, 0, 3.5, pal.accent.Alpha(0.32))
	case box.Hovered():
		box.Border(1, settingsFieldBorderHover(pal))
	}
	return box
}

// settingsSwitch 是设置里的开关（§5.1）：38×22、圆角 11、内边距 2，
// 圆钮 18×18 走 16px 的弹簧。打开时轨道换成强调色。
func settingsSwitch(c *ui.Context, pal palette, label string, on *bool) ui.Element {
	sw := ui.SwitchBase(c, on).Size(38, 22).Shrink(0).
		Cursor(ui.CursorPointer).FocusRing(false)
	if label != "" {
		sw.Label(label)
	}
	pos := sw.AnimateTo("knob", b2f(*on), ui.Animation{
		Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.15},
	})
	off := pal.text.Alpha(0.14)
	if pal.dark {
		off = pal.text.Alpha(0.20)
	}
	track := off.Mix(pal.accent, pos)
	sw.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, track, r.H/2)
		const in = 2.0
		d := r.H - 2*in
		knob := ui.Rect{X: r.X + in + pos*16, Y: r.Y + in, W: d, H: d}
		p.Shadow(knob, d/2, 0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22))
		p.Fill(knob, ui.RGB(255, 255, 255), d/2)
		if sw.FocusVisible() {
			p.FocusRing(r, [4]float32{r.H / 2, r.H / 2, r.H / 2, r.H / 2})
		}
	})
	return sw
}

func settingsSegTrack(pal palette) ui.Color {
	if pal.dark {
		return ui.RGBA(255, 255, 255, 0.07)
	}
	return ui.RGBA(0, 0, 0, 0.06)
}

func settingsSegSelected(pal palette) ui.Color {
	if pal.dark {
		return ui.RGBA(255, 255, 255, 0.20)
	}
	return ui.Hex("#ffffff")
}

// settingsSegmented 是设置里的分段控件（§5.2）：24px 高的轨道、7px 圆角，
// 选中的那格是一枚 5px 圆角的白片，浅浅浮起一层。
func settingsSegmented(c *ui.Context, pal palette, selected *int, labels ...string) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(labels))
	g := parts.Track.Height(settingsControlH - 2).AlignItems(ui.Stretch).Padding(2).
		Radius(radiusSM).Background(settingsSegTrack(pal))
	g.Children(func() {
		for i, label := range labels {
			if i > 0 {
				sep := ui.Box(c).Width(1).Height(12).Shrink(0).AlignSelf(ui.Center)
				if i != *selected && i-1 != *selected {
					sep.Background(pal.text.Alpha(0.14))
				}
			}
			s := parts.Segment(i).FillHeight().Radius(radiusXS+1).
				Padding(0, 12).Center().Cursor(ui.CursorPointer).Label(label)
			sel := i == *selected
			fg := pal.text
			if sel {
				s.Background(settingsSegSelected(pal))
				if pal.dark {
					s.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.25))
				} else {
					s.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.16)).
						Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.04))
				}
			}
			fadeColors(s)
			s.Children(func() {
				ui.Text(c, label).FontSize(12.5).FontWeight(weightMedium).
					TextColor(fg).SingleLine()
			})
		}
	})
	return g
}

// modelsSettingsPane 是模型设置：左边一栏已添加的供应商，右边是选中那家的
// 连接参数与模型清单。两栏之间不画竖线，靠一块软面板和留白分开 —— 和
// 设置窗口其余页同一套表面语言。
func (a *app) modelsSettingsPane(c *ui.Context, pal palette) {
	t := c.Theme()
	if len(a.editCfg.Providers) == 0 {
		a.editCfg.Providers = []llm.Provider{
			{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel},
		}
		a.editCfg.Active = 0
		a.editIdx = 0
	}
	if a.editIdx < 0 || a.editIdx >= len(a.editCfg.Providers) {
		a.editIdx = 0
	}
	if a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = len(a.editCfg.Providers) - 1
	}

	p := &a.editCfg.Providers[a.editIdx]

	// 这一页是卡中卡：左边供应商清单、右边表单。窗口缩小时按卡片实际
	// 宽度把左栏和表单一起收，别让输入框冲出卡片。
	winW, _ := c.Size()
	panelW := settingsPanelW(winW)
	cardW := panelW - settingsNavW - 2*settingsCardGap
	// 清单栏收窄一点：它只放「名称 + 拖动手柄 + 一个圆点」，176 就够；
	// 多出来的宽度全给右边那张表单。
	listW := clamp(cardW*0.34, 196, 224)
	contentW := cardW - listW - 2*settingsPadX
	fieldW := clamp(contentW-80, 120, 230)
	// 表单可用宽度小于 420 时把标签挪到控件上方：标签和 230px 的输入框
	// 并排时只剩 100 出头，「供应商名称」都被截成「Provider N...」。
	stacked := contentW < 420
	// 模型一行放不下「ID + 标签 + 开关」时改成两行：ID 一行、标签一行。
	twoLineModels := contentW < 470

	ui.Row(c).FillWidth().FillHeight().Children(func() {
		// —— 左：供应商列表 ——
		ui.Column(c).Width(listW).Shrink(0).FillHeight().
			Padding(12, 12, 12, 12).Gap(8).Children(func() {
			settingsField(c, pal, &a.providerSearch,
				T("搜索供应商", "Search providers"), false).FillWidth()

			// 列表按内容长、最多 360：只有一家供应商时不该是一整块空面板，
			// 「添加供应商」贴在列表下面，比钉在整栏底部更近。
			rows := a.visibleProviders()
			listH := float32(len(rows))*providerRowH + float32(max(0, len(rows)-1)) + 8
			listH = clamp(listH, providerRowH+10, 360)
			panel := ui.Column(c).Height(listH).Shrink(0).FillWidth().MinHeight(0).
				Radius(settingsGroupR).Background(pal.chip).Clip()
			panel.Children(func() {
				ui.Scroll(c).Grow(1).FillWidth().Padding(3, 4).Children(func() {
					ui.Column(c).Gap(1).FillWidth().Children(func() {
						for k, i := range rows {
							next := len(a.editCfg.Providers)
							if k+1 < len(rows) {
								next = rows[k+1]
							}
							a.providerRow(c, pal, t, i, next)
						}
					})
				})
			})

			add := settingsButton(c, pal, T("添加供应商", "Add Provider"), false, false)
			add.FillWidth()
			add.Menu(func(m *ui.Menu) { a.addProviderMenu(m) })
			ui.Spacer(c)
		})

		// —— 右：选中那家的详情 ——
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			headPad := float32(18)
			if stacked {
				headPad = 12
			}
			ui.Row(c).Height(52).Shrink(0).FillWidth().
				Padding(0, headPad).Gap(8).AlignItems(ui.Center).Children(func() {
				if !stacked {
					ui.Box(c).Size(26, 26).Shrink(0).Radius(radiusSM).Background(pal.chip).
						AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						drawIcon(c, iconSparkles, 13, pal.textMuted)
					})
				}
				ui.Column(c).Gap(1).Children(func() {
					ui.Text(c, providerLabel(*p)).FontSize(13.5).
						FontWeight(weightSemibold).TextColor(pal.text).SingleLine()
					isActive := a.editIdx == a.editCfg.Active
					status, col := T("未生效", "Inactive"), pal.textMuted
					if isActive {
						status, col = T("已启用", "Enabled"), t.Success
					}
					ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
						ui.Box(c).Size(6, 6).Radius(radiusPill).Background(col)
						ui.Text(c, status).FontSize(textBodySM).TextColor(col)
					})
				})
				ui.Spacer(c)
				isActive := a.editIdx == a.editCfg.Active
				sw := settingsSwitch(c, pal, T("启用该供应商", "Enable this provider"), &isActive)
				if sw.Clicked() {
					a.toggleEditActive()
				}
				if stacked {
					settingsIconButton(c, pal, T("刷新", "Refresh"), T("刷新", "Refresh"), iconRefresh, a.probeMagpie)
					if len(a.editCfg.Providers) > 1 {
						settingsIconButton(c, pal, T("删除供应商", "Delete Provider"),
							T("删除", "Delete"), iconTrash, func() { a.removeProvider(a.editIdx) })
					}
				} else {
					if settingsMiniButton(c, pal, T("刷新", "Refresh"), T("刷新", "Refresh"), false).Clicked() {
						a.probeMagpie()
					}
					if len(a.editCfg.Providers) > 1 {
						if settingsMiniButton(c, pal, T("删除供应商", "Delete Provider"), T("删除", "Delete"), false).Clicked() {
							a.removeProvider(a.editIdx)
						}
					}
				}
			})

			ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).
				Padding(settingsPadTop, settingsPadX, settingsPadBot, settingsPadX).Children(func() {
				ui.Column(c).Gap(22).FillWidth().Children(func() {

					// 连接与身份：宽的时候「左边标签、右边控件」，
					// 窄的时候标签在上、控件占满一行。
					settingsGroup(c, pal, T("连接", "Connection"), "", func() {
						basePh := "https://api.anthropic.com"
						if p.Type == "openai" {
							basePh = "https://api.openai.com/v1"
						}
						if isMagpie(p.BaseURL) {
							basePh = "http://127.0.0.1:3425/v1"
						}
						if stacked {
							settingsFieldRow(c, pal, T("基础 URL", "Base URL"), "", func() {
								settingsField(c, pal, &p.BaseURL, basePh, false).FillWidth()
							})
						} else {
							settingsRow(c, pal, T("基础 URL", "Base URL"), "", func() {
								settingsField(c, pal, &p.BaseURL, basePh, false).Width(fieldW)
							})
						}

						keyPh := "sk-…"
						if isMagpie(p.BaseURL) {
							keyPh = T("免密钥（可留空）", "Optional")
						}
						keyControl := func(fill bool) {
							keyRow := ui.Row(c).Gap(6).AlignItems(ui.Center)
							if fill {
								keyRow.FillWidth()
							} else {
								keyRow.Shrink(0)
							}
							keyRow.Children(func() {
								field := settingsField(c, pal, &p.APIKey, keyPh, !a.showAPIKey)
								if fill {
									field.Grow(1).MinWidth(0)
								} else {
									field.Width(fieldW - 34)
								}
								eye := ui.ButtonBase(c).Size(settingsFieldH, settingsFieldH).
									Radius(radiusSM).Background(pal.control).
									Border(1, settingsFieldBorder(pal)).Center().
									Cursor(ui.CursorPointer).
									Label(T("切换密钥显示", "Toggle Key Visibility"))
								if eye.Hovered() {
									eye.Background(pal.controlHover)
								}
								fadeColors(eye)
								eyeSvg := iconEye
								if a.showAPIKey {
									eyeSvg = iconEyeOff
								}
								eye.Children(func() { drawIcon(c, eyeSvg, 13, pal.textMuted) })
								if eye.Clicked() {
									a.showAPIKey = !a.showAPIKey
								}
							})
						}
						if stacked {
							settingsFieldRow(c, pal, T("API 密钥", "API Key"), "", func() { keyControl(true) })
						} else {
							settingsRow(c, pal, T("API 密钥", "API Key"), "", func() { keyControl(false) })
						}

						if stacked {
							settingsFieldRow(c, pal, T("供应商名称", "Provider Name"), "", func() {
								settingsField(c, pal, &p.Name, providerLabel(*p), false).FillWidth()
							})
						} else {
							settingsRow(c, pal, T("供应商名称", "Provider Name"), "", func() {
								settingsField(c, pal, &p.Name, providerLabel(*p), false).Width(fieldW)
							})
						}

						protocolControl := func() {
							mode := 0
							if p.Type == "openai" {
								mode = 1
							}
							seg := settingsSegmented(c, pal, &mode,
								"Anthropic", T("OpenAI 兼容", "OpenAI Compatible"))
							seg.OnChange(func() {
								if mode == 1 {
									p.Type = "openai"
								} else {
									p.Type = "anthropic"
								}
							})
						}
						if stacked {
							// 分段控件按内容宽收：竖排时它会沿列铺满整行，
							// 右半截是一条空轨道，看着像没画完。
							settingsFieldRow(c, pal, T("接口协议", "Protocol"), "", func() {
								ui.Row(c).Children(protocolControl)
							})
						} else {
							settingsRow(c, pal, T("接口协议", "Protocol"), "", protocolControl)
						}
					})

					// 模型
					modelsList := a.modelsForProvider(p)
					var filteredModels []modelTagInfo
					for _, m := range modelsList {
						if a.modelSearch == "" || strings.Contains(strings.ToLower(m.ID), strings.ToLower(a.modelSearch)) {
							filteredModels = append(filteredModels, m)
						}
					}
					settingsGroup(c, pal,
						Tf("模型 · %d / %d", "Models · %d / %d", len(filteredModels), len(modelsList)),
						"", func() {
							search := func() ui.Element {
								return settingsField(c, pal, &a.modelSearch,
									T("搜索模型名称或 ID", "Search model name or ID"), false)
							}
							if stacked {
								settingsFieldRow(c, pal, T("搜索模型", "Search Models"), "", func() {
									search().FillWidth()
								})
							} else {
								settingsRow(c, pal, T("搜索模型", "Search Models"), "", func() {
									search().Width(fieldW)
								})
							}

							ui.Box(c).Padding(11, 0, 10, 0).FillWidth().Children(func() {
								list := ui.Column(c).Radius(radiusSM).Background(pal.card).
									FillWidth().Clip()
								raisedSurface(list, pal)
								list.Children(func() {
									// 标签挤在 ID 和开关之间时会被压成一条几个字宽的
									// 窄条（「本 地」竖着排）。所以标签一律不收缩，
									// 让 ID 先省略；整个一行放不下的时候，标签落到
									// 第二行去（见 twoLineModels）。
									tags := func(m modelTagInfo) {
										for _, tag := range m.Tags {
											ui.Box(c).Shrink(0).Padding(1, 5).Radius(radiusXS).
												Background(pal.chip).Children(func() {
												ui.Text(c, tag).FontSize(10).
													TextColor(pal.textMuted).SingleLine()
											})
										}
									}
									for k, m := range filteredModels {
										m := m
										if k > 0 {
											ui.Box(c).Height(1).FillWidth().Background(pal.rowDivider)
										}
										isCurrent := p.Model == m.ID
										rowH := float32(34)
										if twoLineModels {
											rowH = 46
										}
										row := ui.ButtonBase(c).Height(rowH).FillWidth().Padding(0, 8).
											Cursor(ui.CursorPointer).Label(m.ID).Tooltip(m.ID)
										if isCurrent {
											row.Background(pal.selection)
										} else if row.Hovered() {
											row.Background(pal.rowHover)
										}
										fadeColors(row)
										row.Children(func() {
											ui.Row(c).AlignItems(ui.Center).Gap(6).Fill().Children(func() {
												if twoLineModels {
													ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
														ui.Text(c, m.ID).FontSize(textBodySM).
															TextColor(pal.text).SingleLine()
														if len(m.Tags) > 0 {
															ui.Row(c).Gap(4).Children(func() { tags(m) })
														}
													})
												} else {
													ui.Text(c, m.ID).FontSize(textBodySM).
														TextColor(pal.text).SingleLine().
														Shrink(1).MinWidth(0)
													tags(m)
													ui.Spacer(c)
												}
												swVal := isCurrent
												sw := settingsSwitch(c, pal,
													Tf("使用模型 %s", "Use model %s", m.ID), &swVal)
												if sw.Clicked() {
													p.Model = m.ID
												}
											})
										})
										if row.Clicked() {
											p.Model = m.ID
										}
									}

									ui.Row(c).Padding(6, 8).Gap(8).AlignItems(ui.Center).
										FillWidth().Children(func() {
										ui.Text(c, T("自定义", "Custom")).FontSize(textBodySM).
											TextColor(pal.textMuted).Shrink(0)
										settingsField(c, pal, &p.Model,
											T("如 deepseek-chat, gpt-4o", "e.g. gpt-4o"), false).Grow(1)
									})
								})
							})
						})

					if isMagpie(p.BaseURL) {
						status, col := T("网关离线", "Gateway Offline"), pal.textFaint
						if a.magpieOnline {
							status, col = T("本地网关在线", "Gateway Online"), t.Success
						}
						ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Box(c).Size(6, 6).Radius(radiusPill).Background(col)
							ui.Text(c, status).FontSize(textBodySM).TextColor(col)
						})
					}
				})
			})
		})
	})
}

// promptsSettingsPane 呈现 AI 角色与 Prompt 提示词定制面板。
func (a *app) promptsSettingsPane(c *ui.Context, pal palette) {
	ui.Scroll(c).FillWidth().FillHeight().
		Padding(settingsPadTop, settingsPadX, settingsPadBot, settingsPadX).Children(func() {
		ui.Column(c).Gap(22).FillWidth().MaxWidth(620).Children(func() {
			settingsGroup(c, pal, T("自定义 AI Prompt 提示词", "Custom AI Prompt Templates"),
				T("定制智能提交与新建分支的 Prompt 提示词。支持一键恢复官方预设。",
					"Customize prompt templates for smart commit and branch naming. Supports restoring default presets."),
				func() {
					settingsRow(c, pal, T("恢复默认", "Restore Defaults"),
						T("把下面两段都改回官方预设。", "Put both prompts below back to the shipped presets."),
						func() {
							if settingsButton(c, pal, T("恢复全部预设", "Reset All Defaults"), false, false).Clicked() {
								a.editCommitPrompt = explain.CommitSystemPrompt
								a.editBranchPrompt = explain.BranchSystemPrompt
							}
						})
				})

			// 1. Git Commit 提示词
			promptSection(c, pal, c.Theme(),
				T("Git Commit 智能提交提示词", "Git Commit Message Prompt"),
				T("在改动面板中点击「✨ AI 智能提交」时使用的系统提示词与 Conventional Commits 规范",
					"System prompt used when generating Conventional Commit messages from staged/unstaged changes"),
				&a.editCommitPrompt,
				explain.CommitSystemPrompt,
			)

			// 2. Git Branch 提示词
			promptSection(c, pal, c.Theme(),
				T("Git 分支名称生成提示词", "Git Branch Naming Prompt"),
				T("在新建分支对话框中点击「✨ AI 建议分支名」时使用的命名风格与命名规则",
					"System prompt used when generating branch name recommendations (feat/..., fix/...)"),
				&a.editBranchPrompt,
				explain.BranchSystemPrompt,
			)
		})
	})
}

func promptSection(c *ui.Context, pal palette, t *ui.Theme, title, desc string, value *string, preset string) {
	settingsGroup(c, pal, title, desc, func() {
		settingsRow(c, pal, T("提示词内容", "Prompt"),
			T("留空即使用预设。", "Empty uses the preset."), func() {
				if settingsMiniButton(c, pal, T("恢复预设", "Restore Default"),
					T("恢复预设", "Restore"), false).Clicked() {
					*value = preset
				}
			})

		ui.Box(c).Padding(0, 0, 10, 0).FillWidth().Children(func() {
			var ta ui.Element
			box := ui.Column(c).Radius(radiusSM).Background(pal.control).
				Border(1, settingsFieldBorder(pal)).FillWidth().Clip()
			box.Children(func() {
				ta = ui.TextAreaBase(c, value).FillWidth().MinHeight(110).MaxHeight(170).
					Padding(8, 10).FontSize(13).
					Placeholder(preset).Label(title)
				ui.Box(c).Height(1).FillWidth().Background(pal.rowDivider)
				ui.Row(c).Padding(4, 8).AlignItems(ui.Center).FillWidth().Children(func() {
					ui.Text(c, Tf("%d 字符", "%d chars", len(*value))).FontSize(textCaption).
						TextColor(pal.textFaint).FontFeatures("tnum")
					ui.Spacer(c)
					isPreset := *value == preset || strings.TrimSpace(*value) == ""
					tag := T("自定义", "Custom")
					tagCol := pal.textMuted
					if isPreset {
						tag = T("预设", "Default")
						tagCol = t.Success
					}
					ui.Text(c, tag).FontSize(textCaption).TextColor(tagCol)
				})
			})
			switch {
			case ta.Focused():
				box.Border(1, pal.accent.Alpha(0.75)).
					Shadow(0, 0, 0, 3, pal.accent.Alpha(0.28))
			case box.Hovered():
				box.Border(1, settingsFieldBorderHover(pal))
			}
		})
	})
}

// addProvider 按预设追加一个供应商并选中它。若已存在，则直接聚焦至已有项，避免重复添加。
func (a *app) addProvider(ps preset) {
	// 加预设只「选中编辑」，不启用 —— 启用/禁用是用户自己在详情页那个
	// 开关上决定的，不该被一次添加悄悄把当前生效切成刚加的这家。
	//
	// 顺手把筛选词清掉：刚加的那家落在列表末尾，留着筛选词很可能它根本
	// 不在屏幕上，用户会以为没加上（选中态在看不见的地方，等于没有）。
	// 预设选择走的是菜单，筛选框本来就是给已添加列表用的，清掉不丢东西。
	a.providerSearch = ""
	if idx := findExistingPreset(a.editCfg.Providers, ps); idx >= 0 {
		a.editIdx = idx
		return
	}
	a.editCfg.Providers = append(a.editCfg.Providers, llm.Provider{
		Name:    ps.Label,
		Type:    ps.Type,
		BaseURL: ps.Base,
		Model:   ps.Model,
	})
	a.editIdx = len(a.editCfg.Providers) - 1
}

// addProviderMenu 是底部「添加供应商」菜单：一份预设清单，已经在列表里的
// 那些灰着 —— 灰比「点了没反应」清楚，也比把已加过的从菜单里藏起来好找。
//
// 末尾那条「自定义 OpenAI 兼容」是张白纸（类型 openai、地址和模型都空着），
// 原来底部那个「添加」按钮点下去加的就是它，改成菜单后别把它弄丢了。
func (a *app) addProviderMenu(m *ui.Menu) {
	for i, ps := range providerPresets {
		ps := ps
		if i == len(providerPresets)-1 {
			// 白纸单独一段：上面是「装好就能用」的，这条要用户自己填。
			m.Separator()
		}
		added := findExistingPreset(a.editCfg.Providers, ps) >= 0
		if m.Item(ps.Label).Disabled(added).Chosen() {
			a.addProvider(ps)
		}
	}
}

// visibleProviders 是筛选词筛过之后还要画的供应商下标，按列表顺序。
//
// 记下标而不是记 provider 值：选中态、拖动落地全都按下标走，中间多一层
// 值拷贝只会多一处对不上的机会。
func (a *app) visibleProviders() []int {
	rows := make([]int, 0, len(a.editCfg.Providers))
	q := strings.ToLower(a.providerSearch)
	for i := range a.editCfg.Providers {
		if q != "" && !strings.Contains(strings.ToLower(providerLabel(a.editCfg.Providers[i])), q) {
			continue
		}
		rows = append(rows, i)
	}
	return rows
}

// providerRowH 是供应商列表一行的高度。落点区按它切上下两半，所以单独
// 拎出来当一个数用。
const providerRowH = float32(34)

// providerDrag 是在供应商列表里拖着某一家走的载荷：from 是它拖动前的下标。
//
// 自己定义一个类型而不是复用框架 List 那套：一是那条路要求整行都能拖，
// 而行是个按钮、会先吃掉按下事件；二是载荷带上下标，落地时不用再去猜
// 拖的是谁。
type providerDrag struct {
	from int
}

// providerRow 画供应商列表的一行：拖动手柄、图标、名字、生效状态点。
//
// 行是按钮，但拖动**只认行首那个手柄** —— 手柄是独立的拖动源，按在名字
// 或者空白处仍然是选中（见 providerDrag）。next 是「下一家」在
// a.editCfg.Providers 里的下标：指针落在行的下半条时，这一家要插到
// next 前面（也就是插到这一行后面）。
func (a *app) providerRow(c *ui.Context, pal palette, t *ui.Theme, i, next int) {
	if i < 0 || i >= len(a.editCfg.Providers) {
		return
	}
	label := providerLabel(a.editCfg.Providers[i])
	selected := i == a.editIdx
	isActive := i == a.editCfg.Active
	// 筛选态不给拖：画出来的行下标不再是连着的，落点算出的「下一家」会隔
	// 着好几家，拖一下跳到看不见的地方 —— 不如干脆按住不动。
	draggable := a.providerSearch == ""

	rowBtn := ui.ButtonBase(c).Height(providerRowH).FillWidth().Radius(radiusSM).Padding(0, 8).
		Cursor(ui.CursorPointer).Label(label)
	switch {
	case selected:
		rowBtn.Background(pal.rowSelected)
		if !pal.dark {
			selectedSurface(rowBtn, pal)
		}
	case rowBtn.Hovered():
		rowBtn.Background(pal.rowHover)
	}
	fadeColors(rowBtn)

	var handle ui.Element
	rowBtn.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Fill().Children(func() {
			// 拖动手柄。六个点平时很淡，指着它才亮起来 —— 它是这一行唯一
			// 能拖的地方，别让它和名字抢视线。
			grip := pal.inkTertiary
			if rowBtn.Hovered() || rowBtn.Pressed() {
				grip = pal.inkMuted
			}
			handle = drawIcon(c, iconGrip, 11, grip).Cursor(ui.CursorGrab)
			if draggable {
				handle.Drag(providerDrag{from: i})
			}
			if handle.Valid() && handle.Dragging() {
				handle.Cursor(ui.CursorGrabbing)
			}
			// 供应商图标
			ui.Box(c).Size(20, 20).Radius(radiusXS).Background(pal.card).
				AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				drawIcon(c, iconSparkles, 11, pal.textMuted)
			})
			txtCol := pal.textMuted
			if selected {
				txtCol = pal.text
			}
			ui.Text(c, label).FontSize(textBodySM).FontWeight(weightMedium).
				TextColor(txtCol).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			// 状态圆点：只有当前生效的供应商是实心绿点，其余一律
			// 空心描边 —— 加进来的≠启用的，别让每行都像开着。
			if isActive {
				ui.Box(c).Size(7, 7).Radius(radiusPill).Background(t.Success)
			} else {
				ui.Box(c).Size(7, 7).Radius(radiusPill).Border(1, pal.hairlineStrong).Background(ui.Transparent)
			}
		})
	})
	if rowBtn.Clicked() || handle.Clicked() {
		a.editIdx = i
	}

	// 落点就是行按钮自己。不用另外摆两块半行的落点区：框架只把「指针下
	// 最上面那个元素 + 它的祖先」放进命中链，兄弟元素根本收不到 Drop 和
	// DragOver；而盖在行上面的方块又会让手柄的 id 跟着结构下标一起挪，
	// 拖到一半手柄就不再是当初按下的那个了（手柄靠 id 认出自己在被拖）。
	// 指针在行的上半条 = 插在这一家前面，下半条 = 插到下一家前面。
	dropAt := func() int {
		if _, y, _ := rowBtn.PointerPosition(); y > providerRowH/2 {
			return next
		}
		return i
	}
	if _, over := ui.DragOver[providerDrag](rowBtn); over {
		at := dropAt()
		rowBtn.DrawOver(func(p *ui.Painter, r ui.Rect) {
			y := r.Y
			if at == next {
				y = r.Y + r.H
			}
			// 缩进 4px：不压到行首那个手柄，看着像一条「插进列表里」的缝。
			p.Fill(ui.Rect{X: r.X + 4, Y: y - 1, W: r.W - 8, H: 2}, c.Theme().Accent, 1)
		})
	}
	if draggable {
		if d, ok := ui.Drop[providerDrag](rowBtn); ok {
			a.moveProvider(d.from, dropAt())
		}
	}
}

// moveProvider 把 from 上的那家供应商搬到 to 前面（to 是拖动前的下标，
// 等于列表长度表示搬到末尾）。
func (a *app) moveProvider(from, to int) {
	ps := a.editCfg.Providers
	if from < 0 || from >= len(ps) || to < 0 || to > len(ps) {
		return
	}
	// 落回原位（to == from）或者原地不动（to == from+1，等于搬到自己
	// 前面）都什么都不用做。
	if to == from || to == from+1 {
		return
	}
	// to 说的是「搬到谁前面」，所以先把这一项摘出来，剩下的下标整体前移
	// 一位，再往 at 前面插。
	at := to
	if to > from {
		at--
	}
	moved := ps[from]
	rest := make([]llm.Provider, 0, len(ps)-1)
	rest = append(rest, ps[:from]...)
	rest = append(rest, ps[from+1:]...)
	out := make([]llm.Provider, 0, len(ps))
	out = append(out, rest[:at]...)
	out = append(out, moved)
	out = append(out, rest[at:]...)
	a.editCfg.Providers = out

	// 「正在编辑的」和「当前生效的」都是这份切片的下标，搬完得跟着走 ——
	// 不换的话用户拖的是 A，右边详情页悄悄换成了 B。
	a.editIdx = remapProviderIndex(a.editIdx, from, at)
	if a.editCfg.Active >= 0 {
		a.editCfg.Active = remapProviderIndex(a.editCfg.Active, from, at)
	}
}

// remapProviderIndex 把「拖动前在第 i 位的那个供应商」换算成拖动后的下标。
// from 是被拖走的那一项原来的下标，to 是它拖完之后的下标。
func remapProviderIndex(i, from, to int) int {
	switch {
	case i == from:
		return to
	case from < to && i > from && i <= to:
		return i - 1 // 被它跨过去的那些整体前移一位
	case to < from && i >= to && i < from:
		return i + 1 // 整体后移一位
	}
	return i
}

// toggleEditActive 是详情页头部那个开关绑定的行为：开关当前生效的供应商，
// 点上就把它关掉（Active = -1，AI 回落环境变量），没开着的则把它打开。
func (a *app) toggleEditActive() {
	if a.editIdx == a.editCfg.Active {
		if a.editCfg.Active >= 0 {
			a.editCfg.Active = -1
		}
	} else {
		a.editCfg.Active = a.editIdx
	}
}

func (a *app) removeProvider(i int) {
	if i < 0 || i >= len(a.editCfg.Providers) {
		return
	}
	a.editCfg.Providers = append(a.editCfg.Providers[:i], a.editCfg.Providers[i+1:]...)
	if a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = len(a.editCfg.Providers) - 1
	}
	// Active 保持 -1 不归零：那是用户「关了当前生效」的合法状态。
	if a.editIdx >= len(a.editCfg.Providers) {
		a.editIdx = len(a.editCfg.Providers) - 1
	}
	if a.editIdx < 0 {
		a.editIdx = 0
	}
}

// applySettings 保存设置。
func (a *app) applySettings() {
	cfg := a.store.Config()
	cfg.Providers = a.editCfg.Providers
	cfg.Active = a.editCfg.Active
	cfg.ThemeMode = a.themeMode
	cfg.Locale = a.locale

	// 持久化保存提示词配置
	cfg.CommitPrompt = strings.TrimSpace(a.editCommitPrompt)
	cfg.BranchPrompt = strings.TrimSpace(a.editBranchPrompt)

	a.store.SetConfig(cfg)
	a.persist()

	// Git 配置走另一条路：它不是 Gloss 的设置，落在 git 自己的配置文件里，
	// 写完由横幅/吐司回话（见 applyGitConfig）。
	a.applyGitConfig()

	a.settingsOn = false
}

func providerLabel(p llm.Provider) string {
	if p.Name != "" {
		return p.Name
	}
	if p.Type == "anthropic" {
		return "Anthropic"
	}
	return T("未命名服务商", "Untitled Provider")
}
