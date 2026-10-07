package gitrepo

import (
	"context"
)

// 提交上的几件事：拣选、回退、重置、检出这个提交。
//
// 和别的写口子一个规矩：只发命令、不做界面判断（哪一条菜单该灰、
// 硬重置前要问什么，都在上层）。失败原话由 failure.go 翻成人话。
//
// 卡在合并/变基中间时这些命令 git 大多会拒绝（unmerged files / MERGE_HEAD
// exists），而 reset --hard 例外 —— 它会**静默地把没收尾的那一步连同冲突
// 一起抹掉**（实测：merge 停住时 reset --hard，MERGE_HEAD 直接消失）。
// 所以「有没有卡在半路」这道闸由界面把：菜单项直接灰掉（见 commitMenu）。

// CherryPick 把某个提交重放到当前分支上。
//
// 撞冲突时 git 会停在半路（CHERRY_PICK_HEAD + 未合并条目），返回 error；
// 界面重读之后自然会看到状态条和「继续/跳过/中止」—— 这条路和合并共用
// 同一套收尾。
func (r *Repo) CherryPick(ctx context.Context, hash string) error {
	_, err := run(ctx, r.Root, "cherry-pick", hash)
	return err
}

// Revert 生成一条反向提交（把某个提交的改动抵消掉）。
//
// --no-edit 明写：不带它 git 会开编辑器让用户确认那条「Revert "…"」消息，
// GUI 子进程没有终端（gitEnv 里的 GIT_EDITOR=true 是兜底，这里写出来更
// 直白）。合并提交不给 -m 会被 git 拒收（is a merge but no -m option was
// given），那句话 failure.go 已经翻好了。
func (r *Repo) Revert(ctx context.Context, hash string) error {
	_, err := run(ctx, r.Root, "revert", "--no-edit", hash)
	return err
}

// ResetMode 是重置的三档 —— 区别只在「改动丢到哪一层」。
type ResetMode uint8

const (
	// ResetSoft：只挪分支的指向，改动全留在暂存区。
	ResetSoft ResetMode = iota
	// ResetMixed：挪指向 + 清暂存区，改动留在工作区（git 的默认档）。
	ResetMixed
	// ResetHard：暂存区和工作区一起回到那个提交，没提交的改动**丢掉**。
	ResetHard
)

// flag 是这一档对应的 git 参数。
func (m ResetMode) flag() string {
	switch m {
	case ResetSoft:
		return "--soft"
	case ResetHard:
		return "--hard"
	}
	return "--mixed"
}

// ResetTo 把当前分支挪到某个提交上。
//
// 只动当前分支：git reset 本来就是这个语义，界面上的文案也跟着说
// 「重置 main 到 …」，别让它读成「把那个提交删掉」。
func (r *Repo) ResetTo(ctx context.Context, hash string, mode ResetMode) error {
	_, err := run(ctx, r.Root, "reset", mode.flag(), hash)
	return err
}

// CheckoutDetach 检出某个提交，HEAD 分离。
//
// --detach 明写（和 CheckoutTag 一个道理）：分离是主动要的，别让 git 先
// 念一段它自己的建议。工作区有挡道的改动时 git 会拒收（would be
// overwritten by checkout），原话 failure.go 翻成「有未提交的改动挡着」。
func (r *Repo) CheckoutDetach(ctx context.Context, hash string) error {
	_, err := run(ctx, r.Root, "checkout", "--detach", hash)
	return err
}
