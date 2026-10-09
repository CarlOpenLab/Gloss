package explain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// SelectionSystemPrompt 是针对单行或选定代码片段解释的系统提示词。
const SelectionSystemPrompt = `你是一位资深的 Code Review 专家，擅长用简洁清晰的中文解释代码中特定行或选定代码片段的作用与意图。
用户会给你一个文件中特定的一行代码或一段选定代码，以及它所在文件的上下文。

请解释：
1. **这段代码的作用**：解释所选代码的核心功能、实现逻辑与调用关系
2. **意图与背景**：结合文件上下文推断作者为什么在这里写下/修改这段代码
3. **关键细节与影响**（可选）：潜在风险、边缘条件或需要注意的地方；如果用户有追问，请优先解答用户的疑问

要求：
- 先给出一句话总结，再展开核心要点，语言凝练精准，总长度控制在 250 字以内（用户有特定追问时按需回答）
- 讲清楚"为什么"，结合上下文说明与周围代码的关联
- 引用代码中真实出现的函数名、变量名，引用位置时说明行号
- 不要复述用户给你的代码，直接给出解答
- 用中文回答`

// SelectionRequest 描述要解释的特定代码行或选区。
type SelectionRequest struct {
	File     diff.FileDiff
	Code     string // the specific line or selected slice of code
	LineNum  int    // line number in new file, or old file if deleted
	Question string // optional user follow-up / prompt, defaults to explaining what this specific line/slice does and why it was modified
	Context  string // surrounding file content with line numbers
}

// BuildSelectionPrompt 拼出针对特定行或选区解释的 (systemPrompt, userPrompt) 两条提示词。
func BuildSelectionPrompt(req SelectionRequest) (systemPrompt, userPrompt string) {
	f := req.File
	path := Path(f)

	var b strings.Builder
	if path != "" {
		fmt.Fprintf(&b, "## 文件\n路径：%s（%s，+%d -%d）\n\n", DisplayPath(f), statusText(f.Status), f.AddCount, f.DelCount)
	}

	if req.Context != "" {
		fmt.Fprintf(&b, "以下是该文件的相关上下文内容（每行行首的 N| 是文件里的真实行号）：\n```%s\n%s\n```\n\n",
			Lang(path), req.Context)
	}

	if req.LineNum > 0 {
		fmt.Fprintf(&b, "## 需要解释的代码（位于第 %d 行）：\n", req.LineNum)
	} else {
		fmt.Fprintf(&b, "## 需要解释的代码片段：\n")
	}
	fmt.Fprintf(&b, "```%s\n%s\n```\n", Lang(path), req.Code)

	if req.Question != "" {
		fmt.Fprintf(&b, "\n## 用户追问与关注重点：\n%s\n", req.Question)
	} else {
		fmt.Fprintf(&b, "\n请解释这一行/这一段代码的具体作用、上下文逻辑以及为什么要这样实现或修改。\n")
	}

	return SelectionSystemPrompt, b.String()
}

// SelectionCacheKey 算出单行或选区解释结果的缓存 key：
// 基于文件路径、代码片段、追问内容哈希 + 模型名。
func SelectionCacheKey(path string, code string, question string, model string) string {
	sum := sha256.Sum256([]byte(path + "\n" + code + "\n" + question))
	return hex.EncodeToString(sum[:])[:16] + ":" + model
}

// LineWindow 截取文件中指定行前后 around 行的内容，行首带上真实行号。
func LineWindow(lines []string, lineNum, around, maxChars int) string {
	if len(lines) == 0 || lineNum <= 0 {
		return ""
	}
	start := max(0, lineNum-1-around)
	end := min(len(lines), lineNum+around)
	if start >= end {
		return ""
	}

	var b strings.Builder
	if start > 0 {
		fmt.Fprintf(&b, "…（前面省略 %d 行）\n", start)
	}
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%d| %s\n", i+1, lines[i])
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "\n…（后面省略 %d 行）", len(lines)-end)
	}

	out := b.String()
	if len(out) > maxChars {
		cut := strings.LastIndex(out[:maxChars], "\n")
		if cut < 0 {
			cut = maxChars
		}
		out = out[:cut] + "\n…（文件过长，已截断）"
	}
	return out
}
