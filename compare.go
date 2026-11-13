package main

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// compareState 记录分支间代码差异对比的状态（Fork 风格）。
type compareState struct {
	on      bool             // 是否处于分支对比模式
	base    string           // 基准分支名（如 main）
	target  string           // 对比分支名（如 feature）
	commits []gitrepo.Commit // 两个分支之间的提交列表
	state   loadState        // 加载状态
	err     string           // 错误信息

	// 弹窗状态
	dialog     bool   // 是否打开对比分支选择弹窗
	pickBase   string // 弹窗中选中的基准分支
	pickTarget string // 弹窗中选中的对比分支

	// modal 是「正在看哪两个分支的差异」那个弹窗自己的一份选中。关掉弹窗也
	// 留着，下次打开还是刚才那两个分支。
	//
	// 它不跟着 tab 快照走（saveActiveTab 不收 compare）：弹窗是压在窗口上的
	// 一层，不是某一个项目标签页的内容。开着弹窗时切去别的项目，回来该看见
	// 同一个弹窗，而不是被换成另一个项目的分支对。
	modalCompare string
	modalTarget  string

	// files 是这两个分支之间的文件，tree 是它们建出来的树，review 是右边
	// 那一栏的 diff。
	//
	// 三份都不和主区共用：主区那三份（a.files / a.tree / a.review）跟着 tab
	// 快照走，弹窗改一下就把用户原来在看哪儿顶掉了 —— 而弹窗是临时的一层
	// （见 compareModalFiles）。
	files  []gitrepo.File
	tree   []*filetree.Node
	review reviewState

	// sel 是弹窗文件栏选中的那个文件的下标，list 是那一栏的滚动位置。
	//
	// 它**不**是 a.fileSel：那是主区改动 tab 的选中，跟着 tab 快照走。弹窗
	// 改一下选中就把主区的状态动了，关上之后用户原来在看哪儿就丢了 —— 而
	// 弹窗是临时的一层（见 compareModalFiles）。
	sel    int
	list   ui.ListState
	closed map[string]bool // 收起过哪些目录（和 a.filesClosed 一个意思）
}

// comparableBranches 返回当前所有可用于对比的引用短名列表（本地分支、远程分支、标签）。
func (a *app) comparableBranches() []string {
	var names []string
	seen := map[string]bool{}

	// 当前检出分支优先放最前
	if a.branch != "" {
		names = append(names, a.branch)
		seen[a.branch] = true
	}

	for _, r := range a.refs {
		if r.Kind == gitrepo.RefLocal || r.Kind == gitrepo.RefRemote || r.Kind == gitrepo.RefTag {
			if !seen[r.Name] {
				seen[r.Name] = true
				names = append(names, r.Name)
			}
		}
	}
	return names
}

// startCompare 开启两个分支的内容差异对比（git diff base..target）。
//
// 只填弹窗自己那一份：a.files / a.allFiles / a.fileSel 这些主区的字段一个都
// 不碰。以前这里要 stash 一份 a.workFiles、把 a.files 换成对比的文件，退出时
// 再还原 —— 那正是「比一下分支，工作区的改动就不见了」的来处。现在对比是弹
// 窗，主区从头到尾没离开过工作区改动。
func (a *app) startCompare(base, target string) {
	if a.repoPath == "" || base == "" || target == "" {
		return
	}
	a.compare.on = true
	a.compare.base = base
	a.compare.target = target
	a.compare.state = loadLoading
	a.compare.err = ""
	a.compare.commits = nil
	a.compare.files = nil
	a.compare.sel = -1
	a.compare.closed = nil

	a.loadCompareDiff()
}

// loadCompareDiff 异步加载当前基准与对比分支的差异及提交列表。
func (a *app) loadCompareDiff() {
	if !a.compare.on || a.repoPath == "" {
		return
	}
	a.compare.state = loadLoading
	a.compare.err = ""

	a.compareSeq++
	seq := a.compareSeq
	base, target := a.compare.base, a.compare.target
	dir := a.repoPath

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, dir)
		var files []gitrepo.File
		var commits []gitrepo.Commit
		if err == nil {
			files, err = repo.BranchDiff(ctx, base, target)
			if err == nil {
				commits, _ = repo.BranchCommits(ctx, base, target, 100)
			}
		}
		a.dispatch(func() {
			if a.compareSeq != seq || !a.compare.on {
				return
			}
			if err != nil {
				a.compare.state = loadFailed
				a.compare.err = err.Error()
				return
			}
			a.compare.state = loadReady
			a.compare.commits = commits
			a.compare.files = files
			if a.compare.sel < 0 && len(files) > 0 {
				a.compare.sel = 0
			}
			a.rebuildCompareTree()
		})
	})
}

