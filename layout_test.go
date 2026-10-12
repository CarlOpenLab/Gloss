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

// 红绿灯浮在顶栏那条带子上，项目标签就挨着它 —— 左边要让开绿灯按钮的
// 右沿，而且不能只让一两个像素：两个圆角凑在一起，标签的悬停底色一出来
// 就像和窗口按钮连成了一片。
func TestProjectTabClearsTrafficLights(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	tab, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("顶栏上找不到项目标签：%q", tt.Texts())
	}
	if tab.Y > realTitleBar.Height {
		t.Fatalf("找到的 %q 不在顶栏上：y=%.0f", a.repoName, tab.Y)
	}
	if minX := realTitleBar.Left + 12; tab.X < minX {
		t.Errorf("项目标签左边 x=%.0f，离窗口按钮（右沿 x=%.0f）太近了，至少该到 %.0f",
			tab.X, realTitleBar.Left, minX)
	}
}

// 项目路径不占顶栏的常驻位置。
//
// 从前侧栏收起时，工具栏那块仓库名下面还压着一行压短的路径 —— 项目标签
// 刚报过名字，下一行又是一条路径，同一个项目在几像素之内自我介绍两遍。
// 现在路径只在标签的悬停提示里：不占地方，想看时悬停就有。
func TestProjectPathOnlyOnTabTooltip(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false // 仓库名那块摆出来 —— 从前那行路径就贴在它下面

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	tab, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("顶栏上找不到项目标签：%q", tt.Texts())
	}
	if tab.Y > realTitleBar.Height {
		t.Fatalf("找到的 %q 不在顶栏上（y=%.0f）—— 别拿工具栏那块仓库名当标签",
			a.repoName, tab.Y)
	}
	chip, ok := tt.Find(a.branch)
	if !ok {
		t.Fatalf("工具栏上找不到分支徽章：%q", tt.Texts())
	}

	// 顶栏到工具栏这一整块里不该有任何带斜杠的文字 —— 光比完整路径还不
	// 够：从前那行是压短的（「…/T/xxx/001」），完整路径本来就不会出现。
	band := chip.Y + chip.H
	for _, s := range tt.Texts() {
		if !strings.Contains(s, "/") {
			continue
		}
		if r, found := tt.Find(s); found && r.Y <= band {
			t.Errorf("顶栏这条带子里不该有路径，却有 %q（y=%.0f）", s, r.Y)
		}
	}

	tt.Move(tab.X+tab.W/2, tab.Y+tab.H/2)
	// 框架的 tooltip 要悬停满 600ms 才画（ui/widgets.go 的 Tooltip）
	time.Sleep(700 * time.Millisecond)
	tt.Frame()
	if !tt.HasText(a.repoPath) {
		t.Errorf("悬停在项目标签上该给出完整路径 %q：%q", a.repoPath, tt.Texts())
	}
}

// 标签上只有项目名，没有分支名 —— 名字后面缀一段截掉一半的分支，多项目
// 并排时谁都不好认。分支归下面工具栏的分支徽章（侧栏开着时归侧栏的分支树）。
func TestProjectTabShowsNameOnly(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false // 关掉侧栏，分支名就只剩工具栏徽章那一处

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	for _, s := range tt.Texts() {
		// 「main」和从前的「· main」都算：后缀比对把分隔符一并盖住
		if s != a.branch && !strings.HasSuffix(s, " "+a.branch) {
			continue
		}
		if r, found := tt.Find(s); found && r.Y <= realTitleBar.Height {
			t.Errorf("标签上不该出现分支名，实际 %q（y=%.0f）", s, r.Y)
		}
	}
}

// 切换激活标签，整条带子不能有任何东西挪窝。
//
// 激活态一律只换颜色 —— 底色、边框、文字明度，宽度都是零。曾经激活态
// 把名字换成 semibold：字宽比 regular 多一个多百分点，点一下标签，这个
// 框就宽一点点，右边所有标签连同 [+] 跟着一起挪，看着像整条带子在抖。
func TestTabSwitchKeepsLayoutStill(t *testing.T) {
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
		for _, s := range []string{"zz-alpha", "zz-beta-longer", "新建项目标签页"} {
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
			if after[name] != r {
				t.Errorf("激活第 %d 个标签后 %q 挪了窝：%+v → %+v",
					active, name, r, after[name])
			}
		}
	}
}

