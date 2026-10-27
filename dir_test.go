package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 文件夹右键菜单：一个目录底下的改动一起暂存 / 取消暂存 / 放弃。
//
// 验收回到 git 自己身上（gitOut）—— 拿我们自己的 Load 去验自己的写，
// 两边一起错的时候测试还是绿的。

// dirRepo 建一个带目录层级的仓库：一次提交，然后按 dirty/staged 两种姿势
// 撒改动。返回仓库目录。
//
// dirty = true：internal/ 底下两个未暂存（一个改过的、一个未跟踪的），
// 已暂存的那个在 pkg/ 底下 —— 于是「internal」这个节点只出现在未暂存分组里，
// 右键点它不会有「到底点了哪一棵」的歧义。
// dirty = false：反过来，internal/ 底下两个已暂存。
func dirRepo(t *testing.T, dirty bool) (*app, *ui.Tester) {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	initRepo(t, dir)

	writeFile(t, filepath.Join(dir, "internal/server/handler.go"), "package server\n\nfunc H() {}\n")
	writeFile(t, filepath.Join(dir, "root.go"), "package main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	if dirty {
		// 改过的 + 未跟踪的各一个，都在 internal/server 里
		writeFile(t, filepath.Join(dir, "internal/server/handler.go"), "package server\n\nfunc H() { /* 改了 */ }\n")
		writeFile(t, filepath.Join(dir, "internal/server/fresh.go"), "package server\n\nfunc F() {}\n")
		// 已暂存的那个放 pkg/：证明菜单只数这一个目录底下的
		writeFile(t, filepath.Join(dir, "pkg/staged.go"), "package pkg\n")
		runGit(t, dir, "add", "pkg/staged.go")
	} else {
		writeFile(t, filepath.Join(dir, "internal/server/handler.go"), "package server\n\nfunc H() { /* 改了 */ }\n")
		writeFile(t, filepath.Join(dir, "internal/server/added.go"), "package server\n\nfunc A() {}\n")
		runGit(t, dir, "add", "internal/server")
	}
	// 根下另改一个：对着文件夹来一下不该动到目录外面的文件
	writeFile(t, filepath.Join(dir, "root.go"), "package main\n\nfunc main() {}\n")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()
	return a, tt
}

// dirApp 是「internal/ 底下两个未暂存」那一版。
func dirApp(t *testing.T) (*app, *ui.Tester) { return dirRepo(t, true) }

// stagedDirApp 是「internal/ 底下两个已暂存」那一版。
func stagedDirApp(t *testing.T) (*app, *ui.Tester) { return dirRepo(t, false) }

// 菜单要把「一次动几个文件」说在条目上，而且只数这一个目录底下的。
func TestDirMenuListsCounts(t *testing.T) {
	_, tt := dirApp(t)
	items := rightClick(t, tt, "internal/server")

	for _, w := range []string{
		"全部暂存（2 个文件）",
		"全部放弃变更…（2 个文件）",
		"复制路径",
		"在访达中显示",
	} {
		if !menuHas(items, w) {
			t.Errorf("菜单里没有 %q：%q", w, items)
		}
	}
	// 这一版里 internal/ 底下没有已暂存的东西，那条不该出现
	if menuHas(items, "全部取消暂存（1 个文件）") {
		t.Errorf("没有已暂存的文件，不该报取消暂存：%q", items)
	}
}

