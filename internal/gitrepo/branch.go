package gitrepo

import (
	"context"
)

// 分支：新建（B1 会在这里补上重命名和删除）。
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
