package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/egoist/mygo/ui"
)

// 根目录下的文件也能批量暂存 / 取消暂存 / 放弃，也能多选。
//
// 用户报的是：「工作区改动 如果是根目录的文件 是没办法多选 丢弃 这些操作的」。
//
// 两件事各自缺一道：
//
//   - 批量入口一直是目录行上的右键菜单（dir.go 的 dirMenu），而
//     filetree.BuildAt 只给**带斜杠的路径**建目录节点。README.md 这种根下的
//     文件直接当顶层文件落下来，整棵树上一个目录行都没有 —— 同样的改动挪到
//     internal/ 底下就有「全部暂存 / 全部放弃」，放在根下就一处都没有。
//   - 「多选」这件事在整个文件栏里根本不存在：没有 Cmd 点、没有 Shift 点，
//     也没有⌘A。所以挑三个文件一起暂存只能一个个来。
//
// 修法：根上补一个仓库根目录行，让 dirFiles 顺着它收到根下全部文件；文件栏
// 接上 Cmd / Shift 多选，选中集按文件 key 记，批量动作对着选中集下手。

// rootApp 是一个「改动全在根目录」的仓库：改过的、删掉的、没跟踪的、已暂存的
// 各一个，再加一个一直干净的。
func rootApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	initRepo(t, dir)

	writeFile(t, filepath.Join(dir, "mod.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "gone.go"), "package gone\n")
	writeFile(t, filepath.Join(dir, "staged.go"), "package staged\n")
	writeFile(t, filepath.Join(dir, "clean.go"), "package clean\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	writeFile(t, filepath.Join(dir, "mod.go"), "package main\n\n// 改了\n")
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "fresh.go"), "package fresh\n")
	writeFile(t, filepath.Join(dir, "staged.go"), "package staged\n\n// 暂存了\n")
	runGit(t, dir, "add", "staged.go")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()
	return a, tt
}

// rootDirRow 找出那个仓库根目录行（标题「（根目录）」，见 wrapRootDir）。
//
// 走摊平后的行：分组态下它挂在已暂存 / 未暂存那两个分组行底下，a.tree 的顶层
// 扫不到它。也返回全部根目录行 —— 分组态下两组各有一个，各收自己那一段
// （和 internal/server 在两组下各占一行是一个道理）。
func rootDirRows(a *app) []*filetree.Node {
	var out []*filetree.Node
	for _, r := range flattenRows(a) {
		if r.Node != nil && r.Node.Dir && r.Node.Path == "" {
			out = append(out, r.Node)
		}
	}
	return out
}

// 树根上得有目录行：根下的文件是它的子节点，右键它能批量。
func TestRootDirRowWrapsRootFiles(t *testing.T) {
	a, _ := rootApp(t)

	roots := rootDirRows(a)
	if len(roots) == 0 {
		t.Fatalf("树根上该有一个仓库根目录行，实际 %v", rowKeys(flattenRows(a)))
	}
	// 分组态下两组各有一个根目录行，各收自己那一段的文件：合起来才是根下全部。
	n := 0
	for _, root := range roots {
		n += len(root.Children)
	}
	if n != 4 {
		t.Errorf("根目录行底下该有 4 个文件（改的、删的、没跟踪的、已暂存的），实际 %d 个", n)
	}
}

// 右键仓库根目录行：该有「全部暂存 / 全部取消暂存 / 全部放弃变更」，计数把根下
// 三种未暂存取齐，已暂存的不算进「全部暂存」。
func TestRootDirMenuHasBatchItems(t *testing.T) {
	a, tt := rootApp(t)
	items := rightClickRootDir(t, a, tt)

	for _, want := range []string{
		"全部暂存（3 个文件）",
		"全部放弃变更…（3 个文件）",
		"在访达中显示",
	} {
		if !menuHas(items, want) {
			t.Errorf("菜单里没有 %q：%q", want, items)
		}
	}
	// 已暂存的那个在另一组的根目录行里（见 TestRootDirRowUnderGrouping），
	// 所以这一组里不该有取消暂存
	if menuHas(items, "全部取消暂存（1 个文件）") {
		t.Errorf("这一组里没有已暂存的文件，不该报取消暂存：%q", items)
	}
}

