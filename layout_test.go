package main

import (
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
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

// 侧栏收起来的时候，工具栏自己就贴着窗口左边缘了，这时轮到它让位。
func TestToolbarClearsTitleBarWithoutSidebar(t *testing.T) {
	a := richApp(t)
	a.sidebarOn = false

	tt := ui.NewTester(a.view, 1280, 860)
	tt.SetTitleBar(realTitleBar)
	tt.Frame()
	tt.Frame()

	name, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("找不到仓库名：%q", tt.Texts())
	}
	if name.X < realTitleBar.Left {
		t.Errorf("工具栏文字左边 x=%.0f，红绿灯占到 x=%.0f —— 被压住了",
			name.X, realTitleBar.Left)
	}

	// 反过来：侧栏开着的时候红绿灯被侧栏顶走，工具栏不该再白白缩进去 ——
	// 文字左边贴着侧栏右边缘就行，不该再加上 bar.Left。
	a.sidebarOn = true
	tt.Frame()
	tt.Frame()
	name2, ok := tt.Find(a.repoName)
	if !ok {
		t.Fatalf("侧栏开着时找不到仓库名：%q", tt.Texts())
	}
	if want := float32(sidebarWidth) + 12; name2.X > want+2 {
		t.Errorf("侧栏开着时工具栏文字左边 x=%.0f，该贴着侧栏右边缘的 %.0f",
			name2.X, want)
	}
}

// 卡片和对话框上的 × 要真的点得到。它从 ui.Button(c, "×") 换成了自己画的
// 方形叉号（按钮上不再有文字），这条测试盯着它别变成点不中的装饰。
func TestCloseButtonIsClickable(t *testing.T) {
	a := richApp(t)
	fi, hi := explainFixtures(t, a)
	a.ex = explainState{
		fi:   fi,
		hi:   hi,
		key:  a.explainKey(a.files[fi], hi),
		head: a.files[fi].Hunks[hi].Header,
		text: "这段改动做了什么。",
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if !a.explainOpen() {
		t.Fatal("卡片应该是开着的")
	}

	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("点不到关闭按钮：%v", err)
	}
	tt.Frame()
	if a.explainOpen() {
		t.Error("点了 × 之后卡片该关掉")
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
		r, _, b := rgb(col, y)
		if b-r > 0.06 { // 底色偏蓝 = 胶囊
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
		r, _, b := rgb(x, y)
		if b-r > 0.06 {
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
