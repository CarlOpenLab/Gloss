package main

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

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
)

// preset 是一个常见供应商的预设：接口类型、Base URL、模型都填好，
// 用户只需要补一个 API key（本地网关甚至不需要 key）。
type preset struct {
	Label string
	Type  string
	Base  string
	Model string
}

// providerPresets 覆盖常用服务商及本地网关，按开箱即用性排序。
var providerPresets = []preset{
	{"Magpie（本地网关）", "openai", "http://127.0.0.1:3425/v1", "opencode-go/deepseek-v4.1-flash"},
	{"Anthropic", "anthropic", "", llm.DefaultModel},
	{"DeepSeek", "openai", "https://api.deepseek.com", "deepseek-chat"},
	{"Kimi（Moonshot）", "openai", "https://api.moonshot.cn/v1", "kimi-k2-0905-preview"},
	{"通义千问（DashScope）", "openai", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen-plus"},
	{"智谱 GLM", "openai", "https://open.bigmodel.cn/api/paas/v4", "glm-4.5"},
	{"OpenRouter", "openai", "https://openrouter.ai/api/v1", "anthropic/claude-sonnet-4.5"},
	{"Ollama（本地）", "openai", "http://localhost:11434/v1", "qwen2.5-coder"},
	{"自定义 OpenAI 兼容", "openai", "", ""},
}

// openSettings 打开设置对话框，在配置的副本上编辑 ——
// 点「保存」之前不动真配置。
func (a *app) openSettings() {
	a.editCfg = a.store.Config()
	if len(a.editCfg.Providers) == 0 {
		a.editCfg.Providers = []llm.Provider{{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel}}
	}
	if a.editCfg.Active < 0 || a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = 0
	}
	a.editIdx = a.editCfg.Active
	a.editThemeMode = a.themeMode
	a.editLocale = a.locale
	a.showAPIKey = false
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

// settingsDialog 是偏好与模型设置对话框的入口。
func (a *app) settingsDialog(c *ui.Context, pal palette) {
	if !a.settingsOn {
		return
	}
	open := true
	ui.DialogBase(c, &open, func(back, panel *ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		// 设置窗口是最顶层的一块浮层，按 §2.1 落 Surface 4、圆角 xl(16)（§3）。
		//
		// 用 DialogBase 而不是 Modal：Modal 会先铺一层自己的面板（Surface 3、
		// 圆角 10、四周 20px 内边距），再让内容画上去。设置面板自己就是一块
		// 920×660 的卡片，套进去等于两层圆角不一致的框叠在一起 —— 截图里
		// 看得很清楚，外圈的角比内圈的角更方。
		panel.Width(920).Height(660).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24))
		a.settingsBody(c, pal)
	})
	if !open {
		a.settingsOn = false
	}
}

// settingsSidebarItem 绘制 Linear 风格侧边栏导航条目。
func settingsSidebarItem(c *ui.Context, t *ui.Theme, label string, icon *ui.SVG, selected bool, onClick func()) {
	btn := ui.ButtonBase(c).Height(32).FillWidth().Radius(radiusSM).Padding(0, 10).
		Cursor(ui.CursorPointer).Label(label)

	// 选中态用 Surface 阶梯（和主侧栏的分组条目同一档），强调色只留给图标……
	bg := ui.Transparent
	fg := t.TextMuted
	if selected {
		bg = t.SurfaceHover
		fg = t.Text
	} else if btn.Hovered() {
		bg = t.SurfaceHover.Alpha(0.5)
		fg = t.Text
	}
	btn.Background(bg)
	fadeColors(btn)

	btn.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Fill().Children(func() {
			iconColor := fg
			if selected {
				iconColor = t.Accent
			}
			drawIcon(c, icon, 14, iconColor)
			txt := ui.Text(c, label).FontSize(12).TextColor(fg)
			if selected {
				txt.Bold()
			}
		})
	})
	if btn.Clicked() {
		onClick()
	}
}

