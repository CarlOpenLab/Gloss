package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 窗口用的是隐藏标题栏，红绿灯直接浮在内容上。侧栏顶到窗口最上边，
// 顶上那条带子得空出来，否则关闭按钮正好压在筛选框上。
//
// 无头渲染默认没有窗口控件（TitleBar 是零值），所以这里显式摆上真实
// 尺寸 —— 在本机量出来的那组数，见 screenshot_test.go 的 realTitleBar。
func TestSidebarClearsTitleBar(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	box, ok := tt.Find("筛选分支")
	if !ok {
		t.Fatalf("找不到侧栏的筛选框：%q", tt.Texts())
	}
	if box.Y < realTitleBar.Height {
		t.Errorf("筛选框顶边 y=%.0f，红绿灯占到 y=%.0f —— 被压住了",
			box.Y, realTitleBar.Height)
	}
}

// 红绿灯浮在窗口左上角。侧栏自己的顶栏控件要让开它们，项目行则从标题栏
// 下沿开始，不能和窗口按钮抢同一块地方。
func TestSidebarChromeClearsTrafficLights(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	hide, ok := tt.Find("隐藏侧栏")
	if !ok {
		t.Fatalf("侧栏顶栏上找不到隐藏按钮：%q", tt.Texts())
	}
	if minX := realTitleBar.Left + 8; hide.X < minX {
		t.Errorf("侧栏顶栏按钮左边 x=%.0f，离窗口按钮（右沿 x=%.0f）太近了，至少该到 %.0f",
			hide.X, realTitleBar.Left, minX)
	}

	tab, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("侧栏里找不到项目行：%q", tt.Texts())
	}
	if tab.Y < realTitleBar.Height {
		t.Errorf("项目行顶边 y=%.0f 压在标题栏里（高 %.0f）", tab.Y, realTitleBar.Height)
	}
}

// 项目路径不占侧栏的常驻位置。
//
// 常驻显示完整路径会让侧栏项目行变得很长，也会把项目名挤到看不清。现在
// 路径只在项目行的悬停提示里：不占地方，想看时悬停就有。
func TestProjectPathOnlyOnRowTooltip(t *testing.T) {
	a := richApp(t)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	tab, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("侧栏里找不到项目行：%q", tt.Texts())
	}
	if tab.X >= a.sidebarWidth(nil) {
		t.Fatalf("找到的 %q 不在项目侧栏里（x=%.0f）：%q", a.repoName, tab.X, tt.Texts())
	}
	// 完整路径不该常驻；只有悬停提示才该给出它。
	if tt.HasText(a.repoPath) {
		t.Errorf("项目路径不该常驻显示：%q", tt.Texts())
	}

	tt.Move(tab.X+tab.W/2, tab.Y+tab.H/2)
	// 框架的 tooltip 要悬停满 600ms 才画（ui/widgets.go 的 Tooltip）
	time.Sleep(700 * time.Millisecond)
	tt.Frame()
	if !tt.HasText(a.repoPath) {
		t.Errorf("悬停在项目标签上该给出完整路径 %q：%q", a.repoPath, tt.Texts())
	}
}

// 项目行上只有项目名。分支属于下面的独立行和分支树，不该被拼进项目名里。
func TestProjectRowShowsNameOnly(t *testing.T) {
	a := richApp(t)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	if _, ok := tt.Find(a.repoName); !ok {
		t.Fatalf("侧栏里找不到项目行：%q", tt.Texts())
	}
	for _, s := range tt.Texts() {
		if strings.Contains(s, a.repoName) && strings.Contains(s, a.branch) {
			t.Errorf("项目行不该把分支名拼进项目名，实际 %q", s)
		}
	}
}

