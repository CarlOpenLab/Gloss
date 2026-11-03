package main

import (
	"math"
	"testing"
)

// 向右分屏：原来那一格留在左边，新格落在右边，两格各占一半。
func TestSplitPaneRightKeepsFirstPaneOnTheLeft(t *testing.T) {
	first := &termPane{}
	root := &paneNode{pane: first}

	second := &termPane{}
	root, leaf := splitPaneAt(root, root, paneRight, second)

	if leaf == nil || leaf.pane != second {
		t.Fatalf("新格该是返回的那一格")
	}
	if root.pane != nil {
		t.Fatalf("根节点该变成一个分叉")
	}
	if root.stack {
		t.Fatalf("向右分屏该是左右并排，不是上下叠")
	}
	if root.first == nil || root.first.pane != first {
		t.Fatalf("左边的格子该还是原来那一格")
	}
	if root.second == nil || root.second.pane != second {
		t.Fatalf("右边的格子该是新格")
	}
	if root.ratio != 0.5 {
		t.Fatalf("新分屏该各占一半，实际 %v", root.ratio)
	}
}

// 向左 / 向上分屏：新格落在前面，原来那一格往后让。
func TestSplitPaneLeftAndUpPlaceNewPaneFirst(t *testing.T) {
	t.Run("left", func(t *testing.T) {
		first := &termPane{}
		root := &paneNode{pane: first}
		second := &termPane{}
		root, leaf := splitPaneAt(root, root, paneLeft, second)
		if root.stack {
			t.Fatal("向左分屏该是左右并排")
		}
		if root.first != leaf || root.first.pane != second {
			t.Fatal("新格该排在左边")
		}
		if root.second.pane != first {
			t.Fatal("原来的格子该排在右边")
		}
	})

	t.Run("up", func(t *testing.T) {
		first := &termPane{}
		root := &paneNode{pane: first}
		second := &termPane{}
		root, leaf := splitPaneAt(root, root, paneUp, second)
		if !root.stack {
			t.Fatal("向上分屏该是上下叠")
		}
		if root.first != leaf || root.first.pane != second {
			t.Fatal("新格该排在上面")
		}
		if root.second.pane != first {
			t.Fatal("原来的格子该排在下面")
		}
	})
}

// 拆一个已经在分叉里的格子：只替换它自己，兄弟和更上层都不动。
func TestSplitNestedPaneReplacesOnlyThatLeaf(t *testing.T) {
	left := &termPane{}
	right := &termPane{}
	root := &paneNode{first: &paneNode{pane: left}, second: &paneNode{pane: right}, ratio: 0.5}
	root.first.parent, root.second.parent = root, root

	third := &termPane{}
	root, leaf := splitPaneAt(root, root.second, paneDown, third)

	if root.first.pane != left {
		t.Fatal("左半边不该被带动")
	}
	if root.second.pane != nil || !root.second.stack {
		t.Fatal("右半边该变成上下分屏")
	}
	if root.second.first.pane != right || root.second.second.pane != third {
		t.Fatal("上下分屏该是原来的在上、新格在下")
	}
	if leaf.pane != third || leaf.parent != root.second {
		t.Fatal("新格该挂在右半边的分屏下")
	}
}

// 关掉一格：分叉塌成它的兄弟，焦点落到兄弟那一侧。
func TestClosePaneCollapsesItsSplit(t *testing.T) {
	left := &termPane{}
	right := &termPane{}
	leftNode := &paneNode{pane: left}
	rightNode := &paneNode{pane: right}
	root := &paneNode{first: leftNode, second: rightNode, ratio: 0.5}
	leftNode.parent, rightNode.parent = root, root

	newRoot, focus := closePaneAt(root, leftNode)

	if newRoot != rightNode {
		t.Fatal("关掉左边之后根节点该直接是右边的格子")
	}
	if newRoot.parent != nil {
		t.Fatal("塌下来的根不该还挂着旧分叉")
	}
	if focus != rightNode {
		t.Fatal("焦点该落到兄弟格")
	}
}

// 关嵌在深处的一格：只塌它所在的那道分叉，外面不动。
func TestCloseNestedPaneCollapsesOnlyItsSplit(t *testing.T) {
	left := &termPane{}
	top := &termPane{}
	bottom := &termPane{}
	leftNode := &paneNode{pane: left}
	topNode := &paneNode{pane: top}
	bottomNode := &paneNode{pane: bottom}
	inner := &paneNode{stack: true, ratio: 0.5, first: topNode, second: bottomNode}
	root := &paneNode{first: leftNode, second: inner, ratio: 0.5}
	leftNode.parent, inner.parent = root, root
	topNode.parent, bottomNode.parent = inner, inner

	newRoot, focus := closePaneAt(root, bottomNode)

	if newRoot != root || root.first != leftNode || root.second != topNode {
		t.Fatal("右下的分屏该塌成上面那一格，左边的分屏不动")
	}
	if topNode.parent != root {
		t.Fatal("塌下来的格子该接到原来的父节点上")
	}
	if focus != topNode {
		t.Fatal("焦点该落到兄弟格")
	}
}

// 关掉最后一格：树空了。
func TestCloseLastPaneLeavesNoTree(t *testing.T) {
	only := &paneNode{pane: &termPane{}}
	root, focus := closePaneAt(only, only)
	if root != nil || focus != nil {
		t.Fatalf("关掉最后一格该得到空树，实际 root=%v focus=%v", root, focus)
	}
}

