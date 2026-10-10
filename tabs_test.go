package main

import (
	"path/filepath"
	"testing"
)

func TestTabCreationAndSwitching(t *testing.T) {
	a := newApp()
	repo1 := filepath.Join(t.TempDir(), "repo1")
	repo2 := filepath.Join(t.TempDir(), "repo2")

	a.openInNewTab(repo1)
	if len(a.tabs) != 1 {
		t.Fatalf("预期 1 个标签页，实际 %d", len(a.tabs))
	}
	if a.activeTab != 0 {
		t.Errorf("预期当前激活第 0 个标签页，实际 %d", a.activeTab)
	}

	a.openInNewTab(repo2)
	if len(a.tabs) != 2 {
		t.Fatalf("预期 2 个标签页，实际 %d", len(a.tabs))
	}
	if a.activeTab != 1 {
		t.Errorf("预期当前激活第 1 个标签页，实际 %d", a.activeTab)
	}

	// 切换回第 0 个
	a.switchTab(0)
	if a.activeTab != 0 {
		t.Errorf("switchTab(0) 后应激活第 0 个，实际 %d", a.activeTab)
	}

	// 再次打开已存在的 repo1，应直接激活已有 tab 而不是新建
	a.openInNewTab(repo1)
	if len(a.tabs) != 2 {
		t.Fatalf("重复打开已有仓库不应新建 tab，预期 2 个，实际 %d", len(a.tabs))
	}
	if a.activeTab != 0 {
		t.Errorf("重复打开已存在的 repo1 应切回第 0 个标签页，实际 %d", a.activeTab)
	}
}

func TestTabCycle(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	a.openInNewTab("/tmp/proj2")
	a.openInNewTab("/tmp/proj3")

	if a.activeTab != 2 {
		t.Fatalf("当前应激活最后新开的第 2 个 tab，实际 %d", a.activeTab)
	}

	a.nextTab() // 循环回到 0
	if a.activeTab != 0 {
		t.Errorf("nextTab 预期切到 0，实际 %d", a.activeTab)
	}

	a.prevTab() // 循环回到 2
	if a.activeTab != 2 {
		t.Errorf("prevTab 预期切到 2，实际 %d", a.activeTab)
	}
}

func TestTabClosing(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	a.openInNewTab("/tmp/proj2")
	a.openInNewTab("/tmp/proj3")

	// 当前在 tab 2，关闭 tab 2 后应激活 tab 1
	a.closeTab(2)
	if len(a.tabs) != 2 {
		t.Fatalf("关闭后预期剩余 2 个 tab，实际 %d", len(a.tabs))
	}
	if a.activeTab != 1 {
		t.Errorf("关闭当前 tab 后预期激活第 1 个，实际 %d", a.activeTab)
	}

	// 关闭剩下的全部 tab
	a.closeTab(0)
	a.closeTab(0)
	if len(a.tabs) != 1 {
		t.Fatalf("关闭所有 tab 后应保留一个空兜底 tab，实际 %d", len(a.tabs))
	}
	if a.repoPath != "" {
		t.Errorf("清空所有 tab 后 repoPath 应为空，实际 %q", a.repoPath)
	}
}

func TestViewModeToggle(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")

	if a.viewMode != viewModeGit {
		t.Errorf("默认视图模式应为 Git GUI (0)，实际 %d", a.viewMode)
	}

	a.toggleViewMode()
	if a.viewMode != viewModeTerm {
		t.Errorf("toggleViewMode 后应为终端模式 (1)，实际 %d", a.viewMode)
	}

	tab := a.currentTab()
	if tab == nil || tab.viewMode != viewModeTerm {
		t.Errorf("标签页内部视图模式应同步为终端模式")
	}

	a.toggleViewMode()
	if a.viewMode != viewModeGit {
		t.Errorf("再次 toggleViewMode 应切回 Git GUI 模式 (0)，实际 %d", a.viewMode)
	}
}
