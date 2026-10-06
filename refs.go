package main

import (
	"slices"

	"diff-explainer/internal/filetree"
	"diff-explainer/internal/gitrepo"
)

// 侧栏 refs 树的键。和文件树的 "d:/f:" 分得开 —— 两棵树各有各的 key
// 空间，本来不会撞，但调试和测试输出里一眼能分辨出这是哪棵树。
const (
	refGroup = "g:" // 分组行：g:local / g:remote:origin / g:tag / g:stash
	refLeaf  = "r:" // 分支、标签、储藏
)

// buildRefTree 把读到的引用排成侧栏那棵树，顺便按筛选词筛一道。
//
// 先筛叶子再造树：整个分组都被筛掉时它自然就不出现了，比造完树再回头剪枝
// 少一段代码，也不会漏掉「分组还在、里面空了」这种半拉状态。
//
// 复用 filetree.Node 而不是另起一个节点类型：它有 Key/Label/Dir/Children，
// 正好是摊平（filetree.Flatten）和画行（treeRow）要的东西，Path/FileIdx
// 这两个字段空着就是了。为了几个用不上的字段再写一套节点，不值当。
func buildRefTree(refs []gitrepo.Ref, query string) []*filetree.Node {
	kept := make([]gitrepo.Ref, 0, len(refs))
	for _, ref := range refs {
		if query == "" ||
			filetree.FuzzyMatch(ref.Name, query) ||
			filetree.FuzzyMatch(ref.Label(), query) {
			kept = append(kept, ref)
		}
	}

	local, remote, tags, stashes := splitRefs(kept)

	// 组的顺序是固定的：本地 → 远程 → 标签 → 储藏。空组不画。
	var roots []*filetree.Node
	if len(local) > 0 {
		roots = append(roots, refGroupNode("g:local", "本地分支", refLeaves(local)))
	}
	if len(remote) > 0 {
		roots = append(roots, refGroupNode("g:remote", "远程分支", remoteGroups(remote)))
	}
	if len(tags) > 0 {
		roots = append(roots, refGroupNode("g:tag", "标签", refLeaves(tags)))
	}
	if len(stashes) > 0 {
		roots = append(roots, refGroupNode("g:stash", "储藏", refLeaves(stashes)))
	}
	return roots
}

func splitRefs(refs []gitrepo.Ref) (local, remote, tags, stashes []gitrepo.Ref) {
	for _, ref := range refs {
		switch ref.Kind {
		case gitrepo.RefLocal:
			local = append(local, ref)
		case gitrepo.RefRemote:
			remote = append(remote, ref)
		case gitrepo.RefTag:
			tags = append(tags, ref)
		case gitrepo.RefStash:
			stashes = append(stashes, ref)
		}
	}
	return local, remote, tags, stashes
}

// remoteGroups 把远程分支按 remote 再分一层。
//
// 直接平铺的话，origin 和 upstream 各十几条分支混在一起没法看 —— 而且两条
// remote 上同名的分支（都在 main 上）会长得一模一样。
//
// remote 之间按名字排，不按最近提交时间：远程那一层的顺序应该是稳定的，
// 否则 fetch 一次（或者 git 自己更新一下 remote-tracking ref）侧栏里
// origin 和 upstream 就换个个儿，用户刚点过的那条找不着了。
func remoteGroups(refs []gitrepo.Ref) []*filetree.Node {
	order := make([]string, 0, 4)
	byRemote := map[string][]gitrepo.Ref{}
	for _, ref := range refs {
		if _, ok := byRemote[ref.Remote]; !ok {
			order = append(order, ref.Remote)
		}
		byRemote[ref.Remote] = append(byRemote[ref.Remote], ref)
	}
	slices.Sort(order)

	out := make([]*filetree.Node, 0, len(order))
	for _, name := range order {
		out = append(out, refGroupNode("g:remote:"+name, name, refLeaves(byRemote[name])))
	}
	return out
}

func refLeaves(refs []gitrepo.Ref) []*filetree.Node {
	out := make([]*filetree.Node, 0, len(refs))
	for _, ref := range refs {
		label := ref.Label()
		if ref.Kind == gitrepo.RefStash {
			// 储藏的名字（stash@{0}）人人一样，信息全在信息里
			label = ref.Subject
		}
		out = append(out, &filetree.Node{
			Key:     refLeaf + ref.Full,
			Label:   label,
			Dir:     false,
			FileIdx: -1,
		})
	}
	return out
}

func refGroupNode(key, label string, children []*filetree.Node) *filetree.Node {
	return &filetree.Node{Key: key, Label: label, Path: label, Dir: true, FileIdx: -1, Children: children}
}
