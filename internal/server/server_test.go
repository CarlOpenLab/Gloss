package server

import (
	"strings"
	"testing"

	"diff-explainer/internal/diff"
)

func TestWindowNumbersLines(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\nline6\n"
	h := diff.Hunk{OldStart: 3, OldLines: 2, NewStart: 3, NewLines: 2}

	got := window(content, h, 1, 16000)
	want := "…（前面省略 1 行）\n" +
		"2| line2\n" +
		"3| line3\n" +
		"4| line4\n" +
		"5| line5\n" +
		"6| line6\n" +
		"\n…（后面省略 1 行）"
	if got != want {
		t.Fatalf("window() 行号上下文不对\n got: %q\nwant: %q", got, want)
	}
}

func TestWindowTruncatesOnLineBoundary(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("a-fairly-long-line-of-code\n")
	}
	h := diff.Hunk{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2}

	got := window(b.String(), h, 0, 80)
	if !strings.HasSuffix(got, "…（文件过长，已截断）") {
		t.Fatalf("超长内容应带截断标记，got: %q", got)
	}
	// 截断后除最后一行提示外，每行都应该完整（被 \n 结束）
	body := strings.TrimSuffix(got, "\n…（文件过长，已截断）")
	if strings.Contains(body, "a-fairly-long-line-of-cod\n") {
		t.Fatalf("截断把某一行切了一半: %q", got)
	}
}

func TestBuildPromptsScopesToOneHunk(t *testing.T) {
	file := diff.FileDiff{
		NewPath:  "internal/a.go",
		Status:   "modified",
		AddCount: 3,
		DelCount: 1,
		Hunks: []diff.Hunk{
			{ID: "h1", Header: "@@ -1,2 +1,3 @@", OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3},
			{ID: "h2", Header: "@@ -40,2 +41,2 @@", OldStart: 40, OldLines: 2, NewStart: 41, NewLines: 2},
		},
	}
	sess := &Session{Source: "diff", Title: "本地 Diff", Files: []diff.FileDiff{file}}

	system, user := buildPrompts(sess, file, file.Hunks[1], 1, "41| func foo() {")

	for _, want := range []string{
		"第 2 个",
		"@@ -1,2 +1,3 @@",  // 列出同文件其它块的位置
		"老文件第 40 行起 2 行",   // 当前块的行号区间
		"N| 是文件里的真实行号",     // 上下文带行号
		"41| func foo() {", // 文件上下文内容
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt 缺少 %q\n---\n%s", want, user)
		}
	}
	if !strings.Contains(system, "只解释给你的这一个变更块") {
		t.Errorf("system prompt 缺少「只解释这一个块」的口径要求\n---\n%s", system)
	}
	if strings.Contains(system, "@@ -1,2 +1,3 @@") {
		t.Errorf("system prompt 不应包含具体 diff 内容")
	}
}
