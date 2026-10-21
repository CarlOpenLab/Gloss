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

	// 收起侧栏后，项目下拉和分支徽章应回到主卡工具栏的同一行。现在没有
	// 单独的顶部项目标签带，项目名也不再跨行压在工具栏上方。
	a.sidebarOn = false
	tt.Frame()
	tt.Frame()

	tab, okTab := tt.Find(a.repoName)
	if !okTab {
		t.Fatalf("工具栏上找不到项目名 %q", a.repoName)
	}
	chip, okChip := tt.Find(a.branch)
	if !okChip {
		t.Fatalf("界面上找不到当前分支名 %q", a.branch)
	}
	if abs(float64(tab.Y+tab.H/2-chip.Y-chip.H/2)) > 4 {
		t.Errorf("项目下拉与分支徽章应同在工具栏一行：项目中心 y=%.1f，分支中心 y=%.1f",
			tab.Y+tab.H/2, chip.Y+chip.H/2)
	}
	if tab.X >= chip.X {
		t.Errorf("项目下拉应该在分支徽章左边：项目 x=%.1f，分支 x=%.1f", tab.X, chip.X)
	}
}

// toolbarRepoPoint 给出工具栏上那枚仓库名按钮的点击位置。
//
// 侧栏收起时项目下拉和分支徽章都在工具栏同一行。从分支徽章往左让开按钮
// 自身的一小段宽度，落到的就是仓库按钮。
func toolbarRepoPoint(tt *ui.Tester, branch string) (x, y float32, ok bool) {
	chip, found := tt.Find(branch)
	if !found {
		return 0, 0, false
	}
	return chip.X - 20, chip.Y + chip.H/2, true
}

func TestHeaderRepoDropdownPopover(t *testing.T) {
	a := richApp(t)
	// 仓库下拉按钮在工具栏上：侧栏收起时工具栏摆着「仓库名 + 分支」那块。
	a.sidebarOn = false
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 初始状态下弹窗未开启
	if a.repoDropdownOpen {
		t.Fatal("初始状态 repoDropdownOpen 应该为 false")
	}

	// 仓库按钮就在同排分支徽章的左边（见 toolbarRepoPoint）。
	repoX, repoY, ok := toolbarRepoPoint(tt, a.branch)
	if !ok {
		t.Fatalf("工具栏上找不到分支徽章 %q：%q", a.branch, tt.Texts())
	}
	tt.ClickAt(repoX, repoY)
	tt.Frame()

	// 验证弹窗已开启
	if !a.repoDropdownOpen {
		t.Fatalf("点击仓库按钮（x=%.0f, y=%.0f）后 repoDropdownOpen 应该为 true", repoX, repoY)
	}

	// 验证 HeroUI 弹窗内渲染的标题和操作项
	if !tt.HasText("最近打开的项目") {
		t.Error("弹窗内应显示标题「最近打开的项目」")
	}
	if !tt.HasText("打开其他本地仓库…") {
		t.Error("弹窗内应显示「打开其他本地仓库…」")
	}
	if !tt.HasText("当前") {
		t.Error("弹窗内当前仓库应显示「当前」徽章")
	}

	// 验证弹窗位置在仓库按钮下方，没有向上覆盖或重叠偏移
	titleBox, okTitle := tt.Find("最近打开的项目")
	if !okTitle {
		t.Fatal("未找到弹窗标题「最近打开的项目」")
	}
	if titleBox.Y <= repoY {
		t.Errorf("弹窗应该出现在仓库按钮下方，但 titleBox.Y=%.1f <= 按钮 y=%.1f", titleBox.Y, repoY)
	}

	// 按 Escape 键关闭弹窗
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	if a.repoDropdownOpen {
		t.Error("按 Escape 后 repoDropdownOpen 应该为 false")
	}
}

// 最近打开的项目，一条一条删得掉。
//
// 删的是「这一条记录」，不是「关掉这个项目」：正开着的标签页照旧开着，
// 也不会顺手把这一条打开，只是下一次不再从最近列表里冒出来。
func TestRecentRepoRemovedFromPlusMenu(t *testing.T) {
	a := richApp(t)
	// 再塞两条历史进去（richApp 已经把当前仓库记进去了）。
	dirA := filepath.Join(t.TempDir(), "old-a")
	dirB := filepath.Join(t.TempDir(), "old-b")
	a.store.AddRecentRepo(dirA)
	a.store.AddRecentRepo(dirB)
	a.persist()
	tabsBefore := len(a.tabs)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// 开 [+] 的下拉。
	if err := tt.Click("新建项目标签页"); err != nil {
		t.Fatalf("点不开 [+] 的下拉：%v", err)
	}
	tt.Frame()
	if _, ok := tt.Find(dirA); !ok {
		t.Fatalf("下拉里找不到 %s：%q", dirA, tt.Texts())
	}

	// 点 dirA 那一行的 ×。按标签点：每行的 × 都带着自己那条的名称，
	// 所以点的是哪一条不含糊（ro 坐标会落在行上的文字里，等于点了这一行）。
	if err := tt.Click(Tf("从最近列表移除 %s", "Remove %s from recent", filepath.Base(dirA))); err != nil {
		t.Fatalf("点不到 %s 那行的 ×：%v\n%q", dirA, err, tt.Texts())
	}
	tt.Frame()

	got := a.store.State().RecentRepos
	for _, p := range got {
		if p == dirA {
			t.Errorf("删掉了 %s，它还在最近列表里：%q", dirA, got)
		}
	}
	found := false
	for _, p := range got {
		if p == dirB {
			found = true
		}
	}
	if !found {
		t.Errorf("只该删掉那一条，但 %s 也不见了：%q", dirB, got)
	}
	if _, ok := tt.Find(dirA); ok {
		t.Errorf("删完这一行还画在面板上：%q", tt.Texts())
	}
	if _, ok := tt.Find(dirB); !ok {
		t.Errorf("另一条不该跟着消失：%q", tt.Texts())
	}
	// 点 × 只是删记录：不该顺手把 dirA 打开成一个新标签页。
	if len(a.tabs) != tabsBefore {
		t.Errorf("删一条最近记录开了标签页：%d -> %d", tabsBefore, len(a.tabs))
	}
}
