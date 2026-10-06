package main

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"diff-explainer/internal/diff"
	"diff-explainer/internal/explain"
	"diff-explainer/internal/gitrepo"
	"diff-explainer/internal/llm"
)

// ✨ 解释卡片的尺寸与节奏。
const (
	cardMaxW = 460 // 卡片最大宽度
	cardMaxH = 460 // 卡片最大高度
	cardGap  = 10  // 卡片和 ✨ 按钮之间的间距
	cardMinH = 180 // 上方/下方空间再小也要给这么高

	// explainFlush 是流式输出的节流间隔。
	//
	// 每个 token 都回主线程重绘会把界面拖垮：一次回答几百个 delta，
	// 每个都触发一整帧的布局和绘制。攒够 50ms 再刷一次，肉眼仍是逐字出现。
	explainFlush = 50 * time.Millisecond

	// explainAround 是给模型的上下文里，hunk 前后各取多少行。
	explainAround = 40
	// explainMaxCtx 是上下文文本的字节上限，超了按行截断。
	explainMaxCtx = 6000
)

// explainState 是 ✨ 卡片的状态。
//
// 它整体挂在 app 上而不是元素上：卡片每帧重建，元素上的状态活不过一帧；
// 而且流式输出的收尾发生在后台 goroutine 回到主线程之后，那时元素早就没了。
//
// 线程约定见 app.go：只有主线程能读写这里的字段。唯一会跨线程的是
// *explainMail —— 后台 goroutine 收到的流式片段先投进信箱，主线程再取。
type explainState struct {
	// 打开的是哪个变更块。fi/hi 是 a.files 里的下标，key 是解释缓存的 key ——
	// 下标会因为筛选、刷新而错位，key 用来判断这份状态还成不成立。
	fi   int
	hi   int
	key  string
	head string // hunk 头，卡片上显示「在解释哪一段」

	// anchor 是 ✨ 按钮上一帧的位置。每帧从按钮重新取，所以滚动、
	// 改窗口大小之后卡片会跟着走。
	anchor ui.Rect

	text    string
	loading bool
	cached  bool // 内容来自缓存，不是这次问出来的
	err     string
	cancel  context.CancelFunc

	// mail 是正在进行的这次请求的信箱。它是独立对象而不是 explainState
	// 的内嵌字段，因为 onDelta 在后台 goroutine 上跑：整块替换 explainState
	// 会把 mutex 也换掉，和后台正在进行的加锁撞上。换一个指针就没这问题，
	// 而且指针本身就是「还是不是同一次请求」的凭据。
	mail *explainMail

	scroll ui.ScrollState
}

// explainMail 是流式片段的中转站：后台只写，主线程按节流取。
type explainMail struct {
	mu   sync.Mutex
	buf  strings.Builder
	last time.Time
}

// fill 收下一个片段，并报告这次是否到了该刷新界面的时间。
func (m *explainMail) fill(chunk string) (due bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf.WriteString(chunk)
	if time.Since(m.last) < explainFlush {
		return false
	}
	m.last = time.Now()
	return true
}

// take 取走攒下的片段。
func (m *explainMail) take() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.buf.String()
	m.buf.Reset()
	return s
}

func (a *app) explainOpen() bool { return a.ex.key != "" }

// isExplaining 判断某个变更块是不是当前打开的那个。
func (a *app) isExplaining(fi, hi int) bool {
	if !a.explainOpen() || fi < 0 || fi >= len(a.files) || hi < 0 || hi >= len(a.files[fi].Hunks) {
		return false
	}
	return a.ex.key == a.explainKey(a.files[fi], hi)
}

func (a *app) explainKey(f gitrepo.File, hi int) string {
	return explain.CacheKey(f.FileDiff, f.Hunks[hi], a.modelName)
}

// toggleExplain 点 ✨ 时调用：开着就关掉，关着就打开。
func (a *app) toggleExplain(fi, hi int) {
	if a.isExplaining(fi, hi) {
		a.closeExplain()
		return
	}
	a.openExplain(fi, hi)
}

// openExplain 打开卡片：有缓存就直接显示，没有就发起一次流式请求。
func (a *app) openExplain(fi, hi int) {
	if fi < 0 || fi >= len(a.files) || hi < 0 || hi >= len(a.files[fi].Hunks) {
		return
	}
	a.cancelExplain()

	f := a.files[fi]
	h := f.Hunks[hi]
	a.ex = explainState{
		fi:   fi,
		hi:   hi,
		key:  a.explainKey(f, hi),
		head: hunkHeaderText(h),
	}

	if text, ok := a.store.Lookup(a.ex.key); ok {
		a.ex.text = text
		a.ex.cached = true
		return
	}
	a.startExplain(f.FileDiff, h, hi)
}