// 切换激活项目时，项目行的宽高不能跟着变。
//
// 项目行不再有展开内容，激活只换底色和文字颜色；这条盯的是横向抖动：
// 曾经激活态把名字换成 semibold，字宽多一个多百分点，横向边界就会跟着跳。
func TestProjectSwitchKeepsRowWidthStill(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)

	dir2 := t.TempDir()
	runGit(t, dir2, "init", "-b", "main")
	runGit(t, dir2, "config", "user.email", "test@example.com")
	runGit(t, dir2, "config", "user.name", "Test")
	writeFile(t, filepath.Join(dir2, "a.go"), "package main\n")
	runGit(t, dir2, "add", ".")
	runGit(t, dir2, "commit", "-m", "init")
	a.openInNewTab(dir2)
	settle(t, a)

	// 名字换成不会和别处撞车的两串，好在测试里定位这两个标签
	a.tabs[0].name = "zz-alpha"
	a.tabs[1].name = "zz-beta-longer"

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)

	boxes := func() map[string]ui.Rect {
		tt.Frame()
		out := map[string]ui.Rect{}
		for _, s := range []string{"zz-alpha", "zz-beta-longer", T("添加项目", "Add Project")} {
			r, ok := tt.Find(s)
			if !ok {
				t.Fatalf("界面上找不到 %q：%q", s, tt.Texts())
			}
			out[s] = r
		}
		return out
	}

	before := boxes()
	for _, active := range []int{0, 1} {
		a.activeTab = active
		after := boxes()
		for name, r := range before {
			got := after[name]
			if got.X != r.X || got.W != r.W || got.H != r.H {
				t.Errorf("激活第 %d 个项目后 %q 横向尺寸变了：%+v → %+v",
					active, name, r, got)
			}
		}
	}
}

// 工具栏在主工作卡片里：侧栏收起时要给红绿灯留出位置，侧栏开着时则贴着
// 侧栏右边缘，不再额外缩进。
func TestToolbarHugsSidebarEdge(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	show, ok := tt.Find(T("显示侧栏", "Show Sidebar"))
	if !ok {
		t.Fatalf("侧栏收起时找不到显示按钮：%q", tt.Texts())
	}
	if minX := trafficLightsRight() + shellCardGap; show.X < minX {
		t.Errorf("侧栏收起时第一个工具栏控件左边 x=%.0f，该给自绘红绿灯留到 %.0f", show.X, minX)
	}

	seg, ok := tt.Find("改动")
	if !ok {
		t.Fatalf("找不到工具栏的视图切换：%q", tt.Texts())
	}
	if seg.X <= show.X {
		t.Errorf("视图切换 x=%.0f 没有排在显示侧栏按钮 x=%.0f 右边", seg.X, show.X)
	}

	// 侧栏开着的时候，显示按钮让位，视图切换直接贴着侧栏右边缘。
	a.sidebarOn = true
	tt.Frame()
	tt.Frame()
	seg2, ok := tt.Find("改动")
	if !ok {
		t.Fatalf("侧栏开着时找不到工具栏的视图切换：%q", tt.Texts())
	}
	if want := float32(sidebarWidth); seg2.X < want || seg2.X > want+40 {
		t.Errorf("侧栏开着时工具栏第一个控件左边 x=%.0f，该贴着侧栏右边缘的 %.0f",
			seg2.X, want)
	}
}

