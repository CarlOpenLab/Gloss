package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/store"
)

// 设置里改的色号真的画到了屏幕上。
//
// 只断言调色板里的值不够 —— 中间还隔着 buildGraph 指派的色号、图形栏自己的
// 偏移、以及列表的裁剪区（TestHistoryGraphDrawsPixels 那段注释说的就是这个坑：
// 数据全对，屏幕上却一根线都没有）。这里把某条线的色号改成一个主题里绝不会
// 出现的品红，然后去点那一行的节点中心。
func TestGraphLaneOverrideReachesScreen(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 1100)

	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()

	// 挑一条主线上的提交：它占的色号就是这次要改的那一号。
	const name = "主线二"
	i := -1
	for idx, cm := range a.commits {
		if cm.Subject == name {
			i = idx
			break
		}
	}
	if i < 0 {
		t.Fatalf("测试仓库里没有 %q", name)
	}
	x, y := nodeDotAt(t, tt, a, name)
	cid := a.graph[i].Color

	if before := pixelAt(tt, x, y); isMagenta(before) {
		t.Fatalf("改之前这个点就是品红 %v，这条用例什么也证明不了", before)
	}

	// 深浅两份都改：跑的是哪一份取决于 tester 的主题，两份都改就不用去猜。
	a.setLaneOverride(true, cid, "#ff00ff")
	a.setLaneOverride(false, cid, "#ff00ff")
	tt.Frame()
	tt.Frame()

	if got := pixelAt(tt, x, y); !isMagenta(got) {
		t.Errorf("第 %d 号色改成品红之后 %s 的节点还是 %v —— 自定义没画上去", cid, name, got)
	}

	// 清掉覆盖：这一号该回到方案自带的颜色，也就是改之前那个点。
	a.clearLaneOverrides()
	tt.Frame()
	tt.Frame()
	if got := pixelAt(tt, x, y); isMagenta(got) {
		t.Errorf("清除自定义之后还是品红 %v —— 覆盖没跟着清", got)
	}
}

