// Package explain 把「一个 hunk + 它所在的文件上下文」拼成给模型的提示词，
// 并算出稳定的缓存 key。
//
// 这里全是纯函数：不碰网络、不碰界面、不碰磁盘，所以没有 API key
// 也能完整测。
package explain

import (
	"fmt"
	"strings"

	"diff-explainer/internal/diff"
)

// SystemPrompt 是解释任务的系统提示。
const SystemPrompt = `你是一位资深的 Code Review 专家，擅长用简洁清晰的中文解释代码变更。
用户会给你一个本地未提交改动中的某个代码块（hunk）的内容，以及它所在文件的上下文。

请解释：
1. **这段变更做了什么**：新增的代码实现了什么；删除的代码原本是干什么的、为什么可能被删掉
2. **变更目的**：结合文件上下文推断作者意图
3. **值得注意的点**（可选）：潜在风险、副作用或需要 reviewer 重点关注的地方，没有就省略

要求：
- 先给一句话总结，再用要点展开，总长度控制在 250 字以内
- 不要逐行翻译代码，讲清楚"为什么"比"是什么"更重要
- **只解释给你的这一个变更块**：引用它里面真实出现的函数名、变量名、字符串；上下文里带行号，需要指位置时用「第 N 行」这种说法。不要泛泛介绍整个文件，也不要解释同一个文件里的其它变更块
- 如果从文件上下文能看出这个改动的调用方、被调用方或影响面，用一句话点明；看不出来就不要猜
- 不要复述用户给你的 diff 文本，也不要用"如下"之类的开场白
- 用中文回答`

// Request 描述要解释的一个变更块。
type Request struct {
	File diff.FileDiff
	Hunk diff.Hunk
	// HunkIdx 是它在文件里的序号（从 0 开始），用来告诉模型「只解释这一个」。
	HunkIdx int
	// Context 是 hunk 附近的文件内容，每行行首带真实行号。可以为空。
	Context string
}

// CacheKey 是解释结果的缓存 key：hunk 的内容哈希 + 模型名。
//
// 用内容哈希而不是行号，所以同一个改动块出现在哪个文件位置、哪次提交里
// 都能命中同一份缓存；而换了模型会重新问一遍 —— 不同模型的解释不该互相顶替。
func CacheKey(f diff.FileDiff, h diff.Hunk, model string) string {
	return diff.HunkID(Path(f), h) + ":" + model
}

// BuildPrompt 拼出 (system, user) 两条消息。
func BuildPrompt(req Request) (string, string) {
	f, h := req.File, req.Hunk
	path := Path(f)

	var b strings.Builder
	fmt.Fprintf(&b, "## 文件\n路径：%s（%s，+%d -%d）\n", DisplayPath(f), statusText(f.Status), f.AddCount, f.DelCount)

	// 一个文件常有多个变更块：点名当前是哪一个，并列出其它块的位置，
	// 避免模型把同文件其它改动也当成这次的解释对象。
	if len(f.Hunks) > 1 {
		fmt.Fprintf(&b, "该文件共有 %d 个变更块，下面要给解释的是第 %d 个。其它变更块的位置（不要解释它们）：\n",
			len(f.Hunks), req.HunkIdx+1)
		for i := range f.Hunks {
			if i == req.HunkIdx {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", f.Hunks[i].Header)
		}
	}
	b.WriteByte('\n')

	if req.Context != "" {
		fmt.Fprintf(&b, "以下是该文件的相关内容（每行行首的 N| 是文件里的真实行号）：\n```%s\n%s\n```\n\n",
			Lang(path), req.Context)
	}

	fmt.Fprintf(&b, "## 需要解释的变更块（老文件第 %d 行起 %d 行 → 新文件第 %d 行起 %d 行）\n```diff\n",
		h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	b.WriteString(h.Raw())
	b.WriteString("```\n")
	return SystemPrompt, b.String()
}

// Window 截取文件中 hunk 前后 around 行的内容，行首带上真实行号。
//
// 带行号是为了让模型能精确指认「第几行」，也更容易把改动和上下文对上。
// 超过 maxChars 就按行边界截断，不切断某一行。
func Window(lines []string, h diff.Hunk, around, maxChars int) string {
	var start, end int
	if h.NewStart > 0 {
		start = max(0, h.NewStart-1-around)
		end = min(len(lines), h.NewStart+h.NewLines+around)
	} else {
		// 纯删除的 hunk 在新侧没有行号，按旧侧算
		start = max(0, h.OldStart-1-around)
		end = min(len(lines), h.OldStart+h.OldLines+around)
	}
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

// Path 是这个变更块所属的文件路径。
func Path(f diff.FileDiff) string { return f.Path() }

// DisplayPath 在重命名时把两个路径都写出来。
func DisplayPath(f diff.FileDiff) string {
	if f.Status == "renamed" && f.OldPath != "" && f.NewPath != "" {
		return f.OldPath + " → " + f.NewPath
	}
	return Path(f)
}

func statusText(s string) string {
	switch s {
	case "added":
		return "新增"
	case "deleted":
		return "删除"
	case "renamed":
		return "重命名"
	}
	return "修改"
}

// langOf 是常见后缀到 markdown 代码块标记的映射。
var langOf = map[string]string{
	"go": "go", "ts": "typescript", "tsx": "typescript", "js": "javascript", "jsx": "javascript",
	"vue": "vue", "py": "python", "java": "java", "rs": "rust", "rb": "ruby", "php": "php",
	"c": "c", "h": "c", "cpp": "cpp", "hpp": "cpp", "cc": "cpp", "cs": "csharp",
	"css": "css", "scss": "scss", "html": "html", "json": "json", "md": "markdown",
	"yml": "yaml", "yaml": "yaml", "toml": "toml", "sh": "bash", "sql": "sql",
	"swift": "swift", "kt": "kotlin",
}

// Lang 由路径猜出代码块的语言标记，猜不出返回空串。
func Lang(path string) string {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return ""
	}
	return langOf[strings.ToLower(path[i+1:])]
}
