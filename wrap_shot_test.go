package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 这个文件是人工检查工具：把四处换行开关渲染成 PNG，给 verify 画板用。
// 只在 SHOT_WRAP=1 时跑，不进常规测试。
func TestDumpWrapScreens(t *testing.T) {
	if os.Getenv("SHOT_WRAP") == "" {
		t.Skip("设置 SHOT_WRAP=1 才跑")
	}

	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	initRepo(t, dir)

	// 一条明显超宽的行：三段内容各不相同，改动区和对比弹窗都折得出来。
	longLine := func(tag string) string {
		return "package demo\n\nfunc handleSomethingVeryLong(ctx context.Context, request *SomeRequestType) (*SomeResponseType, error) { // " +
			tag + " " + strings.Repeat("padding ", 12) + "\n"
	}
	path := filepath.Join(dir, "long_line.go")
	writeFile(t, path, longLine("OLD"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "branch", "feature")
	runGit(t, dir, "checkout", "feature")
	writeFile(t, path, longLine("FEATURE"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feature")
	runGit(t, dir, "checkout", "main")
	writeFile(t, path, longLine("WORKTREE"))

	a := loadApp(t, dir)
	out := filepath.Join(".ryugi", "artifacts", "artboard", "soft-wrap", "pages", "verify")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	shot := func(name string) {
		t.Helper()
		settle(t, a)
		tt.Frame()
		writePNG(t, filepath.Join(out, name), tt.Image())
	}

	// 改动区：关 / 开各一张，折行前后的差别一眼看得出来。
	waitReview(t, a, tt)
	shot("review-nowrap.png")
	a.wrapDiff = true
	shot("review-wrap.png")

	// 主界面编辑器 tab：按钮常驻在 tab 栏右端。
	a.setViewMode(viewModeEditor)
	settle(t, a)
	openEditorFile(t, a, "long_line.go")
	a.wrapCode = true
	shot("editor-wrap.png")

	// 整文件代码编辑器：按钮在底栏。
	a.openCodeEditor(listedFile(t, a, "long_line.go"))
	settle(t, a)
	shot("code-editor-wrap.png")
	a.closeCodeEditor()
	settle(t, a)

	// 对比弹窗：按钮在 Unified / Split 旁边。
	a.startCompare("main", "feature")
	settle(t, a)
	for range 8 {
		if a.compare.review.diff != nil {
			break
		}
		settle(t, a)
		tt.Frame()
	}
	if a.compare.review.diff == nil {
		t.Fatalf("对比弹窗的 diff 没装好：%q", a.compare.review.err)
	}
	shot("compare-wrap.png")
	a.exitCompare()
	settle(t, a)

	// 设置页：两个开关和四处按钮是同一份状态。
	st := settingsTester(t, a, settingsWindowW, settingsWindowH)
	a.settingsTab = settingsTabGeneral
	st.Frame()
	scrollTo(t, st, T("代码自动换行", "Wrap Code"), settingsWindowH)
	st.Frame()
	writePNG(t, filepath.Join(out, "settings-wrap.png"), st.Image())
}
