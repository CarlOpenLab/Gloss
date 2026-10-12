package explain

import (
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// PRItem 为 gitrepo.PRItem 的别名，便于在 explain 包中直接使用。
type PRItem = gitrepo.PRItem

// PRReviewSystemPrompt 是对 GitHub Pull Request 进行代码评审的系统提示。
const PRReviewSystemPrompt = `You are a senior GitHub PR Code Review expert, skilled at distilling core changes from code diffs and providing professional, clear, and constructive review feedback.
Your task is to generate a high-quality GitHub PR Review summary based on the provided PR information (number, title, author, branches, etc.) and the complete code changes (diff).

Review output requirements:
1. Structure and format (use clean, well-formatted Markdown):
   - **PR Overview**: Summarize the main purpose and core changes of this PR in 1-2 sentences.
   - **Key Changes**: List the main changes and technical implementation highlights, organized by module or importance.
   - **Potential Risks & Suggestions** (if any): Point out boundary cases, concurrency/performance, security, backward compatibility, or refactoring suggestions; if the implementation is solid with no obvious issues, briefly list verification/testing recommendations.
   - **Review Conclusion**: Provide a clear review verdict (e.g., LGTM 👍, suggest merging after adding tests, or request changes before merging).
2. Maintain a professional, objective, friendly, and constructive tone.
3. Focus on what changed, why it changed, and the potential impact — avoid mechanically translating code line by line.
4. Output the Markdown review body directly. Do not wrap it in unnecessary code block tags, and do not include irrelevant pleasantries.
5. Answer in Chinese.`

// maxPRReviewDiffChars 限制喂给模型的 PR diff 文本最大长度。
const maxPRReviewDiffChars = 16000

// BuildPRReviewPrompt 构建用于 PR Review 评论生成的提示词。
func BuildPRReviewPrompt(pr PRItem, files []diff.FileDiff) (systemPrompt, userPrompt string) {
	var b strings.Builder

	b.WriteString("## Pull Request 信息\n")
	if pr.Number > 0 {
		fmt.Fprintf(&b, "- **PR 编号**: #%d\n", pr.Number)
	}
	if pr.Title != "" {
		fmt.Fprintf(&b, "- **PR 标题**: %s\n", pr.Title)
	}
	if pr.Author != "" {
		fmt.Fprintf(&b, "- **作者**: @%s\n", pr.Author)
	}
	if pr.HeadBranch != "" || pr.BaseBranch != "" {
		fmt.Fprintf(&b, "- **分支**: `%s` → `%s`\n", pr.HeadBranch, pr.BaseBranch)
	}
	if pr.URL != "" {
		fmt.Fprintf(&b, "- **链接**: %s\n", pr.URL)
	}
	b.WriteByte('\n')

	if len(files) == 0 {
		b.WriteString("## 代码改动\n（该 PR 无文件变更）\n")
		return PRReviewSystemPrompt, b.String()
	}

	fmt.Fprintf(&b, "## 变更文件列表（共 %d 个文件）：\n", len(files))
	for _, f := range files {
		fmt.Fprintf(&b, "- %s（%s，+%d -%d）\n", DisplayPath(f), statusText(f.Status), f.AddCount, f.DelCount)
	}
	b.WriteByte('\n')

	b.WriteString("## 代码变更详情 (Diff)：\n")
	truncated := false
	for _, f := range files {
		if truncated {
			break
		}
		if f.Binary {
			fmt.Fprintf(&b, "\n### %s\n（二进制文件已变更）\n", DisplayPath(f))
			continue
		}
		if len(f.Hunks) == 0 {
			continue
		}

		fmt.Fprintf(&b, "\n### %s（%s）\n```diff\n", DisplayPath(f), statusText(f.Status))
		for _, h := range f.Hunks {
			raw := h.Raw()
			if b.Len()+len(raw) > maxPRReviewDiffChars {
				truncated = true
				b.WriteString("…（部分 diff 过长，已省略后续内容）\n")
				break
			}
			b.WriteString(raw)
		}
		b.WriteString("```\n")
	}

	b.WriteString("\n请根据以上 PR 元数据与代码变更，生成一份专业、清晰、具有建设性的 GitHub PR Review 评论总结。")

	return PRReviewSystemPrompt, b.String()
}