// settingsBody 呈现参考 Linear/Cursor 设计风格的双栏清爽偏好设置面板。
func (a *app) settingsBody(c *ui.Context, pal palette) {
	t := c.Theme()
	ui.Row(c).FillWidth().FillHeight().Children(func() {
		// 1. 左侧极简分组导航栏 (Sidebar)
		ui.Column(c).Width(200).FillHeight().Background(pal.surface1).
			BorderWidth(0, 1, 0, 0).BorderColor(pal.hairline).
			Padding(16, 12).Gap(4).Children(func() {
			// 标题区
			ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(4, 8, 14, 8).Children(func() {
				// 图标底板是纯装饰：一块中性的方砖，不借用强调色 ——
				// 它周围没有「选中」也没有「可点」的意思（§5 禁止非语义彩色修饰）。
				ui.Box(c).Size(22, 22).Radius(radiusSM).Background(pal.surface2).
					Border(1, pal.hairline).
					AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					drawIcon(c, iconSettings, 13, pal.inkMuted)
				})
				ui.Text(c, "Gloss "+T("设置", "Settings")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
			})

			// 分组 1: 设置分类
			ui.Text(c, T("设置", "Settings")).FontSize(textCaption).FontWeight(weightSemibold).TextColor(pal.inkSubtle).
				Padding(6, 8, 2, 8)

			settingsSidebarItem(c, t, T("通用偏好", "Preferences"), iconPalette, a.settingsTab == settingsTabGeneral, func() {
				a.settingsTab = settingsTabGeneral
			})

			settingsSidebarItem(c, t, T("模型设置", "Model Settings"), iconSparkles, a.settingsTab == settingsTabModels, func() {
				a.settingsTab = settingsTabModels
			})

			// 分组 2: 系统
			ui.Text(c, T("系统", "System")).FontSize(textCaption).FontWeight(weightSemibold).TextColor(pal.inkSubtle).
				Padding(12, 8, 2, 8)

			settingsSidebarItem(c, t, T("关于 Gloss", "About Gloss"), iconEye, a.settingsTab == settingsTabAbout, func() {
				a.settingsTab = settingsTabAbout
			})

			ui.Spacer(c)

			// 底部版本信息
			ui.Row(c).Padding(8, 8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Gloss v1.0.0").FontSize(textCaption).TextColor(pal.inkTertiary)
			})
		})

		// 2. 右侧主内容区 (Main Content Pane)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(pal.surface3).Children(func() {
			// 顶部标题栏
			ui.Row(c).Height(50).AlignItems(ui.Center).Padding(0, 20, 0, 28).FillWidth().Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					title := T("通用偏好", "Preferences")
					desc := T("定制外观主题、macOS 应用图标及界面语言", "Customize appearance theme, app icon, and display language")
					switch a.settingsTab {
					case settingsTabModels:
						title = T("模型设置", "Model Settings")
						desc = T("配置逐 Hunk AI 解释与智能提交模型服务商及 API Key", "Configure AI models and API keys for explanations & commits")
					case settingsTabAbout:
						title = T("关于 Gloss", "About Gloss")
						desc = T("原生 macOS 智能 Git 客户端", "Native macOS Git GUI with AI-powered diff review")
					}
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, title).FontSize(textHeadline).FontWeight(weightSemibold).TextColor(pal.ink)
						if a.settingsTab == settingsTabModels && a.magpieOnline {
							ui.Box(c).Padding(2, 7).Radius(radiusPill).Background(pal.surface2).Border(1, pal.hairline).Children(func() {
								ui.Text(c, T("Magpie 在线", "Magpie Online")).FontSize(textCaption).FontWeight(weightMedium).TextColor(t.Success)
							})
						}
					})
					ui.Text(c, desc).FontSize(11).TextColor(t.TextMuted).SingleLine()
				})

				ui.Spacer(c)

				if closeButton(c, pal) {
					a.settingsOn = false
				}
			})

			// 顶部分界线
			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 主体内容区（独立的填满空间，由各 Pane 内部全幅滚动）
			ui.Box(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				if a.settingsTab == settingsTabGeneral {
					a.generalSettingsPane(c, pal)
				} else if a.settingsTab == settingsTabAbout {
					a.aboutSettingsPane(c, pal)
				} else {
					a.modelsSettingsPane(c, pal)
				}
			})

			// 底部分界线
			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 底部操作栏（固定高度，与上部完全解耦，永不发生溢出重叠）。
			//
			// 不铺自己的底色：这颗条和上面的内容区只差一档明度，铺了也看不出来，
			// 反而把「取消」压住 —— 次级按钮按 §4.1 就是 Surface 1，底下若是
			// 同一档的深灰，那颗按钮就只剩一道发丝边，像在条上挖了个洞。让它
			// 直接坐在面板底（Surface 4）上，靠上面那道发丝线分区就够了。
			ui.Row(c).Height(48).Shrink(0).AlignItems(ui.Center).Padding(0, 20, 0, 28).
				FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					drawIcon(c, iconShield, 13, t.TextMuted)
					ui.Text(c, T("设置存储于本地配置文件，所有请求由本机直连服务端", "Preferences stored locally; requests connect directly from this device")).
						FontSize(11).TextColor(t.TextMuted)
				})

				ui.Spacer(c)

				ui.Row(c).Gap(10).Children(func() {
					if secondaryAction(c, pal, T("取消", "Cancel"), "", false).Clicked() {
						a.settingsOn = false
					}
					if primaryAction(c, pal, T("保存", "Save"), "", false).Clicked() {
						a.applySettings()
					}
				})
			})
		})
	})
}

