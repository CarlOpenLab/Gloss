package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// listedFile 按路径取一个已经列在界面上的文件。这几个入口都收 gitrepo.File，
// 测试得先拿到列表里的那一份，才谈得上调它们。
func listedFile(t *testing.T, a *app, path string) gitrepo.File {
	t.Helper()
	for _, f := range a.files {
		if pathOf(f) == path {
			return f
		}
	}
	t.Fatalf("文件列表里没有 %s", path)
	return gitrepo.File{}
}

func readDisk(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s: %v", path, err)
	}
	return string(data)
}

// indexContents 直接问 git 要 index 里那一版，不用 Gloss 自己的读路径 ——
// 拿被验证方自己的代码去验证它自己，bug 会互相抵消。
func indexContents(t *testing.T, dir, path string) string {
	t.Helper()
	repo, err := gitrepo.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("打开仓库: %v", err)
	}
	data, err := repo.ContentsAt(context.Background(), ":0", path)
	if err != nil {
		t.Fatalf("读 index 里的 %s: %v", path, err)
	}
	return string(data)
}

// 编辑器编的是工作区那一份，基线是 index —— 行号槽里的增删才对得上「未暂存」。
func TestCodeEditorOpensWorktreeWithIndexBaseline(t *testing.T) {
	a := richApp(t)
	a.openCodeEditor(listedFile(t, a, "internal/server/handler.go"))
	settle(t, a)

	s := a.codeEdit
	if !s.on || s.code == nil || s.buffer == nil {
		t.Fatalf("编辑器没打开：%q", s.err)
	}
	if got := s.buffer.String(); !strings.Contains(got, "CHANGED AGAIN") {
		t.Errorf("缓冲区该是工作区内容：\n%s", got)
	}
	if s.base == nil {
		t.Fatal("未暂存的文件该有 index 基线")
	}
	if got := s.base.String(); strings.Contains(got, "CHANGED AGAIN") {
		t.Errorf("基线该是改动之前那一版：\n%s", got)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	if !tt.HasText("编辑代码") || !tt.HasText("internal/server/handler.go") {
		t.Errorf("弹窗没画出来：%q", tt.Texts())
	}
}

// 未跟踪的文件没有可比版本：能编，但没有基线可画。
func TestCodeEditorUntrackedFileHasNoBaseline(t *testing.T) {
	a := richApp(t)
	a.openCodeEditor(listedFile(t, a, "internal/server/router.go"))
	settle(t, a)

	s := a.codeEdit
	if !s.on || s.code == nil || s.buffer == nil {
		t.Fatalf("未跟踪的文件也该能编辑：%q", s.err)
	}
	if s.base != nil {
		t.Error("未跟踪的文件不该有基线")
	}
	if got := s.buffer.String(); !strings.Contains(got, "func route") {
		t.Errorf("缓冲区该是磁盘上那一份：\n%s", got)
	}
}

// 保存只写工作区：磁盘变了，index 一个字节没动，弹窗自己收起来。
func TestCodeEditorSaveWritesWorktreeOnly(t *testing.T) {
	a := richApp(t)
	path := filepath.Join(a.repoPath, "README.md")
	a.openCodeEditor(listedFile(t, a, "README.md"))
	settle(t, a)
	if a.codeEdit.code == nil {
		t.Fatalf("编辑器没打开：%q", a.codeEdit.err)
	}

	a.codeEdit.buffer.Set("改过的一行\n")
	a.saveCodeEditor()
	settle(t, a)

	if got := readDisk(t, path); got != "改过的一行\n" {
		t.Errorf("磁盘上是 %q", got)
	}
	if got := indexContents(t, a.repoPath, "README.md"); strings.Contains(got, "改过的一行") {
		t.Errorf("保存不该动 index，实际 %q", got)
	}
	if a.codeEdit.on {
		t.Error("保存成功后编辑器该自己关掉")
	}
	// 少了 reload 的话，界面上还是旧内容 —— 这一条盯着写盘之后有没有重读。
	if got := listedFile(t, a, "README.md"); got.Section != gitrepo.SectionUnstaged {
		t.Errorf("保存后 README.md 该出现在未暂存段，实际 %v", got.Section)
	}
}

// 只有未暂存段能写回磁盘：已暂存段的右侧是 index 里的内容，保存它等于拿
// index 覆盖工作区。
func TestNativeDiffEditGateFollowsSection(t *testing.T) {
	a := richApp(t)
	a.openNativeDiff(listedFile(t, a, "internal/server/handler.go"))
	settle(t, a)
	if !a.nativeDiff.canEdit {
		t.Error("未暂存段该允许编辑")
	}
	if a.nativeDiff.editable || a.nativeDiff.diff.Editable {
		t.Error("默认还是只读预览，编辑要用户自己开")
	}
	a.closeNativeDiff()

	runGit(t, a.repoPath, "add", "-A")
	a.reload()
	settle(t, a)
	staged := listedFile(t, a, "internal/server/handler.go")
	if staged.Section != gitrepo.SectionStaged {
		t.Fatalf("夹具该把文件挪进已暂存段，实际 %v", staged.Section)
	}
	a.openNativeDiff(staged)
	settle(t, a)
	if a.nativeDiff.canEdit {
		t.Error("已暂存段不该允许编辑")
	}
}

func TestNativeDiffSaveWritesWorktree(t *testing.T) {
	a := richApp(t)
	path := filepath.Join(a.repoPath, "README.md")
	a.openNativeDiff(listedFile(t, a, "README.md"))
	settle(t, a)
	if !a.nativeDiff.canEdit {
		t.Fatal("未暂存段该允许编辑")
	}

	a.nativeDiff.editable = true
	a.nativeDiff.diff.Editable = true
	a.nativeDiff.diff.New.Set("从 Diff 视图改的\n")
	a.saveNativeDiff()
	settle(t, a)

	if got := readDisk(t, path); got != "从 Diff 视图改的\n" {
		t.Errorf("磁盘上是 %q", got)
	}
	if a.nativeDiff.on {
		t.Error("保存成功后预览该自己关掉")
	}
}

// 冲突编辑器的整文件模式也要有基线，而且换基线时两种模式必须一起换 —— 它
// 们说的得是同一件事。
func TestConflictPlainModeBaselineFollowsSide(t *testing.T) {
	a, _ := mergeConflictApp(t)
	a.openConflictEditor(listedFile(t, a, "a.txt"))
	settle(t, a)

	s := &a.conflictEdit
	if s.code == nil {
		t.Fatalf("冲突编辑器没打开：%q", s.err)
	}
	if s.code.DiffBase != s.ours {
		t.Error("整文件模式默认该以「当前分支」为基线")
	}

	a.setConflictSide(conflictSideBase)
	if s.code.DiffBase != s.base || s.diff.Old != s.base {
		t.Error("换基线后，编辑器的基线和 Diff 的左侧该指同一份缓冲区")
	}
	a.setConflictSide(conflictSideTheirs)
	if s.code.DiffBase != s.theirs || s.diff.Old != s.theirs {
		t.Error("换到「对方分支」时没跟上")
	}
}

// 入口本身也得有人守：菜单里没这一项，编辑器写得再好也没人打得开。
func TestFileMenuOpensCodeEditor(t *testing.T) {
	a := menuApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	items := rightClick(t, tt, "mod.go")
	if !menuHas(items, "编辑代码") {
		t.Fatalf("未暂存文件的菜单里该有「编辑代码」，实际 %v", items)
	}
	if !menuHas(items, "Diff 预览") {
		t.Errorf("「编辑代码」和「Diff 预览」该并排摆着，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("编辑代码"); err != nil {
		t.Fatalf("「编辑代码」不该是灰的：%v", err)
	}
	settle(t, a)
	if !a.codeEdit.on || a.codeEdit.buffer == nil {
		t.Fatalf("点了「编辑代码」该打开编辑器：%q", a.codeEdit.err)
	}
	if got := a.codeEdit.buffer.String(); !strings.Contains(got, "func main() {}") {
		t.Errorf("打开的该是刚点的那个文件：\n%s", got)
	}
}

// 冲突文件不该再摆一个「编辑代码」：它走「内置编辑器」—— 那边要的是三方
// 基线和 git add 收尾，两个入口并存只会让人两边各改一半。
func TestConflictFileMenuHasNoSecondEditor(t *testing.T) {
	a, _ := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	items := rightClick(t, tt, "a.txt")
	if !menuHas(items, "内置编辑器") {
		t.Fatalf("冲突文件的菜单里该有「内置编辑器」，实际 %v", items)
	}
	if menuHas(items, "编辑代码") {
		t.Errorf("冲突文件不该有第二个编辑器入口：%v", items)
	}
}

// 切仓库要把内置编辑器收掉：缓冲区属于上一个仓库，而保存写的是
// a.repoPath 底下的相对路径 —— 留着它，下一次保存就会写错仓库。
func TestSwitchRepoClosesEditors(t *testing.T) {
	a := richApp(t)
	a.openCodeEditor(listedFile(t, a, "README.md"))
	a.openNativeDiff(listedFile(t, a, "README.md"))
	settle(t, a)
	if !a.codeEdit.on || !a.nativeDiff.on {
		t.Fatalf("两个编辑器该都开着：%q", a.codeEdit.err)
	}

	a.openRepo(newRepoDir(t))
	settle(t, a)

	if a.codeEdit.on {
		t.Error("切仓库后整文件编辑器该收掉")
	}
	if a.nativeDiff.on {
		t.Error("切仓库后 Diff 预览该收掉")
	}
}
