package main

import (
	"context"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 这个文件管「git 状态自动刷新」：不再等用户按 ⌘R，改完文件、提交、
// 切分支、fetch 之后界面自己跟上。
//
// 做法是单个后台 goroutine 每 autoRefreshInterval 对当前激活的仓库算一次
// 状态指纹（gitrepo.StateFingerprint），和基线一比，不一样就 dispatch 一趟
// reload。基线的重对齐点只有一处：setAutoWatch（每次模型重读都走它）——
// 应用自己的写操作、手动 ⌘R 结束后的重读都会把基线推到「刚读到的这份」，
// 于是同一份变化只会被自动刷新处理一次，不会和 reloadAfterOp 双重刷新。
//
// 线程约定：这个 goroutine 不碰 app 字段，只读 watchRoot/watchVer/autoFetchOn
// 这几个 atomic；触发刷新通过 a.dispatch 回到主线程（见 app.go 顶上的四条约定）。
// 永不退出，所以不能登记进 a.bg（测试的 settle 会等它等到超时）——它只在
// main.go 里启动，测试不启动，也就没有这个问题。
//
// 同一个 goroutine 还兼「后台自动抓取」（见 autoFetchInterval）：开关打开时
// 每隔几分钟对所有远程跑一次 git fetch。它跑在这个线程上是安全的 —— 抓取
// 只动远程跟踪引用，不碰工作区和 index，和主线程上的写操作（暂存/提交/检出）
// 互不干扰；真撞上 index.lock 那也是别人的操作在等 git 自己的锁。

// autoRefreshInterval 是轮询间隔。2 秒对「改完 ~2 秒内跟上」够快，对没动
// 过的仓库只是一趟 git status + show-ref，代价可忽略。
const autoRefreshInterval = 2 * time.Second

// autoFetchInterval 是后台自动抓取的间隔。
//
// 5 分钟：远程分支的变化本来就以「别人推了东西」为节奏，抓太勤只是白跑
// 进程；抓取的成果是侧栏那几行和超前/落后计数，晚一会儿看见不要紧。
const autoFetchInterval = 5 * time.Minute

// autoReloadMinGap 是主线程上两次自动 reload 的最小间隔（防刷新风暴）。
const autoReloadMinGap = time.Second

// syncAutoWatch 切 tab 之后把自动刷新盯到新激活的仓库上。
//
// 切 tab 恢复的是标签页里存下的一整份快照，不经过 applyModel（不像
// openRepo 那样会再接一次 setAutoWatch），所以这里要单独把根换成新 tab
// 的、并重对齐基线。成功加载过的 tab 存的 path 就是仓库根；没开好的
// （loadFailed 等）就不盯。
func (a *app) syncAutoWatch() {
	root := ""
	if tab := a.currentTab(); tab != nil && tab.state == loadReady {
		root = tab.path
	}
	a.setAutoWatch(root)
}

// startAutoRefresh 在 produce 里由 main.go 调用，起后台轮询。
func (a *app) startAutoRefresh() {
	if a.autoStop != nil {
		return
	}
	stop := make(chan struct{})
	a.autoStop = stop
	// 通道按值传进去，循环只碰自己那份：a.autoStop 这个字段归主线程，
	// 轮询 goroutine 读它就是个数据竞争（race detector 会盯上）。
	go a.autoRefreshLoop(stop)
}

// stopAutoRefresh 停掉轮询（窗口关闭时调用）。
func (a *app) stopAutoRefresh() {
	if a.autoStop != nil {
		close(a.autoStop)
		a.autoStop = nil
	}
}

// setAutoWatch 把「要自动刷新的仓库根」换成 root（"" = 不盯），并让轮询
// 重新对齐基线。只在主线程调用：applyModel、failOpen、切 tab 之后。
//
// 基线重对齐靠 watchVer 自增：轮询 goroutine 发现账号变了，下一 tick 就
// 重新 Open 仓库、把当前指纹当作新的基线，不会把「自己刚重读回来的状态」
// 错当成一次外部变化再刷一遍。
func (a *app) setAutoWatch(root string) {
	a.watchRoot.Store(root)
	a.watchVer.Add(1)
}

// autoRefreshLoop 是后台轮询循环：每档 tick 算一次当前激活仓库的指纹，
// 和基线不同就 dispatch 一趟 reload。
//
// stop 由 startAutoRefresh 按值传进来，除了它，goroutine 只读两个 atomic、
// 只通过 a.dispatch 回主线程，不碰任何 app 字段 —— 这是这个文件头上说的
// 线程约定：轮询线程和主线程之间的共享状态必须是有同步的那几种。
func (a *app) autoRefreshLoop(stop <-chan struct{}) {
	ctx := context.Background()
	ticker := time.NewTicker(autoRefreshInterval)
	defer ticker.Stop()

	// 已经对齐到的版本号；-1 表示还没看过任何东西。
	var seenVer int32 = -1
	var repo *gitrepo.Repo
	var lastFP string
	// 这个仓库有没有远程。一条远程都没有的仓库（本地新建的那种）不抓 ——
	// git fetch --all 对它是静默退 0，白起一个进程。
	hasRemotes := false
	// 上一次自动抓取的时间。零值时第一次 tick 就抓（开关刚打开那一下），
	// 之后按 autoFetchInterval 走。
	var lastFetch time.Time

	for {
		select {
		case <-ticker.C:
			root, _ := a.watchRoot.Load().(string)
			if root == "" {
				seenVer = -1
				repo = nil
				lastFP = ""
				hasRemotes = false
				continue
			}
			if ver := a.watchVer.Load(); ver != seenVer {
				// 仓库（或它刚被重读）换了，先对齐：重新量一遍仓库根和
				// .git 目录，以当前指纹为基线。
				seenVer = ver
				r, err := gitrepo.Open(ctx, root)
				if err != nil {
					repo = nil
					lastFP = ""
					hasRemotes = false
					continue
				}
				repo = r
				lastFP = r.StateFingerprint(ctx)
				// 有没有远程跟着版本一起问：换仓库时 remotes 会变，而
				// 「抓不抓」每 tick 都要用它。
				remotes, err := r.Remotes(ctx)
				hasRemotes = err == nil && len(remotes) > 0
				continue
			}
			if repo == nil {
				continue
			}
			fp := repo.StateFingerprint(ctx)
			if fp != lastFP {
				// 变化交给主线程去整体重读。这里不认领指纹 —— 认领只发生在
				// applyModel 完成后的基线重对齐（watchVer++）上：那样写操作
				// 在飞/正在加载/刚刷过而让路的变化，下一 tick 会再扫到并补上，
				// 不会白白吞掉；自己 reload 完也不会因为这个变化再刷第二遍。
				a.dispatch(a.autoReloadNow)
			}
			// 后台自动抓取。放在指纹比对之后：本地有变化的那一两秒里用户
			// 正在等界面刷新，别让一个可能跑几秒的 fetch 插在同一 tick 上。
			//
			// 抓取就在这个线程上跑，不另起一个：它是每 5 分钟一次的事，
			// 代价是那几秒里指纹也跟着停（自动刷新晚几秒）—— 换成一个
			// goroutine 加一把「在飞」的标记，只为这一点延迟不值当。
			//
			// 失败不重试、不报错：下一档（5 分钟后）自然再来一次。真需要
			// 知道为什么没抓到，手动抓一次就有说法（工具栏那颗、⌘⇧F）。
			if a.autoFetchOn.Load() == 1 && hasRemotes && time.Since(lastFetch) >= autoFetchInterval {
				lastFetch = time.Now()
				if err := repo.Fetch(ctx); err == nil {
					a.dispatch(a.autoFetched)
				}
			}
		case <-stop:
			return
		}
	}
}

// autoFetched 是后台抓取成功之后 dispatch 到主线程的收尾。
//
// 只重读引用，和手动「抓取」一个路子（见 doFetch）：抓取不动工作区也不动
// index，主区那份 diff 一个字节都没变，reload 会把它闪一下。要的是侧栏
// 的远程分支和每个本地分支的 ↑/↓ 计数跟上。
//
// 仓库可能在这一会儿换了（fetch 跑了半天）：只认当前还开着同一个根的那
// 一次，别的直接丢。
func (a *app) autoFetched() {
	if !a.repoReady() {
		return
	}
	a.loadRefs()
}

// autoReloadNow 是 dispatch 到主线程的自动刷新入口。
//
// 这里拦三种情况就不 reload：有写操作在飞（暂存/提交到一半），正在加载
// （openRepo 已经在读了），以及刚自动刷过不到一秒。其它情况直接整体重读
// —— openRepo 对同一仓库保留滚动位置和展开状态，刷新不会把界面弹回开头。
func (a *app) autoReloadNow() {
	if a.op != gitrepo.OpNone {
		return
	}
	if a.state == loadLoading || !a.repoReady() {
		return
	}
	if time.Since(a.autoReloadedAt) < autoReloadMinGap {
		return
	}
	a.autoReloadedAt = time.Now()
	a.reload()
}
