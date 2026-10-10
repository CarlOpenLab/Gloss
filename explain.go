package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/explain"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// ✨ 解释卡片的尺寸与节奏。
const (
	cardMaxW = 460 // 卡片最大宽度
	cardMaxH = 460 // 卡片最大高度
	cardGap  = 10  // 卡片和 ✨ 按钮之间的间距
	cardMinH = 180 // 上方/下方空间再小也要给这么高

	// cardRadius 是卡片的圆角。§3 把「悬浮卡片、AI 解释卡片」归到 lg(12)。
	cardRadius = radiusLG

	// cardMotion 是卡片出现/消失的时长。
	//
	// 160ms：比系统弹窗（200ms 上下）略快 —— 卡片是读代码时随手叫出来的，
	// 慢吞吞地飘进来会挡着正在看的那几行。再快就只剩「闪一下」，看不出是从
	// 按钮那儿出来的。
	cardMotion = 160 * time.Millisecond

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
	// fail 是这次失败整理过的说法（见 llm.Describe）。零值表示没出错 ——
	// 判断「出没出错」看 Title 是不是空的。
	fail llm.Failure
	// needKey 表示这次没发出请求，因为压根没有可用的凭证 —— 卡片这时
	// 讲的是「先去配一个模型」，不是「解释失败了」。
	needKey bool
	cancel  context.CancelFunc

	// mail 是正在进行的这次请求的信箱。它是独立对象而不是 explainState
	// 的内嵌字段，因为 onDelta 在后台 goroutine 上跑：整块替换 explainState
	// 会把 mutex 也换掉，和后台正在进行的加锁撞上。换一个指针就没这问题，
	// 而且指针本身就是「还是不是同一次请求」的凭据。
	mail *explainMail

	scroll ui.ScrollState

	// 单行与选区解释
	isSelection bool   // 是否为单行/选区解释
	lineNum     int    // 行号
	lineCode    string // 代码内容
	question    string // 用户提问/追问
	promptInput string // 追问输入框的文本
	focusPrompt bool   // 是否聚焦追问输入框
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
	if !a.explainOpen() || a.ex.isSelection || fi < 0 || fi >= len(a.files) || hi < 0 || hi >= len(a.files[fi].Hunks) {
		return false
	}
	return a.ex.key == a.explainKey(a.files[fi], hi)
}

// isExplainingSelection 判断某个文件某一行是不是当前正在解释的选区。
func (a *app) isExplainingSelection(fi, lineNum int) bool {
	if !a.explainOpen() || !a.ex.isSelection || fi < 0 || fi >= len(a.files) {
		return false
	}
	return a.ex.fi == fi && a.ex.lineNum == lineNum
}

// toggleSelectionExplain 点代码行上的 ✨ 按钮时调用：开着就关掉，关着就打开。
func (a *app) toggleSelectionExplain(fi int, lineStr string, lineNum int, anchor ui.Rect) {
	if a.isExplainingSelection(fi, lineNum) {
		a.closeExplain()
		return
	}
	a.openSelectionExplain(fi, lineStr, lineNum, "", anchor)
}

// openSelectionExplain 打开针对单行或选区代码的解释卡片：有缓存就直接显示，没有就发起流式请求。
func (a *app) openSelectionExplain(fi int, lineStr string, lineNum int, question string, anchor ui.Rect) {
	if fi < 0 || fi >= len(a.files) {
		return
	}
	a.cancelExplain()

	f := a.files[fi]
	key := explain.SelectionCacheKey(f.Path(), lineStr, question, a.modelName)
	head := fmt.Sprintf("%s : 第 %d 行", displayPath(f), lineNum)
	trimmed := strings.TrimSpace(lineStr)
	if trimmed != "" {
		if len(trimmed) > 36 {
			trimmed = trimmed[:36] + "…"
		}
		head = fmt.Sprintf("第 %d 行: %s", lineNum, trimmed)
	}

	a.ex = explainState{
		fi:          fi,
		hi:          -1,
		key:         key,
		head:        head,
		anchor:      anchor,
		isSelection: true,
		lineNum:     lineNum,
		lineCode:    lineStr,
		question:    question,
		promptInput: question,
	}

	if text, ok := a.store.Lookup(a.ex.key); ok {
		a.ex.text = text
		a.ex.cached = true
		return
	}
	a.startSelectionExplain(f.FileDiff, lineStr, lineNum, question, fileKey(f))
}

