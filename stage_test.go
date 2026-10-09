package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 暂存、提交这条路上的界面行为：勾选框、hunk 上的暂存按钮、提交栏。
//
// 验收一律回到 git 自己身上（gitOut）—— 拿我们自己的 Load 去验我们自己的
// 写，两边一起错的时候测试还是绿的，而那正是最需要它红的时候。

// gitOut 跑一条 git 命令并返回去掉首尾空白后的 stdout。
//
// 和 runGit 分开：那个是「做一件事，失败就 Fatal」，这个是「问一句」，
// 断言要拿它的返回值。
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// dirtyRepoDir 建一个最小仓库：一个提交、一个改过的文件。返回仓库目录。
func dirtyRepoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-m", "init")
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n\nfunc main() {}\n")
	return dir
}

// clickStageBox 点文件行左边那颗暂存勾选框。
//
// 不按文字点：勾选框自己不带文字（画的是方框和勾），而 diff 里 hunk 头上
// 的按钮写的也是「暂存」—— 按文字点命中的会是哪一个说不准。所以先按文件名
// 找到那一行的名字，再往左偏到方框上：名字左边只有 6px 间距和 14px 的方框，
// 中心落在名字左边 13px 处。布局要是动了，测试会在这里红，而不是悄悄点到
// 别处去。
func clickStageBox(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("文件列表里找不到 %q", name)
	}
	tt.ClickAt(r.X-13, r.Y+r.H/2)
	tt.Frame()
}

// halfApp 是一个「一半暂存」的仓库：README.md 进了 index，之后工作区又改了
// 一次 —— 于是同一个路径在「已暂存」和「未暂存」两段里各有一份。
func halfApp(t *testing.T) *app {
	t.Helper()
	a := richApp(t)
	runGit(t, a.repoPath, "add", "README.md")
	writeFile(t, filepath.Join(a.repoPath, "README.md"),
		"# diff explainer\n\n新的一行\n多了第二行\n又加了一行\n")
	a.reload()
	settle(t, a)
	return a
}

// fileIn 按段落找出列表里的某一个文件。找不到就 Fatal。
func fileIn(t *testing.T, a *app, section gitrepo.Section, path string) gitrepo.File {
	t.Helper()
	for _, f := range a.files {
		if f.Section == section && pathOf(f) == path {
			return f
		}
	}
	t.Fatalf("%s 段里没有 %q（当前：%v）", sectionName(section), path, fileListOf(a))
	return gitrepo.File{}
}

func sectionName(s gitrepo.Section) string {
	if s == gitrepo.SectionStaged {
		return "已暂存"
	}
	return "未暂存"
}

func fileListOf(a *app) []string {
	var out []string
	for _, f := range a.files {
		out = append(out, sectionName(f.Section)+":"+pathOf(f))
	}
	return out
}

// 点勾选框 = 把文件放进 index。之后界面自己也得分段。
func TestStageBoxStagesFile(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	clickStageBox(t, tt, "README.md")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "README.md" {
		t.Errorf("index 里是 %q, want README.md", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--name-only"); strings.Contains(got, "README.md") {
		t.Errorf("README.md 整份进 index 了，工作区不该还有它：%q", got)
	}

	// 界面：两段都出来了，README.md 落在「已暂存」那一段
	if !tt.HasText("已暂存") || !tt.HasText("未暂存") {
		t.Errorf("两段都该在界面上，实际：%q", tt.Texts())
	}
	if f := fileIn(t, a, gitrepo.SectionStaged, "README.md"); f.Status != gitrepo.StatusModified {
		t.Errorf("README.md 在已暂存段里该是修改，实际 %v", f.Status)
	}

	// 再点一次 = 拿出来
	clickStageBox(t, tt, "README.md")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("取消暂存后 index 该是空的，实际 %q", got)
	}
	if tt.HasText("已暂存") {
		t.Errorf("没有东西暂存了，不该还摆着「已暂存」段：%q", tt.Texts())
	}
}

