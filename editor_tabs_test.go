package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// openEditorFile 把工作区里的一个文件打开成 tab（和用户在左栏点一下同一条
// 路），并等读盘结束。
func openEditorFile(t *testing.T, a *app, path string) *editorTabState {
	t.Helper()
	f := wsFileByPath(a.wsAll, path)
	if f == nil {
		t.Fatalf("工作区里没有 %s", path)
	}
	a.openEditorTab(*f)
	settle(t, a)
	tab := a.activeEditorTab()
	if tab == nil || tab.path != path {
		t.Fatalf("打开 %s 后激活的不是它", path)
	}
	return tab
}

// 单击文件打开成 tab；已打开的文件再点一次只激活，不重复读盘，缓冲区里
// 没保存的内容也不会被冲掉。
func TestEditorTabsOpenActivateAndDedupe(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	if len(a.editorPane.tabs) == 0 {
		t.Fatal("第一次进编辑器视图该自动打开左栏选中的文件")
	}

	openEditorFile(t, a, "README.md")
	readme := a.activeEditorTab()
	readme.buffer.Set("README 里没保存的内容\n")

	openEditorFile(t, a, "internal/server/handler.go")
	if len(a.editorPane.tabs) != 2 {
		t.Fatalf("打开第二个文件后该有两个 tab，实际 %d", len(a.editorPane.tabs))
	}

	openEditorFile(t, a, "README.md")
	if len(a.editorPane.tabs) != 2 {
		t.Errorf("已打开的文件不该再开一个 tab，实际 %d 个", len(a.editorPane.tabs))
	}
	if a.activeEditorTab() != readme {
		t.Error("重复打开该激活原来那个 tab")
	}
	if got := readme.buffer.String(); !strings.Contains(got, "README 里没保存的内容") {
		t.Errorf("切回来缓冲区内容丢了：%q", got)
	}
}

