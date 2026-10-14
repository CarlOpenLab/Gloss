package gitrepo

import (
	"context"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// StageRange 把未暂存段里 DiffView 划出来的这一段改动放进 index。
//
// oldText 是 index 那份原文、newText 是工作区那份 —— 也就是界面展示「未暂存」
// 时用的同两份文本，补丁按它们生成，行区间是 DiffView 的 0 基末尾开区间。
func (r *Repo) StageRange(ctx context.Context, path, oldText, newText string, hr diff.HunkRange) error {
	return r.applyRange(ctx, path, oldText, newText, hr, false)
}

// UnstageRange 把已暂存段里 DiffView 划出来的这一段改动撤出 index。
//
// 方向反过来：oldText 是 HEAD 那份（已暂存段的基线）、newText 是 index 那份；
// 撤销就是把「HEAD → index」的这段改动反向打到 index 上，index 回到 HEAD 的
// 样子，工作区一个字节都不动。
//
// 和 UnstageHunk 不同，这里不需要调用方替它挡「已暂存的新增文件」：新增在
// 补丁里是 /dev/null 旧侧，反向 apply 掉的是 index 里那条记录本身，不会留下
// 一个空文件。
func (r *Repo) UnstageRange(ctx context.Context, path, oldText, newText string, hr diff.HunkRange) error {
	return r.applyRange(ctx, path, oldText, newText, hr, true)
}

// applyRange 是段级暂存/取消暂存的共同实现：把两份原文 + 一段行区间拼成
// 只包含这一段改动的补丁，喂给 `git apply --cached [--reverse]`。
//
// 和 StageHunk 的 applyHunk 走的路子不同是有原因的：那个口子重跑 git diff、
// 再按 hunk ID 切出「git 自己的 -U3 那一大段」；而 DiffView 的段是 mygo 自己
// 的算法（histogram + indent 启发）划的，和 git 的 hunk 不保证一一对应，
// 切出来的会是好几段拼在一起、或者只剩半段。所以这一条改为按行区间现造补丁
// （diff.RangePatch），粒度就是用户点的那一段。
//
// --whitespace=nowarn：用户配了 apply.whitespace=error/fix 时 git 会拒收甚至
// 改写内容，两条都不能要。
// --recount：上下文或行数头对不上时的兜底（git add -p 也是这么干的），只在
// 第一次失败后试一次，不掩盖真正的冲突。
// 绝不用 --3way：它会动工作区（还会留冲突标记），并且能静默按「我们的/他们的」
// 选边；这个函数的契约是只碰 index。
func (r *Repo) applyRange(ctx context.Context, path, oldText, newText string, hr diff.HunkRange, reverse bool) error {
	patch := diff.RangePatch(path, oldText, newText, hr)
	if patch == "" {
		// 区间越界、或者两侧空区间 —— 用户在界面上看到的行号已经对不上手上
		// 这两份文本了，和 applyHunk 找不到 hunk 是同一种竞态，同一种处理。
		return ErrHunkGone
	}
	apply := []string{"apply", "--cached", "--whitespace=nowarn", "-"}
	if reverse {
		apply = append(apply, "--reverse")
	}
	if _, err := runIn(ctx, r.Root, []byte(patch), apply...); err != nil {
		if _, err2 := runIn(ctx, r.Root, []byte(patch), append(apply, "--recount")...); err2 != nil {
			return err
		}
	}
	return nil
}
