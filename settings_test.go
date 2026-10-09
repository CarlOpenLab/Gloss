package main

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/llm"
	"github.com/CarlOpenLab/Gloss/internal/store"
)

func TestSettingsModalInteractions(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{
		Providers: []llm.Provider{
			{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel},
		},
		Active: 0,
	})
	a.attachStore(s)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// 1. 打开模型设置
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置: %v", err)
	}
	tt.Frame()
	if !a.settingsOn {
		t.Fatal("模型设置未打开")
	}
	if !tt.HasText("模型设置") {
		t.Fatal("未渲染出模型设置标题")
	}

	// 2. 添加 DeepSeek 预设
	if err := tt.Click("DeepSeek"); err != nil {
		t.Fatalf("点快捷添加 DeepSeek: %v", err)
	}
	tt.Frame()

	if len(a.editCfg.Providers) != 2 {
		t.Fatalf("供应商数量应为 2，实际为 %d", len(a.editCfg.Providers))
	}
	if a.editCfg.Active != 1 {
		t.Fatalf("添加后应自动激活新项，当前 active=%d", a.editCfg.Active)
	}
	if a.editIdx != 1 {
		t.Fatalf("添加后应自动聚焦新项，当前 editIdx=%d", a.editIdx)
	}
	if a.editCfg.Providers[1].BaseURL != "https://api.deepseek.com" {
		t.Errorf("DeepSeek BaseURL 错误: %s", a.editCfg.Providers[1].BaseURL)
	}

	// 3. 填入 API key 并保存
	a.editCfg.Providers[1].APIKey = "sk-deepseek-test"
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存: %v", err)
	}
	tt.Frame()

	if a.settingsOn {
		t.Fatal("保存后对话框应该关闭")
	}

	saved := a.store.Provider()
	if saved == nil || saved.APIKey != "sk-deepseek-test" {
		t.Fatalf("设置未正确持久化: %+v", saved)
	}

	// 4. 再次打开设置，测试切换与删除
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("再次点模型设置: %v", err)
	}
	tt.Frame()

	// 点击左侧 Anthropic 切换选中
	if err := tt.Click("Anthropic"); err != nil {
		t.Fatalf("点 Anthropic 列表项: %v", err)
	}
	tt.Frame()
	if a.editIdx != 0 {
		t.Errorf("切换选中小标应为 0，实际为 %d", a.editIdx)
	}

	// 点击删除按钮删除一项
	if err := tt.Click("删除"); err != nil {
		t.Fatalf("点删除按钮: %v", err)
	}
	tt.Frame()
	if len(a.editCfg.Providers) != 1 {
		t.Errorf("删除后应剩下 1 个供应商，实际为 %d", len(a.editCfg.Providers))
	}

	// 取消关闭
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("点取消: %v", err)
	}
	tt.Frame()
	if a.settingsOn {
		t.Fatal("取消后对话框应该关闭")
	}

	// 5. 测试 API Key 隐私明密文切换与 Magpie 接入
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("第三次点模型设置: %v", err)
	}
	tt.Frame()

	// 初始状态下密钥应默认隐藏
	if a.showAPIKey {
		t.Error("打开设置时 API Key 应该默认密文隐藏")
	}

	// 点击眼睛切换明文
	if err := tt.Click("切换密钥显示"); err != nil {
		t.Fatalf("点切换密钥显示: %v", err)
	}
	tt.Frame()
	if !a.showAPIKey {
		t.Error("点击切换后 API Key 应变为明文显示")
	}

	// 测试连接 Magpie 本地网关
	a.magpieOnline = true
	a.magpieModels = []string{"opencode-go/deepseek-v4.1-flash", "codex-2/gpt-6-sol"}
	tt.Frame()

	// 点击「Magpie（本地网关）」
	if err := tt.Click("Magpie（本地网关）"); err != nil {
		t.Fatalf("点 Magpie 预设: %v", err)
	}
	tt.Frame()

	current := a.editCfg.Providers[a.editCfg.Active]
	if !isMagpie(current.BaseURL) {
		t.Errorf("接入后当前供应商 BaseURL 应指向 Magpie，实际为: %s", current.BaseURL)
	}
	if current.Model != "opencode-go/deepseek-v4.1-flash" {
		t.Errorf("接入后模型应首选 deepseek，实际为: %s", current.Model)
	}

	// 点击芯片切换模型（带 provider 命名空间）
	if err := tt.Click("codex-2/gpt-6-sol"); err != nil {
		t.Fatalf("点击选择 codex-2/gpt-6-sol 芯片: %v", err)
	}
	tt.Frame()
	if a.editCfg.Providers[a.editCfg.Active].Model != "codex-2/gpt-6-sol" {
		t.Errorf("点击模型芯片应切换为 codex-2/gpt-6-sol，实际为: %s", a.editCfg.Providers[a.editCfg.Active].Model)
	}

	// 取消关闭
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("点取消: %v", err)
	}
	tt.Frame()
	if a.settingsOn {
		t.Fatal("取消后对话框应该关闭")
	}
}

