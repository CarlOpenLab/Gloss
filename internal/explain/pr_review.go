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
const PRReviewSystemPrompt = `你是一位资深的 GitHub PR Code Review 专家，擅长从代码变更中提炼核心改动，提供专业、清晰且具建设性的评审意见。
你的任务是根据用户提供的 PR 信息（编号、标题、作者、分支等）与完整的代码改动（diff），生成一份高质量的 GitHub PR Review 总结。

评审输出要求：
1. 结构与格式（使用规范、排版美观的 Markdown 格式）：
   - **PR 概览**：用 1-2 句话精炼总结本次 PR 的主要目的与核心改动。
   - **关键变更点**：按模块或重要程度列出主要变更及技术实现要点。
   - **潜在风险与改进建议**（如有）：指出代码中值得关注的边界情况、并发/性能、安全性、向后兼容性或重构建议；若实现良好无明显隐患，简要列出验证/测试建议。
   - **Review 结论**：给出明确的评审意见（例如：LGTM 👍、建议补充测试后合并、或需修改部分实现后合并）。
2. 保持专业、客观、友善与建设性。
3. 重点阐明“改了什么”、“为什么改”以及“潜在影响”，避免机械地逐行翻译代码。
4. 直接输出 Markdown 评审正文，不要在外部包裹多余的代码块标签（如 ` + "```markdown" + `），不要输出无关客套话。
5. 用中文回答。`

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