// startExplain 起一个后台请求。参数全是值 —— 后台 goroutine 绝不碰 app。
func (a *app) startExplain(f diff.FileDiff, h diff.Hunk, hi int) {
	// 提示词在主线程拼：要用到已经加载好的文件内容，而且是纯计算，很快。
	var lines []string
	if cc := a.contents[f.Path()]; cc != nil {
		lines = cc.lines
	}
	system, user := explain.BuildPrompt(explain.Request{
		File:    f,
		Hunk:    h,
		HunkIdx: hi,
		Context: explain.Window(lines, h, explainAround, explainMaxCtx),
	})

	// ctx 和信箱都在主线程建好，立刻挂到状态上 —— 否则用户在请求返回之前
	// 关掉卡片，就没人去取消它了。
	ctx, cancel := context.WithCancel(context.Background())
	a.ex.cancel = cancel
	a.ex.loading = true
	mail := &explainMail{last: time.Now()}
	a.ex.mail = mail

	key := a.ex.key
	prov := a.store.Provider()

	onDelta := func(chunk string) {
		if mail.fill(chunk) {
			a.dispatch(func() { a.flushExplain(mail) })
		}
	}

	a.run(func() {
		text, err := llm.StreamExplanation(ctx, prov, system, user, onDelta)
		a.dispatch(func() {
			if a.ex.key != key {
				return // 已经换到别的变更块了，这次的结果丢掉
			}
			a.flushExplain(mail)
			a.ex.loading = false
			if err != nil {
				if ctx.Err() == nil { // 自己取消的不算错误
					a.ex.err = err.Error()
				}
				return
			}
			a.ex.text = strings.TrimSpace(text)
			a.store.Put(key, a.ex.text)
			a.persist()
		})
	})
}

// flushExplain 把后台攒下的流式片段搬到界面上。只在主线程调用。
//
// mail 必须还是当前这一次请求的信箱：用户切到别的变更块之后，旧请求的
// 尾包还可能飘过来，不认一下就会把上一个 hunk 的解释接在当前卡片后面。
func (a *app) flushExplain(mail *explainMail) {
	if mail != a.ex.mail {
		return
	}
	pending := mail.take()
	if pending == "" {
		return
	}
	a.ex.text += pending

	// 只在本来就贴着底部时跟着滚 —— 用户往上翻看前面的内容时别打扰他。
	if a.ex.scroll.Y >= a.ex.scroll.MaxY-48 {
		a.ex.scroll.Y = math.MaxFloat32
	}
}

// closeExplain 关闭卡片并掐掉进行中的请求。
func (a *app) closeExplain() {
	a.cancelExplain()
	a.ex = explainState{}
}

func (a *app) cancelExplain() {
	if a.ex.cancel != nil {
		a.ex.cancel()
		a.ex.cancel = nil
	}
}

// explainCard 画解释卡片。
//
// 它在 Overlay 层里，所以画在 diff 之上、不受列表裁剪 —— 和 Web 版的
// 悬浮卡片是一个意思。没有做「点外面关掉」的全屏遮罩：遮罩会挡住底下的
// 滚动和点击，而读 diff 的时候正是要一边看一边滚。关闭走 Esc、再点一次
// ✨、或者卡片上的 ×。
func (a *app) explainCard(c *ui.Context, pal palette) {
	if !a.explainOpen() {
		return
	}
	// 刷新仓库或改筛选之后，fi/hi 可能已经指向别的文件了 —— 那时卡片上的
	// 「已 review」会勾到别的文件上，不如直接收起。
	if !a.isExplaining(a.ex.fi, a.ex.hi) {
		a.closeExplain()
		return
	}
	t := c.Theme()
	winW, winH := c.Size()

	w := min(cardMaxW, winW-24)
	at := placeCard(a.ex.anchor, winW, winH, w)

	panel := ui.Box(c)
	ui.Overlay(c, func() {
		panel = ui.Box(c).Absolute().Left(at.x).Width(w).MaxHeight(at.maxH).
			Radius(t.Radius+4).Background(t.Background).
			Border(1, pal.border).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.22))
		if at.up {
			// 往上排时钉住下沿：内容短的话卡片贴着按钮，不会浮在半空
			panel.Bottom(winH - at.y)
		} else {
			panel.Top(at.y)
		}
		panel.Children(func() {
			a.explainHead(c, pal)
			a.explainBody(c, pal, at.maxH)
			a.explainFoot(c, pal)
		})
	})
}