// swapCompare 交换基准分支与对比分支方向。
func (a *app) swapCompare() {
	if !a.compare.on {
		return
	}
	a.compare.base, a.compare.target = a.compare.target, a.compare.base
	a.loadCompareDiff()
}

// rebuildCompareTree 按弹窗这一份文件建文件树。
//
// 和 rebuildTree 是同一副骨架（两段分组、根目录行），但读的是 a.compare.files、
// 写的是 a.compare.tree。分段在这边恒为「都不分段」：分支对比的 diff 里没有
// 已暂存 / 未暂存这回事（FilesFromDiff 给的是 SectionNone）。
func (a *app) rebuildCompareTree() {
	base, files := 0, a.compare.files
	roots := wrapRootDir(filetree.BuildAt(files, base, ""), "")
	a.compare.tree = filetree.Filter(roots, "")
}

// syncCompareReview 让弹窗右边的 diff 跟着弹窗里选中的文件走。每帧调一次，
// 重复调用是安全的。
//
// 和 syncReview 是同一套骨架，但对着弹窗自己那一份：文件在 a.compare.files
// 里、读的是 base..target 两个分支、结果落在 a.compare.review。省掉的三件
// 事见 compareModalDiff。
func (a *app) syncCompareReview() {
	if !a.compare.on {
		return
	}
	s := &a.compare.review
	f := a.compareTarget()
	if f == nil {
		*s = reviewState{}
		return
	}
	if s.loading {
		return
	}
	if s.key == fileKey(*f) && s.fp == f.Fingerprint &&
		(s.diff != nil || s.image || s.err != "") {
		return
	}
	a.loadCompareReview(*f)
}

// loadCompareReview 读选中文件在两个分支上的两侧原文，装进弹窗的 DiffView。
//
// 走的是 loadReview 同一条读盘路径（读盘、大小检查、utf8 检查、建缓冲区全
// 在那儿），只是结果落到 a.compare.review，异步回调认领的也是那一份。
func (a *app) loadCompareReview(f gitrepo.File) {
	s := &a.compare.review
	key, path, fp := fileKey(f), pathOf(f), f.Fingerprint
	root := a.repoPath
	oldRev, newRev, diskNew, ok := a.compareRevs(f)
	if !ok {
		return
	}
	oldPath, newPath := f.OldPath, f.NewPath

	if isImagePreview(f) {
		a.loadCompareReviewImage(f)
		return
	}
	if f.Binary {
		*s = reviewState{key: key, fp: fp, path: path}
		s.err = Tf("%s 是二进制文件，对比弹窗里没有可显示的内容",
			"%s is a binary file, so the compare dialog has nothing to show",
			path)
		return
	}

	*s = reviewState{key: key, fp: fp, path: path, loading: true}

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var oldText, newText string
		tooLarge := false
		if err == nil {
			oldText, tooLarge = readReviewSide(ctx, repo, oldRev, oldPath, false)
			if newText, tooLarge = readReviewSide(ctx, repo, newRev, newPath, diskNew); !tooLarge {
				tooLarge = len(oldText) > maxReviewBytes || len(newText) > maxReviewBytes
			}
		}
		a.dispatch(func() {
			if a.compare.review.path != path || a.compare.review.key != key {
				return // 读盘期间又切走了，这份结果作废
			}
			if !a.compare.on {
				return
			}
			s := &a.compare.review
			s.loading = false
			if err != nil {
				s.err = err.Error()
				return
			}
			if tooLarge {
				s.err = Tf("%s 超过 %d MB，对比弹窗不展开它",
					"%s is over %d MB, so the compare dialog leaves it out",
					path, maxReviewBytes>>20)
				return
			}
			if !utf8.ValidString(oldText) || !utf8.ValidString(newText) {
				s.err = Tf("%s 里有二进制内容，对比弹窗画不了",
					"%s has binary content the compare dialog can't show", path)
				return
			}
			a.fillCompareReview(f, oldText, newText)
		})
	})
}

