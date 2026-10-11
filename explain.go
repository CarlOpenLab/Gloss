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

	// scope 是这次解释的粒度：hunk 头的 ✨ 按钮讲整段，双击代码行讲那一行。
	// 两套粒度共用同一张卡片，只有提示词和缓存 key 不同。
	scope explainScope
	// line 是单行解释指的那一行（scope 为 scopeLine 时有效）：行号和内容，
	// 加上它在 hunk.Lines 里的下标 —— 行号会随刷新漂移，下标才是定位凭据。
	line explainLine
	// question 是单行解释的追问。它不是 app 的字段而是卡片的：追问是「就着
	// 这次解释往下问」，切到另一行/另一段就该重新开始问。
	question string

	// askOn 是追问输入框这一帧是否该抢焦点。和 filepane 的 focusFilter 一个
	// 套路：卡片在覆盖层里构建，和别的输入框抢焦点，置位晚一帧才轮到它。
	askOn bool

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

	// 折叠/收缩状态（GitHub 风格折叠）
	collapsed bool

	// 多次解析历史与翻页卡片
	entries []explainEntry
	curIdx  int
}

// explainEntry 是一条解释记录（支持卡片翻页与历史留存）。
type explainEntry struct {
	ID        string
	Head      string
	Text      string
	Cached    bool
	CreatedAt time.Time
}

// explainScope 是这次解释盯的粒度：整段变更块，还是某一行。
type explainScope uint8

const (
	// scopeHunk 是 hunk 头的 ✨ 按钮那一套：讲整段改了什么。
	scopeHunk explainScope = iota
	// scopeLine 是双击代码行那一套：只讲这一行。
	scopeLine
)

// explainLine 是被单独解释的那一行。
//
// 行号会随刷新、切提交而漂移，所以定位认 hunk 下标 + 行在 hunk 里的下标
// （和 a.ex.fi / a.ex.hi 一个道理）；no 和 text 只是拿来显示和拼提示词的
// 快照，每帧从行模型重新取，不用它倒推位置。
type explainLine struct {
	// lineIdx 是被解释的行在 hunk.Lines 里的下标。
	lineIdx int
	// no 是它在文件里的行号（新侧优先，纯删除的行用旧侧）。0 表示定位失败。
	no int
	// side 是 no 取自哪一侧：删除行在新侧没有行号，说的时候得讲清楚。
	side diff.LineType
	// text 是这一行的内容，去掉行首那个 +/-/空格 标记。
	text string
}

// scopeTag 是卡片角标上的粒度说明：用户得知道这一趟讲的是「一段」还是「一行」。
func (e explainLine) tag() string {
	if e.no <= 0 {
		return ""
	}
	old := e.side == diff.LineDel
	return Tf("第 %d 行", "Line %d", e.no) + sideMark(old)
}

