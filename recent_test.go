package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRecentReposPersistenceAndPalette(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 确认当前仓库被存入了 store 的 LastRepo 与 RecentRepos
	st := a.store.State()
	if st.LastRepo != a.repoPath {
		t.Fatalf("LastRepo 应为当前仓库 %q, 实际为 %q", a.repoPath, st.LastRepo)
	}
	if len(st.RecentRepos) == 0 || st.RecentRepos[0] != a.repoPath {
		t.Fatalf("RecentRepos 首位应为当前仓库 %q", a.repoPath)
	}

	// 验证命令面板中包含了打开最近项目的命令
	cmds := a.commands()
	found := false
	wantLabel := "打开最近项目: " + filepath.Base(a.repoPath)
	for _, cmd := range cmds {
		if cmd.label == wantLabel {
			found = true
			if cmd.hint != a.repoPath {
				t.Errorf("命令提示应该是仓库路径 %q，实际为 %q", a.repoPath, cmd.hint)
			}
			break
		}
	}
	if !found {
		t.Errorf("命令面板中未找到命令 %q", wantLabel)
	}
}

func TestHeaderRepoMenuAndBranchQuickMenu(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 测试主菜单中的「最近打开的项目」
	menu := buildAppMenu(a)
	recentItem := menuItem(t, menu, "最近打开的项目")
	if recentItem == nil {
		t.Fatal("菜单中应该包含「最近打开的项目」")
	}

	// 确认仓库名和当前分支名能在界面上找到，且处于同一水平行（垂直位置居中对齐）
	// 关闭侧栏以唯一定位工具栏上的分支徽章（避免与侧栏分支树中的同名项混淆）
	a.sidebarOn = false
	tt.Frame()
	tt.Frame()

	repoBox, okRepo := tt.Find(a.repoName)
	if !okRepo {
		t.Fatalf("界面上找不到仓库名 %q", a.repoName)
	}
	branchBox, okBranch := tt.Find(a.branch)
	if !okBranch {
		t.Fatalf("界面上找不到当前分支名 %q", a.branch)
	}

	// 两者处于同一行，Y 轴重叠（差值小于 5 像素）
	diffY := repoBox.Y - branchBox.Y
	if diffY < -5 || diffY > 5 {
		t.Errorf("仓库名与分支徽章应水平对齐在同一行：repoY=%.1f, branchY=%.1f", repoBox.Y, branchBox.Y)
	}
}