// 「Fork 式布局」的形式化定义：提交列表在上、文件列表在下 —— 也就是上下分，
// 不是左右分。
//
// 锚点得挑屏幕上唯一的文本。两栏栏头里那句「N 个文件」工具栏上也有一个
// （工作区的文件数），Find 返回先遍历到的那个，拿它当锚点会量到工具栏上去。
// 所以上栏用「N 个提交」，下栏用筛选框。
//
// 两栏左边缘**不再要求相等**：历史 tab 里文件列表左边多一栏提交详情
// （commitdetail.go），文件列表因此整体右移一栏。这里守的是「文件列表在
// 提交列表下面、且不探到它左边」，那才是「上下分」的本意 —— 具体右移多少
// 由详情栏的宽度说，那一栏在改动 tab 里根本不出现。
func TestForkLayoutPanesStack(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()

	cmBox, ok := tt.Find(strconv.Itoa(len(a.commits)) + " 个提交")
	if !ok {
		t.Fatalf("找不到提交栏头（%d 个提交）：%q", len(a.commits), tt.Texts())
	}
	fileBox, ok := tt.Find("筛选文件")
	if !ok {
		t.Fatalf("找不到文件栏的筛选框：%q", tt.Texts())
	}

	if cmBox.Y >= fileBox.Y {
		t.Errorf("提交列表顶边 y=%.0f 不低于文件列表的 y=%.0f —— 该是上下分",
			cmBox.Y, fileBox.Y)
	}
	if fileBox.X < cmBox.X {
		t.Errorf("文件列表探到提交列表左边去了（%.0f < %.0f）—— 那是左右分",
			fileBox.X, cmBox.X)
	}
	if want := float32(sidebarWidth) + shellCardGap + 12; cmBox.X != want {
		t.Errorf("主区左边缘 x=%.0f, want %.0f（侧栏 %d + 卡片间距 %d + 内边距 12）",
			cmBox.X, want, sidebarWidth, shellCardGap)
	}
	// 文件列表右移的那一栏正是提交详情，中间隔着那条 1px 分隔条
	if want := float32(sidebarWidth) + shellCardGap + 12 + detailPaneW + paneSplitterW; fileBox.X != want {
		t.Errorf("文件列表左边缘 x=%.0f, want %.0f（侧栏 + 卡片间距 + 详情栏 %v + 分隔条 %v + 内边距 12）",
			fileBox.X, want, detailPaneW, paneSplitterW)
	}

	// 下半区内部：文件列表在 diff 左边。「已 review」是 diff 文件头里的勾选框，
	// 别处没有，正好当 diff 那一侧的坐标。
	diffBox, ok := tt.Find("已 review")
	if !ok {
		t.Fatalf("diff 的文件头没画出来：%q", tt.Texts())
	}
	if diffBox.X <= fileBox.X {
		t.Errorf("diff（x=%.0f）不在文件列表（x=%.0f）右边", diffBox.X, fileBox.X)
	}
}

// 分组行整行可点：点哪儿都是收起/展开，不是只有一个 12px 的小三角能点。
//
// 顺带盯住收起之后「分组自己还在」—— 收起的是内容，不是把整节一起藏掉。
func TestRefGroupToggles(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if !tt.HasText("feat1") {
		t.Fatalf("展开态该看得到分支：%q", tt.Texts())
	}

	if err := tt.Click("本地分支"); err != nil {
		t.Fatalf("点不到分组行：%v", err)
	}
	tt.Frame()
	if !tt.HasText("本地分支") {
		t.Errorf("收起之后分组标题不该跟着消失：%q", tt.Texts())
	}
	// 判据只用 feat1/feat2：工具栏那个分支 chip 也写着 main，拿 main 当
	// 探针收不收起都是「找得到」。
	for _, gone := range []string{"feat1", "feat2"} {
		if tt.HasText(gone) {
			t.Errorf("收起之后不该还看得到 %q：%q", gone, tt.Texts())
		}
	}

	if err := tt.Click("本地分支"); err != nil {
		t.Fatalf("再点一次：%v", err)
	}
	tt.Frame()
	if !tt.HasText("feat1") {
		t.Errorf("再点一下该展开回来：%q", tt.Texts())
	}
}

