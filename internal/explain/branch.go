package explain

import (
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// BranchSystemPrompt 是推荐 Git 分支名的系统提示。
const BranchSystemPrompt = `You are a professional Git branch naming expert.
Your task is to suggest a clean, concise, standards-compliant branch name based on the user's requirements, feature intent, or code changes (diff).

Naming conventions:
1. Use lower-case kebab-case format. Common type prefixes:
   - feat/xxx (new feature)
   - fix/xxx (bug fix)
   - refactor/xxx (code refactoring)
   - perf/xxx (performance improvement)
   - docs/xxx (documentation)
   - chore/xxx (maintenance, build scripts, dependency updates)
   - test/xxx (testing)
2. Connect words with hyphens "-". Do not use underscores "_" or spaces.
3. Strictly follow Git ref naming rules: no spaces, consecutive dots "..", "~", "^", ":", "?", "*", "[", "\", "@{ ", "//"; must not start or end with "/", "-", or ".".
4. Keep it moderately sized, concisely expressing the core intent (recommended 15-30 characters).
5. Output only the final branch name. Never output any Markdown formatting, code block tags, quotation marks, extra explanations, or greetings.`

// maxBranchDiffChars 是生成分支名时 diff 的文本上限。
const maxBranchDiffChars = 8000

// BuildBranchPrompt 构建生成分支名的提示词。
func BuildBranchPrompt(hint string, files []diff.FileDiff) (systemPrompt, userPrompt string) {
	var b strings.Builder

	if h := strings.TrimSpace(hint); h != "" {
		fmt.Fprintf(&b, "## 功能需求 / 分支意图\n%s\n\n", h)
	}

	if len(files) > 0 {
		fmt.Fprintf(&b, "## 关联代码改动（共 %d 个文件）：\n", len(files))
		truncated := false
		for _, f := range files {
			if truncated {
				break
			}
			fmt.Fprintf(&b, "- %s（%s，+%d -%d）\n", DisplayPath(f), statusText(f.Status), f.AddCount, f.DelCount)
			if f.Binary || len(f.Hunks) == 0 {
				continue
			}
			for _, h := range f.Hunks {
				raw := h.Raw()
				if b.Len()+len(raw) > maxBranchDiffChars {
					truncated = true
					b.WriteString("  …（部分 diff 已省略）\n")
					break
				}
				b.WriteString("```diff\n")
				b.WriteString(raw)
				b.WriteString("```\n")
			}
		}
	}

	b.WriteString("\n请基于上述信息，给出一个最合适的 Git 分支名称（例如 feat/user-profile 或 fix/login-redirect）。只输出分支名本身。")

	return BranchSystemPrompt, b.String()
}

// CleanBranchName 清理模型输出，提取纯净、合法的 git 分支名。
func CleanBranchName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// 如果输出被 Markdown 代码块包裹，提取内部文本
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		var content []string
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "```") {
				continue
			}
			if trimmed != "" {
				content = append(content, trimmed)
			}
		}
		if len(content) > 0 {
			s = content[0]
		}
	}

	// 只取第一行非空内容
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			s = l
			break
		}
	}

	// 剥离常见的命令前缀或标签前缀
	for {
		lower := strings.ToLower(s)
		switch {
		case strings.HasPrefix(lower, "git checkout -b "):
			s = strings.TrimSpace(s[len("git checkout -b "):])
		case strings.HasPrefix(lower, "git branch "):
			s = strings.TrimSpace(s[len("git branch "):])
		case strings.HasPrefix(lower, "git switch -c "):
			s = strings.TrimSpace(s[len("git switch -c "):])
		case strings.HasPrefix(lower, "branch:"):
			s = strings.TrimSpace(s[len("branch:"):])
		case strings.HasPrefix(lower, "branch :"):
			s = strings.TrimSpace(s[len("branch :"):])
		case strings.HasPrefix(s, "分支名：") || strings.HasPrefix(s, "分支名:"):
			idx := strings.Index(s, ":")
			if idx < 0 {
				idx = strings.Index(s, "：")
				if idx >= 0 {
					s = strings.TrimSpace(s[idx+len("："):])
					continue
				}
			} else {
				s = strings.TrimSpace(s[idx+1:])
				continue
			}
		case strings.HasPrefix(s, "分支：") || strings.HasPrefix(s, "分支:"):
			idx := strings.Index(s, ":")
			if idx < 0 {
				idx = strings.Index(s, "：")
				if idx >= 0 {
					s = strings.TrimSpace(s[idx+len("："):])
					continue
				}
			} else {
				s = strings.TrimSpace(s[idx+1:])
				continue
			}
		case strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* "):
			s = strings.TrimSpace(s[2:])
		default:
			goto stripped
		}
	}
stripped:

	// 去除各种引号与包裹字符
	s = strings.Trim(s, " `\"'()[]{}<>;,.")
	s = strings.ToLower(s)

	// 转换为合法的 Git ref 字符集
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_':
			b.WriteByte(c)
		case c == '/':
			// 避免连续的 // 或以 / 开头
			if b.Len() == 0 || strings.HasSuffix(b.String(), "/") {
				continue
			}
			b.WriteByte('/')
		case c == '-' || c == ' ' || c == '\t' || c == '~' || c == '^' || c == ':' || c == '?' || c == '*' || c == '[' || c == '\\' || c == '@':
			// 连续分隔符折叠为一个 -
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") && !strings.HasSuffix(b.String(), "/") {
				b.WriteByte('-')
			}
		case c == '.':
			// 避免连续的 ..
			if b.Len() > 0 && !strings.HasSuffix(b.String(), ".") {
				b.WriteByte('.')
			}
		default:
			if c < 128 {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") && !strings.HasSuffix(b.String(), "/") {
					b.WriteByte('-')
				}
			}
		}
	}

	result := b.String()
	// 避免以 .lock 结尾
	for strings.HasSuffix(result, ".lock") {
		result = strings.TrimSuffix(result, ".lock")
	}
	// 清理头尾的非法字符：/ - .
	result = strings.Trim(result, "/-.")

	return result
}