// startSelectionExplain 起一个后台请求解释特定代码行/选区。
func (a *app) startSelectionExplain(f diff.FileDiff, lineStr string, lineNum int, question string, contentKey string) {
	if !a.store.Provider().Ready() {
		a.ex.loading = false
		a.ex.needKey = true
		return
	}
	a.ex.needKey = false

	var lines []string
	if cc := a.contents[contentKey]; cc != nil {
		lines = cc.lines
	}
	system, user := explain.BuildSelectionPrompt(explain.SelectionRequest{
		File:     f,
		Code:     lineStr,
		LineNum:  lineNum,
		Question: question,
		Context:  explain.LineWindow(lines, lineNum, explainAround, explainMaxCtx),
	})

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
				return
			}
			a.flushExplain(mail)
			a.ex.loading = false
			if err != nil {
				if ctx.Err() == nil {
					a.ex.fail = llm.Describe(err)
				}
				return
			}
			a.ex.text = strings.TrimSpace(text)
			a.store.Put(key, a.ex.text)
			a.persist()
		})
	})
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
	a.startExplain(f.FileDiff, h, hi, fileKey(f))
}

// startExplain 起一个后台请求。参数全是值 —— 后台 goroutine 绝不碰 app。
//
// contentKey 是文件在 a.contents 里的键（fileKey，带段落前缀）。不能拿
// f.Path() 顶替：同一个路径在「已暂存 / 未暂存」两段里是两份内容，拿纯路径
// 去查会命中另一段的那份 —— 已暂存段就会把工作区的代码当上下文发给模型。
func (a *app) startExplain(f diff.FileDiff, h diff.Hunk, hi int, contentKey string) {
	// 连凭证都没有：这一趟发出去必然失败，而且是失败在离代码十万八千里的
	// 地方（SDK 会回一大段英文，讲它找过哪些凭证来源）。与其把那串东西
	// 摆在用户面前，不如直接说清楚该去哪儿配 —— 卡片上会给一个按钮。
	if !a.store.Provider().Ready() {
		a.ex.loading = false
		a.ex.needKey = true
		return
	}
	a.ex.needKey = false

	// 提示词在主线程拼：要用到已经加载好的文件内容，而且是纯计算，很快。
	var lines []string
	if cc := a.contents[contentKey]; cc != nil {
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
					a.ex.fail = llm.Describe(err)
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
	if a.ex.isSelection {
		if a.ex.fi < 0 || a.ex.fi >= len(a.files) {
			a.closeExplain()
			return
		}
	} else if !a.isExplaining(a.ex.fi, a.ex.hi) {
		a.closeExplain()
		return
	}
	winW, winH := c.Size()

	w := min(cardMaxW, winW-24)
	at := placeCard(a.ex.anchor, winW, winH, w)

	panel := ui.Box(c)
	ui.Overlay(c, func() {
		panel = ui.Box(c).Absolute().Left(at.x).Width(w).MaxHeight(at.maxH).
			// 卡片是浮层：Surface 3 + 一道 hairline（§2.1），投影只留一点收边。
			Radius(cardRadius).Background(pal.surface3).
			Border(1, pal.hairline).
			Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.16))
		if at.up {
			// 往上排时钉住下沿：内容短的话卡片贴着按钮，不会浮在半空
			panel.Bottom(winH - at.y)
		} else {
			panel.Top(at.y)
		}
		panel.Children(func() {
			a.explainHead(c, pal)
			a.explainBody(c, pal, at.maxH)
			if a.ex.isSelection {
				a.explainPromptBar(c, pal)
			}
			a.explainFoot(c, pal)
		})

		// 出现和消失都从按钮那一侧滑出来一点点。
		//
		// 方向跟着卡片摆哪走：排在下面就从上方落下来，排在上面就从下方
		// 升上去 —— 位移始终是「从 ✨ 按钮那里出来」，眼睛能顺着这条线
		// 把卡片和它所属的那段改动连起来。8px 就够了，多了像弹窗。
		//
		// 卡片每帧重建，但它在 Overlay 里位置固定，ID 不变，所以起手的
		// 那一帧能认出来是「新出现的」；窗口大小变化不触发动画（框架
		// 自己的约定）。
		//
		// Position 必须关掉。框架默认三项（位置、尺寸、颜色）全动，而卡片
		// 的位置每帧都从 ✨ 按钮重新算 —— 滚一下 diff、按钮挪了半屏，卡片
		// 就会「飘」着追过去：指针按下去时按钮在这儿，松开时它还在半路上，
		// 这一次点击就落空了。关掉它，滚动时卡片和按钮是钉在一起走的，
		// 只在出现/消失那一下滑 8px（Enter/Exit 的 Motion 不受这一项管）。
		//
		// 尺寸同理：解释是流式吐出来的，卡片高度跟着长，动起来就成了
		// 每 50ms 一次的抽搐。颜色留着 —— 换主题时卡片跟着一起淡过去。
		away := float32(8)
		if at.up {
			away = -8
		}
		panel.Transition(ui.ElementTransition{
			Duration: cardMotion,
			Colors:   true, // 只开这一项：不开就等于「三项全开」
			Enter:    &ui.Motion{Y: away, Opacity: 0},
			Exit:     &ui.Motion{Y: away, Opacity: 0},
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
	// 头部和正文靠明度分界，不叠强调色（§5 禁止装饰性染色）：深色下头部
	// 比卡片低一档，浅色下比卡片灰一档，两种模式里它都是一条「带子」。
	ui.Column(c).Shrink(0).Padding(10, 12).Gap(4).
		Background(pal.surface2).
		// 圆角只给上面两角，和卡片一致：头部贴着卡片顶边，四角都给就会
		// 把下面两个角也切掉，和正文之间出现两道缺口。
		Radius(cardRadius, cardRadius, 0, 0).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			sparkleCluster(c, 15, pal.inkMuted)
			title := "AI 解释"
			if a.ex.isSelection {
				title = "AI 单行解释"
			}
			ui.Text(c, title).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
			if a.ex.cached {
				// 「缓存」说的是这条解释的来源，不是状态告警 —— 一枚安静的
				// 规范芯片就够，靠不着强调色。
				specChip(c, pal, "缓存", ui.Transparent, statusChipH, statusChipText)
			}
			ui.Spacer(c)
			if closeButton(c, pal) {
				a.closeExplain()
			}
		})
		ui.Text(c, a.ex.head).Font("monospace").FontSize(textCaption).
			SingleLine().TextColor(pal.inkSubtle)
		if a.ex.question != "" {
			ui.Text(c, "追问: "+a.ex.question).FontSize(textCaption).
				SingleLine().TextColor(pal.inkMuted)
		}
	})
}