// 选中/悬停的底色块是一颗独立的胶囊：上下各留一道缝，左右对称地收进来。
//
// 这条是扫像素的 —— 因为它盯的那个 bug 肉眼不容易说清、却一眼看得出来：
// 底色块原来用 FillHeight（height:100%），而百分比高度会把上下 Margin 顶掉，
// 写 Margin(1) 量出来还是整整 24。于是每一行的底色首尾相连，一列扫下来是
// 一整块板子，不是一颗颗选中项。
func TestTreeSelectionIsPill(t *testing.T) {
	a := graphApp(t)
	settle(t, a)
	for _, ref := range a.refs {
		if ref.Name == "feat1" {
			a.refSel = refLeaf + ref.Full
		}
	}
	if a.refSel == "" {
		t.Fatal("夹具里没有 feat1")
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	img := tt.Image()

	// 侧栏是半透明的，像素是预乘的：取色前先除以 alpha 还原成直通色。
	rgb := func(x, y int) (float32, float32, float32) {
		r, g, b, al := img.At(x, y).RGBA()
		if al == 0 {
			return 0, 0, 0
		}
		return float32(r*255/al) / 255, float32(g*255/al) / 255, float32(b*255/al) / 255
	}

	// lum 是同一套还原之后的相对亮度。
	lum := func(x, y int) float32 {
		r, g, b := rgb(x, y)
		return 0.2126*r + 0.7152*g + 0.0722*b
	}

	// bandLum 是「亮到算底带」的门槛。
	//
	// 判据从「偏蓝」换成亮度了：设计规范把选中底带从中性的灰阶里取色
	// （见 palette.selection），不再用强调色淡染，所以 b-r 这条老判据
	// 在底带上约等于 0，量不出任何东西。亮度这一头分得很开 —— 侧栏在深色
	// 渐变上的底色约 0.105、悬停约 0.15、选中底带约 0.20，0.16 落在悬停
	// 和选中之间，既认得出选中，又不会把悬停误当选中。
	const bandLum = 0.16

	// bandRow 判断 y 这一行是不是选中底带：数整行里「够亮」的像素占比。
	//
	// 占比而不是「有没有」——分组行的灰字和折叠箭头也够亮，但那只是一条细线，
	// 铺不满一行；底带铺满整行，占比接近 1。老判据靠色相把灰字排除在外，
	// 换成亮度之后灰字和底带同色了，只能改用「铺开没有」来分。
	bandRow := func(y int) bool {
		lit := 0
		for x := treePadX; x < sidebarWidth-treePadX; x++ {
			if lum(x, y) > bandLum {
				lit++
			}
		}
		return float32(lit)/float32(sidebarWidth-2*treePadX) > 0.5
	}

	label, ok := tt.Find("feat1")
	if !ok {
		t.Fatalf("侧栏里找不到 feat1：%q", tt.Texts())
	}
	// 名字左边 20 是箭头那一槽：在胶囊里面，又没有文字干扰
	col := int(label.X) - 20
	if col < 0 {
		t.Fatalf("feat1 的 x=%.0f 太靠左，量不了", label.X)
	}

	top, bot := -1, -1
	for y := int(label.Y) - 20; y < int(label.Y+label.H)+20; y++ {
		if bandRow(y) { // 整行都亮 = 底带
			if top < 0 {
				top = y
			}
			bot = y
		}
	}
	if top < 0 {
		t.Fatal("选中行没画出底色")
	}
	if h := bot - top + 1; h != treeRowH-2*treeSelInset {
		t.Errorf("选中底色高 %d，want %d —— 顶满整行就又连成一整块板子了",
			h, treeRowH-2*treeSelInset)
	}
	if mid := float32(top+bot) / 2; mid < label.Y+label.H/2-1 || mid > label.Y+label.H/2+1 {
		t.Errorf("底色中心 y=%.0f 和这一行文字的中心 y=%.0f 没对齐",
			mid, label.Y+label.H/2)
	}

	// 左右边界：两边都该收进来 treePadX，且左右对称
	y := (top + bot) / 2
	left, right := -1, -1
	for x := 0; x < sidebarWidth; x++ {
		if lum(x, y) > bandLum {
			if left < 0 {
				left = x
			}
			right = x
		}
	}
	if left != treePadX {
		t.Errorf("底色左边 x=%d，want %d", left, treePadX)
	}
	if want := sidebarWidth - treePadX - 1; right != want {
		t.Errorf("底色右边 x=%d，want %d（两侧留白要对称）", right, want)
	}
}

// 目录行同理：整行可点，收起后里面的文件就不列了。
func TestFileDirToggles(t *testing.T) {
	a := scrollApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if !tt.HasText("target.txt") {
		t.Fatalf("展开态该看得到文件：%q", tt.Texts())
	}

	if err := tt.Click("m"); err != nil {
		t.Fatalf("点不到目录行：%v", err)
	}
	tt.Frame()
	if !tt.HasText("m") {
		t.Errorf("收起之后目录名不该跟着消失：%q", tt.Texts())
	}
	if tt.HasText("target.txt") {
		t.Errorf("收起之后不该还列出目录里的文件：%q", tt.Texts())
	}

	// 收起状态下点文件树里的别的文件，不该把收起的目录又弹开
	if err := tt.Click("huge1.txt"); err != nil {
		t.Fatalf("点不到 huge1.txt：%v", err)
	}
	tt.Frame()
	if tt.HasText("target.txt") {
		t.Errorf("点别处不该把收起的目录弹开：%q", tt.Texts())
	}
}

// 窄窗口下工具栏把摆不下的控件收进「»」—— 收的该是次要的：窗口最小宽度
// （720）下，「改动 / 历史」还得在。
//
// 这条曾经是反的：左边那块「仓库名 + 路径」有 300 的宽度上限，而窗口收到
// 720 时主区只剩 440 —— 它一个人就吃掉大半条带子，于是连 tab 切换都被挤
// 进「»」菜单里，那是主区唯一的导航。
func TestToolbarKeepsTabsWhenNarrow(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 720, 620)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	for _, want := range []string{"改动", "历史"} {
		if !tt.HasText(want) {
			t.Errorf("最小宽度下 %q 不该被收进「»」：%q", want, tt.Texts())
		}
	}
	if !tt.HasText(a.repoName) {
		t.Errorf("仓库名任何宽度下都该在：%q", tt.Texts())
	}
}

