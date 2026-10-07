package gitrepo

import (
	"context"
	"errors"
	"strings"
)

// ErrBranchNotMerged 表示分支上有尚未合并进当前分支的提交。
// 由 DeleteBranch 在未指定 force 时返回，界面层据此弹出二次确认框。
var ErrBranchNotMerged = errors.New("branch not fully merged")

// 分支：新建、重命名与删除。
//
// 名字的本地体检在界面层（refNameProblem）：宁可漏判不可误判 —— 这里拦下
// 一个 git 本来收得下的名字，用户就没有任何办法绕过去；漏掉一个，结果只是
// git 报错、failure.go 再把话翻出来。

// CreateBranch 从 rev 建一条分支；rev 为空 = 当前 HEAD。
//
// switchTo 为真时用 checkout -b 一步到位：先 branch 再 checkout 是两条
// 命令，中间那一瞬新分支已经存在但还没切过去 —— 期间用户再点一下别的
// （比如检出另一条分支），收尾就没法把话圆回来。一步到位，要么都在要么
// 都不在。
//
// 重名和非法名字 git 的原话分别是 "a branch named 'x' already exists" 和
// "not a valid branch name"，failure.go 里都有 —— 不在这里预判。
func (r *Repo) CreateBranch(ctx context.Context, name, rev string, switchTo bool) error {
	args := []string{"branch"}
	if switchTo {
		args = []string{"checkout", "-b"}
	}
	args = append(args, name)
	if rev != "" {
		args = append(args, rev)
	}
	_, err := run(ctx, r.Root, args...)
	return err
}

// RenameBranch 重命名分支（git branch -m old new）。
//
// 无论 old 是否为当前检出的分支，git 均支持并会自动更新当前 HEAD 指向。
func (r *Repo) RenameBranch(ctx context.Context, old, new string) error {
	_, err := run(ctx, r.Root, "branch", "-m", old, new)
	return err
}

// DeleteBranch 删除本地分支。
//
// force 为 false 时走 git branch -d；若分支上有未合并的提交，git 会返回
// "is not fully merged"，此时转化为 ErrBranchNotMerged 便于上层做二次确认。
// force 为 true 时走 git branch -D 强制删除。
// 尝试删除当前正检出的分支时，git 会返回 "cannot delete branch" 错误。
func (r *Repo) DeleteBranch(ctx context.Context, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(ctx, r.Root, "branch", flag, name)
	if err != nil && !force && strings.Contains(strings.ToLower(err.Error()), "is not fully merged") {
		return ErrBranchNotMerged
	}
	return err
}
