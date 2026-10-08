package explain

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

func TestBuildCommitPromptWithContextHint(t *testing.T) {
	files := []diff.FileDiff{
		parseOne(t, twoHunks),
	}
	sys, user := BuildCommitPrompt(files, "feat: 增加用户登录接口")

	if !strings.Contains(sys, "Conventional Commits") {
		t.Errorf("系统提示词缺少规范要求，sys=%s", sys)
	}
	if !strings.Contains(user, "feat: 增加用户登录接口") {
		t.Errorf("用户提示词中缺少意图提示，user=%s", user)
	}
	if !strings.Contains(user, "a.go") {
		t.Errorf("用户提示词中缺少变更文件，user=%s", user)
	}
	if !strings.Contains(user, "+inserted") {
		t.Errorf("用户提示词中缺少 diff 内容，user=%s", user)
	}
}

func TestBuildCommitPromptEmptyFiles(t *testing.T) {
	_, user := BuildCommitPrompt(nil, "")
	if !strings.Contains(user, "无文件变更") {
		t.Errorf("空文件列表时提示不正确: %s", user)
	}
}

func TestBuildCommitPromptBinaryFile(t *testing.T) {
	files := []diff.FileDiff{
		{
			OldPath: "image.png",
			NewPath: "image.png",
			Status:  "modified",
			Binary:  true,
		},
	}
	_, user := BuildCommitPrompt(files, "")
	if !strings.Contains(user, "二进制文件已变更") {
		t.Errorf("二进制文件未标注: %s", user)
	}
}

func TestBuildCommitPromptTruncation(t *testing.T) {
	// 构造一个超大的 hunk
	var bigDiff strings.Builder
	bigDiff.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -1,1 +1,1000 @@\n")
	for i := 0; i < 500; i++ {
		bigDiff.WriteString("+this is a very long line to exceed character budget 1234567890\n")
	}
	f := parseOne(t, bigDiff.String())

	// 传入多个以确保超过 12000 字符限制
	files := []diff.FileDiff{f, f, f}
	_, user := BuildCommitPrompt(files, "")

	if !strings.Contains(user, "截断") {
		t.Errorf("超大变更未截断: 长度=%d", len(user))
	}
}

func TestCommitSystemPromptConstraints(t *testing.T) {
	for _, kw := range []string{"Conventional Commits", "<type>(<scope>): <subject>", "Markdown"} {
		if !strings.Contains(CommitSystemPrompt, kw) {
			t.Errorf("CommitSystemPrompt 缺失关键约束 %q", kw)
		}
	}
}
