package main

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
	"time"

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

	// 2. 从「添加供应商」菜单里加 DeepSeek
	if err := tt.Click("添加供应商"); err != nil {
		t.Fatalf("点添加供应商: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("DeepSeek"); err != nil {
		t.Fatalf("菜单里选 DeepSeek: %v", err)
	}
	tt.Frame()

	if len(a.editCfg.Providers) != 2 {
		t.Fatalf("供应商数量应为 2，实际为 %d", len(a.editCfg.Providers))
	}
	// 添加预设只选中编辑，不悄悄切成当前生效
	if a.editCfg.Active != 0 {
		t.Fatalf("添加预设不应切换生效，当前 active=%d", a.editCfg.Active)
	}
	if a.editIdx != 1 {
		t.Fatalf("添加后应聚焦新项，当前 editIdx=%d", a.editIdx)
	}
	if a.editCfg.Providers[1].BaseURL != "https://api.deepseek.com" {
		t.Errorf("DeepSeek BaseURL 错误: %s", a.editCfg.Providers[1].BaseURL)
	}

	// 3. 填入 API key，在详情页开关上启用 DeepSeek，然后保存
	a.editCfg.Providers[1].APIKey = "sk-deepseek-test"
	a.toggleEditActive() // 详情页头部那个开关绑定的就是这个行为
	if a.editCfg.Active != 1 {
		t.Fatalf("开关点上后 DeepSeek 应为当前生效，当前 active=%d", a.editCfg.Active)
	}
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
	if a.editCfg.Active != 1 {
		t.Errorf("点击列表项只应选中编辑、不应切换生效，当前 active=%d", a.editCfg.Active)
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

	// 从底部「添加供应商」菜单里把 Magpie 加进来（预设不再摆在左栏里：
	// 点一下就跳到已添加那段上面去，看着像「点一下排序自己变了」）
	if err := tt.Click("添加供应商"); err != nil {
		t.Fatalf("点添加供应商: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Magpie"); err != nil {
		t.Fatalf("菜单里选 Magpie: %v", err)
	}
	tt.Frame()

	// 添加只选中它，当前生效不变（这里添加前已有 Anthropic、DeepSeek
	// 两家 —— 上一步的删除走的是取消，没落盘），Magpie 落在第 len-1 位。
	if want := len(a.editCfg.Providers) - 1; a.editIdx != want {
		t.Fatalf("点预设应聚焦新加的 Magpie（%d），当前 editIdx=%d", want, a.editIdx)
	}
	if a.editCfg.Active != 1 {
		t.Fatalf("点预设不应切换生效，当前 active=%d", a.editCfg.Active)
	}
	a.toggleEditActive() // 用户自己在详情页开关上启用它
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

	// 打开「添加供应商」菜单：已经在列表里的 Anthropic 灰着，选不动
	openAdd := func() {
		t.Helper()
		if err := tt.Click("添加供应商"); err != nil {
			t.Fatalf("点添加供应商: %v", err)
		}
		tt.Frame()
	}
	openAdd()
	if err := tt.ChooseMenuItem("Anthropic"); err == nil {
		t.Error("已添加的 Anthropic 在菜单里应该是灰的")
	}
	tt.CloseMenu()
	tt.Frame()
	if len(a.editCfg.Providers) != 1 {
		t.Fatalf("再点一次 Anthropic 不该增加供应商，数量应为 1，实际为 %d", len(a.editCfg.Providers))
	}

	// 从菜单里添加 DeepSeek
	openAdd()
	if err := tt.ChooseMenuItem("DeepSeek"); err != nil {
		t.Fatalf("菜单里选 DeepSeek: %v", err)
	}
	tt.Frame()
	if len(a.editCfg.Providers) != 2 {
		t.Fatalf("添加 DeepSeek 后应有 2 个供应商，实际为 %d", len(a.editCfg.Providers))
	}
	if a.editIdx != 1 {
		t.Errorf("新加的那家应该被选中，当前 editIdx=%d", a.editIdx)
	}

	// 再加一次：菜单里它已经灰了，进不来
	openAdd()
	if err := tt.ChooseMenuItem("DeepSeek"); err == nil {
		t.Error("已添加的 DeepSeek 在菜单里应该是灰的")
	}
	tt.CloseMenu()
	tt.Frame()
	if len(a.editCfg.Providers) != 2 {
		t.Fatalf("重复添加 DeepSeek 不应增加供应商，数量应仍为 2，实际为 %d", len(a.editCfg.Providers))
	}

	// 左栏只画已经加进来的：没加的预设连行都没有（以前它们接在下面，
	// 点一下会跳进上面那段，看着像排序自己变了）
	for _, name := range []string{"Ollama（本地）", "OpenRouter", "智谱 GLM"} {
		if tt.HasText(name) {
			t.Errorf("未添加的预设 %q 不该出现在左栏里", name)
		}
	}
	for _, name := range []string{"Anthropic", "DeepSeek"} {
		if !tt.HasText(name) {
			t.Errorf("已添加的 %q 应该还在左栏里", name)
		}
	}

	// 取消关闭
	_ = tt.Click("取消")
	tt.Frame()
}

// TestProviderDragReorder 拖行首的手柄能把供应商换位置，选中与生效跟着走。
func TestProviderDragReorder(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{
		Providers: []llm.Provider{
			{Name: "AAA", Type: "openai", BaseURL: "https://a.example.com", Model: "m1"},
			{Name: "BBB", Type: "openai", BaseURL: "https://b.example.com", Model: "m2"},
			{Name: "CCC", Type: "openai", BaseURL: "https://c.example.com", Model: "m3"},
		},
		Active: 1,
	})
	a.attachStore(s)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置: %v", err)
	}
	tt.Frame()

	// Find 拿到的是整行（行按钮先登记 Label，名字文本在它里面）：
	// 行首 8px 内边距 + 11px 手柄，中心大约在行内 13px 处
	ra, _ := tt.Find("AAA")
	rb, _ := tt.Find("BBB")
	if ra.H == 0 || rb.Y <= ra.Y {
		t.Fatalf("三行没摆出来: AAA=%+v BBB=%+v", ra, rb)
	}
	gripX := rb.X + 13
	tt.Move(gripX, rb.Y+rb.H/2)
	tt.Frame()
	if c := tt.Cursor(); c != ui.CursorGrab {
		t.Errorf("悬停在拖动手柄上光标应该是 CursorGrab，实际是 %v", c)
	}

	// 把 BBB 拖到 AAA 的上半条 = 插到 AAA 前面
	tt.Press(gripX, rb.Y+rb.H/2)
	tt.Move(gripX, rb.Y+rb.H/2-10)
	tt.Frame()
	tt.Move(gripX, ra.Y+4)
	tt.Frame()
	tt.Release(gripX, ra.Y+4)
	tt.Frame()
	tt.Frame()

	if got := providerNames(a.editCfg.Providers); got != "BBB, AAA, CCC" {
		t.Fatalf("拖过之后顺序应为 BBB, AAA, CCC，实际为 %s", got)
	}
	// 被拖的那家原来在下标 1，也是当时生效的那家；两处下标都得跟着走
	if a.editIdx != 0 {
		t.Errorf("选中应该跟着被拖的那家走，editIdx=%d", a.editIdx)
	}
	if a.editCfg.Active != 0 {
		t.Errorf("生效的也应该跟着被拖的那家走，active=%d", a.editCfg.Active)
	}

	// 再把它拖回末尾：落在最后一行（CCC）的下半条
	rb, _ = tt.Find("BBB")
	rc, _ := tt.Find("CCC")
	tt.Press(rb.X+13, rb.Y+rb.H/2)
	tt.Move(rb.X+13, rb.Y+rb.H/2+10)
	tt.Frame()
	tt.Move(rb.X+13, rc.Y+rc.H-4)
	tt.Frame()
	tt.Release(rb.X+13, rc.Y+rc.H-4)
	tt.Frame()
	tt.Frame()

	if got := providerNames(a.editCfg.Providers); got != "AAA, CCC, BBB" {
		t.Fatalf("拖到末尾后顺序应为 AAA, CCC, BBB，实际为 %s", got)
	}
	if a.editIdx != 2 || a.editCfg.Active != 2 {
		t.Errorf("拖到末尾后选中与生效都应落在下标 2，实际 editIdx=%d active=%d", a.editIdx, a.editCfg.Active)
	}

	// 点行里的名字仍然是选中，不会因为多了个手柄就点不动
	if err := tt.Click("AAA"); err != nil {
		t.Fatalf("点行名字: %v", err)
	}
	tt.Frame()
	if a.editIdx != 0 {
		t.Errorf("点名字应该选中那一行，editIdx=%d", a.editIdx)
	}
	// 点手柄也一样选中（手柄只吃不拖的那一下）
	ra, _ = tt.Find("AAA")
	tt.ClickAt(ra.X+13, ra.Y+ra.H/2)
	tt.Frame()
	if a.editIdx != 0 {
		t.Errorf("点手柄也应该选中那一行，editIdx=%d", a.editIdx)
	}
}

