package main

import (
	"strconv"
	"strings"
	"testing"

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
//
// 卡片是刚打开就点的 —— 也就是它可能还在淡入。这一条顺带钉住「卡片的位置
// 不能跟着动画走」：位置一动，按下和松开之间按钮就溜到别处去了，这一次
// 点击白点。（见 explain.go 里那段 Position 的说明。）
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
	a.rowsDirty = true // 内嵌解释行要随行模型重建才出现

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

// 工具栏那行路径：放得下就原样显示，放不下从**头上**截 —— 路径的关键
// 信息在末尾（在哪一级目录下、和重名的怎么区分），截尾留下的正好是
// 「/Users/…」这种最没用的部分。
func TestToolbarPathShortening(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)

	// 短路径原样显示
	a.repoPath = "/tmp/gloss"
	tt.Frame()
	if !tt.HasText("/tmp/gloss") {
		t.Errorf("放得下的路径该原样显示：%q", tt.Texts())
	}

	// 长路径：压短了，但尾巴还在（结尾是这个仓库）
	long := "/Users/me/" + strings.Repeat("a-quite-long-directory-name/", 4) + "gloss"
	a.repoPath = long
	tt.Frame()
	if tt.HasText(long) {
		t.Errorf("这么长的路径该被压短：%q", tt.Texts())
	}
	tail := shortText(tt.Texts(), "…/")
	if tail == "" {
		t.Fatalf("压短之后该留下「…/」开头的尾巴：%q", tt.Texts())
	}
	if !strings.HasSuffix(tail, "/gloss") {
		t.Errorf("留下的尾巴该以仓库名结尾，实际 %q", tail)
	}

	// 连两级都放不下（末两节本身就很长）：整行省掉 —— 只剩仓库名的话，
	// 就是上面那行，摆两遍不是信息
	a.repoPath = "/a/" + strings.Repeat("zh", 120) + "/gloss"
	tt.Frame()
	if s := shortText(tt.Texts(), "…/"); s != "" {
		t.Errorf("压不出有意义的尾巴时该整行省掉，实际还有 %q", s)
	}
}

// shortText 找出以 prefix 开头的第一段文字，没有返回空串。
func shortText(texts []string, prefix string) string {
	for _, s := range texts {
		if strings.HasPrefix(s, prefix) {
			return s
		}
	}
	return ""
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
