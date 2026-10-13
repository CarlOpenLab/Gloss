package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// selectEditorFile 把文件栏选中挪到某个文件上，并让编辑器跟上 —— 复刻用户在
// 左栏点一下的那条路（treeRow 的点击处理里就是这么调的）。
func selectEditorFile(t *testing.T, a *app, path string) {
	t.Helper()
	for i := range a.files {
		if pathOf(a.files[i]) == path {
			a.fileSel = i
			a.syncEditorPane()
			return
		}
	}
	t.Fatalf("文件栏里没有 %s", path)
}

// 进「编辑器」视图该把左栏那个文件读进来，并配好 git 基线。
func TestEditorViewLoadsSelectedFile(t *testing.T) {
	a := richApp(t)
	selectEditorFile(t, a, "internal/server/handler.go")
	a.setViewMode(viewModeEditor)
	settle(t, a)

	s := a.editorPane
	if s.code == nil || s.buffer == nil {
		t.Fatalf("编辑器没读到文件：%q", s.err)
	}
	if got := s.buffer.String(); !strings.Contains(got, "line 3: CHANGED AGAIN") {
		t.Errorf("读到的不是工作区那一份：%q", got)
	}
	if s.base == nil {
		t.Error("未暂存的文件该有 index 基线（行号槽要显示相对 index 的改动）")
	}
}

// 换文件（左栏选中变了）编辑器要跟着换缓冲区。
func TestEditorViewFollowsFileSelection(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	selectEditorFile(t, a, "README.md")
	settle(t, a)

	if a.editorPane.path != "README.md" {
		t.Fatalf("编辑器该跟到 README.md，实际 %q", a.editorPane.path)
	}
	if got := a.editorPane.buffer.String(); !strings.Contains(got, "新的一行") {
		t.Errorf("README 内容不对：%q", got)
	}
}

// 保存只写工作区、不动 index；而且编辑器视图存完要继续留着（不像「编辑代码」
// 对话框那样存完就关）。
func TestEditorViewSaveWritesWorktreeOnly(t *testing.T) {
	a := richApp(t)
	selectEditorFile(t, a, "README.md")
	a.setViewMode(viewModeEditor)
	settle(t, a)
	if a.editorPane.code == nil {
		t.Fatalf("编辑器没打开：%q", a.editorPane.err)
	}

	a.editorPane.buffer.Set("编辑器视图改的\n")
	a.saveEditorPane()
	settle(t, a)

	if got := readDisk(t, filepath.Join(a.repoPath, "README.md")); got != "编辑器视图改的\n" {
		t.Errorf("磁盘上是 %q", got)
	}
	if got := indexContents(t, a.repoPath, "README.md"); strings.Contains(got, "编辑器视图改的") {
		t.Errorf("保存不该动 index，实际 %q", got)
	}
	if a.editorPane.code == nil {
		t.Error("编辑器视图保存后该继续留着")
	}
	if a.editorPane.buffer.Modified() {
		t.Error("存完不该再显示未保存圆点")
	}
	if got := listedFile(t, a, "README.md").Section; got != gitrepo.SectionUnstaged {
		t.Errorf("保存后该回到未暂存段，实际 %v", got)
	}
}

// 还原把未保存的改动丢掉、重读磁盘（不是内存回滚）。
func TestEditorViewRevertReloadsFromDisk(t *testing.T) {
	a := richApp(t)
	selectEditorFile(t, a, "README.md")
	a.setViewMode(viewModeEditor)
	settle(t, a)

	a.editorPane.buffer.Set("乱改的一行\n")
	if !a.editorPane.buffer.Modified() {
		t.Fatal("改了缓冲区该显示未保存")
	}

	a.revertEditorPane()
	settle(t, a)

	if got := a.editorPane.buffer.String(); strings.Contains(got, "乱改的一行") {
		t.Errorf("还原后还有未保存内容：%q", got)
	}
	if a.editorPane.buffer.Modified() {
		t.Error("还原后不该还是未保存状态")
	}
}

// 换仓库要把编辑器缓冲区丢掉 —— 否则下一次保存会把上一个仓库的内容写进新仓库
// （保存用的是 a.repoPath 底下的相对路径）。
func TestSwitchRepoClosesEditorPane(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)
	if a.editorPane.code == nil {
		t.Fatalf("前置条件：编辑器该已打开，实际 %q", a.editorPane.err)
	}

	a.openRepo(newRepoDir(t))
	settle(t, a)

	if a.editorPane.code != nil || a.editorPane.buffer != nil {
		t.Error("换仓库后编辑器缓冲区该清空")
	}
}

// 「编辑器」视图下工具栏该给出保存/还原。
func TestEditorToolbarActions(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	for _, want := range []string{"保存", "还原"} {
		if !tt.HasText(want) {
			t.Errorf("工具栏缺 %q，实际文本：%q", want, tt.Texts())
		}
	}
}

// 「编辑器」视图整屏该同时出现左栏文件列表和右边的编辑器栏头。
func TestEditorViewRendersFileListAndHeader(t *testing.T) {
	a := richApp(t)
	a.setViewMode(viewModeEditor)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	for _, want := range []string{"README.md", a.editorPane.path} {
		if !tt.HasText(want) {
			t.Errorf("界面上找不到 %q，实际文本：%q", want, tt.Texts())
		}
	}
}
