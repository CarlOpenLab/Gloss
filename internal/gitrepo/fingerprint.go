package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// 这个文件提供「仓库的 git 状态变了没有」的指纹。它不是给用户看的东西，
// 而是给自动刷新用的：后台每两秒算一次，和上一次重读后的基线比较，
// 一旦不一样就整体重读一遍界面（见 app 的 autorefresh.go）。
//
// 不直接跑 git status 之外的一大堆命令，也不去 diff 每次输出 ——
// 变化检测只需要「变了没有」，不需要知道「哪儿变了」。

// StateFingerprint 返回一个能代表仓库当前 git 状态的字符串。
//
// 三条来源，覆盖界面关心的每一块：
//
//   - statusRaw（--porcelain=v2 --branch -z）：工作区 / 已暂存 / 未跟踪 /
//     当前分支 / 上游超前落后 / 分离 HEAD / 冲突 —— 正是「改动」列表
//     和 ahead-behind 徽标显示的那份状态；
//   - git show-ref --head：所有分支、远端、标签、储藏以及 HEAD ——
//     侧栏的引用树和「历史」看的就是这一份（仓库里进新提交、建分支、
//     fetch 出新远端都会让它变）；
//   - pending 标记文件的存在性：MERGE_HEAD、rebase-merge 这些 ——
//     「正卡在合并/变基中间」那条状态条要跟得上。
//
// 单条命令失败就拼进一条 e:<error> 而不是中断：失败说明仓库状态
// 短时间内读不出来，界面上保留旧内容不刷新，总比反复弹错强。
func (r *Repo) StateFingerprint(ctx context.Context) string {
	var b strings.Builder
	if out, err := r.statusRaw(ctx); err == nil {
		b.Write(out)
	} else {
		b.WriteString("e:status ")
		b.WriteString(err.Error())
	}
	b.WriteByte(0)

	if out, err := run(ctx, r.Root, "show-ref", "--head"); err == nil {
		b.Write(out)
	} else {
		b.WriteString("e:show-ref ")
		b.WriteString(err.Error())
	}
	b.WriteByte(0)

	b.WriteString(pendingMarkers(r.GitDir))
	return b.String()
}

// pendingMarkers 把「有合并/变基/拣选/回退卡着」的标记文件名字拼起来。
// 和 pending.go 用的路径保持一致（rebase 优先的顺序不去管 —— 指纹只管
// 变了没有）。走 stat 不跑 git 进程，一次性也就几个系统调用。
func pendingMarkers(gitDir string) string {
	if gitDir == "" {
		return ""
	}
	var b strings.Builder
	for _, name := range []string{
		"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD",
		"rebase-merge", "rebase-apply", "BISECT_LOG",
	} {
		if _, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
			b.WriteString(name)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
