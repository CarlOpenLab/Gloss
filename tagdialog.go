package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 打标签：历史里某个提交上右键 → 这张表单。
//
// 和检出（checkout.go）分开：那是一次性的动作，点下去就完事；这是一张要填的
// 东西，填错了重来还得把填过的留着。

// tagState 是「新建标签」对话框的状态。
//
// 名字、说明、勾选都存在 app 上而不是控件里：失败时对话框不关，用户改个名字
// 再点一次，说明还在原处 —— 全部重打一遍是对「重名了」这种小事的过度惩罚。
type tagState struct {
	on        bool
	name      string
	msg       string
	annotated bool   // 附注标签：带说明、作者和日期
	rev       string // 打在哪个提交上（完整哈希）；空 = HEAD
	revLabel  string // 那个提交在界面上叫什么（对话框里要写出来）
	focusName bool   // 还欠名称框一次焦点
	// err 是上一次创建失败的原因。留在这里而不是顶上的横幅：对话框还开着，
	// 用户在改的正是出问题的那个字段，把话搬到别处等于让他两头看。
	err gitrepo.Failure
}

// tagMsgH 是标签说明框的高度：四行上下，够写一段话，又不至于把对话框撑成
// 一个编辑器。
const tagMsgH = 76

// openTagDialog 打开打标签的对话框。
func (a *app) openTagDialog(rev, revLabel string) {
	a.tag = tagState{on: true, annotated: true, rev: rev, revLabel: revLabel, focusName: true}
}

// newTagAtHead 是菜单和命令栏里那条「新建标签…」：没有右键指着的提交，
// 打的就是 HEAD。rev 传空串，让 git 自己去解析 HEAD —— 我们手上那份
// 提交列表可能是某条分支的历史，它排第一的那个未必是 HEAD。
func (a *app) newTagAtHead() {
	if !a.repoReady() || a.op != gitrepo.OpNone {
		return
	}
	label := "HEAD"
	if a.branch != "" {
		// 分离 HEAD 时分支名就是短哈希，正好是这时候该显示的东西
		label = a.branch
	}
	a.openTagDialog("", label)
}

// tagDialog 是对话框的入口。
func (a *app) tagDialog(c *ui.Context, pal palette) {
	if !a.tag.on {
		return
	}
	open := true
	// 表单对话框：点背景不关 —— 打到一半的名字不该被误触丢掉（见 specModalSticky）。
	specModalSticky(c, pal, &open, func() { a.tagBody(c, pal) })
	if !open {
		a.tag = tagState{}
	}
}

// tagBody 是表单本体：名称、附注勾选、说明，底下两颗按钮。
func (a *app) tagBody(c *ui.Context, pal palette) {
	t := c.Theme()
	tg := &a.tag

	ui.Column(c).Width(460).Gap(12).Children(func() {
		ui.Text(c, "新建标签").FontSize(textHeadline).FontWeight(weightSemibold)
		// 打在哪儿必须写在脸上：右键是从某一行发起的，但用户的视线可能已经
		// 跟着菜单走了，回来时得能确认自己点的是哪个提交。
		ui.Text(c, "打在 "+tg.revLabel).FontSize(11).SingleLine().TextColor(t.TextMuted)

		ui.Column(c).Gap(3).Children(func() {
			// 小标题写「名称」、输入框的无障碍名字写「标签名称」：两个词不一样
			// 是故意的 —— 屏幕上这个框就在「新建标签」底下，多两个字是废话；
			// 而读屏和测试找的是控件自己，那个名字要能离开上下文站着。
			ui.Text(c, "名称").FontSize(11).TextColor(t.TextMuted)
			in := ui.TextInput(c, &tg.name).Placeholder("v1.0.0").Label("标签名称")
			// 焦点只送一次：每帧都送的话，用户永远跳不到说明框去。
			if tg.focusName {
				in.Focus()
				tg.focusName = false
			}
			// 名字框里回车 = 点「创建标签」。这么小的表单没有第二件非填不可
			// 的事，打完名字顺手敲一下是最自然的收尾。
			if in.Submitted() {
				a.createTag()
			}
		})

		ui.Checkbox(c, &tg.annotated, "附注标签").Label("附注标签")
		ui.Text(c, "带说明、作者和日期；不勾就是轻量标签，只是一个名字。").
			FontSize(11).TextColor(t.TextMuted)

		if tg.annotated {
			// 不加一行「标签说明」的小标题：placeholder 就是那句话，和提交框
			// 一个做法（见 commitbox.go）。标题和输入框的无障碍名字会是同一个
			// 词，摆两个只有测试分得清哪一个是谁。
			ui.TextArea(c, &tg.msg).
				MinHeight(tagMsgH).MaxHeight(tagMsgH).
				Placeholder("标签说明：这个版本有什么变化…").Label("标签说明")
		}

		if msg := tagBlocker(tg); msg != "" {
			ui.Text(c, msg).FontSize(11).TextColor(t.TextMuted)
		}
		failureBox(c, pal, t, tg.err)

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if secondaryAction(c, pal, "取消", "", false).Clicked() {
				a.tag = tagState{}
				return
			}
			btn := primaryAction(c, pal, "创建标签", "在 "+tg.revLabel+" 上打一个标签", tagBlocker(tg) != "" || a.op != gitrepo.OpNone)
			if btn.Clicked() {
				a.createTag()
			}
		})
	})
}

