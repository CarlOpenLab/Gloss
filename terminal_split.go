package main

import (
	"math"

	"github.com/egoist/mygo/plugins/terminal"
)

// paneDir 是分屏与焦点移动的方向。
type paneDir int

const (
	paneLeft paneDir = iota
	paneRight
	paneUp
	paneDown
)

// termPane 是一格终端：一个 shell，和它在分屏树里的位置分开记。
// term 为 nil 且 started 为 false 时表示「还没起」；起失败时 err 有话说。
type termPane struct {
	term    *terminal.Terminal
	err     string
	started bool
}

// paneNode 是一棵分屏树的节点：要么是一格终端（pane 非空），要么是一道
// 分叉（first / second 各一半）。
//
//	stack  false = 左右并排（Row），true = 上下叠（Column）
//	ratio  first 占的比例，另一侧是 1-ratio
type paneNode struct {
	pane          *termPane
	stack         bool
	ratio         float32
	first, second *paneNode
	parent        *paneNode
}

// splitPaneAt 在 at 那一格上分出 dir 方向的第二格，返回新的树根和新格。
//
// 新格落在方向那一侧：向右 / 向下时新格在后，向左 / 向上时新格在前。
func splitPaneAt(root, at *paneNode, dir paneDir, p *termPane) (*paneNode, *paneNode) {
	if at == nil {
		return root, nil
	}
	parent := at.parent
	leaf := &paneNode{pane: p}
	split := &paneNode{stack: dir == paneUp || dir == paneDown, ratio: 0.5}
	if dir == paneLeft || dir == paneUp {
		split.first, split.second = leaf, at
	} else {
		split.first, split.second = at, leaf
	}
	split.first.parent = split
	split.second.parent = split

	if parent == nil {
		return split, leaf
	}
	if parent.first == at {
		parent.first = split
	} else {
		parent.second = split
	}
	split.parent = parent
	return root, leaf
}

// closePaneAt 从树里去掉一格，把它所在的那道分叉塌成兄弟，返回新的树根和
// 该接住焦点的格子（兄弟子树里最靠前的一格）。
//
// 关掉的是最后一格时返回空树。
func closePaneAt(root, at *paneNode) (*paneNode, *paneNode) {
	if at == nil || at.pane == nil {
		return root, nil
	}
	if at.parent == nil {
		return nil, nil
	}
	parent := at.parent
	sibling := parent.first
	if sibling == at {
		sibling = parent.second
	}
	grand := parent.parent
	sibling.parent = grand
	if grand == nil {
		root = sibling
	} else if grand.first == parent {
		grand.first = sibling
	} else {
		grand.second = sibling
	}
	at.parent = nil
	return root, firstLeaf(sibling)
}

// firstLeaf 返回子树里最靠前的一格。
func firstLeaf(n *paneNode) *paneNode {
	for n != nil && n.pane == nil {
		n = n.first
	}
	return n
}

// paneRect 是分屏树在归一化坐标里的位置：x / y / w / h 都在 [0, 1]。
type paneRect struct{ x, y, w, h float32 }

// paneRects 按比例把树摊平成每一格的位置。焦点移动靠它按几何位置判断
// 「哪一格在旁边」，而不是靠树的先后顺序 —— 上下两半的比例不同时，顺序
// 和眼睛看到的方向不是一回事。
func paneRects(root *paneNode) map[*paneNode]paneRect {
	rects := make(map[*paneNode]paneRect)
	var walk func(n *paneNode, r paneRect)
	walk = func(n *paneNode, r paneRect) {
		if n == nil {
			return
		}
		if n.pane != nil {
			rects[n] = r
			return
		}
		if n.stack {
			h := r.h * n.ratio
			walk(n.first, paneRect{r.x, r.y, r.w, h})
			walk(n.second, paneRect{r.x, r.y + h, r.w, r.h - h})
			return
		}
		w := r.w * n.ratio
		walk(n.first, paneRect{r.x, r.y, w, r.h})
		walk(n.second, paneRect{r.x + w, r.y, r.w - w, r.h})
	}
	walk(root, paneRect{0, 0, 1, 1})
	return rects
}