// 「几个文件」只在文件栏栏头报一次。曾经工具栏上也报一遍，两行隔几十像素
// 摆同一个数字，不是强调，是啰嗦。
func TestFileCountReportedOnce(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	if n := countText(tt.Texts(), strconv.Itoa(len(a.files))+" 个文件"); n != 1 {
		t.Errorf("「%d 个文件」该只出现一次，实际 %d 次：%q", len(a.files), n, tt.Texts())
	}
}

// 刷新过程中工具栏按钮零偏移：无论是在 ready 还是 loading 态，
// 刷新、设置、打开仓库按钮的 X 坐标严格一致，不产生任何横向抖动。
func TestRefreshLoadingLayoutDoesNotShift(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	refreshReady, ok1 := tt.Find("刷新")
	settingsReady, ok2 := tt.Find("模型设置…")
	openReady, ok3 := tt.Find("打开仓库…")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("找不到工具栏按钮: %v %v %v", ok1, ok2, ok3)
	}

	// 切换到刷新中状态
	a.state = loadLoading
	tt.Frame()

	refreshLoading, _ := tt.Find("刷新")
	settingsLoading, _ := tt.Find("模型设置…")
	openLoading, _ := tt.Find("打开仓库…")

	if refreshLoading.X != refreshReady.X {
		t.Errorf("刷新按钮 X 坐标偏移：ready=%.1f, loading=%.1f", refreshReady.X, refreshLoading.X)
	}
	if settingsLoading.X != settingsReady.X {
		t.Errorf("设置按钮 X 坐标偏移：ready=%.1f, loading=%.1f", settingsReady.X, settingsLoading.X)
	}
	if openLoading.X != openReady.X {
		t.Errorf("打开仓库按钮 X 坐标偏移：ready=%.1f, loading=%.1f", openReady.X, openLoading.X)
	}
}

// 暂存/取消暂存时不展示工具栏操作文字，避免剧烈拉扯工具栏造成页面抖动。
func TestStageOpDoesNotShowInToolbar(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 模拟正在暂存
	a.op = gitrepo.OpStage
	a.opName = "正在暂存 README.md"
	tt.Frame()

	if tt.HasText("正在暂存 README.md") {
		t.Error("暂存属于瞬时操作，不应在工具栏插入长文字导致抖动")
	}

	// 模拟耗时写操作（如拉取），应展示在 Spacer 之前
	a.op = gitrepo.OpPull
	a.opName = "正在拉取…"
	tt.Frame()

	if !tt.HasText("正在拉取…") {
		t.Error("远程写操作应在工具栏提示")
	}
}

