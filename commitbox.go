package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/explain"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/llm"
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
		btn.Label(T("取消暂存", "Unstage")).Tooltip(T("将该文件移出暂存区", "Remove this file from staging area"))
	case stageConflict:
		btn.Label(T("标记为已解决", "Mark as Resolved")).Tooltip(T("将文件冲突标记为已解决", "Mark conflict as resolved"))
	default:
		btn.Label(T("暂存", "Stage")).Tooltip(T("将该文件加入暂存区", "Add this file to staging area"))
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
	if f.OldPath != "" && f.NewPath != "" && f.OldPath != f.NewPath {
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
	if a.compare.on || a.tab != 0 {
		return false
	}
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

// unstageAll 把所有已暂存改动移出 index。
func (a *app) unstageAll() {
	a.startOp(gitrepo.OpUnstage, "正在取消暂存全部改动", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "", repo.UnstageAll(ctx)
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

// canCommit 是提交按钮的可用条件：
// - 普通提交：有东西暂存着、消息不是空的、没有别的写操作在飞；
// - 修改上次提交：有 HEAD、消息不是空的、没有别的写操作在飞（即使暂存区为 0 也允许仅改提交信息）。
func (a *app) canCommit() bool {
	if !a.repoReady() || a.op == gitrepo.OpCommit || a.op == gitrepo.OpAmend || a.aiCommitLoading || a.riskScan.loading || strings.TrimSpace(a.commitMsg) == "" {
		return false
	}
	if a.amend {
		return a.hasHead
	}
	return a.stagedCount() > 0
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

	ui.Column(c).Key("commit-box-wrap").Shrink(0).FillWidth().
		Padding(8, 12, 10, 12).
		Background(t.Surface).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Children(func() {

		var in *ui.Element

		// HeroUI 风格的一体化 Composer 卡片容器
		cardBorder := pal.border
		if a.commitFocused {
			cardBorder = t.Accent.Alpha(0.65)
		}
		ui.Column(c).FillWidth().Radius(10).
			Background(t.Background).
			Border(1.2, cardBorder).
			Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.04)).Children(func() {

			// 顶部：全宽输入框，充足视野
			in = ui.TextArea(c, &a.commitMsg).Grow(1).FillWidth().MinWidth(0).
				MinHeight(inMin).MaxHeight(inMax).
				Border(0, ui.Transparent).Background(ui.Transparent).
				Padding(8, 10).
				Placeholder(T("提交说明（首行简要概述，空行后输入详细背景…）", "Commit summary (first line summary, blank line for details…)")).Label(T("提交信息", "Commit Message"))

			// 底部：操作与状态一体化工具栏
			ui.Row(c).FillWidth().Height(34).Shrink(0).
				Padding(0, 10).AlignItems(ui.Center).Gap(8).
				Background(t.Surface.Alpha(0.5)).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.border.Alpha(0.45)).Children(func() {

				// 左侧：状态指示徽章与 Amend 开关
				a.commitStatus(c, t, pal)

				ui.Spacer(c)

				// 右侧：功能按钮组（预检 -> AI -> 提交 CTA）
				ui.Row(c).Shrink(0).Gap(7).AlignItems(ui.Center).Children(func() {
					a.riskScanButton(c)
					a.aiCommitButton(c)
					a.commitButton(c)
				})
			})
		})

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

// aiCommitButton 是 AI 生成提交信息按钮（HeroUI Sparkles ✨ 样式）。
func (a *app) aiCommitButton(c *ui.Context) {
	t := c.Theme()
	tip := T("✨ AI 生成提交信息 (⌘I)", "✨ AI Generate Commit Message (⌘I)")
	if a.aiCommitLoading {
		tip = T("正在生成提交信息… 点击取消", "Generating commit message… Click to cancel")
	}
	disabled := !a.aiCommitLoading && a.stagedCount() == 0 && !a.amend
	btn := ui.ButtonBase(c).Shrink(0).Height(26).Radius(6).
		Padding(0, 9).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(T("✨ AI 生成提交信息", "✨ AI Commit Message")).Tooltip(tip).
		Disabled(disabled)

	hovered := btn.Hovered() && !disabled
	var bg, border, fg ui.Color
	switch {
	case a.aiCommitLoading:
		bg, border, fg = t.Accent.Alpha(0.16), t.Accent, t.Accent
	case disabled:
		bg, border, fg = ui.Transparent, t.Border.Alpha(0.3), t.TextMuted.Alpha(0.35)
	case hovered:
		bg, border, fg = t.Accent.Alpha(0.16), t.Accent.Alpha(0.55), t.Accent
	default:
		bg, border, fg = t.Accent.Alpha(0.08), t.Accent.Alpha(0.25), t.Accent
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		if a.aiCommitLoading {
			ui.Spinner(c).Size(11, 11)
			ui.Text(c, T("生成中…", "Generating…")).FontSize(11.5).TextColor(fg)
		} else {
			sparkleIcon(c, 11, fg)
			ui.Text(c, T("AI 生成", "AI Commit")).FontSize(11.5).TextColor(fg)
			// HeroUI Kbd 快捷键微型标签
			ui.Box(c).Radius(3).Padding(1, 4).Background(fg.Alpha(0.12)).Children(func() {
				ui.Text(c, "⌘I").FontSize(9.5).TextColor(fg)
			})
		}
	})

	if btn.Clicked() {
		if a.aiCommitLoading {
			a.cancelAICommit()
		} else {
			a.startAICommit()
		}
	}
}

// riskScanButton 是代码变更安全与风险预检按钮（🛡️ 变更预检）。
func (a *app) riskScanButton(c *ui.Context) {
	t := c.Theme()
	tip := T("扫描暂存区代码中的潜在漏洞与敏感凭据", "Audit staged changes for security vulnerabilities and leaks")
	if a.riskScan.loading {
		tip = T("正在进行代码安全审计… 点击取消", "Auditing changes for security risks… Click to cancel")
	}
	disabled := !a.riskScan.loading && (a.stagedCount() == 0 || !a.store.Provider().Ready())
	btn := ui.ButtonBase(c).Shrink(0).Height(26).Radius(6).
		Padding(0, 9).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(T("🛡️ 变更预检", "🛡️ Audit Changes")).Tooltip(tip).
		Disabled(disabled)

	hovered := btn.Hovered() && !disabled
	var bg, border, fg ui.Color
	switch {
	case a.riskScan.loading:
		bg, border, fg = t.Accent.Alpha(0.16), t.Accent, t.Accent
	case disabled:
		bg, border, fg = ui.Transparent, t.Border.Alpha(0.3), t.TextMuted.Alpha(0.35)
	case hovered:
		bg, border, fg = t.SurfaceHover, t.Border, t.Text
	default:
		bg, border, fg = t.Surface, t.Border.Alpha(0.6), t.TextMuted
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		if a.riskScan.loading {
			ui.Spinner(c).Size(11, 11)
			ui.Text(c, T("预检中…", "Auditing…")).FontSize(11.5).TextColor(fg)
		} else {
			shieldIcon(c, 11, fg)
			ui.Text(c, T("变更预检", "Audit Changes")).FontSize(11.5).TextColor(fg)
		}
	})

	if btn.Clicked() {
		if a.riskScan.loading {
			a.cancelRiskScan()
		} else {
			a.startRiskScan()
		}
	}
}

// startAICommit 发起 AI 提交信息生成。
func (a *app) startAICommit() {
	if a.aiCommitLoading {
		a.cancelAICommit()
		return
	}
	if !a.store.Provider().Ready() {
		a.openSettings()
		return
	}
	var staged []diff.FileDiff
	for _, f := range a.allFiles {
		if f.Section == gitrepo.SectionStaged {
			staged = append(staged, f.FileDiff)
		}
	}
	if len(staged) == 0 {
		return
	}

	a.cancelAICommit()

	ctx, cancel := context.WithCancel(context.Background())
	a.aiCommitCancel = cancel
	a.aiCommitLoading = true
	mail := &explainMail{last: time.Now()}
	a.aiCommitMail = mail

	existingMsg := strings.TrimSpace(a.commitMsg)
	a.commitMsg = ""

	system, user := explain.BuildCommitPrompt(staged, existingMsg)
	prov := a.store.Provider()

	onDelta := func(chunk string) {
		if mail.fill(chunk) {
			a.dispatch(func() { a.flushAICommit(mail) })
		}
	}

	a.run(func() {
		text, err := llm.StreamExplanation(ctx, prov, system, user, onDelta)
		a.dispatch(func() {
			if a.aiCommitMail != mail {
				return
			}
			a.flushAICommit(mail)
			a.aiCommitLoading = false
			a.aiCommitCancel = nil
			if err != nil {
				if ctx.Err() == nil {
					a.opErr = &gitrepo.Failure{
						Title:  "生成提交信息失败",
						Hint:   llm.Describe(err).Hint,
						Detail: err.Error(),
					}
				}
				return
			}
			a.commitMsg = cleanCommitMessage(text)
			a.saveCommitDraft()
		})
	})
}

// flushAICommit 将后台积攒的 token 刷新到界面。
func (a *app) flushAICommit(mail *explainMail) {
	if mail != a.aiCommitMail {
		return
	}
	pending := mail.take()
	if pending == "" {
		return
	}
	a.commitMsg += pending
}

// cancelAICommit 取消进行中的 AI 提交信息生成。
func (a *app) cancelAICommit() {
	if a.aiCommitCancel != nil {
		a.aiCommitCancel()
		a.aiCommitCancel = nil
	}
	a.aiCommitLoading = false
	a.aiCommitMail = nil
}

func cleanCommitMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if strings.HasPrefix(msg, "```") {
		lines := strings.Split(msg, "\n")
		var kept []string
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "```") {
				continue
			}
			kept = append(kept, l)
		}
		msg = strings.TrimSpace(strings.Join(kept, "\n"))
	}
	msg = strings.Trim(msg, "`\"'")
	return strings.TrimSpace(msg)
}

