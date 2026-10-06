package main

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// graphCommits 按 "D:B,E" 这种写法造提交列表：冒号前是哈希，冒号后是父提交
// （逗号分隔）。传入顺序就是列表顺序，也就是拓扑序（子在前、父在后）。
func graphCommits(spec ...string) []gitrepo.Commit {
	commits := make([]gitrepo.Commit, 0, len(spec))
	for _, s := range spec {
		hash, parents, _ := strings.Cut(s, ":")
		c := gitrepo.Commit{Hash: hash}
		if parents != "" {
			c.Parents = strings.Split(parents, ",")
		}
		commits = append(commits, c)
	}
	return commits
}

type rowWant struct {
	lane    int
	in      []int
	out     []int
	through []int
}

// sameInts 把 nil 和空切片当一回事：断言里只关心「有哪些泳道」。
func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// segLanes 把一段段线抽成泳道号 —— 断言里关心的还是「连到哪条泳道」，
// 色号另有测试盯着（TestGraphLineKeepsColor）。
func segLanes(segs []graphSeg) []int {
	out := make([]int, 0, len(segs))
	for _, s := range segs {
		out = append(out, s.Lane)
	}
	return out
}

func checkGraph(t *testing.T, commits []gitrepo.Commit, wantLanes int, want []rowWant) {
	t.Helper()
	rows, maxLanes := buildGraph(commits)
	if len(rows) != len(want) {
		t.Fatalf("算出 %d 行，want %d", len(rows), len(want))
	}
	if maxLanes != wantLanes {
		t.Errorf("泳道数 = %d, want %d", maxLanes, wantLanes)
	}
	for i, w := range want {
		got := rows[i]
		if got.Lane != w.lane {
			t.Errorf("第 %d 行（%s）节点泳道 = %d, want %d", i, commits[i].Hash, got.Lane, w.lane)
		}
		if !sameInts(segLanes(got.In), w.in) {
			t.Errorf("第 %d 行（%s）In = %v, want %v", i, commits[i].Hash, segLanes(got.In), w.in)
		}
		if !sameInts(segLanes(got.Out), w.out) {
			t.Errorf("第 %d 行（%s）Out = %v, want %v", i, commits[i].Hash, segLanes(got.Out), w.out)
		}
		if !sameInts(segLanes(got.Through), w.through) {
			t.Errorf("第 %d 行（%s）Through = %v, want %v", i, commits[i].Hash, segLanes(got.Through), w.through)
		}
	}
}

// 一条直线：全程一条泳道，除了头尾每行上下都有线。
func TestGraphLinear(t *testing.T) {
	checkGraph(t, graphCommits("D:C", "C:B", "B:A", "A:"), 1, []rowWant{
		{lane: 0, in: nil, out: []int{0}, through: nil}, // 分支尖端，上面没有线
		{lane: 0, in: []int{0}, out: []int{0}, through: nil},
		{lane: 0, in: []int{0}, out: []int{0}, through: nil},
		{lane: 0, in: []int{0}, out: nil, through: nil}, // 根提交，下面没有了
	})
}

// 只有一个提交：一个点，没有线。
func TestGraphSingleCommit(t *testing.T) {
	checkGraph(t, graphCommits("A:"), 1, []rowWant{
		{lane: 0, in: nil, out: nil, through: nil},
	})
}

// 空仓库没有行，也不该有泳道。
func TestGraphEmpty(t *testing.T) {
	rows, maxLanes := buildGraph(nil)
	if len(rows) != 0 || maxLanes != 0 {
		t.Errorf("空列表 = %d 行 / %d 泳道, want 0 / 0", len(rows), maxLanes)
	}
}

// 分叉再合并。支线 E←C 从 B 分出去，D 把两条线合起来。
//
//	D ─┬─ E ─ C ─┐
//	   └───────── B ─ A
//
// E 行里 lane 0 让给了 C（节点那条线的下半段），lane 1 的 B 从上面穿过去 ——
// 这就是 Through 的用处。
func TestGraphBranchAndMerge(t *testing.T) {
	checkGraph(t, graphCommits("D:E,B", "E:C", "C:B", "B:A", "A:"), 2, []rowWant{
		{lane: 0, in: nil, out: []int{0, 1}, through: nil},
		{lane: 0, in: []int{0}, out: []int{0}, through: []int{1}},
		// C 的父 B 已经在 lane 1 上等着了，连过去而不是另占一条
		{lane: 0, in: []int{0}, out: []int{1}, through: []int{1}},
		{lane: 1, in: []int{1}, out: []int{1}, through: nil},
		{lane: 1, in: []int{1}, out: nil, through: nil},
	})
}

