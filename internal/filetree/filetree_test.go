package filetree

import (
	"strings"
	"testing"

	"diff-explainer/internal/diff"
	"diff-explainer/internal/gitrepo"
)

func fileDiffWithPath(p string) diff.FileDiff {
	return diff.FileDiff{NewPath: p, Status: "modified"}
}

func files(paths ...string) []gitrepo.File {
	out := make([]gitrepo.File, len(paths))
	for i, p := range paths {
		out[i] = gitrepo.File{
			FileDiff: fileDiffWithPath(p),
			Status:   gitrepo.StatusModified,
		}
	}
	return out
}

func labels(nodes []*Node, depth int) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, strings.Repeat("  ", depth)+n.Label)
		out = append(out, labels(n.Children, depth+1)...)
	}
	return out
}

func TestBuildMergesSingleChildDirChain(t *testing.T) {
	tree := Build(files("internal/server/handler/x.go"))
	got := labels(tree, 0)
	want := []string{"internal/server/handler", "  x.go"}
	if len(got) != len(want) {
		t.Fatalf("树 = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("树 = %q, want %q", got, want)
			break
		}
	}
}

// 目录链合并必须在遇到「分叉」时停下：internal 下有 server 和 gh 两个子目录时，
// internal 不能被合并进任何一条链。
//
// 同层内保持插入顺序（git status 的输出本身按路径排序，所以实际就是字典序）。
func TestBuildStopsMergingAtBranch(t *testing.T) {
	tree := Build(files("internal/gh/y.go", "internal/server/x.go"))
	got := labels(tree, 0)
	want := []string{"internal", "  gh", "    y.go", "  server", "    x.go"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("树 = %q, want %q", got, want)
	}
}

// 目录排在文件前面，否则 "src.txt" 和 "src/" 会交错，层级看起来是乱的。
func TestBuildPutsDirsBeforeFiles(t *testing.T) {
	tree := Build(files("src/main.go", "src.txt", "zzz.go"))
	got := labels(tree, 0)
	want := []string{"src", "  main.go", "src.txt", "zzz.go"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("树 = %q, want %q", got, want)
	}
}

func TestBuildKeysAreUniquePerNode(t *testing.T) {
	tree := Build(files("a/b/c.go", "a/d.go", "e.go"))
	seen := map[string]bool{}
	var walk func([]*Node)
	walk = func(ns []*Node) {
		for _, n := range ns {
			if seen[n.Key] {
				t.Errorf("key 重复: %s", n.Key)
			}
			seen[n.Key] = true
			walk(n.Children)
		}
	}
	walk(tree)
	if len(seen) != 5 { // 目录 a、a/b，文件 a/b/c.go、a/d.go、e.go
		t.Errorf("节点数 = %d, want 5: %v", len(seen), seen)
	}
}

func TestBuildFileIdxPointsAtInput(t *testing.T) {
	fs := files("z.go", "a/x.go")
	tree := Build(fs)
	var found bool
	var walk func([]*Node)
	walk = func(ns []*Node) {
		for _, n := range ns {
			if n.Label == "z.go" {
				found = true
				if n.FileIdx != 0 {
					t.Errorf("z.go FileIdx = %d, want 0", n.FileIdx)
				}
			}
			walk(n.Children)
		}
	}
	walk(tree)
	if !found {
		t.Fatal("没找到 z.go")
	}
}

func TestFlattenRespectsCollapsed(t *testing.T) {
	// internal 下有两个子目录，所以不会被合并，internal 是独立的一行
	tree := Build(files("internal/gh/y.go", "internal/server/x.go", "root.go"))

	all := Flatten(tree, nil, false)
	if len(all) != 6 { // internal、gh、y.go、server、x.go、root.go
		t.Fatalf("全展开 = %d 行, want 6", len(all))
	}

	// 收起 internal 后，它下面的 gh/server 和两个文件都不该再出现
	rows := Flatten(tree, map[string]bool{"d:internal": true}, false)
	if len(rows) != 2 {
		t.Fatalf("收起 internal 后 = %d 行, want 2: %+v", len(rows), rows)
	}
	if rows[0].Node.Label != "internal" || !rows[0].Node.Dir {
		t.Errorf("第一行应该是 internal 目录，实际 %+v", rows[0].Node)
	}
	if rows[1].Node.Label != "root.go" {
		t.Errorf("第二行应该是 root.go，实际 %q", rows[1].Node.Label)
	}
}

