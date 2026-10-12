package explain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// SelectionSystemPrompt 是针对单行或选定代码片段解释的系统提示词。
const SelectionSystemPrompt = `You are a senior Code Review expert, skilled at concisely explaining the purpose and intent of specific lines or selected code snippets.
The user will give you a specific line of code or a selected snippet from a file, along with its surrounding file context.

Please explain:
1. **What this code does**: the core functionality, implementation logic, and calling relationships of the selected code
2. **Intent and background**: infer why the author wrote or modified this code in this location, based on file context
3. **Key details and impact** (optional): potential risks, edge cases, or things to note; if the user has a follow-up question, prioritize answering it

Requirements:
- Start with a one-sentence summary, then expand on core points. Be concise and precise, keeping total length under 250 words (adapt as needed for specific follow-up questions).
- Explain the "why", using context to clarify the relationship with surrounding code.
- Reference actual function names and variable names from the code; mention line numbers when referring to positions.
- Do not restate the code the user gave you — provide the answer directly.
- Answer in Chinese`

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
