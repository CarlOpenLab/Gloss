package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 暂存与提交：文件行左边的勾选框、hunk 头上的暂存按钮、主区底下的提交栏。
//
// 三样控件共用同一条路：startOp（app.go）→ gitrepo 的写操作 → reload。
// 界面自己不拼「点完之后该是什么样」——改完 index，工作区的真相只有 git
// 说了算，自己动手改一份显示状态迟早会和它对不上。

// stageBoxSize 是文件行里那颗勾选框的边长。
//
// 14：树行定死 24 高，勾选框周围要留得出呼吸；和折叠箭头槽（12）挨着摆在
// 同一列上，两个小方块看着才是一套。
const stageBoxSize = 14

// stageState 是勾选框的三种样子。
type stageState uint8

const (
	stageOff      stageState = iota // 未暂存：空框
	stageOn                         // 已暂存：填色 + 勾
	stageConflict                   // 冲突：琥珀色框，点它 = 标记为已解决
)

// stageBox 是文件行左边那颗暂存勾选框。
//
// 不用 ui.Checkbox：那个自带一段文字标签和表单内边距，塞进 24px 的树行里
// 会把行撑高；而且它的勾在别处已经是「已 review」——两件不同的事长成一个
// 样子，用户会以为勾了这头那头也跟着动。这里自己画：方框是「要不要提交」，
// 绿勾是「读没读过」，两种形状一眼分得开。
//
// Label 是固定词、不带文件名（无障碍名字和测试找它的凭据都是它）；想知道
// 是哪一行，Tooltip 里有完整的话。
func stageBox(c *ui.Context, s stageState, lock bool) *ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Size(stageBoxSize, stageBoxSize).Shrink(0).Radius(4).
		Cursor(ui.CursorPointer).Disabled(lock)

	switch s {
	case stageOn:
		btn.Label("取消暂存").Tooltip("把这个文件移出暂存区")
	case stageConflict:
		btn.Label("标记为已解决").Tooltip("冲突已经解决好了？把文件标记为已解决")
	default:
		btn.Label("暂存").Tooltip("把这个文件放进暂存区")
	}

	hovered := btn.Hovered() && !lock
	var border, fill ui.Color
	checked := false
	switch s {
	case stageOn:
		checked = true
		fill, border = t.Accent, t.Accent
		if hovered {
			fill, border = t.AccentHover, t.AccentHover
		}
	case stageConflict:
		border = t.Warning
		if hovered {
			fill = t.Warning.Alpha(0.14)
		}
	default:
		border = t.TextMuted.Alpha(0.4)
		if hovered {
			border = t.TextMuted
			fill = t.Surface
		}
	}
	btn.Background(fill).Border(1.5, border)
	fadeColors(btn)

	if checked {
		btn.Children(func() {
			ui.Box(c).Size(stageBoxSize-6, stageBoxSize-6).Shrink(0).
				Draw(func(p *ui.Painter, r ui.Rect) { drawCheckMark(p, r, t.AccentText) })
		})
	}
	return btn
}

// stageStateOf 说一个文件在该段落里勾选框该是什么样。
func stageStateOf(f gitrepo.File) stageState {
	switch {
	case f.Section == gitrepo.SectionStaged:
		return stageOn
	case f.Status == gitrepo.StatusConflict:
		return stageConflict
	}
	return stageOff
}

// conflictHint 是冲突文件整份暂存时真正发生的事。
//
// 冲突的解法只有一种：把文件改成你要的样子，然后 `git add` 告诉 git
// 「这就算解了」。按钮上的词因此不叫「暂存」——那是别的文件的事。
func stageLabel(f gitrepo.File) string {
	if f.Section == gitrepo.SectionStaged {
		return "取消暂存 " + pathOf(f)
	}
	if f.Status == gitrepo.StatusConflict {
		return "标记为已解决 " + pathOf(f)
	}
	return "暂存 " + pathOf(f)
}