// sideMark 标出这一行取自旧侧还是新侧。空串表示说不清，此时别硬凑一个字。
func sideMark(old bool) string {
	if old {
		return T("（旧）", " (old)")
	}
	return T("（新）", " (new)")
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

// isExplainingLine 判断这一行是不是当前打开的那个单行解释。
//
// lineIdx 是定位凭据（行号会漂），所以要的是「同一段的同一行」而不是「同一个行号」。
func (a *app) isExplainingLine(fi, hi, lineIdx int) bool {
	if !a.explainOpen() || a.ex.scope != scopeLine {
		return false
	}
	return a.ex.fi == fi && a.ex.hi == hi && a.ex.line.lineIdx == lineIdx
}

// explainStillValid 判断卡片还指不指着还在的东西。
//
// 刷新、切文件之后 fi/hi 可能已经落到别的条目上（下标会漂，key 不会），
// 这时卡片讲的就是别的东西了 —— 每次画卡片都先问一句，不成立就收掉。
// 两种粒度分别对：hunk 那一套对 fi+hi，单行那一套再多对一行 lineIdx。
func (a *app) explainStillValid() bool {
	if !a.explainOpen() {
		return false
	}
	if a.ex.fi < 0 || a.ex.fi >= len(a.files) {
		return false
	}
	if a.ex.hi < 0 || a.ex.hi >= len(a.files[a.ex.fi].Hunks) {
		return false
	}
	if a.ex.scope == scopeLine {
		li := a.ex.line.lineIdx
		return li >= 0 && li < len(a.files[a.ex.fi].Hunks[a.ex.hi].Lines)
	}
	return true
}

// explainKey 是这次解释的缓存 key。
//
// 同一行有多个 hunk 时，hunkIdx 也进 key：不同段里的同一行不是同一件事
// （一段在改它，另一段只是它旁边变了），缓存共享会把两段的解释混成一份。
func (a *app) explainKey(f gitrepo.File, hi int) string {
	if a.ex.scope != scopeLine {
		return explain.CacheKey(f.FileDiff, f.Hunks[hi], a.modelName)
	}
	return explain.SelectionCacheKey(explain.Path(f.FileDiff), a.ex.line.text, a.ex.question, a.modelName)
}

// toggleExplain 点 ✨ 时调用：开着就关掉，关着就打开。
func (a *app) toggleExplain(fi, hi int) {
	if a.isExplaining(fi, hi) {
		a.closeExplain()
		return
	}
	a.openExplain(fi, hi)
}

// toggleExplainLine 双击代码行时调用：开着同一行就关掉，否则打开那一行。
func (a *app) toggleExplainLine(fi, hi, lineIdx int) {
	if a.isExplainingLine(fi, hi, lineIdx) {
		a.closeExplain()
		return
	}
	a.openExplainLine(fi, hi, lineIdx)
}

// openExplain 打开卡片：有缓存就直接显示，没有就发起一次流式请求。
func (a *app) openExplain(fi, hi int) {
	if fi < 0 || fi >= len(a.files) || hi < 0 || hi >= len(a.files[fi].Hunks) {
		return
	}
	a.cancelExplain()

	f := a.files[fi]
	h := f.Hunks[hi]
	// scope 先落上、key 后算：explainKey 读 a.ex.scope 决定用哪套算法，
	// 在字面量里一起写等于拿**旧**卡片的状态算新 key —— 上一张是单行
	// 卡片时，这里会算出一个单行的 key 来。
	a.ex = explainState{
		fi:    fi,
		hi:    hi,
		head:  hunkHeaderText(h),
		scope: scopeHunk,
	}
	a.ex.key = a.explainKey(f, hi)

	if text, ok := a.store.Lookup(a.ex.key); ok {
		a.ex.text = text
		a.ex.cached = true
		a.recordExplainEntry(text, true)
		a.rowsDirty = true
		return
	}
	a.rowsDirty = true
	a.startExplain(f.FileDiff, h, hi, fileKey(f))
}

// openExplainLine 只看某一行：要讲清「这一行在干什么、为什么这么写」。
//
// 它复用同一张卡片，所以和 hunk 那颗按钮共享暂存区、已 review 那一类
// 栏脚开关 —— 换粒度不该换掉卡片本身。
func (a *app) openExplainLine(fi, hi, lineIdx int) {
	if fi < 0 || fi >= len(a.files) || hi < 0 || hi >= len(a.files[fi].Hunks) {
		return
	}
	h := a.files[fi].Hunks[hi]
	if lineIdx < 0 || lineIdx >= len(h.Lines) {
		return
	}
	a.cancelExplain()

	f := a.files[fi]
	line := explainLine{
		lineIdx: lineIdx,
		no:      lineNum(h.Lines[lineIdx]),
		side:    h.Lines[lineIdx].Type,
		text:    strings.TrimLeft(h.Lines[lineIdx].Content, "+-"),
	}
	// head 留着 hunk 头：卡片要用它说「在讲哪个文件哪一段」，单行只多加
	// 一行角标说第几行 —— 光有「第 12 行」不知道是哪个文件，还是两个都摆。
	//
	// scope 和 line 必须先落到 a.ex 上再算 key：explainKey 在单行粒度下
	// 读的就是这两个字段，在结构体字面量里算 key 用的是**旧的** a.ex
	// （上一张卡片的 scope 和行内容），算出来的 key 是错的。
	a.ex = explainState{
		fi:    fi,
		hi:    hi,
		head:  hunkHeaderText(h),
		scope: scopeLine,
		line:  line,
	}
	a.ex.key = a.explainKey(f, hi)

	if text, ok := a.store.Lookup(a.ex.key); ok {
		a.ex.text = text
		a.ex.cached = true
		a.recordExplainEntry(text, true)
		a.rowsDirty = true
		return
	}
	a.rowsDirty = true
	a.startExplainLine(f.FileDiff, h, hi, lineIdx, fileKey(f))
}

// lineNum 是这一行在文件里的行号：新侧优先，纯删除的行退回旧侧。
func lineNum(l diff.Line) int {
	if l.NewNum > 0 {
		return l.NewNum
	}
	return l.OldNum
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
	system = a.store.Config().EffectiveExplainPrompt(system)

	// ctx 和信箱都在主线程建好，立刻挂到状态上 —— 否则用户在请求返回之前
	// 关掉卡片，就没人去取消它了。
	ctx, cancel := context.WithCancel(context.Background())
	a.ex.cancel = cancel
	a.ex.loading = true
	mail := &explainMail{last: time.Now()}
	a.ex.mail = mail

	key := a.ex.key
	prov := a.store.Provider()

	a.run(func() { a.streamExplain(ctx, prov, key, mail, system, user) })
}

// startExplainLine 起一个单行解释的后台请求。
//
// 和 startExplain 分开而不是塞个 scope 参数进去：两者的提示词来源不同
// （BuildSelectionPrompt vs BuildPrompt），共用的只有后面那一串流式收尾。
func (a *app) startExplainLine(f diff.FileDiff, h diff.Hunk, hi, lineIdx int, contentKey string) {
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
	system, user := a.selectionPrompt(f, h, hi, lineIdx, lines)
	system = a.store.Config().EffectiveExplainPrompt(system)

	ctx, cancel := context.WithCancel(context.Background())
	a.ex.cancel = cancel
	a.ex.loading = true
	mail := &explainMail{last: time.Now()}
	a.ex.mail = mail

	key := a.ex.key
	prov := a.store.Provider()

	a.run(func() { a.streamExplain(ctx, prov, key, mail, system, user) })
}

// selectionPrompt 拼单行解释的提示词。
//
// 上下文围着这一行取，而不是围着整个 hunk：卡片上写的是「第 12 行」，
// 给的上下文却是前后一整段，模型很容易顺势讲起整段来。行号以 hunk 里的
// 那一行为准 —— 它跟用户双击的那一行是同一行。
func (a *app) selectionPrompt(f diff.FileDiff, h diff.Hunk, hi, lineIdx int, lines []string) (string, string) {
	l := h.Lines[lineIdx]
	no := lineNum(l)
	return explain.BuildSelectionPrompt(explain.SelectionRequest{
		File:     f,
		Code:     strings.TrimLeft(l.Content, "+-"),
		LineNum:  no,
		Question: a.ex.question,
		Context:  explain.LineWindow(lines, no, explainAround/2, explainMaxCtx),
	})
}

// streamExplain 是两种粒度共用的流式收尾。
//
// key 是这次请求的缓存 key，也是「卡片有没有换目标」的凭据：用户在请求
// 返回前点到别的行、别的段，旧结果就该丢掉，不能接在当前卡片后面。
func (a *app) streamExplain(ctx context.Context, prov *llm.Provider, key string, mail *explainMail, system, user string) {
	text, err := llm.StreamExplanation(ctx, prov, system, user, func(chunk string) {
		if mail.fill(chunk) {
			a.dispatch(func() { a.flushExplain(mail) })
		}
	})
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
		a.recordExplainEntry(a.ex.text, false)
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
	a.rowsDirty = true
}

func (a *app) cancelExplain() {
	if a.ex.cancel != nil {
		a.ex.cancel()
		a.ex.cancel = nil
	}
}

// explainInlineRow 以 GitHub 风格在代码行间内嵌呈现 AI 解释卡片或折叠条。
func (a *app) explainInlineRow(c *ui.Context, pal palette, r row) {
	if !a.explainOpen() {
		return
	}
	// 刷新仓库或改筛选之后，fi/hi 可能已经指向别的文件了 —— 收起解释
	if !a.explainStillValid() {
		a.closeExplain()
		return
	}

	if a.ex.collapsed {
		// GitHub 风格折叠态：简洁内嵌胶囊小条，点击展开
		countStr := ""
		if len(a.ex.entries) > 1 {
			countStr = fmt.Sprintf(" (%d/%d)", a.ex.curIdx+1, len(a.ex.entries))
		}
		// 外层 Box 负责左右内边距：卡片铺满列表宽度再加外侧 margin 会顶到
		// 右侧滚动条上，关闭按钮就点不到了。
		ui.Box(c).FillWidth().Shrink(0).Padding(3, 16).Children(func() {
			pill := ui.Row(c).Height(30).Shrink(0).FillWidth().
				Padding(0, 12).Gap(8).AlignItems(ui.Center).
				Radius(radiusSM).Background(pal.surface2).Border(1, pal.hairline).
				Cursor(ui.CursorPointer)
			if pill.Hovered() {
				pill.Background(pal.surface3)
			}
			fadeColors(pill)
			pill.Children(func() {
				sparkleCluster(c, 13, pal.inkMuted)
				ui.Text(c, Tf("✨ AI 解释%s · 点击展开", "✨ AI Explanation%s · Click to expand", countStr)).
					FontSize(11).FontWeight(weightMedium).TextColor(pal.ink)
				drawIcon(c, iconChevronDown, 11, pal.inkSubtle)
				ui.Spacer(c)
				closeBtn := ui.ButtonBase(c).Size(18, 18).Radius(radiusXS).
					AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
				closeBtn.Children(func() {
					ui.Text(c, "×").FontSize(11).TextColor(pal.inkSubtle)
				})
				if closeBtn.Clicked() {
					a.closeExplain()
				}
			})
			if pill.Clicked() {
				a.ex.collapsed = false
				a.rowsDirty = true
			}
		})
		return
	}

	// 展开态：GitHub 风格内嵌卡片，与上下代码流融为一体，彻底消除浮层遮挡代码。
	// 同样用外层 Box 的内边距代替 margin，右侧给滚动条留出空间。
	ui.Box(c).FillWidth().Shrink(0).Padding(6, 16).Children(func() {
		card := ui.Column(c).Shrink(0).FillWidth().Radius(cardRadius).
			Background(pal.surface1).Border(1, pal.hairline).
			Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.06))

		card.Children(func() {
			a.explainHead(c, pal)
			a.explainBody(c, pal, 520)
			if a.ex.scope == scopeLine {
				a.askRow(c, pal)
			}
			a.explainFoot(c, pal)
		})
	})
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// recordExplainEntry 记录一条解析结果，避免重复并保持历史可追溯。
func (a *app) recordExplainEntry(text string, cached bool) {
	if text == "" {
		return
	}
	for i, e := range a.ex.entries {
		if e.Text == text {
			a.ex.curIdx = i
			return
		}
	}
	entry := explainEntry{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Head:      a.ex.head,
		Text:      text,
		Cached:    cached,
		CreatedAt: time.Now(),
	}
	a.ex.entries = append(a.ex.entries, entry)
	a.ex.curIdx = len(a.ex.entries) - 1
}