func TestPresetDeduplication(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{
		Providers: []llm.Provider{
			{Name: "Anthropic", Type: "anthropic", Model: llm.DefaultModel},
		},
		Active: 0,
	})
	a.attachStore(s)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// 打开模型设置
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置: %v", err)
	}
	tt.Frame()

	// 初始有 1 个 (Anthropic)
	if len(a.editCfg.Providers) != 1 {
		t.Fatalf("初始供应商数量应为 1，实际为 %d", len(a.editCfg.Providers))
	}

	// 再次点击预设中的 Anthropic
	if err := tt.Click("Anthropic"); err != nil {
		t.Fatalf("点 Anthropic 预设: %v", err)
	}
	tt.Frame()

	// 供应商数量应仍然为 1（不会被重复添加）
	if len(a.editCfg.Providers) != 1 {
		t.Fatalf("重复点击 Anthropic 预设不应增加供应商，数量应为 1，实际为 %d", len(a.editCfg.Providers))
	}
	if a.editIdx != 0 {
		t.Errorf("重复点击应聚焦至已有项，editIdx=%d", a.editIdx)
	}

	// 添加 DeepSeek 预设
	if err := tt.Click("DeepSeek"); err != nil {
		t.Fatalf("点 DeepSeek 预设: %v", err)
	}
	tt.Frame()
	if len(a.editCfg.Providers) != 2 {
		t.Fatalf("添加 DeepSeek 后应有 2 个供应商，实际为 %d", len(a.editCfg.Providers))
	}

	// 再次点击 DeepSeek 预设
	if err := tt.Click("DeepSeek"); err != nil {
		t.Fatalf("再次点 DeepSeek 预设: %v", err)
	}
	tt.Frame()

	// 供应商数量依然为 2，且聚焦在第 1 项（DeepSeek）
	if len(a.editCfg.Providers) != 2 {
		t.Fatalf("重复点击 DeepSeek 预设不应重复添加，供应商数量应仍为 2，实际为 %d", len(a.editCfg.Providers))
	}
	if a.editIdx != 1 {
		t.Errorf("重复点击应聚焦至已有的 DeepSeek，当前 editIdx=%d", a.editIdx)
	}

	// 检查界面上是否存在已添加标记文本 "已添加"
	if !tt.HasText("已添加") {
		t.Errorf("界面上应对已添加的预设显示「已添加」状态徽章")
	}

	// 取消关闭
	_ = tt.Click("取消")
	tt.Frame()
}

