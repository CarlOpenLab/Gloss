package diff

import (
	"fmt"
	"strings"
)

// HunkRange 是 DiffView 给的一段改动：两个侧各一个 0 基、末尾开的行区间。
//
// 0 基 + 末尾开是 editor.DiffView 的口径，和 git 补丁头里的 1 基闭区间差
// 一个数；换算只发生在 RangePatch 内部，调用方不要自己加减一。
type HunkRange struct{ OldStart, OldEnd, NewStart, NewEnd int }

// noNewlineMark 是 git 用来标注「这一行在文件末尾没有换行符」的那一行。
const noNewlineMark = `\ No newline at end of file`

// RangePatch 生成只包含这一段改动的补丁：path 是仓库内相对路径，
// oldText/newText 是两个侧的**完整原文**（带不带末尾换行由它们自己说）。
//
// 为什么要有它：DiffView 的段是 mygo 自己的算法（histogram + indent 启发）
// 划出来的，和 git 的 -U3 hunk 不保证一一对应，所以不能像 StageHunk 那样
// 重跑 git diff 再按 hunk ID 切片，只能按行区间现造补丁。
//
// 补丁只描述这一段 —— 同文件里别的改动绝不能混进来，这是这个函数的全部
// 意义。git apply 是按行号 + 上下文在目标文件里定位的，多带一段就等于把
// 用户没点的改动也一起改了。
//
// 上下文取旧侧：补丁最终打的是旧侧那份文件（未暂存方向打 index、已暂存方向
// 打 HEAD），git 拿这些行去比对的就是它；从新侧取会在「另一侧先被改过」时
// 对不上。行号按旧侧真实行号（1 基）写头，同理。
//
// 区间越界（或者压根不是一段改动）返回空串，不 panic：调用方拿来的是「点
// 下去那一刻」的行号，文件随后变了就是这个样子，上层据此报「这一段已经变了」。
func RangePatch(path, oldText, newText string, r HunkRange) string {
	if path == "" || r.OldStart < 0 || r.OldEnd < r.OldStart || r.NewStart < 0 || r.NewEnd < r.NewStart {
		return ""
	}
	oldLines, oldNL := splitLines(oldText)
	newLines, newNL := splitLines(newText)
	if r.OldEnd > len(oldLines) || r.NewEnd > len(newLines) {
		return ""
	}
	// 两侧都是空区间 = 没有改动。算出来的补丁体是空的，git 会当成坏补丁
	// 拒收，空串的语义（这段已经变了）反而更接近事实。
	if r.OldStart == r.OldEnd && r.NewStart == r.NewEnd {
		return ""
	}

	// 上下文各最多 3 行（git 的默认，也是用户肉眼能核对一段的尺度）。上限
	// 同时受两侧剩余行数约束：新侧行号是从新侧起点往回数的，只按旧侧取会
	// 算出 0 或负数。
	//
	// 还要收在「两侧真的一模一样」的范围里：DiffView 的两段改动可能只隔
	// 一两行（它按自己的算法划段，不保证相隔 2×上下文），那两行对其中一段
	// 来说挨在旁边、对另一段来说正是改动本身。拿旧侧的改动行当上下文写进
	// 补丁，正向 apply（打 index）能蒙对，反向 apply（撤出 index）就会
	// 「patch does not apply」—— 目标文件上那一行已经不是这个内容了。
	before := min(3, r.OldStart, r.NewStart)
	for before > 0 && oldLines[r.OldStart-before] != newLines[r.NewStart-before] {
		before--
	}
	after := min(3, len(oldLines)-r.OldEnd, len(newLines)-r.NewEnd)
	for k := range after {
		if oldLines[r.OldEnd+k] != newLines[r.NewEnd+k] {
			after = k
			break
		}
	}

	var body []string
	// marks[i] 为真：body[i] 这一行在它所属的那侧文件里没有末尾换行，
	// 后面要跟 noNewlineMark。
	marks := map[int]bool{}
	// 两侧各自「最后一条被写进补丁的行」是哪一行（0 基行号）和它在 body 里
	// 的下标 —— 只有文件的最后一行才可能缺换行，所以只需要看这两个。
	oldLast, newLast := -1, -1
	oldAt, newAt := -1, -1

	for i := range before {
		body = append(body, " "+oldLines[r.OldStart-before+i])
	}
	if before > 0 {
		oldLast, newLast = r.OldStart-1, r.NewStart-1
		oldAt, newAt = before-1, before-1
	}
	// 先删后加：git 自己也是这么排的，混排会让「读补丁」和「比对原文」都变难。
	for i := r.OldStart; i < r.OldEnd; i++ {
		body = append(body, "-"+oldLines[i])
		oldLast = i
		oldAt = len(body) - 1
	}
	for i := r.NewStart; i < r.NewEnd; i++ {
		body = append(body, "+"+newLines[i])
		newLast = i
		newAt = len(body) - 1
	}
	for i := range after {
		body = append(body, " "+oldLines[r.OldEnd+i])
	}
	if after > 0 {
		oldLast, newLast = r.OldEnd+after-1, r.NewEnd+after-1
		oldAt, newAt = len(body)-1, len(body)-1
	}
	// 缺末尾换行的标记要跟在那侧的最后一条补丁行后面，两侧各自判断。末尾
	// 的上下文行是两侧共用的，「旧侧缺、新侧也缺」时 marks 里的同一个下标
	// 只写一次标记（git 也只写一次）。
	if oldAt >= 0 && oldLast == len(oldLines)-1 && !oldNL {
		marks[oldAt] = true
	}
	if newAt >= 0 && newLast == len(newLines)-1 && !newNL {
		marks[newAt] = true
	}

	// 行数为 0 的那一侧，git 写的是「插入点之前那一行」的号 —— 0 基的起点
	// 本身，所以 `@@ -0,0 +1,3 @@`（在空文件开头插入）里旧侧写 0；反过来
	// 纯删除 `@@ -1,2 +0,0 @@` 新侧写 0。
	oldCount := before + (r.OldEnd - r.OldStart) + after
	newCount := before + (r.NewEnd - r.NewStart) + after
	oldFrom, newFrom := r.OldStart-before, r.NewStart-before
	oldHead, newHead := oldFrom, newFrom
	if oldCount > 0 {
		oldHead++
	}
	if newCount > 0 {
		newHead++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", path, path)
	if oldText == "" {
		b.WriteString("new file mode 100644\n")
	}
	if newText == "" {
		b.WriteString("deleted file mode 100644\n")
	}
	// 旧侧整份原文为空 = 这个文件在旧侧还不存在，按 git 的规矩写 /dev/null
	// （它是「新增」这件事在补丁里的唯一表达）。两侧能这么判断是因为这里
	// 拿到的是两份完整原文，而不是某一段。
	if oldText == "" {
		fmt.Fprintf(&b, "--- /dev/null\n")
	} else {
		fmt.Fprintf(&b, "--- a/%s\n", path)
	}
	if newText == "" {
		fmt.Fprintf(&b, "+++ /dev/null\n")
	} else {
		fmt.Fprintf(&b, "+++ b/%s\n", path)
	}
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldHead, oldCount, newHead, newCount)
	for i, line := range body {
		b.WriteString(line)
		b.WriteByte('\n')
		if marks[i] {
			b.WriteString(noNewlineMark)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// splitLines 把一份原文切成行，并告诉调用方最后一行有没有末尾换行。
//
// 按 "\n" 切而不是走 bufio.Scanner：Scanner 的 ScanLines 会吃掉行尾的 \r，
// 那样的行写进补丁里就变了字节，apply 出来的文件和用户手上的不是一份。
// （CRLF 文件的每行带着 \r，正好原样保留。）
func splitLines(s string) (lines []string, finalNewline bool) {
	if s == "" {
		return nil, true // 空文件没有「最后一行」，谈不上缺不缺换行
	}
	parts := strings.Split(s, "\n")
	if last := parts[len(parts)-1]; last == "" {
		return parts[:len(parts)-1], true
	}
	return parts, false
}
