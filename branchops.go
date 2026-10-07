package main

import (
	"context"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 分支操作：删除分支（含未合并二次确认）。

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