// loadCompareReviewImage 读选中图片在两个分支上的两侧字节，解成位图。
//
// 和 loadReviewImage 同一套读法，只是落点是弹窗那一份 review。
func (a *app) loadCompareReviewImage(f gitrepo.File) {
	s := &a.compare.review
	key, path, fp := fileKey(f), pathOf(f), f.Fingerprint
	root := a.repoPath
	oldRev, newRev, diskNew, ok := a.compareRevs(f)
	if !ok {
		return
	}
	oldPath, newPath := f.OldPath, f.NewPath

	*s = reviewState{key: key, fp: fp, path: path, loading: true, image: true}

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var oldSide, newSide imageSide
		if err == nil {
			oldData, oldReadErr := readReviewImageSide(ctx, repo, oldRev, oldPath, false)
			newData, newReadErr := readReviewImageSide(ctx, repo, newRev, newPath, diskNew)
			oldSide, newSide = decodeImageSide(oldData), decodeImageSide(newData)
			if errors.Is(oldReadErr, errImageTooLarge) {
				oldSide = imageSide{err: oversizedImageMessage()}
			}
			if errors.Is(newReadErr, errImageTooLarge) {
				newSide = imageSide{err: oversizedImageMessage()}
			}
		}
		a.dispatch(func() {
			if a.compare.review.path != path || a.compare.review.key != key {
				return
			}
			if !a.compare.on {
				return
			}
			s := &a.compare.review
			s.loading = false
			if err != nil {
				s.err = err.Error()
				return
			}
			s.imgOld, s.imgOldMeta, s.imgOldErr = oldSide.bitmap, oldSide.meta, oldSide.err
			s.imgNew, s.imgNewMeta, s.imgNewErr = newSide.bitmap, newSide.meta, newSide.err
		})
	})
}

// compareRevs 是弹窗里这两个版本各是什么。
//
// 就是 base..target 这一对：对比比的正是两个分支最新提交的内容。
func (a *app) compareRevs(f gitrepo.File) (oldRev, newRev string, diskNew, ok bool) {
	if !a.compare.on || a.compare.base == "" || a.compare.target == "" {
		return "", "", false, false
	}
	return a.compare.base, a.compare.target, false, true
}

// fillCompareReview 拿两侧原文把弹窗的 DiffView 装起来。
//
// 和 fillReview 同一条装法，省掉弹窗里没有的三件：冲突挑边（两个分支之间
// 不会有冲突标记）、可改 / 可暂存（弹窗不对 index 和工作区动手）、草稿
// （同上）。
func (a *app) fillCompareReview(f gitrepo.File, oldText, newText string) {
	s := &a.compare.review
	lang := editor.LanguageFor(pathOf(f))
	makeBuffer := func(text string) *editor.Buffer {
		b := editor.NewBuffer(text)
		b.SetLanguage(lang)
		return b
	}

	s.editable = false
	s.stageable = false
	s.conflict = false
	s.old = makeBuffer(oldText)
	s.new = makeBuffer(newText)
	s.hunkIdx = -1
	s.diff = &editor.DiffState{
		Old:          s.old,
		New:          s.new,
		Layout:       a.reviewLayout(),
		Inline:       editor.DiffInlineWordAlt,
		Indicators:   editor.DiffBars,
		ContextLines: 3,
		Editable:     false,
		HunkActions:  false,
	}
}

// exitCompare 退出分支对比弹窗。
//
// 不还原任何东西：主区的文件列表从没被换掉过（见 startCompare）。这里只把
// 弹窗自己那一份清掉，选中留着 —— 下次打开还是刚才那两个分支、刚才那个文
// 件。
func (a *app) exitCompare() {
	if !a.compare.on {
		return
	}
	a.compare.on = false
	a.compare.state = loadIdle
	a.compare.commits = nil
	a.compare.review = reviewState{}
}

// openCompareDialog 打开对比分支选择对话框。
func (a *app) openCompareDialog(defaultTarget string) {
	branches := a.comparableBranches()
	a.compare.dialog = true

	// 预设基准分支
	if a.branch != "" {
		a.compare.pickBase = a.branch
	} else if len(branches) > 0 {
		a.compare.pickBase = branches[0]
	}

	// 预设目标分支
	if defaultTarget != "" {
		a.compare.pickTarget = defaultTarget
	} else {
		for _, b := range branches {
			if b != a.compare.pickBase {
				a.compare.pickTarget = b
				break
			}
		}
		if a.compare.pickTarget == "" && len(branches) > 0 {
			a.compare.pickTarget = branches[0]
		}
	}
}

// compareModal 是「正在看哪两个分支的差异」这个弹窗：分支对比开着的时候，
// 换基准、换对比、交换方向、退出，都收在这一层里。
//
// 原来它是一条压在主区顶上的带子（compareBar），和 PR 检视条一个样。问题
// 在它**占的是主区的槽**：两个分支一比，改动 tab 的文件列表 + diff 被整块
// 换成对比的 diff，工作区还有什么没提交当场看不见了 —— 而「比完顺手把改动
// 收进去」正是比完之后最常做的事。而且那条带子自己有 700 的固有宽度，
// 720 的最小窗口里得折成两行才摆得下（见 compareBarWideW 的旧注释）。
//
// 弹窗没有这两个问题：主区一动不动（Esc 之外不接任何全局键，见 modalOn），
// 宽度也不再是窗口的约束 —— 弹窗自己有 640 的下限，装不下就内部滚动。
func (a *app) compareModal(c *ui.Context, pal palette) {
	if !a.compare.on {
		return
	}
	open := true
	specModalSticky(c, pal, &open, func() { a.compareModalBody(c, pal) })
	if !open {
		a.exitCompare()
	}
}

