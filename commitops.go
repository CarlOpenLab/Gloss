package main

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 提交行右键点开之后的那些事：拣选、回退、重置、从这里建分支、检出这个
// 提交、复制哈希。
//
// 菜单顺序照「顺手程度」排：加东西的（标签、拣选、回退）在前，挪位置的
//（重置、建分支、检出）在中，拿走一个值的（复制哈希）收尾。
//
// 「卡在半路」（Pending != None）时前几条直接灰掉。不灰也能料到 git 会
// 拦（unmerged files、MERGE_HEAD exists……），但有一个例外必须我们自己
// 拦：**reset --hard 会静默地把没收尾的那一步连同冲突一起抹掉**（实测：
// merge 停住时 reset --hard，MERGE_HEAD 直接消失，工作区回到 HEAD）。
// 一颗按下去能把用户解决了一半的合并悄悄丢掉的按钮，不该亮着。

// commitMenu 是提交行的右键菜单。
func (a *app) commitMenu(m *ui.Menu, cm gitrepo.Commit) {
	busy := a.op != gitrepo.OpNone
	// 卡在合并/变基中间：拣选/回退/重置都被 git 拒收，不摆一颗按了必响
	// 的按钮。检出不在此列 —— 从半路跳到别的提交本来就被 git 挡
	//（you need to resolve your current index first），拦它反而多一层。
	held := busy || a.pending.Active()

	if m.Item("在 " + cm.Short + " 上新建标签…").Disabled(busy).Chosen() {
		a.openTagDialog(cm.Hash, cm.Short+" · "+cm.Subject)
	}
	if m.Item("拣选这个提交（cherry-pick 到当前分支）").Disabled(held).Chosen() {
		a.cherryPickCommit(cm)
	}
	if m.Item("回退这个提交（生成一条反向提交）").Disabled(held).Chosen() {
		a.revertCommit(cm)
	}
	m.Separator()
	if m.Item("重置 " + a.commitMenuBranch() + " 到 " + cm.Short + "…").Disabled(held).Chosen() {
		a.openResetDialog(cm)
	}
	if m.Item("从 " + cm.Short + " 建分支…").Disabled(busy).Chosen() {
		a.openNewBranchDialog(cm.Hash, cm.Short+" · "+cm.Subject)
	}
	if m.Item(T("检出这个提交（分离 HEAD）", "Checkout This Commit (Detached HEAD)")).Disabled(busy).Chosen() {
		a.detachAt(cm)
	}
	m.Separator()
	if m.Item(T("复制哈希", "Copy Commit Hash")).Disabled(busy).Chosen() {
		// 菜单回调里拿不到 *ui.Context（WriteClipboard 是它的方法），
		// 记一笔，下一帧开头再写进剪贴板（见 view）。
		a.copyText = cm.Hash
	}
}

// commitMenuBranch 是重置菜单里那个「重置谁」的名字。
//
// HEAD 分离时 a.branch 是短哈希，直接拼进菜单会读成「重置 3f9a2c1 到
// a1b2c3d…」—— 两个哈希并排，看着像要重置那个提交。分离时就说 HEAD。
func (a *app) commitMenuBranch() string {
	if a.detached || a.branch == "" {
		return "HEAD"
	}
	return a.branch
}

// cherryPickCommit 把一条提交拣到当前分支上。
//
// 收货时切回「改动」栏：拣选成不成、撞不撞冲突，答案都在工作区里 ——
// 成了是多出一条提交，撞了是冲突文件列表，被工作区挡下时那些挡道的
// 改动本身就在「改动」栏里。
func (a *app) cherryPickCommit(cm gitrepo.Commit) {
	a.startOp(gitrepo.OpCherryPick, "正在拣选 "+cm.Short,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已拣选 " + cm.Short + "：" + cm.Subject, repo.CherryPick(ctx, cm.Hash)
		}, func(bool) {
			a.finishCheckout()
			a.reloadAfterOp()
		})
}

// revertCommit 生成一条反向提交。
func (a *app) revertCommit(cm gitrepo.Commit) {
	a.startOp(gitrepo.OpRevert, "正在回退 "+cm.Short,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已生成反向提交（回退 " + cm.Short + "）", repo.Revert(ctx, cm.Hash)
		}, func(bool) {
			a.finishCheckout()
			a.reloadAfterOp()
		})
}

// detachAt 检出某个提交（分离 HEAD）。
//
// 和检出标签走同一套收尾（checkoutRef → finishCheckout）：换过 HEAD 之后
// 「在看哪条历史」整个作废，界面挪回「改动」栏。
func (a *app) detachAt(cm gitrepo.Commit) {
	a.startOp(gitrepo.OpCheckout, "正在检出 "+cm.Short,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已检出 " + cm.Short + "（分离 HEAD）", repo.CheckoutDetach(ctx, cm.Hash)
		}, func(ok bool) {
			if ok {
				a.finishCheckout()
			}
			a.reloadAfterOp()
		})
}
