package main

import (
	"fmt"

	"diff-explainer/internal/diff"
	"diff-explainer/internal/gitrepo"
	"diff-explainer/internal/highlight"
)

// 扁平行模型：把整个 diff（所有文件）摊成一维的行数组，喂给虚拟化的
// ui.List。List 只构建可见行，所以几万行的 diff 和几百行一样快。
//
// 这是派生数据，不是持久状态：文件列表、展开状态、视图模式一变就整体重建
// （见 app.go 的 rowsDirty）。不要试图增量修改它。

type rowKind uint8

const (
	rowFileHeader rowKind = iota // 文件头，滚动时钉在顶部
	rowHunkHeader                // @@ 那一行
	rowLine                      // 一行代码
	rowGap                       // 「N 行未改动」的折叠条
	rowNote                      // 二进制文件、文件过大之类的提示
)

const (
	// expandStep 是每次点「展开」放出来的行数。一次全放出来会让滚动位置
	// 突然跳很远，分步展开更符合直觉。
	expandStep = 100

	// inlineGap 以内的未改动区间直接摊开显示，不折叠成条。
	// 只有一两行没改动还要再点一次展开，纯属添乱。
	inlineGap = 12
)

type row struct {
	kind rowKind
	file int // 在 files 里的下标

	// rowHunkHeader / rowLine：hunk 下标，以及在 hunk.Lines 里的下标。
	//
	// split 模式下一条行左右各放一行代码：右边用 hunk/lineIdx（和 unified
	// 一致），左边用 lhunk/lidx。某一侧为 -1 表示那一侧空着（对面是新增
	// 或删除行）。unified 模式下只有右边那侧有意义。
	hunk    int
	lineIdx int
	lhunk   int
	lidx    int

	// rowLine 来自展开的未改动区间时，内容不在 patch 里，得按行号去
	// 文件内容里取；newNo/oldNo 是两个侧的真实行号。
	fromGap bool
	newNo   int
	oldNo   int

	// rowGap：折叠条代表的区间
	gapKey string
	gapN   int // 还折叠着多少行

	note string // rowNote 的文字

	key string // ui.ListState.Key 用，必须全局唯一
}

// fileContent 是一个文件在新侧的内容，按行切好，外加两侧的高亮切分。
//
// 整个值为 nil 表示内容还没加载到（或文件太大没读）：未改动区间照样折叠
// 显示，只是点不开，代码也不上色。
type fileContent struct {
	lines []string
	// hl / oldHL 是按整份文件切好的 token，渲染时按行号取用。
	// oldHL 可能为 nil（没有删除行，或旧文件还原不出来），此时删除行
	// 用默认前景色画。
	hl    *highlight.File
	oldHL *highlight.File
}

// buildRows 把文件列表摊成扁平行数组。
//
// contents 按文件路径索引（不是切片下标）：侧栏筛选会让文件列表变短，
// 用下标当 key 的话展开状态会整体错位。
//
// expanded 记录每个未改动区间已经展开了多少行，key 见 gapKeyOf。
// 它跨帧保留，所以用户展开过的区间在重绘、筛选、刷新之后仍然是展开的。
//
// split 为 true 时按左右对照排布：一段删除和紧跟其后的新增一一配对，
// 配不上的那一侧留空。
func buildRows(files []gitrepo.File, contents map[string]*fileContent, expanded map[string]int, split bool) []row {
	var rows []row
	for fi := range files {
		f := &files[fi]
		path := fileKey(*f)

		rows = append(rows, row{
			kind: rowFileHeader,
			file: fi,
			key:  "h:" + path,
		})

		if len(f.Hunks) == 0 {
			rows = append(rows, row{
				kind: rowNote,
				file: fi,
				note: noteFor(*f),
				key:  "n:" + path,
			})
			continue
		}

		var lines []string
		if cc := contents[path]; cc != nil {
			lines = cc.lines
		}

		// delta 是到目前为止「新侧比旧侧多出来的行数」。未改动区间在两侧
		// 的行号不同，靠它换算：旧行号 = 新行号 − delta。
		delta := 0
		prevEnd := 0 // 上一个 hunk 覆盖到的最后一个新侧行号

		for hi := range f.Hunks {
			h := &f.Hunks[hi]

			rows = appendGap(rows, fi, path, prevEnd, h.NewStartLine(), delta, expanded, lines)

			rows = append(rows, row{
				kind: rowHunkHeader,
				file: fi,
				hunk: hi,
				key:  "k:" + h.ID,
			})
			rows = appendHunkLines(rows, fi, hi, h, split)

			delta += h.NewLines - h.OldLines
			prevEnd = h.NewEndLine()
		}

		// 末尾的未改动区间：要等知道文件总行数才算得出来
		if len(lines) > 0 {
			rows = appendGap(rows, fi, path, prevEnd, len(lines)+1, delta, expanded, lines)
		}
	}
	return rows
}