// explainBody 是卡片正文：流式解释，可滚动。
func (a *app) explainBody(c *ui.Context, pal palette, maxH float32) {
	t := c.Theme()
	// 头尾各占一块，剩下的给正文。这里给个保守的估算就够了 ——
	// 真算出精确值要等布局，而这是绝对定位的元素，量不到。
	bodyH := maxH - 96
	if a.ex.isSelection {
		bodyH -= 38
	}
	if bodyH < 60 {
		bodyH = 60
	}

	ui.Box(c).MaxHeight(bodyH).MinHeight(60).Shrink(1).Children(func() {
		ui.Scroll(c).TrackScroll(&a.ex.scroll).Children(func() {
			ui.Column(c).Padding(10, 12).Children(func() {
				a.explainText(c, t, pal)
			})
		})
	})
}

// explainText 是正文的三种状态：出错、空、有内容。
func (a *app) explainText(c *ui.Context, t *ui.Theme, pal palette) {
	switch {
	case a.ex.fail.Title != "":
		ui.Text(c, a.ex.fail.Title).Bold().FontSize(12).TextColor(t.Danger)
		if h := a.ex.fail.Hint; h != "" {
			ui.Text(c, h).FontSize(12).TextColor(t.TextMuted)
		}
		// 供应商自己的原话放最后、最小，而且是等宽的：它是一句机器输出，
		// 不是我们的话。翻译不准的时候，证据在这儿。
		if d := a.ex.fail.Detail; d != "" {
			ui.Text(c, d).Font("monospace").FontSize(11).TextColor(t.TextMuted)
		}
		cardActions(c, func() {
			// 钥匙不对的时候，能动手改的是设置里的那行；重试是给「改在
			// 别处了（环境变量、供应商控制台）」留的路。两颗都摆出来，
			// 设置那颗做成主按钮 —— 它才是这个错的正解。
			if a.ex.fail.Auth {
				if primaryAction(c, pal, "模型设置…", "", false).Clicked() {
					a.openSettings()
				}
			}
			if secondaryAction(c, pal, "重试", "", false).Clicked() {
				a.retryExplain()
			}
		})
		return

	case a.ex.needKey:
		ui.Text(c, "还没有配置模型").Bold().FontSize(12)
		ui.Text(c, "解释代码要接一个模型，填上 API key 就能用了。").
			FontSize(12).TextColor(t.TextMuted)
		cardActions(c, func() {
			if primaryAction(c, pal, "模型设置…", "", false).Clicked() {
				a.openSettings()
			}
		})
		return

	case a.ex.text == "":
		if a.ex.loading {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				msg := "正在读这段改动…"
				if a.ex.isSelection {
					msg = "正在分析代码…"
				}
				ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		ui.Text(c, "没有内容").FontSize(12).TextColor(t.TextMuted)
		return
	}

	a.mdBlocks(c, mdStyleFor(t, pal))
}

// cardActions 是卡片正文底下那一行动作按钮。
//
// 上面留 8 —— 和段落间距取同一个数（见 mdBlockGap），卡片里「一块和一块
// 之间」的距离就这一个值。不给就是零：正文的字底直接压着按钮的色块。
//
// 用 Row 收住，是因为 Column 默认横向拉满：直接摆按钮，它会变成一条横贯
// 卡片的色带 —— 那是个默认值，不是个决定。设置对话框、空状态里按钮都是
// 它自己的宽度，卡片没道理另开一种。
const cardActionGap = 8

func cardActions(c *ui.Context, body func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(cardActionGap, 0, 0, 0).Children(body)
}

// retryExplain 重来一次：还问同一个变更块，但绕开缓存里那份失败。
//
// 直接 startExplain 而不是 openExplain：失败的结果不落缓存，重新问本来就
// 走的是这条路，这里只是把它收成一个名字，好让几个按钮都指过来。
func (a *app) retryExplain() {
	if a.ex.isSelection {
		if a.ex.fi < 0 || a.ex.fi >= len(a.files) {
			return
		}
		f := a.files[a.ex.fi]
		a.ex.text = ""
		a.ex.scroll = ui.ScrollState{}
		a.ex.fail = llm.Failure{}
		a.ex.needKey = false
		a.startSelectionExplain(f.FileDiff, a.ex.lineCode, a.ex.lineNum, a.ex.question, fileKey(f))
		return
	}
	if a.ex.fi < 0 || a.ex.fi >= len(a.files) || a.ex.hi < 0 || a.ex.hi >= len(a.files[a.ex.fi].Hunks) {
		return
	}
	f := a.files[a.ex.fi]
	// 上一趟已经吐出来的半截话要清掉：连接断了也会先把收到的那段交给
	// 界面（那是对的 —— 断线前的内容不该丢），可它已经不是这次的答案了，
	// 不清的话新答案会接在这半句话后面。
	a.ex.text = ""
	a.ex.scroll = ui.ScrollState{}
	a.ex.fail = llm.Failure{}
	a.ex.needKey = false
	a.startExplain(f.FileDiff, f.Hunks[a.ex.hi], a.ex.hi, fileKey(f))
}

// explainPromptBar 是选区解释卡片底部的追问输入框。
func (a *app) explainPromptBar(c *ui.Context, pal palette) {
	if !a.ex.isSelection {
		return
	}
	ui.Row(c).Shrink(0).Padding(6, 12).Gap(6).AlignItems(ui.Center).
		Background(pal.surface2).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
		in := ui.TextInput(c, &a.ex.promptInput).Placeholder("输入追问或关注重点…").Grow(1)
		if a.ex.focusPrompt {
			in.Focus()
			a.ex.focusPrompt = false
		}
		btn := secondaryAction(c, pal, "提问", "", false)
		if (in.Submitted() || btn.Clicked()) && !a.ex.loading {
			q := strings.TrimSpace(a.ex.promptInput)
			if q != "" {
				a.openSelectionExplain(a.ex.fi, a.ex.lineCode, a.ex.lineNum, q, a.ex.anchor)
			}
		}
	})
}