// 更常见的形状：合并提交的父是「主线在前、支线在后」。
//
//	   ┌─ E ─ C ─┐
//	D ─┴───────── B ─ A
//
// C 的父 B 在 lane 0 上等着，所以 C 那个点连回左边，自己的 lane 1 到此为止。
// lane 0 既是一条贯穿线（B 的线正从上面下来），又出现在 Out 里 —— 两条线在
// 行底端点相接，不是重复。
func TestGraphMergeWithMainlineFirst(t *testing.T) {
	checkGraph(t, graphCommits("D:B,E", "E:C", "C:B", "B:A", "A:"), 2, []rowWant{
		{lane: 0, in: nil, out: []int{0, 1}, through: nil},
		{lane: 1, in: []int{1}, out: []int{1}, through: []int{0}},
		{lane: 1, in: []int{1}, out: []int{0}, through: []int{0}},
		{lane: 0, in: []int{0}, out: []int{0}, through: nil},
		{lane: 0, in: []int{0}, out: nil, through: nil},
	})
}

// 两条支线在这里汇合，节点泳道是最左边那条，其余并进来。
func TestGraphTwoBranchesConverge(t *testing.T) {
	checkGraph(t, graphCommits("M:A,B", "A:C", "B:C", "C:"), 2, []rowWant{
		{lane: 0, in: nil, out: []int{0, 1}, through: nil},
		{lane: 0, in: []int{0}, out: []int{0}, through: []int{1}},
		// B 的父 C 已经在 lane 0 上等着（A 留下的），连过去
		{lane: 1, in: []int{1}, out: []int{0}, through: []int{0}},
		{lane: 0, in: []int{0}, out: nil, through: nil},
	})
}

// octopus：一个提交三个父，分别占三条泳道；三条支线各自连回同一个祖先。
func TestGraphOctopus(t *testing.T) {
	checkGraph(t, graphCommits("M:A,B,C", "A:Z", "B:Z", "C:Z", "Z:"), 3, []rowWant{
		{lane: 0, in: nil, out: []int{0, 1, 2}, through: nil},
		{lane: 0, in: []int{0}, out: []int{0}, through: []int{1, 2}},
		{lane: 1, in: []int{1}, out: []int{0}, through: []int{0, 2}},
		{lane: 2, in: []int{2}, out: []int{0}, through: []int{0}},
		{lane: 0, in: []int{0}, out: nil, through: nil},
	})
}

// 日志被截断：最后一条的父提交根本没读进来。不能给它开泳道 —— 否则会画一条
// 通到底部、尽头什么都没有的线。
func TestGraphTruncatedLogHasNoDanglingLane(t *testing.T) {
	// 合并提交的第二个父不在列表里，最尾部那条的父也不在
	checkGraph(t, graphCommits("M:A,X", "A:B", "B:Z"), 1, []rowWant{
		{lane: 0, in: nil, out: []int{0}, through: nil},
		{lane: 0, in: []int{0}, out: []int{0}, through: nil},
		{lane: 0, in: []int{0}, out: nil, through: nil},
	})
}

// 泳道数超过调色板也要能算出来 —— 图形栏靠它定宽，超出的部分挤在右边缘。
func TestGraphWideHistory(t *testing.T) {
	// 8 个分支尖端各占一条泳道，最后一个提交把它们全合起来
	spec := []string{"R:A,B,C,D,E,F,G,H"}
	for _, h := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		spec = append(spec, h+":Z")
	}
	spec = append(spec, "Z:")

	rows, maxLanes := buildGraph(graphCommits(spec...))
	if maxLanes != 8 {
		t.Fatalf("泳道数 = %d, want 8", maxLanes)
	}
	if len(rows) != 10 {
		t.Fatalf("行数 = %d, want 10", len(rows))
	}
	// 合并提交把 8 条线全铺开
	if !sameInts(segLanes(rows[0].Out), []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("合并行的 Out = %v", segLanes(rows[0].Out))
	}
	// 最后一行的前一行应该把 7 条汇聚到 lane 0
	if !sameInts(segLanes(rows[8].Out), []int{0}) || rows[8].Lane != 7 {
		t.Errorf("H 行 = lane %d, out %v", rows[8].Lane, segLanes(rows[8].Out))
	}
}

