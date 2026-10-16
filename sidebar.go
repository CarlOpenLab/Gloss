package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/filetree"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

const (
	sidebarWidth    = 280
	sidebarDefaultW = float32(sidebarWidth)
	sidebarMinW     = float32(180)
	sidebarMaxW     = float32(600)
)

// sidebar 是左侧栏：工作区导航（改动/历史）+ 分支/远程/标签/储藏树。
//
// 参照 Fork 布局规范，侧栏顶部承载工作区导航。
func (a *app) sidebar(c *ui.Context, pal palette) {
	t := c.Theme()
	sw := a.sidebarWidth(c)

	// 侧栏底色是**不透明**的。
	//
	// 这里原来铺的是 pal.sideBg.Alpha(0.55)：窗口开了 VibrancySidebar，想让
	// 系统毛玻璃透上来。代价是侧栏的明度交给了壁纸 —— 侧栏和画布本来就只差
	// 7~8 个灰阶（#0f1011 对 #010102、#efeff0 对 #ffffff），乘 0.55 之后只剩
	// 4 个，深色下和近黑画布、浅色下和白画布都糊成一片，换张壁纸又是另一副样子。
	// Linear 的侧栏是一块实色的面（深色 #0f1011，浅色 #efeff0），层次交给明度
	// 阶梯和 1px 发丝线，不交给半透明。
	ui.Column(c).Width(sw).Shrink(0).
		Background(pal.sideBg).
		Children(func() {
			// 侧栏顶部不再有工作区导航。改动 / 历史 已经并进工具栏那段
			// 「改动 / 历史 / 终端 / 编辑器」的图标切换（见 app.toolbar）：
			// 四个「现在看哪一块」的选择本该在一处，而侧栏这一栏本来就是
			// 分支 / 远程 / 标签 / 储藏树，多出来的两行导航只是把它往下压。
			if a.repoReady() {
				ui.Box(c).Key("ref-filter-wrap").Padding(8, 8, 6, 8).Shrink(0).Children(func() {
					a.refFilterInput(c, pal)
				})
			}
			ui.Box(c).Key("sidebar-refs-wrap").Grow(1).FillWidth().Children(func() {
				a.refsView(c, pal)
			})

			// 侧栏右边缘拖动手柄：鼠标悬停显示双向箭头，可拖拽实时调节侧栏宽度，双击复位默认
			grip := float32(6)
			resizer := ui.Box(c).Absolute().Top(0).Bottom(0).Right(0).Width(grip).
				Cursor(ui.CursorResizeEW).Role(ui.RoleNone)
			if resizer.DoubleClicked() {
				a.resetSidebarWidth()
			}
			if dx, _, held := resizer.Dragged(); held && dx != 0 {
				a.setSidebarWidth(c, sw+dx)
				a.sidebarDragging = true
			} else if a.sidebarDragging && !held {
				a.sidebarDragging = false
				a.saveViewSettings()
			}
			resizer.Draw(func(p *ui.Painter, r ui.Rect) {
				if resizer.Hovered() || resizer.Pressed() {
					p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, t.Accent, 0)
				}
			})
		})
}

// refFilterInput 是侧栏的筛选框。
//
// 名字写成「筛选分支」而不是「筛选」：主区下半还有一个筛选框（筛文件），
// 两个框名字一样的话，测试和无障碍工具都分不出找的是哪一个。
func (a *app) refFilterInput(c *ui.Context, pal palette) {
	searchField(c, pal, &a.refFilter, T("筛选分支", "Filter branches"))
}

func (a *app) refsView(c *ui.Context, pal palette) {
	t := c.Theme()

	switch a.refsState {
	case loadIdle:
		// 空白不等于「没事发生」：说明这个位置以后会有什么，比留一片
		// 灰底好 —— 用户第一次打开时看到的就是这一屏。
		emptyNote(c, t, T("打开仓库后，此处将展示分支、远程与标签。", "Branches, remotes, and tags will appear here once a repository is opened."))
		return
	case loadLoading:
		if len(a.refs) == 0 {
			emptyNote(c, t, T("正在读取分支列表…", "Loading branches…"))
			return
		}
	case loadFailed:
		if len(a.refs) == 0 {
			emptyNote(c, t, T("无法读取分支列表", "Failed to load branches"))
			return
		}
	}

	if a.refFilter != a.refsFilter {
		a.rebuildRefTree()
	}
	if len(a.refTree) == 0 {
		emptyNote(c, t, T("无匹配分支", "No matching branches"))
		return
	}

	// 筛选态一律展开：搜出来的分支藏在收起的分组里，等于没搜到。
	rows := filetree.Flatten(a.refTree, a.refsClosed, a.refFilter != "")
	a.refsList.Key = func(i int) any { return rows[i].Node.Key }
	ui.List(c, &a.refsList, len(rows), func(i int) {
		a.refRow(c, pal, rows[i])
	}).Grow(1)
}

