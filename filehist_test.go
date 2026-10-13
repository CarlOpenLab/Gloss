package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// fileHistApp 是一个有文件历史的仓库：a.go 被改过三次（提交了三次），另外还有
// 一个刚 add 进来、从没提交过的新文件（历史是空的）。
func fileHistApp(t *testing.T) *app {
	t.Helper()
	a := menuApp(t)
	// menuApp 留下的那几个改动先清干净，只留 staged.go 一条已暂存的改动
	runGit(t, a.repoPath, "checkout", "--", ".")
	runGit(t, a.repoPath, "commit", "-m", "first")
	writeFile(t, filepath.Join(a.repoPath, "mod.go"), "package main\n\n// 第二版\n")
	runGit(t, a.repoPath, "add", "mod.go")
	runGit(t, a.repoPath, "commit", "-m", "tweak mod")
	writeFile(t, filepath.Join(a.repoPath, "mod.go"), "package main\n\n// 第三版\n")
	runGit(t, a.repoPath, "add", "mod.go")
	runGit(t, a.repoPath, "commit", "-m", "tweak mod again")
	// 工作区再改一次，让这条路径在「未暂存」段里也有一行
	writeFile(t, filepath.Join(a.repoPath, "mod.go"), "package main\n\n// 第四版（还没提交）\n")
	// 从没提交过的新文件
	writeFile(t, filepath.Join(a.repoPath, "fresh.go"), "package fresh\n")
	runGit(t, a.repoPath, "add", "fresh.go")

	a.reload()
	settle(t, a)
	return a
}