// 泳道回收：一条支线结束后，它的泳道要让给后来者，不能一路涨下去。
func TestGraphReusesFreedLanes(t *testing.T) {
	// 两条支线一前一后，第二条应该捡起第一条腾出来的 lane 1
	rows, maxLanes := buildGraph(graphCommits(
		"M1:B,E1", "E1:C1", "C1:B", "M2:B,E2", "E2:C2", "C2:B", "B:",
	))
	if maxLanes != 2 {
		t.Fatalf("泳道数 = %d, want 2（第二条支线该捡起腾出来的lane）", maxLanes)
	}
	for i, r := range rows {
		if r.Lane > 1 {
			t.Errorf("第 %d 行的泳道 = %d，超出了 2 条的宽度", i, r.Lane)
		}
	}
}

// 色号跟着「线」走，不是跟着泳道号。
//
//	D ─┬─ E ─ C ─┐
//	   └───────── B ─ A
//
// D→E→C→B→A 是一条链，可它中途换了泳道：走到 C 那行时，B 已经被 lane 1 上
// 那条线（D 直接连过来的那条）等着了，于是连线并到 lane 1 上。按泳道号定色
// 的话，B 和 A 两行会忽然换成 lane 1 的颜色，往下看就像换了一条分支。
func TestGraphLineKeepsColor(t *testing.T) {
	rows, _ := buildGraph(graphCommits("D:E,B", "E:C", "C:B", "B:A", "A:"))

	want := rows[0].Color
	for i, name := range []string{"D", "E", "C", "B", "A"} {
		if rows[i].Color != want {
			t.Errorf("%s 那一行的节点色号 = %d，主线一开始是 %d —— 线换泳道不该换色",
				name, rows[i].Color, want)
		}
	}

	// 旁边那条支线（D 直接连到 B）得是另一个色号，否则两条线糊在一起
	branch := rows[0].Out[1]
	if branch.Color == want {
		t.Errorf("支线的色号 = %d，和主线一样 —— 两条线得分得开", branch.Color)
	}
	// 它贯穿 C 那一行的时候还得是同一个色号
	if got := rows[2].Through[0]; got.Lane != branch.Lane || got.Color != branch.Color {
		t.Errorf("C 那行的贯穿线 = {lane %d, 色号 %d}，它在 D 那行是 {lane %d, 色号 %d}",
			got.Lane, got.Color, branch.Lane, branch.Color)
	}

	// 色号得落在调色板里，laneColor 按它取模
	for i, r := range rows {
		if r.Color < 0 || r.Color >= laneCount {
			t.Errorf("第 %d 行的节点色号 = %d，超出 0..%d", i, r.Color, laneCount-1)
		}
		for _, s := range append(append(append([]graphSeg{}, r.In...), r.Out...), r.Through...) {
			if s.Color < 0 || s.Color >= laneCount {
				t.Errorf("第 %d 行 lane %d 的线色号 = %d，超出 0..%d",
					i, s.Lane, s.Color, laneCount-1)
			}
		}
	}
}

// pickColor 挑的是「当前没被占用的最小色号」，不是「已经用过几个」。
//
// 区别在色号用得不连续的时候：0 和 2 在用、1 空着，下一个新线该拿 1。
// 按「最大号 +1」算是 3，一路涨下去转一圈回来就会撞上还活着的线 ——
// 屏幕上两条同色的线，谁也认不出谁。空泳道的色号是 -1，不参与。
func TestPickColor(t *testing.T) {
	cases := []struct {
		used []int
		want int
		why  string
	}{
		{nil, 0, "一条线都没有，从 0 开始"},
		{[]int{0}, 1, "接着往下拿"},
		{[]int{0, 2}, 1, "中间空着的 1 先用掉，别一路涨上去"},
		{[]int{-1, 0, -1}, 1, "空泳道的 -1 不算占用"},
		{[]int{0, 1, 2, 3, 4, 5, 7}, 6, "只剩一个空位也挑得出来"},
		{[]int{0, 1, 2, 3, 4, 5, 6, 7}, 0, "全占着就随便给一个，不能崩"},
	}
	for _, c := range cases {
		if got := pickColor(c.used); got != c.want {
			t.Errorf("pickColor(%v) = %d, want %d（%s）", c.used, got, c.want, c.why)
		}
	}
}