// refRow 画 refs 树的一行。
func (a *app) refRow(c *ui.Context, pal palette, r filetree.Row) {
	t := c.Theme()
	n := r.Node

	if n.Dir {
		a.refGroupRow(c, t, pal, r)
		return
	}

	ref, ok := a.refByFull[strings.TrimPrefix(n.Key, refLeaf)]
	if !ok {
		// 「所有分支」不是一条引用：没有 Full 可以查，也不用查 —— 它要的
		// 是所有引用合起来的那份历史。选中态跟着 histAll 走，这样侧栏说得出
		// 「现在看的是全部」。
		if n.Key == refAll {
			row, _ := treeRow(c, t, pal, treeRowOpts{Depth: r.Depth, Selected: a.histAll}, func() {
				ui.Text(c, n.Label).FontSize(12).SingleLine().
					Grow(1).MinWidth(0).TextColor(pal.inkMuted)
			})
			if row.Clicked() {
				a.selectAllRefs()
			}
			return
		}
		ui.Text(c, n.Label).FontSize(12).SingleLine()
		return
	}

	// 「当前检出的分支」那个圆点画在折叠箭头槽里（见 treeRowOpts.Dot）
	opts := treeRowOpts{Depth: r.Depth, Selected: n.Key == a.refSel}
	if ref.Head {
		c := pal.laneColor(0)
		opts.Dot = &c
	}

	row, _ := treeRow(c, t, pal, opts, func() {
		if ref.Kind == gitrepo.RefStash {
			// 储藏的名字（stash@{0}）人人都一样，认出它是哪一条全靠信息；
			// 两个都画，名字压暗当索引，信息才是给人读的。
			ui.Text(c, ref.Name).FontSize(11).Font("monospace").Shrink(0).
				TextColor(t.TextMuted)
			ui.Text(c, ref.Subject).FontSize(12).SingleLine().
				Grow(1).MinWidth(0).TextColor(t.Text)
			return
		}

		// 常规分支名用 Ink Muted，只有当前检出的那条提到 Ink（§2.3）：
		// 一列分支读的是「有哪几条」，「我在哪条」才需要更亮的那一档。
		txt := ui.Text(c, n.Label).FontSize(12).SingleLine().
			Grow(1).MinWidth(0).TextColor(pal.inkMuted)
		if ref.Head {
			txt.FontWeight(weightSemibold).TextColor(pal.ink)
			// HEAD 芯片走 §2.5 的配方，和提交行里那些引用芯片同一副面孔。
			// 用 Ink 而不是 laneColor(0)：主干现在是钢青蓝，跟下去就和本地
			// 分支芯片那颗 accentBlue 点同色，两种引用分不开（见 badgeDot）。
			specChip(c, pal, "HEAD", pal.ink, rowChipH, rowChipText)
		}
		// 有上游的分支把「和远程差几个提交」摆在名字后面。
		//
		// 本地 / 远程两棵树分开列，看起来是两套互不相干的东西；把这几个数
		// 放在本地分支上，「main 和 origin/main 是同一条线、现在差俩提交」
		// 就不用点两次才看出来了。上游已经被远程删掉也在这儿说一声。
		if track := trackLabel(*ref); track != "" {
			// 上游没了用危险色：那是一件得处理的事（推上去会新建一条远程
			// 分支），压暗成和其它计数一样等于「看一眼就过去」。
			trackColor := t.TextMuted
			if ref.UpstreamGone {
				trackColor = pal.danger
			}
			ui.Text(c, track).FontSize(10).Font("monospace").FontFeatures("tnum").
				Shrink(0).TextColor(trackColor)
		}
	})

	// 右键菜单按引用的种类给出可用操作；双击 = 检出（仅分支和标签支持）。
	row.ContextMenu(func(m *ui.Menu) { a.refMenu(m, *ref) })

	// 双击要问在单击之前：这两下是各自独立的（第二下按下去时两个都是真），
	// 反过来写的话双击会先切一次历史再检出，屏幕上闪一下完全无关的东西。
	if row.DoubleClicked() && checkoutable(*ref) {
		a.refDoubleClicked(n.Key)
		a.checkoutRef(*ref)
		return
	}
	if row.Clicked() && !a.swallowTrailingClick(n.Key) {
		a.selectRef(n.Key, *ref)
	}
}

// refDoubleClicked 记下「这一行刚被双击过」，见 swallowTrailingClick。
func (a *app) refDoubleClicked(key string) {
	a.dblKey, a.dblAt = key, time.Now()
}

// swallowTrailingClick 吃掉双击之后紧跟的那一次单击，返回 true。
//
// 一次双击是两次完整的按下抬起，而每次抬起都会让框架渲染一帧（真实鼠标也
// 是这样，按键之间隔着好几帧）：第一下抬起已经算了一次单击，第二下抬起又
// 算一次。后面这一次要是不吃掉，刚双击检出完，它就把界面又切去历史 tab ——
// 用户看到的是「双击了，人却去了别的地方」。
//
// 时间窗用框架自己认双击的那 500ms（ui/input.go 的 lastPress）：同一个窗口
// 里、同一行上的紧跟单击就是双击的尾巴，再晚就是用户真的又点了一下。
func (a *app) swallowTrailingClick(key string) bool {
	if a.dblKey == "" || a.dblKey != key || time.Since(a.dblAt) > 500*time.Millisecond {
		return false
	}
	a.dblKey = ""
	return true
}

