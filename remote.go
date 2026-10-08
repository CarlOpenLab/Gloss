package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 远程：抓取、拉取、推送。三条命令本身在 internal/gitrepo/remote.go，这里
// 管的是界面那一半 —— 三颗按钮长什么样、什么时候是灰的、成了之后重读什么。

// doFetch 抓取所有远程的分支和标签。
//
// 成了只重读引用，不走 reload：抓取不动工作区也不动 index，主区那份 diff
// 一个字节都没变，而 reload 会先把主区换成「正在读取仓库…」再读回来 ——
// 为了侧栏几行远程分支把眼前的 diff 闪一下，不值。
func (a *app) doFetch() {
	if !a.canReachRemote() {
		// 按住快捷键能走到这儿，而这时候按钮和菜单都是灰的。不能让它真跑：
		// `git fetch --all` 在没有远程时**静默成功**，弹一句「已抓取远程的
		// 更新」是骗人。灰按钮不该有第二条暗路。
		return
	}
	a.startOp(gitrepo.OpFetch, "正在抓取…", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "已抓取远程的更新", repo.Fetch(ctx)
	}, func(ok bool) {
		if ok {
			a.loadRefs()
		}
	})
}

// doPull 把当前分支的上游拉下来合进当前分支。
//
// 成功**和失败**都整体重读：拉取要动工作区和 index，失败最常见的两种（冲突、
// 被未提交的改动挡住）恰恰是最需要看准眼前状态的时候 —— 冲突文件得带着 !
// 状态出现在列表里，用户才知道下一步去解决谁。
func (a *app) doPull() {
	if !a.canReachRemote() {
		return
	}
	a.startOp(gitrepo.OpPull, "正在拉取…", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "已拉取最新的改动", repo.Pull(ctx)
	}, func(ok bool) {
		a.reloadAfterOp()
	})
}

// doPush 推送当前分支。
//
// branch 在主线程上先取好：后台那次 Push 执行时读的是本地的这份值，而不是
// 回调里再去读 a.branch —— 写闸门保证这期间没有别的写操作，但把「用哪条分支」
// 和「点了哪一颗按钮」对上，比事后解释容易。
//
// 成了也只重读引用：推送不改本地历史，第一次推还会把上游设上（侧栏的远程
// 分支因此多一条），这些都在 refs 里。
func (a *app) doPush() {
	if !a.canReachRemote() {
		return
	}
	branch := a.branch
	a.startOp(gitrepo.OpPush, "正在推送…", func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		return "已推送 " + branch, repo.Push(ctx, branch)
	}, func(ok bool) {
		if ok {
			a.loadRefs()
		}
	})
}

// canReachRemote 说「和远程打交道」这件事现在有没有对象：仓库开着、至少配了
// 一条远程、且没有别的写操作在跑。
//
// 菜单（syncMenu）和工具栏（remoteBar）都问它同一个问题 —— 两处各写一套判断
// 的话，菜单亮着而按钮是灰的，用户不知道该信哪个。快捷键那条路也用它兜底：
// 灰按钮点不动，⇧⌘F 却按得下去。
func (a *app) canReachRemote() bool {
	return a.repoReady() && len(a.remotes) > 0 && a.op == gitrepo.OpNone
}

// remoteBar 画工具栏上那一组远程按钮：抓取、拉取、推送。
//
// 三颗挨着摆（组内间距 2px，和工具栏别处的 10px 拉开差距）：它们是一组动作
// （和远程打交道），中间插进「刷新」这种本地动作会让人以为是一排等价的按钮。
// 只有图标没有字 —— 工具栏已经排了四颗带字的按钮，再加三颗带字的，720px 的
// 窗口宽度下整排都会被收进「»」菜单；而这三颗图标是 Git 客户端里通行的那三个
// （向下的箭头、进托盘、出托盘），配上 tooltip 足够认。
func (a *app) remoteBar(c *ui.Context) {
	// 一条远程都没有的仓库（本地新建的那种）根本不画这一组：三颗永远灰着的
	// 按钮占掉的是「刷新」的地方 —— 720px 的最小宽度下，这三颗正好是把刷新
	// 挤进「»」的那些像素。有没有远程是仓库的固有属性、不是一时的状态，用户
	// 不会隔一会儿就想「现在能推了吗」；真配了远程，刷新一次它们就出来了。
	//
	// 有远程而只是**此刻**不能动（分离 HEAD、另一个操作在飞）才画成灰的 ——
	// 那种灰是暂时的，值得摆在那儿，hover 上还说明原因。
	if len(a.remotes) == 0 {
		return
	}
	ui.Row(c).Gap(2).Shrink(0).Children(func() {
		off := !a.canReachRemote()

		// 「为什么是灰的」得说出来：一颗灰按钮自己弹不出 tooltip（框架不给禁用
		// 元素报 hover），所以由外层盒子代说。顺序也照这个来 —— 最靠前的原因
		// 是最根本的那个。
		common := ""
		switch {
		case !a.repoReady():
			common = "先打开一个仓库"
		case a.op != gitrepo.OpNone:
			common = a.opName + "，等它跑完"
		}
		// 分离 HEAD 只挡拉取和推送：抓取不碰当前分支，照常可以用。
		branchOff := ""
		if a.detached {
			branchOff = "HEAD 是分离的 —— 先新建一条分支，或者检出一条已有的"
		}

		fetchTip := "把远程的分支和标签抓到本地（不动工作区）"
		if common != "" {
			fetchTip = common
		}
		if iconToolButton(c, "抓取", fetchTip, off, drawFetchIcon, 0) {
			a.doFetch()
		}

		pullTip := "把当前分支的上游拉下来合进来"
		if a.behind > 0 {
			pullTip = fmt.Sprintf("把当前分支的上游拉下来合进来（落后 %d 个提交）", a.behind)
		}
		if common != "" {
			pullTip = common
		} else if branchOff != "" {
			pullTip = branchOff
		}
		if iconToolButton(c, "拉取", pullTip, off || a.detached, drawPullIcon, a.behind) {
			a.doPull()
		}

		pushTip := "把当前分支推上去（第一次会自动设好上游）"
		if a.ahead > 0 {
			pushTip = fmt.Sprintf("把当前分支推上去（超前 %d 个提交）", a.ahead)
		}
		if common != "" {
			pushTip = common
		} else if branchOff != "" {
			pushTip = branchOff
		}
		if iconToolButton(c, "推送", pushTip, off || a.detached, drawPushIcon, a.ahead) {
			a.doPush()
		}
	})
}

