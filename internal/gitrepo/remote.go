package gitrepo

import (
	"context"
	"errors"
	"strings"
)

// 远程：抓取、拉取、推送。
//
// 三个都是「要出门」的命令，可能等上好几秒，也可能停在凭证上 —— 但它们和
// 本地写操作共用一条闸门（见 app 的 startOp）：拉取会动工作区和 index，
// 推送要读 refs，同时跑两个只会让失败变得没法解释。GIT_TERMINAL_PROMPT=0
// 保证绝不会挂在一个没人能回答的提示上；需要登录时 git 直接报错，界面把那句
// 话翻成「在终端里先登录一次」。

// ErrNoRemote 表示这个仓库没有可以推的远程。
//
// 单独一个错误而不是让 git 去报：`git push` 在没有上游时给的是
// "The current branch x has no upstream branch"，那句话指向的是「设上游」，
// 而这里真正的问题是「一条远程都没有」—— 两件事的出路完全不同。
var ErrNoRemote = errors.New("no remote to push to")

// ErrNoBranch 表示要推的那条分支在本地不存在。
//
// 界面在分离 HEAD 上把推送按钮画成灰的，但它还有菜单和命令栏两条路进来，
// 而这时候 branch 是**短哈希**（见 branchInfo）—— 直接拿它去推，git 报的是
// "src refspec a1b2c3d does not match any"，翻成人话是「这条引用已经不在了」，
// 和真正的原因（没有分支在手上）差着十万八千里。
var ErrNoBranch = errors.New("not on a branch")

// Fetch 抓取所有远程的分支和标签，并清掉远端已经删掉的引用。
//
// --all：一次把所有远程都抓下来，不用挑。
// --prune：远端删掉的分支这里跟着消失。不 prune 的话侧栏会一直挂着一条早
// 就不存在的分支，点它还报「这条引用已经不在了」—— 明明是它的错，却要用户
// 自己反应过来是远端删了。
//
// 没有远程时 `git fetch --all` 静默退 0（什么都不做，也不报错），所以「能不
// 能抓」要由上层判（Model.Remotes），不能等这条命令失败 —— 那种「点了没反应」
// 比灰按钮难受得多。
func (r *Repo) Fetch(ctx context.Context) error {
	_, err := run(ctx, r.Root, "fetch", "--all", "--prune")
	return err
}

// Pull 把当前分支的上游拉下来合进当前分支。
//
// --no-edit：合并需要一段提交信息时（真合并，不是快进），git 会去拉编辑器；
// GUI 进程没有终端，那是一次静默的挂起。给了它就自动用默认那句。
//
// 不加 --ff-only，也不指定 --rebase：用户配了 pull.rebase / pull.ff 就按他
// 的来，那是他的仓库和他的习惯。
func (r *Repo) Pull(ctx context.Context) error {
	_, err := run(ctx, r.Root, "pull", "--no-edit")
	return err
}

// Push 推送 branch 这条分支。
//
// 已经有上游就直接推。没有上游时用 origin 建一条（`-u`）：第一次把新分支推
// 上去是最常见的场景，让它失败、逼用户去终端里配一次上游，是把我们该做的判断
// 推给了用户。
//
// 没有 origin 就返回 ErrNoRemote，**不**从别的远程里挑一个：多个远程时哪一个
// 才是「那个远程」只有用户知道，猜错了就是把代码推到了不该去的地方。
func (r *Repo) Push(ctx context.Context, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" || !r.hasBranch(ctx, branch) {
		return ErrNoBranch
	}
	if r.upstream(ctx, branch) != "" {
		_, err := run(ctx, r.Root, "push")
		return err
	}
	remotes, err := r.Remotes(ctx)
	if err != nil {
		return err
	}
	if !hasRemote(remotes, "origin") {
		return ErrNoRemote
	}
	_, err = run(ctx, r.Root, "push", "-u", "origin", branch)
	return err
}

// hasBranch 说本地有没有这条分支。
//
// 拿它先验一次，而不是让 `git push` 自己撞：空仓库、分离 HEAD、拼错的分支名
// 都会走到这里，而 git 对这三种情况各有各的绕法。
func (r *Repo) hasBranch(ctx context.Context, branch string) bool {
	out, err := run(ctx, r.Root, "for-each-ref", "--format=%(refname)",
		"refs/heads/"+branch)
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// upstream 返回 branch 的上游完整引用名，没有上游时返回空串。
//
// 用 for-each-ref 而不是 `rev-parse --abbrev-ref @{u}`：后者在没有上游时
// 报错、还会把错误混进 stderr，而这里「没有上游」是正常情况，不是异常。
func (r *Repo) upstream(ctx context.Context, branch string) string {
	out, err := run(ctx, r.Root, "for-each-ref", "--format=%(upstream)",
		"refs/heads/"+branch)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func hasRemote(remotes []string, name string) bool {
	for _, r := range remotes {
		if r == name {
			return true
		}
	}
	return false
}

// Upstream 返回 branch 的上游引用名，没有上游时返回空串。
func (r *Repo) Upstream(ctx context.Context, branch string) string {
	return r.upstream(ctx, branch)
}

// PushForce 强制推送当前分支（--force-with-lease）。
//
// 用 --force-with-lease 而不是裸 --force：只在远程分支还是本地上次抓取
// 时的样子才覆盖，别人新推上去的提交不会被静默吃掉。别人推过时 git 报错
// 并含 "stale info"，failure.go 会翻成「远程上有你还没见过的提交」。
func (r *Repo) PushForce(ctx context.Context, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" || !r.hasBranch(ctx, branch) {
		return ErrNoBranch
	}
	_, err := run(ctx, r.Root, "push", "--force-with-lease")
	return err
}

// PushTag 把本地标签推送到指定远程。
func (r *Repo) PushTag(ctx context.Context, remote, tag string) error {
	if remote == "" {
		remote = "origin"
	}
	_, err := run(ctx, r.Root, "push", remote, "refs/tags/"+tag)
	return err
}

// DeleteRemoteBranch 删除指定远程上的分支（git push <remote> --delete <branch>）。
func (r *Repo) DeleteRemoteBranch(ctx context.Context, remote, branch string) error {
	_, err := run(ctx, r.Root, "push", remote, "--delete", branch)
	return err
}
