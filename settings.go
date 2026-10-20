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

// settingsDialog 是偏好与模型设置对话框的入口。
func (a *app) settingsDialog(c *ui.Context, pal palette) {
	if !a.settingsOn {
		return
	}
	// 面板尺寸跟着窗口收。固定 960×680 在最小窗口（720×620）下四边都会
	// 冲出屏幕：底部的「保存 / 取消」被推到窗口外面，用户根本点不到。
	// 四周各留 16px 呼吸，上限仍是设计稿的 960×680。
	winW, winH := c.Size()
	panelW := clamp(winW-32, 420, 960)
	panelH := clamp(winH-32, 320, 680)

	open := true
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		// 设置窗口是最顶层的一块浮层，按 §2.1 落 Surface 4、圆角 xl(16)（§3）。
		//
		// 用 DialogBase 而不是 Modal：Modal 会先铺一层自己的面板（Surface 3、
		// 圆角 10、四周 20px 内边距），再让内容画上去。设置面板自己就是一块
		// 920×660 的卡片，套进去等于两层圆角不一致的框叠在一起 —— 截图里
		// 看得很清楚，外圈的角比内圈的角更方。
		panel.Width(panelW).Height(panelH).Radius(radiusXL).
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
		ui.Column(c).Width(180).FillHeight().Background(pal.surface1).
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

			// 通用偏好在模型设置前头：这一页改的是界面本身（外观、语言、拓扑图
			// 配色、终端字体），打开设置的人十次里有九次是来找它的；模型与密钥
			// 配完一次就很少再动。
			settingsSidebarItem(c, t, T("通用偏好", "Preferences"), iconPalette, a.settingsTab == settingsTabGeneral, func() {
				a.settingsTab = settingsTabGeneral
			})

			settingsSidebarItem(c, t, T("模型设置", "Model Settings"), iconSparkles, a.settingsTab == settingsTabModels, func() {
				a.settingsTab = settingsTabModels
			})

			settingsSidebarItem(c, t, T("提示词设置", "Prompts"), iconSliders, a.settingsTab == settingsTabPrompts, func() {
				a.settingsTab = settingsTabPrompts
			})

			// Git 配置紧跟在几个「Gloss 自己的设置」后面：这一页改的其实是
			// git 的配置文件，但那也是设置里的一类，不该被塞进「系统」组 ——
			// 组里那两项是更新与关于，跟仓库没关系。
			settingsSidebarItem(c, t, T("Git 配置", "Git Config"), iconGitBranch, a.settingsTab == settingsTabGit, func() {
				a.settingsTab = settingsTabGit
			})

			// 分组 2: 系统
			ui.Text(c, T("系统", "System")).FontSize(textCaption).FontWeight(weightSemibold).TextColor(pal.inkSubtle).
				Padding(12, 8, 2, 8)

			settingsSidebarItem(c, t, T("更新", "Updates"), iconRefresh, a.settingsTab == settingsTabUpdates, func() {
				a.settingsTab = settingsTabUpdates
			})

			settingsSidebarItem(c, t, T("关于 Gloss", "About Gloss"), iconEye, a.settingsTab == settingsTabAbout, func() {
				a.settingsTab = settingsTabAbout
			})

			ui.Spacer(c)

			// 底部版本信息
			ui.Row(c).Padding(8, 8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Gloss "+appVersionLabel()).FontSize(textCaption).TextColor(pal.inkTertiary)
			})
		})

		// 2. 右侧主内容区 (Main Content Pane)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Background(pal.surface3).Children(func() {
			// 非模型选项卡提供极简微顶栏放关闭按钮；模型选项卡直接顶格，布局保持极简纯净
			if a.settingsTab != settingsTabModels {
				ui.Row(c).Height(38).Padding(0, 20, 0, 24).AlignItems(ui.Center).FillWidth().Children(func() {
					ui.Spacer(c)
					if closeButton(c, pal) {
						a.settingsOn = false
					}
				})
			}

			// 主体内容区（独立的填满空间，由各 Pane 内部全幅滚动）
			ui.Box(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				if a.settingsTab == settingsTabGeneral {
					a.generalSettingsPane(c, pal)
				} else if a.settingsTab == settingsTabPrompts {
					a.promptsSettingsPane(c, pal)
				} else if a.settingsTab == settingsTabAbout {
					a.aboutSettingsPane(c, pal)
				} else if a.settingsTab == settingsTabUpdates {
					a.updatesSettingsPane(c, pal)
				} else if a.settingsTab == settingsTabGit {
					a.gitSettingsPane(c, pal)
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
			//
			// 只在 macOS 上摆：这一项改的是 Dock 图标和系统「关于」里的
			// 那个（见 syncAppIcon），Windows 上 SetDockIcon 会去设窗口
			// 图标、Linux 上什么都不做 —— 摆出来点得动却没处看，不如不摆。
			// 别的地方的图标按平台各有一套默认，不需要用户选。
			if isMacPlatform() {
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
						{"white", T("纯白极简", "Pure White"), T("明亮纯白质感 · 干净通透", "Porcelain clean white · Clean & crisp"), bmWhite},
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
			}

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
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label("−").Tooltip(Tf("缩小代码字号 %s", "Zoom Out %s", K("⌘−")))
						if minusBtn.Hovered() {
							minusBtn.Background(pal.surface3)
						}
						fadeColors(minusBtn)
						minusBtn.Children(func() { ui.Text(c, "−").FontSize(textBody).TextColor(pal.ink) })
						if minusBtn.Clicked() {
							a.setCodeSize(a.codeSizeValue() - codeSizeStep)
						}

						plusBtn := ui.ButtonBase(c).Size(28, 28).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label("+").Tooltip(Tf("放大代码字号 %s", "Zoom In %s", K("⌘+")))
						if plusBtn.Hovered() {
							plusBtn.Background(pal.surface3)
						}
						fadeColors(plusBtn)
						plusBtn.Children(func() { ui.Text(c, "+").FontSize(textBody).TextColor(pal.ink) })
						if plusBtn.Clicked() {
							a.setCodeSize(a.codeSizeValue() + codeSizeStep)
						}

						resetBtn := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(T("重置", "Reset")).Tooltip(Tf("恢复默认字号 %s", "Actual Size %s", K("⌘0")))
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

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 5. 提交拓扑图配色：先选方案，再按号微调。
			//
			// 覆盖按明暗各存一份，两排色井都摆出来 —— 同一支色落在深色底和
			// 浅色底上的可读性差得很远，而「随系统」的人两个外观都会看到，
			// 只显示当前那一排的话，另一份就得切一次外观才能调。
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					drawIcon(c, iconPalette, 15, t.Accent)
					ui.Text(c, T("拓扑图配色", "Commit Graph Colors")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				})
				ui.Text(c, T("提交列表左边那几条泳道线的颜色。0 号是主干，也用在「当前分支」的圆点上；自定义只改拓扑图，状态字母和分支芯片的亮色不动。", "Colors of the lane lines in the commit list. Lane 0 is the mainline, shared with the current-branch dot; overrides apply to the graph only, never to status letters or ref chips.")).
					FontSize(11).TextColor(t.TextMuted)

				ui.Row(c).Gap(12).FillWidth().Children(func() {
					for _, lp := range lanePresets {
						lp := lp
						name, desc := lanePresetLabel(lp.id)
						selected := a.graphPreset == lp.id || (a.graphPreset == "" && lp.id == lanePresets[0].id)
						card := ui.ButtonBase(c).Grow(1).Height(60).Radius(radiusMD).Padding(8, 12).
							Cursor(ui.CursorPointer).Label(Tf("%s配色", "%s colors", name))

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
							ui.Column(c).Gap(4).Grow(1).Children(func() {
								ui.Text(c, name).FontSize(textBodySM).
									FontWeight(weightSemibold).TextColor(pal.ink)
								ui.Text(c, desc).FontSize(textCaption).TextColor(pal.inkSubtle).SingleLine()
								// 一排色点：这套方案长什么样扫一眼就知道，不用先选上再去看图。
								ui.Row(c).Gap(3).Children(func() {
									for _, col := range lp.lanes(a.isDark(c)) {
										ui.Box(c).Size(9, 9).Radius(radiusPill).Background(col)
									}
								})
							})
						})

						if card.Clicked() {
							a.setGraphPreset(lp.id)
						}
					}
				})

				for _, row := range []struct {
					dark  bool
					label string
				}{
					{true, T("深色外观", "Dark appearance")},
					{false, T("浅色外观", "Light appearance")},
				} {
					row := row
					ui.Row(c).AlignItems(ui.Center).Gap(10).FillWidth().Children(func() {
						ui.Text(c, row.label).FontSize(textCaption).Width(76).Shrink(0).TextColor(pal.inkMuted)
						laneCols := a.laneSet().lanes(row.dark)
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							for i := 0; i < laneCount; i++ {
								i := i
								col := laneCols[i]
								tip := Tf("第 %d 号泳道线的颜色", "Color of lane %d", i)
								if i == 0 {
									tip = T("主干（0 号）的颜色", "Mainline (lane 0) color")
								}
								ui.Column(c).Gap(2).AlignItems(ui.Center).Children(func() {
									ui.Text(c, Tf("%d", "%d", i)).FontSize(9).TextColor(pal.inkTertiary)
									if ui.ColorWell(c, &col).Label(tip).Tooltip(tip).Changed() {
										a.setLaneOverride(row.dark, i, hexOf(col))
									}
								})
							}
						})
					})
				}

				if clearBtn := secondaryAction(c, pal,
					T("清除自定义颜色", "Clear Custom Colors"),
					T("回到方案自带的 8 色", "Back to the preset's 8 colors"),
					!a.hasLaneOverrides()); clearBtn.Clicked() {
					a.clearLaneOverrides()
				}
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 6. 内置终端：字体族与字号。
			//
			// 这两项都推给已经开着的 shell（见 syncTerminalFont），所以改完
			// 不用重启终端 —— 重启会把屏幕上的输出和正在跑的命令一起丢掉。
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					drawIcon(c, iconTerminal, 15, t.Accent)
					ui.Text(c, T("内置终端", "Built-in Terminal")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				})
				ui.Text(c, T("字体族从下拉里挑一个等宽字体：这台机器上装着的排在前面（Nerd Font 最前），没装的留在后面当候选，选了会退到系统等宽。改动对已经开着的会话立即生效。", "Pick a monospace family: what this machine has installed comes first (Nerd Fonts on top), uninstalled ones stay below as candidates and fall back to the system monospace. Changes reach open sessions at once.")).
					FontSize(11).TextColor(t.TextMuted)

				// 要用的那一帧才去扫已装字体（见 ensureFontScan）。
				a.ensureFontScan()

				ui.Row(c).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
					ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, T("字体族", "Font Family")).FontSize(textBodySM).
							FontWeight(weightMedium).TextColor(pal.ink)
						// 存的是单一字族；空串（手改配置文件留下的）在下拉里按
						// 「系统等宽」显示 —— 两者本来就是同一件事。
						family := a.termFontFamily
						if family == "" {
							family = termFontGenericMono
						}
						if ui.Combobox(c, &family, a.termFontOptions()).
							FillWidth().Label(T("终端字体族", "Terminal font family")).Changed() {
							a.setTermFontFamily(family)
						}
						if !a.fontScanDone {
							ui.Text(c, T("正在读取已装字体…", "Reading installed fonts…")).
								FontSize(textCaption).TextColor(pal.inkTertiary)
						}
					})
					reset := secondaryAction(c, pal, T("默认字体", "Default font"), T("用系统等宽字体", "Use the system monospace"), a.termFontFamily == "" || a.termFontFamily == termFontGenericMono)
					if reset.Clicked() {
						a.setTermFontFamily("")
					}
				})

				ui.Row(c).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
					ui.Column(c).Gap(2).Grow(1).Children(func() {
						ui.Text(c, T("字号", "Font Size")).FontSize(textBodySM).
							FontWeight(weightMedium).TextColor(pal.ink)
						sizeVal := a.termFontSizeValue()
						sizeLabel := Tf("%.0f px", "%.0f px", sizeVal)
						if a.termFontSize == 0 {
							sizeLabel = Tf("%.0f px (默认)", "%.0f px (Default)", sizeVal)
						}
						ui.Text(c, sizeLabel).FontSize(textCaption).TextColor(pal.inkSubtle)
					})

					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						minusBtn := ui.ButtonBase(c).Size(28, 28).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(T("缩小终端字号", "Smaller terminal font")).Tooltip(T("缩小终端字号", "Smaller terminal font"))
						if minusBtn.Hovered() {
							minusBtn.Background(pal.surface3)
						}
						fadeColors(minusBtn)
						minusBtn.Children(func() { ui.Text(c, "−").FontSize(textBody).TextColor(pal.ink) })
						if minusBtn.Clicked() {
							a.setTermFontSize(a.termFontSizeValue() - termFontStep)
						}

						plusBtn := ui.ButtonBase(c).Size(28, 28).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(T("放大终端字号", "Larger terminal font")).Tooltip(T("放大终端字号", "Larger terminal font"))
						if plusBtn.Hovered() {
							plusBtn.Background(pal.surface3)
						}
						fadeColors(plusBtn)
						plusBtn.Children(func() { ui.Text(c, "+").FontSize(textBody).TextColor(pal.ink) })
						if plusBtn.Clicked() {
							a.setTermFontSize(a.termFontSizeValue() + termFontStep)
						}

						resetBtn := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairlineStrong).
							AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(T("重置字号", "Reset size")).Tooltip(T("恢复默认字号", "Back to the default size"))
						if resetBtn.Hovered() {
							resetBtn.Background(pal.surface3)
						}
						fadeColors(resetBtn)
						resetBtn.Children(func() {
							ui.Text(c, T("重置字号", "Reset size")).FontSize(textButton).FontWeight(weightMedium).TextColor(pal.inkMuted)
						})
						if resetBtn.Clicked() {
							a.setTermFontSize(0)
						}
					})
				})
			})

			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)

			// 7. 远程同步：后台自动抓取。
			//
			// 只抓不拉。抓取（git fetch）只更新远程跟踪分支，不碰工作区和
			// index，不可能在半路撞出冲突；拉取会把提交合并进当前分支，那一下
			// 必须用户自己点（工具栏那颗、⌘⇧P）。开着它，侧栏的远程分支和每个
			// 本地分支后面的 ↑n ↓n 会自己跟上，不用先手动抓一次。
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					drawIcon(c, iconFetch, 15, t.Accent)
					ui.Text(c, T("远程同步", "Remote Sync")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				})

				on := a.autoFetch
				settingRow(c, pal,
					T("后台自动抓取", "Fetch in the Background"),
					T("每 5 分钟对所有远程抓取一次，只更新远程分支，不动工作区。", "Fetches all remotes every 5 minutes. Updates remote-tracking refs only; never touches the working tree."),
					func() {
						// 开关挂一个自己的名字：屏幕阅读器念得出「这是个开关、
						// 现在开着没有」，测试也点得到它（行左边那行字是纯文本，
						// 点了不 toggle）。
						sw := ui.Switch(c, &on).Label(T("后台自动抓取开关", "Fetch in the Background switch"))
						if sw.Clicked() {
							a.setAutoFetch(on)
						}
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
							ui.Text(c, appVersionLabel()).FontSize(textCaption).FontWeight(weightMedium).
								TextColor(pal.inkSubtle)
						})
					})
					ui.Text(c, T("原生智能 Git 客户端", "Native Git GUI with AI-powered diff review")).
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
						T("AI 变更审查", "AI Change Review"),
						T("变更块级风险提示与一键智能提交信息生成", "Risk analysis on code changes and Conventional Commits generation"),
					},
					{
						iconPalette,
						T("克制优雅的视觉设计", "Focused & Distraction-Free"),
						T("深度集成系统外观，深浅色无缝切换，全 Neutral 极简黑白图标设计，零多余杂色", "Deep system appearance integration, seamless dark/light switching, pure neutral monochrome icons"),
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

			// 外部链接按钮：官网落地宣传页与 GitHub
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				btnWeb := secondaryAction(c, pal, T("访问官方网站", "Official Website"),
					T("在浏览器中打开 Gloss 落地宣传页与在线演示。", "Open the Gloss landing page in your browser."), false)
				if btnWeb.Clicked() && a.openURL != nil {
					a.openURL("https://carlopenlab.github.io/Gloss/")
				}

				btnRepo := secondaryAction(c, pal, "GitHub",
					T("访问 GitHub 开源项目仓库与最新发布版。", "Open the GitHub open-source repository and latest releases."), false)
				if btnRepo.Clicked() && a.openURL != nil {
					a.openURL("https://github.com/CarlOpenLab/Gloss")
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
// 状态全部来自 mygo 的 updater 插件；未启用更新的构建只展示说明，不给出可点的控件效果。
func (a *app) updatesSettingsPane(c *ui.Context, pal palette) {
	prefs := loadUpdatePrefs()

	ui.Scroll(c).FillWidth().FillHeight().Padding(28, 36).Children(func() {
		ui.Column(c).Gap(18).FillWidth().Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				drawIcon(c, iconRefresh, 16, pal.ink)
				ui.Text(c, T("软件更新", "Software Update")).FontSize(textDisplay).FontWeight(weightSemibold).TextColor(pal.ink)
			})

			settingSection(c, pal, T("版本", "Version"),
				T("更新源：github.com/CarlOpenLab/Gloss 的正式发布版。", "Update channel: official releases from github.com/CarlOpenLab/Gloss."),
				func() {
					settingRow(c, pal, T("当前版本", "Current Version"), "", func() {
						ui.Text(c, appVersionLabel()).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.inkMuted)
					})
					settingDivider(c, pal)
					settingRow(c, pal, T("上次检查", "Last Checked"), "", func() {
						ui.Text(c, lastCheckLabel(prefs.lastCheck)).FontSize(textBodySM).TextColor(pal.inkMuted)
					})
				})

			checksDesc := T("启动时及后台定期检查新版本。", "Check for new versions at launch and periodically in the background.")
			if !prefs.available {
				checksDesc = T("当前构建未启用自动更新（开发构建，或由包管理器安装）。", "Automatic updates are unavailable in this build.")
			}
			settingSection(c, pal, T("自动更新", "Automatic Updates"), checksDesc, func() {
				if !prefs.available {
					settingRow(c, pal, T("自动检查更新", "Check for Updates Automatically"), "", func() {
						ui.Text(c, T("不可用", "Unavailable")).FontSize(textBodySM).TextColor(pal.inkTertiary)
					})
					settingDivider(c, pal)
					settingRow(c, pal, T("自动下载更新", "Download Updates Automatically"), "", func() {
						ui.Text(c, T("不可用", "Unavailable")).FontSize(textBodySM).TextColor(pal.inkTertiary)
					})
					return
				}
				autoCheck := prefs.automaticChecks
				settingRow(c, pal, T("自动检查更新", "Check for Updates Automatically"), "", func() {
					sw := ui.Switch(c, &autoCheck)
					if sw.Clicked() {
						applyAutomaticChecks(prefs, autoCheck)
					}
				})
				settingDivider(c, pal)
				autoDownload := prefs.automaticDownloads
				settingRow(c, pal, T("自动下载更新", "Download Updates Automatically"),
					T("发现新版本后后台下载，下次启动时安装。", "Download new versions in the background and install them on the next launch."), func() {
						sw := ui.Switch(c, &autoDownload)
						if sw.Clicked() {
							applyAutomaticDownloads(prefs, autoDownload)
						}
					})
			})

			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				btn := secondaryAction(c, pal, T("检查更新…", "Check for Updates…"),
					T("立即向更新源查询是否有新版本。", "Query the update channel for a new version now."), !prefs.available)
				if btn.Clicked() && prefs.available {
					checkForUpdatesNow()
				}
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

// modelsSettingsPane 呈现参考用户界面的 Master-Detail 双栏模型供应管理面板。
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

	ui.Row(c).FillWidth().FillHeight().Children(func() {
		// ==========================================
		// 1. 左侧子栏：供应商列表 (宽度 240)
		// ==========================================
		ui.Column(c).Width(240).FillHeight().Background(pal.surface1).
			BorderWidth(0, 1, 0, 0).BorderColor(pal.hairline).Children(func() {

			// 搜索框
			ui.Row(c).Padding(10, 10, 6, 10).FillWidth().Children(func() {
				ui.TextInput(c, &a.providerSearch).Placeholder(T("搜索供应商", "Search providers")).FillWidth()
			})

			// 可滚动供应商列表。
			//
			// 这一栏只画「已经加进来的」供应商。未添加的预设原来接在已添加
			// 那段的屁股后面，点一下会当场跳进上面那段、下面的行跟着整体
			// 挪一格 —— 用起来就像「点了它一下，排序自己变了」。加预设改走
			// 底部「添加供应商」那个菜单（见 addProviderMenu）。
			//
			// 顺序由用户自己拖行首那个手柄决定。拖动是自己接的（没有用
			// 框架 List 的 Reorder）：那条路要求整行都是拖动源，而行本身是
			// 个按钮 —— 按钮会先一步吃掉按下事件，拖不起来。手柄当拖动源、
			// 行自己当落点，正好也就成了「只有手柄能拖」（见 providerRow）。
			rows := a.visibleProviders()

			ui.Scroll(c).Grow(1).FillWidth().Padding(4, 6).Children(func() {
				ui.Column(c).Gap(2).FillWidth().Children(func() {
					for k, i := range rows {
						k, i := k, i
						// 上沿 = 插在这一家前面，下沿 = 插在下一家前面
						// （末尾那一行的下沿就是「放到最后」）。
						next := len(a.editCfg.Providers)
						if k+1 < len(rows) {
							next = rows[k+1]
						}
						a.providerRow(c, pal, t, i, next)
					}
				})
			})

			// 底部 + 添加 按钮：点开一份预设清单
			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)
			addBtn := ui.ButtonBase(c).Height(36).FillWidth().Padding(0, 12).
				AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(T("添加供应商", "Add Provider"))
			addBtn.Menu(func(m *ui.Menu) { a.addProviderMenu(m) })
			if addBtn.Hovered() {
				addBtn.Background(pal.surface2)
			}
			fadeColors(addBtn)
			addBtn.Children(func() {
				ui.Row(c).Fill().AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Text(c, "+").FontSize(14).TextColor(pal.inkMuted)
					ui.Text(c, T("添加供应商", "Add Provider")).FontSize(textCaption).FontWeight(weightMedium).TextColor(pal.ink)
					ui.Spacer(c)
					drawIcon(c, iconChevronDown, 10, pal.inkSubtle)
				})
			})
		})

		// ==========================================
		// 2. 右侧子栏：服务商详情及模型配置 (对齐截图)
		// ==========================================
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {

			// 详情顶部 Header（图标、名称、状态与主开关）
			ui.Row(c).Height(54).Padding(0, 20).AlignItems(ui.Center).FillWidth().
				BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Box(c).Size(28, 28).Radius(radiusSM).Background(pal.surface2).
						Border(1, pal.hairline).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						drawIcon(c, iconSparkles, 14, pal.ink)
					})
					ui.Column(c).Gap(1).Children(func() {
						ui.Text(c, providerLabel(*p)).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
						statusDesc := T("未生效", "Inactive")
						statusCol := pal.inkSubtle
						if a.editIdx == a.editCfg.Active {
							statusDesc = T("已启用", "Enabled")
							statusCol = t.Success
						}
						ui.Text(c, statusDesc).FontSize(textCaption).TextColor(statusCol)
					})
				})
				ui.Spacer(c)
				// 主开关
				isActive := a.editIdx == a.editCfg.Active
				sw := ui.Switch(c, &isActive)
				if sw.Clicked() {
					// 这个开关是用户自己控制启用/禁用的地方：点选供应商
					// 只选中编辑，开不开由这里说了算（可开可关）。
					a.toggleEditActive()
				}
				ui.Box(c).Width(12)
				if closeButton(c, pal) {
					a.settingsOn = false
				}
			})

			// 详情滚动区域
			ui.Scroll(c).Grow(1).FillWidth().Padding(16, 20).Children(func() {
				ui.Column(c).Gap(14).FillWidth().Children(func() {

					// 基础 URL
					ui.Column(c).Gap(4).FillWidth().Children(func() {
						ui.Text(c, T("基础 URL", "Base URL")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
						ph := "https://api.anthropic.com"
						if p.Type == "openai" {
							ph = "https://api.openai.com/v1"
						}
						if isMagpie(p.BaseURL) {
							ph = "http://127.0.0.1:3425/v1"
						}
						ui.TextInput(c, &p.BaseURL).Placeholder(ph).FillWidth()
					})

					// API 密钥
					ui.Column(c).Gap(4).FillWidth().Children(func() {
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							ui.Text(c, T("API 密钥 (API Key)", "API Key")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
							ui.Spacer(c)
							keyTip := T("未配置", "Not set")
							if isMagpie(p.BaseURL) {
								keyTip = T("本地网关免密钥直连", "No key required")
							} else if p.APIKey != "" {
								keyTip = T("✓ 密钥已配置", "✓ Configured")
							}
							ui.Text(c, keyTip).FontSize(textCaption).TextColor(pal.inkSubtle)
						})
						ui.Row(c).AlignItems(ui.Center).Gap(6).FillWidth().Children(func() {
							keyPh := "sk-…"
							if isMagpie(p.BaseURL) {
								keyPh = T("免密钥（可留空）", "Optional")
							}
							in := ui.TextInput(c, &p.APIKey).Placeholder(keyPh).Grow(1)
							if !a.showAPIKey {
								in.Password()
							}
							eyeBtn := ui.ButtonBase(c).Size(28, 28).Radius(radiusSM).
								Background(pal.surface2).Border(1, pal.hairlineStrong).
								AlignItems(ui.Center).Justify(ui.Center).
								Cursor(ui.CursorPointer).Label(T("切换密钥显示", "Toggle Key Visibility"))
							if eyeBtn.Hovered() {
								eyeBtn.Background(pal.surface3)
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

					// 供应商名称与协议
					ui.Row(c).Gap(12).FillWidth().Children(func() {
						ui.Column(c).Gap(4).Grow(1).Children(func() {
							ui.Text(c, T("供应商名称", "Provider Name")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
							ui.TextInput(c, &p.Name).Placeholder(providerLabel(*p)).FillWidth()
						})
						ui.Column(c).Gap(4).Shrink(0).Children(func() {
							ui.Text(c, T("接口协议", "Protocol")).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink)
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
					})

					// 模型列表区域
					modelsList := a.modelsForProvider(p)
					var filteredModels []modelTagInfo
					for _, m := range modelsList {
						if a.modelSearch == "" || strings.Contains(strings.ToLower(m.ID), strings.ToLower(a.modelSearch)) {
							filteredModels = append(filteredModels, m)
						}
					}

					ui.Column(c).Gap(8).FillWidth().Children(func() {
						// 标题与模型数量
						ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
							drawIcon(c, iconCheck, 14, t.Success)
							ui.Text(c, Tf("模型 · %d / %d", "Models · %d / %d", len(filteredModels), len(modelsList))).
								FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.ink)
						})

						// 搜索模型
						ui.TextInput(c, &a.modelSearch).Placeholder(T("搜索模型名称或 ID", "Search model name or ID")).FillWidth()

						// 模型列表框
						ui.Column(c).Radius(radiusMD).Background(pal.surface2).Border(1, pal.hairline).FillWidth().Children(func() {
							for _, m := range filteredModels {
								m := m
								isCurrent := p.Model == m.ID
								row := ui.ButtonBase(c).Height(38).Padding(0, 12).Radius(radiusSM).FillWidth().
									Cursor(ui.CursorPointer).Label(m.ID)
								bg := ui.Transparent
								if row.Hovered() {
									bg = pal.surface3.Alpha(0.6)
								}
								row.Background(bg)
								fadeColors(row)

								row.Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(8).Fill().Children(func() {
										txtCol := pal.ink
										if isCurrent {
											txtCol = pal.ink
										}
										ui.Text(c, m.ID).FontSize(textBodySM).FontWeight(weightMedium).TextColor(txtCol)

										// 标签 (视觉、推理、代码等)
										for _, tag := range m.Tags {
											ui.Box(c).Padding(1, 6).Radius(radiusXS).
												Background(t.Success.Alpha(0.12)).Children(func() {
												ui.Text(c, tag).FontSize(10).FontWeight(weightMedium).TextColor(t.Success)
											})
										}

										ui.Spacer(c)

										// 模型选择开关
										swVal := isCurrent
										sw := ui.Switch(c, &swVal)
										if sw.Clicked() {
											p.Model = m.ID
										}
									})
								})
								if row.Clicked() {
									p.Model = m.ID
								}
							}

							// 底部自定义模型 ID 输入
							ui.Box(c).Height(1).FillWidth().Background(pal.hairline)
							ui.Row(c).Padding(6, 12).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
								ui.Text(c, T("自定义模型 ID:", "Custom Model ID:")).FontSize(textCaption).TextColor(pal.inkSubtle)
								ui.TextInput(c, &p.Model).Placeholder(T("如 deepseek-chat, gpt-4o, claude-3-5-sonnet...", "e.g. gpt-4o")).Grow(1)
							})
						})
					})
				})
			})

			// 底部操作栏
			ui.Row(c).Height(48).Padding(0, 20).AlignItems(ui.Center).FillWidth().
				BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
				if isMagpie(p.BaseURL) {
					statusCol := pal.inkTertiary
					statusText := T("网关离线", "Gateway Offline")
					if a.magpieOnline {
						statusCol = t.Success
						statusText = T("本地网关在线", "Gateway Online")
					}
					ui.Row(c).AlignItems(ui.Center).Gap(5).Padding(0, 8).Height(24).Radius(radiusPill).
						Background(pal.surface2).Border(1, pal.hairline).Children(func() {
						ui.Box(c).Size(6, 6).Radius(radiusPill).Background(statusCol)
						ui.Text(c, statusText).FontSize(textCaption).TextColor(statusCol)
					})
				}

				ui.Spacer(c)

				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					// 刷新按钮
					refreshBtn := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(radiusSM).
						Background(pal.surface2).Border(1, pal.hairlineStrong).Cursor(ui.CursorPointer).
						Label(T("刷新", "Refresh"))
					if refreshBtn.Hovered() {
						refreshBtn.Background(pal.surface3)
					}
					fadeColors(refreshBtn)
					refreshBtn.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
							drawIcon(c, iconRefresh, 12, pal.inkSubtle)
							ui.Text(c, T("刷新", "Refresh")).FontSize(textCaption).FontWeight(weightMedium).TextColor(pal.ink)
						})
					})
					if refreshBtn.Clicked() {
						a.probeMagpie()
					}

					// 删除供应商按钮。
					//
					// 「设为当前生效」在改版时删掉了：启用/禁用由详情页头部
					// 那个开关独管，这里只剩删除，别让同一种操作有两个入口。
					if len(a.editCfg.Providers) > 1 {
						delBtn := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(radiusSM).
							Background(ui.Transparent).Border(1, pal.hairlineStrong).Cursor(ui.CursorPointer).
							Label(T("删除供应商", "Delete Provider"))
						if delBtn.Hovered() {
							delBtn.Background(t.Danger.Alpha(0.12)).Border(1, t.Danger.Alpha(0.3))
						}
						fadeColors(delBtn)
						delBtn.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
								drawIcon(c, iconTrash, 12, t.Danger)
								ui.Text(c, T("删除", "Delete")).FontSize(textCaption).FontWeight(weightMedium).TextColor(t.Danger)
							})
						})
						if delBtn.Clicked() {
							a.removeProvider(a.editIdx)
						}
					}
				})
			})
		})
	})
}

