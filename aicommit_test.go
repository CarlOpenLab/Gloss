package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAICommitGeneration(t *testing.T) {
	srv, calls := fakeLLM(t, "feat(server): update handler logic")
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	if a.stagedCount() == 0 {
		t.Fatal("没有暂存的文件")
	}

	a.startAICommit()
	settle(t, a)

	if *calls != 1 {
		t.Fatalf("LLM 被调用了 %d 次，want 1", *calls)
	}
	if a.commitMsg != "feat(server): update handler logic" {
		t.Errorf("生成提交信息不符合预期: %q", a.commitMsg)
	}
	if a.aiCommitLoading {
		t.Error("完成后 aiCommitLoading 应该为 false")
	}
}

func TestAICommitGenerationFromUI(t *testing.T) {
	srv, _ := fakeLLM(t, "feat: update api")
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if err := tt.Click("✨ AI 生成提交信息"); err != nil {
		t.Fatalf("点 AI 生成按钮失败: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.commitMsg != "feat: update api" {
		t.Errorf("生成的提交信息不正确: %q", a.commitMsg)
	}
}

func TestAICommitShortcut(t *testing.T) {
	srv, _ := fakeLLM(t, "chore: cleanup files")
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyI)
	tt.Frame()
	settle(t, a)

	if a.commitMsg != "chore: cleanup files" {
		t.Errorf("快捷键 ⌘I 触发的提交信息不正确: %q", a.commitMsg)
	}
}

func TestAICommitWithoutProviderOpensSettings(t *testing.T) {
	a := richApp(t)
	clearProviderEnv(t)
	a.stageAll()
	settle(t, a)

	a.startAICommit()
	settle(t, a)

	if !a.settingsOn {
		t.Error("未配置 provider 时，应该打开模型设置窗口")
	}
}

func TestAICommitCancel(t *testing.T) {
	srv, _ := fakeLLM(t, "slow chunk")
	a := richApp(t)
	withProvider(t, a, srv)
	a.stageAll()
	settle(t, a)

	a.startAICommit()
	if !a.aiCommitLoading {
		t.Fatal("发起后应该处于 loading 状态")
	}
	a.cancelAICommit()
	if a.aiCommitLoading {
		t.Error("取消后 loading 应该为 false")
	}
	settle(t, a)
}

func TestAIBranchGeneration(t *testing.T) {
	srv, calls := fakeLLM(t, "```bash\nfeat/handler-logic-update\n```")
	a := richApp(t)
	withProvider(t, a, srv)

	a.openNewBranchDialog("", "HEAD")
	if !a.newBranch.on {
		t.Fatal("新建分支对话框未打开")
	}

	a.startAIBranch()
	settle(t, a)

	if *calls != 1 {
		t.Fatalf("LLM 被调用了 %d 次，want 1", *calls)
	}
	if a.newBranch.name != "feat/handler-logic-update" {
		t.Errorf("生成的推荐分支名不正确: %q", a.newBranch.name)
	}
	if a.newBranch.aiLoading {
		t.Error("完成后 aiLoading 应该为 false")
	}
}

func TestAIBranchGenerationFromUI(t *testing.T) {
	srv, _ := fakeLLM(t, "fix/router-bug")
	a := richApp(t)
	withProvider(t, a, srv)

	a.openNewBranchDialog("", "HEAD")
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if err := tt.Click("✨ AI 建议分支名"); err != nil {
		t.Fatalf("点击 AI 建议分支名按钮失败: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.newBranch.name != "fix/router-bug" {
		t.Errorf("生成的推荐分支名不正确: %q", a.newBranch.name)
	}
}

func TestAIBranchWithoutProviderOpensSettings(t *testing.T) {
	a := richApp(t)
	clearProviderEnv(t)
	a.openNewBranchDialog("", "HEAD")

	a.startAIBranch()
	settle(t, a)

	if !a.settingsOn {
		t.Error("未配置 provider 时，应该打开模型设置窗口")
	}
}

func TestCleanCommitMessage(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "feat: normal message",
			want:  "feat: normal message",
		},
		{
			input: "```\nfeat(ui): add button\n\nSome body\n```",
			want:  "feat(ui): add button\n\nSome body",
		},
		{
			input: "```git\nfix: handle nil pointer\n```",
			want:  "fix: handle nil pointer",
		},
		{
			input: "`feat: wrapped in backticks`",
			want:  "feat: wrapped in backticks",
		},
		{
			input: "\"feat: wrapped in quotes\"",
			want:  "feat: wrapped in quotes",
		},
	}

	for _, tc := range cases {
		got := cleanCommitMessage(tc.input)
		if got != tc.want {
			t.Errorf("cleanCommitMessage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
