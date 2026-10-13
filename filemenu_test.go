package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// menuApp 是一个「什么状态都有」的仓库：改过的文件、暂存过的文件、删掉的
// 文件、改名的文件、没跟踪过的新文件 —— 文件行菜单要按状态长出不同的项，
// 一份仓库里全摆齐，各条用例各取所需。
func menuApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(dir, "mod.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "gone.go"), "package gone\n")
	writeFile(t, filepath.Join(dir, "old.go"), "package old\n")
	writeFile(t, filepath.Join(dir, "staged.go"), "package staged\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// 改了内容（未暂存）
	writeFile(t, filepath.Join(dir, "mod.go"), "package main\n\nfunc main() {}\n")
	// 删掉了（未暂存）
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}
	// 改了名（未暂存）：git 要到 status 时才认出来，工作区里就是旧名没了、
	// 新名是新的
	if err := os.Rename(filepath.Join(dir, "old.go"), filepath.Join(dir, "new.go")); err != nil {
		t.Fatal(err)
	}
	// 暂存了（并且工作区没再改）
	writeFile(t, filepath.Join(dir, "staged.go"), "package staged\n\n// staged change\n")
	runGit(t, dir, "add", "staged.go")
	// 没跟踪过的新文件
	writeFile(t, filepath.Join(dir, "fresh.go"), "package fresh\n")

	return loadApp(t, dir)
}

func TestFileMenuItemsBySection(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 未暂存、改了内容：暂存 + 丢弃 + 复制 + 历史 + 已查看，没有 .gitignore
	items := rightClick(t, tt, "mod.go")
	for _, want := range []string{
		"暂存 mod.go", "丢弃更改…", "在编辑器打开", "在访达中显示",
		"复制路径", "复制相对路径", "复制文件名", "查看文件历史…", "标记为已查看",
	} {
		if !menuHas(items, want) {
			t.Errorf("未暂存文件的菜单里该有 %q，实际 %v", want, items)
		}
	}
	if menuHas(items, "加入 .gitignore") {
		t.Errorf("已跟踪的文件不该有「加入 .gitignore」：%v", items)
	}
	tt.CloseMenu()

	// 已暂存：取消暂存 + 查看历史，不给丢弃（那一段反悔走的是取消暂存）
	items = rightClick(t, tt, "staged.go")
	if !menuHas(items, "取消暂存 staged.go") {
		t.Errorf("已暂存文件的菜单里该有「取消暂存 staged.go」，实际 %v", items)
	}
	if menuHas(items, "丢弃更改…") {
		t.Errorf("已暂存的文件不该有「丢弃更改…」（那是未暂存段的事）：%v", items)
	}
	tt.CloseMenu()

	// 未跟踪：删除文件 + 加入 .gitignore，历史灰着
	items = rightClick(t, tt, "fresh.go")
	if !menuHas(items, "删除这个文件…") {
		t.Errorf("未跟踪文件的菜单里该有「删除这个文件…」，实际 %v", items)
	}
	if !menuHas(items, "加入 .gitignore") {
		t.Errorf("未跟踪文件的菜单里该有「加入 .gitignore」，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("查看文件历史…"); err == nil {
		t.Error("未跟踪的文件没有历史，那一项应该是灰的")
	}
	if err := tt.ChooseMenuItem("删除这个文件…"); err != nil {
		t.Fatalf("「删除这个文件…」不该是灰的：%v", err)
	}
	tt.Frame()
	if !a.confirm.on {
		t.Fatal("丢弃/删除之前该弹一次确认框")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("点取消: %v", err)
	}
	tt.Frame()
	if a.confirm.on {
		t.Error("取消之后确认框该关掉")
	}
	if _, err := os.Stat(filepath.Join(a.repoPath, "fresh.go")); err != nil {
		t.Errorf("取消了就不该动文件：%v", err)
	}
}

// 丢弃未暂存的改动：文件恢复成 index 里那份。
func TestDiscardUnstagedChanges(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "mod.go")
	if err := tt.ChooseMenuItem("丢弃更改…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.confirm.on {
		t.Fatal("丢弃改动该先问一句")
	}
	if err := tt.Click("丢弃更改"); err != nil {
		t.Fatalf("点确认: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, a.repoPath, "mod.go"); got != "package main\n" {
		t.Errorf("丢弃之后该回到上次暂存的样子，实际 %q", got)
	}
	if a.confirm.on {
		t.Error("确认之后对话框该关掉")
	}
	if hasFileRow(a, "mod.go") {
		t.Errorf("丢弃之后列表里不该还有 mod.go：%v", fileListOf(a))
	}
}

// 丢弃删除：文件被恢复回来。
func TestDiscardRestoresDeletedFile(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "gone.go")
	if err := tt.ChooseMenuItem("丢弃更改…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("丢弃更改"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, a.repoPath, "gone.go"); got != "package gone\n" {
		t.Errorf("删掉的文件该被恢复回来，实际 %q", got)
	}
}