// tagBlocker 是「还差什么才能创建」：非空时按钮灰着，这句话就是那一灰的
// 说明。名字还没开始填的时候一声不吭 —— 一个刚打开的表单不该先挨一句数落，
// 空框加 placeholder 已经把该做什么说清楚了。
func tagBlocker(tg *tagState) string {
	name := strings.TrimSpace(tg.name)
	if name != "" {
		if p := refNameProblem(name); p != "" {
			return p
		}
	}
	if tg.annotated && strings.TrimSpace(tg.msg) == "" {
		return "附注标签要写一句说明 —— 或者取消勾选，打一个轻量标签。"
	}
	return ""
}

// refNameProblem 是标签名/分支名的本地体检，返回一句人话，没问题时返回空串。
// 两处共用一份：git 对这两类名字的规矩本来就是一回事（check-ref-format），
// 各写一份迟早一处改了另一处没改。
//
// 宁可漏判不可误判：这里拦下一个 git 本来收得下的名字，用户就没有任何办法
// 绕过去；漏掉一个，结果只是 git 报错、界面再把话翻出来（failure.go 里
// 「这个名字不能当标签」那一条）。所以只挡一眼看得出的毛病。
func refNameProblem(name string) string {
	switch {
	case name == "":
		// 两个调用方都先判了非空，这里是兜底；措辞不带「标签」二字，
		// 免得哪天从别处调过来，屏幕上冒出一句张冠李戴的话。
		return "先起个名字。"
	case strings.HasPrefix(name, "-"):
		// 不是 git 的规矩，是我们的：名字是当参数递过去的，以 - 开头会被
		// 当成选项解析。
		return "名字不能以 - 开头。"
	case strings.ContainsAny(name, " \t~^:?*[\\"):
		return "名字里不能有空格和 ~ ^ : ? * [ \\ 这些字符。"
	case strings.Contains(name, ".."):
		return "名字里不能有连续的 .."
	case strings.HasPrefix(name, "/"), strings.HasSuffix(name, "/"):
		return "名字不能以 / 开头或结尾。"
	case strings.HasSuffix(name, ".lock"):
		return "名字不能以 .lock 结尾。"
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "名字里有控制字符。"
		}
	}
	return ""
}

// createTag 打标签。
//
// 成功后只重读引用（+ 历史 tab 的提交列表），不走 reload：标签不影响任何
// 一份 diff，把主区重读一遍只会白闪一下。
func (a *app) createTag() {
	tg := &a.tag
	if !tg.on || tagBlocker(tg) != "" || a.op != gitrepo.OpNone {
		return
	}
	name := strings.TrimSpace(tg.name)
	msg := strings.TrimSpace(tg.msg)
	if !tg.annotated {
		// 说明只有附注标签用得上；勾回来还在（msg 没动），所以这里不能反过来
		// 把它清掉。
		msg = ""
	}
	rev := tg.rev
	tg.err = gitrepo.Failure{}

	a.startOp(gitrepo.OpTag, "正在打标签 "+name,
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "已打上标签 " + name, repo.CreateTag(ctx, name, msg, rev)
		}, func(ok bool) {
			if !ok {
				// 失败要留在对话框里。startOp 已经把它放进顶部横幅了，
				// 搬过来再把横幅清掉 —— 一句话说两遍，其中一遍还在背后。
				if a.opErr != nil {
					tg.err = *a.opErr
					a.opErr = nil
				}
				return
			}
			a.tag = tagState{}
			a.loadRefs()
			if a.tab == 1 {
				a.loadCommits()
			}
		})
}
