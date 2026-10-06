package main

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/gitrepo"
)

// graphRow 是一行提交的泳道布局。
//
// 每行自己算好自己那段，渲染时不看邻居 —— ui.List 是虚拟化的，滚到哪行才建
// 哪行，任何「记住上一行画到哪」的做法一滚动就错位。
type graphRow struct {
	Lane    int   // 节点在第几条泳道
	In      []int // 从上面连进节点的泳道；第一个是节点自己那条，其余是汇进来的分支
	Out     []int // 从节点连到下面的泳道；第一个对应第一个父提交
	Through []int // 贯穿本行的泳道：和节点无关，但要从上画到下
}

// buildGraph 把拓扑序的提交列表算成逐行的泳道布局，顺便给出这套图一共占几条
// 泳道 —— 图形栏按它定宽，滚动时文字才不会左右跳。
//
// 一次自上而下遍历（子在前、父在后，--topo-order 保证）。泳道里记的是「这条
// 线在等哪个提交」，于是「一个提交的哈希出现在几条泳道里」就等于「有几条线在
// 这里汇合」—— 合并提交不用单独判断，它是这个规则的副产品。
func buildGraph(commits []gitrepo.Commit) (rows []graphRow, maxLanes int) {
	if len(commits) == 0 {
		return nil, 0
	}

	// 只有这一页里读得到的父提交才配泳道。日志是截断的（logLimit），尾部
	// 那几条的父往往没读进来 —— 给它们占了泳道，就会画出一条通到底部、
	// 尽头什么都没有的死线，还平白把图形栏撑宽。
	visible := make(map[string]bool, len(commits))
	for _, c := range commits {
		visible[c.Hash] = true
	}

	var lanes []string // 泳道 → 正在等它的提交哈希，"" 表示空着
	rows = make([]graphRow, 0, len(commits))

	for _, cm := range commits {
		// 有几条泳道在等这个提交？多于一条就是汇合点。
		var at []int
		for i, h := range lanes {
			if h == cm.Hash {
				at = append(at, i)
			}
		}

		lane := 0
		if len(at) > 0 {
			lane = at[0]
		} else {
			// 没有线在等它：分支尖端，占一条空泳道
			lane = freeLane(&lanes)
		}

		// 本行开始时的样子。下面要先把这些泳道腾空，但贯穿线正是靠
		// 「腾空前占着、腾空后还占着」认出来的，得先留个底。
		before := append([]string(nil), lanes...)

		// 汇进来的分支并到节点上，它们到此为止
		for _, i := range at {
			lanes[i] = ""
		}

		var out []int
		for k, p := range cm.Parents {
			if !visible[p] {
				continue
			}
			// 已经有泳道在等这个父提交就直接连过去。这一步对第一个父
			// 也要做：无脑占下节点自己那条，会和旁边那条等同一个提交的
			// 泳道并排走下去，变成两条平行的线连到同一个点。
			j := waitingLane(lanes, p)
			if j < 0 {
				if k == 0 {
					j = lane // 第一个父接着走节点这条线，直线下去
				} else {
					j = freeLane(&lanes)
				}
			}
			lanes[j] = p
			out = append(out, j)
		}

		// 前后都占着、又不是节点自己那条、也不是汇进来的 —— 那是条和本行
		// 无关、但要贯穿过去的线。
		//
		// Out 里的泳道不能排除：父提交可能本来就在别的泳道上等着，那条线
		// 正从上往下穿，节点连过去刚好在行底接上，两条线只在端点相交，不是
		// 重复。In 里的则必须排除 —— 它们在这一行并进了节点，画满整行会多
		// 出一截。
		var through []int
		for i := 0; i < len(before) && i < len(lanes); i++ {
			if before[i] == "" || lanes[i] == "" || i == lane || slices.Contains(at, i) {
				continue
			}
			through = append(through, i)
		}

		rows = append(rows, graphRow{Lane: lane, In: at, Out: out, Through: through})
		if len(lanes) > maxLanes {
			maxLanes = len(lanes)
		}
	}
	return rows, maxLanes
}

// freeLane 找一条空泳道，全占着就往后新开一条。
func freeLane(lanes *[]string) int {
	for i, h := range *lanes {
		if h == "" {
			return i
		}
	}
	*lanes = append(*lanes, "")
	return len(*lanes) - 1
}

// waitingLane 找正在等 h 的泳道，没有就返回 -1。
func waitingLane(lanes []string, h string) int {
	for i, v := range lanes {
		if v == h {
			return i
		}
	}
	return -1
}