// syncCurrentExplainEntry 把当前翻到的卡片数据同步到主显示。
func (a *app) syncCurrentExplainEntry() {
	if a.ex.curIdx < 0 || a.ex.curIdx >= len(a.ex.entries) {
		return
	}
	e := a.ex.entries[a.ex.curIdx]
	a.ex.text = e.Text
	a.ex.cached = e.Cached
	a.ex.scroll = ui.ScrollState{}
}

// deleteCurrentExplainEntry 删除当前卡片条目。
func (a *app) deleteCurrentExplainEntry() {
	if len(a.ex.entries) == 0 {
		a.closeExplain()
		return
	}
	if a.ex.curIdx >= 0 && a.ex.curIdx < len(a.ex.entries) {
		a.ex.entries = append(a.ex.entries[:a.ex.curIdx], a.ex.entries[a.ex.curIdx+1:]...)
	}
	if len(a.ex.entries) == 0 {
		a.closeExplain()
		return
	}
	if a.ex.curIdx >= len(a.ex.entries) {
		a.ex.curIdx = len(a.ex.entries) - 1
	}
	a.syncCurrentExplainEntry()
}

// explainHead 是卡片顶部：支持拖拽位移、标题、翻页 < 1/3 >、删除某次、收缩折叠与关闭。
func (a *app) explainHead(c *ui.Context, pal palette) {
	ui.Column(c).Shrink(0).Padding(10, 12).Gap(4).
		Background(pal.surface2).
		Radius(cardRadius, cardRadius, 0, 0).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			sparkleCluster(c, 15, pal.inkMuted)
			ui.Text(c, T("AI 解释", "AI Explanation")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
			if a.ex.cached {
				specChip(c, pal, T("缓存", "Cached"), ui.Transparent, statusChipH, statusChipText)
			}
			ui.Spacer(c)

			// 多次解析翻页控制器 < 1/3 >
			if len(a.ex.entries) > 1 {
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					prevBtn := ui.ButtonBase(c).Size(20, 20).Radius(radiusXS).
						AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
						Tooltip(T("上一条解析", "Previous explanation"))
					if prevBtn.Hovered() && a.ex.curIdx > 0 {
						prevBtn.Background(pal.surface3)
					}
					fadeColors(prevBtn)
					prevBtn.Children(func() {
						col := pal.ink
						if a.ex.curIdx <= 0 {
							col = pal.inkTertiary
						}
						ui.Text(c, "‹").FontSize(14).TextColor(col)
					})
					if prevBtn.Clicked() && a.ex.curIdx > 0 {
						a.ex.curIdx--
						a.syncCurrentExplainEntry()
					}

					ui.Text(c, fmt.Sprintf("%d/%d", a.ex.curIdx+1, len(a.ex.entries))).
						FontSize(11).Font("monospace").TextColor(pal.inkSubtle)

					nextBtn := ui.ButtonBase(c).Size(20, 20).Radius(radiusXS).
						AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
						Tooltip(T("下一条解析", "Next explanation"))
					if nextBtn.Hovered() && a.ex.curIdx < len(a.ex.entries)-1 {
						nextBtn.Background(pal.surface3)
					}
					fadeColors(nextBtn)
					nextBtn.Children(func() {
						col := pal.ink
						if a.ex.curIdx >= len(a.ex.entries)-1 {
							col = pal.inkTertiary
						}
						ui.Text(c, "›").FontSize(14).TextColor(col)
					})
					if nextBtn.Clicked() && a.ex.curIdx < len(a.ex.entries)-1 {
						a.ex.curIdx++
						a.syncCurrentExplainEntry()
					}
				})
			}

			// 删除单次解析
			delBtn := ui.ButtonBase(c).Size(20, 20).Radius(radiusXS).
				AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
				Tooltip(T("删除此条解析", "Delete this explanation"))
			if delBtn.Hovered() {
				delBtn.Background(pal.surface3)
			}
			fadeColors(delBtn)
			delBtn.Children(func() {
				drawIcon(c, iconTrash, 12, pal.inkSubtle)
			})
			if delBtn.Clicked() {
				a.deleteCurrentExplainEntry()
			}

			// 收缩/折叠按钮
			collapseBtn := ui.ButtonBase(c).Size(20, 20).Radius(radiusXS).
				AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
				Tooltip(T("收缩解析窗口", "Collapse explanation"))
			if collapseBtn.Hovered() {
				collapseBtn.Background(pal.surface3)
			}
			fadeColors(collapseBtn)
			collapseBtn.Children(func() {
				drawIcon(c, iconChevronUp, 12, pal.inkSubtle)
			})
			if collapseBtn.Clicked() {
				a.ex.collapsed = true
			}

			if closeButton(c, pal) {
				a.closeExplain()
			}
		})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.ex.head).Font("monospace").FontSize(textCaption).
					SingleLine().MinWidth(0).Grow(1).TextColor(pal.inkSubtle)
				// 单行解释时把那一行指出来。放在 hunk 头这一行右端，
				// 不另起一行：卡片讲的就是这一行，标题栏里就该看见是第几行。
				if tag := a.ex.line.tag(); a.ex.scope == scopeLine && tag != "" {
					ui.Text(c, tag).Font("monospace").FontSize(textCaption).SingleLine().
						TextColor(pal.inkMuted)
				}
			})
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
				if primaryAction(c, pal, T("模型设置…", "Model Settings…"), "", false).Clicked() {
					a.openSettings()
				}
			}
			if secondaryAction(c, pal, T("重试", "Retry"), "", false).Clicked() {
				a.retryExplain()
			}
		})
		return

	case a.ex.needKey:
		ui.Text(c, T("还没有配置模型", "No model configured")).Bold().FontSize(12)
		ui.Text(c, T("解释代码要接一个模型，填上 API key 就能用了。", "Configure an AI model with an API key to explain code changes.")).
			FontSize(12).TextColor(t.TextMuted)
		cardActions(c, func() {
			if primaryAction(c, pal, T("模型设置…", "Model Settings…"), "", false).Clicked() {
				a.openSettings()
			}
		})
		return

	case a.ex.text == "":
		if a.ex.loading {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, T("正在读这段改动…", "Reading code changes…")).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		ui.Text(c, T("没有内容", "No content")).FontSize(12).TextColor(t.TextMuted)
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
		ui.Checkbox(c, &v, T("已 review", "Reviewed"))
		if v != viewed {
			a.toggleViewed(*f)
		}
		ui.Spacer(c)
		if a.ex.loading {
			ui.Text(c, T("生成中…", "Generating…")).FontSize(textCaption).TextColor(pal.inkSubtle)
		}
	})
}