// pixelAt 取图上一点的 RGB（0~255）。
func pixelAt(tt *ui.Tester, x, y int) [3]uint8 {
	r, g, b, _ := tt.Image().At(x, y).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

// nodeDotAt 返回某个提交那一行节点圆点上的取样点（窗口坐标）。
//
// 取样落在实心圆点里（靠上半个半径），别取正中心：正中心可能压着选中行
// 的高亮，也可能被抗锯齿糊到边缘；圆点里离中心半个半径的位置最稳。
func nodeDotAt(t *testing.T, tt *ui.Tester, a *app, name string) (int, int) {
	t.Helper()
	box, ok := tt.Find(name)
	if !ok {
		t.Fatalf("屏幕上找不到 %q（窗口不够高？）", name)
	}
	i := -1
	for idx, cm := range a.commits {
		if cm.Subject == name {
			i = idx
			break
		}
	}
	if i < 0 || i >= len(a.graph) {
		t.Fatalf("提交 %q 不在图里", name)
	}
	// 行里的文字是垂直居中的，所以文字框中线就是节点中心那条水平线；
	// 往上挪 1px，落在实心圆点里、又避开中心那条抗锯齿最重的位置。
	x := int(a.sidebarWidth(nil)+shellCardGap) + int(laneX(a.graph[i].Lane, a.maxLanes))
	y := int(box.Y+box.H/2) - 1
	return x, y
}

// isMagenta 判「这个像素是那支品红」：留一点余量给抗锯齿，但红蓝都得亮、
// 绿得压得很低 —— 用等值比较会被边缘像素坑死。
func isMagenta(p [3]uint8) bool {
	return p[0] > 200 && p[1] < 80 && p[2] > 200
}

// 节点圆点外面那一圈必须留白：连线从上面进来、从下面出去，就是不能从圆点
// 中间穿过去（截图里「线穿过圆点」的观感正是这么来的）。取圆点上下 5px 处：
// 那里在圆点外、留白内，而且再往外 4px 就是竖线本身。留白是线自己让出来的
// （graphNodeEdge），不是拿行底色盖的 —— 行底是半透明的亚克力卡片。
func TestGraphNodeKnockout(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	a.setTab(1)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	x, y := nodeDotAt(t, tt, a, "主线三")
	cy := y + 1 // nodeDotAt 取的是中心上方 1px
	dot := pixelAt(tt, x, cy)
	line := pixelAt(tt, x, cy-9)
	halo := pixelAt(tt, x, cy-5)

	if dot != line {
		t.Fatalf("圆点和它上面的连线应该是同一个色：dot=%v line=%v", dot, line)
	}
	if halo == dot {
		t.Fatalf("圆点上方的留白被线穿了：%v（圆点色 %v）", halo, dot)
	}
}

// scrollTo 把通用偏好里的一项滚进视野，再交给调用方去点。
//
// 两件事一起判：一是这一项真的被排版了（被裁掉时 Find 会给出 W/H 都是 0 的
// 位置，照着它点等于点空），二是它落在面板内容区里（面板最高 680，
// settingsDialog 里那个 clamp；底下的「取消 / 保存」占了最后几十像素）。
// 只判一条都会踩空：前者点到 (0,0)，后者点到模态遮罩上，把对话框点关。
func scrollTo(t *testing.T, tt *ui.Tester, label string, winH float32) {
	t.Helper()
	top, bottom := float32(150), winH-180
	for i := 0; i < 60; i++ {
		box, ok := tt.Find(label)
		if ok && box.W > 0 && box.H > 0 && box.Y >= top && box.Y+box.H <= bottom {
			return
		}
		dy := float32(120)
		if ok && box.Y < top {
			dy = -120
		}
		tt.Scroll(640, winH/2, 0, dy)
		tt.Frame()
	}
	box, ok := tt.Find(label)
	t.Fatalf("滚动之后 %q 还是不可点（%+v, ok=%v）：%q", label, box, ok, tt.Texts())
}

// 设置页里的配色与终端字体：控件改完要同时落到状态和磁盘上。
func TestGraphAndTerminalSettingsPersist(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	a.attachStore(s)

	const winH float32 = 900
	tt := ui.NewTester(a.view, 1280, 900)
	still(tt)
	tt.Frame()

	a.openSettings()
	a.settingsTab = settingsTabGeneral
	tt.Frame()

	if !tt.HasText("拓扑图配色") || !tt.HasText("经典配色") || !tt.HasText("灰阶配色") {
		t.Fatalf("通用偏好里没有配色分区：%q", tt.Texts())
	}
	if !tt.HasText("内置终端") || !tt.HasText("字体族") {
		t.Fatalf("通用偏好里没有终端字体分区：%q", tt.Texts())
	}

	// 换成「鲜艳」：状态和磁盘一起变。
	scrollTo(t, tt, "鲜艳配色", winH)
	if err := tt.Click("鲜艳配色"); err != nil {
		t.Fatalf("点配色卡片：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if a.graphPreset != "vivid" {
		t.Errorf("graphPreset 应为 vivid，实际 %q", a.graphPreset)
	}
	if got := s.Config().GraphPreset; got != "vivid" {
		t.Errorf("方案没落盘：%q", got)
	}

	// 覆盖第 3 号色，再点「清除自定义颜色」。
	a.setLaneOverride(true, 3, "#ff00ff")
	tt.Frame()
	if got := s.Config().GraphLanesDark; len(got) != laneCount || got[3] != "#ff00ff" {
		t.Fatalf("覆盖没落盘：%q", got)
	}
	scrollTo(t, tt, "清除自定义颜色", winH)
	if err := tt.Click("清除自定义颜色"); err != nil {
		t.Fatalf("点清除自定义颜色：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if a.hasLaneOverrides() || s.Config().GraphLanesDark != nil || s.Config().GraphLanesLight != nil {
		t.Errorf("清除没生效：%q / %q", s.Config().GraphLanesDark, s.Config().GraphLanesLight)
	}

	// 终端字号：+ 一步（默认 13 → 14），再重置回默认。
	scrollTo(t, tt, "放大终端字号", winH)
	if err := tt.Click("放大终端字号"); err != nil {
		t.Fatalf("点终端字号 +：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if a.termFontSize != termFontDefault+termFontStep {
		t.Errorf("终端字号该是 %v，实际 %v", termFontDefault+termFontStep, a.termFontSize)
	}
	if got := s.Config().TermFontSize; got != a.termFontSize {
		t.Errorf("终端字号没落盘：%v", got)
	}
	scrollTo(t, tt, "重置字号", winH)
	if err := tt.Click("重置字号"); err != nil {
		t.Fatalf("点终端字号重置：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if a.termFontSize != 0 || s.Config().TermFontSize != 0 {
		t.Errorf("重置后该回到默认（0），实际 %v / %v", a.termFontSize, s.Config().TermFontSize)
	}

	// 终端字体族：设进去再用「默认字体」清掉。
	a.setTermFontFamily("JetBrains Mono")
	tt.Frame()
	if got := s.Config().TermFontFamily; got != "JetBrains Mono" {
		t.Fatalf("字体族没落盘：%q", got)
	}
	scrollTo(t, tt, "默认字体", winH)
	if err := tt.Click("默认字体"); err != nil {
		t.Fatalf("点默认字体：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if a.termFontFamily != "" || s.Config().TermFontFamily != "" {
		t.Errorf("「默认字体」该把它清回空：%q", s.Config().TermFontFamily)
	}
}

// 字体族是个下拉：点开是一列常见等宽字体，选一个就落盘。
func TestTerminalFontFamilyPicker(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	a.attachStore(s)

	tt := ui.NewTester(a.view, 1280, 900)
	still(tt)
	tt.Frame()

	a.openSettings()
	a.settingsTab = settingsTabGeneral
	tt.Frame()

	scrollTo(t, tt, "终端字体族", 900)
	if err := tt.Click("终端字体族"); err != nil {
		t.Fatalf("点字体族下拉：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText("JetBrains Mono") || !tt.HasText("Fira Code") || !tt.HasText("Cascadia Mono") {
		t.Fatalf("下拉里该列出常见等宽字体：%q", tt.Texts())
	}

	if err := tt.Click("Menlo"); err != nil {
		t.Fatalf("选中 Menlo：%v（%q）", err, tt.Texts())
	}
	tt.Frame()
	if a.termFontFamily != "Menlo" {
		t.Errorf("选中的字体没进状态：%q", a.termFontFamily)
	}
	if got := s.Config().TermFontFamily; got != "Menlo" {
		t.Errorf("选中的字体没落盘：%q", got)
	}

	// 落盘的那一份要能在重新打开时原样回来（persist 是后台任务，先等它跑完）。
	settle(t, a)
	if again := store.Open(s.Dir()); again.Config().TermFontFamily != "Menlo" {
		t.Errorf("重开之后字体族丢了：%q", again.Config().TermFontFamily)
	}
}