// 提交图的尺寸。行高是定死的 commitH = 56，泳道宽 14 和它相配：节点直径 8，
// 左右各留 3 像素，相邻两条线不会挨在一起。
const (
	graphLaneW  = 14
	graphPad    = 8 // 图形栏左边的留白
	graphNodeR  = 4 // 节点半径
	graphLineW  = 1.5
	graphMaxCol = 5 // 最多铺几条泳道，再多就都挤在最右边那条上
)

// graphWidth 是图形栏的宽度。
//
// 按整张图的最大泳道数定宽，不按当前行 —— 后者会让每一行的文字带着图一起
// 左右横跳。空列表也留一条泳道的宽度，免得列表在加载和就绪之间闪一下。
func graphWidth(maxLanes int) float32 {
	n := min(max(maxLanes, 1), graphMaxCol)
	return graphPad + float32(n)*graphLaneW
}

// laneX 是第 i 条泳道中心线的 x（相对图形栏左边缘）。
//
// 超过 graphMaxCol 的泳道都并到最右边那条上：线会重叠，但总比把提交信息
// 挤没了强。并上去的线在同一个 x 上，画出来是重叠的竖线，看着像一条 ——
// 这种仓库（一个提交十来个父）本来也是拿来看个大概。
func laneX(i, maxLanes int) float32 {
	i = min(max(i, 0), min(maxLanes, graphMaxCol)-1)
	return graphPad + float32(i)*graphLaneW + graphLaneW/2
}

// drawGraphRow 画一行提交的图形：贯穿本行的线、连到节点的线、节点。
//
// r 是图形栏本行的矩形（窗口坐标，DIP）。只看自己这一行的数据，不参考上下
// 邻居 —— ui.List 是虚拟化的，滚到哪行才建哪行，任何「上一行画到哪」的状态
// 都会在滚动时错位。
func drawGraphRow(p *ui.Painter, r ui.Rect, pal palette, row graphRow, maxLanes int) {
	// laneX 算的是「相对图形栏左边缘」的 x，画到窗口坐标上还得加上 r.X。
	// 漏掉这一下不会报错 —— 线会画到图形栏左边去（侧栏底下），而且被列表的
	// 裁剪区裁掉，屏幕上什么都看不见。
	at := func(l int) float32 { return r.X + laneX(l, maxLanes) }
	mid := r.Y + r.H/2
	nodeX := at(row.Lane)

	// 贯穿本行的线，上下各多画一像素盖住行与行之间的接缝：描边是圆头的，
	// 正好停在行边界上会留下一道发丝粗的缝。
	//
	// 多画这一像素不影响性能 —— 引擎按「路径相对自身包围盒的形状」缓存
	// 蒙版，行高和 x 都是整数，每一行的这条线形状完全一样，栅格化只做一次。
	for _, l := range row.Through {
		drawSeg(p, at(l), r.Y-1, r.Y+r.H+1, pal.laneColor(l))
	}

	// 上半行：从上面连进节点
	for _, l := range row.In {
		if l == row.Lane {
			drawSeg(p, at(l), r.Y-1, mid, pal.laneColor(l))
		} else {
			drawVein(p, at(l), r.Y, nodeX, mid, pal.laneColor(l))
		}
	}

	// 下半行：从节点连到下面
	for _, l := range row.Out {
		if l == row.Lane {
			drawSeg(p, at(l), mid, r.Y+r.H+1, pal.laneColor(l))
		} else {
			drawVein(p, nodeX, mid, at(l), r.Y+r.H, pal.laneColor(l))
		}
	}

	// 节点画在最后，盖住底下的线头
	var dot ui.Path
	dot.Circle(nodeX, mid, graphNodeR)
	p.FillPath(&dot, pal.laneColor(row.Lane))
}

// drawSeg 是竖直的直线段。
func drawSeg(p *ui.Painter, x, y0, y1 float32, c ui.Color) {
	var path ui.Path
	path.MoveTo(x, y0).LineTo(x, y1)
	p.StrokePath(&path, graphLineW, c)
}

// drawVein 是斜着连到节点的线。走 S 形：两个控制点都落在首尾之间的中间
// 高度上，线从水平方向离开、也从水平方向到达 —— 直接拉一根直线的话，它和
// 竖直的泳道接起来是个折角，看着像断了。
func drawVein(p *ui.Painter, x0, y0, x1, y1 float32, c ui.Color) {
	var path ui.Path
	my := (y0 + y1) / 2
	path.MoveTo(x0, y0).CubeTo(x0, my, x1, my, x1, y1)
	p.StrokePath(&path, graphLineW, c)
}
