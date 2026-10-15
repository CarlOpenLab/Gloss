package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/egoist/mygo/ui"
)

// workspaceApp 建一个「有干净文件、也有改动」的仓库：编辑器那一栏列的是整个
// 工作区，测试里必须真的有没动过的文件，才断言得出「不只有改动」这件事。
func workspaceApp(t *testing.T) *app {
	t.Helper()
	dir := filepath.Join(t.TempDir())
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(dir, "README.md"), "# 说明\n\n没动过的文件\n")
	writeFile(t, filepath.Join(dir, "internal/clean/untouched.go"), "package clean\n\n// 没动过\n")
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "ignored.log"), "不该出现的\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// 改一个（已在索引里），再加一个（未跟踪）—— 这两个才是「改动」。
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(dir, "new.go"), "package main\n\n// 新的\n")
	return loadApp(t, dir)
}

// 两个列表各记各的：编辑器栏那份来自 git ls-files（整个工作区），改动栏那份
// 来自状态。切换视图不该把一份列表的筛选词或选中带到另一份上去。
func TestWorkspaceListAndChangedListAreSeparate(t *testing.T) {
	a := workspaceApp(t)
	settle(t, a)

	ws := workspacePaths(a.wsAll)
	for _, want := range []string{"README.md", "a.go", "internal/clean/untouched.go", "new.go"} {
		if !containsString(ws, want) {
			t.Fatalf("工作区列表里少了 %s：%v", want, ws)
		}
	}
	if containsString(ws, "ignored.log") {
		t.Fatalf("被 .gitignore 忽略的文件不该出现在工作区列表里：%v", ws)
	}
	if got := len(a.files); got != 2 {
		t.Fatalf("改动栏该只有 2 个改动的文件，实际 %d 个：%v", got, workspacePaths(a.files))
	}
}

// 编辑器视图的左栏是整份工作区：没改过的文件也在，并且点得开 —— 它读的是
// 工作区里那份内容。改动栏仍然是「只有改动」。
func TestEditorViewListsWholeWorkspace(t *testing.T) {
	a := workspaceApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	for _, want := range []string{"README.md", "untouched.go"} {
		if !tt.HasText(want) {
			t.Fatalf("编辑器栏里没有列出 %s（整份工作区都该列出来）", want)
		}
	}
	// README.md、a.go、new.go、internal/clean/untouched.go，加上 .gitignore
	// —— 计数报的是整份工作区，不是改动数。
	if !tt.HasText("5 个文件") {
		t.Fatalf("编辑器栏的计数该是整份工作区（5 个文件），实际画面里没有")
	}
	// 底栏报的是「工作区里改过几个」—— 从改动列表切过来时丢掉的就是这条。
	if !tt.HasText("已改动") {
		t.Fatalf("编辑器栏底栏该报出改过的文件数")
	}

	a.setViewMode(viewModeGit)
	tt.Frame()
	if tt.HasText("untouched.go") {
		t.Fatalf("改动栏不该列出没改过的文件")
	}
}

// 选一个没改过的文件，编辑器照样把它读进来 —— 这是「编辑器看整个工作区」
// 的意义所在。
func TestEditorViewOpensUnchangedFile(t *testing.T) {
	a := workspaceApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	selectEditorFile(t, a, "internal/clean/untouched.go")
	settle(t, a)

	if a.editorPane.path != "internal/clean/untouched.go" {
		t.Fatalf("编辑器打开的是 %q", a.editorPane.path)
	}
	if a.editorPane.buffer == nil {
		t.Fatalf("编辑器没读到文件：%q", a.editorPane.err)
	}
	if got := a.editorPane.buffer.String(); !strings.Contains(got, "没动过") {
		t.Fatalf("编辑器里的内容不对：%q", got)
	}
	// 没改动 ⇒ 行号边上不该冒改动标记：基线要么没有，要么与正文一模一样。
	if base := a.editorPane.code.DiffBase; base != nil {
		if base.String() != a.editorPane.buffer.String() {
			t.Fatalf("没改过的文件不该带改动标记：基线与正文对不上")
		}
	}
}

