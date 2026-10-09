package explain

import (
	"fmt"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// BranchSystemPrompt 是推荐 Git 分支名的系统提示。
const BranchSystemPrompt = `你是一位专业的 Git 分支命名专家。
你的任务是根据用户的需求描述、功能意图或代码变更（diff），建议一个规范、精练、符合 Git 标准的分支名称。

命名规范：
1. 采用 lower-case kebab-case 格式，常用类型前缀包括：
   - feat/xxx (新功能或特性)
   - fix/xxx (缺陷修复)
   - refactor/xxx (代码重构)
   - perf/xxx (性能优化)
   - docs/xxx (文档更新)
   - chore/xxx (日常维护、构建脚本或依赖升级)
   - test/xxx (测试相关)
2. 单词之间用连字符 "-" 连接，不要用下划线 "_" 或空格。
3. 严格遵守 Git ref 命名规则：禁止包含空格、连续的点 ".."、"~"、"^"、":"、"?"、"*"、"["、"\"、"@{ "、"//"；不能以 "/"、"-" 或 "." 开头或结尾。
4. 长度适中，精炼表达核心意图（建议 15~30 字符）。
5. 严格仅输出最终的分支名，绝对不要输出任何 Markdown 格式、代码块标签、引号包裹、额外解释或问候语。`

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