// promptsSettingsPane 呈现 AI 角色与 Prompt 提示词定制面板。
func (a *app) promptsSettingsPane(c *ui.Context, pal palette) {
	t := c.Theme()
	ui.Scroll(c).FillWidth().FillHeight().Padding(20, 24).Children(func() {
		ui.Column(c).Gap(18).FillWidth().Children(func() {
			// 顶部提示与重置全部
			ui.Row(c).FillWidth().Padding(10, 14).Radius(radiusMD).
				Background(pal.surface2).Border(1, pal.hairline).
				AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Box(c).Size(24, 24).Radius(radiusSM).Background(pal.surface3).
					AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					drawIcon(c, iconSliders, 13, pal.inkSubtle)
				})
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, T("自定义 AI Prompt 提示词", "Custom AI Prompt Templates")).
						FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.ink)
					ui.Text(c, T("定制智能提交与新建分支的 Prompt 提示词。支持一键恢复官方预设。",
						"Customize prompt templates for smart commit and branch naming. Supports restoring default presets.")).
						FontSize(textCaption).TextColor(pal.inkSubtle)
				})
				resetAllBtn := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(radiusSM).
					Background(pal.surface2).Border(1, pal.hairlineStrong).Cursor(ui.CursorPointer).
					Label(T("恢复全部预设", "Reset All to Defaults"))
				if resetAllBtn.Hovered() {
					resetAllBtn.Background(pal.surface3)
				}
				fadeColors(resetAllBtn)
				resetAllBtn.Children(func() {
					ui.Text(c, T("恢复全部预设", "Reset All Defaults")).FontSize(textCaption).FontWeight(weightMedium).TextColor(pal.inkMuted)
				})
				if resetAllBtn.Clicked() {
					a.editCommitPrompt = explain.CommitSystemPrompt
					a.editBranchPrompt = explain.BranchSystemPrompt
				}
			})

			// 1. Git Commit 提示词
			promptSection(c, pal, t,
				T("Git Commit 智能提交提示词", "Git Commit Message Prompt"),
				T("在改动面板中点击「✨ AI 智能提交」时使用的系统提示词与 Conventional Commits 规范",
					"System prompt used when generating Conventional Commit messages from staged/unstaged changes"),
				&a.editCommitPrompt,
				explain.CommitSystemPrompt,
			)

			// 2. Git Branch 提示词
			promptSection(c, pal, t,
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
	ui.Column(c).Gap(6).FillWidth().Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(1).Grow(1).Children(func() {
				ui.Text(c, title).FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.ink)
				ui.Text(c, desc).FontSize(textCaption).TextColor(pal.inkSubtle)
			})
			ui.Spacer(c)
			resetBtn := ui.ButtonBase(c).Height(24).Padding(0, 8).Radius(radiusSM).
				Background(pal.surface2).Border(1, pal.hairlineStrong).Cursor(ui.CursorPointer).
				Label(T("恢复预设", "Restore Preset"))
			if resetBtn.Hovered() {
				resetBtn.Background(pal.surface3)
			}
			fadeColors(resetBtn)
			resetBtn.Children(func() {
				ui.Text(c, T("恢复预设", "Restore Default")).FontSize(11).FontWeight(weightMedium).TextColor(pal.inkSubtle)
			})
			if resetBtn.Clicked() {
				*value = preset
			}
		})

		ui.Column(c).Radius(radiusMD).Background(pal.surface2).Border(1, pal.hairline).FillWidth().Children(func() {
			ui.TextArea(c, value).FillWidth().MinHeight(100).MaxHeight(160).
				Radius(radiusSM).Placeholder(preset)
			ui.Box(c).Height(1).FillWidth().Background(pal.hairline)
			ui.Row(c).Padding(4, 10).AlignItems(ui.Center).FillWidth().Children(func() {
				ui.Text(c, Tf("%d 字符", "%d chars", len(*value))).FontSize(10).TextColor(pal.inkTertiary)
				ui.Spacer(c)
				isPreset := *value == preset || strings.TrimSpace(*value) == ""
				tag := T("自定义", "Custom")
				tagCol := pal.inkMuted
				if isPreset {
					tag = T("预设", "Default")
					tagCol = t.Success
				}
				ui.Text(c, tag).FontSize(10).TextColor(tagCol)
			})
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
	bg := ui.Transparent
	border := ui.Transparent
	if selected {
		bg = pal.surface2
		border = pal.hairlineStrong
	} else if rowBtn.Hovered() {
		bg = pal.surface2.Alpha(0.5)
	}
	rowBtn.Background(bg).Border(1, border)
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
			ui.Box(c).Size(20, 20).Radius(radiusXS).Background(pal.surface3).
				Border(1, pal.hairline).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				drawIcon(c, iconSparkles, 11, pal.inkMuted)
			})
			txtCol := pal.inkMuted
			if selected {
				txtCol = pal.ink
			}
			ui.Text(c, label).FontSize(textCaption).FontWeight(weightMedium).
				TextColor(txtCol).SingleLine()
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

// secretHint 在列表里提示有没有填 key —— 不显示 key 本身。
func secretHint(p llm.Provider) string {
	if p.APIKey == "" {
		return T("未配置密钥", "No API Key")
	}
	return T("已配置密钥", "API Key Set")
}