// askRow 是单行解释底下的追问框：就着这一行再问一句，答案接在同一张卡片里。
//
// 只有单行解释有它。「这一段为什么改」是 hunk 那颗按钮的问题，问错了对象
// 会得到一段泛泛的解释，不如不给。
//
// 输入框和卡片是一帧里构建的，所以它自己会的 Changed() 读不到 Enter ——
// 回车在单行编辑器里是我们的 Submitted()，光标进去那一帧就该问值。
func (a *app) askRow(c *ui.Context, pal palette) {
	ui.Box(c).Shrink(0).Padding(8, 12).FillWidth().Children(func() {
		in := ui.TextInput(c, &a.ex.question).
			Placeholder(T("就这一行追问…", "Ask about this line…")).
			Label(T("追问", "Follow-up question"))
		fadeColors(in)
		if a.ex.askOn {
			in.Focus()
			a.ex.askOn = false
		}

		if in.Submitted() {
			a.ex.askOn = true
			a.reAskLine()
		}
	})
}

// reAskLine 用当前的问题重问这一行。
//
// 老答案整块换掉：追问是在同一张卡片里接着聊，但每一次请求都该是一次完整
// 的回答，不是接在半句话后面。
func (a *app) reAskLine() {
	if !a.explainOpen() || a.ex.scope != scopeLine {
		return
	}
	if a.ex.fi < 0 || a.ex.fi >= len(a.files) || a.ex.hi < 0 || a.ex.hi >= len(a.files[a.ex.fi].Hunks) {
		return
	}
	f := a.files[a.ex.fi]
	h := f.Hunks[a.ex.hi]
	if a.ex.line.lineIdx < 0 || a.ex.line.lineIdx >= len(h.Lines) {
		return
	}
	a.cancelExplain()
	a.ex.text = ""
	a.ex.scroll = ui.ScrollState{}
	a.ex.fail = llm.Failure{}
	a.ex.needKey = false
	// key 要重算：问题变了，缓存命中该变。换 key 也顺带让旧请求的结果落空
	// （streamExplain 里比 key 才知道该不该收）。
	a.ex.key = a.explainKey(f, a.ex.hi)
	a.startExplainLine(f.FileDiff, h, a.ex.hi, a.ex.line.lineIdx, fileKey(f))
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
