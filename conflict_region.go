package main

import "strings"

// 工作区文件里的冲突标记。
//
// 冲突文件的 patch 是合并格式（diff --cc、@@@ 头），Gloss 的解析器不认，
// 所以改动区以前对冲突文件只能摆一张「挑一边」的卡片 —— 看不到两边到底
// 差在哪。这里把工作区文本里的 <<<<<<< 块自己切开，改动区就能把每一块连同
// 两侧的内容一起画出来，并给每一块配上「用这边 / 用那边」。
//
// 解析的是 git 写进工作区的那份文本（而不是 index 的三个 stage）：用户
// 看到的、要编辑的就是这一份 —— 三个 stage 只是「挑边」要用的底料，而
// 手工改过的部分（比如已经删掉某个块的标记）只在工作区里。

// conflictChoice 说一块冲突要用哪边的正文。
type conflictChoice uint8

const (
	conflictTakeOurs   conflictChoice = iota // 我们这边（git 的 --ours）
	conflictTakeTheirs                       // 对方那边（git 的 --theirs）
	conflictTakeBoth                         // 两边都要，先我们后对方
	conflictTakeNone                         // 两边都不要
)

// conflictRegion 是文本里的一块冲突：start 到 end 是**闭区间**的行下标，
// 含 <<<<<<< 与 >>>>>>> 两行标记。
type conflictRegion struct {
	start, end int
	// 两侧的正文（不含标记行）。diff3 风格（merge.conflictStyle=diff3/zdiff3）
	// 时中间还有一段共同祖先，落在 base 里。
	ours, base, theirs []string
	// 标记行后面的字：<<<<<<< 后面是当前分支名，>>>>>>> 后面是对方的名字。
	// git 写的是「<<<<<<< HEAD」这种，原样显示比我们自己编一个名字更可信。
	oursLabel, theirsLabel string
}

// oursLines 是这块冲突里「我们这边」在文本中占的行下标（含标记行）。
func (r conflictRegion) span() (int, int) { return r.start, r.end }

// parseConflictRegions 扫出文本里所有的冲突块。
//
// 认不出标记的行一律当普通内容：用户可能已经手工改过一半（删了 >>>>>>> 却
// 留着 <<<<<<<），那种半截状态不假装自己有结构 —— 少报一块，比报一块位置
// 错的好。（不完整的块会整块丢掉：宁可让用户自己去编辑器里改。）
func parseConflictRegions(lines []string) []conflictRegion {
	var out []conflictRegion
	for i := 0; i < len(lines); i++ {
		if !isMarker(lines[i], "<<<<<<<") {
			continue
		}
		r := conflictRegion{start: i, end: -1, oursLabel: markerLabel(lines[i], "<<<<<<<")}
		j := i + 1
		side := &r.ours
		for ; j < len(lines); j++ {
			switch {
			case isMarker(lines[j], "|||||||"):
				side = &r.base
			case isMarker(lines[j], "======="):
				side = &r.theirs
			case isMarker(lines[j], ">>>>>>>"):
				r.theirsLabel = markerLabel(lines[j], ">>>>>>>")
				r.end = j
			default:
				*side = append(*side, lines[j])
			}
			if r.end >= 0 {
				break
			}
		}
		if r.end < 0 {
			break // 块没写完，后面的更不可能完整
		}
		out = append(out, r)
		i = r.end
	}
	return out
}

// isMarker 判断一行是不是某个冲突标记。git 写的标记行是「<<<<<<< HEAD」
// 这样：七个符号 + 一个空格 + 名字，名字可以为空。
func isMarker(line, mark string) bool {
	return strings.HasPrefix(line, mark+" ") || line == mark
}

// markerLabel 取标记行后面的名字（没有就是空串）。
func markerLabel(line, mark string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, mark))
}

// conflictChoiceLines 返回某一选择要留下的正文。
//
// 单独一个函数：界面上「把这一块换成什么」和「把这一块换成什么文本」必须
// 是同一份答案 —— 两处各写一遍，迟早一处改了另一处没改。
func conflictChoiceLines(r conflictRegion, choice conflictChoice) []string {
	switch choice {
	case conflictTakeOurs:
		return r.ours
	case conflictTakeTheirs:
		return r.theirs
	case conflictTakeBoth:
		return append(append([]string{}, r.ours...), r.theirs...)
	}
	return nil
}

// replaceConflictRegion 把一块冲突按选择的结果换成纯正文，返回新的文本行。
//
// 下游只改这一块、其余原样搬过来：用户点了「用我这边」之后，别的块仍然
// 是冲突状态（还带着标记），要一个一个选完 —— 一次点掉全部冲突那是另一颗
// 按钮的事（顶上那条的「用当前分支的」）。
func replaceConflictRegion(lines []string, r conflictRegion, choice conflictChoice) []string {
	take := conflictChoiceLines(r, choice)
	out := make([]string, 0, len(lines)-(r.end-r.start+1)+len(take))
	out = append(out, lines[:r.start]...)
	out = append(out, take...)
	return append(out, lines[r.end+1:]...)
}

// conflictRegionHint 是挂在冲突块上方的那行说明里的两边名字。
func conflictRegionHint(r conflictRegion) (string, string) {
	ours, theirs := r.oursLabel, r.theirsLabel
	if ours == "" {
		ours = "HEAD"
	}
	if theirs == "" {
		theirs = "对方"
	}
	return ours, theirs
}