// 刷新同一个仓库时，侧栏分支树、筛选框与底部提交栏必须平稳保留，
// 绝不能在加载期间卸载闪烁或变成空状态。
func TestReloadKeepsSidebarAndCommitBoxMounted(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	settle(t, a)
	tt.Frame()

	// 确认准备就绪时存在分支树与提交栏
	if !tt.HasText("筛选分支") {
		t.Fatal("准备就绪时侧栏应有分支筛选框")
	}
	if !tt.HasText("提交信息") {
		t.Fatal("准备就绪时应有底部提交框")
	}

	// 触发重新加载同一个仓库（模拟刷新中）
	a.state = loadLoading
	a.loadingPath = a.repoPath
	a.refsState = loadLoading
	tt.Frame()

	// 刷新期间侧栏与提交栏继续平稳驻留，绝不卸载
	if !tt.HasText("筛选分支") {
		t.Errorf("刷新期间侧栏分支筛选框被卸载闪烁：%q", tt.Texts())
	}
	if tt.HasText("正在读分支…") {
		t.Errorf("已有分支时刷新期间不该闪现「正在读分支…」：%q", tt.Texts())
	}
	if !tt.HasText("提交信息") {
		t.Errorf("刷新期间底部提交框被卸载闪烁：%q", tt.Texts())
	}

	// 模拟真实 a.reload() 调用
	settle(t, a)
	a.reload()
	tt.Frame()
	t.Logf("Texts after a.reload(): %v", tt.Texts())
	if !tt.HasText("筛选分支") {
		t.Errorf("调用 a.reload() 第一帧侧栏分支筛选框被卸载闪烁：%q", tt.Texts())
	}
	if tt.HasText("正在读分支…") {
		t.Errorf("调用 a.reload() 第一帧已有分支时不该闪现「正在读分支…」：%q", tt.Texts())
	}
	if !tt.HasText("提交信息") {
		t.Errorf("调用 a.reload() 第一帧底部提交框被卸载闪烁：%q", tt.Texts())
	}

	settle(t, a)
	tt.Frame()
	t.Logf("Texts after settle: %v", tt.Texts())
	if !tt.HasText("筛选分支") {
		t.Errorf("刷新完成侧栏分支筛选框缺失：%q", tt.Texts())
	}
	if !tt.HasText("main") {
		t.Errorf("刷新完成分支 main 缺失：%q", tt.Texts())
	}
}

// 测试快速暂存写操作（OpStage / OpUnstage）期间，提交按钮和快捷键提示不闪烁禁用。
func TestStagingOpKeepsCommitButtonAndHintStable(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	settle(t, a)
	tt.Frame()

	// 模拟已暂存 1 个文件并输入提交信息
	a.commitMsg = "test commit"
	a.allFiles[0].Section = gitrepo.SectionStaged
	tt.Frame()

	if !a.canCommit() {
		t.Fatal("有暂存和消息时应允许提交")
	}
	if !tt.HasText("⌘⏎") {
		t.Fatal("允许提交时应显示 ⌘⏎ 提示")
	}

	// 模拟点击暂存另一个文件，触发 OpStage
	a.op = gitrepo.OpStage
	tt.Frame()

	if !a.canCommit() {
		t.Error("暂存写操作进行中，canCommit 不应闪退禁用导致透明度闪烁")
	}
	if !tt.HasText("⌘⏎") {
		t.Error("暂存写操作进行中，⌘⏎ 快捷键提示不应闪退消失")
	}
}