func TestThemeAndLocaleSettings(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	a.attachStore(s)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// 1. 测试外观切换逻辑
	a.setThemeMode("light")
	if a.themeMode != "light" {
		t.Errorf("themeMode 应为 light, 实际为 %s", a.themeMode)
	}
	if a.isDark(nil) {
		t.Error("浅色模式下 isDark 应该为 false")
	}

	a.setThemeMode("dark")
	if a.themeMode != "dark" {
		t.Errorf("themeMode 应为 dark, 实际为 %s", a.themeMode)
	}
	if !a.isDark(nil) {
		t.Error("深色模式下 isDark 应该为 true")
	}

	a.setThemeMode("system")
	if a.themeMode != "system" {
		t.Errorf("themeMode 应为 system, 实际为 %s", a.themeMode)
	}

	// 2. 测试语言切换逻辑
	a.setLocale("en")
	if !IsEnglish() {
		t.Error("切换至 en 后 IsEnglish 应为 true")
	}
	if CurrentLocale() != LocaleEn {
		t.Errorf("CurrentLocale 应为 en, 实际为 %s", CurrentLocale())
	}
	if got := T("中文", "English"); got != "English" {
		t.Errorf("英文模式下 T() 应返回 English, 实际为 %s", got)
	}

	a.setLocale("zh")
	if IsEnglish() {
		t.Error("切换至 zh 后 IsEnglish 应为 false")
	}
	if CurrentLocale() != LocaleZh {
		t.Errorf("CurrentLocale 应为 zh, 实际为 %s", CurrentLocale())
	}
	if got := T("中文", "English"); got != "中文" {
		t.Errorf("中文模式下 T() 应返回 中文, 实际为 %s", got)
	}

	// 3. 打开设置面板并切换到通用偏好选项卡
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置: %v", err)
	}
	tt.Frame()

	if !a.settingsOn {
		t.Fatal("设置窗口应处于打开状态")
	}

	// 切换到「通用偏好」选项卡
	if err := tt.Click("通用偏好"); err != nil {
		t.Fatalf("点击「通用偏好」选项卡: %v", err)
	}
	tt.Frame()

	if a.settingsTab != 1 {
		t.Errorf("settingsTab 应为 1（通用偏好），实际为 %d", a.settingsTab)
	}

	// 验证选项卡内卡片渲染
	if !tt.HasText("外观主题") || !tt.HasText("界面语言") {
		t.Error("通用偏好页面应包含外观主题与界面语言分区")
	}

	// 点击浅色模式卡片
	if err := tt.Click("浅色模式"); err != nil {
		t.Fatalf("点击「浅色模式」卡片: %v", err)
	}
	tt.Frame()
	if a.themeMode != "light" {
		t.Errorf("点击浅色卡片后 themeMode 应为 light，实际为 %s", a.themeMode)
	}

	// 点击深色模式卡片
	if err := tt.Click("深色模式"); err != nil {
		t.Fatalf("点击「深色模式」卡片: %v", err)
	}
	tt.Frame()
	if a.themeMode != "dark" {
		t.Errorf("点击深色卡片后 themeMode 应为 dark，实际为 %s", a.themeMode)
	}

	// 点击 English 卡片
	if err := tt.Click("English"); err != nil {
		t.Fatalf("点击「English」卡片: %v", err)
	}
	tt.Frame()
	if !IsEnglish() {
		t.Error("点击 English 卡片后语言应切换为英文")
	}

	// 切换回简体中文
	if err := tt.Click("简体中文"); err != nil {
		t.Fatalf("点击「简体中文」卡片: %v", err)
	}
	tt.Frame()
	if IsEnglish() {
		t.Error("点击简体中文卡片后语言应切换为中文")
	}

	// 4. 测试应用图标切换
	if !tt.HasText("应用图标") {
		t.Error("通用偏好页面应包含「应用图标」分区")
	}
	if err := tt.Click("纯白极简"); err != nil {
		t.Fatalf("点击「纯白极简」图标卡片: %v", err)
	}
	tt.Frame()
	if a.appIcon != "white" {
		t.Errorf("点击纯白卡片后 appIcon 应为 white，实际为 %s", a.appIcon)
	}

	if err := tt.Click("纯黑极简"); err != nil {
		t.Fatalf("点击「纯黑极简」图标卡片: %v", err)
	}
	tt.Frame()
	if a.appIcon != "black" {
		t.Errorf("点击纯黑卡片后 appIcon 应为 black，实际为 %s", a.appIcon)
	}

	// 5. 测试切换至「关于 Gloss」选项卡
	if err := tt.Click("关于 Gloss"); err != nil {
		t.Fatalf("点击「关于 Gloss」侧边栏条目: %v", err)
	}
	tt.Frame()
	if a.settingsTab != settingsTabAbout {
		t.Errorf("settingsTab 应为 settingsTabAbout (%d)，实际为 %d", settingsTabAbout, a.settingsTab)
	}
	if !tt.HasText("v1.0.0") {
		t.Error("关于面板应渲染版本号 v1.0.0")
	}

	// 取消关闭
	_ = tt.Click("取消")
	tt.Frame()
	if a.settingsOn {
		t.Fatal("取消后对话框应该关闭")
	}
}

func TestAppIconFilesAreNeutralMonochrome(t *testing.T) {
	for name, data := range map[string][]byte{
		"black": iconPureBlackPNG,
		"white": iconPureWhitePNG,
	} {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		bounds := img.Bounds()
		if bounds.Dx() != 1024 || bounds.Dy() != 1024 {
			t.Errorf("%s icon size should be 1024x1024, got %dx%d", name, bounds.Dx(), bounds.Dy())
		}
		// 抽样验证 RGB 通道完全中性 (R == G == B)，确保绝无蓝色或杂色渐变
		for y := bounds.Min.Y; y < bounds.Max.Y; y += 4 {
			for x := bounds.Min.X; x < bounds.Max.X; x += 4 {
				c := img.At(x, y)
				r, g, b, a := c.RGBA()
				if a > 0 {
					if r != g || g != b {
						t.Fatalf("%s icon has non-neutral color at (%d, %d): r=%d, g=%d, b=%d", name, x, y, r, g, b)
					}
				}
			}
		}
	}
}
