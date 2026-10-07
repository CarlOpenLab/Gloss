package main

import (
	"context"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 储藏操作：应用、取出、丢弃。

// stashApply 应用指定储藏的内容（git stash apply <name>）。
func (a *app) stashApply(ref gitrepo.Ref) {
	if !a.repoReady() || a.op != gitrepo.OpNone || a.pending.Active() {
		return
	}
	a.startOp(gitrepo.OpStash, "正在应用 "+ref.Name,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已应用 " + ref.Name, repo.StashApply(ctx, ref.Name)
		}, func(bool) {
			a.finishCheckout()
			a.reloadAfterOp()
		})
}

// stashPop 取出指定储藏的内容并从储藏列表丢弃（git stash pop <name>）。
func (a *app) stashPop(ref gitrepo.Ref) {
	if !a.repoReady() || a.op != gitrepo.OpNone || a.pending.Active() {
		return
	}
	a.startOp(gitrepo.OpStash, "正在取出并丢弃 "+ref.Name,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已取出并丢弃 " + ref.Name, repo.StashPop(ctx, ref.Name)
		}, func(bool) {
			a.finishCheckout()
			a.reloadAfterOp()
		})
}

// promptStashDrop 询问是否丢弃储藏。
func (a *app) promptStashDrop(ref gitrepo.Ref) {
	if !a.repoReady() || a.op != gitrepo.OpNone {
		return
	}
	desc := ref.Subject
	if desc == "" {
		desc = ref.Name
	}
	a.askConfirm(confirmState{
		title: "丢弃储藏 " + ref.Name,
		body:  "确定要丢弃储藏 " + ref.Name + " 吗？\n" + desc + "\n丢弃后该储藏的改动将无法找回。",
		yes:   "丢弃储藏",
		onYes: func() {
			a.stashDrop(ref)
		},
	})
}

// stashDrop 删除指定储藏（git stash drop <name>）。
func (a *app) stashDrop(ref gitrepo.Ref) {
	a.startOp(gitrepo.OpStash, "正在丢弃 "+ref.Name,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已丢弃 " + ref.Name, repo.StashDrop(ctx, ref.Name)
		}, func(ok bool) {
			if ok {
				a.loadRefs()
			}
		})
}
