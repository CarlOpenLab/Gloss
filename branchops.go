package main

import (
	"context"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 分支与远程操作：
// - 本地分支：删除（含未合并二次确认）、合并进当前分支、变基到目标分支；
// - 远程分支：删除远程分支（带二次确认）；
// - 标签：推送标签到远程；
// - 远程推送：强制推送（--force-with-lease 二次确认）。

// promptDeleteBranch 询问是否删除本地分支。当前检出的分支不允许删除。
func (a *app) promptDeleteBranch(name string) {
	if !a.repoReady() || a.op != gitrepo.OpNone || name == a.branch {
		return
	}
	a.askConfirm(confirmState{
		title: "删除分支 " + name,
		body:  "确定要删除本地分支 " + name + " 吗？",
		yes:   "删除分支",
		onYes: func() {
			a.deleteBranch(name, false)
		},
	})
}

// deleteBranch 删除分支。未合并时触发二次确认（-d 拒绝转 -D 强制）。
func (a *app) deleteBranch(name string, force bool) {
	title := "正在删除分支 " + name
	if force {
		title = "正在强制删除分支 " + name
	}
	a.startOp(gitrepo.OpBranch, title,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已删除分支 " + name, repo.DeleteBranch(ctx, name, force)
		}, func(ok bool) {
			if !ok {
				// 未合并分支被 -d 拒收：弹二次确认
				if a.opErr != nil && strings.Contains(a.opErr.Title, "还没合并") {
					target := a.branch
					if target == "" {
						target = "HEAD"
					}
					a.opErr = nil // 不挂在顶部横幅，转入对话框确认
					a.askConfirm(confirmState{
						title: name + " 还没合并进 " + target,
						body:  "它上面有没并进当前分支的提交。删掉之后就找不回来了（reflog 里短时间还能捞回来）。",
						yes:   "仍然删除",
						onYes: func() {
							a.deleteBranch(name, true)
						},
					})
				}
				return
			}
			a.loadRefs()
		})
}

// mergeBranch 把 branch 合并进当前分支。
//
// 干净合并自动创建提交并刷新；撞冲突时 git 停在 MERGE_HEAD，
// 失败横幅提示冲突，同时重读仓库后切入 PendingBar（显示冲突与继续/放弃操作）。
func (a *app) mergeBranch(name string) {
	if !a.repoReady() || a.op != gitrepo.OpNone || a.pending.Active() {
		return
	}
	curr := a.commitMenuBranch()
	a.startOp(gitrepo.OpMerge, "正在合并 "+name,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已把 " + name + " 合并进 " + curr, repo.Merge(ctx, name)
		}, func(bool) {
			a.finishCheckout()
			a.reloadAfterOp()
		})
}

// promptRebase 弹出变基确认框。变基会改写提交历史，必须经用户确认。
func (a *app) promptRebase(upstream string) {
	if !a.repoReady() || a.op != gitrepo.OpNone || a.pending.Active() {
		return
	}
	curr := a.commitMenuBranch()
	a.askConfirm(confirmState{
		title: "变基 " + curr + " 到 " + upstream,
		body:  "将把 " + curr + " 上的未合并提交移到 " + upstream + " 的最新提交之后。\n变基会改写提交历史。如果出现冲突，需要逐个解决或放弃变基。",
		yes:   "开始变基",
		onYes: func() {
			a.rebaseOnto(upstream)
		},
	})
}

// rebaseOnto 把当前分支变基到 upstream。
func (a *app) rebaseOnto(upstream string) {
	curr := a.commitMenuBranch()
	a.startOp(gitrepo.OpRebase, "正在变基到 "+upstream,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已把 " + curr + " 变基到 " + upstream, repo.Rebase(ctx, upstream)
		}, func(bool) {
			a.finishCheckout()
			a.reloadAfterOp()
		})
}

// promptDeleteRemoteBranch 弹出删除远程分支确认框。
func (a *app) promptDeleteRemoteBranch(remote, branch string) {
	if !a.canReachRemote() {
		return
	}
	fullName := remote + "/" + branch
	a.askConfirm(confirmState{
		title: "删除远程分支 " + fullName,
		body:  "确定要从 " + remote + " 删除远程分支 " + branch + " 吗？\n这会从远端彻底移除该分支，其他协作者将无法再访问它。你的本地分支不会被删除。",
		yes:   "删除远程分支",
		onYes: func() {
			a.deleteRemoteBranch(remote, branch)
		},
	})
}

// deleteRemoteBranch 删除指定远程上的分支（git push <remote> --delete <branch>）。
func (a *app) deleteRemoteBranch(remote, branch string) {
	fullName := remote + "/" + branch
	a.startOp(gitrepo.OpPush, "正在删除远程分支 "+fullName,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已删除远程分支 " + fullName, repo.DeleteRemoteBranch(ctx, remote, branch)
		}, func(ok bool) {
			if ok {
				a.loadRefs()
			}
		})
}

// pushTag 把本地标签推送到指定远程。
func (a *app) pushTag(remote, tag string) {
	if remote == "" {
		remote = "origin"
	}
	a.startOp(gitrepo.OpPush, "正在推送标签 "+tag,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已推送标签 " + tag + " 到 " + remote, repo.PushTag(ctx, remote, tag)
		}, nil)
}

// hasUpstream 判断 branch 是否配置了上游远程分支。
func (a *app) hasUpstream(branch string) bool { return a.upstreamOf(branch) != "" }

// upstreamOf 返回本地分支跟踪的上游远程分支名（形如 origin/main），没配
// 上游时为空。
//
// 「删除远程分支」要知道删的是哪一个：上游名里有远程名也有分支名，本地面板
// 上右键一个分支，得从它自己的上游推出来，不能猜 origin。
func (a *app) upstreamOf(branch string) string {
	if branch == "" || a.refByFull == nil {
		return ""
	}
	ref, ok := a.refByFull["refs/heads/"+branch]
	if !ok {
		return ""
	}
	return ref.Upstream
}

// splitUpstream 把上游名的远程与分支拆开：第一个 / 之前是远程名，其余是分支名
// —— 分支名自己带 / 很正常（feat/x），所以只按第一个拆。
func splitUpstream(upstream string) (remote, branch string) {
	if i := strings.Index(upstream, "/"); i > 0 {
		return upstream[:i], upstream[i+1:]
	}
	return "", upstream
}

// promptForcePush 弹出强制推送二次确认。
func (a *app) promptForcePush() {
	if !a.canReachRemote() || a.detached || !a.hasUpstream(a.branch) {
		return
	}
	branch := a.branch
	ref := a.refByFull["refs/heads/"+branch]
	upstream := "远程分支"
	if ref != nil && ref.Upstream != "" {
		upstream = ref.Upstream
	}
	a.askConfirm(confirmState{
		title: "强制推送 " + branch,
		body:  "将使用 --force-with-lease 覆盖 " + upstream + "。\n这会改写远程提交历史。如果别人在你上次抓取后向该分支推送了新提交，Git 会安全地拒绝本次操作。",
		yes:   "强制推送",
		onYes: func() {
			a.doForcePush()
		},
	})
}

// doForcePush 执行带有租约保护的强制推送。
func (a *app) doForcePush() {
	if !a.canReachRemote() || a.detached {
		return
	}
	branch := a.branch
	a.startOp(gitrepo.OpForcePush, "正在强制推送 "+branch,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已强制推送 " + branch, repo.PushForce(ctx, branch)
		}, func(ok bool) {
			if ok {
				a.loadRefs()
			}
		})
}
