package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/explain"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// riskScanState 保存提交前代码变更安全与风险预检的状态。
type riskScanState struct {
	loading bool
	dialog  bool
	cancel  context.CancelFunc
	report  explain.RiskReport
	err     string
	scroll  ui.ScrollState
}

// startRiskScan 发起对暂存区全部代码改动的安全预检。
func (a *app) startRiskScan() {
	if a.riskScan.loading {
		a.cancelRiskScan()
		return
	}
	if !a.store.Provider().Ready() {
		a.openSettings()
		return
	}
	var staged []diff.FileDiff
	for _, f := range a.allFiles {
		if f.Section == gitrepo.SectionStaged {
			staged = append(staged, f.FileDiff)
		}
	}
	if len(staged) == 0 {
		return
	}

	a.cancelRiskScan()

	ctx, cancel := context.WithCancel(context.Background())
	a.riskScan.cancel = cancel
	a.riskScan.loading = true
	a.riskScan.dialog = true
	a.riskScan.err = ""
	a.riskScan.report = explain.RiskReport{}

	system, user := explain.BuildRiskScanPrompt(staged)
	prov := a.store.Provider()

	a.run(func() {
		text, err := llm.StreamExplanation(ctx, prov, system, user, nil)
		a.dispatch(func() {
			if ctx.Err() != nil {
				return
			}
			a.riskScan.loading = false
			a.riskScan.cancel = nil
			if err != nil {
				a.riskScan.err = err.Error()
				return
			}
			report := explain.ParseRiskReport(text)
			a.riskScan.report = report
		})
	})
}

// cancelRiskScan 取消正在进行的预检。
func (a *app) cancelRiskScan() {
	if a.riskScan.cancel != nil {
		a.riskScan.cancel()
		a.riskScan.cancel = nil
	}
	a.riskScan.loading = false
}

// closeRiskScan 关闭预检弹窗。
func (a *app) closeRiskScan() {
	a.cancelRiskScan()
	a.riskScan.dialog = false
}

// proceedToCommitFromScan 关闭预检弹窗并继续提交。
func (a *app) proceedToCommitFromScan() {
	a.closeRiskScan()
	if a.canCommit() {
		a.doCommit()
	} else {
		a.commitFocused = true
	}
}

// riskScanDialog 是安全预检对话框入口。
func (a *app) riskScanDialog(c *ui.Context, pal palette) {
	if !a.riskScan.dialog {
		return
	}
	open := true
	specModal(c, pal, &open, func() { a.riskScanBody(c, pal) })
	if !open {
		a.closeRiskScan()
	}
}

