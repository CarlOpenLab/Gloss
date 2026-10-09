package main

import (
	"github.com/egoist/mygo/ui"
)

// 确认框：给「做下去就收不回来」的动作加一道闸。
//
// 不直接用框架的 ui.AlertDialog，四个实测出来的原因：
//
//   - Esc 只认字面量 "Cancel" 按钮 —— 全中文界面上会冒出一颗英文按钮；
//     （它的对齐还只认 "align-center" 这类字面量，跟着一起坏。）
//   - 点背景不关（只带了 flagModal，没有人判 back.Clicked）；
//   - 最后一个按钮 AutoFocus 而且吃 Enter —— 对「硬重置」「删分支」这种
//     按钮，误按一下回车 = 直接执行，恰好是最危险的默认值；
//   - 只有一行 message，写不下「会丢什么」。
//
// 所以自己搭：ui.Modal 原生处理 Esc 和点背景（都当归「取消」），按钮文字
// 全中文自由，布局能摆下两段话。状态挂在 app 上、回调留在状态里 ——
// 和 tagdialog.go 同一个形状。

// confirmState 是一次确认对话框的状态。
type confirmState struct {
	on    bool
	title string
	// body 是「做下去会发生什么」，一两句话。可以留空（事情一目了然时），
	// 但危险动作不该留 —— 用户要在点按钮之前读到会丢什么。
	body string
	// yes 是确认键上的字：完整的动作名（「中止合并」），不是「确定」。
	// 按钮自己在对话框里站着，写「确定」就等于让用户回头往上找这句
	// 「要确定什么」。
	yes string
	// onYes 是确认之后执行的那件事。
	onYes func()
	// focusNo 还欠「取消」键一次焦点 —— 打开后回车 = 取消。危险按钮要先
	// 按一次 Tab 才够得着：多出来的一步就是这道闸的用处。
	focusNo bool
}

// askConfirm 打开确认框。cancel 文案固定「取消」：对话框里只有两个选项，
// 另一个写得再清楚，「取消」也不会认错。
func (a *app) askConfirm(st confirmState) {
	st.on = true
	st.focusNo = true
	a.confirm = st
}

// confirmDialog 是对话框入口，画在 view 的对话框那一层。
func (a *app) confirmDialog(c *ui.Context, pal palette) {
	if !a.confirm.on {
		return
	}
	open := true
	specModal(c, pal, &open, func() { a.confirmBody(c, pal) })
	if !open {
		// Esc 和点背景走到这里，效果等于「取消」——不必再问一次。
		a.confirm = confirmState{}
	}
}

func (a *app) confirmBody(c *ui.Context, pal palette) {
	t := c.Theme()
	cf := &a.confirm

	ui.Column(c).Width(420).Gap(12).Children(func() {
		ui.Text(c, cf.title).FontSize(textHeadline).FontWeight(weightSemibold)
		if cf.body != "" {
			ui.Text(c, cf.body).FontSize(12).TextColor(t.TextMuted)
		}

		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel := secondaryAction(c, pal, "取消", "", false)
			if cf.focusNo {
				// 焦点只送一次（同 tagdialog.go 的名称框）：每帧都送的话，
				// Tab 永远跳不到确认键上，这道闸就成了死路。
				cancel.Focus()
				cf.focusNo = false
			}
			if cancel.Clicked() {
				a.confirm = confirmState{}
				return
			}
			if primaryAction(c, pal, cf.yes, "", false).Clicked() {
				// 先把状态清掉再跑回调：回调里可能把它换成别的对话框
				//（比如失败时留话），别让这里的事后清理把新的那个抹掉。
				run := cf.onYes
				a.confirm = confirmState{}
				if run != nil {
					run()
				}
			}
		})
	})
}