// compareModalW/H 是对比弹窗面板的尺寸区间。
//
// 弹窗里要同时摆下「哪两个分支在比」和这两个分支之间的 diff，所以比其它
// 表单弹窗大：下限和代码编辑器对齐（640×420），上限收到 1000×640 ——
// 再高就盖住半个主区，而主区此刻还是活的（工作区改动还在背后）。
const (
	compareModalMinW = float32(640)
	compareModalMinH = float32(420)
	compareModalMaxW = float32(1000)
	compareModalMaxH = float32(640)
)

func (a *app) compareModalBody(c *ui.Context, pal palette) {
	t := c.Theme()
	branches := a.comparableBranches()
	w, h := c.Size()
	panelW := clamp(w-32, compareModalMinW, compareModalMaxW)
	panelH := clamp(h-32, compareModalMinH, compareModalMaxH)

	ui.Column(c).Width(panelW).Height(panelH).Gap(0).Children(func() {
		// —— 标题条：在比哪两条分支 + 退出 ——
		//
		// 44 而不是别的弹窗的 56：这条上只有一行字，下面选择器那一行紧跟着
		// 也是 44 —— 两条等宽等高并排，中间只隔一根发丝线，比一条 48 一条
		// 44 更像一整块。弹窗总共就那么高，省下的都要给正文（见 compareModalH）。
		ui.Row(c).Height(44).Shrink(0).FillWidth().Padding(0, 14).Gap(8).
			AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).
			Children(func() {
				ui.Box(c).Size(18, 18).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Icon(iconGitCompare, r, pal.inkMuted)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, "分支对比").FontSize(uiFontSize(textHeadline)).
						FontWeight(weightSemibold).TextColor(pal.ink)
					ui.Text(c, "直接比较两个分支最新提交的内容 (git diff base..target)").
						FontSize(uiFontSize(textCaption)).SingleLine().TextColor(pal.inkMuted)
				})
				if xButton(c, pal, "退出对比") {
					a.exitCompare()
				}
			})

		// —— 选择器一行：基准 ⇄ 对比 + 统计 ——
		//
		// 不铺底：这是弹窗面板上的一条，面板本身已经是实色 Surface 4（见
		// opaqueSurfaceOwners 的 specModalOpts），控件静息用 chip、悬停
		// rowHover。写 pal.surface2 会在面板上再抠一块同色的板。
		ui.Row(c).Height(44).Shrink(0).FillWidth().Padding(0, 14).Gap(8).
			AlignItems(ui.Center).Children(func() {
			baseBtn := ui.ButtonBase(c).Height(28).Shrink(1).Padding(0, 8).Radius(radiusMD).
				Background(pal.chip).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Gap(4).
				Cursor(ui.CursorPointer).Tooltip("更换基准分支 (Base)").Label("基准分支")
			if baseBtn.Hovered() {
				baseBtn.Background(pal.rowSelected)
			}
			fadeColors(baseBtn)
			baseBtn.Children(func() {
				ui.Text(c, "基准:").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
				ui.Text(c, a.compare.base).FontSize(uiFontSize(textButton)).SingleLine().MinWidth(0).MaxWidth(180).
					FontWeight(weightMedium).TextColor(pal.ink)
				ui.Text(c, "▾").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			})
			baseBtn.Menu(func(m *ui.Menu) {
				for _, b := range branches {
					name := b
					if m.Item(name).Checked(name == a.compare.base).Chosen() {
						a.startCompare(name, a.compare.target)
					}
				}
			})

			swapBtn := ui.ButtonBase(c).Size(28, 28).Shrink(0).Radius(radiusMD).Center().
				Cursor(ui.CursorPointer).Tooltip("交换基准与对比分支方向").Label("交换分支")
			if swapBtn.Hovered() {
				swapBtn.Background(pal.rowHover)
			}
			fadeColors(swapBtn)
			swapBtn.Children(func() {
				ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Icon(iconSwap, r, pal.inkMuted)
				})
			})
			if swapBtn.Clicked() {
				a.swapCompare()
			}

			targetBtn := ui.ButtonBase(c).Height(28).Shrink(1).Padding(0, 8).Radius(radiusMD).
				Background(pal.chip).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Gap(4).
				Cursor(ui.CursorPointer).Tooltip("更换对比分支 (Compare)").Label("对比分支")
			if targetBtn.Hovered() {
				targetBtn.Background(pal.rowSelected)
			}
			fadeColors(targetBtn)
			targetBtn.Children(func() {
				ui.Text(c, "对比:").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
				ui.Text(c, a.compare.target).FontSize(uiFontSize(textButton)).SingleLine().MinWidth(0).MaxWidth(180).
					FontWeight(weightMedium).TextColor(pal.ink)
				ui.Text(c, "▾").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			})
			targetBtn.Menu(func(m *ui.Menu) {
				for _, b := range branches {
					name := b
					if m.Item(name).Checked(name == a.compare.target).Chosen() {
						a.startCompare(a.compare.base, name)
					}
				}
			})

			ui.Spacer(c)

			switch a.compare.state {
			case loadLoading:
				ui.Spinner(c)
				ui.Text(c, "正在计算改动…").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			case loadFailed:
				ui.Text(c, "对比失败: "+a.compare.err).FontSize(uiFontSize(textCaption)).TextColor(t.Danger)
			default:
				ui.Textf(c, "%d 个提交 · %d 个改动文件", len(a.compare.commits), len(a.allFiles)).
					FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			}
		})

		// —— 正文：这两个分支之间的文件与 diff ——
		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			a.compareModalFiles(c, pal)
		})

		// —— 底栏 ——
		//
		// 也是 44：和标题条/选择器一行对齐。这一条上只有一个「退出对比」，
		// 再高就是把空的地方算成间隔。
		ui.Row(c).Height(44).Shrink(0).FillWidth().Padding(0, 12).Gap(8).
			AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).
			Children(func() {
				ui.Text(c, "退出后回到工作区改动；比的是两个分支，不动工作区").
					FontSize(uiFontSize(textCaption)).TextColor(pal.inkMuted).Grow(1).MinWidth(0)
				if secondaryAction(c, pal, "退出对比", "退出分支对比 (Esc)", false).Clicked() {
					a.exitCompare()
				}
			})
	})
}

