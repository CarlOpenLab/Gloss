package main

import (
	"io"
	"path/filepath"
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

	// 进终端视图：自动补一个会话，并在这一帧把它起起来。
	a.setViewMode(viewModeTerm)
	tt.Frame()
	if len(tab.terms) != 1 || tab.termActive != 0 {
		t.Fatalf("进终端视图该有一个会话：%d 个，激活 %d", len(tab.terms), tab.termActive)
	}
	if *spawns != 1 {
		t.Fatalf("进终端视图该起一次 shell，实际 %d 次", *spawns)
	}
	if !tt.HasText("终端 1") {
		t.Errorf("空标题该回落到「终端 1」，实际 %q", tt.Texts())
	}

	// 多渲染几帧不该重复起：一枚芯片就是一个 shell。
	tt.Frame()
	tt.Frame()
	if *spawns != 1 {
		t.Errorf("渲染多帧只该起一次 shell，实际 %d 次", *spawns)
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

	// 再关两下：倒数第二个关掉之后剩一个，最后一个关掉时自动补一个新的。
	for range 2 {
		if err := tt.Click("关闭终端标签页"); err != nil {
			t.Fatalf("找不到关闭按钮：%v（%q）", err, tt.Texts())
		}
		if len(tab.terms) != 1 {
			t.Fatalf("关会话后该一直有且只有一个会话可用，实际 %d", len(tab.terms))
		}
	}
	if tab.terms[0].ord != 4 {
		t.Errorf("关掉最后一个该补一个编号继续往后发的新会话（ord 4），实际 %d", tab.terms[0].ord)
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