// 工作区里改了名：git 把它报成「旧路径被删」+「新路径是未跟踪」两行，
// 各按自己的类型丢 —— 两个都丢完才是「改名前」的样子。
func TestDiscardWorktreeRename(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 旧路径那一行：恢复回来
	r, ok := tt.Find("old.go")
	if !ok {
		t.Fatalf("列表里该有 old.go（被删了）：%v", fileListOf(a))
	}
	if !hasFileRow(a, "new.go") {
		t.Fatalf("列表里该有 new.go（未跟踪）：%v", fileListOf(a))
	}
	tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	if err := tt.ChooseMenuItem("丢弃更改…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("丢弃更改"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, a.repoPath, "old.go"); got != "package old\n" {
		t.Errorf("旧名字的文件该回来，实际 %q", got)
	}

	// 新路径那一行：当成未跟踪文件删掉
	items := rightClick(t, tt, "new.go")
	if !menuHas(items, "删除这个文件…") {
		t.Fatalf("改名之后的新路径在 git 眼里就是未跟踪，菜单该给「删除这个文件…」，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("删除这个文件…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("删除文件"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if _, err := os.Stat(filepath.Join(a.repoPath, "new.go")); !os.IsNotExist(err) {
		t.Errorf("新名字的文件该被删掉：%v", err)
	}
	if out := gitOut(t, a.repoPath, "status", "--porcelain"); strings.Contains(out, "old.go") || strings.Contains(out, "new.go") {
		// 这个仓库里别的文件还脏着，只看改名那一对：它们该从 status 里消失
		t.Errorf("改名那一对都丢完之后 status 里不该还有它们，实际：%q", out)
	}
}

// 丢弃未跟踪：git 不认识它，「丢弃」就是从磁盘上删掉。
func TestDiscardUntrackedDeletesFile(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "fresh.go")
	if err := tt.ChooseMenuItem("删除这个文件…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("从磁盘上删掉") {
		t.Errorf("删未跟踪文件该说清是真的删：%q", tt.Texts())
	}
	if err := tt.Click("删除文件"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if _, err := os.Stat(filepath.Join(a.repoPath, "fresh.go")); !os.IsNotExist(err) {
		t.Errorf("未跟踪的文件该被删掉：%v", err)
	}
	if hasFileRow(a, "fresh.go") {
		t.Errorf("删掉之后列表里不该还有它：%v", fileListOf(a))
	}
}

// 加入 .gitignore：写进仓库根的 .gitignore，路径带前导斜杠，同一行不重复。
func TestAddToGitignore(t *testing.T) {
	a := menuApp(t)
	writeFile(t, filepath.Join(a.repoPath, ".gitignore"), "*.log\n")
	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "fresh.go")
	if err := tt.ChooseMenuItem("加入 .gitignore"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	got := fileText(t, a.repoPath, ".gitignore")
	if got != "*.log\n/fresh.go\n" {
		t.Errorf(".gitignore 该多出一行 /fresh.go，实际 %q", got)
	}
	if hasFileRow(a, "fresh.go") {
		t.Errorf("忽略之后 fresh.go 不该还在列表里：%v", fileListOf(a))
	}

	// 再加一次：同一行不重复写
	a.addToGitignore(gitrepo.File{
		FileDiff: diff.FileDiff{NewPath: "fresh.go"},
		Status:   gitrepo.StatusUntracked,
		Section:  gitrepo.SectionUnstaged,
	})
	settle(t, a)
	if got := fileText(t, a.repoPath, ".gitignore"); got != "*.log\n/fresh.go\n" {
		t.Errorf("同一行不该写第二遍，实际 %q", got)
	}
}

// 复制三项：路径、相对路径、文件名。
func TestFileMenuCopyPaths(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	cases := []struct {
		item string
		want string
	}{
		{"复制路径", filepath.Join(a.repoPath, "mod.go")},
		{"复制相对路径", "mod.go"},
		{"复制文件名", "mod.go"},
	}
	for _, tc := range cases {
		rightClick(t, tt, "mod.go")
		if err := tt.ChooseMenuItem(tc.item); err != nil {
			t.Fatalf("%s: %v", tc.item, err)
		}
		tt.Frame()
		tt.Frame()
		if got := tt.Clipboard(); got != tc.want {
			t.Errorf("%s 写进剪贴板的该是 %q，实际 %q", tc.item, tc.want, got)
		}
	}
}

// 「在访达中显示」交给注入的揭示器，拿到的是仓库里的绝对路径。
func TestRevealInFinderReceivesAbsPath(t *testing.T) {
	a := menuApp(t)
	var got string
	a.revealPath = func(p string) { got = p }

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "mod.go")
	if err := tt.ChooseMenuItem("在访达中显示"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)

	if want := filepath.Join(a.repoPath, "mod.go"); got != want {
		t.Errorf("揭示器收到 %q，want %q", got, want)
	}
}

// 标记为已查看：菜单项跟着状态换名字，点一下就把文件标灰。
func TestFileMenuTogglesViewed(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	items := rightClick(t, tt, "mod.go")
	if !menuHas(items, "标记为已查看") || menuHas(items, "取消已查看") {
		t.Fatalf("没标记过的文件该显示「标记为已查看」，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("标记为已查看"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	f := fileIn(t, a, gitrepo.SectionUnstaged, "mod.go")
	if !a.isViewed(f) {
		t.Fatal("点过之后该是已查看")
	}
	items = rightClick(t, tt, "mod.go")
	if !menuHas(items, "取消已查看") {
		t.Errorf("标记之后该显示「取消已查看」，实际 %v", items)
	}
}

// 历史 tab 里的文件行是只读的（SectionNone）：菜单里没有暂存/丢弃这些
// 会动工作区的项，读的那几项还在。
func TestFileMenuOnHistoryRowsIsReadOnly(t *testing.T) {
	a := menuApp(t)
	runGit(t, a.repoPath, "add", ".")
	runGit(t, a.repoPath, "commit", "-m", "second")
	a.reload()
	a.setTab(1)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if len(a.files) == 0 {
		t.Fatal("历史 tab 里没选出提交的文件")
	}
	name := pathOf(a.files[0])
	items := rightClick(t, tt, name)
	for _, no := range []string{"丢弃更改…", "删除这个文件…", "加入 .gitignore"} {
		if menuHas(items, no) {
			t.Errorf("历史里的文件行不该有 %q：%v", no, items)
		}
	}
	if !menuHas(items, "复制相对路径") || !menuHas(items, "查看文件历史…") {
		t.Errorf("读的那几项该还在，实际 %v", items)
	}
	for _, it := range items {
		if strings.HasPrefix(it, "暂存 ") || strings.HasPrefix(it, "取消暂存 ") || strings.HasPrefix(it, "标记为已解决 ") {
			t.Errorf("历史里的文件行不该有 %q：%v", it, items)
		}
	}
}

// hasFileRow 看当前文件列表里还有没有这条路径。
func hasFileRow(a *app, path string) bool {
	for _, f := range a.files {
		if pathOf(f) == path {
			return true
		}
	}
	return false
}