// riskScanBody 渲染预检弹窗内部。
func (a *app) riskScanBody(c *ui.Context, pal palette) {
	t := c.Theme()
	rs := &a.riskScan

	ui.Column(c).Width(620).MaxHeight(540).Gap(14).Children(func() {
		// 顶部标题栏
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			shieldIcon(c, 16, pal.inkMuted)
			ui.Text(c, "代码变更安全与风险预检").FontSize(textHeadline).
				FontWeight(weightSemibold).TextColor(pal.ink)
			ui.Spacer(c)
			if closeButton(c, pal) {
				a.closeRiskScan()
				return
			}
		})

		// 加载中状态
		if rs.loading {
			ui.Column(c).Padding(36, 0).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在全面预检暂存区代码中的潜在漏洞与凭证泄露…").
					FontSize(textBody).FontWeight(weightMedium).TextColor(pal.ink)
				ui.Text(c, "分析凭证泄露、代码注入、并发异常及破坏性变更").
					FontSize(textCaption).TextColor(pal.inkSubtle)
				ui.Row(c).Padding(8, 0).Children(func() {
					if secondaryAction(c, pal, "取消预检", "", false).Clicked() {
						a.closeRiskScan()
					}
				})
			})
			return
		}

		// 错误状态。走「横幅」那条规则：语义底色可以留（§5 允许语义色），
		// 因为这里说的正是「出事了」，而不是某种装饰。
		if rs.err != "" {
			ui.Column(c).Padding(16).Radius(radiusMD).
				Background(t.Danger.Alpha(0.10)).Border(1, t.Danger.Alpha(0.35)).Gap(8).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					warnIcon(c, 14, t.Danger)
					ui.Text(c, "预检扫描失败").FontSize(textBody).
						FontWeight(weightSemibold).TextColor(t.Danger)
				})
				ui.Text(c, rs.err).FontSize(textBodySM).TextColor(pal.inkMuted)
			})

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if secondaryAction(c, pal, "关闭", "", false).Clicked() {
					a.closeRiskScan()
				}
				if primaryAction(c, pal, "重试", "", false).Clicked() {
					a.startRiskScan()
				}
			})
			return
		}

		// 整体安全徽标 (Security Badge)
		//
		// 结论用一颗语义圆点带出来，而不是把整条横幅涂成红/黄/绿：这条带子
		// 后面还跟着一段结论摘要，底色一染，摘要那些字就得挑颜色配它 ——
		// 而摘要本身就是普通正文。
		badgeIcon, badgeText := rs.report.StatusBadge()
		var badgeDot ui.Color
		switch badgeText {
		case "安全通过":
			badgeDot = pal.success
		case "需注意":
			badgeDot = pal.warning
		default: // "高危发现"
			badgeDot = pal.danger
		}

		ui.Row(c).Padding(10, 14).Radius(radiusMD).
			Background(pal.surface2).Border(1, pal.hairline).
			AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(chipDot, chipDot).Shrink(0).Radius(chipDot / 2).
					Background(badgeDot)
				ui.Text(c, badgeIcon+" "+badgeText).FontSize(textBody).
					FontWeight(weightSemibold).TextColor(pal.ink)
			})
			ui.Text(c, "·").FontSize(textBodySM).TextColor(pal.inkTertiary)
			ui.Text(c, rs.report.Summary).FontSize(textBodySM).TextColor(pal.inkMuted).Grow(1)
		})

		// 发现项列表 (Findings List)
		if len(rs.report.Findings) == 0 {
			ui.Box(c).Padding(24, 16).Radius(radiusMD).
				Background(pal.surface2).Border(1, pal.hairline).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, "✅ 暂存区代码未检出已知安全漏洞、凭证机密或危险并发缺陷。").
						FontSize(textBodySM).TextColor(pal.inkMuted)
				})
			})
		} else {
			ui.Scroll(c).TrackScroll(&rs.scroll).MaxHeight(280).Grow(1).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					for _, f := range rs.report.Findings {
						riskFindingCard(c, pal, f)
					}
				})
			})
		}

		// 底部操作栏
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			if len(rs.report.Findings) > 0 {
				ui.Textf(c, "共发现 %d 处风险项", len(rs.report.Findings)).
					FontSize(textCaption).TextColor(pal.inkSubtle)
			}
			ui.Spacer(c)
			if secondaryAction(c, pal, "关闭", "", false).Clicked() {
				a.closeRiskScan()
			}
			commitBtn := primaryAction(c, pal, "继续提交", "确认预检结果并提交已暂存改动", false)
			if commitBtn.Clicked() {
				a.proceedToCommitFromScan()
			}
		})
	})
}

// riskFindingCard 渲染单条安全或代码风险卡片。
func riskFindingCard(c *ui.Context, pal palette, f explain.RiskFinding) {
	// 严重度是「这一条有多要紧」，正落在 §2.5 那张表上：语义靠 6px 圆点，
	// 整块芯片保持中性。低危不再借用强调色 —— 它既不重要也不该抢眼。
	//
	// 三色走 pal 而不是 t：调色板里的 success/warning/danger 就是从主题搬
	// 过来的同一份值，主包统一从这一个入口拿，省得同一种红有两个名字。
	var dot ui.Color
	switch f.Severity {
	case explain.RiskCritical, explain.RiskHigh:
		dot = pal.danger
	case explain.RiskMedium:
		dot = pal.warning
	default:
		dot = pal.inkTertiary
	}

	ui.Column(c).Padding(10, 12).Radius(radiusMD).
		Background(pal.surface2).Border(1, pal.hairline).Gap(6).Children(func() {
		// 行 1: 严重度芯片 + 标题 + 文件路径与行号
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			specChip(c, pal, string(f.Severity), dot, rowChipH, rowChipText)
			ui.Text(c, f.Title).FontSize(textBodySM).FontWeight(weightSemibold).
				TextColor(pal.ink).Grow(1)
			if f.FilePath != "" {
				loc := f.FilePath
				if f.Line > 0 {
					loc = fmt.Sprintf("%s:%d", f.FilePath, f.Line)
				}
				ui.Text(c, loc).Font("monospace").FontSize(textCaption).TextColor(pal.inkSubtle)
			}
		})

		// 行 2: 风险描述
		if f.Description != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "风险描述:").FontSize(textCaption).TextColor(pal.inkSubtle).Shrink(0)
				ui.Text(c, f.Description).FontSize(textCaption).TextColor(pal.inkMuted).Grow(1)
			})
		}

		// 行 3: 建议修复。标签用严重度那档色 —— 这是全文唯一一处「带颜色的字」，
		// 而它指的正是上面那颗圆点，一眼能对上是哪一条。
		if f.Suggestion != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "建议修复:").FontSize(textCaption).TextColor(dot).Shrink(0)
				ui.Text(c, f.Suggestion).FontSize(textCaption).TextColor(pal.inkMuted).Grow(1)
			})
		}
	})
}