// mdMarkerW 是列表符号那一栏的宽度。
//
// 定宽右对齐，而不是让符号自己撑：「•」和「10.」宽度差一倍，跟着内容走的话
// 每一项的正文起点都不一样，一列读下来是锯齿。
const mdMarkerW = 18

// mdDotH 是无序列表那个圆点所在方盒的高度，取正文一行的高度。
//
// 13px 正文量出来的行距是 16（两行之间的基线距离）。圆点画在这一行的正中间，
// 而不是让布局去居中：圆点盒比一行矮，它撑不高行，位置就由这里说了算。
const mdDotH = 16

// mdMarker 画列表项前面那一栏：无序的是圆点，有序的是编号。
//
// 圆点自己画，不用字体的「•」：那个字形在中文环境下落在一个比正文低、
// 也比正文重的位置上（CJK 字体的中圆点是按全角方格居中的），一列读下来
// 像一串挂着的小石子。和箭头、叉号一样，这种几何形状自己画才控得住。
func (a *app) mdMarker(c *ui.Context, st mdStyle, marker string) {
	if marker != "•" {
		ui.Text(c, marker).FontSize(13).Width(mdMarkerW).
			TextAlign(ui.End).TextColor(st.bullet)
		return
	}
	ui.Box(c).Width(mdMarkerW).Height(mdDotH).Shrink(0).
		Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			path.Circle(r.X+r.W/2, r.Y+r.H/2, 2.5)
			p.FillPath(&path, st.bullet)
		})
}