// focusPaneInDir 返回 from 在 dir 方向上相邻的格子；那个方向没有格子时
// 返回 nil。
//
// 先看方向上的空隙，空隙一样近再看垂直方向的偏移：两格并排时，正对着的
// 那一格比斜对面的更该接住焦点。
func focusPaneInDir(root, from *paneNode, dir paneDir) *paneNode {
	rects := paneRects(root)
	fr, ok := rects[from]
	if !ok {
		return nil
	}
	fx, fy := fr.x+fr.w/2, fr.y+fr.h/2
	var best *paneNode
	var bestGap, bestOffset float32
	for n, r := range rects {
		if n == from {
			continue
		}
		cx, cy := r.x+r.w/2, r.y+r.h/2
		var gap, offset float32
		switch dir {
		case paneLeft:
			if cx >= fx-0.0001 || overlap1D(r.y, r.y+r.h, fr.y, fr.y+fr.h) <= 0 {
				continue
			}
			gap = max(0, fr.x-(r.x+r.w))
			offset = float32(math.Abs(float64(cy - fy)))
		case paneRight:
			if cx <= fx+0.0001 || overlap1D(r.y, r.y+r.h, fr.y, fr.y+fr.h) <= 0 {
				continue
			}
			gap = max(0, r.x-(fr.x+fr.w))
			offset = float32(math.Abs(float64(cy - fy)))
		case paneUp:
			if cy >= fy-0.0001 || overlap1D(r.x, r.x+r.w, fr.x, fr.x+fr.w) <= 0 {
				continue
			}
			gap = max(0, fr.y-(r.y+r.h))
			offset = float32(math.Abs(float64(cx - fx)))
		case paneDown:
			if cy <= fy+0.0001 || overlap1D(r.x, r.x+r.w, fr.x, fr.x+fr.w) <= 0 {
				continue
			}
			gap = max(0, r.y-(fr.y+fr.h))
			offset = float32(math.Abs(float64(cx - fx)))
		}
		if best == nil || gap < bestGap-0.0001 || (math.Abs(float64(gap-bestGap)) <= 0.0001 && offset < bestOffset) {
			best, bestGap, bestOffset = n, gap, offset
		}
	}
	return best
}

// overlap1D 是两条线段重叠的长度，不重叠时为负。
func overlap1D(a0, a1, b0, b1 float32) float32 {
	return min(a1, b1) - max(a0, b0)
}

// 分屏比例的下限：两边各留 10%，再小就点不中、也读不了。
const (
	paneRatioMin = float32(0.1)
	paneRatioMax = float32(0.9)
)

// resizePaneAt 把离 at 最近、方向也对得上的那道分隔线挪 delta。
//
// at 在哪一侧不影响方向：按「分隔线往哪边走」算，和拖分隔线的手感一致。
func resizePaneAt(root, at *paneNode, dir paneDir, delta float32) {
	if at == nil {
		return
	}
	stack := dir == paneUp || dir == paneDown
	for n := at.parent; n != nil; n = n.parent {
		if n.stack != stack {
			continue
		}
		if dir == paneRight || dir == paneDown {
			n.ratio = clamp(n.ratio+delta, paneRatioMin, paneRatioMax)
		} else {
			n.ratio = clamp(n.ratio-delta, paneRatioMin, paneRatioMax)
		}
		return
	}
}

// equalizePanes 把整棵树里每一道分叉都调回一半。
func equalizePanes(root *paneNode) {
	if root == nil || root.pane != nil {
		return
	}
	root.ratio = 0.5
	equalizePanes(root.first)
	equalizePanes(root.second)
}

// paneLeaves 按树里的顺序返回所有格子。配色、字体、关会话这些「对每一格
// 都做一遍」的事走这里。
func paneLeaves(root *paneNode) []*paneNode {
	var out []*paneNode
	var walk func(n *paneNode)
	walk = func(n *paneNode) {
		if n == nil {
			return
		}
		if n.pane != nil {
			out = append(out, n)
			return
		}
		walk(n.first)
		walk(n.second)
	}
	walk(root)
	return out
}

// paneResizeStep 是键盘调一档大小的比例。
const paneResizeStep = float32(0.03)