// appendHunkLines 把一个 hunk 的行加进行模型。
func appendHunkLines(rows []row, fi, hi int, h *diff.Hunk, split bool) []row {
	if !split {
		for li := range h.Lines {
			rows = append(rows, row{
				kind:    rowLine,
				file:    fi,
				hunk:    hi,
				lineIdx: li,
				lhunk:   -1,
				lidx:    -1,
				key:     fmt.Sprintf("l:%s:%d", h.ID, li),
			})
		}
		return rows
	}

	for _, p := range pairHunk(*h) {
		rows = append(rows, row{
			kind:    rowLine,
			file:    fi,
			hunk:    hi,
			lineIdx: p.right,
			lhunk:   hi,
			lidx:    p.left,
			// key 取两侧里较大的那个下标：它在 hunk 内唯一，而且和
			// unified 模式下同一行的 key 一致 —— 切换视图时列表的
			// 滚动位置因此不会跳。
			key: fmt.Sprintf("l:%s:%d", h.ID, max(p.left, p.right)),
		})
	}
	return rows
}

// pair 是 split 模式下一行左右两侧的内容来源，-1 表示那一侧空着。
type pair struct{ left, right int }

// pairHunk 把 hunk 里的行按「先删后增」配对。
//
// 一段删除和紧随其后的一段新增一一对齐，改一行就显示成一行；多出来的一侧
// 留空。未改动的行两侧都占，行号各按各的算。
func pairHunk(h diff.Hunk) []pair {
	var out []pair
	var dels, adds []int

	flush := func() {
		for i := 0; i < max(len(dels), len(adds)); i++ {
			p := pair{left: -1, right: -1}
			if i < len(dels) {
				p.left = dels[i]
			}
			if i < len(adds) {
				p.right = adds[i]
			}
			out = append(out, p)
		}
		dels, adds = dels[:0], adds[:0]
	}

	for i := range h.Lines {
		switch h.Lines[i].Type {
		case diff.LineDel:
			dels = append(dels, i)
		case diff.LineAdd:
			adds = append(adds, i)
		default:
			// 未改动行把前后两段增删隔开，不能跨过去配对
			flush()
			out = append(out, pair{left: i, right: i})
		}
	}
	flush()
	return out
}

// gapKeyOf 是未改动区间的稳定标识。
func gapKeyOf(path string, from int) string {
	return fmt.Sprintf("g:%s:%d", path, from)
}

// appendGap 在 prevEnd 和 nextStart 之间插入未改动区间。
// prevEnd 是上一个 hunk 结束的新侧行号（0 表示文件开头），
// nextStart 是下一个 hunk 开始的新侧行号（文件行数+1 表示文件末尾）。
func appendGap(rows []row, fi int, path string, prevEnd, nextStart, delta int, expanded map[string]int, lines []string) []row {
	from := prevEnd + 1
	n := nextStart - from
	if n <= 0 {
		return rows
	}

	// 区间短就直接摊开，不值得折一条
	if n <= inlineGap {
		return appendGapLines(rows, fi, path, from, n, delta, lines)
	}

	key := gapKeyOf(path, from)
	shown := expanded[key]
	if shown > n {
		shown = n
	}
	rows = appendGapLines(rows, fi, path, from, shown, delta, lines)
	if shown < n {
		rows = append(rows, row{
			kind:   rowGap,
			file:   fi,
			gapKey: key,
			gapN:   n - shown,
			key:    "gap:" + key,
		})
	}
	return rows
}

// appendGapLines 把区间里已经展开的 shown 行插进去。
// lines 为 nil 时（内容没加载到）插占位行，内容留空 —— 行号仍然对得上。
func appendGapLines(rows []row, fi int, path string, from, shown, delta int, lines []string) []row {
	for i := 0; i < shown; i++ {
		newNo := from + i
		rows = append(rows, row{
			kind:    rowLine,
			file:    fi,
			fromGap: true,
			newNo:   newNo,
			oldNo:   newNo - delta,
			lhunk:   -1,
			lidx:    -1,
			key:     fmt.Sprintf("gl:%s:%d", path, newNo),
		})
	}
	return rows
}

// fileKey 是文件的稳定标识。用新路径，删除的文件退回旧路径。
func fileKey(f gitrepo.File) string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// noteFor 给没有 hunk 的文件生成一条说明。
func noteFor(f gitrepo.File) string {
	switch {
	case f.Binary:
		return "二进制文件已变更"
	case f.TooLarge:
		return "文件过大，未显示内容"
	case f.Status == gitrepo.StatusRenamed:
		return "仅重命名，内容没有变化"
	}
	return "没有可显示的文本差异"
}