// stagePaths 是整文件暂存/取消暂存要交给 git 的路径。
//
// 重命名得新旧两个都给：只给新路径的话，旧路径在 index 里还挂着「已跟踪」，
// 暂存完是一半重命名一半还没，用户得再点一次（见 gitrepo.StageFile）。
func stagePaths(f gitrepo.File) []string {
	if f.OldPath != "" && f.OldPath != f.NewPath {
		return []string{f.OldPath, f.NewPath}
	}
	return []string{pathOf(f)}
}

// toggleStageFile 把一个文件整份放进 / 撤出 index。
func (a *app) toggleStageFile(f gitrepo.File) {
	paths := stagePaths(f)
	label := stageLabel(f)
	if f.Section == gitrepo.SectionStaged {
		a.startOp(gitrepo.OpUnstage, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			return "", repo.UnstageFile(ctx, paths...)
		}, func(bool) { a.reloadAfterOp() })
		return
	}
	a.startOp(gitrepo.OpStage, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "", repo.StageFile(ctx, paths...)
	}, func(bool) { a.reloadAfterOp() })
}

// hunkStageable 判断一个文件的这一段能不能按块暂存。
//
// 不能的几种各有原因，都落到「整文件操作」这一条路上：
//   - 未跟踪：文件根本不在 index 里，`git diff` 看不见它，重新切不出那段
//     （展示用的 patch 是我们合成的，没有 index 行和 mode 头，拿去 apply
//     会被 core.autocrlf 和 LFS 过滤器咬）。整文件 `git add` 是正解。
//   - 冲突：合并格式的 patch（@@@ 头）git apply 不认，整文件 add = 已解决。
//   - 二进制 / 过大 / 没有 hunk：没有「段」可点。
//   - 重命名 / 复制：patch 的文件头里带着 rename 指令，按块 apply 会把整个
//     重命名一起落进 index —— 按钮上写的是「暂存这一段」，底下动的却是
//     文件的位置，说的和做的不是一回事。
//   - 已暂存的新增，反向：`@@ -0,0 +1,N @@` 反着 apply 会在 index 里留下
//     一个空文件，而不是把条目删掉（整文件 `git reset` 才是对的）。
func (a *app) hunkStageable(f gitrepo.File) bool {
	if len(f.Hunks) == 0 || f.Binary || f.TooLarge {
		return false
	}
	switch f.Section {
	case gitrepo.SectionUnstaged:
		switch f.Status {
		case gitrepo.StatusUntracked, gitrepo.StatusConflict,
			gitrepo.StatusRenamed, gitrepo.StatusCopied:
			return false
		}
		return true
	case gitrepo.SectionStaged:
		switch f.Status {
		case gitrepo.StatusAdded, gitrepo.StatusRenamed, gitrepo.StatusCopied:
			return false
		}
		return true
	}
	return false
}

// toggleStageHunk 暂存 / 取消暂存某一段改动。
//
// 传的是 hunk 的 ID（路径 + 带行号的原文的哈希）而不是下标：执行时会在
// 后台重新跑一条同基线的 diff 再切片，两次之间文件被改过就找不到这一段，
// 那时得到 ErrHunkGone —— 正常竞态，横幅说一声、刷新列表，不重试。
func (a *app) toggleStageHunk(fi, hi int) {
	if fi < 0 || fi >= len(a.files) || hi < 0 || hi >= len(a.files[fi].Hunks) {
		return
	}
	f := a.files[fi]
	if !a.hunkStageable(f) {
		return
	}
	h := f.Hunks[hi]
	path, id := pathOf(f), h.ID
	reverse := f.Section == gitrepo.SectionStaged

	label := "正在暂存这段改动"
	kind := gitrepo.OpStage
	if reverse {
		label = "正在取消暂存这段改动"
		kind = gitrepo.OpUnstage
	}
	a.startOp(kind, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		if reverse {
			return "", repo.UnstageHunk(ctx, path, id)
		}
		return "", repo.StageHunk(ctx, path, id)
	}, func(ok bool) {
		// 失败也要重读：ErrHunkGone 说明界面拿的已经是旧 patch 了。
		a.reloadAfterOp()
	})
}

