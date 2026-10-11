package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// TestApplyModelArmWatcher：每次仓库加载完成，都要把自动刷新盯到这个仓库、
// 并重对齐基线 —— 这是「外部一改就自动刷，应用自己重读不白刷」的支点。
func TestApplyModelArmWatcher(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	a := loadAppFrom(t, dir)
	if root, _ := a.watchRoot.Load().(string); root != dir {
		t.Fatalf("打开仓库后 watchRoot 应指向仓库根 %q，实际 %q", dir, root)
	}
	v0 := a.watchVer.Load()
	// 主动 reload 一次（模拟 ⌘R / 写操作后的重读），基线该跟着重对齐。
	a.reload()
	settle(t, a)
	if v := a.watchVer.Load(); v <= v0 {
		t.Fatalf("reload 完成后 watchVer 应递增，v0=%d v=%d", v0, v)
	}
}

// loadAppFrom 建一个空的 git 仓库并加载（不造填充内容的前置仓库：
// 自动刷新关注的是「打开仓库后 watcher 挂没挂」，不需要 diff 内容）。
func loadAppFrom(t *testing.T, dir string) *app {
	t.Helper()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return loadApp(t, dir)
}

// TestAutoRefreshLoopDetectsExternalChange 盯的是这条链路本身：仓库没动时
// 不该乱刷，被外部改动（编辑器里存盘、别的终端里提交）之后轮询要自己发现
// 并 dispatch 一趟 reload —— 其余几条测的是「挂没挂上」「让不让路」，
// 都不覆盖它。
func TestAutoRefreshLoopDetectsExternalChange(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	a := loadAppFrom(t, dir)

	// 轮询只在 main.go 里启动（测试不启动，settle 才不会等一个永不退出的
	// goroutine）。这里手动起、手动停。
	//
	// 中途不能停下来重启：新起的 goroutine 第一趟是基线对齐，它会拿「当时
	// 的指纹」当基线 —— 那样改动在重启里就被悄悄认下了，后面再也扫不出来。
	// 所以整条测试用同一个循环跑到底。
	a.startAutoRefresh()
	defer a.stopAutoRefresh()

	// 等头两三个 tick（2s 一档）：第一档做基线对齐，之后的档次指纹和基线
	// 一致，一趟都不该发。顺带把可能排进队列的闭包执行掉再断言 —— 不然
	// 一次莫须有的刷新会一直躺在队列里，等下面改写文件时才被 drain 出来，
	// 看着像「扫到了改动」，其实是个假阳性。
	seq := a.repoSeq
	time.Sleep(autoRefreshInterval*2 + 500*time.Millisecond)
	a.loop.drain()
	if a.repoSeq != seq {
		t.Fatal("仓库没动时轮询不该触发 reload")
	}

	// 外部改动：另一个进程往工作区写了个新文件。
	writeFile(t, filepath.Join(dir, "external.txt"), "saved from an editor\n")

	// 轮询自己该发现它并 dispatch 一趟 reload。
	waitForReload(t, a, 15*time.Second, seq)
}

// waitForReload 等到 repoSeq 变了（= autoReloadNow 在主线程上跑过一次），
// 期间不停把 dispatch 出来的闭包投回主线程执行：轮询跑在自己的 goroutine
// 上、经 a.dispatch 跳回主线程，而测试的 dispatch 只是入队 —— 没人 drain
// 的话 autoReloadNow 永远排在队列里，界面也就永远不会重读。
func waitForReload(t *testing.T, a *app, d time.Duration, seq int) {
	t.Helper()
	deadline := time.Now().Add(d)
	for a.repoSeq == seq {
		a.loop.drain()
		if time.Now().After(deadline) {
			t.Fatal("外部改动后轮询应触发一次 reload，但 repoSeq 没变")
		}
		time.Sleep(20 * time.Millisecond)
	}
	a.loop.drain()
}

// TestAutoReloadNowGating：自动刷新触发时，写操作在飞或刚刷过就该让路，
// 只有真正闲下来才整体重读。
func TestAutoReloadNowGating(t *testing.T) {
	dir := t.TempDir()
	a := loadAppFrom(t, dir)

	// 写操作在飞（比如正在暂存）：不该打断重读。
	seq := a.repoSeq
	a.op = gitrepo.OpStage
	a.autoReloadNow()
	if a.repoSeq != seq {
		t.Fatal("写操作进行中时自动刷新不应触发 reload")
	}
	a.op = gitrepo.OpNone

	// 正常情况：触发一次整体重读。
	seq = a.repoSeq
	a.autoReloadedAt = time.Time{}
	a.autoReloadNow()
	if a.repoSeq == seq {
		t.Fatal("没有写操作时自动刷新应触发 reload")
	}
	settle(t, a)

	// 刚刷过（在冷却窗口内）：不再来一趟。
	seq = a.repoSeq
	a.autoReloadNow()
	if a.repoSeq != seq {
		t.Fatal("冷却窗口内的再次触发不应重复 reload")
	}
}