// 删掉的文件点勾选框能放进 index，再点一次能拿出来。
func TestStageBoxStagesDeletedFile(t *testing.T) {
	a := richApp(t)
	// 在工作区删掉已提交的 a.go / README.md
	if err := os.Remove(filepath.Join(a.repoPath, "README.md")); err != nil {
		t.Fatal(err)
	}
	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	clickStageBox(t, tt, "README.md")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); !strings.Contains(got, "README.md") {
		t.Errorf("index 里该有 README.md，实际: %q", got)
	}
	if f := fileIn(t, a, gitrepo.SectionStaged, "README.md"); f.Status != gitrepo.StatusDeleted {
		t.Errorf("README.md 在已暂存段里该是删除，实际 %v", f.Status)
	}

	// 再点一次取消暂存
	clickStageBox(t, tt, "README.md")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); strings.Contains(got, "README.md") {
		t.Errorf("取消暂存后 index 不该有 README.md，实际: %q", got)
	}
}

// 点 hunk 上的「暂存这一段」只把那一段放进 index，同一个文件别的段留在
// 工作区 —— 这是 hunk 级暂存的全部意义。
func TestStageHunkStagesOnlyThatHunk(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	// 只看 handler.go：它是唯一有两段改动的文件，第一颗「暂存这一段」
	// 就落在它的第一段上。
	a.fileFilter = "handler"
	tt.Frame()

	if err := tt.Click("暂存这一段"); err != nil {
		t.Fatalf("hunk 头上该有暂存按钮：%v", err)
	}
	settle(t, a)
	tt.Frame()

	cached := gitOut(t, a.repoPath, "diff", "--cached")
	if n := strings.Count(cached, "@@ "); n != 1 {
		t.Errorf("index 里该只有一段改动，实际 %d 段：\n%s", n, cached)
	}
	if !strings.Contains(cached, "line 3: CHANGED AGAIN") {
		t.Errorf("第一段该进 index 了：\n%s", cached)
	}
	if strings.Contains(cached, "line 52: 也改了") {
		t.Errorf("第二段不该跟着进 index：\n%s", cached)
	}

	work := gitOut(t, a.repoPath, "diff")
	if !strings.Contains(work, "line 52: 也改了") {
		t.Errorf("第二段该还在工作区：\n%s", work)
	}
	if strings.Contains(work, "line 3: CHANGED AGAIN") {
		t.Errorf("第一段已经在 index 里了，工作区不该再有：\n%s", work)
	}

	// 界面：同一个文件现在两段里各有一份，各剩一段
	if f := fileIn(t, a, gitrepo.SectionStaged, "internal/server/handler.go"); len(f.Hunks) != 1 {
		t.Errorf("已暂存那份该只剩一段，实际 %d 段", len(f.Hunks))
	}
	if f := fileIn(t, a, gitrepo.SectionUnstaged, "internal/server/handler.go"); len(f.Hunks) != 1 {
		t.Errorf("未暂存那份该只剩一段，实际 %d 段", len(f.Hunks))
	}
}

// 取消暂存一个 hunk：反向把 index 里那一段拿回工作区。
func TestUnstageHunkFromSectionButton(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "internal/server/handler.go")
	a.reload()
	settle(t, a)
	tt := ui.NewTester(a.view, 1280, 860)
	a.fileFilter = "handler"
	tt.Frame()

	// 两段都在 index 里，第一颗按钮是「取消暂存这一段」
	if err := tt.Click("取消暂存这一段"); err != nil {
		t.Fatalf("已暂存段的 hunk 头上该有取消暂存按钮：%v", err)
	}
	settle(t, a)
	tt.Frame()

	cached := gitOut(t, a.repoPath, "diff", "--cached")
	if strings.Contains(cached, "line 3: CHANGED AGAIN") {
		t.Errorf("第一段该撤出 index 了：\n%s", cached)
	}
	if !strings.Contains(cached, "line 52: 也改了") {
		t.Errorf("第二段还在 index 里，不该被连坐：\n%s", cached)
	}
	if work := gitOut(t, a.repoPath, "diff"); !strings.Contains(work, "line 3: CHANGED AGAIN") {
		t.Errorf("撤出来的那一段该回到工作区：\n%s", work)
	}
}

