package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 检出：侧栏里的分支和标签，双击或右键菜单走同一条路。
//
// 打标签的对话框在 tagdialog.go —— 那是一张表单，和这里的一次性动作不是
// 一回事。

// checkoutPlan 是「点这条引用之后该发生什么」，在动手之前一次算清楚。
//
// 单独一个结构是因为它有三个消费者：右键菜单的文案、执行时的那条 git
// 命令、成功后的那句提示。三处各算一遍的话，改了「本地已有同名分支就降级」
// 这条规则，很容易只改到其中一处 —— 菜单上写着「检出并新建本地分支」而实际
// 只是切过去，用户就会以为新建过。
type checkoutPlan struct {
	kind  gitrepo.RefKind
	name  string // 本地分支名 / 标签名
	ref   string // 远程分支的完整引用名（CheckoutRemote 用）
	menu  string // 右键菜单上那一行
	label string // 进行中：Spinner 旁边那行字
	note  string // 成功后的一句（Toast）
	noop  bool   // 已经在那儿了：不发命令，只报一句
}

// planCheckout 把一条引用翻译成一次检出。
//
// 远程分支先算本地名（origin/feature/x → feature/x）。本地**已经有**这个
// 名字时降级成检出本地那条：用户要的是「把远程这条切下来」，已经拉过的情况
// 下就是切过去 —— 让他去撞一句 "a branch named 'x' already exists" 再自己
// 想明白，是把我们该做的判断推给了他。
func (a *app) planCheckout(ref gitrepo.Ref) (checkoutPlan, bool) {
	switch ref.Kind {
	case gitrepo.RefLocal:
		if ref.Head {
			return checkoutPlan{noop: true, note: "已经在这条分支上了"}, true
		}
		return checkoutPlan{
			kind: gitrepo.RefLocal, name: ref.Name,
			menu:  "检出 " + ref.Name,
			label: "正在切换到 " + ref.Name,
			note:  "已切换到 " + ref.Name,
		}, true

	case gitrepo.RefRemote:
		local := remoteLocalName(ref)
		if existing, ok := a.refByFull["refs/heads/"+local]; ok {
			if existing.Head {
				return checkoutPlan{noop: true, note: "已经在 " + local + " 上了"}, true
			}
			return checkoutPlan{
				kind: gitrepo.RefLocal, name: existing.Name,
				menu:  "检出 " + local + "（本地已经有了）",
				label: "正在切换到 " + local,
				note:  "已切换到 " + local,
			}, true
		}
		return checkoutPlan{
			kind: gitrepo.RefRemote, name: local, ref: ref.Full,
			menu:  "检出并新建本地分支 " + local,
			label: "正在检出 " + local,
			note:  "已检出 " + local + "，跟踪 " + ref.Name,
		}, true

	case gitrepo.RefTag:
		return checkoutPlan{
			kind: gitrepo.RefTag, name: ref.Name,
			menu:  "检出标签 " + ref.Name + "（分离 HEAD）",
			label: "正在切换到标签 " + ref.Name,
			note:  "已切换到标签 " + ref.Name + "（分离 HEAD）",
		}, true
	}
	return checkoutPlan{}, false
}

// remoteLocalName 是远程分支落到本地该叫什么。
func remoteLocalName(ref gitrepo.Ref) string {
	return strings.TrimPrefix(ref.Name, ref.Remote+"/")
}

// checkoutable 说这条引用能不能检出。储藏不能：它不是一个可以「站上去」
// 的位置，只是暂存起来的一份改动。
func checkoutable(ref gitrepo.Ref) bool { return ref.Kind != gitrepo.RefStash }

// checkoutRef 检出侧栏里的一条引用。
func (a *app) checkoutRef(ref gitrepo.Ref) {
	if !a.repoReady() || a.op != gitrepo.OpNone {
		return
	}
	plan, ok := a.planCheckout(ref)
	if !ok {
		return
	}
	if plan.noop {
		// 说一声而不是默不作声：双击了没反应，用户分不清是「已经是这样了」
		// 还是「这个程序坏了」。
		a.opNote = plan.note
		return
	}

	a.startOp(gitrepo.OpCheckout, plan.label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		switch plan.kind {
		case gitrepo.RefRemote:
			return plan.note, repo.CheckoutRemote(ctx, plan.name, plan.ref)
		case gitrepo.RefTag:
			return plan.note, repo.CheckoutTag(ctx, plan.name)
		default:
			return plan.note, repo.CheckoutLocal(ctx, plan.name)
		}
	}, func(ok bool) {
		if ok {
			a.finishCheckout()
		}
		// 失败也要重读：checkout 失败可能是「工作区挡道」，而工作区在这期间
		// 未必没动过 —— 把眼前的真相重读一遍，比留在上一次的快照上强。
		a.reloadAfterOp()
	})
}