// 打开文件历史：列出这个文件的提交（新的在前），点一条切到历史 tab 看它的
// diff。
func TestFileHistoryDialogJumpsToCommit(t *testing.T) {
	a := fileHistApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "mod.go")
	if err := tt.ChooseMenuItem("查看文件历史…"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.fileHist.on {
		t.Fatal("该弹出文件历史")
	}
	if !tt.HasText("文件历史") {
		t.Errorf("弹窗标题没画出来：%q", tt.Texts())
	}
	if got := len(a.fileHist.commits); got != 3 {
		t.Fatalf("mod.go 有 3 次提交，读到 %d 条", got)
	}
	want := []string{"tweak mod again", "tweak mod", "init"}
	for i, w := range want {
		if a.fileHist.commits[i].Subject != w {
			t.Errorf("第 %d 条该是 %q，实际 %q", i, w, a.fileHist.commits[i].Subject)
		}
	}
	if !tt.HasText("tweak mod") {
		t.Errorf("列表里该看得见提交主题：%q", tt.Texts())
	}

	// 点第二条（tweak mod）→ 切到历史 tab 并选中它
	hash := a.fileHist.commits[1].Hash
	if err := tt.Click("tweak mod"); err != nil {
		t.Fatalf("点提交行: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.fileHist.on {
		t.Error("跳过去之后弹窗该关掉")
	}
	if a.tab != 1 {
		t.Fatalf("该切到历史 tab，实际 tab=%d", a.tab)
	}
	if a.commitSel < 0 || a.commits[a.commitSel].Hash != hash {
		t.Errorf("选中的该是 %s，实际 commitSel=%d", hash[:8], a.commitSel)
	}
	if a.histDiff != loadReady || len(a.histFiles) == 0 {
		t.Errorf("下半区该是那次提交的 diff，state=%v files=%d", a.histDiff, len(a.histFiles))
	}
}

// 从改动 tab 直接跳过去：工作区的文件列表要留着，切回来还在。
func TestFileHistoryKeepsWorkFiles(t *testing.T) {
	a := fileHistApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	workCount := len(a.allFiles)
	if workCount == 0 {
		t.Fatal("改动 tab 里该有文件")
	}

	rightClick(t, tt, "mod.go")
	if err := tt.ChooseMenuItem("查看文件历史…"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if err := tt.Click("tweak mod again"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	a.setTab(0)
	settle(t, a)
	tt.Frame()
	if got := len(a.allFiles); got != workCount {
		t.Errorf("切回改动 tab 后文件列表该还在（%d 条），实际 %d 条", workCount, got)
	}
}

// 历史里的提交不在当前那份列表里（历史长过一屏，或者看的是别的分支）：
// 把历史切到「从这次提交开始看」，而不是什么都不做。
func TestFileHistoryJumpOutsideCurrentLog(t *testing.T) {
	a := fileHistApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "mod.go")
	if err := tt.ChooseMenuItem("查看文件历史…"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	hash := a.fileHist.commits[2].Hash // 那次 "first" 提交
	// 假装当前这份列表是另一条分支的历史：换成一段不含它的日志
	a.histRev = "HEAD"
	a.commits = a.commits[:0]
	a.commitSel = -1
	a.openCommitInHistory(hash)
	tt.Frame() // 跳转在下一帧开头做（见 consumeHistoryJump）
	settle(t, a)
	tt.Frame()

	if a.tab != 1 {
		t.Fatalf("该切到历史 tab，实际 tab=%d", a.tab)
	}
	if a.histRev != hash {
		t.Errorf("该把历史切到这次提交上，实际 histRev=%q", a.histRev)
	}
	if len(a.commits) == 0 {
		t.Fatal("换过去之后该重新读出提交列表")
	}
	if a.commits[0].Hash != hash {
		t.Errorf("列表第一条该就是那次提交，实际 %s", a.commits[0].Short)
	}
	if a.commitSel != 0 {
		t.Errorf("该选中第 0 条，实际 commitSel=%d", a.commitSel)
	}
}

// 从没提交过的文件：历史是空的，弹窗要说「还没有」，而不是转圈或者报错。
func TestFileHistoryEmptyForUncommittedFile(t *testing.T) {
	a := fileHistApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "fresh.go")
	if err := tt.ChooseMenuItem("查看文件历史…"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.fileHist.state != loadReady {
		t.Fatalf("读完了该是 loadReady，实际 %v", a.fileHist.state)
	}
	if len(a.fileHist.commits) != 0 {
		t.Errorf("还没提交过的文件不该有历史，读到 %d 条", len(a.fileHist.commits))
	}
	if !tt.HasText("Git 里还没有这个文件的历史") {
		t.Errorf("空态该说清还没有历史：%q", tt.Texts())
	}
}

// Esc 关掉弹窗，状态跟着清干净。
func TestFileHistoryClosesOnEscape(t *testing.T) {
	a := fileHistApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	rightClick(t, tt, "mod.go")
	if err := tt.ChooseMenuItem("查看文件历史…"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.fileHist.on {
		t.Fatal("Esc 该关掉文件历史")
	}
	if len(a.fileHist.commits) != 0 {
		t.Errorf("关掉之后该把提交列表清干净，还剩 %d 条", len(a.fileHist.commits))
	}
	if a.modalOn() {
		t.Error("关掉之后不该还算「有弹窗开着」")
	}
}

// 文件名不同、内容一样：--follow 要跟得住（这里直接用 git 侧的读法钉住，
// 界面只是把 FileLog 的结果摆出来）。
func TestFileLogFollowsRename(t *testing.T) {
	a := fileHistApp(t)
	runGit(t, a.repoPath, "mv", "mod.go", "renamed.go")
	runGit(t, a.repoPath, "commit", "-m", "rename mod")
	// 改名之后再改一次内容
	writeFile(t, filepath.Join(a.repoPath, "renamed.go"), "package main\n\n// 改名之后\n")
	runGit(t, a.repoPath, "add", "renamed.go")
	runGit(t, a.repoPath, "commit", "-m", "edit after rename")

	repo, err := gitrepo.Open(context.Background(), a.repoPath)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := repo.FileLog(context.Background(), "renamed.go", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) < 5 {
		t.Fatalf("--follow 该把改名之前的历史也带上（至少 5 次），只读到 %d 次", len(commits))
	}
	if got := commits[len(commits)-1].Subject; got != "init" {
		t.Errorf("最老的一次该是 init，实际 %q", got)
	}
}
