package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/explain"
)

// 测试单行代码 AI 解释打开、流式渲染及内容呈现
func TestSelectionExplainOpensAndStreams(t *testing.T) {
	srv, calls := fakeLLM(t, "这一行代码的作用是", "初始化系统配置。")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	lineStr := "cfg := initializeConfig()"
	lineNum := 10
	anchor := ui.Rect{X: 200, Y: 150, W: 60, H: 20}

	a.openSelectionExplain(fi, lineStr, lineNum, "", anchor)
	if !a.explainOpen() {
		t.Fatal("openSelectionExplain 之后卡片应该是打开的")
	}
	if !a.ex.isSelection {
		t.Fatal("卡片应该标记为 isSelection = true")
	}

	settle(t, a)
	tt.Frame()

	if *calls != 1 {
		t.Fatalf("应请求 1 次模型，实际 %d 次", *calls)
	}
	if !tt.HasText("AI 单行解释") {
		t.Errorf("卡片标题应显示 'AI 单行解释'：%q", tt.Texts())
	}
	if !tt.HasText("第 10 行: " + lineStr) {
		t.Errorf("卡片头部应包含行号与代码片段：%q", tt.Texts())
	}
	if !tt.HasText("这一行代码的作用是初始化系统配置。") {
		t.Errorf("解释内容未完整显示：%q", tt.Texts())
	}
}

// 测试单行解释的缓存命中与持久化
func TestSelectionExplainCacheHit(t *testing.T) {
	srv, calls := fakeLLM(t, "已缓存的单行解释")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	lineStr := "return nil"
	lineNum := 42
	anchor := ui.Rect{X: 100, Y: 200, W: 50, H: 20}

	// 第一次调用：请求模型并写入缓存
	a.openSelectionExplain(fi, lineStr, lineNum, "", anchor)
	settle(t, a)
	tt.Frame()

	if *calls != 1 {
		t.Fatalf("第一次应调用 1 次模型，实际 %d 次", *calls)
	}

	// 检查 store
	expectedKey := explain.SelectionCacheKey(a.files[fi].Path(), lineStr, "", a.modelName)
	cachedText, ok := a.store.Lookup(expectedKey)
	if !ok || cachedText != "已缓存的单行解释" {
		t.Fatalf("解释未正确存入 store: got %q, ok %v", cachedText, ok)
	}

	// 关闭卡片
	a.closeExplain()
	if a.explainOpen() {
		t.Fatal("closeExplain 后卡片应已关闭")
	}

	// 第二次调用：命中缓存，不再请求模型
	a.openSelectionExplain(fi, lineStr, lineNum, "", anchor)
	settle(t, a)
	tt.Frame()

	if *calls != 1 {
		t.Errorf("命中缓存时不应再次调用模型，实际调用 %d 次", *calls)
	}
	if !a.ex.cached {
		t.Error("第二次打开应该标记为命中缓存")
	}
	if a.ex.loading {
		t.Error("命中缓存时不应处于 loading 状态")
	}
	if !tt.HasText("已缓存的单行解释") {
		t.Errorf("卡片未显示缓存的解释：%q", tt.Texts())
	}
}

// 测试带用户追问 / 指定重点的解释流程
func TestSelectionExplainWithCustomQuestion(t *testing.T) {
	srv, calls := fakeLLM(t, "针对并发安全问题的解答：此处使用了互斥锁保护。")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	lineStr := "mu.Lock()"
	lineNum := 88
	question := "这段代码是否存在死锁风险？"
	anchor := ui.Rect{X: 150, Y: 180, W: 50, H: 20}

	a.openSelectionExplain(fi, lineStr, lineNum, question, anchor)
	settle(t, a)
	tt.Frame()

	if *calls != 1 {
		t.Fatalf("应请求 1 次模型，实际 %d 次", *calls)
	}
	if !tt.HasText("追问: " + question) {
		t.Errorf("卡片头部应包含追问信息：%q", tt.Texts())
	}
	if !tt.HasText("针对并发安全问题的解答：此处使用了互斥锁保护。") {
		t.Errorf("未包含回答内容：%q", tt.Texts())
	}

	// 追问与空追问的缓存 key 应该互不相同
	emptyQKey := explain.SelectionCacheKey(a.files[fi].Path(), lineStr, "", a.modelName)
	customQKey := explain.SelectionCacheKey(a.files[fi].Path(), lineStr, question, a.modelName)
	if emptyQKey == customQKey {
		t.Errorf("不同追问的缓存 key 不应相同")
	}
}

