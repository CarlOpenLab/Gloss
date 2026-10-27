package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

func setupTestGh(t *testing.T) {
	t.Helper()
	// mock 是个无扩展名 sh 脚本：Windows 的 exec 只认 .exe/.bat，起不来，
	// 依赖它的用例在那边只能跳过（和 internal/gitrepo 的 ghpr_test 同款）。
	if runtime.GOOS == "windows" {
		t.Skip("mock gh 是个 sh 脚本，Windows 起不来，这条只在 macOS/Linux 上有意义")
	}
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	content := `#!/bin/sh
cmd="$1"
sub="$2"

if [ "$cmd" = "pr" ] && [ "$sub" = "list" ]; then
  cat << 'EOF'
[
  {
    "number": 101,
    "title": "feat: add user profile page",
    "author": {"login": "octocat"},
    "headRefName": "feat/profile",
    "baseRefName": "main",
    "updatedAt": "2026-10-09T08:00:00Z",
    "url": "https://github.com/CarlOpenLab/Gloss/pull/101"
  }
]
EOF
  exit 0
fi

if [ "$cmd" = "pr" ] && [ "$sub" = "diff" ]; then
  cat << 'EOF'
diff --git a/profile.go b/profile.go
index 0000000..1111111 100644
--- /dev/null
+++ b/profile.go
@@ -0,0 +1,5 @@
+package main
+
+func ProfilePage() string {
+	return "profile"
+}
EOF
  exit 0
fi

if [ "$cmd" = "pr" ] && [ "$sub" = "comment" ]; then
  # 验证评论参数
  exit 0
fi

echo "unknown command: $@" >&2
exit 1
`
	if err := os.WriteFile(ghPath, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write mock gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPRPaletteAndMenu(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 1. 验证命令栏中存在「查看 GitHub Pull Requests…」命令
	cmds := a.commands()
	var prCmd *command
	for i := range cmds {
		if cmds[i].label == "查看 GitHub Pull Requests…" {
			prCmd = &cmds[i]
			break
		}
	}
	if prCmd == nil {
		t.Fatal("未在 commands() 中找到「查看 GitHub Pull Requests…」")
	}

	// 2. 执行命令栏的 PR 打开命令
	prCmd.run(a)
	if !a.prDlg.on {
		t.Error("执行命令后 a.prDlg.on 应该为 true")
	}

	// 3. 验证菜单同步项
	if a.menu != nil && a.menu.prList != nil {
		if !a.menu.prList.IsEnabled() {
			t.Error("仓库就绪时 prList 菜单项应该处于启用状态")
		}
	}
}

func TestPRDialogListFlow(t *testing.T) {
	setupTestGh(t)
	a := testApp(t)

	// 打开 PR 列表对话框
	a.openPRDialog()
	settle(t, a)

	if !a.prDlg.on {
		t.Fatal("PR 对话框应打开")
	}
	if len(a.prDlg.items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(a.prDlg.items))
	}
	pr := a.prDlg.items[0]
	if pr.Number != 101 || pr.Title != "feat: add user profile page" || pr.Author != "octocat" {
		t.Fatalf("PR item 数据异常: %+v", pr)
	}

	// 渲染对话框 UI
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !tt.HasText("GitHub Pull Requests") {
		t.Errorf("界面应包含标题「GitHub Pull Requests」，当前文字：%q", tt.Texts())
	}
	if !tt.HasText("#101") || !tt.HasText("feat: add user profile page") {
		t.Errorf("界面应包含 PR 编号与标题，当前文字：%q", tt.Texts())
	}

	// 点击 PR 卡片进入检视
	if err := tt.Click("PR #101: feat: add user profile page"); err != nil {
		t.Fatalf("点击 PR 行失败: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.prDlg.on {
		t.Error("点击 PR 后列表对话框应关闭")
	}
	if !a.prReview.on {
		t.Error("点击 PR 后应进入 a.prReview.on 检视状态")
	}
	if a.prReview.state != loadReady {
		t.Fatalf("PR 差异加载状态 = %v, want loadReady, err = %s", a.prReview.state, a.prReview.err)
	}
	if len(a.allFiles) != 1 || a.allFiles[0].Path() != "profile.go" {
		t.Fatalf("加载的 diff 文件不正确：%+v", a.allFiles)
	}
}

func TestPRReviewDiffAndRestore(t *testing.T) {
	setupTestGh(t)
	a := testApp(t)

	// 初始状态下记录手头文件数量
	initialCount := len(a.allFiles)

	pr := gitrepo.PRItem{
		Number:     101,
		Title:      "feat: add user profile page",
		Author:     "octocat",
		HeadBranch: "feat/profile",
		BaseBranch: "main",
	}

	// 开启 PR 检视
	a.openPRReview(pr)
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !a.prReview.on {
		t.Fatal("PR 检视应该开启")
	}
	if !tt.HasText("PR 检视") {
		t.Errorf("界面顶部应出现「PR 检视」条，当前文字：%q", tt.Texts())
	}
	if !tt.HasText("✨ 生成并发布 AI Review 评论") {
		t.Errorf("界面应包含「✨ 生成并发布 AI Review 评论」按钮，当前文字：%q", tt.Texts())
	}

	// 退出 PR 检视
	if err := tt.Click("退出 PR 检视"); err != nil {
		t.Fatalf("点击退出检视按钮失败: %v", err)
	}
	tt.Frame()

	if a.prReview.on {
		t.Error("退出后 a.prReview.on 应为 false")
	}
	if len(a.allFiles) != initialCount {
		t.Errorf("退出后工作区文件数恢复为 %d，got %d", initialCount, len(a.allFiles))
	}
}

func TestPRReviewAIReviewAndPost(t *testing.T) {
	setupTestGh(t)
	srv, _ := fakeLLM(t, "## PR 概览\n本 PR 新增了用户个人资料页面。\n\n## Review 结论\nLGTM 👍")
	a := testApp(t)
	withProvider(t, a, srv)

	pr := gitrepo.PRItem{
		Number:     101,
		Title:      "feat: add user profile page",
		Author:     "octocat",
		HeadBranch: "feat/profile",
		BaseBranch: "main",
	}

	a.openPRReview(pr)
	settle(t, a)

	// 打开 AI Review 模态框并生成
	a.openPRReviewModal()
	settle(t, a)

	if !a.prReview.reviewModal {
		t.Fatal("AI Review 模态框应处于打开状态")
	}
	if !strings.Contains(a.prReview.reviewText, "LGTM") {
		t.Errorf("生成的 Review 文本应包含 LGTM，got:\n%s", a.prReview.reviewText)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 提交评论到 GitHub
	a.postPRReviewComment()
	settle(t, a)

	if !a.prReview.postSuccess {
		t.Fatalf("发布评论应该成功，err = %s", a.prReview.postErr)
	}

	tt.Frame()
	if !tt.HasText("已成功将 Review 评论发布至 GitHub PR #101！") {
		t.Errorf("界面应显示成功提示，当前文字：%q", tt.Texts())
	}
}