// 焦点按方向找相邻的那一格。四宫格：左上 A、左下 C、右上 B、右下 D。
func TestFocusPaneMovesByDirection(t *testing.T) {
	a, b, c, d := &termPane{}, &termPane{}, &termPane{}, &termPane{}
	aN, bN := &paneNode{pane: a}, &paneNode{pane: b}
	cN, dN := &paneNode{pane: c}, &paneNode{pane: d}
	left := &paneNode{stack: true, ratio: 0.5, first: aN, second: cN}
	right := &paneNode{stack: true, ratio: 0.5, first: bN, second: dN}
	root := &paneNode{first: left, second: right, ratio: 0.5}
	left.parent, right.parent = root, root
	aN.parent, cN.parent = left, left
	bN.parent, dN.parent = right, right

	cases := []struct {
		from *paneNode
		dir  paneDir
		want *paneNode
	}{
		{aN, paneRight, bN},
		{aN, paneDown, cN},
		{aN, paneLeft, nil},
		{aN, paneUp, nil},
		{cN, paneUp, aN},
		{cN, paneRight, dN},
		{bN, paneLeft, aN},
		{bN, paneDown, dN},
		{dN, paneLeft, cN},
		{dN, paneUp, bN},
	}
	for _, c := range cases {
		if got := focusPaneInDir(root, c.from, c.dir); got != c.want {
			t.Errorf("从 %p 往 %d 走该到 %p，实际 %p", c.from, c.dir, c.want, got)
		}
	}
}

// 比例不均时也按实际位置找相邻格，不是按树的顺序。
func TestFocusPaneUsesLayoutNotTreeOrder(t *testing.T) {
	a, b, c := &termPane{}, &termPane{}, &termPane{}
	aN, bN := &paneNode{pane: a}, &paneNode{pane: b}
	// 左半边：A 在上，B 在下；右半边整列是 C。
	left := &paneNode{stack: true, ratio: 0.3, first: aN, second: bN}
	right := &paneNode{pane: c}
	root := &paneNode{first: left, second: right, ratio: 0.6}
	left.parent, right.parent = root, root
	aN.parent, bN.parent = left, left

	if got := focusPaneInDir(root, aN, paneRight); got != right {
		t.Fatalf("A 往右该到 C，实际 %p", got)
	}
	if got := focusPaneInDir(root, bN, paneRight); got != right {
		t.Fatalf("B 往右该到 C，实际 %p", got)
	}
	if got := focusPaneInDir(root, right, paneDown); got != nil {
		t.Fatalf("C 已经占满右边整列，往下不该有格子，实际 %p", got)
	}
}

// 调大小：找离当前格最近、方向也对得上的那道分叉，把分隔线往按的方向挪。
func TestResizePaneMovesNearestMatchingSplit(t *testing.T) {
	a, b := &termPane{}, &termPane{}
	aN, bN := &paneNode{pane: a}, &paneNode{pane: b}
	root := &paneNode{ratio: 0.5, first: aN, second: bN}
	aN.parent, bN.parent = root, root

	// 从左格往右拉：分隔线右移，左格变大。
	resizePaneAt(root, aN, paneRight, 0.1)
	if math.Abs(float64(root.ratio-0.6)) > 0.0001 {
		t.Fatalf("往右拉该变成 0.6，实际 %v", root.ratio)
	}
	// 从右格往右拉：分隔线还是右移，右格变小。
	resizePaneAt(root, bN, paneRight, 0.1)
	if math.Abs(float64(root.ratio-0.7)) > 0.0001 {
		t.Fatalf("从右格往右拉也该把分隔线右移，实际 %v", root.ratio)
	}
	// 往上 / 下没有竖分叉，不动。
	resizePaneAt(root, aN, paneUp, 0.1)
	if math.Abs(float64(root.ratio-0.7)) > 0.0001 {
		t.Fatalf("没有竖分叉时上下调大小不该动横比例，实际 %v", root.ratio)
	}
}

// 比例夹在两侧都留得下一格的范围里。
func TestResizePaneClampsRatio(t *testing.T) {
	a, b := &termPane{}, &termPane{}
	aN, bN := &paneNode{pane: a}, &paneNode{pane: b}
	root := &paneNode{ratio: 0.5, first: aN, second: bN}
	aN.parent, bN.parent = root, root

	resizePaneAt(root, aN, paneRight, 10)
	if root.ratio != paneRatioMax {
		t.Fatalf("拉到底该停在 %v，实际 %v", paneRatioMax, root.ratio)
	}
	resizePaneAt(root, aN, paneLeft, 10)
	if root.ratio != paneRatioMin {
		t.Fatalf("缩到底该停在 %v，实际 %v", paneRatioMin, root.ratio)
	}
}

// 均分：树里每一道分叉都回到一半。
func TestEqualizePanesResetsEverySplit(t *testing.T) {
	a, b, c := &termPane{}, &termPane{}, &termPane{}
	aN, bN, cN := &paneNode{pane: a}, &paneNode{pane: b}, &paneNode{pane: c}
	inner := &paneNode{stack: true, ratio: 0.2, first: bN, second: cN}
	root := &paneNode{ratio: 0.8, first: aN, second: inner}
	aN.parent, inner.parent = root, root
	bN.parent, cN.parent = inner, inner

	equalizePanes(root)

	if root.ratio != 0.5 || inner.ratio != 0.5 {
		t.Fatalf("均分后每道分叉都该是一半，实际 %v / %v", root.ratio, inner.ratio)
	}
}
