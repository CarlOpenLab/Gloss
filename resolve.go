package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 冲突文件的逐个解决：diff 里那张卡片上的三颗按钮、文件行的右键菜单、
// 「在编辑器打开」。
//
// 全部的重点在一句话上：**ours/theirs 是 git 的词，不是用户的词**。
// 同一个 --ours 在合并里是「当前分支」、在变基里是「变基目标」—— 变基是
// 把你的提交往目标上重放，两边正好反过来（实测 git 2.50：变基中
// `checkout --ours` 写进工作区的是变基目标的内容）。照着合并的习惯给按钮
// 写「用我这边」，用户点下去拿到的是别人的代码，而且毫无提示。所以文案
// 按「卡在哪一步」现算，tooltip 里再把 git 的名字和真实引用名对上说全。

// conflictActions 是一个冲突文件此刻能做的动作和它们的说法。
//
// 集中在一处算：diff 卡片、文件行右键菜单用的是同一份 —— 两处各写各的，
// 迟早一处改了另一处没改，而按钮上的词就是用户判断「点下去会发生什么」
// 的全部依据。
type conflictActions struct {
	ours, theirs       string // 按钮上的词
	oursTip, theirsTip string // 悬停说明：含 git 的 --ours/--theirs 和真实引用名
	note               string // 卡片上那句白话：处境 + 怎么挑
}

// conflictActionsFor 按「卡在哪一步」算出按钮文案。
func (a *app) conflictActionsFor(f gitrepo.File) conflictActions {
	p := a.pending
	cur := a.branch
	if cur == "" {
		cur = "当前分支"
	}

	var act conflictActions
	var oursWho, theirsWho string
	switch {
	case p.Kind == gitrepo.PendingRebase:
		onto := a.refNameFor(p.Onto)
		oursWho = "变基目标"
		if onto != "" {
			oursWho += "（" + onto + "）"
		}
		theirsWho = "正在重放的那条提交"
		act.ours, act.theirs = "用变基目标的", "用重放中的"
		act.oursTip = "变基目标上的内容（git 的 --ours）—— 变基里 ours 在目标那边，和合并相反"
		act.theirsTip = "正在重放的提交的内容（git 的 --theirs）—— 也就是你那条提交重放出来的样子"

	case p.Kind == gitrepo.PendingMerge:
		other := a.refNameFor(p.Other)
		oursWho = "当前分支（" + cur + "）"
		theirsWho = "被并入的那边"
		if other != "" {
			theirsWho = "被并入的 " + other
		}
		act.ours, act.theirs = "用当前分支的", "用被并入的"
		act.oursTip = fmt.Sprintf("当前分支 %s 上的内容（git 的 --ours）", cur)
		act.theirsTip = theirsWho + "的内容（git 的 --theirs）"

	case p.Kind == gitrepo.PendingCherryPick:
		other := a.refNameFor(p.Other)
		oursWho = "当前分支（" + cur + "）"
		theirsWho = "被拣选的那条提交"
		if other != "" {
			theirsWho = "被拣选的 " + other
		}
		act.ours, act.theirs = "用当前分支的", "用被拣选的"
		act.oursTip = fmt.Sprintf("当前分支 %s 上的内容（git 的 --ours）", cur)
		act.theirsTip = theirsWho + "的内容（git 的 --theirs）"

	case p.Kind == gitrepo.PendingRevert:
		oursWho = "当前分支（" + cur + "）"
		theirsWho = "回退后的样子"
		act.ours, act.theirs = "用当前分支的", "用回退后的"
		act.oursTip = fmt.Sprintf("当前分支 %s 上的内容（git 的 --ours）", cur)
		act.theirsTip = "回退完成后的样子 —— 被回退提交的父版本（git 的 --theirs）"

	default:
		// 没有可继续的操作却停着冲突（储藏 apply/pop、打补丁撞的）：
		// ours 还是当前分支，「对方」就是带进来的那一份。
		oursWho = "当前分支（" + cur + "）"
		theirsWho = "对方那边"
		act.ours, act.theirs = "用当前分支的", "用对方的"
		act.oursTip = fmt.Sprintf("当前分支 %s 上的内容（git 的 --ours）", cur)
		act.theirsTip = "另一方的那份内容（git 的 --theirs）—— 储藏或补丁带进来的就是它"
	}

	// 哪边把文件删了要说出来：删除-修改冲突里，一颗写着「用对方那边的」
	// 的按钮按下去是**把文件删掉**，不提前说清就是挖坑。
	switch {
	case !f.Stages.Ours() && !f.Stages.Theirs():
		act.note = "两边都没有这个文件的版本了 —— 挑哪边都是接受删除。"
	case !f.Stages.Ours():
		act.note = fmt.Sprintf("%s 删掉了这个文件 —— 点「%s」保持删除；想要那边的内容，点「%s」。",
			oursWho, act.ours, act.theirs)
	case !f.Stages.Theirs():
		act.note = fmt.Sprintf("%s 删掉了这个文件 —— 点「%s」接受删除；点「%s」留下这边的改动。",
			theirsWho, act.theirs, act.ours)
	default:
		act.note = "两边都改了这个文件 —— 挑一边定下来，也可以去编辑器里手工改，改完勾左边的方框标为已解决。"
		if p.Kind == gitrepo.PendingRebase {
			act.note += "注意变基里两边是反的：ours 是变基目标，theirs 是正在重放的提交。"
		}
	}
	return act
}

