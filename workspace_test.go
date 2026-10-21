package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/filetree"
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
	// 根一层的文件和目录都在。
	for _, want := range []string{"README.md", "a.go", "new.go", "internal/clean"} {
		if !tt.HasText(want) {
			t.Fatalf("编辑器栏里没有列出 %s（整份工作区都该列出来）", want)
		}
	}
	// 但树默认全收起：子目录里的文件先不露面，免得一进编辑器就被一大片
	// 文件糊住。想看就点开。
	if tt.HasText("untouched.go") {
		t.Fatalf("目录默认收起，untouched.go 不该先露出来")
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

	// 点开那条目录链，文件就露出来了。（internal/clean 只有一个子项，
	// 按 filetree 的规矩合并成一行。）
	tt.Click("internal/clean")
	tt.Frame()
	if !tt.HasText("untouched.go") {
		t.Fatalf("展开 internal/clean 之后该看到 untouched.go")
	}

	a.setViewMode(viewModeGit)
	tt.Frame()
	if tt.HasText("untouched.go") {
		t.Fatalf("改动栏不该列出没改过的文件")
	}
}

// 工作区树把每个文件的状态带进来（treeRowIn 据此上色）：改过的 a.go 是 M，
// new.go 是新增的未跟踪文件，没动过的不带状态 —— 于是「有色」本身成为信息。
func TestWorkspaceTreeCarriesStatusForColor(t *testing.T) {
	a := workspaceApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	status := map[string]gitrepo.Status{}
	for _, f := range a.wsFiles {
		status[pathOf(f)] = f.Status
	}
	if status["a.go"] != gitrepo.StatusModified {
		t.Fatalf("a.go 该带 M，实际 %q", status["a.go"])
	}
	if status["new.go"] != gitrepo.StatusUntracked {
		t.Fatalf("new.go 该是未跟踪，实际 %q", status["new.go"])
	}
	if status["README.md"] != "" || status["internal/clean/untouched.go"] != "" {
		t.Fatalf("没动过的文件不该带状态")
	}
}

// 编辑器栏的文件树只画名字：没有状态字母、没有增删行数，改动过的文件按
// 状态给名字上色（VSCode 那种）。
//
// 那一栏的用途是「打开仓库里某个文件看一眼」，一列里绝大多数文件没动过 ——
// 每个改动过的都挂一枚字母加两个数字，扫下来全是噪声。改动栏反过来，字母和
// 增删行数正是那里要读的东西（见 rowSpec.plain）。
func TestWorkspaceTreeRowsArePlain(t *testing.T) {
	a := workspaceApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 名字都在
	for _, want := range []string{"a.go", "new.go", "README.md"} {
		if !tt.HasText(want) {
			t.Errorf("编辑器栏里找不到 %q：%q", want, tt.Texts())
		}
	}
	// 字母和增删行数不在。用精确匹配数：HasText 是子串匹配，"M" 会命中
	// 任何含 M 的文本。
	texts := tt.Texts()
	if n := countText(texts, "M"); n != 0 {
		t.Errorf("编辑器栏不该画状态字母，实际有 %d 个 %q：%q", n, "M", texts)
	}
	for _, c := range []string{"+1", "−1"} {
		if n := countText(texts, c); n != 0 {
			t.Errorf("编辑器栏不该画增删行数，实际有 %d 个 %q：%q", n, c, texts)
		}
	}
	// 名字前头也不该有暂存勾选框（它的无障碍名字就是「暂存」）。一列文件里
	// 绝大多数没动过，摆一排会改 index 的方框在那儿，随手点一行就可能暂存
	// 一个文件；要暂存去改动栏或右键菜单。
	if n := countText(texts, T("暂存", "Stage")); n != 0 {
		t.Errorf("编辑器栏不该摆暂存勾选框，实际有 %d 个：%q", n, texts)
	}

	// 上色：a.go 改过是琥珀（暖），new.go 未跟踪是绿，README.md 没动过是中性灰。
	// 三种都验到才算「按改动上色」在画 —— 只验改过的那一个的话，全屏一个色
	// 也能蒙混过关。取这一行里最深的像素：字形边缘带 LCD 次像素抗锯齿，
	// 直接扫整行会把灰字边缘的彩色条纹误当成文字底色。
	hue := func(name string) (warm, green bool) {
		box, ok := tt.Find(name)
		if !ok {
			t.Fatalf("找不到 %q 好取色", name)
		}
		y := int(box.Y + box.H/2)
		core := [3]uint8{255, 255, 255}
		coreSum := 3 * 255
		for x := int(box.X); x < int(box.X+box.W); x++ {
			p := pixelAt(tt, x, y)
			if sum := int(p[0]) + int(p[1]) + int(p[2]); sum < coreSum {
				core, coreSum = p, sum
			}
		}
		warm = int(core[0]) > int(core[2])+20
		green = int(core[1]) > int(core[0])+20 && int(core[1]) > int(core[2])+20
		return warm, green
	}
	if warm, _ := hue("a.go"); !warm {
		t.Error("改过的文件该是暖色（琥珀）")
	}
	if _, green := hue("new.go"); !green {
		t.Error("新增的文件该是绿的")
	}
	if warm, green := hue("README.md"); warm || green {
		t.Errorf("没动过的文件不该带色 warm=%v green=%v", warm, green)
	}

	// 对照：改动栏照旧摆勾选框 —— 那里列的全是改动，勾选框正是要用的东西。
	// 没有这一条，上面那条可能只是因为整个勾选框坏了。
	a.setViewMode(viewModeGit)
	settle(t, a)
	tt.Frame()
	if n := countText(tt.Texts(), T("暂存", "Stage")); n == 0 {
		t.Errorf("改动栏照旧该有暂存勾选框：%q", tt.Texts())
	}
}

// 目录行取子树里最重的那一档上色：冲突 > 删除 > 修改 > 新增，且只含没动过
// 的文件的目录不上色 —— 不然目录链会一路绿到底，把「这儿动过」指错地方。
func TestDirChangeStatusPicksUrgentChild(t *testing.T) {
	mk := func(p string, st gitrepo.Status) gitrepo.File {
		return gitrepo.File{FileDiff: diff.FileDiff{NewPath: p}, Status: st}
	}
	files := []gitrepo.File{
		mk("x/mod.go", gitrepo.StatusModified),
		mk("x/new.go", gitrepo.StatusUntracked),
		mk("x/clean.go", ""),
	}
	tree := filetree.Build(files)
	var find func(ns []*filetree.Node, key string) *filetree.Node
	find = func(ns []*filetree.Node, key string) *filetree.Node {
		for _, n := range ns {
			if n.Key == key {
				return n
			}
			if got := find(n.Children, key); got != nil {
				return got
			}
		}
		return nil
	}
	dir := find(tree, "d:x")
	if dir == nil {
		t.Fatalf("该有目录 x")
	}
	if s, ok := dirChangeStatus(dir, files); !ok || s != gitrepo.StatusModified {
		t.Fatalf("x 里有 M 和未跟踪，该报最重的 M，实际 %q/%v", s, ok)
	}

	cleanFiles := []gitrepo.File{mk("y/clean.go", "")}
	if s, ok := dirChangeStatus(find(filetree.Build(cleanFiles), "d:y"), cleanFiles); ok {
		t.Fatalf("y 里只有没动过的文件，不该有色：%q", s)
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