// 红绿灯那条带子归顶栏管（见上），工具栏在它下面，不必再让位 —— 侧栏
// 收起时它贴着窗口左边缘，侧栏开着时贴着侧栏右边缘。
func TestToolbarHugsSidebarEdge(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	// 拿视图切换那两枚图标按钮定位工具栏的左沿：带文字的控件（仓库名、
	// 分支徽章）在这两种状态里位置会变，只有它一直排在头一个。
	seg, ok := tt.Find("Git")
	if !ok {
		t.Fatalf("找不到工具栏的视图切换：%q", tt.Texts())
	}
	if seg.X > 40 {
		t.Errorf("侧栏收起时工具栏第一个控件左边 x=%.0f，该贴着窗口左边缘", seg.X)
	}

	// 反过来：侧栏开着的时候工具栏跟在侧栏后面，不该再多缩一次 ——
	// 左边贴着侧栏右边缘就行，不该再加上 bar.Left。
	a.sidebarOn = true
	tt.Frame()
	tt.Frame()
	seg2, ok := tt.Find("Git")
	if !ok {
		t.Fatalf("侧栏开着时找不到工具栏的视图切换：%q", tt.Texts())
	}
	if want := float32(sidebarWidth); seg2.X < want || seg2.X > want+40 {
		t.Errorf("侧栏开着时工具栏第一个控件左边 x=%.0f，该贴着侧栏右边缘的 %.0f",
			seg2.X, want)
	}
}

// 「Fork 式布局」的形式化定义：提交列表在上、文件列表在下，两块都从侧栏
// 右边开始 —— 也就是上下分，不是左右分。
//
// 锚点得挑屏幕上唯一的文本。两栏栏头里那句「N 个文件」工具栏上也有一个
// （工作区的文件数），Find 返回先遍历到的那个，拿它当锚点会量到工具栏上去。
// 所以上栏用「N 个提交」，下栏用筛选框 —— 筛选框的矩形左边就是它那一栏的
// 左边缘 +12，和栏头同宽，两者相等就等于两栏左边缘相等。
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
	if cmBox.X != fileBox.X {
		t.Errorf("两栏的左边缘没对齐：提交 %.0f vs 文件 %.0f", cmBox.X, fileBox.X)
	}
	if want := float32(sidebarWidth) + 12; cmBox.X != want {
		t.Errorf("主区左边缘 x=%.0f, want %.0f（侧栏 %d + 内边距 12）",
			cmBox.X, want, sidebarWidth)
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
	// 在底带上约等于 0，量不出任何东西。亮度这一头分得很开 —— 侧栏底色
	// 约 0.035、悬停的 Surface 约 0.062、选中底带约 0.126，0.09 落在
	// 悬停和选中之间，既认得出选中，又不会把悬停误当选中。
	const bandLum = 0.09

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

// 侧栏顶部不摆仓库卡片。
//
// 项目名归顶栏那个项目标签（见 projectTabBar）—— 侧栏再压一张「仓库名 +
// 路径」的卡片，同一个名字在一屏里出现两遍，读起来像两个东西，而不是两处
// 强调。这条盯的就是「卡片别回来」。
//
// 判据卡得很死：工作区导航（「改动」）必须紧贴在侧栏内容区的最上沿。卡片
// 在时它被整张卡片连同那行路径压到下面去（实测 nav.Y 从 82 掉到 120+），
// 所以「离侧栏顶端多远」是个真的能分辨的判据 —— 松成「别掉到半屏以下」
// 等于没写，卡片回来时测试照样绿。
func TestSidebarHasNoRepoCard(t *testing.T) {
	a := richApp(t)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	nav, ok := tt.Find("改动")
	if !ok {
		t.Fatalf("侧栏上找不到工作区导航「改动」：%q", tt.Texts())
	}
	// 没有卡片时实测 nav.Y=44（紧贴侧栏内容顶端，比顶栏下沿还高）。
	// 卡片回来时它被整张卡片连同那行路径压到 67 以上 ——差的这 20 多像素
	// 就是判据，所以阈值卡在 60：够放过字号/留白的变化，不够放过一张卡片。
	// 写成「别掉到半屏以下」那种松判据等于没写，卡片回来时测试照样绿。
	if maxY := float32(60); nav.Y > maxY {
		t.Errorf("工作区导航掉到了 y=%.0f（没有卡片时是 44）—— 上面像是又压了仓库卡片",
			nav.Y)
	}
}