// compareModalFiles 是对比弹窗正文：左边文件列表，右边 diff。
//
// 和主区的 detailPane 是同一副骨架（文件栏 + 分隔条 + diff），但**不共用**
// 那一份代码：那边读的是 a.fileSel / a.filesList / a.filePaneWidth 这些跟着
// tab 快照走的字段，弹窗改一下选中就把主区的状态动了 —— 而弹窗是临时的一
// 层，关上之后主区该回到用户原来在看的地方。所以这里用自己的一份 sel 和
// ListState。
func (a *app) compareModalFiles(c *ui.Context, pal palette) {
	t := c.Theme()
	if len(a.compare.files) == 0 {
		ui.Box(c).Fill().Center().Children(func() {
			ui.Column(c).Center().Gap(6).Padding(24).Children(func() {
				headline(c, pal, T("两分支之间无代码差异", "No Code Differences"))
				ui.Textf(c, T("分支 %s 与 %s 内容完全一致。", "Branches %s and %s are identical."),
					a.compare.base, a.compare.target).FontSize(uiFontSize(textBodySM)).TextColor(t.TextMuted)
			})
		})
		return
	}
	if a.compare.sel < 0 {
		a.compare.sel = 0
	}
	if a.compare.sel >= len(a.compare.files) {
		a.compare.sel = len(a.compare.files) - 1
	}

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(compareFilePaneW).Shrink(0).FillHeight().
			BorderWidth(0, 1, 0, 0).BorderColor(pal.hairline).Children(func() {
			// 计数那一行贴着顶：上下各 6（原来 7、且下面还拖着一截空隙），
			// 让第一行文件紧跟着它 —— 用户是从这一行字开始往下看文件的。
			ui.Box(c).Shrink(0).Padding(6, 12).Children(func() {
				ui.Textf(c, "%s", Tf("%d 个文件", "%d files", len(a.compare.files))).FontSize(uiFontSize(textCaption)).
					Font("monospace").FontFeatures("tnum").SingleLine().TextColor(t.TextMuted)
			})
			ui.Box(c).Grow(1).FillWidth().Children(func() {
				rows := filetree.Flatten(a.compare.tree, a.compare.closed, false)
				a.compare.list.Key = func(i int) any { return rows[i].Node.Key }
				ui.List(c, &a.compare.list, len(rows), func(i int) {
					a.compareModalRow(c, pal, rows[i])
				}).Grow(1)
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.compareModalDiff(c, pal)
		})
	})
}

// compareFilePaneW 是对比弹窗里文件栏的宽度。
//
// 比主区那栏窄（300 → 240）：弹窗自己要给 diff 腾地方，而这里只列文件名
// 和增删行数，240 够。
const compareFilePaneW = float32(240)

