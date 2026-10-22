package main

import (
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func TestTabCreationAndSwitching(t *testing.T) {
	a := newApp()
	repo1 := filepath.Join(t.TempDir(), "repo1")
	repo2 := filepath.Join(t.TempDir(), "repo2")

	a.openInNewTab(repo1)
	if len(a.tabs) != 1 {
		t.Fatalf("预期 1 个标签页，实际 %d", len(a.tabs))
	}
	if a.activeTab != 0 {
		t.Errorf("预期当前激活第 0 个标签页，实际 %d", a.activeTab)
	}

	a.openInNewTab(repo2)
	if len(a.tabs) != 2 {
		t.Fatalf("预期 2 个标签页，实际 %d", len(a.tabs))
	}
	if a.activeTab != 1 {
		t.Errorf("预期当前激活第 1 个标签页，实际 %d", a.activeTab)
	}

	// 切换回第 0 个
	a.switchTab(0)
	if a.activeTab != 0 {
		t.Errorf("switchTab(0) 后应激活第 0 个，实际 %d", a.activeTab)
	}

	// 再次打开已存在的 repo1，应直接激活已有 tab 而不是新建
	a.openInNewTab(repo1)
	if len(a.tabs) != 2 {
		t.Fatalf("重复打开已有仓库不应新建 tab，预期 2 个，实际 %d", len(a.tabs))
	}
	if a.activeTab != 0 {
		t.Errorf("重复打开已存在的 repo1 应切回第 0 个标签页，实际 %d", a.activeTab)
	}
}

func TestTabCycle(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	a.openInNewTab("/tmp/proj2")
	a.openInNewTab("/tmp/proj3")

	if a.activeTab != 2 {
		t.Fatalf("当前应激活最后新开的第 2 个 tab，实际 %d", a.activeTab)
	}

	a.nextTab() // 循环回到 0
	if a.activeTab != 0 {
		t.Errorf("nextTab 预期切到 0，实际 %d", a.activeTab)
	}

	a.prevTab() // 循环回到 2
	if a.activeTab != 2 {
		t.Errorf("prevTab 预期切到 2，实际 %d", a.activeTab)
	}
}

func TestTabClosing(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	a.openInNewTab("/tmp/proj2")
	a.openInNewTab("/tmp/proj3")

	// 当前在 tab 2，关闭 tab 2 后应激活 tab 1
	a.closeTab(2)
	if len(a.tabs) != 2 {
		t.Fatalf("关闭后预期剩余 2 个 tab，实际 %d", len(a.tabs))
	}
	if a.activeTab != 1 {
		t.Errorf("关闭当前 tab 后预期激活第 1 个，实际 %d", a.activeTab)
	}

	// 关闭剩下的全部 tab
	a.closeTab(0)
	a.closeTab(0)
	if len(a.tabs) != 1 {
		t.Fatalf("关闭所有 tab 后应保留一个空兜底 tab，实际 %d", len(a.tabs))
	}
	if a.repoPath != "" {
		t.Errorf("清空所有 tab 后 repoPath 应为空，实际 %q", a.repoPath)
	}
}

func TestViewModeToggle(t *testing.T) {
	a := newApp()
	a.openInNewTab("/tmp/proj1")

	if a.viewMode != viewModeGit {
		t.Errorf("默认视图模式应为 Git GUI (0)，实际 %d", a.viewMode)
	}

	a.toggleViewMode()
	if a.viewMode != viewModeTerm {
		t.Errorf("toggleViewMode 后应为终端模式 (1)，实际 %d", a.viewMode)
	}

	tab := a.currentTab()
	if tab == nil || tab.viewMode != viewModeTerm {
		t.Errorf("标签页内部视图模式应同步为终端模式")
	}

	a.toggleViewMode()
	if a.viewMode != viewModeGit {
		t.Errorf("再次 toggleViewMode 应切回 Git GUI 模式 (0)，实际 %d", a.viewMode)
	}
}

// stubTerminal 把「起 shell」换成替身。
//
// 会话列表的增、删、切换是纯逻辑：为它每次真起一个 shell，既慢，又要在
// 用例结束之后照顾一堆 SIGHUP 还没走完的子进程。替身不碰 pty，只记一笔
// 「有人要起终端了」—— 用例要断言的正是这一步有没有发生、发生了几次。
func stubTerminal(t *testing.T) *int {
	t.Helper()
	calls := new(int)
	old := terminalNew
	terminalNew = func(terminal.Options) (*terminal.Terminal, error) {
		*calls++
		return nil, nil
	}
	t.Cleanup(func() { terminalNew = old })
	return calls
}

// TestTerminalSessions 走一遍终端会话的增、删、切换，以及「关掉最后一个」。
func TestTerminalSessions(t *testing.T) {
	spawns := stubTerminal(t)
	a := newApp()
	a.openInNewTab("/tmp/proj1")
	tab := a.currentTab()
	tt := ui.NewTester(a.view, 1000, 700)

	// Git 面板里不该有 shell：终端是进了终端视图才有的东西。
	tt.Frame()
	if len(tab.terms) != 0 || *spawns != 0 {
		t.Fatalf("没进终端视图就起了会话：%d 个，起了 %d 次", len(tab.terms), *spawns)
	}

	// 进终端视图：不擅自起 shell —— 一个会话都没有，落空状态页。
	a.setViewMode(viewModeTerm)
	tt.Frame()
	if len(tab.terms) != 0 || *spawns != 0 {
		t.Fatalf("进终端视图不该擅自起会话：%d 个，起了 %d 次", len(tab.terms), *spawns)
	}
	if !tt.HasText("没有终端会话") {
		t.Errorf("空状态页该有标题，实际 %q", tt.Texts())
	}

	// 多渲染几帧照样一个都不起。
	tt.Frame()
	tt.Frame()
	if *spawns != 0 {
		t.Errorf("没有会话时渲染多帧也不该起 shell，实际 %d 次", *spawns)
	}

	// 空状态页上的「新建终端」起第一个会话，并在这一帧把它起起来。
	if err := tt.Click("新建终端"); err != nil {
		t.Fatalf("空状态页上找不到新建按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 1 || tab.termActive != 0 {
		t.Fatalf("点「新建终端」该有一个会话：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}
	if *spawns != 1 {
		t.Fatalf("新会话该起一次 shell，实际 %d 次", *spawns)
	}
	if !tt.HasText("终端 1") {
		t.Errorf("空标题该回落到「终端 1」，实际 %q", tt.Texts())
	}

	// ⌘⇧T：再开一个，并切到新的那个。
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	if len(tab.terms) != 2 || tab.termActive != 1 {
		t.Fatalf("⌘⇧T 该再开一个会话并切过去：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}
	if *spawns != 2 {
		t.Errorf("新会话该起一次 shell，实际共 %d 次", *spawns)
	}
	if !tt.HasText("终端 2") {
		t.Errorf("第二枚芯片该是「终端 2」，实际 %q", tt.Texts())
	}

	// 芯片带右边那枚 + 是同一个动作。
	if err := tt.Click("新建终端标签页"); err != nil {
		t.Fatalf("芯片带上找不到新建按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 3 || tab.termActive != 2 {
		t.Fatalf("+ 该再开一个会话并切过去：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}

	// 点第一枚芯片切回第一个会话。
	if err := tt.Click("终端 1"); err != nil {
		t.Fatalf("找不到第一枚芯片：%v（%q）", err, tt.Texts())
	}
	if tab.termActive != 0 || a.activeSession() != tab.terms[0] {
		t.Fatalf("点芯片该切过去：激活 %d", tab.termActive)
	}

	// × 关掉第一枚：剩下的两枚各自留在原地，激活的跟到原来的第二个上。
	before := *spawns
	if err := tt.Click("关闭终端标签页"); err != nil {
		t.Fatalf("芯片上找不到关闭按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 2 {
		t.Fatalf("关掉一个后该剩两个会话，实际 %d", len(tab.terms))
	}
	if tab.terms[0].ord != 2 || tab.terms[1].ord != 3 {
		t.Fatalf("剩下的会话不该重编号：%d, %d", tab.terms[0].ord, tab.terms[1].ord)
	}
	if *spawns != before {
		t.Errorf("关会话不该顺手再起 shell，多起了 %d 次", *spawns-before)
	}

	// 再关一下：剩最后一个，不补。
	if err := tt.Click("关闭终端标签页"); err != nil {
		t.Fatalf("找不到关闭按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 1 || tab.terms[0].ord != 3 {
		t.Fatalf("关掉一个后该剩 ord 3 那一个，实际 %d 个", len(tab.terms))
	}
	if *spawns != before {
		t.Errorf("关会话不该顺手再起 shell，多起了 %d 次", *spawns-before)
	}

	// 关掉最后一个：落空状态页，不补新的 shell。
	if err := tt.Click("关闭终端标签页"); err != nil {
		t.Fatalf("找不到关闭按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 0 {
		t.Fatalf("关掉最后一个该剩空状态，实际 %d 个", len(tab.terms))
	}
	if !tt.HasText("没有终端会话") {
		t.Errorf("关光之后该回空状态页，实际 %q", tt.Texts())
	}
	if *spawns != before {
		t.Errorf("关掉最后一个不该补新 shell，多起了 %d 次", *spawns-before)
	}

	// 空状态页上再新建：编号接着往后发（ord 4）。
	if err := tt.Click("新建终端"); err != nil {
		t.Fatalf("空状态页上找不到新建按钮：%v（%q）", err, tt.Texts())
	}
	if len(tab.terms) != 1 || tab.terms[0].ord != 4 {
		t.Fatalf("新会话该是编号继续往后发的那一个（ord 4），实际 %d 个", len(tab.terms))
	}
	if *spawns != before+1 {
		t.Errorf("新建该起一次 shell，实际多起 %d 次", *spawns-before)
	}

	// 重启只换壳不换芯片：会话还是那一个，编号也不动。
	cur := a.activeSession()
	if cur == nil {
		t.Fatal("没有激活的会话")
	}
	before = *spawns
	a.restartCurrentTerminal()
	tt.Frame()
	if a.activeSession() != cur {
		t.Errorf("重启后该还是同一个会话")
	}
	if cur.ord != tab.terms[0].ord {
		t.Errorf("重启不该换编号：%d", cur.ord)
	}
	if *spawns != before+1 {
		t.Errorf("重启该重新起一次 shell，实际 %d 次", *spawns-before)
	}

	// 清屏只作用于当前会话，不碰列表。
	a.clearCurrentTerminal()
	if len(tab.terms) != 1 || a.activeSession() != cur {
		t.Errorf("清屏不该动会话列表：%d 个", len(tab.terms))
	}

	// 关窗 / 退出那条路：全放掉，不补。
	a.closeAllTerminals()
	if len(tab.terms) != 0 {
		t.Errorf("closeAllTerminals 后不该有会话，实际 %d", len(tab.terms))
	}
}

// fontFamilyWithFallback 只补一个尾巴，且不重复补。
func TestFontFamilyWithFallback(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},                                 // 空交给插件自己（它就是系统等宽）
		{"   ", ""},                              // 只有空格同理
		{"Menlo", "Menlo, monospace"},            // 补一个
		{"Menlo, monospace", "Menlo, monospace"}, // 已经有了
		{"Menlo, MONOSPACE", "Menlo, MONOSPACE"}, // 大小写不敏感
		{" Menlo ", "Menlo, monospace"},          // 顺带去掉两头的空格
		{"monospace", "monospace"},               // 整串就是它
	}
	for _, c := range cases {
		if got := fontFamilyWithFallback(c.in); got != c.want {
			t.Errorf("fontFamilyWithFallback(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 选了个这台机器没装的字体族，终端不能变成比例字体。
//
// 上一条用例拿「列数」当插件字体度量的探针，这条接着用它验兜底：不存在的字族
// 配上 monospace 尾巴，一屏列数必须和系统等宽**一模一样**。实测过没有兜底时的
// 样子 —— 同样的窗口掉到 53 列（引擎换了一支比例字体），终端会错行，所以这个
// 尾巴不是装饰。
func TestTerminalFontFallsBackToMonospace(t *testing.T) {
	colsFor := func(family string) int {
		t.Helper()
		conn := newRecordingConn()
		old := terminalNew
		terminalNew = func(o terminal.Options) (*terminal.Terminal, error) {
			o.Conn = conn
			return terminal.New(o)
		}
		defer func() {
			terminalNew = old
			conn.Close()
		}()

		a := newApp()
		a.openInNewTab(t.TempDir())
		a.termFontFamily = family
		tt := ui.NewTester(a.view, 1000, 700)
		a.setViewMode(viewModeTerm)
		a.newTerminalSession(a.currentTab()) // 终端视图不擅自起 shell，会话自己开
		tt.Frame()
		tt.Frame()
		return waitCols(t, conn, 0)
	}

	base := colsFor("")
	if got := colsFor("NoSuchFontFamilyZZZ"); got != base {
		t.Errorf("没装的字体族该退到系统等宽（%d 列），实际 %d 列 —— 兜底没生效，终端会错行", base, got)
	}
}

// recordingConn 冒充终端对端：它不跑任何程序，只记下插件告诉它的屏幕尺寸。
//
// 字号变了，插件的字体度量跟着变，格子尺寸就得重算 —— 而 SetFont 没有返回值
// 可以断言，列数就是「字体真的推到了活着的会话」能看得见的那一面。
type recordingConn struct {
	mu    sync.Mutex
	cols  int
	rows  int
	calls int
	done  chan struct{}
	once  sync.Once
}

func newRecordingConn() *recordingConn { return &recordingConn{done: make(chan struct{})} }

func (c *recordingConn) Read([]byte) (int, error) { <-c.done; return 0, io.EOF }

func (c *recordingConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *recordingConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return nil
}

func (c *recordingConn) Resize(cols, rows int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cols, c.rows, c.calls = cols, rows, c.calls+1
	return nil
}

func (c *recordingConn) size() (int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cols, c.rows, c.calls
}

// waitCols 等插件报出一个和 prev 不同的列数。
//
// 列数是插件在自己的布局里算出来、再交给对端的，中间跨一条 goroutine，
// 所以这里只能等，不能「拍一帧就断言」。
func waitCols(t *testing.T, c *recordingConn, prev int) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cols, _, _ := c.size(); cols > 0 && cols != prev {
			return cols
		}
		time.Sleep(10 * time.Millisecond)
	}
	cols, _, _ := c.size()
	t.Fatalf("等不到新的列数（还是 %d）", cols)
	return 0
}

// 内置终端的字体：起会话时交给插件，改了之后推给活着的会话。
//
// 这比「Options.Font 传对了」远一截：pty 换成假连接，插件本体是真的，
// 于是字号的变化会一路走到格子尺寸 —— 那是终端真正拿字号干的事。
func TestTerminalFontReachesLiveSession(t *testing.T) {
	conn := newRecordingConn()
	t.Cleanup(func() { conn.Close() })

	old := terminalNew
	terminalNew = func(o terminal.Options) (*terminal.Terminal, error) {
		o.Conn = conn
		return terminal.New(o)
	}
	t.Cleanup(func() { terminalNew = old })

	a := newApp()
	a.openInNewTab(t.TempDir())
	a.setTermFontSize(termFontDefault)
	tt := ui.NewTester(a.view, 1000, 700)

	a.setViewMode(viewModeTerm)
	a.newTerminalSession(a.currentTab()) // 终端视图不擅自起 shell，会话自己开
	tt.Frame()
	tt.Frame()

	at13 := waitCols(t, conn, 0)

	// 改字号：不重启 shell，也不该丢会话。
	cur := a.activeSession()
	a.setTermFontSize(24)
	tt.Frame()
	tt.Frame()

	at24 := waitCols(t, conn, at13)
	if at24 >= at13 {
		t.Errorf("字号 %d → 24 之后一屏该少放几列：%d → %d", termFontDefault, at13, at24)
	}
	if a.activeSession() != cur || cur.started != true {
		t.Errorf("改字号不该换会话或重启 shell")
	}

	// 字体没变时不该反复推：多渲染几帧，列数与推送次数都不动。
	_, _, calls := conn.size()
	tt.Frame()
	tt.Frame()
	if cols, _, after := conn.size(); cols != at24 || after != calls {
		t.Errorf("字体没变却又推了一次：列数 %d → %d，推送 %d → %d", at24, cols, calls, after)
	}
}

// dragBetween 按住 (fromX, fromY) 一路挪到 (toX, toY) 再松手。中间分几步走，
// 因为起手要指针挪够几个 DIP（见 mygo 的 dragStart）：一下就跳过去不算拖。
func dragBetween(tt *ui.Tester, fromX, fromY, toX, toY float32) {
	tt.Press(fromX, fromY)
	for k := 1; k <= 4; k++ {
		f := float32(k) / 4
		tt.Move(fromX+(toX-fromX)*f, fromY+(toY-fromY)*f)
	}
	tt.Release(toX, toY)
}

// reorder 是项目标签页和终端芯片两处拖动共用的那一步：落点一律是「插到
// 第 to 项前面」，而把要挪的那一项摘掉之后，落点在它后面的下标都要减一。
// 这一步算错了，表现是「往右拖一格没动、拖两格跑过头」。
func TestReorderMovesItem(t *testing.T) {
	cases := []struct {
		name     string
		from, to int
		want     []string
	}{
		{"往右挪一格", 0, 2, []string{"b", "a", "c", "d"}},
		{"往右挪两格", 0, 3, []string{"b", "c", "a", "d"}},
		{"往左挪", 3, 1, []string{"a", "d", "b", "c"}},
		{"挪到最末", 0, 4, []string{"b", "c", "d", "a"}},
		{"挪到最前", 3, 0, []string{"d", "a", "b", "c"}},
		// to == from（插到自己前面）和 to == from+1（插到自己后面）都是原地
		// 不动：这两个落点说的就是「现在这样」。
		{"插在自己前面", 1, 1, []string{"a", "b", "c", "d"}},
		{"插在自己后面", 1, 2, []string{"a", "b", "c", "d"}},
		// 越界的落点原样返回，而不是把某一页弄丢。
		{"落点越界", 1, 9, []string{"a", "b", "c", "d"}},
		{"来源越界", 9, 1, []string{"a", "b", "c", "d"}},
	}
	for _, tc := range cases {
		got := reorder([]string{"a", "b", "c", "d"}, tc.from, tc.to)
		if len(got) != len(tc.want) {
			t.Errorf("%s：长度 %d，想要 %d（%q）", tc.name, len(got), len(tc.want), got)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s：%q，想要 %q", tc.name, got, tc.want)
				break
			}
		}
	}
}

// moveTab 挪的是顺序，不是选择：活动的那一页跟着它自己走（按指针认回来），
// 而且新的顺序会落进 store —— 下一次启动看到的就是拖完的样子。
func TestMoveTabKeepsActiveAndPersistsOrder(t *testing.T) {
	a := newApp()
	repo1 := filepath.Join(t.TempDir(), "repo1")
	repo2 := filepath.Join(t.TempDir(), "repo2")
	repo3 := filepath.Join(t.TempDir(), "repo3")
	a.openInNewTab(repo1)
	a.openInNewTab(repo2)
	a.openInNewTab(repo3)

	// 回到第一页，再把第一页拖到最末。
	a.switchTab(0)
	active := a.currentTab()
	a.moveTab(0, 3)

	if len(a.tabs) != 3 {
		t.Fatalf("挪完还有 %d 页，想要 3 页", len(a.tabs))
	}
	if got := []string{a.tabs[0].path, a.tabs[1].path, a.tabs[2].path}; got[0] != repo2 || got[1] != repo3 || got[2] != repo1 {
		t.Errorf("顺序是 %q，想要 [%s %s %s]", got, repo2, repo3, repo1)
	}
	if a.activeTab != 2 {
		t.Errorf("活动页下标是 %d，想要 2", a.activeTab)
	}
	if a.currentTab() != active {
		t.Error("挪完之后活动页换人了 —— 挪顺序不该改选择")
	}
	if got := a.store.State().OpenRepos; len(got) != 3 || got[2] != repo1 {
		t.Errorf("存下来的打开顺序是 %q，想要末位是 %s", got, repo1)
	}
	// 再挪回来一次：从最末拖到最前。
	a.moveTab(2, 0)
	if got := a.tabs[0].path; got != repo1 {
		t.Errorf("挪回最前后第一页是 %s，想要 %s", got, repo1)
	}
	if a.currentTab() != active || a.activeTab != 0 {
		t.Errorf("挪回最前后活动页是下标 %d（%v），想要下标 0 上的那一页", a.activeTab, a.currentTab() == active)
	}
}

// 项目行拖一下就能换位置：按住第一项拖到第三项的下半段，它就插到最末。
// 这是拿走鼠标那一下真会发生的事 —— reorder 算得对不代表接得上（落点怎么
// 算、Drop 落在哪一行上，都是另一码事）。
func TestProjectRowsDragToReorder(t *testing.T) {
	a := newApp()
	repo1 := filepath.Join(t.TempDir(), "repo1")
	repo2 := filepath.Join(t.TempDir(), "repo2")
	repo3 := filepath.Join(t.TempDir(), "repo3")
	a.openInNewTab(repo1)
	a.openInNewTab(repo2)
	a.openInNewTab(repo3)
	// 回到第一页：拖它的才是「第一页」。
	a.switchTab(0)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	first, ok := tt.Find("repo1")
	if !ok {
		t.Fatalf("顶栏上找不到 repo1 那一页：%q", tt.Texts())
	}
	last, ok := tt.Find("repo3")
	if !ok {
		t.Fatalf("顶栏上找不到 repo3 那一页：%q", tt.Texts())
	}
	// 落到最后一行的下半段 = 插到它后面。
	dragBetween(tt, first.X+first.W/2, first.Y+first.H/2, last.X+last.W/2, last.Y+last.H-3)
	tt.Frame()

	got := []string{a.tabs[0].path, a.tabs[1].path, a.tabs[2].path}
	if got[0] != repo2 || got[1] != repo3 || got[2] != repo1 {
		t.Fatalf("拖完顺序是 %q，想要 [%s %s %s]", got, repo2, repo3, repo1)
	}
	if a.currentTab().path != repo1 {
		t.Errorf("拖完活动页是 %s，想要 repo1 —— 拖动不该改选择", a.currentTab().path)
	}
}

// 终端芯片也一样拖得动，而且拖的是顺序不是「现在看哪个 shell」。
func TestTerminalChipsDragToReorder(t *testing.T) {
	stubTerminal(t)

	a := newApp()
	a.openInNewTab(filepath.Join(t.TempDir(), "repo1"))
	a.setSection(sectionTerminal)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	tab := a.currentTab()
	a.newTerminalSession(tab)
	a.newTerminalSession(tab)
	a.newTerminalSession(tab)
	tt.Frame()
	if len(tab.terms) != 3 {
		t.Fatalf("该有三个会话，实际 %d", len(tab.terms))
	}
	// 看的是第一枚，把第一枚拖到最末。
	tab.termActive = 0
	active := tab.terms[0]
	tt.Frame()

	first, ok := tt.Find("终端 1")
	if !ok {
		t.Fatalf("芯片带上找不到「终端 1」：%q", tt.Texts())
	}
	last, ok := tt.Find("终端 3")
	if !ok {
		t.Fatalf("芯片带上找不到「终端 3」：%q", tt.Texts())
	}
	dragBetween(tt, first.X+first.W/2, first.Y+first.H/2, last.X+last.W-3, last.Y+last.H/2)
	tt.Frame()

	if len(tab.terms) != 3 {
		t.Fatalf("拖完还剩 %d 枚芯片，想要 3 枚", len(tab.terms))
	}
	if tab.terms[0].ord != 2 || tab.terms[1].ord != 3 || tab.terms[2].ord != 1 {
		t.Errorf("拖完的顺序是 ord %d,%d,%d，想要 2,3,1",
			tab.terms[0].ord, tab.terms[1].ord, tab.terms[2].ord)
	}
	if tab.termActive != 2 || tab.terms[2] != active {
		t.Errorf("拖完看的是第 %d 枚（是不是当初那一枚：%v），想要最末那枚",
			tab.termActive, tab.termActive >= 0 && tab.termActive < len(tab.terms) && tab.terms[tab.termActive] == active)
	}
}

// 四选一那一段：section 是「视图模式 + 改动/历史」折出来的下标，⌘1~⌘4 与
// 工具栏那四格走的是同一条路。
func TestSectionsSwitchToolbarAndKeys(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)

	if got := a.section(); got != sectionChanges {
		t.Fatalf("开局该在改动，实际 %d", got)
	}
	tt.Key(ui.Cmd, ui.Key2)
	if a.section() != sectionHistory || a.viewMode != viewModeGit || a.tab != 1 {
		t.Errorf("⌘2 之后：section=%d viewMode=%d tab=%d，想要历史 / Git 面板 / 1",
			a.section(), a.viewMode, a.tab)
	}
	tt.Key(ui.Cmd, ui.Key3)
	if a.section() != sectionTerminal {
		t.Errorf("⌘3 之后 section=%d，想要终端", a.section())
	}
	tt.Key(ui.Cmd, ui.Key4)
	if a.section() != sectionEditor {
		t.Errorf("⌘4 之后 section=%d，想要编辑器", a.section())
	}
	// 从编辑器回改动：走的是工具栏那一格，不是快捷键。
	if err := tt.Click("改动"); err != nil {
		t.Fatalf("工具栏上点不动「改动」：%v", err)
	}
	if a.section() != sectionChanges || a.viewMode != viewModeGit {
		t.Errorf("点完「改动」：section=%d viewMode=%d，想要改动 / Git 面板", a.section(), a.viewMode)
	}
	// Ctrl+~ 从终端切回 Git 面板：回到的还是刚才那一页（历史），不是被重置
	// 成改动 —— 四格并成一条之后，底下那两个字段各自还记着「回 Git 面板时
	// 看哪一页」（见 section 上那段）。
	tt.Key(ui.Cmd, ui.Key2)
	tt.Key(ui.Cmd, ui.Key3)
	if a.section() != sectionTerminal {
		t.Fatalf("切到终端之后 section=%d", a.section())
	}
	tt.Key(ui.Ctrl, ui.KeyBackquote)
	if a.viewMode != viewModeGit || a.tab != 1 || a.section() != sectionHistory {
		t.Errorf("从终端切回来：viewMode=%d tab=%d section=%d，想要 Git 面板 / 1 / 历史",
			a.viewMode, a.tab, a.section())
	}
	// 工具栏上那两格也点得动。
	if err := tt.Click("改动"); err != nil {
		t.Fatalf("工具栏上点不动「改动」：%v", err)
	}
	if a.section() != sectionChanges {
		t.Errorf("点完「改动」section=%d，想要改动", a.section())
	}
	if err := tt.Click("历史"); err != nil {
		t.Fatalf("工具栏上点不动「历史」：%v", err)
	}
	if a.section() != sectionHistory {
		t.Errorf("点完「历史」section=%d，想要历史", a.section())
	}
	// 并进来的那两格（终端 / 编辑器）同样点得动 —— 它们以前不在这一条上，
	// 只有快捷键这一条路，并进来之后鼠标也得走得通。
	if err := tt.Click("终端"); err != nil {
		t.Fatalf("工具栏上点不动「终端」：%v", err)
	}
	if a.section() != sectionTerminal || a.viewMode != viewModeTerm {
		t.Errorf("点完「终端」：section=%d viewMode=%d，想要终端 / 终端", a.section(), a.viewMode)
	}
	if err := tt.Click("编辑器"); err != nil {
		t.Fatalf("工具栏上点不动「编辑器」：%v", err)
	}
	if a.section() != sectionEditor || a.viewMode != viewModeEditor {
		t.Errorf("点完「编辑器」：section=%d viewMode=%d，想要编辑器 / 编辑器", a.section(), a.viewMode)
	}
}

// 项目行拖动：落点按整张列表算，压在行与行之间的缝里也照样能插进去 ——
// 不必先碰到某一条提示线。拖动过程中行序不动，松手才落位；落点处浮出
// 一块浅色空位，告诉人松手会插到哪儿。
func TestProjectRowsDropAtGapAndEnd(t *testing.T) {
	a := newApp()
	repo1 := filepath.Join(t.TempDir(), "repo1")
	repo2 := filepath.Join(t.TempDir(), "repo2")
	repo3 := filepath.Join(t.TempDir(), "repo3")
	a.openInNewTab(repo1)
	a.openInNewTab(repo2)
	a.openInNewTab(repo3)
	a.switchTab(0)

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	// 1) 把第一页拖到最后一页下面：落在行外的空白里也算「最末」。
	first, ok := tt.Find("repo1")
	if !ok {
		t.Fatalf("找不到 repo1 那一页：%q", tt.Texts())
	}
	last, ok := tt.Find("repo3")
	if !ok {
		t.Fatalf("找不到 repo3 那一页：%q", tt.Texts())
	}
	tt.Press(first.X+first.W/2, first.Y+first.H/2)
	tt.Frame()
	tt.Move(last.X+last.W/2, last.Y+last.H-4)
	tt.Frame()
	tt.Release(last.X+last.W/2, last.Y+last.H-4)
	tt.Frame()
	tt.Frame()

	if got := []string{a.tabs[0].path, a.tabs[1].path, a.tabs[2].path}; got[0] != repo2 || got[1] != repo3 || got[2] != repo1 {
		t.Fatalf("拖到末尾之后顺序是 %q，想要 [%s %s %s]", got, repo2, repo3, repo1)
	}
	if a.currentTab().path != repo1 {
		t.Errorf("拖完活动页是 %s，想要 repo1 —— 拖动不该改选择", a.currentTab().path)
	}

	// 2) 再把它拖到第二、三行之间的缝里：插到第三行前面。
	// 先重新量一遍行，顺序已经变了。
	first, _ = tt.Find("repo2")
	moved, ok := tt.Find("repo1")
	if !ok {
		t.Fatalf("找不到 repo1 那一页：%q", tt.Texts())
	}
	// repo2 下沿与 repo3 上沿之间那 2px 的缝。
	gapY := first.Y + first.H + 1
	tt.Press(moved.X+moved.W/2, moved.Y+moved.H/2)
	tt.Frame()
	tt.Move(moved.X+moved.W/2, gapY)
	tt.Frame()
	tt.Release(moved.X+moved.W/2, gapY)
	tt.Frame()
	tt.Frame()

	if got := []string{a.tabs[0].path, a.tabs[1].path, a.tabs[2].path}; got[0] != repo2 || got[1] != repo1 || got[2] != repo3 {
		t.Fatalf("落在两行之间的缝里顺序是 %q，想要 [%s %s %s]", got, repo2, repo1, repo3)
	}

	// 松手才落盘：重开 store 看到的就是拖完的顺序。
	if got := a.store.State().OpenRepos; len(got) != 3 || got[1] != repo1 {
		t.Errorf("落盘顺序是 %q，想要第二位是 %s", got, repo1)
	}
}

// pasteConn 冒充终端的对端：记下写进来的一切字节。
//
// 图片粘贴的断言要看「程序到底收到了什么」—— 插件把按键编成字节写进 pty，
// 这一份就是 pty 那头。
type pasteConn struct {
	mu   sync.Mutex
	buf  []byte
	done chan struct{}
	once sync.Once
}

func newPasteConn() *pasteConn { return &pasteConn{done: make(chan struct{})} }

func (c *pasteConn) Read([]byte) (int, error) { <-c.done; return 0, io.EOF }

func (c *pasteConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	return len(p), nil
}

func (c *pasteConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return nil
}

func (c *pasteConn) Resize(cols, rows int) error { return nil }

// waitPaste 等到对端收到包含 want 的字节（写输入走插件自己的 goroutine，
// 只能等，不能拍一帧就断言）。
func waitPaste(t *testing.T, c *pasteConn, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := string(c.buf)
		c.mu.Unlock()
		if strings.Contains(got, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.mu.Lock()
	got := string(c.buf)
	c.mu.Unlock()
	t.Fatalf("对端没收到 %q，实际收到 %q", want, got)
}

// TestTerminalPasteForwardsImageKey：剪贴板上是图片（没有文本）时，⌘V 要把
// Ctrl+V 补发给前台程序 —— AI CLI（Claude Code 等）的「粘贴图片」是自己读
// 系统剪贴板的，等的就是这个按键；插件的粘贴分支只会把文本送进去，图片这一
// 下会被整键吃掉。有文本时仍走原来的 Bracketed Paste，Ctrl+V 也仍旧直达到
// 程序，两条老路都不许变。
func TestTerminalPasteForwardsImageKey(t *testing.T) {
	conn := newPasteConn()
	t.Cleanup(func() { conn.Close() })
	old := terminalNew
	terminalNew = func(o terminal.Options) (*terminal.Terminal, error) {
		o.Conn = conn
		return terminal.New(o)
	}
	t.Cleanup(func() { terminalNew = old })

	a := newApp()
	a.openInNewTab(t.TempDir())
	a.setViewMode(viewModeTerm)
	a.newTerminalSession(a.currentTab())
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Frame()

	// Ctrl+V 老路：直达到程序，一个字节都不多包。
	tt.Key(ui.Ctrl, ui.KeyV)
	waitPaste(t, conn, "\x16")

	// 图片（空文本）+ ⌘V：补发 Ctrl+V。
	tt.SetClipboard("")
	tt.Key(ui.Super, ui.KeyV)
	waitPaste(t, conn, "\x16")

	// 文本 + ⌘V：照旧把文本送进程序，不补发按键。
	conn.mu.Lock()
	conn.buf = nil
	conn.mu.Unlock()
	tt.SetClipboard("pasted text")
	tt.Key(ui.Super, ui.KeyV)
	waitPaste(t, conn, "pasted text")
	conn.mu.Lock()
	got := string(conn.buf)
	conn.mu.Unlock()
	if strings.Contains(got, "\x16") {
		t.Errorf("有文本时不该补发 Ctrl+V，实际写了 %q", got)
	}
}

// 切换项目只换工作区，不碰各项目自己的终端会话。
//
// 会话挂在 repoTab 上（见 repoTab.terms），saveActiveTab / restoreActiveTab
// 都不动它；这条测试把这份约定钉死：切走再切回来，会话个数、顺序和指针
// 都必须原样，不能像重开项目那样重新起 shell。
func TestSwitchProjectKeepsTerminals(t *testing.T) {
	a := richApp(t)
	a.openInNewTab(a.repoPath)
	other := committedRepoDir(t)
	a.openInNewTab(other)
	settle(t, a)

	first := a.tabs[0]
	a.newTerminalSession(first)
	a.newTerminalSession(first)
	kept := []*termSession{first.terms[0], first.terms[1]}
	first.termActive = 1

	a.switchTab(1)
	settle(t, a)
	a.switchTab(0)
	settle(t, a)

	if got := len(first.terms); got != 2 {
		t.Fatalf("切走再切回后终端会话该还是 2 个，实际 %d 个", got)
	}
	if first.terms[0] != kept[0] || first.terms[1] != kept[1] {
		t.Fatalf("切项目后终端会话被换掉了：%+v，want %+v", first.terms, kept)
	}
	if first.termActive != 1 {
		t.Errorf("切回项目后该还停在第二个会话，实际激活 %d", first.termActive)
	}
}

// 拖芯片时芯片带已经是预览：被拖的那枚画在将要落下的位置，tab.terms 还
// 没动；松手才提交。项目标签页那边也有同样的一段。
func TestDraggingATerminalChipPreviewsItsPlace(t *testing.T) {
	stubTerminal(t)
	a := newApp()
	a.openInNewTab(filepath.Join(t.TempDir(), "repo1"))
	a.setSection(sectionTerminal)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	tab := a.currentTab()
	for i := 0; i < 3; i++ {
		a.newTerminalSession(tab)
	}
	tt.Frame()
	tab.termActive = 0
	tt.Frame()

	first, ok := tt.Find("终端 1")
	if !ok {
		t.Fatalf("找不到第一枚芯片：%q", tt.Texts())
	}
	last, ok := tt.Find("终端 3")
	if !ok {
		t.Fatalf("找不到第三枚芯片：%q", tt.Texts())
	}
	tt.Press(first.X+first.W/2, first.Y+first.H/2)
	for k := 1; k <= 5; k++ {
		f := float32(k) / 5
		tt.Move(first.X+first.W/2+(last.X+last.W-first.X-first.W/2)*f, first.Y+first.H/2)
		tt.Frame()
	}

	// 预览：芯片一已经画到最右，但会话列表还是原来的顺序。
	moved, _ := tt.Find("终端 1")
	third, _ := tt.Find("终端 3")
	if moved.X < third.X {
		t.Fatalf("拖动中芯片一在 x=%.1f，想要它已经到最右（芯片三在 %.1f）", moved.X, third.X)
	}
	ords := []int{tab.terms[0].ord, tab.terms[1].ord, tab.terms[2].ord}
	if ords[0] != 1 || ords[1] != 2 || ords[2] != 3 {
		t.Fatalf("拖动还没松手，会话顺序已经变成 %v", ords)
	}

	tt.Release(last.X+last.W-4, last.Y+last.H/2)
	tt.Frame()
	if tab.terms[0].ord != 2 || tab.terms[1].ord != 3 || tab.terms[2].ord != 1 {
		t.Fatalf("松手后的顺序是 %d,%d,%d，想要 2,3,1",
			tab.terms[0].ord, tab.terms[1].ord, tab.terms[2].ord)
	}
}

// Escape 放下一次芯片拖动：预览收回，顺序不动。
func TestEscapePutsATerminalChipBack(t *testing.T) {
	stubTerminal(t)
	a := newApp()
	a.openInNewTab(filepath.Join(t.TempDir(), "repo1"))
	a.setSection(sectionTerminal)
	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()

	tab := a.currentTab()
	for i := 0; i < 3; i++ {
		a.newTerminalSession(tab)
	}
	tt.Frame()
	tab.termActive = 0
	tt.Frame()

	first, _ := tt.Find("终端 1")
	last, _ := tt.Find("终端 3")
	tt.Press(first.X+first.W/2, first.Y+first.H/2)
	for k := 1; k <= 5; k++ {
		f := float32(k) / 5
		tt.Move(first.X+first.W/2+(last.X+last.W-first.X-first.W/2)*f, first.Y+first.H/2)
		tt.Frame()
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Release(last.X+last.W-4, last.Y+last.H/2)
	tt.Frame()

	ords := []int{tab.terms[0].ord, tab.terms[1].ord, tab.terms[2].ord}
	if ords[0] != 1 || ords[1] != 2 || ords[2] != 3 {
		t.Fatalf("Escape 之后顺序变成了 %v", ords)
	}
	back, _ := tt.Find("终端 1")
	second, _ := tt.Find("终端 2")
	if back.X > second.X {
		t.Fatalf("Escape 之后芯片一还在 x=%.1f，没有回到芯片二左边（%.1f）", back.X, second.X)
	}
}
