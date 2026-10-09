// Package explain 提供代码解释、提交信息生成和分支名推荐的提示词与领域逻辑。
package explain

import (
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// CommitSystemPrompt 是生成 Git 提交信息的系统提示。
const CommitSystemPrompt = `你是一位专业的 Git 提交信息专家，严格遵循 Conventional Commits 规范。
你的任务是根据用户提供的代码变更（diff）和可选的上下文提示，生成精准、清晰、规范的 Git 提交信息。

规范与输出要求：
1. 格式：严格遵循 Conventional Commits 规范：
   <type>(<scope>): <subject>

   [可选的正文 body，解释修改的原因与背景，用空行与首行隔开]
2. 类型 (type) 常见为：feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert 等。
3. 作用域 (scope) 可选，标明变动的模块或功能区域。
4. 首行主题 (subject) 简明扼要概括本次提交，首字母不大写，结尾不加句号。如果用户提示或代码上下文主要为中文，优先使用中文；否则使用英文。
5. 正文 (如有) 聚焦于“为什么修改”和“有何影响”，不要机械复述 diff 细节。
6. 直接输出最终的提交信息内容，绝对不要包含任何 Markdown 代码块标签（如 ` + "```" + ` 或 ` + "```git" + `）、引号包裹、问候语或解释性废话。`

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