// mdBlocks 把解释渲染成一块块元素。
//
// 不用一个 RichText 装下全文：那样段落一折行就顶到最左边，列表项的后续行
// 会和符号对齐，代码块也只有贴着字的底色。见 markdown.go 顶上的说明。
func (a *app) mdBlocks(c *ui.Context, st mdStyle) {
	blocks := mdBlocks(a.ex.text, st)
	for i, b := range blocks {
		a.mdBlock(c, st, b, mdBlockGap(blocks, i))
	}
}

// mdBlockGap 是这一块和上一块之间留的空。
//
// 列表项之间收窄（同属一组的几行该挨着），其余段落之间放宽 —— 段与段的
// 界线一眼看得出来，卡片才不像一坨字。
func mdBlockGap(blocks []mdBlock, i int) float32 {
	if i == 0 {
		return 0
	}
	if blocks[i].kind == mdBullet && blocks[i-1].kind == mdBullet {
		return 3
	}
	return 8
}

func (a *app) mdBlock(c *ui.Context, st mdStyle, b mdBlock, gap float32) {
	switch b.kind {
	case mdCode:
		// 代码块有自己的底、内边距和圆角 —— 一片贴字的灰底认不出是代码块
		ui.Box(c).Margin(gap, 0, 0, 0).Padding(7, 9).Radius(radiusSM).
			Background(st.codeBg).Children(func() {
			ui.RichText(c, ui.Span{Text: b.code, Font: st.mono, Color: st.code}).
				FontSize(12)
		})

	case mdBullet:
		// 符号和正文分两栏：折行自然落在正文那一栏里（悬挂缩进）
		ui.Row(c).Margin(gap, 0, 0, 0).AlignItems(ui.Start).Gap(6).Children(func() {
			a.mdMarker(c, st, b.marker)
			ui.RichText(c, b.spans...).Grow(1).MinWidth(0).FontSize(13)
		})

	case mdQuote:
		// 左边一条竖线，而不是行首一个「▏」字符：字符会跟着折行跑到第二行
		// 的行首，看着像正文里混进了一个怪符号
		ui.Row(c).Margin(gap, 0, 0, 0).AlignItems(ui.Start).Gap(8).Children(func() {
			// 圆角取宽度的一半，竖线两端才是圆的 —— 这是几何，不是 §3 的
			// 角 token，所以写出来而不是套一个 token。
			ui.Box(c).Width(2).Shrink(0).Radius(1).AlignSelf(ui.Stretch).
				Background(st.bullet.Alpha(0.4))
			ui.RichText(c, b.spans...).Grow(1).MinWidth(0).FontSize(13)
		})

	default:
		// 段落和标题：spans 里已经带了粗体和颜色
		ui.RichText(c, b.spans...).Margin(gap, 0, 0, 0).FontSize(13)
	}
}

// explainFoot 是卡片底部：已 review 勾选。
func (a *app) explainFoot(c *ui.Context, pal palette) {
	if a.ex.fi >= len(a.files) {
		return
	}
	f := &a.files[a.ex.fi]
	viewed := a.isViewed(*f)

	ui.Row(c).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.hairline).Children(func() {
		v := viewed
		ui.Checkbox(c, &v, "已 review")
		if v != viewed {
			a.toggleViewed(*f)
		}
		ui.Spacer(c)
		if a.ex.loading {
			ui.Text(c, "生成中…").FontSize(textCaption).TextColor(pal.inkSubtle)
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
		// 列表符号用卡片自己的强调色，不用 pal.ref：后者是「分支引用」的
		// 意思（提交列表的徽章、侧栏的引用、文件行的状态），一颗提亮的点
		// 落在 AI 卡片里会让人以为和某条分支有关。强调色是这张卡的头和
		// 火花在用的一色 —— 深色下是白、浅色下是黑，和卡片底色永远拉得开。
		bullet: t.Accent,
		mono:   "monospace",
	}
}
