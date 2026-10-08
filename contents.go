package main

import (
	"context"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/highlight"
)

// maxContentBytes 是读进内存做行号换算和语法高亮的上限。
// 再大的文件（生成的代码、打包产物）读进来只会拖慢界面，收益为零。
const maxContentBytes = 2 << 20

// maxHighlightBytes 是值得高亮的文件大小上限。再大的多半是生成出来的
// 代码（打包产物、协议桩），给它们做词法切分只会白白烧 CPU。
const maxHighlightBytes = 512 << 10

// contentBatch 是攒够多少个文件就往主线程交一次结果。
//
// 一个文件一次 dispatch 的话，改动多的仓库会瞬间排上百次重绘；攒批之后
// 未改动区间的行号会分几次补齐，用户几乎察觉不到。
const contentBatch = 32

// loadContents 在后台把仓库里各文件在新侧的内容读进来。
//
// 只有「展开未改动区间」和「语法高亮」需要内容，所以读不到（文件被删、
// 超过大小上限、权限不足）不算错误：那些区间保持折叠，其余部分照常显示。
//
// 读的是 allFiles 而不是 files —— 内容按路径缓存，筛选只是换个视角看
// 同一批文件，不该因为它重新读盘。
func (a *app) loadContents(seq int) {
	if a.contents == nil {
		a.contents = map[string]*fileContent{}
	}
	// 每个要读的文件记下「读哪一侧」：已暂存段的正文在 index 里（git show
	// :0:path），不在工作区 —— 半暂存的文件两者不同，而这里读错不会报错，
	// 只会把行号和颜色悄悄画错（见 gitrepo.NewLinesAt）。
	type req struct {
		key, path string
		staged    bool
	}
	var reqs []req
	// diffs 存每个文件的 hunk，后台还原旧文件的高亮要用。
	// 存值不存指针：后台 goroutine 因此完全不碰 app 上的任何东西。
	diffs := make(map[string]diff.FileDiff, len(a.allFiles))
	for i := range a.allFiles {
		f := &a.allFiles[i]
		// 删除的文件没有新侧内容；已缓存的、明确标了过大的都跳过
		p := fileKey(*f)
		if f.NewPath == "" || f.TooLarge || a.contents[p] != nil {
			continue
		}
		reqs = append(reqs, req{key: p, path: pathOf(*f), staged: f.Section == gitrepo.SectionStaged})
		diffs[p] = f.FileDiff
	}
	if len(reqs) == 0 {
		return
	}
	root := a.repoPath

	compareTarget := ""
	if a.compare.on {
		compareTarget = a.compare.target
	}

	// 后台只拿值参数，结果经 dispatch 回主线程 —— 见 app.go 顶部的线程约定
	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		if err != nil {
			return
		}

		batch := make(map[string]*fileContent, contentBatch)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			// 交给 dispatch 的是这一批的快照：batch 会继续被后面的读入填充
			done := batch
			batch = make(map[string]*fileContent, contentBatch)
			a.dispatch(func() {
				if a.contentSeq != seq {
					return // 已经切到别的仓库了，这批结果作废
				}
				for p, cc := range done {
					a.contents[p] = cc
				}
				a.rowsDirty = true
			})
		}

		for _, rq := range reqs {
			var lines []string
			if compareTarget != "" {
				lines = repo.NewLinesAt(ctx, compareTarget, rq.path, maxContentBytes)
			} else if rq.staged {
				lines = repo.NewLinesAt(ctx, ":0", rq.path, maxContentBytes)
			} else {
				lines = repo.NewLines(rq.path, maxContentBytes)
			}
			if lines == nil {
				continue
			}
			batch[rq.key] = buildContent(rq.path, lines, diffs[rq.key])
			if len(batch) >= contentBatch {
				flush()
			}
		}
		flush()
	})
}

// buildContent 把读到的内容加工成渲染要用的形状：切行 + 两侧的高亮切分。
//
// 高亮按「展开 tab 之后」的文本切分，渲染时对每一行也做同样的展开，
// 两边的字节下标才对得上。tab 展开是逐字节长度都会变的操作，
// 不做这一步的话颜色会整段错位。
func buildContent(path string, lines []string, f diff.FileDiff) *fileContent {
	fc := &fileContent{lines: lines}
	if !highlight.NeedHighlight(path) || byteLen(lines) > maxHighlightBytes {
		return fc
	}
	fc.hl = highlight.NewLines(path, expandAll(lines))

	// 删除行的高亮要整份旧文件一起切。旧文件的内容能从 diff 直接还原，
	// 不必再读一次盘。
	if !hasDeletions(f) {
		return fc
	}
	if old := gitrepo.OldLines(f, lines); old != nil {
		fc.oldHL = highlight.NewLines(path, expandAll(old))
	}
	return fc
}

// hasDeletions 判断这个文件有没有删除行 —— 没有的话不值得花时间还原旧文件。
func hasDeletions(f diff.FileDiff) bool {
	for hi := range f.Hunks {
		for li := range f.Hunks[hi].Lines {
			if f.Hunks[hi].Lines[li].Type == diff.LineDel {
				return true
			}
		}
	}
	return false
}

// byteLen 是这些行拼起来大致有多少字节（只用来判断值不值得高亮，
// 差一个换行无所谓）。
func byteLen(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}

// expandAll 把每一行的 tab 展开，返回新切片。
func expandAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = expandTabs(l)
	}
	return out
}
