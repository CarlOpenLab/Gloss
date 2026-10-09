package explain

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

func TestBuildSelectionPromptBasic(t *testing.T) {
	f := parseOne(t, twoHunks)
	sys, user := BuildSelectionPrompt(SelectionRequest{
		File:    f,
		Code:    "inserted",
		LineNum: 2,
	})

	if sys != SelectionSystemPrompt {
		t.Errorf("system prompt mismatch")
	}

	for _, want := range []string{
		"a.go",
		"第 2 行",
		"inserted",
		"请解释这一行/这一段代码的具体作用",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt 缺 %q:\n%s", want, user)
		}
	}
}

func TestBuildSelectionPromptWithQuestion(t *testing.T) {
	f := parseOne(t, twoHunks)
	_, user := BuildSelectionPrompt(SelectionRequest{
		File:     f,
		Code:     "inserted",
		LineNum:  2,
		Question: "为什么要在这里插入这一行？是否存在并发安全问题？",
	})

	if !strings.Contains(user, "用户追问与关注重点：") {
		t.Errorf("缺少追问标头:\n%s", user)
	}
	if !strings.Contains(user, "为什么要在这里插入这一行？是否存在并发安全问题？") {
		t.Errorf("追问内容未包含在 prompt 中:\n%s", user)
	}
	if strings.Contains(user, "请解释这一行/这一段代码的具体作用、上下文逻辑以及为什么要这样实现或修改。") {
		t.Errorf("有用户追问时不应出现默认提示语:\n%s", user)
	}
}

func TestBuildSelectionPromptSnippetWithoutLineNum(t *testing.T) {
	f := parseOne(t, twoHunks)
	_, user := BuildSelectionPrompt(SelectionRequest{
		File:    f,
		Code:    "one\ninserted\ntwo",
		LineNum: 0,
	})

	if !strings.Contains(user, "## 需要解释的代码片段：") {
		t.Errorf("未指定行号时应显示代码片段标头:\n%s", user)
	}
	if strings.Contains(user, "位于第 0 行") {
		t.Errorf("不应显示第 0 行:\n%s", user)
	}
}

func TestBuildSelectionPromptWithContext(t *testing.T) {
	f := parseOne(t, twoHunks)
	_, user := BuildSelectionPrompt(SelectionRequest{
		File:    f,
		Code:    "tail",
		LineNum: 23,
		Context: "21| old\n22| new\n23| tail",
	})

	if !strings.Contains(user, "真实行号") {
		t.Errorf("缺上下文说明:\n%s", user)
	}
	if !strings.Contains(user, "22| new") {
		t.Errorf("上下文内容未包含:\n%s", user)
	}
}

func TestBuildSelectionPromptRenamedFile(t *testing.T) {
	f := diff.FileDiff{
		OldPath:  "old.go",
		NewPath:  "new.go",
		Status:   "renamed",
		AddCount: 1,
		DelCount: 1,
	}
	_, user := BuildSelectionPrompt(SelectionRequest{
		File:    f,
		Code:    "package new",
		LineNum: 1,
	})

	if !strings.Contains(user, "old.go → new.go") {
		t.Errorf("缺少重命名路径:\n%s", user)
	}
	if !strings.Contains(user, "重命名") {
		t.Errorf("缺少重命名状态:\n%s", user)
	}
}

func TestSelectionCacheKey(t *testing.T) {
	k1 := SelectionCacheKey("a.go", "x := 1", "", "claude-3-5-sonnet")
	k2 := SelectionCacheKey("a.go", "x := 1", "", "claude-3-5-sonnet")
	if k1 != k2 {
		t.Fatalf("相同输入生成的缓存 key 应该一致: %s vs %s", k1, k2)
	}

	// 改变代码
	k3 := SelectionCacheKey("a.go", "x := 2", "", "claude-3-5-sonnet")
	if k1 == k3 {
		t.Errorf("不同代码应该生成不同的 key")
	}

	// 改变路径
	k4 := SelectionCacheKey("b.go", "x := 1", "", "claude-3-5-sonnet")
	if k1 == k4 {
		t.Errorf("不同路径应该生成不同的 key")
	}

	// 改变追问
	k5 := SelectionCacheKey("a.go", "x := 1", "这段代码安全吗？", "claude-3-5-sonnet")
	if k1 == k5 {
		t.Errorf("不同追问应该生成不同的 key")
	}

	// 改变模型
	k6 := SelectionCacheKey("a.go", "x := 1", "", "gpt-4o")
	if k1 == k6 {
		t.Errorf("不同模型应该生成不同的 key")
	}

	if !strings.HasSuffix(k1, ":claude-3-5-sonnet") {
		t.Errorf("key 应该以模型名作为后缀: %s", k1)
	}
}

func TestLineWindow(t *testing.T) {
	lines := []string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
		"line 10",
	}

	// 居中取窗口：around = 2，lineNum = 5 -> start=2 (line 3), end=7 (line 7)
	w := LineWindow(lines, 5, 2, 1000)
	if !strings.Contains(w, "3| line 3") || !strings.Contains(w, "5| line 5") || !strings.Contains(w, "7| line 7") {
		t.Errorf("窗口未包含目标行及前后行:\n%s", w)
	}
	if !strings.Contains(w, "前面省略 2 行") {
		t.Errorf("缺少前置省略提示:\n%s", w)
	}
	if !strings.Contains(w, "后面省略 3 行") {
		t.Errorf("缺少后置省略提示:\n%s", w)
	}

	// 开头附近：around = 3, lineNum = 1
	wStart := LineWindow(lines, 1, 3, 1000)
	if strings.Contains(wStart, "前面省略") {
		t.Errorf("开头不应有前面省略提示:\n%s", wStart)
	}
	if !strings.Contains(wStart, "1| line 1") {
		t.Errorf("未包含第一行:\n%s", wStart)
	}

	// 结尾附近：around = 3, lineNum = 10
	wEnd := LineWindow(lines, 10, 3, 1000)
	if strings.Contains(wEnd, "后面省略") {
		t.Errorf("结尾不应有后面省略提示:\n%s", wEnd)
	}
	if !strings.Contains(wEnd, "10| line 10") {
		t.Errorf("未包含最后一行:\n%s", wEnd)
	}

	// 无效输入
	if LineWindow(nil, 1, 2, 1000) != "" {
		t.Errorf("空行切片应返回空串")
	}
	if LineWindow(lines, 0, 2, 1000) != "" {
		t.Errorf("lineNum <= 0 应返回空串")
	}

	// 长度超限截断
	wTrunc := LineWindow(lines, 5, 4, 30)
	if !strings.Contains(wTrunc, "已截断") {
		t.Errorf("超长内容应被截断:\n%s", wTrunc)
	}
}