// iconDraw 是画一颗图标的函数（和 toolButton 收的是同一种）。
type iconDraw = func(p *ui.Painter, r ui.Rect, col ui.Color)

// drawFetchIcon 画「抓取」（Lucide cloud-download）。
func drawFetchIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconFetch, r, col)
}

// drawPullIcon 画「拉取」（Lucide arrow-down-to-line）。
func drawPullIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconPull, r, col)
}

// drawPushIcon 画「推送」（Lucide arrow-up-from-line）。
func drawPushIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	p.Icon(iconPush, r, col)
}

// iconToolButton 是工具栏上一颗带有徽标（badge）展示能力的图标按钮。
// 精确尺寸 28x28 方形，图标 16x16 居中（留白 6px），圆角 6px。
// 当 badge > 0 时，在按钮右上角呈现高对比、带切口环（cutout ring）的精致微胶囊徽标。
func iconToolButton(c *ui.Context, label, tip string, off bool, icon iconDraw, badge int) bool {
	t := c.Theme()
	btn := ui.ButtonBase(c).Size(28, 28).Shrink(0).Radius(t.Radius).
		Center().Cursor(ui.CursorPointer).Label(label).Tooltip(tip).Disabled(off)

	bg := ui.Transparent
	fg := t.TextMuted
	switch {
	case off:
		fg = t.TextMuted.Alpha(0.38)
		btn.Cursor(ui.CursorDefault)
	case btn.Pressed():
		bg = t.SurfacePressed
		fg = t.Text
	case btn.Hovered():
		bg = t.SurfaceHover
		fg = t.Text
	}
	btn.Background(bg)
	fadeColors(btn)

	btn.Children(func() {
		ui.Box(c).Size(16, 16).Shrink(0).
			Draw(func(p *ui.Painter, r ui.Rect) { icon(p, r, fg) })
	})

	if badge > 0 {
		btn.DrawOver(func(p *ui.Painter, r ui.Rect) {
			drawButtonBadge(p, r, badge, bg, off, t)
		})
	}

	return btn.Clicked()
}

// drawButtonBadge 在按钮右上角绘制工艺级微徽标。
// 遵循 Apple HIG / Awwwards 级别视觉精度：
// 1. 采用纯净切口环（cutout ring），在图标与微徽标交界处营造清晰空间层级。
// 2. 动态自适应形态：个位数呈现完美的 13x13 几何正圆；多位数平滑过渡为精炼胶囊。
// 3. 数字采用半粗体高对比度排版，包含针对无衬线数字的 -0.5px 光学垂直基准微调。
func drawButtonBadge(p *ui.Painter, r ui.Rect, count int, btnBg ui.Color, off bool, t *ui.Theme) {
	text := strconv.Itoa(count)
	if count > 99 {
		text = "99+"
	}

	fontSize := float32(9)
	span := ui.Span{Text: text, Size: fontSize, Weight: 600}
	tw, th := p.MeasureText(0, span)

	// 高度 13px，圆角 6.5px。单数正圆 13x13，双数及以上自适应胶囊
	badgeH := float32(13)
	badgeW := badgeH
	if tw+7 > badgeW {
		badgeW = tw + 7
	}

	// 坐标吸附在 28x28 方块右上角
	bx := r.X + r.W - badgeW - 1
	by := r.Y + 1

	// 切口外环（Knockout Cutout）：1.5px 纯净背景层
	cutout := btnBg
	if cutout.A == 0 {
		cutout = t.Background
	}
	cutoutR := ui.Rect{X: bx - 1.5, Y: by - 1.5, W: badgeW + 3, H: badgeH + 3}
	p.Fill(cutoutR, cutout, (badgeH+3)/2)

	// 徽标胶囊
	badgeBg := t.Accent
	badgeFg := t.AccentText
	if off {
		badgeBg = t.Accent.Alpha(0.4)
		badgeFg = t.AccentText.Alpha(0.7)
	}

	badgeR := ui.Rect{X: bx, Y: by, W: badgeW, H: badgeH}
	p.Fill(badgeR, badgeBg, badgeH/2)

	// 光学对齐绘制文字
	tx := bx + (badgeW-tw)/2
	ty := by + (badgeH-th)/2 - 0.5
	span.Color = badgeFg
	p.RichText(tx, ty, 0, span)
}