// 提交按钮：把 index 里的东西提交成一次提交，消息从输入框来。
func TestCommitButtonCommitsStagedFiles(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 打字进输入框：点 Label 拿到焦点，再打 —— 顺便验证了焦点真的落在
	// 提交框上（字没进别的框）。
	if err := tt.Click("提交信息"); err != nil {
		t.Fatalf("提交框该在界面上：%v", err)
	}
	tt.Type("把 README 的措辞改一下")
	tt.Frame()
	if a.commitMsg != "把 README 的措辞改一下" {
		t.Fatalf("敲的字没进提交框，commitMsg = %q", a.commitMsg)
	}

	if err := tt.Click("提交 1 个文件"); err != nil {
		t.Fatalf("该有一颗能用的提交按钮：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "log", "-1", "--format=%s"); got != "把 README 的措辞改一下" {
		t.Errorf("提交信息 = %q", got)
	}
	if got := gitOut(t, a.repoPath, "show", "--name-only", "--format=", "HEAD"); got != "README.md" {
		t.Errorf("这次提交该只含 README.md，实际 %q", got)
	}
	if a.opErr != nil {
		t.Errorf("提交成功了却报错：%+v", *a.opErr)
	}
	if a.commitMsg != "" {
		t.Errorf("提交成功后输入框该清空，实际 %q", a.commitMsg)
	}
	if n := len(a.files); n != 2 {
		t.Errorf("提交后只剩两个未暂存文件，实际 %d：%v", n, fileListOf(a))
	}
}

// ⌘⏎ 提交 —— 输入框里回车是换行，所以提交走的是带 ⌘ 的这一路。
func TestCommitWithCmdEnter(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Click("提交信息")
	tt.Type("走键盘提交")
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyEnter)
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "log", "-1", "--format=%s"); got != "走键盘提交" {
		t.Errorf("⌘⏎ 该提交，log 里是 %q", got)
	}
}

