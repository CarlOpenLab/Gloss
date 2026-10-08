package explain

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

func TestBuildBranchPromptWithHintAndFiles(t *testing.T) {
	files := []diff.FileDiff{
		parseOne(t, twoHunks),
	}
	sys, user := BuildBranchPrompt("用户权限系统改造", files)

	if !strings.Contains(sys, "lower-case kebab-case") {
		t.Errorf("系统提示词缺少规范，sys=%s", sys)
	}
	if !strings.Contains(user, "用户权限系统改造") {
		t.Errorf("用户提示词中缺少意图，user=%s", user)
	}
	if !strings.Contains(user, "a.go") {
		t.Errorf("用户提示词中缺少文件列表，user=%s", user)
	}
}

func TestBuildBranchPromptEmpty(t *testing.T) {
	_, user := BuildBranchPrompt("", nil)
	if !strings.Contains(user, "分支名称") {
		t.Errorf("提示词内容异常: %s", user)
	}
}

func TestCleanBranchName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "feat/user-login",
			want:  "feat/user-login",
		},
		{
			input: "  `feat/user-login`  ",
			want:  "feat/user-login",
		},
		{
			input: "\"feat/add-oauth-support\"",
			want:  "feat/add-oauth-support",
		},
		{
			input: "```bash\nfeat/dark-mode\n```",
			want:  "feat/dark-mode",
		},
		{
			input: "git checkout -b feat/checkout-feature",
			want:  "feat/checkout-feature",
		},
		{
			input: "git branch fix/crash-on-boot",
			want:  "fix/crash-on-boot",
		},
		{
			input: "分支名：feat/user-profile",
			want:  "feat/user-profile",
		},
		{
			input: "Branch: feat/test-coverage",
			want:  "feat/test-coverage",
		},
		{
			input: "- feat/api-update",
			want:  "feat/api-update",
		},
		{
			input: "Feat/User_Login With Spaces!",
			want:  "feat/user_login-with-spaces",
		},
		{
			input: "feat/invalid..dots/test.lock",
			want:  "feat/invalid.dots/test",
		},
		{
			input: "/fix/leading-and-trailing-slash/",
			want:  "fix/leading-and-trailing-slash",
		},
		{
			input: "feat//duplicate///slashes",
			want:  "feat/duplicate/slashes",
		},
		{
			input: "",
			want:  "",
		},
	}

	for _, tc := range cases {
		got := CleanBranchName(tc.input)
		if got != tc.want {
			t.Errorf("CleanBranchName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBranchSystemPromptConstraints(t *testing.T) {
	for _, kw := range []string{"kebab-case", "feat/", "fix/", "Git ref"} {
		if !strings.Contains(BranchSystemPrompt, kw) {
			t.Errorf("BranchSystemPrompt 缺失关键约束 %q", kw)
		}
	}
}
