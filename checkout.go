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
		// 被别的 worktree 检出着：git 会拒绝，而且用户真正想做的多半是
		// 「去那个工作区里操作」，不是在这里再切一次。
		if ref.Worktree != "" {
			return checkoutPlan{noop: true, note: "已在 worktree " + ref.Worktree + " 检出"}, true
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
			if existing.Worktree != "" {
				return checkoutPlan{noop: true, note: "已在 worktree " + existing.Worktree + " 检出"}, true
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
// loadLoading，那一刻 repoReady 是 false，调了等于没调。它里面清筛选
// 那几句是这里必须做的，所以照抄一份。
func (a *app) finishCheckout() {
	// 视图也要管：从编辑器视图里双击检出时，光把 a.tab 置成 0，屏幕还停在
	// 编辑器那一栏 —— 用户看到的是「双击了，界面没动」（见 enterHistory）。
	if a.viewMode != viewModeGit {
		a.setViewMode(viewModeGit)
	}
	a.tab = 0
	// 两个 tab 的文件集完全不同，筛选词跟着作废（和 setTab 一个道理）
	a.fileFilter = ""
	a.fileTreeFilter = ""

	// 「在看哪条历史」全部作废。histRev 存的是引用的全名，换完分支它多半
	// 还解析得到（refs/heads/main 一直都在），照着读会得到一份和眼前对不上
	// 的历史；refSel 是侧栏的选中态，指向的那棵树也已经重建过了。
	a.histRev = ""
	a.histAll = false
	a.stashRev = ""
	a.commitSel = -1
	a.refSel = ""
	a.commits = nil
	a.allCommits = nil
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

// localBranchMenu 是本地分支的右键菜单：检出、对比差异、合并、变基、重命名、
// 删除本地分支（有上游时还能就地删远程那份）、复制分支名。
func (a *app) localBranchMenu(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	isHead := ref.Head || ref.Name == a.branch
	// 被别的 worktree 检出着：检出和删除 git 都会拒绝，菜单上也不该摆成
	// 能点的样子（删除项同理，见下面）。
	occupied := ref.Worktree != "" && !isHead
	held := busy || isHead || a.pending.Active()

	if m.Item("检出 " + ref.Name).Disabled(busy || isHead || occupied).Chosen() {
		a.checkoutRef(ref)
	}
	a.worktreeMenuItems(m, ref)
	m.Separator()
	// 对比两项都不设 busy：它们只是读两个 ref 的内容算 diff（git diff base..
	// target），不碰 index 也不碰工作区 —— 推送正在飞的时候比一下两个分支
	// 完全站得住。弹窗自己也是开在另一层上的（见 compareModal）。
	if !isHead && a.branch != "" {
		if m.Item("与当前分支 (" + a.branch + ") 对比代码差异").Chosen() {
			a.startCompare(a.branch, ref.Name)
		}
	}
	if m.Item("与其他分支对比代码差异…").Chosen() {
		a.openCompareDialog(ref.Name)
	}
	m.Separator()
	if m.Item("把 " + ref.Name + " 合并进当前分支").Disabled(held).Chosen() {
		a.mergeBranch(ref.Name)
	}
	if m.Item("把当前分支变基到 " + ref.Name + "…").Disabled(held).Chosen() {
		a.promptRebase(ref.Name)
	}
	m.Separator()
	// 重命名不设 busy：它只是弹一个对话框填新名字，git 那条命令在确认框里
	// 才发（见 renameBranch，那儿自己有 busy 兜底）。拉取正在飞的时候先把
	// 名字想好、对话框开着，没碍着谁。
	if m.Item("重命名 " + ref.Name + "…").Chosen() {
		a.openRenameBranchDialog(ref.Name)
	}
	if m.Item(T("删除分支 "+ref.Name+"…", "Delete Branch "+ref.Name+"…")).Disabled(busy || isHead || occupied).Chosen() {
		a.promptDeleteBranch(ref.Name)
	}
	// 本地分支也顺手能删掉它在远程那份：删的仍然是**远程分支**，与上一条删
	// 本地完全两回事，所以是独立的菜单项、独立的二次确认（确认框里会写明
	// 本地分支不动）。
	//
	// 只有这个分支真有上游时才摆出来 —— 没有上游就无从知道该删哪个远程分支，
	// 摆一条永远灰着的只是噪音。
	if upstream := a.upstreamOf(ref.Name); upstream != "" {
		remote, upstreamBranch := splitUpstream(upstream)
		if m.Item(T("删除远程分支 "+upstream+"…", "Delete Remote Branch "+upstream+"…")).
			Disabled(busy || !a.canReachRemote()).Chosen() {
			a.promptDeleteRemoteBranch(remote, upstreamBranch)
		}
	}
	m.Separator()
	// 复制名字不设 busy：同上（commitops 的复制哈希）—— 只是把已经在那儿的
	// 一串字符写进剪贴板。
	if m.Item(T("复制分支名", "Copy Branch Name")).Chosen() {
		a.copyText = ref.Name
	}
}

// worktreeMenuItems 在本地分支菜单的「检出」之后插入 worktree 相关动作。
//
// 三种处境各给各的出路：分支被正常 worktree 占着 → 打开或移除那个工作区；
// 记录失效（目录没了）→ 只给清理；分支空闲 → 检出到一个新工作区。
func (a *app) worktreeMenuItems(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	occupied := ref.Worktree != "" && !ref.Head
	switch {
	case occupied && ref.WorktreePrunable:
		if m.Item(T("清理失效 Worktree 记录", "Prune Stale Worktree Record")).Disabled(busy).Chosen() {
			a.pruneWorktrees()
		}
	case occupied:
		if m.Item(T("打开所在 Worktree", "Open Worktree")).Disabled(busy).Chosen() {
			// 推迟一帧再换项目：菜单还开着的时候重铺标签页，会把这一帧
			// 已经挂好的控件和新的活动项目对到一起去（见 recent 项目行同款
			// 的 dispatch）。
			a.dispatch(func() { a.openInNewTab(ref.Worktree) })
		}
		if !ref.WorktreeMain {
			if m.Item(T("移除 Worktree…", "Remove Worktree…")).Disabled(busy).Chosen() {
				a.promptRemoveWorktree(ref.Worktree, false)
			}
		}
	case !ref.Head && a.repoReady():
		if m.Item(T("在新 Worktree 中检出…", "Check Out in New Worktree…")).Disabled(busy).Chosen() {
			a.openNewWorktreeDialog(ref.Name)
		}
	}
}

// remoteBranchMenu 是远程分支的右键菜单：检出、对比差异、删除远程分支、复制分支名。
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
	// 对比不设 busy（同 localBranchMenu：读两个 ref 算 diff，不碰工作区）。
	if a.branch != "" {
		if m.Item(T("与当前分支 ("+a.branch+") 对比代码差异", "Compare with current branch ("+a.branch+")")).Chosen() {
			a.startCompare(a.branch, ref.Name)
		}
	}
	if m.Item(T("与其他分支对比代码差异…", "Compare with other branch…")).Chosen() {
		a.openCompareDialog(ref.Name)
	}
	m.Separator()
	remote := ref.Remote
	if remote == "" {
		remote = "origin"
	}
	localName := remoteLocalName(ref)
	canRemote := a.canReachRemote()
	if m.Item(T("删除远程分支 "+ref.Name+"…", "Delete Remote Branch "+ref.Name+"…")).Disabled(!canRemote).Chosen() {
		a.promptDeleteRemoteBranch(remote, localName)
	}
	m.Separator()
	// 复制名字不设 busy：同上（commitops 的复制哈希）—— 只是把已经在那儿的
	// 一串字符写进剪贴板。
	if m.Item(T("复制分支名", "Copy Branch Name")).Chosen() {
		a.copyText = ref.Name
	}
}

// tagMenu 是标签的右键菜单：检出、对比差异、推送到远程、复制标签名。
func (a *app) tagMenu(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	if m.Item(T("检出标签 "+ref.Name+"（分离 HEAD）", "Checkout Tag "+ref.Name+" (Detached HEAD)")).Disabled(busy).Chosen() {
		a.checkoutRef(ref)
	}
	m.Separator()
	// 对比不设 busy（同 localBranchMenu）。
	if a.branch != "" {
		if m.Item(T("与当前分支 ("+a.branch+") 对比代码差异", "Compare with current branch ("+a.branch+")")).Chosen() {
			a.startCompare(a.branch, ref.Name)
		}
	}
	if m.Item(T("与其他分支/标签对比代码差异…", "Compare with other branch/tag…")).Chosen() {
		a.openCompareDialog(ref.Name)
	}
	m.Separator()
	canRemote := a.canReachRemote()
	if m.Item(T("推送标签 "+ref.Name+" 到 origin", "Push Tag "+ref.Name+" to origin")).Disabled(!canRemote).Chosen() {
		a.pushTag("origin", ref.Name)
	}
	m.Separator()
	if m.Item(T("复制标签名", "Copy Tag Name")).Chosen() {
		a.copyText = ref.Name
	}
}

// stashMenu 是储藏条目的右键菜单：应用、取出、丢弃、复制储藏名。
func (a *app) stashMenu(m *ui.Menu, ref gitrepo.Ref) {
	busy := a.op != gitrepo.OpNone
	held := busy || a.pending.Active()

	if m.Item(T("应用储藏（Apply）", "Apply Stash")).Disabled(held).Chosen() {
		a.stashApply(ref)
	}
	if m.Item(T("应用并丢弃（Pop）", "Pop Stash")).Disabled(held).Chosen() {
		a.stashPop(ref)
	}
	m.Separator()
	if m.Item(T("丢弃储藏（Drop）…", "Drop Stash…")).Disabled(busy).Chosen() {
		a.promptStashDrop(ref)
	}
	m.Separator()
	if m.Item(T("复制储藏名", "Copy Stash Name")).Chosen() {
		a.copyText = ref.Name
	}
}