// refGroupRow 画一个分组行（本地分支 / 远程 / 标签 / 储藏）。整行可点，
// 点哪儿都是收起或展开 —— 只有一个 12px 的小三角能点，用起来很别扭。
func (a *app) refGroupRow(c *ui.Context, t *ui.Theme, pal palette, r filetree.Row) {
	open := len(r.Node.Children) > 0 && !a.refsClosed[r.Node.Key]

	// 分组行不参与选中：它不是一个可以「去看历史」的东西
	row, arrow := treeRow(c, t, pal, treeRowOpts{Depth: r.Depth, HasKids: true, Open: open}, func() {
		ui.Text(c, r.Node.Label).FontSize(textCaption).FontWeight(weightSemibold).
			SingleLine().TextColor(pal.inkSubtle)
	})

	if arrow.Clicked() || row.Clicked() {
		a.toggleRefGroup(r.Node.Key, open)
	}
}

// toggleRefGroup 收起/展开一个分组，并把状态记下来。
func (a *app) toggleRefGroup(key string, open bool) {
	if a.refsClosed == nil {
		a.refsClosed = map[string]bool{}
	}
	if open {
		a.refsClosed[key] = true
		return
	}
	delete(a.refsClosed, key)
}

// selectRef 是点了一条引用：把历史切到它上面。
//
// commitSel 必须一起清掉。不清的话，从一条长历史切到短历史时旧下标往往
// 仍然合法，于是「选中的提交」悄悄变成了另一个 —— 界面上没有任何异常，
// 只是显示的是别处的 diff。
func (a *app) selectRef(key string, ref gitrepo.Ref) {
	a.refSel = key

	if ref.Kind == gitrepo.RefStash {
		a.selectStash(ref)
		return
	}

	a.histAll = false
	// 从「所有分支」点回一条具体分支时也要重读：那时候 histRev 是空串，
	// 屏幕上却是全部引用那份历史。
	if a.histRev != ref.Full {
		a.histRev = ref.Full
		a.commitSel = -1
		a.commits = nil
		a.histFiles = nil
		a.histListID = ""
		a.stashRev = "" // 换看分支了，下半区不再是储藏
		a.histDiff = loadIdle
	}
	a.enterHistory()
	a.loadCommits()
}

// enterHistory 把界面带到历史那一栏。
//
// 从终端 / 编辑器视图里点分支、点储藏，光改 a.tab 是不够的：主区画哪一栏归
// viewMode 管，tab 只在 Git 面板里活着。不先把视图切回去，提交列表读得再对
// 也是画在别人那一栏上 —— 用户看到的就是「点了没反应」。
func (a *app) enterHistory() {
	if a.viewMode != viewModeGit {
		a.setViewMode(viewModeGit)
	}
	a.tab = 1
}

// selectAllRefs 是点了「所有分支」：提交列表换成所有引用合起来的历史。
//
// histAll 和 histRev 互斥：开着 histAll 时 histRev 是空串，loadCommits 因此
// 走 LogAll（git log --all）而不是某一条 revision。
func (a *app) selectAllRefs() {
	a.histAll = true
	a.histRev = ""
	a.refSel = "" // 「所有分支」那一行不是引用，选中态由 histAll 说
	a.commitSel = -1
	a.commits = nil
	a.histFiles = nil
	a.histListID = ""
	a.stashRev = ""
	a.histDiff = loadIdle
	a.enterHistory()
	a.loadCommits()
}

// histName 是提交列表栏头里「现在看的是哪条历史」的那行字。
//
// 不能把 histRev 原样摆上去：它是 refs/heads/feat1 这种完整引用名，用户点
// 的时候看的是 feat1。「所有分支」也不是引用名，得有个说法。
func (a *app) histName() string {
	if a.histAll {
		return T("所有分支", "All Branches")
	}
	if a.histRev == "" {
		return ""
	}
	return shortRev(a.histRev)
}

// shortRev 把完整引用名缩成侧栏里那个样子：refs/heads/feat1 → feat1，
// refs/remotes/origin/main → origin/main。认不出来（哈希、HEAD~3 这类）
// 原样返回。
func shortRev(full string) string {
	for _, prefix := range []string{"refs/heads/", "refs/remotes/", "refs/tags/"} {
		if s, ok := strings.CutPrefix(full, prefix); ok {
			return s
		}
	}
	return full
}

// rebuildRefTree 依据当前引用和筛选词重建侧栏的树。
//
// 展开/收起的状态不用在这里初始化：那一份存的是「用户收起过哪些」，
// 没记过的默认就是展开的（见 refsClosed）。
func (a *app) rebuildRefTree() {
	a.refsFilter = a.refFilter
	a.refTree = buildRefTree(a.refs, a.refFilter)
}