// 测试干净仓库（无改动）刷新时，主区提示稳定不出现闪烁。
func TestCleanRepoReloadDoesNotFlickerEmptyMessage(t *testing.T) {
	a := richApp(t)
	// 将所有文件标记为无改动（空列表）
	a.files = nil
	a.allFiles = nil
	a.tree = nil
	tt := ui.NewTester(a.view, 1280, 860)
	settle(t, a)
	tt.Frame()

	if !tt.HasText("没有未提交的改动") {
		t.Fatalf("干净仓库主区应显示无未提交改动：%q", tt.Texts())
	}

	// 刷新第一帧
	a.state = loadLoading
	a.loadingPath = a.repoPath
	tt.Frame()

	if !tt.HasText("没有未提交的改动") {
		t.Errorf("刷新第一帧不该将干净仓库提示换成转圈或空白：%q", tt.Texts())
	}
	if tt.HasText("正在读取仓库…") {
		t.Errorf("已有仓库刷新时不应在主区居中闪现「正在读取仓库…」：%q", tt.Texts())
	}
}

// 文件栏栏头在「部分暂存」（既有已暂存又有未暂存）时不该折行。
//
// 这条曾经是坏的：栏头把勾选框、文件数、两颗文字药丸和一组增删总数挤在
// 300px 里，半暂存时四样一起出现，摆不下就各自折成两行，整条栏头看着像
// 碎了。现在栏头只留勾选框与文件数，多余的都去掉了。
func TestFileHeaderNoWrapWhenHalfStaged(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	box, ok := tt.Find(strconv.Itoa(len(a.files)) + " 个文件")
	if !ok {
		t.Fatalf("找不到文件栏头：%q", tt.Texts())
	}
	// 单行 11px 的字高在 13 上下；折成两行会到 26 往上。
	if box.H > 20 {
		t.Errorf("部分暂存时文件栏头折行了：高 %.0f", box.H)
	}
}

// 增删总数只在栏脚「总计」里报一次。曾经栏头也摆一份，两处相隔几十像素
// 报同一个数字 —— 和「几个文件」是同一条规则。
func TestFileTotalsReportedOnce(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	add := "+" + strconv.Itoa(a.totalAdd())
	if n := countText(tt.Texts(), add); n != 1 {
		t.Errorf("「%s」该只在栏脚报一次，实际 %d 次：%q", add, n, tt.Texts())
	}
}

// 窄窗口下提交栏底部的动作按钮不能被顶出窗口右边。
//
// 主区放不下「状态 + 预检 + AI 生成 + 提交」一行时，提交栏要折成两行，
// 而不是让提交按钮跑到窗口外面去 —— 那是用户唯一必须够得着的一颗按钮。
func TestCommitBarKeepsActionsWhenNarrow(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 720, 620)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	btn, ok := tt.Find("提交")
	if !ok {
		t.Fatalf("窄窗口下找不到提交按钮：%q", tt.Texts())
	}
	if btn.X+btn.W > 720+1 {
		t.Errorf("提交按钮右边缘 x=%.0f 出了 720 的窗口", btn.X+btn.W)
	}
	if btn.Y+btn.H > 620+1 {
		t.Errorf("提交按钮下边缘 y=%.0f 出了 620 的窗口", btn.Y+btn.H)
	}
}

// 窄窗口下分支对比栏折成两行：上一行「基准 ⇄ 对比 + 退出」，下一行模式与
// 统计。原来是一整条不可换行的 Row，720 下「退出对比」被顶到窗口外，
// 标题也被 Shrink(0) 的邻居挤成竖排。
func TestCompareBarKeepsExitWhenNarrow(t *testing.T) {
	a := richApp(t)
	a.startCompare("main", "feature", true)
	a.compare.state = loadReady

	tt := ui.NewTester(a.view, 720, 620)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	exit, ok := tt.Find("退出对比")
	if !ok {
		t.Fatalf("窄窗口下找不到退出对比按钮：%q", tt.Texts())
	}
	if exit.X+exit.W > 720+1 {
		t.Errorf("退出对比右边缘 x=%.0f 出了 720 的窗口", exit.X+exit.W)
	}
}