// cardPlacement 是卡片该摆哪儿。
type cardPlacement struct {
	// x 是左边缘；y 在 up 为假时是上边缘，为真时是下边缘 ——
	// 贴下沿摆才能保证内容短的时候卡片紧挨着按钮，而不是浮在半空。
	x, y, maxH float32
	// up 表示排在 ✨ 按钮上方（下方放不下而且上方更宽敞）。
	up bool
}

// placeCard 算卡片的位置和最大高度：默认排在 ✨ 按钮下方、右边缘和按钮
// 对齐；下方放不下而上方更宽敞时翻到上方。两边都夹在窗口内。
func placeCard(anchor ui.Rect, winW, winH, w float32) cardPlacement {
	at := cardPlacement{x: clamp(anchor.X+anchor.W-w, 12, max(12, winW-w-12))}

	below := winH - (anchor.Y + anchor.H) - cardGap - 12
	above := anchor.Y - cardGap - 12
	if below < 240 && above > below {
		at.up = true
		at.maxH = min(cardMaxH, max(cardMinH, above))
		at.y = anchor.Y - cardGap
		return at
	}
	at.maxH = min(cardMaxH, max(cardMinH, below))
	at.y = anchor.Y + anchor.H + cardGap
	return at
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// explainHead 是卡片顶部：标题、缓存标记、关闭按钮，下面一行是 hunk 头。
func (a *app) explainHead(c *ui.Context, pal palette) {
	t := c.Theme()
	ui.Column(c).Shrink(0).Padding(10, 12).Gap(4).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "✨ AI 解释").Bold().FontSize(13)
			if a.ex.cached {
				ui.Text(c, "缓存").FontSize(10).Padding(1, 6).Radius(4).
					Background(t.Surface).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if closeButton(c) {
				a.closeExplain()
			}
		})
		ui.Text(c, a.ex.head).Font("monospace").FontSize(11).
			SingleLine().TextColor(t.TextMuted)
	})
}

// explainBody 是卡片正文：流式解释，可滚动。
func (a *app) explainBody(c *ui.Context, pal palette, maxH float32) {
	t := c.Theme()
	// 头尾各占一块，剩下的给正文。这里给个保守的估算就够了 ——
	// 真算出精确值要等布局，而这是绝对定位的元素，量不到。
	bodyH := maxH - 96
	if bodyH < 60 {
		bodyH = 60
	}

	ui.Box(c).MaxHeight(bodyH).MinHeight(60).Shrink(1).Children(func() {
		ui.Scroll(c).TrackScroll(&a.ex.scroll).Children(func() {
			ui.Column(c).Padding(10, 12).Gap(2).Children(func() {
				a.explainText(c, t, pal)
			})
		})
	})
}

// explainText 是正文的三种状态：出错、空、有内容。
func (a *app) explainText(c *ui.Context, t *ui.Theme, pal palette) {
	switch {
	case a.ex.err != "":
		ui.Text(c, "解释失败了").Bold().FontSize(12).TextColor(t.Danger)
		ui.Text(c, a.ex.err).FontSize(12).TextColor(t.TextMuted)
		if ui.Button(c, "重试").Clicked() {
			f := a.files[a.ex.fi]
			a.startExplain(f.FileDiff, f.Hunks[a.ex.hi], a.ex.hi)
		}
		return

	case a.ex.text == "":
		if a.ex.loading {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在读这段改动…").FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		ui.Text(c, "没有内容").FontSize(12).TextColor(t.TextMuted)
		return
	}

	ui.RichText(c, mdSpans(a.ex.text, mdStyleFor(t, pal))...).FontSize(13)
}

// explainFoot 是卡片底部：已 review 勾选。
func (a *app) explainFoot(c *ui.Context, pal palette) {
	if a.ex.fi >= len(a.files) {
		return
	}
	f := &a.files[a.ex.fi]
	viewed := a.isViewed(*f)

	ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Children(func() {
		v := viewed
		ui.Checkbox(c, &v, "已 review")
		if v != viewed {
			a.toggleViewed(*f)
		}
		ui.Spacer(c)
		if a.ex.loading {
			ui.Text(c, "生成中…").FontSize(11).TextColor(c.Theme().TextMuted)
		}
	})
}

// mdStyleFor 从主题里取 Markdown 渲染要用的颜色。
func mdStyleFor(t *ui.Theme, pal palette) mdStyle {
	return mdStyle{
		text:   t.Text,
		muted:  t.TextMuted,
		code:   pal.code,
		codeBg: pal.gapBg,
		head:   t.Text,
		bullet: pal.ref,
		mono:   "monospace",
	}
}