// generalSettingsPane 呈现通用设置（外观主题、界面语言、代码字号等）。
func (a *app) generalSettingsPane(c *ui.Context, pal palette) {
	t := c.Theme()
	ui.Scroll(c).FillWidth().FillHeight().Padding(20, 28).Children(func() {
		ui.Column(c).Gap(20).FillWidth().Children(func() {

			// 1. 外观色彩模式
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					drawIcon(c, iconMonitor, 15, t.Accent)
					ui.Text(c, T("外观主题", "Appearance Theme")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				})
				ui.Text(c, T("选择 Gloss 界面的视觉色彩模式，支持随系统自动切换", "Select color scheme for Gloss, or follow system appearance automatically")).
					FontSize(11).TextColor(t.TextMuted)

				ui.Row(c).Gap(12).FillWidth().Children(func() {
					themeCards := []struct {
						id    string
						label string
						desc  string
						icon  *ui.SVG
					}{
						{"system", T("跟随系统", "Follow System"), T("自适应系统深浅色", "Sync with macOS"), iconMonitor},
						{"light", T("浅色模式", "Light Mode"), T("经典高对比度明亮色", "Crisp light theme"), iconSun},
						{"dark", T("深色模式", "Dark Mode"), T("暗黑灰蓝沉浸配色", "GitHub dark slate"), iconMoon},
					}

					for _, tc := range themeCards {
						tc := tc
						selected := a.themeMode == tc.id
						card := ui.ButtonBase(c).Grow(1).Height(62).Radius(radiusMD).Padding(8, 12).
							Cursor(ui.CursorPointer).Label(tc.label)

						// 选中态原来把底、边、图标、圆点四处都染成强调色。
						// 现在只留两条：边框和图标 —— 「选中的是这个」说两遍
						// 就够了，底上的那层淡蓝只是把卡片糊成一块。
						border := pal.hairlineStrong
						bg := pal.surface1
						if selected {
							border = t.Accent
							bg = pal.selection
						} else if card.Hovered() {
							bg = pal.surface3
						}
						card.Border(1, border).Background(bg)
						fadeColors(card)

						card.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(10).Fill().Children(func() {
								col := t.TextMuted
								if selected {
									col = t.Accent
								}
								drawIcon(c, tc.icon, 18, col)
								ui.Column(c).Gap(2).Grow(1).Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
										ui.Text(c, tc.label).FontSize(textBodySM).
											FontWeight(weightSemibold).TextColor(pal.ink)
									})
									ui.Text(c, tc.desc).FontSize(textCaption).TextColor(pal.inkSubtle).SingleLine()
								})
							})
						})

						if card.Clicked() {
							a.setThemeMode(tc.id)
						}
					}
				})
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 2. 应用图标 (Application Icon)
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					drawIcon(c, iconAppWindow, 15, t.Accent)
					ui.Text(c, T("应用图标", "Application Icon")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				})
				ui.Text(c, T("选择 Gloss 在 macOS 坞栏（Dock）及系统的极简图标，免重启即时生效", "Choose minimalist icon style for macOS Dock; applies immediately")).
					FontSize(11).TextColor(t.TextMuted)

				bmBlack, bmWhite := getIconBitmaps()
				iconCards := []struct {
					id     string
					label  string
					desc   string
					bitmap *ui.Bitmap
				}{
					{"black", T("纯黑极简", "Pure Black"), T("深色曜石质感 · 高对比克制", "Obsidian neutral black · High contrast"), bmBlack},
					{"white", T("纯白极简", "Pure White"), T("明亮纯白质感 · 干净通透", "Porcelain neutral white · Clean & crisp"), bmWhite},
				}

				ui.Row(c).Gap(12).FillWidth().Children(func() {
					for _, ic := range iconCards {
						ic := ic
						selected := a.appIcon == ic.id || (a.appIcon == "" && ic.id == "black")
						card := ui.ButtonBase(c).Grow(1).Height(64).Radius(radiusMD).Padding(8, 12).
							Cursor(ui.CursorPointer).Label(ic.label)

						// 选中态原来把底、边、图标、圆点四处都染成强调色。
						// 现在只留两条：边框和图标 —— 「选中的是这个」说两遍
						// 就够了，底上的那层淡蓝只是把卡片糊成一块。
						border := pal.hairlineStrong
						bg := pal.surface1
						if selected {
							border = t.Accent
							bg = pal.selection
						} else if card.Hovered() {
							bg = pal.surface3
						}
						card.Border(1, border).Background(bg)
						fadeColors(card)

						card.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(12).Fill().Children(func() {
								if ic.bitmap != nil {
									ui.Box(c).Size(40, 40).Radius(radiusMD).Clip().Children(func() {
										ui.Image(c, ic.bitmap).Size(40, 40)
									})
								} else {
									ui.Box(c).Size(40, 40).Radius(radiusMD).Background(pal.surface3)
								}

								ui.Column(c).Gap(2).Grow(1).Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
										ui.Text(c, ic.label).FontSize(textBodySM).
											FontWeight(weightSemibold).TextColor(pal.ink)
									})
									ui.Text(c, ic.desc).FontSize(textCaption).TextColor(pal.inkSubtle).SingleLine()
								})
							})
						})

						if card.Clicked() {
							a.setAppIcon(ic.id)
						}
					}
				})
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 3. 界面语言
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					drawIcon(c, iconGlobe, 15, t.Accent)
					ui.Text(c, T("界面语言", "Display Language")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				})
				ui.Text(c, T("选择界面展示所使用的语言（切换后即时生效）", "Choose display language for the application (applies immediately)")).
					FontSize(11).TextColor(t.TextMuted)

				ui.Row(c).Gap(12).FillWidth().Children(func() {
					langCards := []struct {
						id    string
						label string
						desc  string
					}{
						{"zh", "简体中文", "Simplified Chinese"},
						{"en", "English", "United States (US)"},
					}

					for _, lc := range langCards {
						lc := lc
						selected := a.locale == lc.id
						card := ui.ButtonBase(c).Grow(1).Height(54).Radius(radiusMD).Padding(8, 14).
							Cursor(ui.CursorPointer).Label(lc.label)

						// 选中态原来把底、边、图标、圆点四处都染成强调色。
						// 现在只留两条：边框和图标 —— 「选中的是这个」说两遍
						// 就够了，底上的那层淡蓝只是把卡片糊成一块。
						border := pal.hairlineStrong
						bg := pal.surface1
						if selected {
							border = t.Accent
							bg = pal.selection
						} else if card.Hovered() {
							bg = pal.surface3
						}
						card.Border(1, border).Background(bg)
						fadeColors(card)

						card.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(10).Fill().Children(func() {
								ui.Column(c).Gap(2).Grow(1).Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
										ui.Text(c, lc.label).FontSize(textBodySM).
											FontWeight(weightSemibold).TextColor(pal.ink)
									})
									ui.Text(c, lc.desc).FontSize(textCaption).TextColor(pal.inkSubtle)
								})
							})
						})

						if card.Clicked() {
							a.setLocale(lc.id)
						}
					}
				})
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 4. Diff 代码字号与视图偏好
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Text(c, T("Diff 与代码视图", "Diff & Code View")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)

				ui.Row(c).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
					ui.Column(c).Gap(2).Grow(1).Children(func() {
						ui.Text(c, T("代码字体大小", "Code Font Size")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
						sizeVal := a.codeSizeValue()
						sizeLabel := Tf("%.1f px", "%.1f px", sizeVal)
						if a.codeSize == 0 {
							sizeLabel = Tf("%.1f px (默认)", "%.1f px (Default)", sizeVal)
						}
						ui.Text(c, sizeLabel).FontSize(textCaption).TextColor(pal.inkSubtle)
					})

					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						minusBtn := ui.ButtonBase(c).Size(28, 28).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label("−").Tooltip(T("缩小代码字号 ⌘−", "Zoom Out ⌘-"))
						if minusBtn.Hovered() {
							minusBtn.Background(pal.surface3)
						}
						fadeColors(minusBtn)
						minusBtn.Children(func() { ui.Text(c, "−").FontSize(textBody).TextColor(pal.ink) })
						if minusBtn.Clicked() {
							a.setCodeSize(a.codeSizeValue() - codeSizeStep)
						}

						plusBtn := ui.ButtonBase(c).Size(28, 28).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label("+").Tooltip(T("放大代码字号 ⌘+", "Zoom In ⌘+"))
						if plusBtn.Hovered() {
							plusBtn.Background(pal.surface3)
						}
						fadeColors(plusBtn)
						plusBtn.Children(func() { ui.Text(c, "+").FontSize(textBody).TextColor(pal.ink) })
						if plusBtn.Clicked() {
							a.setCodeSize(a.codeSizeValue() + codeSizeStep)
						}

						resetBtn := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(T("重置", "Reset")).Tooltip(T("恢复默认字号 ⌘0", "Actual Size ⌘0"))
						if resetBtn.Hovered() {
							resetBtn.Background(pal.surface3)
						}
						fadeColors(resetBtn)
						resetBtn.Children(func() {
							ui.Text(c, T("重置", "Reset")).FontSize(textButton).FontWeight(weightMedium).TextColor(pal.inkMuted)
						})
						if resetBtn.Clicked() {
							a.setCodeSize(0)
						}
					})
				})
			})
		})
	})
}