// 对着仓库根「全部暂存」：根下未暂存的三个一起进 index，一个不漏。
func TestRootDirMenuStagesRootFiles(t *testing.T) {
	a, tt := rootApp(t)
	rightClickRootDir(t, a, tt)
	if err := tt.ChooseMenuItem("全部暂存（3 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	want := "fresh.go\ngone.go\nmod.go\nstaged.go"
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != want {
		t.Errorf("index 里是 %q，want %q", got, want)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); got != "" {
		t.Errorf("该没有未暂存改动了，实际 %q", got)
	}
}

// 对着仓库根「全部取消暂存」：已暂存的退回未暂存，没暂存的原样不动，
// 一直干净的那个不该被卷进来。
func TestRootDirMenuUnstagesRootFiles(t *testing.T) {
	a, tt := rootApp(t)
	rightClickRootRow(t, a, tt, true)
	if err := tt.ChooseMenuItem("全部取消暂存（1 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("index 该空了，实际 %q", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); !strings.Contains(got, "staged.go") {
		t.Errorf("staged.go 该回到未暂存：%q", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); strings.Contains(got, "clean.go") {
		t.Errorf("clean.go 一直没动过，不该出现：%q", got)
	}
}

// 对着仓库根「全部放弃变更」要先问一句，确认后未暂存的三个都没了，
// 已暂存的那个一个字都不丢。
func TestRootDirMenuDiscardsRootFiles(t *testing.T) {
	a, tt := rootApp(t)
	rightClickRootDir(t, a, tt)
	if err := tt.ChooseMenuItem("全部放弃变更…（3 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("丢弃前该先问一句")
	}
	// 还没确认：什么都不该动
	if got := gitOut(t, a.repoPath, "status", "--porcelain"); !strings.Contains(got, "fresh.go") {
		t.Errorf("没确认就动了工作区：%q", got)
	}

	a.confirm.onYes()
	settle(t, a)
	tt.Frame()

	if got := fileText(t, a.repoPath, "mod.go"); got != "package main\n" {
		t.Errorf("mod.go 该回到原样，实际 %q", got)
	}
	if got := fileText(t, a.repoPath, "gone.go"); got != "package gone\n" {
		t.Errorf("gone.go 该被恢复回来，实际 %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.repoPath, "fresh.go")); !os.IsNotExist(err) {
		t.Errorf("未跟踪的 fresh.go 该被删掉：%v", err)
	}
	// 已暂存的不受影响：丢弃只动未暂存那一段
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "staged.go" {
		t.Errorf("已暂存的 staged.go 该原样留着，实际 %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.repoPath, "clean.go")); err != nil {
		t.Errorf("clean.go 该还在：%v", err)
	}
}

// 一个文件都没变的仓库不该凭空多出一行来。
func TestRootDirRowAbsentWhenClean(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	initRepo(t, dir)
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if len(a.tree) != 0 {
		t.Errorf("干净的仓库树上该一行都没有，实际 %d 行", len(a.tree))
	}
}

// 根下的文件和子目录里的文件同时在时，两边各自批量，互不干扰。
func TestRootAndNestedDirsEachBatchTheirOwn(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	initRepo(t, dir)
	writeFile(t, filepath.Join(dir, "root.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "internal/server/handler.go"), "package server\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	writeFile(t, filepath.Join(dir, "root.go"), "package main\n\n// root\n")
	writeFile(t, filepath.Join(dir, "internal/server/handler.go"), "package server\n\n// nested\n")
	writeFile(t, filepath.Join(dir, "internal/server/fresh.go"), "package server\n\nfunc F() {}\n")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 仓库根：只有 root.go 一个
	rightClickRootDir(t, a, tt)
	if err := tt.ChooseMenuItem("全部暂存（1 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "root.go" {
		t.Errorf("index 里该只有 root.go，实际 %q", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); !strings.Contains(got, "internal/server/handler.go") {
		t.Errorf("internal/ 底下两个该还是未暂存：%q", got)
	}

	// 子目录：两个一起
	rightClick(t, tt, "internal/server")
	if err := tt.ChooseMenuItem("全部暂存（2 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	want := "internal/server/fresh.go\ninternal/server/handler.go\nroot.go"
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != want {
		t.Errorf("index 里是 %q，want %q", got, want)
	}
}

// 分组态（已暂存 + 未暂存各一段）下根目录行照样在：它是未暂存那一组的子节点，
// 计数只数未暂存那几个。
func TestRootDirRowUnderGrouping(t *testing.T) {
	a, tt := rootApp(t)
	items := rightClickRootDir(t, a, tt)
	if !menuHas(items, "全部暂存（3 个文件）") {
		t.Errorf("分组态下根目录行也该有批量暂存：%q", items)
	}
	if menuHas(items, "全部暂存（4 个文件）") {
		t.Errorf("已暂存的不该算进「全部暂存」：%q", items)
	}
}

// 多选：Cmd 点两个根下的文件，两个都是选中态。
func TestCmdClickSelectsMultipleRootFiles(t *testing.T) {
	a, tt := rootApp(t)
	before := a.fileSel

	clickRow(t, tt, "mod.go", ui.Cmd)
	if got := a.fileSel; got == before {
		t.Errorf("Cmd 点 mod.go 该选中它，fileSel 还是 %d", got)
	}
	clickRow(t, tt, "fresh.go", ui.Cmd)
	tt.Frame()

	if !a.filesChosen.Has("u:mod.go") {
		t.Errorf("mod.go 该在选中集里")
	}
	if !a.filesChosen.Has("u:fresh.go") {
		t.Errorf("fresh.go 该在选中集里")
	}
	if a.filesChosen.Len() != 2 {
		t.Errorf("该选中 2 个，实际 %d", a.filesChosen.Len())
	}
	// 没点过的不该被捎上
	if a.filesChosen.Has("s:staged.go") || a.filesChosen.Has("u:gone.go") {
		t.Errorf("没点过的不该在选中集里")
	}
}

// 多选之后批量暂存：选中的两个进 index，没选中的一个都不动。
func TestCmdClickThenStageSelected(t *testing.T) {
	a, tt := rootApp(t)

	clickRow(t, tt, "mod.go", ui.Cmd)
	clickRow(t, tt, "fresh.go", ui.Cmd)
	tt.Frame()

	a.stageSelected()
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "fresh.go\nmod.go\nstaged.go" {
		t.Errorf("index 里是 %q，want 选中的两个加原本已暂存的 staged.go", got)
	}
	// gone.go 没选中，该还是未暂存的删除
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); !strings.Contains(got, "gone.go") {
		t.Errorf("没选中的 gone.go 该还是未暂存：%q", got)
	}
}

// 多选之后批量放弃：只丢选中的那两个，先问一句。
func TestCmdClickThenDiscardSelected(t *testing.T) {
	a, tt := rootApp(t)

	clickRow(t, tt, "mod.go", ui.Cmd)
	clickRow(t, tt, "fresh.go", ui.Cmd)
	tt.Frame()

	a.discardSelected()
	settle(t, a)
	tt.Frame()

	if !a.confirm.on {
		t.Fatal("丢弃前该先问一句")
	}
	a.confirm.onYes()
	settle(t, a)
	tt.Frame()

	if got := fileText(t, a.repoPath, "mod.go"); got != "package main\n" {
		t.Errorf("选中的 mod.go 该回到原样，实际 %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.repoPath, "fresh.go")); !os.IsNotExist(err) {
		t.Errorf("选中的 fresh.go 该被删掉：%v", err)
	}
	// 没选中的 gone.go 该还是删着的状态
	if _, err := os.Stat(filepath.Join(a.repoPath, "gone.go")); !os.IsNotExist(err) {
		t.Errorf("没选中的 gone.go 该还是删着的：%v", err)
	}
}

// 没有多选时，批量动作退回单文件那一套（不能对着空选中集动手，也不能一把
// 把整个仓库都暂存了）。
func TestBatchActionsFallBackToSingleSelection(t *testing.T) {
	a, tt := rootApp(t)

	clickRow(t, tt, "mod.go", 0)
	tt.Frame()
	if a.filesChosen.Len() != 0 {
		t.Errorf("普通点击不该留下多选，实际 %d 个", a.filesChosen.Len())
	}

	a.stageSelected()
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "mod.go\nstaged.go" {
		t.Errorf("没多选时该只暂存选中的那一个，实际 %q", got)
	}
}

// 选中跟着文件走，不跟着行走：筛选之后树重建、行全变了，选中的还是那批文件。
func TestSelectionFollowsFilesNotRows(t *testing.T) {
	a, tt := rootApp(t)

	clickRow(t, tt, "mod.go", ui.Cmd)
	clickRow(t, tt, "fresh.go", ui.Cmd)
	tt.Frame()
	if a.filesChosen.Len() != 2 {
		t.Fatalf("该选中 2 个，实际 %d", a.filesChosen.Len())
	}

	a.fileFilter = "fresh"
	a.applyFilter()
	settle(t, a)
	tt.Frame()

	if !a.filesChosen.Has("u:fresh.go") {
		t.Errorf("筛过之后 fresh.go 该还在选中集里")
	}
	if !a.filesChosen.Has("u:mod.go") {
		t.Errorf("mod.go 被筛掉了也不该从选中集里消失：选中的是文件，不是行")
	}
}

// 选中的文件被丢掉之后，选中集该把它放掉 —— 不能留着指向一个已经不存在的文件。
func TestSelectionDropsDiscardedFiles(t *testing.T) {
	a, tt := rootApp(t)

	clickRow(t, tt, "mod.go", ui.Cmd)
	tt.Frame()
	if !a.filesChosen.Has("u:mod.go") {
		t.Fatal("该选中 mod.go")
	}

	a.discardSelected()
	settle(t, a)
	tt.Frame()
	a.confirm.onYes()
	settle(t, a)
	tt.Frame()

	if a.filesChosen.Has("u:mod.go") {
		t.Errorf("丢掉之后 mod.go 不该还在选中集里")
	}
}

// ---- 这一组小工具 ----

// flattenRows 用和应用同一套 Flatten 把树摊平。
func flattenRows(a *app) []filetree.Row {
	return flattenRowsFiltered(a, false)
}

func flattenRowsFiltered(a *app, flat bool) []filetree.Row {
	return filetree.Flatten(a.tree, a.filesClosed, flat)
}

// rightClickRootDir 右键**未暂存那一组**的仓库根目录行，返回菜单项。
//
// 默认点未暂存那一组的根目录行：批量暂存 / 放弃只对未暂存的改动有意义
// （和 internal/server 在两段下各占一行是一个道理）。没分组时 key 是 "r:"，
// 整棵树都是未暂存。
func rightClickRootDir(t *testing.T, a *app, tt *ui.Tester) []string {
	return rightClickRootRow(t, a, tt, false)
}

// rightClickRootRow 右键仓库根目录行；staged 为真时点已暂存那一组的那个。
func rightClickRootRow(t *testing.T, a *app, tt *ui.Tester, staged bool) []string {
	t.Helper()
	rows := flattenRows(a)
	key := ""
	for _, r := range rows {
		// 没分组时 key 是 "r:"（整棵树都是未暂存），分组时是 "r:u|" / "r:s|"。
		want := "r:u"
		if staged {
			want = "r:s"
		}
		if r.Node != nil && r.Node.Dir && r.Node.Path == "" &&
			(r.Node.Key == "r:" || strings.HasPrefix(r.Node.Key, want)) {
			key = r.Node.Key
			break
		}
	}
	if key == "" {
		t.Fatalf("树上没有仓库根目录行（staged=%v）：%v", staged, rowKeys(rows))
	}
	rect := rowRectAt(t, tt, a, key)
	tt.RightClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
	return tt.Menu()
}

// rowRectAt 算出 key 那一行的矩形。
//
// 不能拿标签去 Find：分组态下两组各有一个「（根目录）」（internal/server 同
// 样是两组各一行），而 Tester.Find 只给第一个 —— 拿它点第二个，永远点在第一
// 个上。行高写死 24（treeRowH），所以找一个标签只出现一次的行当下标，按行号
// 差换算过去。
func rowRectAt(t *testing.T, tt *ui.Tester, a *app, key string) ui.Rect {
	t.Helper()
	rows := flattenRows(a)
	idx := -1
	for i, r := range rows {
		if r.Node.Key == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("树上没有 %s 这一行：%v", key, rowKeys(rows))
	}
	anchor, label := -1, ""
	for i, r := range rows {
		if !r.Node.Dir && labelCount(rows, r.Node.Label) == 1 {
			anchor, label = i, r.Node.Label
			break
		}
	}
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("界面上找不到 %q", label)
	}
	return ui.Rect{X: r.X, Y: r.Y + float32(idx-anchor)*treeRowH, W: r.W, H: r.H}
}

// labelCount 数一个标签在摊平行里出现几次。
func labelCount(rows []filetree.Row, label string) int {
	n := 0
	for _, r := range rows {
		if r.Node.Label == label {
			n++
		}
	}
	return n
}

// clickRow 带修饰键点一下某一行。
//
// 修饰键必须走在点击里面（ClickAtWith）：Tester.Press / Release 不带 Mods 字段，
// HoldModifiers 只发 ModifiersChanged —— 修饰键放在外面，点下去的那一下事件里
// 没有它，clickMods 读到的是 0，多选就选不上。
func clickRow(t *testing.T, tt *ui.Tester, name string, mods ui.Modifiers) {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("界面上找不到 %q", name)
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.ClickAtWith(mods, x, y)
	tt.Frame()
}

// rowKeys 把扁平行的 key 列出来，失败时用得上。
func rowKeys(rows []filetree.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Node.Key
	}
	return out
}

// rowRect 找某一行的矩形。同名的行（同一个名字出现在已暂存和未暂存两段）取
// 第一个。
func rowRect(t *testing.T, tt *ui.Tester, name string) (ui.Rect, bool) {
	t.Helper()
	return tt.Find(name)
}