// 对着目录「全部暂存」：internal/ 底下两个一起进 index，目录外面的不动。
func TestDirMenuStagesEverythingUnder(t *testing.T) {
	a, tt := dirApp(t)
	rightClick(t, tt, "internal/server")
	if err := tt.ChooseMenuItem("全部暂存（2 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "internal/server/fresh.go\ninternal/server/handler.go\npkg/staged.go" {
		t.Errorf("index 里是 %q，want internal/ 底下两个加 pkg/staged.go", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); strings.Contains(got, "internal/") {
		t.Errorf("internal/ 底下还有未暂存改动：%q", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); !strings.Contains(got, "root.go") {
		t.Errorf("根下的 root.go 该还是未暂存：%q", got)
	}
}

// 对着目录「全部取消暂存」：已暂存的那些退回未暂存，未暂存的原样不动。
func TestDirMenuUnstagesEverythingUnder(t *testing.T) {
	a, tt := stagedDirApp(t)
	rightClick(t, tt, "internal/server")
	if err := tt.ChooseMenuItem("全部取消暂存（2 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("index 里还有 %q，want 空", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); !strings.Contains(got, "internal/server/handler.go") {
		t.Errorf("改过的那个该回到未暂存：%q", got)
	}
	// 新增的那个退回未跟踪（git reset 的语义就是这样，和单文件取消暂存一致）
	if got := gitOut(t, a.repoPath, "status", "--porcelain"); !strings.Contains(got, "?? internal/server/added.go") {
		t.Errorf("added.go 该变成未跟踪：%q", got)
	}
}

// 对着目录「全部放弃变更」要先问一句：未跟踪的那些会连文件一起删掉，
// 这句话必须在点确认之前读到。
func TestDirMenuDiscardAsksFirst(t *testing.T) {
	a, tt := dirApp(t)
	rightClick(t, tt, "internal/server")
	if err := tt.ChooseMenuItem("全部放弃变更…（2 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.confirm.on {
		t.Fatalf("丢弃前该先问一句")
	}
	if !strings.Contains(a.confirm.body, "删掉") {
		t.Errorf("确认框没说未跟踪的文件会删掉：%q", a.confirm.body)
	}
	// 还没确认：什么都不该动
	if got := gitOut(t, a.repoPath, "status", "--porcelain"); !strings.Contains(got, "internal/server/fresh.go") {
		t.Errorf("没确认就动了工作区：%q", got)
	}

	// 确认：改过的回到原样，未跟踪的从磁盘上拿走，目录外面的不动
	a.confirm.onYes()
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--name-only"); strings.Contains(got, "internal/") {
		t.Errorf("internal/ 底下还有未暂存改动：%q", got)
	}
	if got := gitOut(t, a.repoPath, "status", "--porcelain"); strings.Contains(got, "fresh.go") {
		t.Errorf("未跟踪的 fresh.go 该被删掉：%q", got)
	}
	if got := gitOut(t, a.repoPath, "status", "--porcelain"); !strings.Contains(got, "root.go") {
		t.Errorf("根下的 root.go 不该被卷进去：%q", got)
	}
	// 已暂存的那个不受影响：丢弃只动未暂存那一段
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "pkg/staged.go" {
		t.Errorf("pkg/ 底下已暂存的被卷进丢弃了：%q", got)
	}
}

// 历史 tab 里点提交看 diff：那边的文件夹是只读的，菜单里只剩读的几项。
func TestDirMenuReadOnlyInHistory(t *testing.T) {
	a, tt := dirApp(t)
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()

	items := rightClick(t, tt, "internal/server")
	if len(items) == 0 {
		t.Fatalf("历史里的目录也该有右键菜单")
	}
	for _, s := range items {
		if strings.Contains(s, "暂存") || strings.Contains(s, "丢弃") {
			t.Errorf("历史里的目录给了写操作 %q：%q", s, items)
		}
	}
}

// 目录里有一个重命名：git status 把它报成「旧路径被删」+「新路径未跟踪」，
// 两条都得给 git，否则暂存完是一半重命名一半还没。
func TestDirMenuStagesRenameCompletely(t *testing.T) {
	a, tt := dirApp(t)
	// git mv 会顺手把改名放进 index，那不是我们要测的状态 —— 撤出来，让
	// 工作区里躺着一次「还没暂存的改名」
	runGit(t, a.repoPath, "mv", "internal/server/handler.go", "internal/server/handler2.go")
	runGit(t, a.repoPath, "reset", "-q")
	a.reload()
	settle(t, a)
	tt.Frame()

	items := rightClick(t, tt, "internal/server")
	if !menuHas(items, "全部暂存（3 个文件）") {
		t.Fatalf("菜单里没有「全部暂存（3 个文件）」：%q", items)
	}
	if err := tt.ChooseMenuItem("全部暂存（3 个文件）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	// 三条路径都进了 index（改名的新旧两条 + 那个新增）。用 --no-renames 数：
	// 开着rename检测时 git 把删掉的旧路径和某个新增配成一对，--name-only
	// 就只报新路径那一个，「旧路径在不在这一批里」正好被它藏起来。
	//
	// 而旧路径漏掉正是这里要防的事：只给新路径的话，index 里剩下半截删除，
	// 用户得再点一次才真的变成改名。git 把旧路径和哪个新增配对是它自己的
	// 相似度判断，不归我们管。
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--no-renames", "--name-only"); got !=
		"internal/server/fresh.go\ninternal/server/handler.go\ninternal/server/handler2.go" {
		t.Errorf("index 里是 %q，want 改名的新旧两条加那个新增", got)
	}
	// 工作区里不该再留着没暂存的改动
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); strings.Contains(got, "internal/") {
		t.Errorf("internal/ 底下还有未暂存改动：%q", got)
	}
}