// aboutSettingsPane 呈现关于 Gloss 的产品信息、版本与核心特性。
func (a *app) aboutSettingsPane(c *ui.Context, pal palette) {
	bmBlack, bmWhite := getIconBitmaps()
	currentBm := bmBlack
	if a.appIcon == "white" {
		currentBm = bmWhite
	}

	ui.Scroll(c).FillWidth().FillHeight().Padding(28, 36).Children(func() {
		ui.Column(c).Gap(20).FillWidth().Children(func() {
			// 顶部品牌区
			ui.Row(c).AlignItems(ui.Center).Gap(16).Children(func() {
				if currentBm != nil {
					ui.Box(c).Size(64, 64).Radius(radiusLG).Clip().Children(func() {
						ui.Image(c, currentBm).Size(64, 64)
					})
				} else {
					ui.Box(c).Size(64, 64).Radius(radiusLG).Background(pal.surface2)
				}
				ui.Column(c).Gap(4).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, "Gloss").FontSize(textDisplay).FontWeight(weightSemibold).TextColor(pal.ink)
						ui.Box(c).Padding(2, 8).Radius(radiusPill).Background(pal.surface2).
							Border(1, pal.hairline).Children(func() {
							ui.Text(c, "v1.0.0").FontSize(textCaption).FontWeight(weightMedium).
								TextColor(pal.inkSubtle)
						})
					})
					ui.Text(c, T("原生 macOS 智能 Git 客户端", "Native macOS Git GUI with AI-powered diff review")).
						FontSize(textBodySM).TextColor(pal.inkSubtle)
				})
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 特性卡片列表
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				features := []struct {
					icon  *ui.SVG
					title string
					desc  string
				}{
					{
						iconZap,
						T("极致轻量与性能", "Lightweight & Fast"),
						T("基于 Go 原生图形引擎，瞬间冷启动与顺滑渲染，极低内存开销", "Instant startup and smooth rendering powered by native Go engine"),
					},
					{
						iconSparkles,
						T("逐 Hunk AI 智能审查", "Per-Hunk AI Review"),
						T("代码行级智能解释、变更风险提示与一键智能提交信息生成", "Smart hunk explanation, risk analysis, and Conventional Commits generation"),
					},
					{
						iconPalette,
						T("克制优雅的视觉设计", "Focused & Distraction-Free"),
						T("跟随 macOS 深浅色外观无缝切换，全 Neutral 极简黑白图标设计，零多余杂色", "Seamless dark/light mode switching with pure neutral monochrome icons"),
					},
				}

				for _, f := range features {
					ui.Row(c).Padding(10, 14).Radius(radiusMD).Background(pal.surface2).
						Border(1, pal.hairline).
						AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
						// 装饰性图标底板同样去色：三张卡片并排，三个蓝方块
						// 就成了一种「彩色的墙纸」。
						ui.Box(c).Size(28, 28).Radius(radiusSM).Background(pal.surface3).
							Border(1, pal.hairline).
							AlignItems(ui.Center).Justify(ui.Center).Children(func() {
							drawIcon(c, f.icon, 15, pal.inkMuted)
						})
						ui.Column(c).Gap(2).Grow(1).Children(func() {
							ui.Text(c, f.title).FontSize(textBodySM).FontWeight(weightSemibold).
								TextColor(pal.ink)
							ui.Text(c, f.desc).FontSize(textCaption).TextColor(pal.inkSubtle)
						})
					})
				}
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 底部版权与声明
			ui.Row(c).AlignItems(ui.Center).FillWidth().Children(func() {
				ui.Text(c, "Copyright © 2026 CarlOpenLab. Released under the MIT License.").
					FontSize(textCaption).TextColor(pal.inkTertiary)
			})
		})
	})
}

