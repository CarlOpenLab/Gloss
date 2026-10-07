package main

import (
	"context"

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
		if iconToolButton(c, "抓取", fetchTip, off, drawFetchIcon) {
			a.doFetch()
		}

		pullTip := "把当前分支的上游拉下来合进来"
		if common != "" {
			pullTip = common
		} else if branchOff != "" {
			pullTip = branchOff
		}
		if iconToolButton(c, "拉取", pullTip, off || a.detached, drawPullIcon) {
			a.doPull()
		}

		pushTip := "把当前分支推上去（第一次会自动设好上游）"
		if common != "" {
			pushTip = common
		} else if branchOff != "" {
			pushTip = branchOff
		}
		if iconToolButton(c, "推送", pushTip, off || a.detached, drawPushIcon) {
			a.doPush()
		}
	})
}

// iconDraw 是画一颗图标的函数（和 toolButton 收的是同一种）。
type iconDraw = func(p *ui.Painter, r ui.Rect, col ui.Color)

// drawFetchIcon 画「抓取」：一支向下的箭头，落在一条横线上。
//
// 和「拉取」只差底下那笔：抓取是把远程的引用取到本地那条线上（不碰工作区），
// 拉取是落进工作区（一个托盘里）。两条命令本来就只差这一步，图标照着这件事
// 画，比给它们编两个毫不相干的形状好记。
//
// 线画得**通宽**（0.10~0.90）、托盘画得窄（0.24~0.76）：两个图标不这么分
// 就长得太像了 —— 15px 上一条线和一个托盘本来就没差几个像素，宽度是唯一
// 一眼分得出的那笔。
func drawFetchIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	iconArrow(p, r, 0.5, 0.10, 0.62, col)

	var line ui.Path
	x0, y := iconPt(r, 0.10, 0.86)
	x1, _ := iconPt(r, 0.90, 0.86)
	line.MoveTo(x0, y).LineTo(x1, y)
	p.StrokePath(&line, 1.5, col)
}

// drawPullIcon 画「拉取」：向下的箭头落进托盘。
func drawPullIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	iconArrow(p, r, 0.5, 0.08, 0.58, col)
	drawTray(p, r, col)
}

// drawPushIcon 画「推送」：箭头从托盘里出来、向上。
//
// 和拉取是同一套形状的上下翻转 —— 两个动作互为反向，图标也该互为反向。
func drawPushIcon(p *ui.Painter, r ui.Rect, col ui.Color) {
	iconArrow(p, r, 0.5, 0.78, 0.12, col)
	drawTray(p, r, col)
}

// drawTray 画一个托盘：两侧的短边加一条底边，口朝上。
//
// 高度只给到 0.70~0.88：15px 的方盒里，再高的托盘会把箭头挤成一小截，而箭头
// 才是这三个图标里认人的那部分。
func drawTray(p *ui.Painter, r ui.Rect, col ui.Color) {
	pt := func(x, y float32) (float32, float32) { return iconPt(r, x, y) }
	var path ui.Path
	x, y := pt(0.24, 0.70)
	path.MoveTo(x, y)
	x, y = pt(0.24, 0.88)
	path.LineTo(x, y)
	x, y = pt(0.76, 0.88)
	path.LineTo(x, y)
	x, y = pt(0.76, 0.70)
	path.LineTo(x, y)
	p.StrokePath(&path, 1.5, col)
}

// iconArrow 画一支竖直的箭头：从 y0 到 y1，尖端在 y1（y1 在上就是向上的箭头）。
//
// 头是实心三角，和工具栏上另外几颗图标一致：15px 上描边画的箭头只有一两个
// 像素粗，看不出朝向。三角要给得足（半宽 0.23、长 0.30 个方盒）—— 试过
// 0.19/0.26 那种「更秀气」的比例，缩到 15px 就成了一枚细钉子，指向要盯着
// 看才认得出。
func iconArrow(p *ui.Painter, r ui.Rect, x, y0, y1 float32, col ui.Color) {
	dir := float32(1)
	if y1 < y0 {
		dir = -1
	}
	x0, py0 := iconPt(r, x, y0)
	_, py1 := iconPt(r, x, y1)

	var shaft ui.Path
	shaft.MoveTo(x0, py0).LineTo(x0, py1)
	p.StrokePath(&shaft, 1.5, col)

	hw := r.W * 0.23 // 头的一半宽
	hl := r.H * 0.30 // 头的长度
	var head ui.Path
	head.MoveTo(x0, py1)
	head.LineTo(x0-hw, py1-dir*hl)
	head.LineTo(x0+hw, py1-dir*hl)
	head.Close()
	p.FillPath(&head, col)
}

// iconToolButton 是工具栏上一颗只有图标的按钮。
//
// 和 toolButton 分开而不是加个开关：带字那颗的 Padding 和 Gap 是按「图标+
// 文字」定的，图标单摆时同样的内边距会撑出一颗偏宽的按钮。
//
// Label 是无障碍名字，也是测试找它的凭据（屏幕上没有字可找）；Tooltip 给
// 鼠标用户 —— 灰掉的按钮尤其靠它：三颗按钮都不带字，一个灰方块不说自己
// 为什么灰，就只剩猜了。
//
// Tooltip 必须写在 Disabled 前头：框架的 Tooltip 是**构建时**判一次 hover
// （ui/widgets.go），而 Hovered() 对禁用元素报 false —— 先 Disabled 再
// Tooltip，那句解释永远不会浮出来。这个次序没有别的东西守着，钉它的是
// TestDisabledRemoteButtonExplainsItself。
func iconToolButton(c *ui.Context, label, tip string, off bool, icon iconDraw) bool {
	btn := ui.Button(c, "").Label(label).Tooltip(tip).Padding(0, 8).Disabled(off)
	fadeColors(btn)
	btn.Children(func() {
		col := c.Theme().Text
		ui.Box(c).Size(toolIconSize, toolIconSize).Shrink(0).
			Draw(func(p *ui.Painter, r ui.Rect) { icon(p, r, col) })
	})
	return btn.Clicked()
}
