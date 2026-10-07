// Package filetree 把文件变更列表拼成目录树，供侧边栏渲染。
//
// 算法移植自 Web 版（web/src/fileTree.ts），最关键的一条是：只含单个子目录的
// 目录链会合并成一行（internal/server/handler 显示成一行而不是三层），
// 否则深路径会把侧边栏横向撑爆。
package filetree

import (
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// Node 是文件树里的一个节点。
type Node struct {
	Key      string // "d:<path>" 或 "f:<path>"，用作列表 key 和折叠状态
	Label    string // 显示名；合并的目录链形如 "internal/server"
	Path     string // 完整路径
	Dir      bool
	FileIdx  int // 在输入 files 切片里的下标；目录恒为 -1
	Children []*Node
}

// Row 是树展开成一维列表后的一行，带缩进深度。
type Row struct {
	Node  *Node
	Depth int
}

// dirNode 是构建过程中的中间目录节点。
type dirNode struct {
	dirs  map[string]*dirNode
	files map[string]int // 文件名 -> 在输入切片里的下标
	order []string       // "d:<name>" / "f:<name>"，保留插入顺序
}

func newDir() *dirNode {
	return &dirNode{dirs: map[string]*dirNode{}, files: map[string]int{}}
}

// Build 把文件列表拼成树。
//
// files 的顺序决定同层内的先后。git status 的输出本身按路径排序，
// 所以传进来就是有序的。
func Build(files []gitrepo.File) []*Node {
	return BuildAt(files, 0, "")
}

// BuildAt 和 Build 一样建树，但 FileIdx 从 base 开始数、节点 Key 带 keyPrefix。
//
// 文件列表分成「已暂存 / 未暂存」两段、各建一棵树时用：base 把 FileIdx 对回
// 扁平列表（两段共用同一份切片的拼接），keyPrefix 让两段的折叠状态互不干扰
// —— 同一个路径可以同时在两段里（暂存之后又改了），Key 不带前缀就会串。
func BuildAt(files []gitrepo.File, base int, keyPrefix string) []*Node {
	root := newDir()
	for i, f := range files {
		path := f.NewPath
		if path == "" {
			path = f.OldPath
		}
		parts := strings.Split(path, "/")

		d := root
		for _, name := range parts[:len(parts)-1] {
			next, ok := d.dirs[name]
			if !ok {
				next = newDir()
				d.dirs[name] = next
				d.order = append(d.order, "d:"+name)
			}
			d = next
		}
		name := parts[len(parts)-1]
		if _, dup := d.files[name]; !dup {
			d.order = append(d.order, "f:"+name)
		}
		d.files[name] = base + i
	}
	return walk(root, "", keyPrefix)
}

// walk 把中间结构转成对外的 Node 树。
//
// 同层内目录排在文件前面 —— 和 finder/Fork 这些文件浏览器的习惯一致，
// 也让「目录名和用户名重名」时不会交错得看不清层级。
func walk(d *dirNode, prefix, keyPrefix string) []*Node {
	var out []*Node

	emit := func(want byte) {
		for _, o := range d.order {
			if o[0] != want {
				continue
			}
			name := o[2:]
			if want == 'f' {
				idx := d.files[name]
				out = append(out, &Node{
					Key:     "f:" + keyPrefix + prefix + name,
					Label:   name,
					Path:    prefix + name,
					FileIdx: idx,
				})
				continue
			}

			sub := d.dirs[name]
			label, path := name, prefix+name
			// 只含单个子目录的目录链合并成一行
			for len(sub.order) == 1 && sub.order[0][0] == 'd' {
				child := sub.order[0][2:]
				label += "/" + child
				path += "/" + child
				sub = sub.dirs[child]
			}
			out = append(out, &Node{
				Key:      "d:" + keyPrefix + path,
				Label:    label,
				Path:     path,
				Dir:      true,
				FileIdx:  -1,
				Children: walk(sub, path+"/", keyPrefix),
			})
		}
	}
	emit('d')
	emit('f')
	return out
}

// Flatten 把树展开成一维列表，跳过折叠目录里的内容。
//
// flat 为 true 时忽略 collapsed（用于「全部展开」和筛选态 —— 筛选时
// 必须无视折叠状态，否则搜到的文件可能藏在收起的目录里看不见）。
func Flatten(nodes []*Node, collapsed map[string]bool, flat bool) []Row {
	var out []Row
	var rec func(ns []*Node, depth int)
	rec = func(ns []*Node, depth int) {
		for _, n := range ns {
			out = append(out, Row{Node: n, Depth: depth})
			if n.Dir && (flat || !collapsed[n.Key]) {
				rec(n.Children, depth+1)
			}
		}
	}
	rec(nodes, 0)
	return out
}

// Filter 返回只包含匹配文件的树（保留匹配文件的所有祖先目录）。
//
// query 为空时返回原树。匹配的是完整路径而不是单层文件名 —— 用户
// 输入 "srv" 时希望找到 internal/server/x.go，而不是只筛当前层的名字。
func Filter(nodes []*Node, query string) []*Node {
	if query == "" {
		return nodes
	}
	var rec func(ns []*Node) []*Node
	rec = func(ns []*Node) []*Node {
		var out []*Node
		for _, n := range ns {
			if !n.Dir {
				if FuzzyMatch(n.Path, query) {
					out = append(out, n)
				}
				continue
			}
			kids := rec(n.Children)
			if len(kids) > 0 {
				clone := *n
				clone.Children = kids
				out = append(out, &clone)
			}
		}
		return out
	}
	return rec(nodes)
}

// FuzzyMatch 做大小写不敏感的子序列匹配，和 godiff 的 Filter files 行为一致：
// 输入 "inmh" 能匹配 "internal/model/handler.go"。
func FuzzyMatch(s, query string) bool {
	if query == "" {
		return true
	}
	hay := strings.ToLower(s)
	q := strings.ToLower(query)
	i := 0
	for j := 0; j < len(hay) && i < len(q); j++ {
		if hay[j] == q[i] {
			i++
		}
	}
	return i >= len(q)
}
