package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 「卡住了」的状态条：仓库正停在合并/变基/拣选/回退/补丁序列的中间，
// 主列顶上摆一条琥珀色的带子，把「继续 / 跳过 / 中止」摆在脸上。
//
// 和失败横幅（opbanner.go）分开伺候：
//
//   - 横幅说的是「刚才那一下出了什么事」，是**消息**，读完自己关掉；
//     状态条说的是「仓库现在是什么状态」，是**状态**，事情没完它不消失。
//   - 颜色也不同：横幅是红的（出错了），这条是琥珀的（停在半路，不是坏）。
//     变基撞冲突是正常流程的一部分，用错误红去吓人，用户会以为仓库坏了。
//
// 还有第三种处境：**没有可继续的步骤，却有一堆冲突文件**（储藏
// apply/pop 撞出来的 —— 它不写 MERGE_HEAD）。这时候一颗按钮都不摆：
// 「继续」点下去 git 会回一句 no rebase in progress，摆一颗按了没用的
// 按钮比不摆更让人困惑。

// pendingBar 画状态条。没开仓库、没卡住、也没有冲突文件时什么都不画。
func (a *app) pendingBar(c *ui.Context) {
	if !a.repoReady() {
		// 打开失败/正在打开时，workFiles 可能还是上一个仓库的 —— 一条
		// 说着旧仓库冲突的状态条摆在错误卡片上面，比不画更糟。
		return
	}
	conflicts := a.conflictCount()
	if !a.pending.Active() && conflicts == 0 {
		return
	}
	t := c.Theme()
	title, hint := a.pendingText(conflicts)
	busy := a.op != gitrepo.OpNone

	ui.Row(c).Shrink(0).FillWidth().Padding(8, 12).Gap(10).
		AlignItems(ui.Start).
		Background(t.Warning.Alpha(0.10)).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Warning.Alpha(0.35)).
		Children(func() {
			warnIcon(c, 14, t.Warning)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(12).Bold().TextColor(t.Text)
				ui.Text(c, hint).FontSize(11).TextColor(t.TextMuted)
			})
			a.pendingButtons(c, conflicts, busy)
		})
}

// pendingButtons 画「继续 / 跳过 / 中止」。顺序照 macOS 的老规矩：肯定的、
// 主要的那个在最右 —— 手指往右下走的默认落点是「继续」，「中止」离得最远。
func (a *app) pendingButtons(c *ui.Context, conflicts int, busy bool) {
	p := a.pending
	if !p.Active() {
		// 没有可继续的步骤（储藏撞的冲突就长这样）：一颗按钮都不摆。
		// 摆一颗按了 git 也不认的按钮，比不摆更让人困惑。
		return
	}

	// 「还差什么所以点不了」要说出来。Tooltip 必须在 Disabled 之前（框架的
	// Tooltip 是构建时判一次 hover，而 Hovered() 对禁用元素报 false ——
	// 先 Disabled 再 Tooltip，那句解释永远不会浮出来）。
	blocker := ""
	switch {
	case busy:
		blocker = a.opName + "，等它跑完"
	case conflicts > 0:
		blocker = fmt.Sprintf("还有 %d 个冲突文件没解决 —— 列表里标 ! 的就是", conflicts)
	}

	ui.Row(c).Gap(6).Shrink(0).Children(func() {
		if ui.Button(c, "中止").Clicked() {
			a.abortOp()
		}
		// merge 没有 --skip：合并是你自己发起的一件事，要么做完要么中止。
		if p.Kind != gitrepo.PendingMerge {
			if ui.Button(c, "跳过").Tooltip(blocker).Disabled(blocker != "").Clicked() {
				a.skipOp()
			}
		}
		if ui.PrimaryButton(c, "继续").Tooltip(blocker).Disabled(blocker != "").Clicked() {
			a.continueOp()
		}
	})
}