// 编辑器栏的筛选只筛那一栏：改动栏的列表和它自己的筛选词都不受影响。
func TestWorkspaceFilterStaysInItsOwnPane(t *testing.T) {
	a := workspaceApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	changed := len(a.files)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Click("筛选工作区文件")
	tt.Type("untouched")
	a.wsFilter = "untouched" // 输入框在测试里不一定逐字回调，直接落状态
	a.applyWsFilter()

	if got := workspacePaths(a.wsFiles); len(got) != 1 || got[0] != "internal/clean/untouched.go" {
		t.Fatalf("筛选之后编辑器栏该只剩 1 个文件，实际 %v", got)
	}
	if len(a.files) != changed {
		t.Fatalf("改动栏不该被编辑器栏的筛选词筛掉：%d → %d", changed, len(a.files))
	}
	if a.fileFilter != "" {
		t.Fatalf("编辑器栏的筛选词串到改动栏上了：%q", a.fileFilter)
	}

	a.setViewMode(viewModeGit)
	tt.Frame()
	tt.HasText("a.go")
	if !containsString(workspacePaths(a.files), "a.go") {
		t.Fatalf("切回改动栏后改动列表该还在：%v", workspacePaths(a.files))
	}
}

// 列表重建之后选中的文件按路径找回来，不能因为下标变了就指到别的文件上。
func TestWorkspaceSelectionSurvivesReload(t *testing.T) {
	a := workspaceApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	selectEditorFile(t, a, "new.go")
	settle(t, a)

	// 加一个排在前面的文件，把下标整体顶后一位。
	writeFile(t, filepath.Join(a.repoPath, "0.go"), "package main\n")
	repo, err := gitrepo.Open(context.Background(), a.repoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a.applyModel(m)
	settle(t, a)

	if got := a.selectedWorkspaceKey(); got != "new.go" {
		t.Fatalf("重载之后选中的该还是 new.go，实际 %q", got)
	}
}

// workspaceFiles 是纯函数，几条合入规则单独测：工作区里没改的文件要进来，
// 改过的沿用状态那份，同一路径两段只留一行（留未暂存那份），索引里删掉的
// 文件也要留住。
func TestWorkspaceFilesMergeRules(t *testing.T) {
	m := &gitrepo.Model{
		Files: []gitrepo.File{
			{
				FileDiff: diff.FileDiff{NewPath: "both.go"},
				Status:   gitrepo.StatusAdded,
				Section:  gitrepo.SectionStaged,
				Staged:   true,
			},
			{
				FileDiff: diff.FileDiff{NewPath: "both.go", AddCount: 2},
				Status:   gitrepo.StatusModified,
				Section:  gitrepo.SectionUnstaged,
				Unstaged: true,
			},
			{
				FileDiff: diff.FileDiff{OldPath: "gone.go"},
				Status:   gitrepo.StatusDeleted,
				Section:  gitrepo.SectionStaged,
				Staged:   true,
			},
		},
		Worktree: []gitrepo.WorktreeFile{
			{Path: "clean.go"},
			{Path: "both.go"},
			{Path: "sub/dir.go"},
		},
	}

	got := workspaceFiles(m)
	if want := []string{"both.go", "clean.go", "gone.go", "sub/dir.go"}; !sameStrings(workspacePaths(got), want) {
		t.Fatalf("合出来的列表不对：%v，想要 %v", workspacePaths(got), want)
	}

	byPath := map[string]gitrepo.File{}
	for _, f := range got {
		byPath[pathOf(f)] = f
	}
	if f := byPath["clean.go"]; f.Status != "" || f.Section != gitrepo.SectionNone {
		t.Fatalf("没改过的文件不该带状态：%+v", f)
	}
	if f := byPath["both.go"]; !f.Unstaged {
		t.Fatalf("同一路径该留未暂存那一份：%+v", f)
	}
	if f := byPath["gone.go"]; f.Status != gitrepo.StatusDeleted {
		t.Fatalf("索引里删掉的文件也该留在列表里：%+v", f)
	}
}

func workspacePaths(files []gitrepo.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, pathOf(f))
	}
	return out
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