// commitButton 是那颗提交按钮（HeroUI Solid Primary 按钮设计）。
func (a *app) commitButton(c *ui.Context) {
	t := c.Theme()
	label := T("提交", "Commit")
	tip := T("提交已暂存的更改 ⌘⏎", "Commit staged changes ⌘⏎")
	if a.amend {
		label = T("修改上次提交", "Amend Last Commit")
		tip = T("追加修改至最近一次提交 (--amend) ⌘⏎", "Amend previous commit (--amend) ⌘⏎")
		if n := a.stagedCount(); n > 0 {
			label = Tf("修改上次提交 (%d 个文件)", "Amend (%d files)", n)
		}
	} else if n := a.stagedCount(); n > 0 {
		label = Tf("提交 %d 个文件", "Commit %d files", n)
	}

	can := a.canCommit()
	btn := ui.ButtonBase(c).Shrink(0).Height(26).Radius(6).
		Padding(0, 10).Gap(5).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Disabled(!can)

	var bg, border, fg ui.Color
	if can {
		btn.Cursor(ui.CursorPointer)
		if btn.Hovered() {
			bg, border, fg = t.AccentHover, t.AccentHover, t.AccentText
		} else {
			bg, border, fg = t.Accent, t.Accent, t.AccentText
		}
		btn.Shadow(0, 2, 8, 0, t.Accent.Alpha(0.32))
	} else {
		bg, border, fg = t.Surface, t.Border.Alpha(0.35), t.TextMuted.Alpha(0.4)
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		drawIcon(c, iconGitCommit, 12, fg)
		ui.Text(c, label).FontSize(11.5).Bold().TextColor(fg)
		if can {
			ui.Box(c).Radius(3).Padding(1, 4).Background(t.AccentText.Alpha(0.2)).Children(func() {
				ui.Text(c, "⌘⏎").FontSize(9.5).TextColor(fg)
			})
		}
	})

	if btn.Clicked() {
		a.doCommit()
	}
}

