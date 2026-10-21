package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// projectPlusRect 返回标签栏 [+] 的矩形。
//
// [+] 是纯图标按钮、没有可见文字，但它的无障碍 Label 会被 Tester 记成 label
// 节点，所以直接按标签找 —— 比从相邻标签的宽度推算坐标稳（标签有没有关闭
// 按钮、有没有脏标记都会让推算错位）。
func projectPlusRect(t *testing.T, tt *ui.Tester) ui.Rect {
	t.Helper()
	r, ok := tt.Find(T("新建项目标签页", "New Project Tab"))
	if !ok {
		t.Fatalf("标签栏里找不到 [+] 按钮")
	}
	return r
}

// committedRepoDir 造一个带一次提交的仓库，供打开过的项目使用。
func committedRepoDir(t *testing.T) string {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "b.go"), "package b\n")
	runGit(t, dir, "add", "b.go")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

// openPlusMenu 点开顶栏 [+] 的「新建项目」下拉。
func openPlusMenu(t *testing.T, tt *ui.Tester, a *app) {
	t.Helper()
	r := projectPlusRect(t, tt)
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if !a.newTabMenuOpen {
		t.Fatalf("点击 [+] 应打开「新建项目」下拉")
	}
}

func TestNewProjectMenuListsRecentThenOpenFolder(t *testing.T) {
	a := richApp(t)
	other := committedRepoDir(t)
	a.store.AddRecentRepo(other)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	openPlusMenu(t, tt, a)

	recentRect, ok := tt.Find(filepath.Base(other))
	if !ok {
		t.Fatalf("下拉里应列出打开过的项目 %q", other)
	}
	folderRect, ok := tt.Find(T("打开本地文件夹…", "Open Local Folder…"))
	if !ok {
		t.Fatalf("下拉里应有末位项「打开本地文件夹…」")
	}
	if folderRect.Y <= recentRect.Y {
		t.Fatalf("「打开本地文件夹…」应排在历史项目之后：历史 y=%.1f，末位 y=%.1f",
			recentRect.Y, folderRect.Y)
	}
}

// 高度是这次改动的重点：面板从窗口最顶端往下展开，历史最多 10 条，窗口一矮
// 就可能把末位的「打开本地文件夹…」顶出去 —— 而它在滚动列表之外，顶出去就
// 再也够不到了。
func TestNewProjectMenuKeepsOpenFolderVisibleWithFullHistory(t *testing.T) {
	a := richApp(t)
	for i := 0; i < 14; i++ { // 超过 store 上限（10），确保塞满
		a.store.AddRecentRepo(filepath.Join("/tmp", fmt.Sprintf("gloss-recent-%02d", i)))
	}

	const winH = 300 // 矮窗口：满历史若不按窗口收缩，末位项会落到 300 之外
	tt := ui.NewTester(a.view, 1280, winH)
	tt.Frame()
	openPlusMenu(t, tt, a)

	folderRect, ok := tt.Find(T("打开本地文件夹…", "Open Local Folder…"))
	if !ok {
		t.Fatalf("下拉里应有末位项「打开本地文件夹…」")
	}
	if folderRect.Y < 0 {
		t.Fatalf("末位项被挤到窗口上边界之外：y=%.1f", folderRect.Y)
	}
	if bottom := folderRect.Y + folderRect.H; bottom > winH {
		t.Fatalf("满历史把「打开本地文件夹…」顶出了窗口：底部 y=%.1f > 窗口高 %d", bottom, winH)
	}
}

func TestNewProjectMenuOpensRecentProjectInNewTab(t *testing.T) {
	a := richApp(t)
	other := committedRepoDir(t)
	a.store.AddRecentRepo(other)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	openPlusMenu(t, tt, a)

	base := filepath.Base(other)
	row, ok := tt.Find(base)
	if !ok {
		t.Fatalf("下拉里应列出 %q", other)
	}
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if a.newTabMenuOpen {
		t.Fatalf("选中项目后下拉应关闭")
	}
	if len(a.tabs) != 1 {
		t.Fatalf("选中的项目应成为一个项目标签页，实际 %d 个", len(a.tabs))
	}
	if !sameRepo(a.repoPath, other) {
		t.Fatalf("选中的项目应被打开 %q，实际 %q", other, a.repoPath)
	}
}

// 已经在标签页里的项目要标出来：否则「选一个」和「又开一个」看起来一样。
func TestNewProjectMenuMarksAlreadyOpenProject(t *testing.T) {
	a := richApp(t)
	settle(t, a)
	a.openInNewTab(a.repoPath) // 当前仓库成为一个标签页
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	openPlusMenu(t, tt, a)

	if !tt.HasText(T("已打开", "Open")) {
		t.Fatalf("当前已打开的项目在下拉里应带「已打开」标记")
	}
}