// compareModalRow 是对比弹窗文件栏里的一行。只画名字和增删行数 —— 这里没有
// 勾选框（弹窗里不对 index 动手，见 hunkStageable）。
//
// 右键菜单照给：弹窗里确实没有批量那一套（那是主区的事），可「这条路径到底
// 叫什么」正是对着两个分支比的时候最常要复制的东西 —— 去终端里 git show 一
// 下、或者贴进 issue。所以这里挂的是同一份 fileMenu / dirMenu，写操作那几条
// 它们自己会收起来（对比出来的文件是 SectionNone，见 fileMenu）。
//
// 目录行能收能展：弹窗的文件栏比主区窄，一个仓库比下来几十上百个文件，收
// 不起来就只能从头滚到尾。收起状态记在 a.compare.closed 里，和主区的
// filesClosed 一个意思、各记各的。
//
// 目录行走 treeRow，和主区的目录行同一个画法（filepane.go 的 treeRowIn）。
// 早先这里自己搭了一个 Box：Box 是**竖排**容器，AlignItems(Center) 在竖排里
// 是**横轴**居中 —— 于是「（根目录）」「src」这些字横在行中间，两头空着。
// 看着难看之外还歪：深一层的目录缩进多 14px，居中的起始位置也跟着挪，一列
// 目录名因此参差不齐。
func (a *app) compareModalRow(c *ui.Context, pal palette, r filetree.Row) {
	t := c.Theme()
	n := r.Node
	if n.FileIdx < 0 || n.FileIdx >= len(a.compare.files) {
		hasKids := n.Dir && len(n.Children) > 0
		open := hasKids && !a.compare.closed[n.Key]
		row, arrow := treeRow(c, t, pal, treeRowOpts{
			Depth: r.Depth, HasKids: hasKids, Open: open,
		}, func() {
			ui.Text(c, n.Label).FontSize(uiFontSize(textBodySM)).SingleLine().
				Grow(1).MinWidth(0).TextColor(t.TextMuted)
		})
		// 目录的右键菜单要一个 rowSpec，而它只读里面的 list（见 dirMenu）——
		// 那一份给的是对比这一份文件，于是批量那几条因为凑不出改动而不出现。
		// tree 传什么都行，这里不碰折叠状态（弹窗自己记在 a.compare.closed）。
		row.ContextMenu(func(m *ui.Menu) {
			a.dirMenu(m, n, rowSpec{list: a.compare.files, tree: treeChanges})
		})
		if arrow.Clicked() || row.Clicked() {
			a.toggleCompareDir(n.Key)
		}
		return
	}
	f := a.compare.files[n.FileIdx]
	sel := n.FileIdx == a.compare.sel
	hasKids := n.Dir && len(n.Children) > 0
	open := hasKids && !a.compare.closed[n.Key]
	row, arrow := treeRow(c, t, pal, treeRowOpts{
		Depth: r.Depth, Selected: sel, HasKids: hasKids, Open: open,
	}, func() {
		ui.Text(c, n.Label).FontSize(uiFontSize(textBodySM)).SingleLine().Grow(1).MinWidth(0).
			TextColor(t.Text)
		if f.AddCount > 0 {
			ui.Textf(c, "+%d", f.AddCount).FontSize(uiFontSize(rowChipText)).Bold().
				Font("monospace").FontFeatures("tnum").TextColor(pal.addBar)
		}
		if f.DelCount > 0 {
			ui.Textf(c, "−%d", f.DelCount).FontSize(uiFontSize(rowChipText)).Bold().
				Font("monospace").FontFeatures("tnum").TextColor(pal.delBar)
		}
		ui.Text(c, string(f.Status.Letter())).FontSize(uiFontSize(rowChipText)).Bold().
			Width(12).TextAlign(ui.Center).TextColor(statusColor(f.Status, pal)).
			Tooltip(f.Status.Label())
	})
	row.ContextMenu(func(m *ui.Menu) { a.fileMenu(m, f) })
	if arrow.Clicked() {
		a.toggleCompareDir(n.Key)
		return
	}
	if row.Clicked() {
		a.compare.sel = n.FileIdx
	}
}

// toggleCompareDir 收起 / 展开弹窗文件栏里的一个目录。
func (a *app) toggleCompareDir(key string) {
	if a.compare.closed == nil {
		a.compare.closed = map[string]bool{}
	}
	if a.compare.closed[key] {
		delete(a.compare.closed, key)
		return
	}
	a.compare.closed[key] = true
}