// stageAll 把所有改动放进 index。
//
// 用 `git add -A` 一条命令，不遍历 a.files：文件列表有未跟踪文件的数量上限
// （maxUntracked），逐条加会漏掉上限之外的那些 —— 用户点的是「全部暂存」，
// 少暂存几个是最说不清的一类 bug。
func (a *app) stageAll() {
	a.startOp(gitrepo.OpStage, "正在暂存全部改动", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "", repo.StageAll(ctx)
	}, func(bool) { a.reloadAfterOp() })
}

// —— 提交栏 ——

// commitInputH 是提交信息输入框的高度；再多一段就内部滚动。
const commitInputH = 58

// commitInputCompactH 是矮窗口里输入框的高度：一行半，够看清在写什么。
const commitInputCompactH = 32

// commitCompactH 是窗口高度低于它时切换成紧凑档。
//
// 640 是量出来的：常规档的提交栏占 100 上下，工具栏 52，再留出文件列表和
// diff 的最低限度 —— 窗口比这还矮时，两行的输入框吃掉的就是 diff 的地盘。
const commitCompactH = 640

// stagedCount / unstagedCount 数的是 allFiles 而不是 files：筛选只是换个
// 视角看同一批改动，不该改变「点提交会提交多少东西」。
func (a *app) stagedCount() int { return a.countSection(gitrepo.SectionStaged) }

func (a *app) unstagedCount() int { return a.countSection(gitrepo.SectionUnstaged) }

func (a *app) countSection(s gitrepo.Section) int {
	n := 0
	for _, f := range a.allFiles {
		if f.Section == s {
			n++
		}
	}
	return n
}

// canCommit 是提交按钮的可用条件：有东西暂存着、消息不是空的、没有别的
// 写操作在飞。
func (a *app) canCommit() bool {
	return a.repoReady() && a.op == gitrepo.OpNone &&
		a.stagedCount() > 0 && strings.TrimSpace(a.commitMsg) != ""
}

// commitBoxOn 决定这一帧要不要画提交栏。
//
// 历史 tab 没有它（历史是只读的）；仓库没打开没有它；工作区干净、消息也
// 空着的时候也没有 —— 底下一句「没有未提交的改动」，再压一条用不了的提交
// 栏，只是把空状态又挤小了一点。消息不为空就画：那份草稿得让人看得见。
func (a *app) commitBoxOn() bool {
	if a.tab != 0 || !a.repoReady() {
		return false
	}
	return len(a.allFiles) > 0 || strings.TrimSpace(a.commitMsg) != ""
}

// commitBox 是主区底下的提交栏：输入框 + 提交按钮 + 一行状态。
//
// 摆在主列底部整宽，而不是塞进 300px 的文件栏里：提交信息可能是一段话，
// 窄栏里三行就满了；而且它是「改动」这一栏的收尾动作，横贯主区才看得出
// 「这些改动写完就去那儿」。
func (a *app) commitBox(c *ui.Context, pal palette) {
	if !a.commitBoxOn() {
		// 提交栏这一帧没画（切去了历史 tab 之类）：输入框不存在了，焦点
		// 无从谈起 —— 顺手把没提交的消息落盘，别等下一次机会。
		if a.commitFocused {
			a.commitFocused = false
			a.saveCommitDraft()
		}
		return
	}
	t := c.Theme()
	_, winH := c.Size()
	// 两档都是同一个 TextArea，只把高度夹紧 —— 不换成单行输入框。
	//
	// 框架的编辑器状态跟着元素身份走，而元素身份由位置决定：同一个位置上
	// 换控件类型，之前那个多行框的布局区域会留给新元素，单行输入框于是顶着
	// 60px 的最小高度不放（ui/editor.go 的 inputHeight 读的正是这份状态）。
	// 换类型看着更「对」，实际上要跟框架的内部状态打架；夹高度没有这个问题，
	// 而两档的差别本来也只是高矮。
	//
	// 空消息时编辑器自己算出来的高度是三行（60 上下），所以上限那一头必须
	// 真的低于它，否则夹了等于没夹。
	inMin, inMax := float32(commitInputH), float32(commitInputH+40)
	if winH < commitCompactH {
		inMin, inMax = commitInputCompactH, commitInputCompactH
	}

	ui.Column(c).Shrink(0).FillWidth().Padding(10, 12).Gap(8).
		Background(t.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Children(func() {
		var in *ui.Element
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			in = ui.TextArea(c, &a.commitMsg).Grow(1).MinWidth(0).
				MinHeight(inMin).MaxHeight(inMax).
				Placeholder("提交信息…").Label("提交信息")
			a.commitButton(c)
		})
		a.commitStatus(c, t)

		// 草稿在「焦点刚离开」的那一帧落盘：每敲一个键存一次的话，store.Save
		// 会把三个文件整体重写一遍，打字都在做无用功；而离开输入框是「这一
		// 段写完了」的自然信号。
		focused := in != nil && in.Focused()
		if a.commitFocused && !focused {
			a.saveCommitDraft()
		}
		a.commitFocused = focused
	})
}