// 没有暂存的东西时提交不了：按钮灰着，⌘⏎ 也不该真发出一次注定失败的提交。
func TestCommitNeedsStagedChanges(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Click("提交信息")
	tt.Type("还没挑文件")
	tt.Frame()

	if a.canCommit() {
		t.Error("没有暂存的文件，不该能提交")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	settle(t, a)
	tt.Frame()

	if a.opErr != nil {
		t.Errorf("没东西可提交时按 ⌘⏎ 不该真去提交：%+v", *a.opErr)
	}
	if got := gitOut(t, a.repoPath, "log", "--format=%s"); strings.Contains(got, "还没挑文件") {
		t.Errorf("不该有这次提交：%s", got)
	}
	if !tt.HasText("还没有暂存的文件") {
		t.Errorf("该说清为什么交不了：%q", tt.Texts())
	}
}

// 提交被钩子拦下：git 的原话留在 Detail 里，消息一个字都不能丢。
func TestCommitFailureKeepsMessage(t *testing.T) {
	a := richApp(t)
	hooks := filepath.Join(a.repoPath, ".git", "hooks")
	writeFile(t, filepath.Join(hooks, "pre-commit"),
		"#!/bin/sh\necho 'husky - pre-commit hook exited with code 1 (error)' >&2\nexit 1\n")
	if err := os.Chmod(filepath.Join(hooks, "pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 别让全局的 core.hooksPath 把钩子指到别处去 —— 这条测试要验的是
	// 「钩子拦下提交之后界面怎么做」，钩子没跑起来就什么都没验。
	runGit(t, a.repoPath, "config", "core.hooksPath", hooks)

	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Click("提交信息")
	tt.Type("这条提交会被钩子拦下")
	tt.Frame()
	if err := tt.Click("提交 1 个文件"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("钩子拦下了提交，界面该有失败横幅")
	}
	if !strings.Contains(a.opErr.Title, "钩子") {
		t.Errorf("标题该说清是钩子：%q", a.opErr.Title)
	}
	if !strings.Contains(a.opErr.Detail, "pre-commit") {
		t.Errorf("Detail 该留着钩子自己的原话：%q", a.opErr.Detail)
	}
	if !tt.HasText(a.opErr.Title) {
		t.Errorf("失败要说在界面上，实际：%q", tt.Texts())
	}
	// 写了一半的话不能因为一次失败就没了
	if a.commitMsg != "这条提交会被钩子拦下" {
		t.Errorf("提交失败后消息该原样留着，实际 %q", a.commitMsg)
	}
	if a.op != gitrepo.OpNone {
		t.Errorf("操作结束后闸门该放开了，op = %v", a.op)
	}
	// 还没提交出去，但东西还在 index 里 —— 改完钩子直接再点一次就行
	if got := gitOut(t, a.repoPath, "log", "-1", "--format=%s"); got != "init" {
		t.Errorf("不该有这次提交：%q", got)
	}
	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "README.md" {
		t.Errorf("提交失败不该动 index，实际 %q", got)
	}
}

// 失败横幅可以关掉。
func TestOpErrorBannerDismissable(t *testing.T) {
	a := richApp(t)
	a.opErr = &gitrepo.Failure{Title: "这次暂存没成功", Hint: "再试一次。", Detail: "error: nope"}
	tt := ui.NewTester(a.view, 1280, 860)

	if !tt.HasText("这次暂存没成功") {
		t.Fatalf("失败横幅该在：%q", tt.Texts())
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("横幅该有关闭按钮：%v", err)
	}
	tt.Frame()

	if a.opErr != nil {
		t.Error("点了关闭，横幅该收起来")
	}
	if tt.HasText("这次暂存没成功") {
		t.Errorf("关掉之后不该还在：%q", tt.Texts())
	}
}

// 草稿跟着仓库存：切走再切回来，写了一半的话还在；别的仓库有自己的那份。
func TestCommitDraftPerRepo(t *testing.T) {
	a := newApp()
	dirA, dirB := dirtyRepoDir(t), dirtyRepoDir(t)

	a.openRepo(dirA)
	settle(t, a)
	a.commitMsg = "A 的半句话"
	a.saveCommitDraft()

	// 换一个仓库：拾回来的该是那个仓库自己的草稿（它没有）
	a.openRepo(dirB)
	settle(t, a)
	if a.commitMsg != "" {
		t.Errorf("另一个仓库没有草稿，实际拾回 %q", a.commitMsg)
	}
	a.commitMsg = "B 的半句话"
	a.saveCommitDraft()

	// 切回 A：那份还在，而且是 A 的
	a.openRepo(dirA)
	settle(t, a)
	if a.commitMsg != "A 的半句话" {
		t.Errorf("切回来该拾回 A 的草稿，实际 %q", a.commitMsg)
	}
}

// 焦点离开提交框的那一帧落盘草稿 —— 不逐键存（store 一次要重写三个文件）。
func TestCommitDraftSavedOnFocusLoss(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Click("提交信息")
	tt.Type("打完这句去点别处")
	tt.Frame()

	// 点去筛选框：焦点离开提交框
	if err := tt.Click("筛选文件"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if got := a.store.State().Drafts[a.repoPath]; got != "打完这句去点别处" {
		t.Errorf("焦点离开时该把草稿落盘，store 里是 %q", got)
	}
}

// 暂存之后 diff 会整份重读，但用户刚摊开的那段上下文不该跟着收回去 ——
// 动不动就把眼前的东西全折起来，是最劝退的那类反馈。
//
// 保住的只有「行号没动」的那些区间：key 是区间起始行号，暂存了它**上面**
// 的一段，行号就挪了，那时收回去是对的（内容本来也换了一批）。
func TestExpansionSurvivesStageReload(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	a.fileFilter = "handler"
	tt.Frame()

	const bar = "⋯  展开 39 行未改动"
	if !tt.HasText(bar) {
		t.Fatalf("没有折叠条：%q", tt.Texts())
	}
	if err := tt.Click(bar); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("line 20: some code here") {
		t.Fatal("点了折叠条却没摊开")
	}

	// 暂存另一个文件：handler.go 一个字节没动，摊开的那段该还在
	a.fileFilter = ""
	tt.Frame()
	clickStageBox(t, tt, "router.go")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "internal/server/router.go" {
		t.Fatalf("该暂存的是 router.go，index 里却是 %q", got)
	}
	if !tt.HasText("line 20: some code here") {
		t.Errorf("暂存别的文件之后，这边摊开的区间被收回去了：%q", tt.Texts())
	}
}

// 换了个对象（另一个提交、另一份储藏）就不能把上一个的展开状态带过去 ——
// 它们和改动列表的路径可能一模一样，光看 key 分不出是谁的区间。
func TestExpansionDroppedWhenListChanges(t *testing.T) {
	a := richApp(t)
	key := gapKeyOf("u:internal/server/handler.go", 10)
	a.expanded = map[string]int{key: expandStep}

	// 同一个对象的刷新：留着
	a.showFiles(a.allFiles, workListID(a.repoPath, a.branch))
	if _, ok := a.expanded[key]; !ok {
		t.Fatal("同一个列表刷新一次，展开状态不该丢")
	}
	// 换了个对象：清掉
	a.showFiles(a.allFiles, "c:别处")
	if len(a.expanded) != 0 {
		t.Errorf("换了对象之后展开状态该清空，实际 %v", a.expanded)
	}

	// 同一个仓库、换了一条分支也是换了对象：路径一样，行号却对不上
	//（同一个文件在另一条分支上是另一段历史），展开状态必须跟着作废。
	a.expanded = map[string]int{key: expandStep}
	a.showFiles(a.allFiles, workListID(a.repoPath, "另一条分支"))
	if len(a.expanded) != 0 {
		t.Errorf("换了分支之后展开状态该清空，实际 %v", a.expanded)
	}
}

// 筛选词只对着路径，不对着内部的段落前缀 —— 搜「s:」不该把暂存段整个捞出来。
func TestFilterIgnoresSectionKey(t *testing.T) {
	a := halfApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	a.fileFilter = "s:"
	tt.Frame()
	if !tt.HasText("没有匹配的文件") {
		t.Errorf("「s:」是段落前缀不是路径，不该匹配到东西：%q", tt.Texts())
	}

	// 筛选本身是好使的（不然上面那条可能只是因为整个筛选坏了）
	a.fileFilter = "README"
	tt.Frame()
	if !tt.HasText("README.md") {
		t.Errorf("筛选 README 该留下 README.md：%q", tt.Texts())
	}
	// 半暂存的文件两段各一份，两段都该留下
	if n := len(a.files); n != 2 {
		t.Errorf("README.md 两段各一份，筛选后该是 2 个，实际 %d：%v", n, fileListOf(a))
	}
}

// 「已 review」按段落各算各的：同一个路径在已暂存/未暂存是两份不同的
// diff，也是两次不同的 review。
func TestViewedIsPerSection(t *testing.T) {
	a := halfApp(t)
	staged := fileIn(t, a, gitrepo.SectionStaged, "README.md")
	unstaged := fileIn(t, a, gitrepo.SectionUnstaged, "README.md")

	a.toggleViewed(staged)
	if !a.isViewed(staged) {
		t.Fatal("刚标记的那一份该是已 review")
	}
	if a.isViewed(unstaged) {
		t.Error("同一路径的另一段是另一份 diff，不该跟着变成已 review")
	}
	if n := a.viewedCount(); n != 1 {
		t.Errorf("viewedCount = %d, want 1", n)
	}

	// 两段都标上就是两份
	a.toggleViewed(unstaged)
	if n := a.viewedCount(); n != 2 {
		t.Errorf("两段都标记后 viewedCount = %d, want 2", n)
	}
}

// 写闸门：操作还在飞的时候再点一次，不能发出第二次写。
//
// 提交是最好观察的一处 —— 第二次提交会因为没有东西可提交而失败，界面上
// 就会冒出一条本不该有的失败横幅。
func TestCommitGateIgnoresSecondClick(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)
	tt := ui.NewTester(a.view, 1280, 860)

	tt.Click("提交信息")
	tt.Type("只该有一条")
	tt.Frame()

	before := gitOut(t, a.repoPath, "log", "--oneline")
	// 中间不 settle：第一次提交还挂在闸门上，第二次点击必须被挡回去
	if err := tt.Click("提交 1 个文件"); err != nil {
		t.Fatal(err)
	}
	if a.op == gitrepo.OpNone {
		t.Fatal("第一次点击之后该有操作在飞")
	}
	tt.Click("提交 1 个文件")

	settle(t, a)
	tt.Frame()

	if a.opErr != nil {
		t.Errorf("第二次点击该被闸门挡下，却发出了第二次写：%+v", *a.opErr)
	}
	after := gitOut(t, a.repoPath, "log", "--oneline")
	if len(strings.Split(after, "\n")) != len(strings.Split(before, "\n"))+1 {
		t.Errorf("该只多一条提交：\n%s", after)
	}
	if got := gitOut(t, a.repoPath, "log", "-1", "--format=%s"); got != "只该有一条" {
		t.Errorf("最后一次提交的信息是 %q", got)
	}
}

// 刷新同一份列表时，未变更文件的已读内容和高亮缓存保留，不被整体清空导致重读闪烁。
func TestContentPreservedAcrossReloads(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	settle(t, a)
	tt.Frame()

	key := fileKey(a.files[0])
	if a.contents[key] == nil {
		t.Fatal("richApp 应该已经读过文件内容")
	}
	oldContent := a.contents[key]

	// 重新读取同一个仓库
	a.showFiles(a.allFiles, workListID(a.repoPath, a.branch))

	if a.contents[key] == nil || a.contents[key] != oldContent {
		t.Errorf("同一个仓库刷新，文件指纹未变的内容应该保留，实际: %v", a.contents[key])
	}
}

// 全部暂存与全部取消暂存：验证顶部药丸、全选勾选框、提交栏快捷药丸及 ⌘⇧A 快捷键。
func TestStageAllAndUnstageAll(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	settle(t, a)
	tt.Frame()

	totalFiles := len(a.files)
	if totalFiles < 2 {
		t.Fatalf("测试需要至少 2 个改动文件，实际 %d", totalFiles)
	}

	// 初始状态：index 为空，全部为未暂存
	if a.stagedCount() != 0 {
		t.Fatalf("初始已暂存数量该为 0，实际 %d", a.stagedCount())
	}

	// 1. 点击提交栏或栏头的「全部暂存」
	if err := tt.Click("全部暂存"); err != nil {
		t.Fatalf("界面上应该有「全部暂存」按钮: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--name-only"); got != "" {
		t.Errorf("全部暂存后工作区未暂存该为空，实际: %q", got)
	}
	if a.stagedCount() != totalFiles {
		t.Errorf("全部暂存后 stagedCount = %d, want %d", a.stagedCount(), totalFiles)
	}

	// 2. 点击「全部取消」
	if err := tt.Click("全部取消"); err != nil {
		t.Fatalf("全部暂存后界面上应该有「全部取消」: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("全部取消后 index 该为空，实际: %q", got)
	}
	if a.stagedCount() != 0 {
		t.Errorf("全部取消后 stagedCount = %d, want 0", a.stagedCount())
	}

	// 3. 快捷键 ⌘⇧A 暂存全部
	tt.Key(ui.Cmd|ui.Shift, ui.KeyA)
	settle(t, a)
	tt.Frame()

	if a.stagedCount() != totalFiles {
		t.Errorf("快捷键 ⌘⇧A 暂存后 stagedCount = %d, want %d", a.stagedCount(), totalFiles)
	}

	// 4. 再次按 ⌘⇧A 全部取消暂存
	tt.Key(ui.Cmd|ui.Shift, ui.KeyA)
	settle(t, a)
	tt.Frame()

	if a.stagedCount() != 0 {
		t.Errorf("快捷键 ⌘⇧A 取消暂存后 stagedCount = %d, want 0", a.stagedCount())
	}
}