// compareModalDiff 画弹窗右边那一栏：一个文件的两侧 diff。
//
// 走的是 review.go 那一套（reviewRevs → loadReview → fillReview），不另写一
// 份读盘：对比态的 reviewRevs 给的就是 base/target 两个分支，缓冲区、词法、
// diff 算法全在那儿，重写一遍只会和它对不上。
//
// 所以这里只是把 reviewPane 的身子搬过来，省掉弹窗里没有的几件东西：栏头
// 的整文件暂存（不对 index 动手）、已 review 勾选、hunk 操作条、冲突挑边
// （两个分支之间不会有冲突标记）。
func (a *app) compareModalDiff(c *ui.Context, pal palette) {
	s := &a.compare.review
	switch {
	case s.loading || (s.diff == nil && !s.image && s.err == ""):
		ui.Box(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Column(c).Center().Gap(8).Children(func() {
				ui.Spinner(c).Size(18, 18)
				ui.Text(c, T("正在读取这个文件…", "Loading file…")).FontSize(uiFontSize(textBodySM)).
					TextColor(pal.inkMuted)
			})
		})
		return
	case s.err != "":
		ui.Box(c).Grow(1).FillWidth().Center().Children(func() {
			ui.Column(c).Center().Gap(10).Padding(24).Children(func() {
				headline(c, pal, T("这个文件画不出 diff", "This file's diff can't be shown"))
				ui.Text(c, s.err).Font("monospace").FontSize(uiFontSize(textCaption)).
					SingleLine().TextColor(pal.inkMuted)
			})
		})
		return
	case s.image:
		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			if f := a.compareTarget(); f != nil {
				a.reviewImageBody(c, pal, f)
			}
		})
		return
	}

	// 栏头只留路径和增删数：这个弹窗说的是「这两个分支之间这个文件怎么了」。
	ui.Row(c).Height(34).Shrink(0).FillWidth().Padding(0, 12).Gap(8).AlignItems(ui.Center).
		Background(pal.chip).BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		f := a.compareTarget()
		if f == nil {
			ui.Text(c, T("未选择文件", "No file selected")).FontSize(uiFontSize(textBodySM)).
				TextColor(pal.inkMuted)
			return
		}
		if f.Status != gitrepo.StatusUntracked {
			specChip(c, pal, string(f.Status.Letter()), statusColor(f.Status, pal), 20, textCaption)
		}
		ui.Text(c, displayPath(*f)).Font("monospace").FontSize(uiFontSize(textBodySM)).Shrink(2).
			SingleLine().TextColor(pal.ink)
		ui.Spacer(c)
		if s.diff != nil {
			added, deleted := s.diff.Stats()
			if added > 0 {
				ui.Textf(c, "+%d", added).Font("monospace").FontSize(uiFontSize(textCaption)).Bold().
					FontFeatures("tnum").TextColor(pal.addBar)
			}
			if deleted > 0 {
				ui.Textf(c, "−%d", deleted).Font("monospace").FontSize(uiFontSize(textCaption)).Bold().
					FontFeatures("tnum").TextColor(pal.delBar)
			}
		}

		// 统一视图 / 两栏对照也在这儿：它管的是**右边这一块 diff** 怎么排，
		// 和工具栏上那一对是同一个 a.split、同两个快捷键（⌘U / ⌘S），只是弹窗
		// 盖着的时候伸手去够工具栏那条不现实。
		//
		// 图片文件不放：那玩意儿只有一种看法（reviewImageBody 自己画），摆了
		// 一个点不动的开关在那儿。
		if !s.image {
			mode := 0
			if a.split {
				mode = 1
			}
			toolbarSegmentedIcons(c, pal, &mode, false,
				segmentedIconItem{label: "Unified", tip: Tf("统一视图（单栏） %s", "Unified View %s", K("⌘U")), icon: iconUnified},
				segmentedIconItem{label: "Split", tip: T("左右对照（分栏）", "Split View"), icon: iconSplit},
			)
			if (mode == 1) != a.split {
				a.setSplit(mode == 1)
			}
		}
	})

	s.diff.Layout = a.reviewLayout()
	// 画布不透明：和主区同一条理由（reviewPane 的注释）—— 增删淡染从背景色
	// 现算，背景一半透的话淡染跟着变，文字区没有稳定对比度。
	th := a.codeEditorTheme(c, pal)
	s.diff.Theme = th
	s.diff.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
	// 行高压到 1.3：mygo 的 DiffState.LineHeight 不设就默认 1.5，于是一行
	// 代码占 1.5×12.5 ≈ 19px —— 弹窗里「上下间隔太多」说的主要是这个，代码
	// 行之间空出来的比字本身还高。1.3 时约 16px，字贴得紧但仍然一行一行分得
	// 清（下降部不咬上一行）。主区不动：那边一屏能看到的行少一半也无妨，
	// 而且用户看的是改动，不是行数。
	s.diff.LineHeight = 1.3
	s.diff.TabSize = tabWidth
	s.diff.Annotations = nil
	s.diff.RenderAnnotation = nil
	s.diff.RenderHunkActions = nil
	s.diff.HunkActions = false

	ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		editor.DiffView(c, s.diff).Fill()
	})
}