// commitStatus 是提交卡片左下侧的状态指示徽章与 Amend 选项。
func (a *app) commitStatus(c *ui.Context, t *ui.Theme, pal palette) {
	ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		if a.op == gitrepo.OpCommit || a.op == gitrepo.OpAmend {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, a.opName).FontSize(11.5).TextColor(t.TextMuted)
			return
		}
		if a.aiCommitLoading {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, T("正在通过 AI 生成提交信息…", "Generating commit message with AI…")).FontSize(11.5).TextColor(t.Accent)
			return
		}
		if a.riskScan.loading {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, T("正在进行代码安全审计…", "Auditing changes for security risks…")).FontSize(11.5).TextColor(t.Accent)
			return
		}

		n, m := a.stagedCount(), a.unstagedCount()
		switch {
		case a.amend:
			if n == 0 {
				ui.Text(c, T("未暂存新改动，仅修改上次提交的信息。", "No staged changes, amending message only.")).
					FontSize(11).TextColor(t.TextMuted)
			} else {
				ui.Box(c).Radius(5).Padding(2, 7).Background(pal.ref.Alpha(0.12)).Border(1, pal.ref.Alpha(0.3)).Children(func() {
					ui.Textf(c, T("已暂存 %d 个文件，将合并入最近提交", "%d files staged, will merge into last commit"), n).FontSize(11).
						Font("monospace").FontFeatures("tnum").TextColor(pal.ref)
				})
			}
		case n == 0:
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(t.TextMuted.Alpha(0.4))
				ui.Text(c, T("还没有暂存的文件", "No staged changes — select files above to commit")).
					FontSize(11).TextColor(t.TextMuted)
				if m > 0 && a.op == gitrepo.OpNone {
					if quickStageAllChip(c, t) {
						a.stageAll()
					}
				}
			})
		default:
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				// HeroUI 风格的已暂存状态胶囊 Chip
				ui.Box(c).Radius(5).Padding(2, 7).
					Background(t.Accent.Alpha(0.10)).
					Border(1, t.Accent.Alpha(0.28)).Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
						ui.Textf(c, T("已暂存 %d 个文件", "%d files staged"), n).FontSize(11).Bold().
							Font("monospace").FontFeatures("tnum").TextColor(t.Accent)
					})
				})
				if a.op == gitrepo.OpNone {
					if quickUnstageAllChip(c, t) {
						a.unstageAll()
					}
				}
				if m > 0 {
					ui.Textf(c, T("(%d 个未暂存)", "(%d unstaged)"), m).FontSize(10.5).
						Font("monospace").FontFeatures("tnum").TextColor(t.TextMuted)
					if a.op == gitrepo.OpNone {
						if quickStageRemainingChip(c, t) {
							a.stageAll()
						}
					}
				}
			})
		}

		if a.hasHead {
			ui.Box(c).Size(1, 12).Background(pal.border.Alpha(0.6))
			amendVal := a.amend
			ui.Checkbox(c, &amendVal, T("修改上次提交 (Amend)", "Amend Last Commit")).Label(T("修改上次提交 (Amend)", "Amend Last Commit"))
			if amendVal != a.amend {
				a.toggleAmend(amendVal)
			}
		}
	})
}

