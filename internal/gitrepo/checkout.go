package gitrepo

import (
	"context"
)

// 检出与标签。和 write.go 那几个暂存口子一样：只发命令，不做界面上的判断
// （该不该给这条引用挂菜单、本地重名怎么办，都在上层）。

// CheckoutLocal 检出本地分支。
//
// 传**短名**，不传 refs/heads/x：checkout 拿全名时会当成一个 commit-ish，
// HEAD 变成分离的（量过：`git checkout refs/heads/topic` 会打印
// "Note: switching to 'refs/heads/topic'." 然后 detached）。只有名字确实是
// 分支名，git 才把 HEAD 附上去。
//
// 同名标签会让 git 打一句 warning 然后按 heads 优先消歧 —— checkout 和
// rev-parse 不一样，它站在分支这一边，正是我们要的（Log 那边必须传全名，
// 见 Ref.Full 的说明）。
func (r *Repo) CheckoutLocal(ctx context.Context, name string) error {
	_, err := run(ctx, r.Root, "checkout", name)
	return err
}

// CheckoutRemote 从远程分支建一条本地分支并跟踪它。
//
// remoteRef 要传全名（refs/remotes/origin/x）：短名 origin/x 在仓库里同时
// 有同名分支时会指向别处，而这一步建出来的分支名和上游关系都靠它定。
//
// 本地重名不在这里兜：`git checkout -b x --track …` 遇到本地已有 x 会直接
// 报 "a branch named 'x' already exists"，上层先查一遍本地引用，重名时
// 降级成检出本地那条（见 app 的 checkoutRef）。
func (r *Repo) CheckoutRemote(ctx context.Context, local, remoteRef string) error {
	_, err := run(ctx, r.Root, "checkout", "-b", local, "--track", remoteRef)
	return err
}

// CheckoutTag 检出标签。
//
// --detach 是明写出来的：`git checkout refs/tags/v1` 本来也会分离 HEAD
// （全名不是分支名），但那样 git 会先念一段 detached HEAD 的建议 ——
// 分离是我们主动要的，就自己说清楚，别让它来提醒。
func (r *Repo) CheckoutTag(ctx context.Context, name string) error {
	_, err := run(ctx, r.Root, "checkout", "--detach", "refs/tags/"+name)
	return err
}

// CreateTag 在 rev 上打一个标签：msg 非空 = 附注标签（带作者、日期和说明
// 的对象），为空 = 轻量标签（一个指向提交的指针）。
//
// 不用 `-a -m ""` 造「空附注标签」：git 会直接拒收（no tag message?），
// 而 `-a` 不带 `-m` 会去拉编辑器 —— GUI 进程没有终端，那是一次静默的挂起。
// rev 为空表示 HEAD。
func (r *Repo) CreateTag(ctx context.Context, name, msg, rev string) error {
	args := []string{"tag"}
	if msg != "" {
		args = append(args, "-a", "-m", msg)
	}
	args = append(args, name)
	if rev != "" {
		args = append(args, rev)
	}
	_, err := run(ctx, r.Root, args...)
	return err
}