// compareTarget 是弹窗里当前选中的那个文件。
//
// 和 reviewTarget 是同一个意思，但读的是弹窗自己那份 sel：主区的 fileSel
// 不能被弹窗改动（见 compareModalFiles）。
func (a *app) compareTarget() *gitrepo.File {
	if a.compare.sel < 0 || a.compare.sel >= len(a.compare.files) {
		return nil
	}
	return &a.compare.files[a.compare.sel]
}

// compareDialog 是选择两分支进行对比的模态对话框。
func (a *app) compareDialog(c *ui.Context, pal palette) {
	if !a.compare.dialog {
		return
	}
	branches := a.comparableBranches()

	open := true
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		// 表单对话框：点背景不关（Esc 和「取消」照旧）—— 两个分支挑到一半，
		// 被背景上误触的一下打回默认值。见 specModalSticky。
		if back.Clicked() {
			open = true
		}
		back.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(460).Padding(20).Radius(radiusXL).
			Background(pal.surface4).Border(1, pal.hairline).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.24))
		panel.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Box(c).Size(18, 18).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Icon(iconGitCompare, r, pal.inkMuted)
				})
				ui.Text(c, "对比分支代码差异").FontSize(uiFontSize(textHeadline)).
					FontWeight(weightSemibold).TextColor(pal.ink)
			})

			ui.Box(c).Height(16)

			// 基准分支 (Base)
			ui.Text(c, "基准分支 (Base):").FontSize(uiFontSize(textBodySM)).TextColor(pal.inkMuted)
			ui.Box(c).Height(4)
			baseBtn := ui.ButtonBase(c).Height(30).FillWidth().Padding(0, 10).Radius(radiusMD).
				Background(pal.surface2).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Justify(ui.SpaceBetween).
				Cursor(ui.CursorPointer)
			baseBtn.Children(func() {
				ui.Text(c, a.compare.pickBase).FontSize(uiFontSize(textBody)).TextColor(pal.ink)
				ui.Text(c, "▾").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			})
			baseBtn.Menu(func(m *ui.Menu) {
				for _, b := range branches {
					name := b
					if m.Item(name).Checked(name == a.compare.pickBase).Chosen() {
						a.compare.pickBase = name
					}
				}
			})

			ui.Box(c).Height(12)

			// 快速交换按钮
			ui.Row(c).Justify(ui.Center).Children(func() {
				sw := ui.ButtonBase(c).Size(28, 28).Radius(radiusMD).Center().
					Cursor(ui.CursorPointer).Tooltip("交换两分支").Label("交换基准与对比分支")
				if sw.Hovered() {
					sw.Background(pal.surface2)
				}
				fadeColors(sw)
				sw.Children(func() {
					ui.Box(c).Size(14, 14).Draw(func(p *ui.Painter, r ui.Rect) {
						p.Icon(iconSwap, r, pal.inkMuted)
					})
				})
				if sw.Clicked() {
					a.compare.pickBase, a.compare.pickTarget = a.compare.pickTarget, a.compare.pickBase
				}
			})

			ui.Box(c).Height(12)

			// 对比分支 (Compare)
			ui.Text(c, "对比分支 (Compare):").FontSize(uiFontSize(textBodySM)).TextColor(pal.inkMuted)
			ui.Box(c).Height(4)
			targetBtn := ui.ButtonBase(c).Height(30).FillWidth().Padding(0, 10).Radius(radiusMD).
				Background(pal.surface2).Border(1, pal.hairlineStrong).AlignItems(ui.Center).Justify(ui.SpaceBetween).
				Cursor(ui.CursorPointer)
			targetBtn.Children(func() {
				ui.Text(c, a.compare.pickTarget).FontSize(uiFontSize(textBody)).TextColor(pal.ink)
				ui.Text(c, "▾").FontSize(uiFontSize(textCaption)).TextColor(pal.inkSubtle)
			})
			targetBtn.Menu(func(m *ui.Menu) {
				for _, b := range branches {
					name := b
					if m.Item(name).Checked(name == a.compare.pickTarget).Chosen() {
						a.compare.pickTarget = name
					}
				}
			})

			ui.Box(c).Height(20)

			// 底部按钮
			ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
				if secondaryAction(c, pal, "取消", "", false).Clicked() {
					a.compare.dialog = false
				}
				startBtn := primaryAction(c, pal, "开始对比", "", false)
				if a.compare.pickBase == "" || a.compare.pickTarget == "" || a.compare.pickBase == a.compare.pickTarget {
					startBtn.Disabled(true)
				}
				if startBtn.Clicked() {
					a.compare.dialog = false
					a.startCompare(a.compare.pickBase, a.compare.pickTarget)
				}
			})
		})
	})
	if !open {
		a.compare.dialog = false
	}
}
