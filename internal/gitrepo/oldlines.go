package gitrepo

import (
	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// OldLines 由新侧内容和 hunk 反推出旧侧内容。
//
// 删除的行在界面上也要高亮，而高亮必须整份文件一起切（多行字符串、
// 块注释单看一行是判断不出来的）。旧文件的内容本来要再跑一次
// `git show` 才能拿到，但这里能直接从 diff 里还原：hunk 里每个删除行的
// 原文都在，没被 hunk 覆盖的位置两侧内容相同 —— 拼回去就是旧文件。
//
// newLines 是文件在新侧的内容（newLines[0] 是第 1 行）。
// 对不齐时返回 nil，调用方按「没有旧侧内容」处理即可。
func OldLines(f diff.FileDiff, newLines []string) []string {
	if len(f.Hunks) == 0 || len(newLines) == 0 {
		return nil
	}
	out := make([]string, 0, len(newLines))
	next := 1 // 下一个要处理的新侧行号
	for hi := range f.Hunks {
		h := &f.Hunks[hi]
		start, end := h.NewStartLine(), h.NewEndLine()

		// hunk 之前那段没改动：两侧一样，从新侧抄
		for ; next < start; next++ {
			if next-1 >= len(newLines) {
				return nil
			}
			out = append(out, newLines[next-1])
		}
		// hunk 本身：上下文行照抄，删除行取原文（新增行在旧侧不存在）
		for i := range h.Lines {
			if h.Lines[i].Type != diff.LineAdd {
				out = append(out, h.Lines[i].Content)
			}
		}
		next = end + 1
	}
	// 最后一个 hunk 之后的部分
	for ; next <= len(newLines); next++ {
		out = append(out, newLines[next-1])
	}
	return out
}