// settingSection 绘制参考 Linear/Cursor 的分组设置卡片外框。
func settingSection(c *ui.Context, pal palette, title, desc string, content func()) {
	ui.Column(c).Gap(6).FillWidth().Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.ink)
			if desc != "" {
				ui.Text(c, desc).FontSize(textCaption).TextColor(pal.inkSubtle)
			}
		})
		ui.Column(c).Radius(radiusMD).Background(pal.surface2).Border(1, pal.hairline).
			FillWidth().Children(content)
	})
}

// settingRow 绘制分组卡片内部的单行设置项（左侧标题说明，右侧控件）。
func settingRow(c *ui.Context, pal palette, label, sub string, control func()) {
	ui.Row(c).Padding(7, 14).AlignItems(ui.Center).FillWidth().Children(func() {
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(textBodySM).TextColor(pal.ink)
			if sub != "" {
				ui.Text(c, sub).FontSize(textCaption).TextColor(pal.inkSubtle)
			}
		})
		control()
	})
}

// settingDivider 绘制卡片内行与行之间的发丝微细分割线。
func settingDivider(c *ui.Context, pal palette) {
	ui.Box(c).Height(1).FillWidth().Background(pal.hairline)
}

// modelsSettingsPane 呈现对标 Linear/Cursor 的清爽分组卡片式模型设置页面。
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
	if a.editCfg.Active < 0 || a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = 0
	}

	p := &a.editCfg.Providers[a.editIdx]
	activeP := a.editCfg.Providers[a.editCfg.Active]

	ui.Scroll(c).FillWidth().FillHeight().Padding(16, 24).Children(func() {
		ui.Column(c).Gap(12).FillWidth().Children(func() {

			// 1. 生效模型与服务商选择
			settingSection(c, pal, T("生效服务商与快捷预设", "Active Provider & Presets"),
				T("当前全局生效的 AI 解释与提交模型，可一键切换或追加常用预设", "Active LLM for AI explanations & conventional commits with quick-add presets"),
				func() {
					// 行 1: 已配置服务商切换、设为生效与删除
					ui.Row(c).Padding(7, 14).AlignItems(ui.Center).FillWidth().Children(func() {
						ui.Column(c).Gap(1).Shrink(0).Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
								ui.Text(c, T("已配置服务商", "Configured Providers")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
								ui.Text(c, "("+providerLabel(activeP)+")").FontSize(textCaption).TextColor(pal.inkSubtle)
							})
							ui.Text(c, T("点击切换编辑项，支持一键生效或删除", "Select provider to edit, activate, or delete")).FontSize(textCaption).TextColor(pal.inkSubtle)
						})
						ui.Spacer(c)
						ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
							for i := range a.editCfg.Providers {
								i := i
								curr := &a.editCfg.Providers[i]
								selected := i == a.editIdx
								isActive := i == a.editCfg.Active

								chip := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(radiusSM).
									AlignItems(ui.Center).Gap(5).Cursor(ui.CursorPointer).Label(providerLabel(*curr))
								// 两件事要在一枚芯片上说清楚：哪一枚正在编辑（选中）、
								// 哪一枚在生效（圆点）。原来两者都用强调色，并排时
								// 分不出谁是谁 —— 现在只有「生效」那枚带圆点。
								bg := pal.surface2
								border := pal.hairlineStrong
								if selected {
									bg = pal.selection
									border = pal.hairlineTertiary
								} else if chip.Hovered() {
									bg = pal.surface3
								}
								chip.Background(bg).Border(1, border)
								fadeColors(chip)

								chip.Children(func() {
									if isActive {
										ui.Box(c).Size(chipDot, chipDot).Shrink(0).Radius(chipDot / 2).
											Background(pal.laneColor(0))
									}
									txtCol := pal.inkMuted
									if selected {
										txtCol = pal.ink
									}
									ui.Text(c, providerLabel(*curr)).FontSize(textButton).
										FontWeight(weightMedium).TextColor(txtCol)
								})
								if chip.Clicked() {
									a.editIdx = i
								}
							}

							if a.editIdx != a.editCfg.Active {
								useBtn := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(radiusSM).
									Background(pal.surface2).Border(1, pal.hairlineStrong).Cursor(ui.CursorPointer).
									Label(T("设为当前生效", "Set Active"))
								if useBtn.Hovered() {
									useBtn.Background(pal.surface3)
								}
								fadeColors(useBtn)
								useBtn.Children(func() {
									ui.Text(c, T("设为当前生效", "Set Active")).FontSize(textButton).
										FontWeight(weightMedium).TextColor(pal.ink)
								})
								if useBtn.Clicked() {
									a.editCfg.Active = a.editIdx
								}
							}

							if len(a.editCfg.Providers) > 1 {
								delBtn := ui.ButtonBase(c).Height(26).Padding(0, 8).Radius(radiusSM).
									AlignItems(ui.Center).Gap(4).Background(ui.Transparent).
									Border(1, pal.hairlineStrong).Cursor(ui.CursorPointer).
									Label(T("删除", "Delete")).Tooltip(T("删除当前选中的服务商", "Delete provider"))
								if delBtn.Hovered() {
									delBtn.Background(t.Danger.Alpha(0.12)).Border(1, t.Danger.Alpha(0.3))
								}
								fadeColors(delBtn)
								delBtn.Children(func() {
									drawIcon(c, iconTrash, 12, t.Danger)
									ui.Text(c, T("删除", "Delete")).FontSize(textButton).
										FontWeight(weightMedium).TextColor(t.Danger)
								})
								if delBtn.Clicked() {
									a.removeProvider(a.editIdx)
								}
							}
						})
					})

					settingDivider(c, pal)

					// 行 2: 快捷添加常用预设
					ui.Column(c).Padding(7, 14).Gap(6).FillWidth().Children(func() {
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							ui.Text(c, T("快捷添加预设", "Quick-Add Presets")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
							ui.Spacer(c)
							ui.Text(c, T("点击一键填入常用服务商模板", "Click to quickly add provider template")).FontSize(textCaption).TextColor(pal.inkSubtle)
						})

						ui.Row(c).Gap(6).Wrap().FillWidth().Children(func() {
							for _, ps := range providerPresets {
								ps := ps
								existingIdx := findExistingPreset(a.editCfg.Providers, ps)
								isAdded := existingIdx >= 0

								btn := ui.ButtonBase(c).Height(26).Padding(0, 8).Radius(radiusSM).
									AlignItems(ui.Center).Gap(4).Cursor(ui.CursorPointer).Label(ps.Label)
								bg := pal.surface2
								border := pal.hairlineStrong
								if isAdded {
									bg = pal.surface1
								} else if btn.Hovered() {
									bg = pal.surface3
									border = pal.hairlineTertiary
								}
								btn.Background(bg).Border(1, border)
								fadeColors(btn)

								btn.Children(func() {
									if isAdded {
										drawIcon(c, iconCheck, 11, t.Success)
									} else {
										ui.Text(c, "+").FontSize(textCaption).TextColor(pal.inkMuted)
									}
									ui.Text(c, ps.Label).FontSize(textCaption).TextColor(pal.ink).SingleLine()
									if isAdded {
										ui.Box(c).Padding(1, 4).Radius(radiusXS).Background(pal.surface1).
											Border(1, pal.hairline).Children(func() {
											ui.Text(c, T("已添加", "Added")).FontSize(9).TextColor(pal.inkTertiary)
										})
									} else if strings.HasPrefix(ps.Label, "Magpie") && a.magpieOnline {
										ui.Box(c).Padding(1, 4).Radius(radiusXS).Background(pal.surface1).
											Border(1, pal.hairline).Children(func() {
											ui.Text(c, T("在线", "Online")).FontSize(9).
												FontWeight(weightMedium).TextColor(t.Success)
										})
									}
								})

								if btn.Clicked() {
									if isAdded {
										a.editIdx = existingIdx
										a.editCfg.Active = existingIdx
									} else {
										a.addProvider(ps)
									}
								}
							}
						})
					})

					settingDivider(c, pal)

					// 行 3: 本地网关状态探测
					settingRow(c, pal, T("本地网关 (127.0.0.1:3425)", "Local Gateway (127.0.0.1:3425)"),
						T("免鉴权直连本机运行的 Magpie 或兼容网关", "Direct local connection for Magpie or compatible gateway"),
						func() {
							ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
								// 网关在不在线是一句状态，不是一个严重程度：底色
								// 保持中性，只有那颗圆点和文字跟着语义走。
								statusCol := pal.inkTertiary
								statusText := T("未检测到本地网关", "Gateway Offline")
								statusDot := pal.inkTertiary
								if a.magpieOnline {
									statusCol = t.Success
									statusDot = t.Success
									statusText = T("本地网关在线", "Gateway Online")
								}
								ui.Row(c).Shrink(0).Height(statusChipH).AlignItems(ui.Center).Gap(5).
									Padding(0, 8).Radius(radiusPill).Background(pal.surface2).
									Border(1, pal.hairline).Children(func() {
									ui.Box(c).Size(chipDot, chipDot).Shrink(0).Radius(chipDot / 2).
										Background(statusDot)
									ui.Text(c, statusText).FontSize(textCaption).
										FontWeight(weightMedium).TextColor(statusCol)
								})

								refreshBtn := ui.ButtonBase(c).Size(24, 24).Radius(radiusSM).
									Background(ui.Transparent).Border(1, pal.hairlineStrong).
									AlignItems(ui.Center).Justify(ui.Center).
									Cursor(ui.CursorPointer).Label(T("检测网关", "Check Gateway")).Tooltip(T("重新探测本地网关", "Re-probe gateway"))
								if refreshBtn.Hovered() {
									refreshBtn.Background(pal.surface2)
								}
								fadeColors(refreshBtn)
								refreshBtn.Children(func() {
									drawIcon(c, iconRefresh, 12, pal.inkSubtle)
								})
								if refreshBtn.Clicked() {
									a.probeMagpie()
								}
							})
						})
				})

			// 2. 服务商详细配置参数 (Provider Configuration)
			settingSection(c, pal, T("服务商参数配置", "Provider Parameters"),
				T("配置当前选定服务商的连接协议、密钥与模型标识", "Configure connection protocol, credentials, and model identifiers for selected provider"),
				func() {
					// 字段 1: 服务商名称
					settingRow(c, pal, T("服务商名称", "Provider Name"),
						T("在列表中显示的自定义标识名称", "Display label for this provider"),
						func() {
							ui.TextInput(c, &p.Name).Placeholder(T("例如：Magpie、公司自建", "e.g. Magpie, Custom")).
								Label(T("服务商名称", "Provider Name")).Width(260)
						})

					settingDivider(c, pal)

					// 字段 2: 接口协议
					settingRow(c, pal, T("接口协议", "API Protocol"),
						T("选择 API 通信协议规范", "Protocol specification format"),
						func() {
							mode := 0
							if p.Type == "openai" {
								mode = 1
							}
							ui.Segmented(c, &mode, "Anthropic", T("OpenAI 兼容", "OpenAI Compatible"))
							if mode == 1 {
								p.Type = "openai"
							} else {
								p.Type = "anthropic"
							}
						})

					settingDivider(c, pal)

					// 字段 3: Base URL
					hint := T("官方服务留空，亦可填入中转或代理地址", "Leave empty for official, or enter gateway URL")
					if isMagpie(p.BaseURL) {
						hint = T("已连接本地网关 (127.0.0.1:3425)", "Connected to local gateway (127.0.0.1:3425)")
					}
					settingRow(c, pal, "Base URL", hint, func() {
						ph := "https://api.example.com/v1"
						if p.Type == "anthropic" {
							ph = T("留空使用官方接口", "Leave empty for official API")
						}
						ui.TextInput(c, &p.BaseURL).Placeholder(ph).Label("Base URL").Width(280)
					})

					settingDivider(c, pal)

					// 字段 4: API Key
					keyTip := T("未配置（默认读取系统环境变量）", "Not configured (reads env)")
					if isMagpie(p.BaseURL) {
						keyTip = T("✓ 本地网关免鉴权直连", "✓ Local gateway connected (no key required)")
					} else if p.APIKey != "" {
						keyTip = T("✓ 密钥已配置", "✓ API key configured")
					}
					settingRow(c, pal, T("API 密钥 (API Key)", "API Key"), keyTip, func() {
						ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
							keyPh := "sk-…"
							if isMagpie(p.BaseURL) {
								keyPh = T("免密钥（可留空）", "Optional")
							}
							in := ui.TextInput(c, &p.APIKey).Placeholder(keyPh).Label("API Key").Width(220)
							if !a.showAPIKey {
								in.Password()
							}

							eyeBtn := ui.ButtonBase(c).Size(26, 26).Radius(radiusSM).
								Background(ui.Transparent).Border(1, pal.hairlineStrong).
								AlignItems(ui.Center).Justify(ui.Center).
								Cursor(ui.CursorPointer).Label(T("切换密钥显示", "Toggle Key Visibility")).Tooltip(T("切换明文/密文显示", "Toggle visibility"))
							if eyeBtn.Hovered() {
								eyeBtn.Background(pal.surface2)
							}
							fadeColors(eyeBtn)
							eyeSvg := iconEye
							if a.showAPIKey {
								eyeSvg = iconEyeOff
							}
							eyeBtn.Children(func() {
								drawIcon(c, eyeSvg, 13, pal.inkSubtle)
							})
							if eyeBtn.Clicked() {
								a.showAPIKey = !a.showAPIKey
							}
						})
					})

					settingDivider(c, pal)

					// 字段 5: 模型标识
					modelTip := T("指定模型名称或标识符（支持服务商前缀）", "Model identifier with optional provider prefix")
					if p.Type == "anthropic" {
						modelTip = T("留空默认使用 ", "Default: ") + llm.DefaultModel
					}
					settingRow(c, pal, T("模型标识 (Model)", "Model Identifier"), modelTip, func() {
						phModel := llm.DefaultModel
						if p.Type == "openai" {
							phModel = "例如 opencode-go/deepseek-v4.1-flash"
						}
						ui.TextInput(c, &p.Model).Placeholder(phModel).Label(T("模型名称", "Model Name")).Width(260)
					})

					// 字段 6: 网关检测到的可用模型
					if isMagpie(p.BaseURL) && len(a.magpieModels) > 0 {
						settingDivider(c, pal)
						ui.Column(c).Padding(7, 14).Gap(6).FillWidth().Children(func() {
							ui.Row(c).AlignItems(ui.Center).Children(func() {
								ui.Text(c, T("网关检测可用模型", "Gateway Detected Models")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
								ui.Spacer(c)
								ui.Text(c, T("点击快速填入模型标识", "Click to select model")).FontSize(textCaption).TextColor(pal.inkSubtle)
							})

							ui.Row(c).Gap(6).Wrap().Children(func() {
								for _, m := range a.magpieModels {
									m := m
									selected := p.Model == m
									chip := ui.ButtonBase(c).Height(24).Padding(0, 8).Radius(radiusXS).
										Cursor(ui.CursorPointer).Label(m)
									if selected {
										chip.Background(pal.selection).Border(1, pal.hairlineTertiary)
									} else {
										chip.Background(pal.surface2).Border(1, pal.hairlineStrong)
										if chip.Hovered() {
											chip.Background(pal.surface3)
										}
									}
									fadeColors(chip)

									parts := strings.SplitN(m, "/", 2)
									provTag, modelTag := "", m
									if len(parts) == 2 {
										provTag, modelTag = parts[0], parts[1]
									}

									chip.Children(func() {
										ui.Row(c).AlignItems(ui.Center).Gap(3).Children(func() {
											if provTag != "" {
												// 服务商前缀压到最暗一档：它只是
												// 「这个名字属于谁」的索引。
												ui.Text(c, provTag+"/").FontSize(9.5).
													TextColor(pal.inkTertiary)
											}
											col := pal.inkMuted
											if selected {
												col = pal.ink
											}
											ui.Text(c, modelTag).FontSize(textCaption).
												FontWeight(weightMedium).TextColor(col)
										})
									})
									if chip.Clicked() {
										p.Model = m
									}
								}
							})
						})
					}
				})
		})
	})
}