// pendingText 是状态条上那两行话。
//
// 标题只说「哪一步、停成什么样」：「变基停在冲突上」。变基的进度和目标
// 归工具栏的分支芯片（「正在变基 topic → main（第 1/2 步）」）—— 两边
// 都写全的话，同一句话会在屏幕上出现两次，隔着三十像素互相复读。
func (a *app) pendingText(conflicts int) (title, hint string) {
	p := a.pending
	if !p.Active() {
		return fmt.Sprintf("有 %d 个冲突文件要解决", conflicts),
			"点开每个文件、在卡片上挑一边（或者手工改完「标记为已解决」），然后照常提交。储藏的改动撞上手头的文件时就是这样 —— 这一步没有「继续」也没有「中止」。"
	}

	noun := pendingNoun(p.Kind)
	switch {
	case p.Kind == gitrepo.PendingAm && conflicts > 0:
		title = "补丁应用停在冲突上"
	case p.Kind == gitrepo.PendingAm:
		title = "补丁还没应用完"
	case conflicts > 0:
		title = noun + "停在冲突上"
	default:
		title = noun + "还没收尾"
	}
	// 补丁的进度只有这儿能说：am 期间分支芯片说的还是当前分支，没有步数。
	if p.Kind == gitrepo.PendingAm && p.Total > 0 {
		title += fmt.Sprintf("（第 %d/%d 封）", p.Step, p.Total)
	}

	if conflicts > 0 {
		return title, fmt.Sprintf("还有 %d 个冲突文件没解决（列表里标 ! 的）：点开文件、在卡片上挑一边（或者手工改完「标记为已解决」），再点「继续」。", conflicts)
	}
	switch p.Kind {
	case gitrepo.PendingMerge:
		hint = "冲突都解决了：点「继续」收尾这次合并；不做了就「中止」，一切回到动手之前。"
	case gitrepo.PendingRebase:
		hint = "这一步解决了：点「继续」接着放后面的提交；这条不要了就「跳过」；整个不做了就「中止变基」。"
	case gitrepo.PendingCherryPick:
		hint = "解决完了：点「继续」把它提交进来；不需要这条提交就「跳过」。"
	case gitrepo.PendingRevert:
		hint = "解决完了：点「继续」生成这条反向提交；不需要就「跳过」。"
	case gitrepo.PendingAm:
		hint = "解决完了：点「继续」应用后面的补丁；这封不要了就「跳过」。"
	}
	return title, hint
}

// rebaseLabel 是变基期间那句「正在变基 topic → main（第 1/2 步）」，
// 分支芯片和状态条共用一句 —— 两处各写各的，迟早一处改了另一处没改。
func (a *app) rebaseLabel() string {
	p := a.pending
	s := "正在变基"
	if p.Branch != "" {
		s += " " + p.Branch
	}
	if onto := a.refNameFor(p.Onto); onto != "" {
		s += " → " + onto
	}
	if p.Total > 0 {
		s += fmt.Sprintf("（第 %d/%d 步）", p.Step, p.Total)
	}
	return s
}

// refNameFor 给一个提交哈希找回它的人名：有本地分支指着它就用分支名，
// 再退到远程分支/标签，都没有就显示短哈希。
//
// 变基目标、被合进来的提交在 git 那边都只有哈希，而「→ main」对用户
// 有用，「→ 3f9a2c1」没有。读不到引用表（刚启动、引用还在读）时退回
// 短哈希 —— 那也好过一片空白。
func (a *app) refNameFor(sha string) string {
	if sha == "" {
		return ""
	}
	// 本地分支优先：变基目标和被合并的通常是本地分支名，远程分支的名字
	// 跟在它后面（origin/main）也在表里，先撞上哪个是遍历顺序说了算 ——
	// 所以分两趟走。
	for _, r := range a.refs {
		if r.Kind == gitrepo.RefLocal && strings.HasPrefix(sha, r.Hash) {
			return r.Name
		}
	}
	for _, r := range a.refs {
		if strings.HasPrefix(sha, r.Hash) {
			return r.Name
		}
	}
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return sha
}

