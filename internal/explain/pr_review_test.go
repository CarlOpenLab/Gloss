package explain

import (
	"strings"
	"testing"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

func TestBuildPRReviewPrompt(t *testing.T) {
	pr := PRItem{
		Number:     42,
		Title:      "feat: add github PR review integration",
		Author:     "carl",
		HeadBranch: "feat/pr-integration",
		BaseBranch: "main",
		UpdatedAt:  time.Now(),
		URL:        "https://github.com/CarlOpenLab/Gloss/pull/42",
	}

	files := []diff.FileDiff{
		{
			OldPath:  "app.go",
			NewPath:  "app.go",
			Status:   "modified",
			AddCount: 10,
			DelCount: 2,
			Hunks: []diff.Hunk{
				{
					Header:   "@@ -1,5 +1,13 @@",
					OldStart: 1,
					OldLines: 5,
					NewStart: 1,
					NewLines: 13,
					Lines: []diff.Line{
						{Type: diff.LineContext, Content: "package main"},
						{Type: diff.LineAdd, Content: "func PRReview() {}"},
					},
				},
			},
		},
		{
			OldPath:  "logo.png",
			NewPath:  "logo.png",
			Status:   "modified",
			Binary:   true,
			AddCount: 0,
			DelCount: 0,
		},
	}

	sys, user := BuildPRReviewPrompt(pr, files)

	if !strings.Contains(sys, "GitHub PR Code Review 专家") {
		t.Errorf("System prompt missing role definition: %s", sys)
	}

	expectedParts := []string{
		"#42",
		"feat: add github PR review integration",
		"@carl",
		"feat/pr-integration",
		"main",
		"https://github.com/CarlOpenLab/Gloss/pull/42",
		"变更文件列表（共 2 个文件）",
		"app.go",
		"logo.png",
		"二进制文件已变更",
		"+func PRReview() {}",
	}

	for _, p := range expectedParts {
		if !strings.Contains(user, p) {
			t.Errorf("User prompt expected to contain %q, but got:\n%s", p, user)
		}
	}
}

func TestBuildPRReviewPrompt_EmptyFiles(t *testing.T) {
	pr := PRItem{
		Number: 1,
		Title:  "empty pr",
	}
	_, user := BuildPRReviewPrompt(pr, nil)
	if !strings.Contains(user, "无文件变更") {
		t.Errorf("Expected mention of no changes, got:\n%s", user)
	}
}