// addProvider 按预设追加一个供应商并选中它。若已存在，则直接聚焦至已有项，避免重复添加。
func (a *app) addProvider(ps preset) {
	if idx := findExistingPreset(a.editCfg.Providers, ps); idx >= 0 {
		a.editIdx = idx
		a.editCfg.Active = idx
		return
	}
	a.editCfg.Providers = append(a.editCfg.Providers, llm.Provider{
		Name:    ps.Label,
		Type:    ps.Type,
		BaseURL: ps.Base,
		Model:   ps.Model,
	})
	a.editCfg.Active = len(a.editCfg.Providers) - 1
	a.editIdx = a.editCfg.Active
}

func (a *app) removeProvider(i int) {
	if i < 0 || i >= len(a.editCfg.Providers) {
		return
	}
	a.editCfg.Providers = append(a.editCfg.Providers[:i], a.editCfg.Providers[i+1:]...)
	if a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = len(a.editCfg.Providers) - 1
	}
	if a.editCfg.Active < 0 {
		a.editCfg.Active = 0
	}
	if a.editIdx >= len(a.editCfg.Providers) {
		a.editIdx = len(a.editCfg.Providers) - 1
	}
	if a.editIdx < 0 {
		a.editIdx = 0
	}
}

// applySettings 保存设置，改名或换模型都会让旧解释对不上，所以顺便
// 关掉当前卡片 —— 它显示的可能已经是另一个模型的话了。
func (a *app) applySettings() {
	cfg := a.store.Config()
	cfg.Providers = a.editCfg.Providers
	cfg.Active = a.editCfg.Active
	cfg.ThemeMode = a.themeMode
	cfg.Locale = a.locale
	a.store.SetConfig(cfg)
	a.persist()
	a.modelName = a.store.Provider().EffectiveModel()

	a.settingsOn = false
	if a.explainOpen() {
		a.closeExplain()
	}
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

// secretHint 在列表里提示有没有填 key —— 不显示 key 本身。
func secretHint(p llm.Provider) string {
	if p.APIKey == "" {
		return T("未配置密钥", "No API Key")
	}
	return T("已配置密钥", "API Key Set")
}