// providerNames 把供应商名字连起来，方便断言顺序。
func providerNames(ps []llm.Provider) string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return strings.Join(out, ", ")
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
	if !tt.HasText(appVersionLabel()) {
		t.Errorf("关于面板应渲染版本号 %s", appVersionLabel())
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

// TestToggleEditActive：详情页开关可开可关 —— 关掉当前生效的供应商后
// 处于「没有激活供应商」的状态，store.Provider() 回落环境变量。
func TestToggleEditActive(t *testing.T) {
	a := newApp()
	s := store.Open(t.TempDir())
	cfg := store.Config{
		Providers: []llm.Provider{
			{Name: "A", Type: "openai", Model: "m-a"},
			{Name: "B", Type: "openai", Model: "m-b"},
		},
		Active: 0,
	}
	s.SetConfig(cfg)
	a.attachStore(s)
	// 模拟 openSettings：编辑的是配置的副本。
	a.editCfg = s.Config()
	a.editIdx = 1

	// 选中 B，B 不是当前生效 → 打开它。
	a.toggleEditActive()
	if a.editCfg.Active != 1 {
		t.Fatalf("开关应由 0 切到 1，实际 %d", a.editCfg.Active)
	}

	// B 已经是当前生效了 → 再点开关把它关掉。
	a.toggleEditActive()
	if a.editCfg.Active != -1 {
		t.Fatalf("再点开关应关掉当前生效（-1），实际 %d", a.editCfg.Active)
	}

	// 从「没有激活」再点一次 → 又把 B 打开。
	a.toggleEditActive()
	if a.editCfg.Active != 1 {
		t.Fatalf("空态再点开关应重新打开 B，实际 %d", a.editCfg.Active)
	}

	// store 面对 Active=-1 必须回落 nil（界面靠它走环境变量凭证链）。
	cfg.Active = -1
	s.SetConfig(cfg)
	if p := s.Provider(); p != nil {
		t.Fatalf("Active=-1 时 Provider() 应回落 nil，实际 %v", p)
	}
}

func TestSettingsDialogSavesProviders(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("点模型设置：%v", err)
	}
	tt.Frame()
	if !a.settingsOn {
		t.Fatal("设置对话框没打开")
	}
	if !tt.HasText("模型设置") {
		t.Errorf("对话框内容没画出来：%q", tt.Texts())
	}

	// 加一个 DeepSeek，改掉 API key，保存
	if err := tt.Click("添加供应商"); err != nil {
		t.Fatalf("点添加供应商：%v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("DeepSeek"); err != nil {
		t.Fatalf("菜单里选 DeepSeek：%v", err)
	}
	tt.Frame()
	// 加预设只选中编辑，不悄悄切成当前生效 —— 启用是详情页那个开关的事
	if got := a.editCfg.Providers[a.editIdx].BaseURL; got != "https://api.deepseek.com" {
		t.Errorf("预设没填进 Base URL：%+v", a.editCfg.Providers[a.editIdx])
	}
	if a.editCfg.Active == a.editIdx {
		t.Errorf("加预设不该顺手把它设为当前生效，active=%d", a.editCfg.Active)
	}
	a.editCfg.Providers[a.editIdx].APIKey = "sk-test"
	a.toggleEditActive() // 详情页头部那个开关绑定的就是这个行为
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存：%v", err)
	}
	tt.Frame()

	if a.settingsOn {
		t.Error("保存之后对话框应该关掉")
	}
	p := a.store.Provider()
	if p == nil || p.APIKey != "sk-test" {
		t.Errorf("设置没写进 store：%+v", p)
	}
}