// 测试 Esc 键关闭单行解释卡片
func TestSelectionExplainEscapeCloses(t *testing.T) {
	srv, _ := fakeLLM(t, "单行代码解释")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	a.openSelectionExplain(fi, "x := 1", 5, "", ui.Rect{X: 100, Y: 100, W: 50, H: 20})
	settle(t, a)
	tt.Frame()

	if !a.explainOpen() {
		t.Fatal("卡片未打开")
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	if a.explainOpen() {
		t.Error("按 Esc 后卡片应关闭")
	}
}

// 测试 toggleSelectionExplain 切换打开/关闭卡片
func TestSelectionExplainToggle(t *testing.T) {
	srv, _ := fakeLLM(t, "切换测试")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	lineStr := "flag = true"
	lineNum := 12
	anchor := ui.Rect{X: 100, Y: 100, W: 50, H: 20}

	// 第一次 toggle：打开
	a.toggleSelectionExplain(fi, lineStr, lineNum, anchor)
	settle(t, a)
	tt.Frame()
	if !a.explainOpen() {
		t.Fatal("第一次 toggle 应打开卡片")
	}

	// 再次 toggle 同一行：收起
	a.toggleSelectionExplain(fi, lineStr, lineNum, anchor)
	tt.Frame()
	if a.explainOpen() {
		t.Fatal("再次 toggle 同一行应收起卡片")
	}
}

// 测试选区解释卡片中的追问输入栏提交追问
func TestSelectionExplainPromptBarFollowUp(t *testing.T) {
	srv, calls := fakeLLM(t, "回答首个问题", "回答第二个追问")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	lineStr := "go worker()"
	lineNum := 30
	anchor := ui.Rect{X: 100, Y: 100, W: 50, H: 20}

	a.openSelectionExplain(fi, lineStr, lineNum, "", anchor)
	settle(t, a)
	tt.Frame()

	if *calls != 1 {
		t.Fatalf("第一次调用模型次数不符合：%d", *calls)
	}

	// 模拟在卡片输入栏输入追问并点击提问
	a.ex.promptInput = "worker 中未捕获 panic 会怎样？"
	if err := tt.Click("提问"); err != nil {
		t.Fatalf("点击提问按钮失败：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if *calls != 2 {
		t.Fatalf("追问后应发起第 2 次模型请求，实际 %d 次", *calls)
	}
	if a.ex.question != "worker 中未捕获 panic 会怎样？" {
		t.Errorf("追问内容不符合：%q", a.ex.question)
	}
}

// 测试选区解释中的重试逻辑
func TestSelectionExplainRetry(t *testing.T) {
	srv, calls := fakeLLM(t, "重试成功后的解释")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	lineStr := "err := doSomething()"
	lineNum := 15
	anchor := ui.Rect{X: 100, Y: 100, W: 50, H: 20}

	a.openSelectionExplain(fi, lineStr, lineNum, "", anchor)
	settle(t, a)
	tt.Frame()

	// 模拟失败状态
	a.ex.fail.Title = "请求超时"
	a.ex.text = ""
	tt.Frame()

	if !tt.HasText("请求超时") {
		t.Fatalf("卡片上应显示错误信息")
	}

	// 点击重试
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("点击重试失败：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if *calls != 2 {
		t.Fatalf("重试后应发起第 2 次请求，实际 %d 次", *calls)
	}
	if !strings.Contains(a.ex.text, "重试成功后的解释") {
		t.Errorf("重试后内容未更新：%q", a.ex.text)
	}
}

// 测试 Diff 视图多行 Range 选区与 Notion 悬浮工具栏
func TestRangeSelectionAndNotionToolbar(t *testing.T) {
	srv, _ := fakeLLM(t, "选区多行解释结果")
	a := richApp(t)
	withProvider(t, a, srv)
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	a.rebuildRows()
	tt.Frame()

	// 1. 测试 updateDiffRangeSel 选定多行
	a.updateDiffRangeSel(fi, 10, 13, ui.Rect{X: 100, Y: 200, W: 600, H: 80})
	tt.Frame()

	if !a.diffSel.active {
		t.Fatal("diffSel 应该处于 active 状态")
	}
	if a.diffSel.startLine != 10 || a.diffSel.endLine != 13 {
		t.Fatalf("选区行号应为 10..13，实际为 %d..%d", a.diffSel.startLine, a.diffSel.endLine)
	}

	// 2. 检查 Notion 浮动工具栏是否渲染
	if !tt.HasText("4 行 (L10-L13)") {
		t.Errorf("Notion 工具栏应显示行数信息，实际文本: %q", tt.Texts())
	}
	if !tt.HasText("复制") || !tt.HasText("AI 解析") {
		t.Errorf("Notion 工具栏应包含复制和 AI 解析按钮")
	}

	// 3. 点击「复制」
	if err := tt.Click("复制所选代码"); err != nil {
		t.Fatalf("点击复制按钮失败: %v", err)
	}
	tt.Frame()
	if a.diffSel.active {
		t.Error("点击复制后选区应关闭")
	}
	if a.opNote != "已复制 4 行代码到剪贴板" {
		t.Errorf("opNote 应为简洁的行数提示，实际为 %q", a.opNote)
	}
	if strings.Contains(a.opNote, "some code here") {
		t.Errorf("opNote 绝不应把被复制的代码内容弹出来: %q", a.opNote)
	}

	// 4. 再次选定多行并点击「AI 解析」
	a.updateDiffRangeSel(fi, 5, 8, ui.Rect{X: 100, Y: 100, W: 600, H: 80})
	tt.Frame()
	if err := tt.Click("AI 解析所选代码"); err != nil {
		t.Fatalf("点击 AI 解析按钮失败: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if !a.explainOpen() {
		t.Fatal("点击 AI 解析后解释卡片应打开")
	}
	if !a.ex.isSelection {
		t.Fatal("解释卡片应标记为 isSelection")
	}
}

// 测试英文模式下「Explain」按钮文案适配，以及单行点击不弹浮层、拖拽选区才弹浮层
func TestEnglishExplainButtonsAndSingleClickNoToolbar(t *testing.T) {
	srv, _ := fakeLLM(t, "English explanation")
	a := richApp(t)
	withProvider(t, a, srv)
	a.setLocale("en")
	defer a.setLocale("zh")
	tt := ui.NewTester(a.view, 1280, 860)

	fi, _ := explainFixtures(t, a)
	a.rebuildRows()
	tt.Frame()

	// 1. 验证英文模式下按钮显示 "Explain" 而非 "解释"
	if !tt.HasText("Explain") {
		t.Errorf("英文模式下按钮应显示 'Explain'，实际文本: %q", tt.Texts())
	}
	if tt.HasText("解释") {
		t.Errorf("英文模式下不应显示中文 '解释'")
	}

	// 2. 模拟单击单行（无拖拽、无多行选区），选区浮层不应该弹出
	a.updateDiffRangeSel(fi, 3, 3, ui.Rect{X: 100, Y: 100, W: 600, H: 20})
	a.diffSel.draggedRange = false
	tt.Frame()

	if tt.HasText("Copy") || tt.HasText("复制") {
		t.Errorf("单行点击时不应弹出 Notion 选区浮层")
	}

	// 3. 模拟拖拽多行选区，选区浮层应该正常弹出
	a.updateDiffRangeSel(fi, 3, 6, ui.Rect{X: 100, Y: 100, W: 600, H: 80})
	tt.Frame()

	if !tt.HasText("4 lines (L3-L6)") {
		t.Errorf("多行选区应显示英文行数信息: %q", tt.Texts())
	}
	if !tt.HasText("Copy") {
		t.Errorf("多行选区应显示英文 Copy 按钮: %q", tt.Texts())
	}
}
