package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// preset 是一个常见供应商的预设：接口类型、Base URL、模型都填好，
// 用户只需要补一个 API key。
type preset struct {
	Label string
	Type  string
	Base  string
	Model string
}

// providerPresets 覆盖国内外常用的几家，顺序按「开箱即用的可能性」排。
// Anthropic 排第一：不填 Base URL 时它走官方接口和 ANTHROPIC_API_KEY。
var providerPresets = []preset{
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
		// 一条都没有时先放一个 Anthropic 的空壳，用户不用先点「添加」
		a.editCfg.Providers = []llm.Provider{{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel}}
	}
	if a.editCfg.Active < 0 || a.editCfg.Active >= len(a.editCfg.Providers) {
		a.editCfg.Active = 0
	}
	a.settingsOn = true
}

// settingsDialog 是「模型设置」对话框的入口。
func (a *app) settingsDialog(c *ui.Context, pal palette) {
	if !a.settingsOn {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.settingsBody(c, pal) })
	if !open {
		a.settingsOn = false
	}
}

func (a *app) settingsBody(c *ui.Context, pal palette) {
	t := c.Theme()
	ui.Column(c).Width(560).Gap(12).Children(func() {
		ui.Text(c, "模型设置").Bold().FontSize(15)
		ui.Text(c, "解释代码用的是这里选中的模型。没有 API key 时，Anthropic 会读 "+
			"ANTHROPIC_API_KEY 环境变量。").
			FontSize(11).TextColor(t.TextMuted)

		ui.Column(c).Gap(4).Children(func() { a.providerList(c) })

		ui.Row(c).Gap(6).Wrap().Children(func() {
			ui.Text(c, "添加：").FontSize(11).TextColor(t.TextMuted)
			for _, p := range providerPresets {
				p := p
				if ui.Button(c, p.Label).Clicked() {
					a.addProvider(p)
				}
			}
		})

		if a.editIdx >= 0 && a.editIdx < len(a.editCfg.Providers) {
			a.providerForm(c, pal)
		} else {
			ui.Text(c, "上面还没有供应商，先添加一个。").
				FontSize(12).TextColor(t.TextMuted).PaddingY(12)
		}

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.settingsOn = false
			}
			if ui.PrimaryButton(c, "保存").Clicked() {
				a.applySettings()
			}
		})
	})
}

// providerList 是供应商列表：点一行选中，右边的 × 删掉。
func (a *app) providerList(c *ui.Context) {
	t := c.Theme()
	if len(a.editCfg.Providers) == 0 {
		return
	}
	ui.Scroll(c).MaxHeight(160).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			for i := range a.editCfg.Providers {
				p := &a.editCfg.Providers[i]
				selected := i == a.editCfg.Active

				bg := ui.Transparent
				if selected {
					bg = t.Surface
				}
				row := ui.Row(c).Padding(6, 10).Radius(t.Radius).Gap(8).
					AlignItems(ui.Center).Background(bg).Cursor(ui.CursorPointer)
				if row.Hovered() && !selected {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					if selected {
						ui.Text(c, "●").FontSize(10).TextColor(t.Accent)
					} else {
						ui.Text(c, "○").FontSize(10).TextColor(t.TextMuted)
					}
					ui.Text(c, providerLabel(*p)).FontSize(12).Grow(1).MinWidth(0).SingleLine()
					ui.Text(c, secretHint(*p)).FontSize(11).TextColor(t.TextMuted).SingleLine()
					if removeButton(c) {
						a.removeProvider(i)
					}
				})
				if row.Clicked() {
					a.editCfg.Active = i
				}
			}
		})
	})
}

// providerForm 是选中那个供应商的字段。
func (a *app) providerForm(c *ui.Context, pal palette) {
	t := c.Theme()
	p := &a.editCfg.Providers[a.editIdx]

	field := func(label string, fn func()) {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
			fn()
		})
	}

	ui.Column(c).Gap(8).Padding(10).Radius(t.Radius).
		Background(t.Surface).Border(1, pal.border).Children(func() {
		field("名称", func() {
			ui.TextInput(c, &p.Name).Placeholder("给这个供应商起个名字")
		})
		field("接口类型", func() {
			mode := 0
			if p.Type == "openai" {
				mode = 1
			}
			ui.Segmented(c, &mode, "Anthropic", "OpenAI 兼容")
			p.Type = "anthropic"
			if mode == 1 {
				p.Type = "openai"
			}
		})
		field(baseLabel(p.Type), func() {
			ph := "https://api.example.com/v1"
			if p.Type == "anthropic" {
				ph = "留空用官方接口，也可以填中转地址"
			}
			ui.TextInput(c, &p.BaseURL).Placeholder(ph)
		})
		field("API Key", func() {
			ui.TextInput(c, &p.APIKey).Placeholder("sk-…")
		})
		field(modelLabel(p.Type), func() {
			ui.TextInput(c, &p.Model).Placeholder(llm.DefaultModel)
		})
	})
}

// addProvider 按预设追加一个供应商并选中它。
func (a *app) addProvider(ps preset) {
	a.editCfg.Providers = append(a.editCfg.Providers, llm.Provider{
		Name:    ps.Label,
		Type:    ps.Type,
		BaseURL: ps.Base,
		Model:   ps.Model,
	})
	a.editCfg.Active = len(a.editCfg.Providers) - 1
}

func (a *app) removeProvider(i int) {
	if i < 0 || i >= len(a.editCfg.Providers) {
		return
	}
	a.editCfg.Providers = append(a.editCfg.Providers[:i], a.editCfg.Providers[i+1:]...)
	switch {
	case len(a.editCfg.Providers) == 0:
		a.editCfg.Active = 0
	case a.editCfg.Active > i, a.editCfg.Active >= len(a.editCfg.Providers):
		a.editCfg.Active--
	}
}

// applySettings 保存设置，改名或换模型都会让旧解释对不上，所以顺便
// 关掉当前卡片 —— 它显示的可能已经是另一个模型的话了。
func (a *app) applySettings() {
	cfg := a.store.Config()
	cfg.Providers = a.editCfg.Providers
	cfg.Active = a.editCfg.Active
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
	return "未命名的供应商"
}

// secretHint 在列表里提示有没有填 key —— 不显示 key 本身。
func secretHint(p llm.Provider) string {
	if p.APIKey == "" {
		return "未填 API Key"
	}
	return "已填 API Key"
}

func baseLabel(typ string) string {
	if typ == "anthropic" {
		return "Base URL（可留空）"
	}
	return "Base URL"
}

func modelLabel(typ string) string {
	if typ == "anthropic" {
		return "模型（留空用 " + llm.DefaultModel + "）"
	}
	return "模型"
}
