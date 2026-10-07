package main

import (
	"github.com/egoist/mygo/ui"
)

// 写操作的结果横幅，挂在主列最顶上。
//
// 失败的要说清楚三件事：出了什么事（Title）、能怎么办（Hint）、git 自己的
// 原话（Detail）。前两句是我们翻译的，翻译可能不准 —— 原话留着，遇到怪问题
// 才有得对（见 gitrepo/failure.go）。
//
// 成功的走 Toast（窗口底部，几秒后自己消失）：一句「已提交 a1b2c3d」不需要
// 用户动手关掉，占着一条横幅反而是障碍。失败的必须自己关 —— 它带着要读的
// 内容，不能替用户决定读完了。

// opBanner 画失败横幅，顺便把成功提示递给 Toast。
func (a *app) opBanner(c *ui.Context, pal palette) {
	a.toastNote(c)
	if a.opErr == nil || a.opErr.Empty() {
		return
	}
	f := *a.opErr
	t := c.Theme()

	ui.Row(c).Shrink(0).FillWidth().Padding(9, 12).Gap(10).
		AlignItems(ui.Start).
		Background(t.Danger.Alpha(0.10)).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Danger.Alpha(0.35)).
		Children(func() {
			warnIcon(c, 14, t.Danger)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, f.Title).FontSize(12).Bold().TextColor(t.Text)
				if f.Hint != "" {
					ui.Text(c, f.Hint).FontSize(11).TextColor(t.TextMuted)
				}
				if f.Detail != "" {
					// git 的原话：等宽、小一号、压过色。它是证据不是结论。
					ui.Text(c, f.Detail).Font("monospace").FontSize(11).
						TextColor(t.TextMuted).Opacity(0.85)
				}
			})
			if closeButton(c) {
				a.opErr = nil
			}
		})
}

// toastNote 把 a.opNote 里的成功提示交给 Toast 一次。
//
// 每帧都 Toast 会把它的计时不断重置（框架按消息去重、重新计时），提示就永远
// 不消失 —— 所以记下已经播报过的那句话。opNote 每次操作都会被重写，字符串
// 相同的那次本来就是同一件事，播一遍就够。
func (a *app) toastNote(c *ui.Context) {
	if a.opNote == "" || a.opNote == a.toasted {
		return
	}
	a.toasted = a.opNote
	c.Toast(a.opNote)
}

// warnIcon 画一个圆圈里一个感叹号。
//
// 和叉号、箭头、勾一样不用字符：这类符号落到后备字体上就没有统一的形状和
// 基线，而横幅上这颗是「出事了」的第一眼信号，不能变形。
func warnIcon(c *ui.Context, size float32, col ui.Color) {
	ui.Box(c).Size(size, size).Shrink(0).Margin(1, 0, 0, 0).
		Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			rad := min(r.W, r.H) * 0.44

			var ring ui.Path
			ring.Circle(cx, cy, rad)
			p.StrokePath(&ring, 1.4, col)

			var stem ui.Path
			stem.MoveTo(cx, cy-rad*0.48).LineTo(cx, cy+rad*0.08)
			p.StrokePath(&stem, 1.6, col)

			var dot ui.Path
			dot.Circle(cx, cy+rad*0.48, 0.9)
			p.FillPath(&dot, col)
		})
}