// 视图默认左右对照（split）模式。
func TestSplitSettingDefault(t *testing.T) {
	a := richApp(t)
	settle(t, a)
	if !a.split {
		t.Error("默认应该开启 split 模式")
	}
}

// 设置里的「更新」页签：版本、上次检查时间、自动检查/下载开关与手动检查入口都要接上。
func TestUpdatesSettingsPane(t *testing.T) {
	origLoad, origSave, origCheck := loadUpdatePrefs, saveUpdatePrefs, checkForUpdatesNow
	defer func() {
		loadUpdatePrefs, saveUpdatePrefs, checkForUpdatesNow = origLoad, origSave, origCheck
	}()

	checkedAt := time.Date(2026, 10, 14, 9, 30, 0, 0, time.Local)
	prefs := updatePrefs{available: true, automaticChecks: true, automaticDownloads: false, lastCheck: checkedAt}
	loadUpdatePrefs = func() updatePrefs { return prefs }

	var saved [][2]bool
	saveUpdatePrefs = func(automaticChecks, automaticDownloads bool) {
		saved = append(saved, [2]bool{automaticChecks, automaticDownloads})
	}
	checkCalls := 0
	checkForUpdatesNow = func() { checkCalls++ }

	a := richApp(t)
	settle(t, a)
	a.setLocale("zh")

	s := store.Open(t.TempDir())
	a.attachStore(s)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)

	if err := tt.Click("模型设置…"); err != nil {
		t.Fatalf("打开设置面板失败: %v", err)
	}
	tt.Frame()
	if err := tt.Click(T("更新", "Updates")); err != nil {
		t.Fatalf("点击「更新」分栏失败: %v", err)
	}
	tt.Frame()

	if a.settingsTab != settingsTabUpdates {
		t.Fatalf("设置页签应切到更新页，实际 %d", a.settingsTab)
	}
	if !tt.HasText(appVersionLabel()) {
		t.Errorf("更新页应显示当前版本 %s", appVersionLabel())
	}
	if !tt.HasText(lastCheckLabel(checkedAt)) {
		t.Errorf("更新页应显示上次检查时间 %s", lastCheckLabel(checkedAt))
	}
	if !tt.HasText("自动检查更新") || !tt.HasText("自动下载更新") {
		t.Error("更新页应显示两个自动更新开关")
	}

	if err := tt.Click("检查更新…"); err != nil {
		t.Fatalf("点击「检查更新…」失败: %v", err)
	}
	tt.Frame()
	if checkCalls != 1 {
		t.Errorf("点击「检查更新…」应触发一次检查，实际 %d 次", checkCalls)
	}

	// 开关回写：变化时按当前另一个开关的值一起写回。
	saved = nil
	applyAutomaticChecks(prefs, false)
	applyAutomaticDownloads(prefs, true)
	if len(saved) != 2 || saved[0] != [2]bool{false, false} || saved[1] != [2]bool{true, true} {
		t.Fatalf("开关写回不符合预期: %v", saved)
	}

	// 值没变、或当前构建不支持自动更新时不写回。
	saved = nil
	applyAutomaticChecks(prefs, true)
	applyAutomaticDownloads(updatePrefs{available: false, automaticChecks: false}, true)
	if len(saved) != 0 {
		t.Fatalf("不该发生写回: %v", saved)
	}

	// 未启用更新的构建：只说明不可用，不给开关。
	prefs = updatePrefs{}
	tt.Frame()
	if !tt.HasText("当前构建未启用自动更新（开发构建，或由包管理器安装）。") {
		t.Error("未启用更新的构建应显示说明")
	}
}

// 默认的 loadUpdatePrefs 在没注册 updater 插件的测试进程里也必须安全返回。
func TestUpdatePrefsWithoutPlugin(t *testing.T) {
	prefs := loadUpdatePrefs()
	if prefs.available {
		t.Error("未打包的测试进程不应报告自动更新可用")
	}
	if prefs.automaticChecks || prefs.automaticDownloads || !prefs.lastCheck.IsZero() {
		t.Error("不可用时其余字段应为零值")
	}
	if got := lastCheckLabel(time.Time{}); got == "" {
		t.Error("未检查时应给出占位文案")
	}
}