// 设置面板在最小窗口下要收进屏幕：固定 960×680 会让底部的保存 / 取消
// 四边出屏、点不到。
func TestSettingsFitsWhenNarrow(t *testing.T) {
	a := richApp(t)
	a.openSettings()

	tt := ui.NewTester(a.view, 720, 620)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	save, ok := tt.Find("保存")
	if !ok {
		t.Fatalf("设置面板里找不到保存按钮：%q", tt.Texts())
	}
	if save.X+save.W > 720+1 || save.Y+save.H > 620+1 {
		t.Errorf("保存按钮出屏：x=%.0f..%.0f y=%.0f..%.0f",
			save.X, save.X+save.W, save.Y, save.Y+save.H)
	}
}

// 工作区导航（改动 / 历史 / 终端 / 编辑器）在工具栏那一段四选一。
//
// 侧栏现在只有项目与分支，不再摆改动 / 历史的入口；判据是这四格必须落在
// 侧栏右边缘之后，排成一行，并从左到右依次排列。哪一格缩回侧栏或顺序变了
// 都会失败。
func TestWorkspaceNavLivesInToolbar(t *testing.T) {
	a := richApp(t)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.Frame()

	sw := a.sidebarWidth(nil)
	order := []string{"改动", "历史", "终端", "编辑器"}
	rects := make([]ui.Rect, 0, len(order))
	for _, label := range order {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("工具栏上找不到「%s」这一格：%q", label, tt.Texts())
		}
		rects = append(rects, r)
		if r.X < sw {
			t.Errorf("「%s」的左沿 x=%.0f 落在侧栏里（侧栏宽 %.0f）—— 它该在工具栏上",
				label, r.X, sw)
		}
	}
	for i := 1; i < len(rects); i++ {
		if rects[i].X <= rects[i-1].X {
			t.Errorf("四格的顺序不对：%s 没排在 %s 右边（x=%.0f，前一格 x=%.0f）",
				order[i], order[i-1], rects[i].X, rects[i-1].X)
		}
		if rects[i].Y != rects[i-1].Y {
			t.Errorf("四格没排在一行：「%s」y=%.0f，「%s」y=%.0f",
				order[i-1], rects[i-1].Y, order[i], rects[i].Y)
		}
	}
}

// macOS 的窗口按钮由视图自己画（main.go 用 Frameless 收走系统那三颗），
// 位置仍是左上角：三颗都要有、排成一行、落在顶栏中线上，并且不压住卡片里
// 的工具栏内容。
func TestWindowTrafficLightsAreSelfDrawn(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	labels := []string{
		T("关闭窗口", "Close Window"),
		T("最小化窗口", "Minimize Window"),
		T("缩放窗口", "Zoom Window"),
	}
	var last ui.Rect
	for i, label := range labels {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("顶栏上找不到自绘窗口按钮 %q：%q", label, tt.Texts())
		}
		if r.X < 6 || r.X+r.W > trafficLightsRight()+2 {
			t.Errorf("%q 的横坐标 x=%.0f..%.0f 不在左上角红绿灯区（6..%.0f）里",
				label, r.X, r.X+r.W, trafficLightsRight()+2)
		}
		if mid := r.Y + r.H/2; mid < shellBarMid-2 || mid > shellBarMid+2 {
			t.Errorf("%q 的中心 y=%.0f，该在顶栏中线 %.0f 上", label, mid, float32(shellBarMid))
		}
		if i > 0 && r.X <= last.X {
			t.Errorf("%q 没有排在前一颗右边", label)
		}
		last = r
	}

	// 侧栏收起时，卡片里的工具栏要让开它们，不能从圆点底下穿过去。
	a.setSidebar(false)
	tt.Frame()
	show, ok := tt.Find(T("显示侧栏", "Show Sidebar"))
	if !ok {
		t.Fatalf("侧栏收起后找不到显示按钮：%q", tt.Texts())
	}
	if show.X < trafficLightsRight() {
		t.Errorf("显示侧栏按钮 x=%.0f 压在红绿灯（右沿 %.0f）下面", show.X, trafficLightsRight())
	}
}
