package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/gitrepo"
)

// testApp 建一个真实的临时 git 仓库并加载它，返回可以直接渲染的 app。
// 用真仓库而不是构造假数据：这样测试同时覆盖了 gitrepo 层和界面层，
// 而且不需要 API key、不需要网络。
func testApp(t *testing.T) *app {
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

	// 一个已跟踪文件被修改
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n\nfunc main() {}\n")
	// 一个深层路径的新文件，用来验证目录链合并
	writeFile(t, filepath.Join(dir, "internal/server/x.go"), "package server\n")
	// 一个根目录下的新文件
	writeFile(t, filepath.Join(dir, "README.md"), "# hi\n")

	return loadApp(t, dir)
}

// loadApp 打开 dir 这个仓库并等后台读取跑完，返回可以直接渲染的 app。
func loadApp(t *testing.T, dir string) *app {
	t.Helper()
	repo, err := gitrepo.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	a := newApp()
	a.applyModel(m)
	settle(t, a)
	return a
}

// settle 等所有后台任务跑完。测试里 dispatch 是同步执行，所以后台任务
// 结束时回调也已经跑过了 —— 等它等到就等于界面状态已经稳定。
func settle(t *testing.T, a *app) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		a.bg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("后台任务超时")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// richApp 建一个内容够丰富的仓库：多个 hunk、能折的未改动区间、删除、
// 新增文件、带 tab 和中文的行 —— diff 视图的各种排版情况都能覆盖到。
func richApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	// 60 行的文件，改动分别在第 3 行和第 52 行 —— 中间隔着一大段
	// 未改动区间，正好看到折叠条
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d: some code here", i))
	}
	lines[2] = "line 3: CHANGED"
	lines[51] = "line 52: CHANGED TOO"
	writeFile(t, filepath.Join(dir, "internal/server/handler.go"), strings.Join(lines, "\n")+"\n")

	writeFile(t, filepath.Join(dir, "README.md"), "# diff explainer\n\n旧的一行\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	lines[2] = "line 3: CHANGED AGAIN"
	lines[5] = "line 6: 新加的一行 with 中文"
	lines[51] = "line 52: 也改了"
	writeFile(t, filepath.Join(dir, "internal/server/handler.go"), strings.Join(lines, "\n")+"\n")

	writeFile(t, filepath.Join(dir, "README.md"), "# diff explainer\n\n新的一行\n多了第二行\n")
	// 内容写得像真的 Go 代码，截图里能看出语法高亮对不对
	writeFile(t, filepath.Join(dir, "internal/server/router.go"), `package server

import (
	"fmt"
	"net/http"
)

const maxBody = 1 << 20

// route 注册所有处理器。
func route(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// 健康检查不写日志，免得被探针刷屏
		fmt.Fprintln(w, "ok")
	})
}
`)

	return loadApp(t, dir)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestViewShowsChangedFiles(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	for _, want := range []string{
		"a.go",
		"README.md",
		"internal/server", // 单子目录链合并成一行
		"x.go",
	} {
		if !tt.HasText(want) {
			t.Errorf("界面上找不到 %q，实际文本：%q", want, tt.Texts())
		}
	}
}

func TestViewShowsTotals(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	// a.go 加了 2 行，x.go 加了 1 行，README.md 加了 1 行
	if !tt.HasText("Total: +4 −0") {
		t.Errorf("底部统计不对，实际文本：%q", tt.Texts())
	}
}

func TestViewShowsRepoAndBranch(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if !tt.HasText("main") {
		t.Errorf("顶栏没有显示分支名，实际文本：%q", tt.Texts())
	}
	if a.repoName == "" || !tt.HasText(a.repoName) {
		t.Errorf("顶栏没有显示仓库名 %q，实际文本：%q", a.repoName, tt.Texts())
	}
}

// 筛选只留匹配的文件，不匹配的连同其目录一起消失。
func TestFilterNarrowsTree(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if !tt.HasText("README.md") {
		t.Fatal("筛选前应该有 README.md")
	}

	a.filter = "x.go"
	tt.Frame()

	if !tt.HasText("x.go") {
		t.Errorf("筛选后应该留下 x.go，实际：%q", tt.Texts())
	}
	if tt.HasText("README.md") || tt.HasText("a.go") {
		t.Errorf("筛选后不该还有 README.md / a.go，实际：%q", tt.Texts())
	}
}

func TestFilterNoMatchShowsMessage(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	a.filter = "zzzznothing"
	tt.Frame()

	if !tt.HasText("没有匹配的文件") {
		t.Errorf("无匹配时应给出提示，实际：%q", tt.Texts())
	}
}

func TestViewedCount(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)

	if a.viewedCount() != 0 {
		t.Fatalf("初始 viewedCount = %d, want 0", a.viewedCount())
	}

	a.toggleViewed(a.files[0])
	tt.Frame()
	if a.viewedCount() != 1 {
		t.Errorf("标记后 viewedCount = %d, want 1", a.viewedCount())
	}
	if !tt.HasText("✓ 1/3") {
		t.Errorf("底部应显示 ✓ 1/3，实际：%q", tt.Texts())
	}

	// 再点一次取消
	a.toggleViewed(a.files[0])
	if a.viewedCount() != 0 {
		t.Errorf("取消后 viewedCount = %d, want 0", a.viewedCount())
	}
}

// 文件内容变了，之前的「已 review」标记必须失效 —— 否则改完代码
// 界面还显示看过了，等于骗自己。
func TestViewedInvalidatedWhenContentChanges(t *testing.T) {
	a := testApp(t)
	f := a.files[0]
	a.toggleViewed(f)
	if !a.isViewed(f) {
		t.Fatal("标记后应该是已 review")
	}

	// 模拟文件又被改了：模型重新加载，指纹变了
	writeFile(t, filepath.Join(a.repoPath, f.NewPath), "package main\n\nfunc main() {}\n\n// changed again\n")
	repo, err := gitrepo.Open(context.Background(), a.repoPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := repo.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.applyModel(m)

	// 找到同一个文件的新版本
	var next gitrepo.File
	for _, nf := range a.files {
		if nf.NewPath == f.NewPath {
			next = nf
		}
	}
	if a.isViewed(next) {
		t.Error("文件内容变了，已 review 标记应该失效")
	}
}

func TestEmptyStateBeforeOpeningRepo(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1280, 860)

	if !tt.HasText("还没有打开仓库") {
		t.Errorf("初始应该显示空状态，实际：%q", tt.Texts())
	}
}

func TestFailedStateShowsError(t *testing.T) {
	a := newApp()
	a.state = loadFailed
	a.errMsg = "不是 git 仓库"
	tt := ui.NewTester(a.view, 1280, 860)

	if !tt.HasText("打不开这个仓库") || !tt.HasText("不是 git 仓库") {
		t.Errorf("失败态应该显示错误信息，实际：%q", tt.Texts())
	}
}

// 深色和浅色两套配色都要能正常渲染（配色选取走的是 Theme.Dark 分支）。
func TestRendersInBothThemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := testApp(t)
		tt := ui.NewTester(a.view, 1280, 860)
		tt.SetDark(dark)
		if !tt.HasText("a.go") {
			t.Errorf("dark=%v 时界面渲染不完整：%q", dark, tt.Texts())
		}
	}
}

// 窄窗口下不能崩（侧栏固定宽 280，主区被压缩到很窄）。
func TestNarrowWindow(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 420, 320)
	tt.SetSize(360, 300)
	if !tt.HasText("a.go") {
		t.Errorf("窄窗口下渲染不完整：%q", tt.Texts())
	}
}