// finishCheckout 是换完分支之后的收尾：把界面挪回「改动」那一栏。
//
// 不调 setTab：它开头就是 `if !a.repoReady()` —— 而这里紧接着要打
// loadLoading，那一刻 repoReady 是 false，调了等于没调。它里面那几句
// （关卡片、清筛选）是这里必须做的，所以照抄一份。
func (a *app) finishCheckout() {
	a.tab = 0
	a.closeExplain()
	// 两个 tab 的文件集完全不同，筛选词跟着作废（和 setTab 一个道理）
	a.fileFilter = ""
	a.fileTreeFilter = ""

	// 「在看哪条历史」全部作废。histRev 存的是引用的全名，换完分支它多半
	// 还解析得到（refs/heads/main 一直都在），照着读会得到一份和眼前对不上
	// 的历史；refSel 是侧栏的选中态，指向的那棵树也已经重建过了。
	a.histRev = ""
	a.stashRev = ""
	a.commitSel = -1
	a.refSel = ""
	a.commits = nil
	a.histFiles = nil
	a.histListID = ""
	a.hist = loadIdle
	a.histDiff = loadIdle

	// 在飞的「读历史」「读那个提交」作废。它们问的是换分支之前的仓库，而
	// 结果回来时会顺手选中最近那个提交（刚把 commitSel 清成 -1，正好撞上
	// 那条自动选中），于是「在看哪条历史」又自己长回来一个 —— 界面回到了
	// 改动栏，可下半区已经被人从背后换过了。
	a.logSeq++
	a.diffSeq++
}

// workListID 是「改动」这份列表的身份：仓库 + 分支。
//
// 带上分支是因为切分支换的是整份内容 —— 同一个路径在另一条分支上是另一段
// 历史，行号对不上，留着上一条分支摊开的区间只会在新分支上冒出一段谁也没
// 展开过的内容。提交不动这个 id（分支没变），所以提交完展开状态还在。
func workListID(repoPath, branch string) string {
	return "w:" + repoPath + "@" + branch
}

// refMenu 是侧栏一条引用的右键菜单。
func (a *app) refMenu(m *ui.Menu, ref gitrepo.Ref) {
	switch ref.Kind {
	case gitrepo.RefLocal:
		a.localBranchMenu(m, ref)
	case gitrepo.RefRemote:
		a.remoteBranchMenu(m, ref)
	case gitrepo.RefTag:
		a.tagMenu(m, ref)
	case gitrepo.RefStash:
		a.stashMenu(m, ref)
	default:
		plan, ok := a.planCheckout(ref)
		if !ok {
			return
		}
		busy := a.op != gitrepo.OpNone
		if m.Item(plan.menu).Disabled(busy || plan.noop).Chosen() {
			a.checkoutRef(ref)
		}
	}
}

// localBranchMenu 是本地分支的右键菜单：检出、合并、变基、重命名、删除、复制分支名。
func (a *app) localBranchMenu(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	isHead := ref.Head || ref.Name == a.branch
	held := busy || isHead || a.pending.Active()

	if m.Item("检出 " + ref.Name).Disabled(busy || isHead).Chosen() {
		a.checkoutRef(ref)
	}
	m.Separator()
	if m.Item("把 " + ref.Name + " 合并进当前分支").Disabled(held).Chosen() {
		a.mergeBranch(ref.Name)
	}
	if m.Item("把当前分支变基到 " + ref.Name + "…").Disabled(held).Chosen() {
		a.promptRebase(ref.Name)
	}
	m.Separator()
	if m.Item("重命名 " + ref.Name + "…").Disabled(busy).Chosen() {
		a.openRenameBranchDialog(ref.Name)
	}
	if m.Item("删除分支 " + ref.Name + "…").Disabled(busy || isHead).Chosen() {
		a.promptDeleteBranch(ref.Name)
	}
	m.Separator()
	if m.Item("复制分支名").Disabled(busy).Chosen() {
		a.copyText = ref.Name
	}
}

// remoteBranchMenu 是远程分支的右键菜单：检出、删除远程分支、复制分支名。
func (a *app) remoteBranchMenu(m *ui.Menu, ref gitrepo.Ref) {
	plan, ok := a.planCheckout(ref)
	if !ok {
		return
	}
	busy := a.op != gitrepo.OpNone
	if m.Item(plan.menu).Disabled(busy || plan.noop).Chosen() {
		a.checkoutRef(ref)
	}
	m.Separator()
	remote := ref.Remote
	if remote == "" {
		remote = "origin"
	}
	localName := remoteLocalName(ref)
	canRemote := a.canReachRemote()
	if m.Item("删除远程分支 " + ref.Name + "…").Disabled(!canRemote).Chosen() {
		a.promptDeleteRemoteBranch(remote, localName)
	}
	m.Separator()
	if m.Item("复制分支名").Disabled(busy).Chosen() {
		a.copyText = ref.Name
	}
}

// tagMenu 是标签的右键菜单：检出、推送到远程、复制标签名。
func (a *app) tagMenu(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	if m.Item("检出标签 " + ref.Name + "（分离 HEAD）").Disabled(busy).Chosen() {
		a.checkoutRef(ref)
	}
	m.Separator()
	canRemote := a.canReachRemote()
	if m.Item("推送标签 " + ref.Name + " 到 origin").Disabled(!canRemote).Chosen() {
		a.pushTag("origin", ref.Name)
	}
	m.Separator()
	if m.Item("复制标签名").Disabled(busy).Chosen() {
		a.copyText = ref.Name
	}
}

// stashMenu 是储藏条目的右键菜单：应用、取出、丢弃、复制储藏名。
func (a *app) stashMenu(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	held := busy || a.pending.Active()

	if m.Item("应用储藏（Apply）").Disabled(held).Chosen() {
		a.stashApply(ref)
	}
	if m.Item("应用并丢弃（Pop）").Disabled(held).Chosen() {
		a.stashPop(ref)
	}
	m.Separator()
	if m.Item("丢弃储藏（Drop）…").Disabled(busy).Chosen() {
		a.promptStashDrop(ref)
	}
	m.Separator()
	if m.Item("复制储藏名").Disabled(busy).Chosen() {
		a.copyText = ref.Name
	}
}