// conflictCount 数一数工作区里还有几个没解决的冲突文件。
//
// 数的是 workFiles（没被筛选词筛过的那份）：筛选只影响显示，冲突解没解决
// 不该跟着筛选词走 —— 搜别的词把冲突文件藏起来，也不能让「继续」变得可点。
func (a *app) conflictCount() int {
	n := 0
	for _, f := range a.workFiles {
		if f.Status == gitrepo.StatusConflict {
			n++
		}
	}
	return n
}

// continueOp / skipOp / abortOp 是状态条上那三颗按钮的去处。
//
// 三个都无条件重读：继续会挪 HEAD、跳过会丢提交、中止会还原工作区，
// 哪一件不重读眼前就都是旧状态；失败时更要重读 —— 「no rebase in
// progress」这类失败恰恰说明界面手里的状态已经过期了。

func (a *app) continueOp() {
	p := a.pending
	if !p.Active() || a.op != gitrepo.OpNone {
		return
	}
	a.startOp(pendingOp(p.Kind), "正在完成"+pendingNoun(p.Kind)+"…",
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return pendingDone(p.Kind), repo.Continue(ctx, p.Kind)
		}, func(ok bool) { a.reloadAfterOp() })
}

func (a *app) skipOp() {
	p := a.pending
	if !p.Active() || a.op != gitrepo.OpNone {
		return
	}
	a.startOp(pendingOp(p.Kind), "正在跳过这一步…",
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已跳过这一步", repo.Skip(ctx, p.Kind)
		}, func(ok bool) { a.reloadAfterOp() })
}

// abortOp 先开确认框。中止会把已经解决的那部分也一起丢掉，一步回到动手
// 之前 —— 值得多问一句。
func (a *app) abortOp() {
	p := a.pending
	if !p.Active() || a.op != gitrepo.OpNone {
		return
	}
	noun := pendingNoun(p.Kind)
	a.askConfirm(confirmState{
		title: "中止这次" + noun + "？",
		body:  "整个撤销：工作区和分支回到动手之前的样子 —— 已经解决的文件也要重来一次。",
		yes:   "中止" + noun,
		onYes: func() {
			a.startOp(pendingOp(p.Kind), "正在中止"+noun+"…",
				func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
					return "已中止" + noun + "，一切回到动手之前", repo.Abort(ctx, p.Kind)
				}, func(ok bool) { a.reloadAfterOp() })
		},
	})
}

// pendingNoun 是这一步在界面上的名字：「中止合并」「正在完成变基…」都用它。
func pendingNoun(k gitrepo.PendingKind) string {
	switch k {
	case gitrepo.PendingMerge:
		return "合并"
	case gitrepo.PendingRebase:
		return "变基"
	case gitrepo.PendingCherryPick:
		return "拣选"
	case gitrepo.PendingRevert:
		return "回退"
	case gitrepo.PendingAm:
		return "补丁"
	}
	return "操作"
}

// pendingDone 是「继续」走通之后那句成功提示。
func pendingDone(k gitrepo.PendingKind) string {
	switch k {
	case gitrepo.PendingMerge:
		return "合并完成"
	case gitrepo.PendingRebase:
		return "变基完成"
	case gitrepo.PendingCherryPick:
		return "拣选完成"
	case gitrepo.PendingRevert:
		return "回退完成"
	case gitrepo.PendingAm:
		return "补丁应用完了"
	}
	return "这一步完成了"
}

// pendingOp 把卡住的种类映射成写操作种类 —— 失败横幅（failure.go）靠它
// 挑兜底说法：「变基没成功」比「这个操作没成功」有用得多。
func pendingOp(k gitrepo.PendingKind) gitrepo.Op {
	switch k {
	case gitrepo.PendingMerge:
		return gitrepo.OpMerge
	case gitrepo.PendingRebase:
		return gitrepo.OpRebase
	case gitrepo.PendingCherryPick:
		return gitrepo.OpCherryPick
	case gitrepo.PendingRevert:
		return gitrepo.OpRevert
	case gitrepo.PendingAm:
		return gitrepo.OpAm
	}
	return gitrepo.OpNone
}