// doCommit 提交 index 里的内容（或修改上次提交）。
//
// 成功后清掉消息和草稿 —— 但那要在「用户没在这期间又打下一段话」的前提下
// 才成立：提交是异步的，消息框一直可编辑，直接清空会吃掉刚敲进去的字。
func (a *app) doCommit() {
	if !a.canCommit() {
		return
	}
	msg := a.commitMsg
	if a.amend {
		a.startOp(gitrepo.OpAmend, "正在修改上次提交…", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			if err := repo.CommitAmend(ctx, msg); err != nil {
				return "", err
			}
			return "已修改上次提交 " + repo.ShortHead(ctx), nil
		}, func(ok bool) {
			if ok {
				a.amend = false
				if a.commitMsg == msg {
					a.commitMsg = ""
					a.saveCommitDraft()
				}
			}
			a.reloadAfterOp()
		})
		return
	}
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

// quickStageAllChip 是提交栏空暂存时的「全部暂存」快捷微按钮。
func quickStageAllChip(c *ui.Context, t *ui.Theme) bool {
	btn := ui.ButtonBase(c).Shrink(0).Height(20).Radius(4).
		Padding(0, 7).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(T("全部暂存", "Stage All")).Tooltip(T("一键全部暂存工作区所有改动", "Stage all changes in working tree"))

	hovered := btn.Hovered()
	var bg, border, fg ui.Color
	if hovered {
		bg, border, fg = t.Accent.Alpha(0.18), t.Accent.Alpha(0.55), t.Accent
	} else {
		bg, border, fg = t.Accent.Alpha(0.09), t.Accent.Alpha(0.25), t.Accent
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		ui.Text(c, T("+ 全部暂存", "+ Stage All")).FontSize(10.5).Bold().TextColor(fg)
	})
	return btn.Clicked()
}

// quickUnstageAllChip 是提交栏中的「全部取消」快捷微按钮。
func quickUnstageAllChip(c *ui.Context, t *ui.Theme) bool {
	btn := ui.ButtonBase(c).Shrink(0).Height(20).Radius(4).
		Padding(0, 6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(T("全部取消", "Unstage All")).Tooltip(T("一键取消暂存所有已暂存改动", "Unstage all staged changes"))

	hovered := btn.Hovered()
	var bg, border, fg ui.Color
	if hovered {
		bg, border, fg = t.SurfaceHover, t.Border, t.Text
	} else {
		bg, border, fg = ui.Transparent, t.Border.Alpha(0.4), t.TextMuted
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		ui.Text(c, T("全部取消", "Unstage All")).FontSize(10.5).TextColor(fg)
	})
	return btn.Clicked()
}

// quickStageRemainingChip 是提交栏中的「暂存剩余」快捷微按钮。
func quickStageRemainingChip(c *ui.Context, t *ui.Theme) bool {
	btn := ui.ButtonBase(c).Shrink(0).Height(20).Radius(4).
		Padding(0, 6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(T("暂存剩余", "Stage Remaining")).Tooltip(T("将剩余未暂存改动全部加入暂存区", "Stage remaining changes"))

	hovered := btn.Hovered()
	var bg, border, fg ui.Color
	if hovered {
		bg, border, fg = t.Accent.Alpha(0.16), t.Accent.Alpha(0.45), t.Accent
	} else {
		bg, border, fg = t.Surface, t.Border.Alpha(0.45), t.TextMuted
	}
	btn.Background(bg).Border(1, border)
	fadeColors(btn)

	btn.Children(func() {
		ui.Text(c, T("+ 暂存剩余", "+ Stage Remaining")).FontSize(10.5).TextColor(fg)
	})
	return btn.Clicked()
}