// 目录链合并后，折叠状态的 key 是合并后的完整路径 —— 这一点很容易搞错，
// 单独钉一个测试。
func TestCollapseKeyUsesMergedPath(t *testing.T) {
	tree := Build(files("internal/server/x.go"))
	if tree[0].Key != "d:internal/server" {
		t.Fatalf("合并后的 key = %q, want d:internal/server", tree[0].Key)
	}
	// 用合并后的 key 折叠，内容应该消失
	rows := Flatten(tree, map[string]bool{"d:internal/server": true}, false)
	if len(rows) != 1 {
		t.Errorf("按合并 key 折叠后 = %d 行, want 1", len(rows))
	}
	// 用未合并的 key 折叠则不该生效
	rows = Flatten(tree, map[string]bool{"d:internal": true}, false)
	if len(rows) != 2 {
		t.Errorf("用未合并的 key 折叠不该生效，实际 %d 行", len(rows))
	}
}

func TestFlattenFlatIgnoresCollapsed(t *testing.T) {
	tree := Build(files("internal/gh/y.go", "internal/server/x.go"))
	rows := Flatten(tree, map[string]bool{"d:internal": true}, true)
	if len(rows) != 5 {
		t.Errorf("flat 模式应该无视折叠，得到 5 行，实际 %d", len(rows))
	}
}

func TestFlattenDepth(t *testing.T) {
	tree := Build(files("internal/gh/y.go"))
	rows := Flatten(tree, nil, false)
	// internal/gh 合并成一行，y.go 在它下面一层
	wantDepths := []int{0, 1}
	if len(rows) != len(wantDepths) {
		t.Fatalf("行数 = %d, want %d: %+v", len(rows), len(wantDepths), rows)
	}
	for i, w := range wantDepths {
		if rows[i].Depth != w {
			t.Errorf("行 %d 深度 = %d, want %d", i, rows[i].Depth, w)
		}
	}
}

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		s, query string
		want     bool
	}{
		{"internal/server/x.go", "srv", true},    // 子序列
		{"internal/server/x.go", "ISX", true},    // 大小写不敏感
		{"internal/server/x.go", "server", true}, // 连续子串当然也算
		{"internal/server/x.go", "zzz", false},
		{"a.go", "", true},        // 空查询匹配一切
		{"a.go", "a.go.x", false}, // 查询比目标长
	}
	for _, c := range cases {
		if got := FuzzyMatch(c.s, c.query); got != c.want {
			t.Errorf("FuzzyMatch(%q, %q) = %v, want %v", c.s, c.query, got, c.want)
		}
	}
}

func TestFilterKeepsAncestors(t *testing.T) {
	fs := files("internal/gh/y.go", "internal/server/x.go", "README.md")
	tree := Build(fs)

	got := labels(Filter(tree, "y.go"), 0)
	want := []string{"internal", "  gh", "    y.go"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("筛选结果 = %q, want %q", got, want)
	}
}

func TestFilterMatchesFullPath(t *testing.T) {
	fs := files("internal/gh/y.go", "internal/server/x.go")
	tree := Build(fs)

	// "server" 只出现在路径里，不在任何单层文件名里 —— 匹配必须针对完整路径
	got := labels(Filter(tree, "server"), 0)
	if strings.Join(got, "|") != "internal|  server|    x.go" {
		t.Errorf("筛选 'server' = %q", got)
	}
}

func TestFilterEmptyQueryReturnsAll(t *testing.T) {
	tree := Build(files("a.go", "b.go"))
	if len(Filter(tree, "")) != 2 {
		t.Error("空查询应该返回整棵树")
	}
}

func TestFilterNoMatchReturnsEmpty(t *testing.T) {
	tree := Build(files("a.go"))
	if got := Filter(tree, "zzzzz"); len(got) != 0 {
		t.Errorf("无匹配时应返回空，实际 %+v", got)
	}
}
