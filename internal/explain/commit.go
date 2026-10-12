// Package explain 提供代码解释、提交信息生成和分支名推荐的提示词与领域逻辑。
package explain

import (
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// CommitSystemPrompt 是生成 Git 提交信息的系统提示。
const CommitSystemPrompt = `You are a professional Git commit message expert, strictly following the Conventional Commits specification.
Your task is to generate a precise, clear, and standard Git commit message based on the provided code changes (diff) and optional context hints.

Specifications and output requirements:
1. Format: Strictly follow Conventional Commits:
   <type>(<scope>): <subject>

   [Optional body explaining the reason and background, separated from the first line by a blank line]
2. Common types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert, etc.
3. Scope is optional; indicate the module or functional area affected.
4. The subject line should concisely summarize the commit. Do not capitalize the first letter and do not end with a period. Use the language specified in the instructions (Chinese or English).
5. The body (if any) should focus on the reason and impact — do not mechanically restate diff details.
6. Output only the final commit message content. Never wrap it in Markdown code block tags, quotation marks, greetings, or explanatory filler.`

// maxCommitDiffChars 限制喂给模型的 diff 文本最大长度，避免超长 token 消耗。
const maxCommitDiffChars = 12000

// BuildCommitPrompt 构建生成提交信息的提示词。
func BuildCommitPrompt(files []diff.FileDiff, contextHint string) (systemPrompt, userPrompt string) {
	var b strings.Builder

	if hint := strings.TrimSpace(contextHint); hint != "" {
		fmt.Fprintf(&b, "## 用户提供的意图提示 / 备注\n%s\n\n", hint)
	}

	if len(files) == 0 {
		b.WriteString("（暂存区无文件变更）\n")
		return CommitSystemPrompt, b.String()
	}

	fmt.Fprintf(&b, "## 暂存区文件变更（共 %d 个文件）：\n", len(files))

	truncated := false
	for _, f := range files {
		if truncated {
			break
		}
		fmt.Fprintf(&b, "\n### %s（%s，+%d -%d）\n", DisplayPath(f), statusText(f.Status), f.AddCount, f.DelCount)
		if f.Binary {
			b.WriteString("（二进制文件已变更）\n")
			continue
		}

		if len(f.Hunks) == 0 {
			continue
		}

		b.WriteString("```diff\n")
		for _, h := range f.Hunks {
			raw := h.Raw()
			if b.Len()+len(raw) > maxCommitDiffChars {
				rem := maxCommitDiffChars - b.Len()
				if rem > 0 && rem < len(raw) {
					cut := strings.LastIndex(raw[:rem], "\n")
					if cut > 0 {
						b.WriteString(raw[:cut])
					}
				}
				b.WriteString("\n…（diff 过长，后续变更已截断）\n")
				truncated = true
				break
			}
			b.WriteString(raw)
		}
		b.WriteString("```\n")
	}

	if truncated {
		b.WriteString("\n注意：部分文件的 diff 过长已做截断，请结合已有改动与文件列表概括整体意图。\n")
	}

	return CommitSystemPrompt, b.String()
}
