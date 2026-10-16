package main

import (
	"slices"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 侧栏 refs 树的键。和文件树的 "d:/f:" 分得开 —— 两棵树各有各的 key
// 空间，本来不会撞，但调试和测试输出里一眼能分辨出这是哪棵树。
const (
	refGroup = "g:" // 分组行：g:local / g:remote:origin / g:tag / g:stash
	refLeaf  = "r:" // 分支、标签、储藏
	// refAll 是「所有分支」那一行。它不是一条引用（没有 Full 可以喂给
	// Log），所以单独一个键，不走 refByFull 那条查表的路。
	refAll = "a:all"
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

	// 「所有分支」是本地分组的第一行：点它看的是所有引用合起来的历史，
	// 不是某一条线。它不是一条 Ref（没有 Full 可以喂给 Log），所以树里
	// 用 refAll 这个键占位，refRow 见到这个键走自己的渲染和点击。
	//
	// 筛选时也留着：用户搜一个分支名时，「看看全部」往往正是他想做的
	// 下一件事。空仓库（一条引用都没有）不画 —— 那会儿历史也是空的。
	allNode := (*filetree.Node)(nil)
	if len(kept) > 0 {
		allNode = &filetree.Node{
			Key: refAll, Label: T("所有分支", "All Branches"), Path: "all",
			FileIdx: -1,
		}
	}

	// 组的顺序是固定的：本地 → 远程 → 标签 → 储藏。空组不画。
	//
	// 本地分组多画一种情况：一条本地分支都没有时也画 —— 那至少还有
	// 「所有分支」那一行点得动（只看远程分支的仓库、刚克隆完还没建分支）。
	// 别的组都有东西时反而不画，不然「本地分支」四个字下面空空如也。
	others := len(remote) + len(tags) + len(stashes)
	var roots []*filetree.Node
	if len(local) > 0 || (allNode != nil && others == 0) {
		kids := refLeaves(local)
		if allNode != nil {
			kids = append([]*filetree.Node{allNode}, kids...)
		}
		roots = append(roots, refGroupNode("g:local", T("本地分支", "Local Branches"), kids))
	}
	if len(remote) > 0 {
		roots = append(roots, refGroupNode("g:remote", T("远程分支", "Remote Branches"), remoteGroups(remote)))
	}
	if len(tags) > 0 {
		roots = append(roots, refGroupNode("g:tag", T("标签", "Tags"), refLeaves(tags)))
	}
	if len(stashes) > 0 {
		roots = append(roots, refGroupNode("g:stash", T("储藏", "Stashes"), refLeaves(stashes)))
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

// trackLabel 是侧栏本地分支行尾巴上那截「和上游差几个提交」。
//
// 只本地分支有上游，别的种类一律空串。同步（或没有上游）也返回空 ——
// 一条分支后面挂个「↑0 ↓0」只会把一列分支搞得更难读。
//
// 上游被远程删掉（%(upstream:track) 的 [gone]）返回「上游已删除」：那种
// 分支推上去会新建一条远程分支，用户至少该知道自己跟踪的那条已经没了。
func trackLabel(ref gitrepo.Ref) string {
	if ref.Kind != gitrepo.RefLocal || ref.Upstream == "" {
		return ""
	}
	if ref.UpstreamGone {
		return T("上游已删除", "upstream gone")
	}
	switch {
	case ref.Ahead > 0 && ref.Behind > 0:
		return Tf("↑%d ↓%d", "↑%d ↓%d", ref.Ahead, ref.Behind)
	case ref.Ahead > 0:
		return Tf("↑%d", "↑%d", ref.Ahead)
	case ref.Behind > 0:
		return Tf("↓%d", "↓%d", ref.Behind)
	}
	return ""
}