// conflictH 是冲突卡片的高度：说明最多两行 + 一排按钮。
//
// 两行不是富余：变基那张卡片的白话最长（「…注意变基里两边是反的：ours 是
// 变基目标，theirs 是正在重放的提交。」），单行放不下，被省略号吃掉的正
// 好是那句最要命的解释。文字让它折行，卡片按两行留高。
const conflictH = 88

// conflictRow 是冲突文件的卡片（原来是条「文件有冲突：手动解决后…」的
// 说明行）：说清处境，把三条出路摆在脸上。
//
// 它不画 diff：冲突文件的 patch 是合并格式（diff --cc、@@@ 头），解析器
// 不认，工作区里那份带着 <<<<<<< 的文本也没什么好看的 —— 要看就去编辑器。
func (a *app) conflictRow(c *ui.Context, pal palette, r row) {
	f := a.fileAt(r.file)
	if f == nil {
		return
	}
	t := c.Theme()
	act := a.conflictActionsFor(*f)
	busy := a.op != gitrepo.OpNone

	ui.Column(c).Height(conflictH).Shrink(0).FillWidth().
		Padding(10, codeGap+barW).Gap(8).Justify(ui.Center).Children(func() {
		ui.Text(c, act.note).FontSize(11).TextColor(t.TextMuted)
		// Wrap：最小宽度（720）下三颗按钮一行摆不下，与其让第三颗被
		// 窗沿切掉半个，不如让它整颗折到第二行。
		ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			if conflictButton(c, pal, act.ours, act.oursTip, busy).Clicked() {
				a.resolveFile(*f, gitrepo.SideOurs, act.ours)
			}
			if conflictButton(c, pal, act.theirs, act.theirsTip, busy).Clicked() {
				a.resolveFile(*f, gitrepo.SideTheirs, act.theirs)
			}
			if conflictButton(c, pal, "在编辑器打开", "在这个文件的外部编辑器里打开，手工改冲突", busy).Clicked() {
				a.openInEditor(*f)
			}
		})
	})
}

// conflictButton 是冲突卡片上的一颗按钮。
//
// 只是给 secondaryAction 起个短名字：tip 和 busy 收进实参，那颗按钮自己
// 保证 Tooltip 记在 Disabled 之前（框架的 Tooltip 构建时问一次 Hovered()，
// 而禁用元素不报 hover —— 顺序反了那句解释就永远浮不出来）。
func conflictButton(c *ui.Context, pal palette, label, tip string, busy bool) *ui.Element {
	return secondaryAction(c, pal, label, tip, busy).Shrink(0)
}

// resolveFile 用某一边解决一个冲突文件。
//
// 走 startOp 这唯一的写闸门；失败（包括「这个文件已经没有冲突了」这种
// 竞态）也重读 —— 那句话本身就说明界面拿的是旧状态。
func (a *app) resolveFile(f gitrepo.File, side gitrepo.ConflictSides, label string) {
	path := pathOf(f)
	a.startOp(gitrepo.OpConflict, "正在解决 "+path, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		var err error
		if side == gitrepo.SideOurs {
			err = repo.ResolveOurs(ctx, path)
		} else {
			err = repo.ResolveTheirs(ctx, path)
		}
		return "已按「" + label + "」解决 " + path, err
	}, func(bool) {
		a.reloadAfterOp()
	})
}

// 冲突文件在文件行上的右键菜单在 filemenu.go 里：那几项（挑一边、在编辑器
// 打开、标记为已解决）和别的文件共用一份菜单，只是对冲突文件多摆最前面。

// openInEditor 让这个文件在外部编辑器里打开（冲突文件要手工改时用）。
//
// 打开器是 a.openPath 注入的（main.go 接系统的「打开」）：测试里它是 nil，
// 点它什么都不发生 —— 无头环境里「打开一个文件」本来就无从谈起。真跑在
// goroutine 里而不是当场调用：打开文件可能占住主线程几百毫秒，而主线程
// 每毫秒都在画画。
func (a *app) openInEditor(f gitrepo.File) {
	if a.openPath == nil || a.repoPath == "" {
		return
	}
	open, path := a.openPath, filepath.Join(a.repoPath, pathOf(f))
	a.run(func() { open(path) })
}