// 关闭有未保存改动的 tab 要先问；取消留着，确认后消失并标 dead。
func TestEditorTabDirtyDotAndCloseConfirm(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	tab := openEditorFile(t, a, "README.md")
	tab.buffer.Set("改了没存\n")

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if !tt.HasText("未保存的改动") {
		t.Errorf("tab 上该有未保存标记：%q", tt.Texts())
	}

	a.closeEditorTab(a.editorPane.active)
	tt.Frame()
	if !a.confirm.on {
		t.Fatal("关闭脏 tab 该先弹确认框")
	}
	if !strings.Contains(a.confirm.title, "关闭") || a.confirm.yes != "仍然关闭" {
		t.Errorf("确认框不对：title=%q yes=%q", a.confirm.title, a.confirm.yes)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.editorTabIndex("README.md") < 0 {
		t.Error("取消之后 tab 该还在")
	}

	a.closeEditorTab(a.editorPane.active)
	tt.Frame()
	if err := tt.Click("仍然关闭"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.editorTabIndex("README.md") >= 0 {
		t.Error("确认之后 tab 该关掉")
	}
	if !tab.dead {
		t.Error("关掉的 tab 该标 dead，迟到的读盘/保存结果才能丢弃")
	}
}

// 切视图、切项目都不该丢 tab 和未保存的缓冲区。
func TestEditorTabsSurviveViewAndProjectSwitch(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath) // richApp 只加载状态；项目标签页从这开始
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	openEditorFile(t, a, "README.md")
	tab := openEditorFile(t, a, "internal/server/handler.go")
	tab.buffer.Set("切来切去也不能丢\n")

	// 切终端再回来。
	a.setViewMode(viewModeTerm)
	ui.NewTester(a.view, 1280, 860).Frame()
	a.setViewMode(viewModeEditor)
	settle(t, a)
	if len(a.editorPane.tabs) != 2 || a.activeEditorTab() != tab {
		active := "nil"
		if t := a.activeEditorTab(); t != nil {
			active = t.path
		}
		t.Fatalf("切视图回来 tab 丢了：%d 个，激活 %s", len(a.editorPane.tabs), active)
	}
	if got := tab.buffer.String(); !strings.Contains(got, "切来切去也不能丢") {
		t.Errorf("切视图回来缓冲区丢了：%q", got)
	}

	// 切到另一个项目再切回来。
	other := newRepoDir(t)
	a.openInNewTab(other)
	settle(t, a)
	if a.repoPath != other {
		t.Fatalf("前置条件：该切到新项目 %q，实际 %q", other, a.repoPath)
	}
	a.switchTab(0)
	settle(t, a)

	if len(a.editorPane.tabs) != 2 {
		t.Fatalf("切回项目后 tab 该还在，实际 %d 个", len(a.editorPane.tabs))
	}
	if a.activeEditorTab() == nil || a.activeEditorTab().path != "internal/server/handler.go" {
		t.Error("切回项目后该恢复原来激活的 tab")
	}
	if got := a.activeEditorTab().buffer.String(); !strings.Contains(got, "切来切去也不能丢") {
		t.Errorf("切项目回来缓冲区丢了：%q", got)
	}
}

// 方向键只在左栏移动选中，不该一路按出 tab。
func TestEditorArrowNavDoesNotOpenTabs(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	openEditorFile(t, a, "README.md")
	handler := openEditorFile(t, a, "internal/server/handler.go")
	before := len(a.editorPane.tabs)
	selBefore := a.wsSel

	a.navWorkspaceFile(1)

	if len(a.editorPane.tabs) != before {
		t.Errorf("方向键不该打开 tab：%d → %d", before, len(a.editorPane.tabs))
	}
	if a.activeEditorTab() != handler {
		t.Error("方向键不该切换激活的 tab")
	}
	if a.wsSel == selBefore {
		t.Error("方向键该移动左栏选中")
	}
}

// 仓库刷新不碰编辑器 tab：不跟随左栏选中、不新开、不替换当前 tab；关掉
// 最后一个 tab 后刷新也不会把它重新打开。
func TestEditorRefreshDoesNotTouchTabs(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	readme := openEditorFile(t, a, "README.md")
	handler := openEditorFile(t, a, "internal/server/handler.go")

	// 左栏选中挪开（但不再点开），刷新时不能跟着它走。
	a.selectWorkspaceKey("README.md")
	a.reload()
	settle(t, a)

	if len(a.editorPane.tabs) != 2 || a.activeEditorTab() != handler {
		t.Fatalf("刷新动了 tab：%d 个", len(a.editorPane.tabs))
	}
	if readme.buffer == nil || handler.buffer == nil {
		t.Error("刷新不该重建缓冲区")
	}

	a.closeEditorTab(a.editorTabIndexPtr(readme))
	a.closeEditorTab(a.editorTabIndexPtr(handler))
	if len(a.editorPane.tabs) != 0 {
		t.Fatalf("前置条件：tab 该关光了，实际 %d", len(a.editorPane.tabs))
	}
	a.reload()
	settle(t, a)
	if len(a.editorPane.tabs) != 0 {
		t.Errorf("关光之后刷新不该重新打开文件，实际 %d 个", len(a.editorPane.tabs))
	}
}

// 第一次进编辑器视图打开左栏选中的文件；再进来恢复上次激活的 tab，不再
// 跟随左栏选中。
func TestEditorFirstEnterOpensSelectionAndReenterRestores(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.selectWorkspaceKey("README.md")

	a.setViewMode(viewModeEditor)
	settle(t, a)
	if len(a.editorPane.tabs) != 1 || a.activeEditorTab().path != "README.md" {
		t.Fatalf("第一次进来该打开选中的文件，实际 %d 个", len(a.editorPane.tabs))
	}

	handler := openEditorFile(t, a, "internal/server/handler.go")
	a.selectWorkspaceKey("README.md") // 左栏选中回到 README，但没有点开

	a.setViewMode(viewModeTerm)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	if a.activeEditorTab() != handler {
		t.Error("再次进入编辑器视图该恢复上次激活的 tab，不该跟随左栏选中")
	}
	if len(a.editorPane.tabs) != 2 {
		t.Errorf("再次进来不该新开 tab，实际 %d 个", len(a.editorPane.tabs))
	}
}

// 搜索跳转到已打开的文件：不新开 tab，直接激活它并选中命中；文件被左栏
// 筛选藏起来时，钉住目标不能丢。
func TestWorkspaceSearchActivatesExistingTab(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	readme := openEditorFile(t, a, "README.md")
	handler := openEditorFile(t, a, "internal/server/handler.go")

	// 把 README 从文件树里筛掉：搜索跳转仍要把它的 tab 切到前面。
	a.wsFilter = "handler"
	a.applyWsFilter()

	text := readme.buffer.String()
	at := strings.Index(text, "多了第二行")
	if at < 0 {
		t.Fatalf("前置条件：README 内容里没有命中词：%q", text)
	}
	hit := wsSearchHit{Path: "README.md", Line: 4, Text: "多了第二行", LfPos: at, Len: len("多了第二行")}
	a.openWsSearchHit(hit)

	if len(a.editorPane.tabs) != 2 {
		t.Errorf("搜索跳转不该新开 tab，实际 %d 个", len(a.editorPane.tabs))
	}
	if a.activeEditorTab() != readme {
		t.Error("搜索跳转该激活 README 的 tab")
	}
	if a.wsPinPath != "README.md" {
		t.Errorf("被筛掉的文件该保持钉住，实际 %q", a.wsPinPath)
	}
	if got := readme.code.Selection(); got.Start() != at || got.End() != at+len("多了第二行") {
		t.Errorf("命中该被选中：%v，want [%d,%d)", got, at, at+len("多了第二行"))
	}
	_ = handler
}

// 读盘途中切到别的项目：结果照样写回那个 tab 对象，切回来不能永远转圈。
func TestEditorLoadCompletesAfterProjectSwitch(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	f := wsFileByPath(a.wsAll, "internal/server/handler.go")
	a.openEditorTab(*f) // 读盘还没完成就切走
	a.openInNewTab(newRepoDir(t))
	settle(t, a)

	a.switchTab(0)
	settle(t, a)

	i := a.editorTabIndex("internal/server/handler.go")
	if i < 0 {
		t.Fatal("切回来之后 tab 丢了")
	}
	tab := a.editorPane.tabs[i]
	if tab.loading || tab.code == nil {
		t.Errorf("迟到的那次读盘该写回 tab，实际 loading=%v err=%q", tab.loading, tab.err)
	}
	if tab.dead {
		t.Error("还活着的项目里的 tab 不该被标 dead")
	}
}

// 保存途中切项目：写盘目标在开始时就捕获，成功后只标记那份缓冲，不去
// 刷新眼前这个项目。
func TestEditorSaveWhileSwitchingProjectsTargetsCapturedRepo(t *testing.T) {
	a := richApp(t)
	dirA := a.repoPath
	a.openInNewTab(dirA)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	readme := openEditorFile(t, a, "README.md")
	readme.buffer.Set("A 项目保存的内容\n")

	a.saveEditorPane() // 后台写盘
	other := newRepoDir(t)
	a.openInNewTab(other)
	settle(t, a)

	if got := readDisk(t, dirA+"/README.md"); got != "A 项目保存的内容\n" {
		t.Errorf("保存没落到发起时的仓库：%q", got)
	}
	if readme.buffer.Modified() {
		t.Error("保存成功后该 MarkSaved 发起时的缓冲")
	}
	if a.repoPath != other {
		t.Errorf("当前项目被保存流程带偏了：%q", a.repoPath)
	}
	if len(a.tabs) < 1 || a.tabs[0].state != loadIdle {
		t.Error("切走的项目该置回 loadIdle，下次切回去整体重读")
	}
}

// 编辑器视图里 ⌘W 先关当前文件 tab；文件 tab 关光后才关项目。
func TestCloseActiveClosesEditorTabFirst(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	openEditorFile(t, a, "README.md")
	openEditorFile(t, a, "internal/server/handler.go")
	projects := len(a.tabs)

	a.closeActive()
	if len(a.editorPane.tabs) != 1 {
		t.Fatalf("⌘W 该先关文件 tab，剩 %d 个", len(a.editorPane.tabs))
	}
	if len(a.tabs) != projects {
		t.Error("还有文件 tab 时不该关项目")
	}

	a.closeActive()
	if len(a.editorPane.tabs) != 0 {
		t.Fatalf("文件 tab 该关光，剩 %d 个", len(a.editorPane.tabs))
	}
	if len(a.tabs) != projects {
		t.Error("关文件 tab 不该动项目标签页")
	}

	a.closeActive()
	if len(a.tabs) != projects-1 {
		t.Errorf("没有文件 tab 之后 ⌘W 该关项目，剩 %d 个", len(a.tabs))
	}
}

func TestCloseRepoClearsEditorTabs(t *testing.T) {
	a := richApp(t)
	dir := a.repoPath
	a.openInNewTab(dir) // 项目标签页从这开始，closeTab 才有东西可关
	settle(t, a)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	openEditorFile(t, a, "README.md")
	openEditorFile(t, a, "internal/server/handler.go")

	a.closeTab(0)
	settle(t, a)
	if len(a.editorPane.tabs) != 0 {
		t.Fatalf("关闭项目后 tab 该清空，实际 %d 个", len(a.editorPane.tabs))
	}

	a.openInNewTab(dir)
	settle(t, a)
	if len(a.editorPane.tabs) != 0 {
		t.Errorf("重新打开项目不该恢复文件 tab，实际 %d 个", len(a.editorPane.tabs))
	}
}

// 保存只写当前激活的 tab：另一个 tab 的未保存内容不动，MarkSaved 也只落
// 在它自己的缓冲上。
func TestEditorSaveTargetsActiveTab(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	readme := openEditorFile(t, a, "README.md")
	handler := openEditorFile(t, a, "internal/server/handler.go")

	readme.buffer.Set("README 的未保存内容\n")
	handler.buffer.Set("handler.go 保存这一份\n")

	a.saveEditorPane()
	settle(t, a)

	if got := readDisk(t, a.repoPath+"/internal/server/handler.go"); got != "handler.go 保存这一份\n" {
		t.Errorf("激活 tab 没写盘：%q", got)
	}
	if got := readDisk(t, a.repoPath+"/README.md"); strings.Contains(got, "README 的未保存内容") {
		t.Errorf("不该把另一个 tab 的内容写盘：%q", got)
	}
	if !readme.buffer.Modified() {
		t.Error("另一个 tab 仍是未保存状态，不该被 MarkSaved")
	}
	if handler.buffer.Modified() {
		t.Error("保存过的 tab 不该还是未保存")
	}
}