// commitButton 是那颗提交按钮。
//
// Disabled 而不是「点了没反应」：为什么不能提交写在旁边的状态行里，按钮
// 自己灰着就是那句说明的颜色。
func (a *app) commitButton(c *ui.Context) {
	label := "提交"
	if n := a.stagedCount(); n > 0 {
		label = "提交 " + strconv.Itoa(n) + " 个文件"
	}
	btn := ui.PrimaryButton(c, label).Shrink(0).Tooltip("提交已暂存的改动 ⌘⏎").
		Disabled(!a.canCommit())
	if btn.Clicked() {
		a.doCommit()
	}
}

// commitStatus 是提交栏底下那行：暂存了什么、还差什么、正在忙什么。
func (a *app) commitStatus(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
		if a.op == gitrepo.OpCommit {
			ui.Spinner(c)
			ui.Text(c, a.opName).FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			return
		}

		n, m := a.stagedCount(), a.unstagedCount()
		switch {
		case n == 0:
			ui.Text(c, "还没有暂存的文件 —— 勾上文件左边的方框，把要提交的改动放进来。").
				FontSize(11).TextColor(t.TextMuted)
		default:
			ui.Textf(c, "已暂存 %d 个文件", n).FontSize(11).Font("monospace").
				FontFeatures("tnum").TextColor(t.TextMuted)
			if m > 0 {
				ui.Text(c, "·").FontSize(11).TextColor(t.TextMuted)
				ui.Textf(c, "还有 %d 个未暂存，不会进这次提交", m).FontSize(11).
					TextColor(t.TextMuted)
			}
		}
		ui.Spacer(c)
		if a.canCommit() {
			// 不用等宽字体：⌘ 在等宽字体里多半落到后备字体上，垂直度量
			// 和旁边的中文对不齐，看着像掉出了这一行。
			ui.Text(c, "⌘⏎").FontSize(11).TextColor(t.TextMuted).Tooltip("提交")
		}
	})
}

// doCommit 提交 index 里的内容。
//
// 成功后清掉消息和草稿 —— 但那要在「用户没在这期间又打下一段话」的前提下
// 才成立：提交是异步的，消息框一直可编辑，直接清空会吃掉刚敲进去的字。
func (a *app) doCommit() {
	if !a.canCommit() {
		return
	}
	msg := a.commitMsg
	a.startOp(gitrepo.OpCommit, "正在提交…", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		if err := repo.Commit(ctx, msg); err != nil {
			return "", err
		}
		return "已提交 " + repo.ShortHead(ctx), nil
	}, func(ok bool) {
		if ok && a.commitMsg == msg {
			a.commitMsg = ""
			a.saveCommitDraft()
		}
		a.reloadAfterOp()
	})
}
