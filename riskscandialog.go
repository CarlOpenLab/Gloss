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
	ui.Modal(c, &open, func() { a.riskScanBody(c, pal) })
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
			shieldIcon(c, 16, t.Accent)
			ui.Text(c, "代码变更安全与风险预检").Bold().FontSize(15)
			ui.Spacer(c)
			if closeButton(c) {
				a.closeRiskScan()
				return
			}
		})

		// 加载中状态
		if rs.loading {
			ui.Column(c).Padding(36, 0).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在全面预检暂存区代码中的潜在漏洞与凭证泄露…").
					Bold().FontSize(13).TextColor(t.Text)
				ui.Text(c, "分析凭证泄露、代码注入、并发异常及破坏性变更").
					FontSize(11).TextColor(t.TextMuted)
				ui.Row(c).Padding(8, 0).Children(func() {
					if ui.Button(c, "取消预检").Clicked() {
						a.closeRiskScan()
					}
				})
			})
			return
		}

		// 错误状态
		if rs.err != "" {
			ui.Column(c).Padding(16).Radius(t.Radius).
				Background(t.Danger.Alpha(0.10)).Border(1, t.Danger.Alpha(0.35)).Gap(8).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					warnIcon(c, 14, t.Danger)
					ui.Text(c, "预检扫描失败").Bold().FontSize(13).TextColor(t.Danger)
				})
				ui.Text(c, rs.err).FontSize(12).TextColor(t.Text)
			})

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "关闭").Clicked() {
					a.closeRiskScan()
				}
				if ui.PrimaryButton(c, "重试").Clicked() {
					a.startRiskScan()
				}
			})
			return
		}

		// 整体安全徽标 (Security Badge)
		badgeIcon, badgeText := rs.report.StatusBadge()
		var badgeBg, badgeBorder, badgeFg ui.Color
		switch badgeText {
		case "安全通过":
			badgeBg = t.Success.Alpha(0.12)
			badgeBorder = t.Success.Alpha(0.40)
			badgeFg = t.Success
		case "需注意":
			badgeBg = t.Warning.Alpha(0.12)
			badgeBorder = t.Warning.Alpha(0.40)
			badgeFg = t.Warning
		default: // "高危发现"
			badgeBg = t.Danger.Alpha(0.12)
			badgeBorder = t.Danger.Alpha(0.40)
			badgeFg = t.Danger
		}

		ui.Row(c).Padding(10, 14).Radius(t.Radius).
			Background(badgeBg).Border(1, badgeBorder).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, badgeIcon+" "+badgeText).Bold().FontSize(14).TextColor(badgeFg)
			ui.Text(c, "·").FontSize(12).TextColor(badgeFg.Alpha(0.6))
			ui.Text(c, rs.report.Summary).FontSize(12).TextColor(t.Text).Grow(1)
		})

		// 发现项列表 (Findings List)
		if len(rs.report.Findings) == 0 {
			ui.Box(c).Padding(24, 16).Radius(t.Radius).
				Background(t.Surface).Border(1, t.Border.Alpha(0.4)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, "✅ 暂存区代码未检出已知安全漏洞、凭证机密或危险并发缺陷。").
						FontSize(12).TextColor(t.TextMuted)
				})
			})
		} else {
			ui.Scroll(c).TrackScroll(&rs.scroll).MaxHeight(280).Grow(1).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					for _, f := range rs.report.Findings {
						riskFindingCard(c, t, f)
					}
				})
			})
		}

		// 底部操作栏
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			if len(rs.report.Findings) > 0 {
				ui.Textf(c, "共发现 %d 处风险项", len(rs.report.Findings)).
					FontSize(11).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if ui.Button(c, "关闭").Clicked() {
				a.closeRiskScan()
			}
			commitBtn := ui.PrimaryButton(c, "继续提交").Tooltip("确认预检结果并提交已暂存改动")
			if commitBtn.Clicked() {
				a.proceedToCommitFromScan()
			}
		})
	})
}

// riskFindingCard 渲染单条安全或代码风险卡片。
func riskFindingCard(c *ui.Context, t *ui.Theme, f explain.RiskFinding) {
	var pillBg, pillBorder, pillFg ui.Color
	switch f.Severity {
	case explain.RiskCritical, explain.RiskHigh:
		pillBg = t.Danger.Alpha(0.12)
		pillBorder = t.Danger.Alpha(0.40)
		pillFg = t.Danger
	case explain.RiskMedium:
		pillBg = t.Warning.Alpha(0.12)
		pillBorder = t.Warning.Alpha(0.40)
		pillFg = t.Warning
	default:
		pillBg = t.Accent.Alpha(0.10)
		pillBorder = t.Accent.Alpha(0.35)
		pillFg = t.Accent
	}

	ui.Column(c).Padding(10, 12).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border.Alpha(0.5)).Gap(6).Children(func() {
		// 行 1: 严重度 Pill + 标题 + 文件路径与行号
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Box(c).Padding(2, 6).Radius(3).
				Background(pillBg).Border(1, pillBorder).Children(func() {
				ui.Text(c, string(f.Severity)).FontSize(10).Bold().TextColor(pillFg)
			})
			ui.Text(c, f.Title).Bold().FontSize(12).TextColor(t.Text).Grow(1)
			if f.FilePath != "" {
				loc := f.FilePath
				if f.Line > 0 {
					loc = fmt.Sprintf("%s:%d", f.FilePath, f.Line)
				}
				ui.Text(c, loc).Font("monospace").FontSize(11).TextColor(t.TextMuted)
			}
		})

		// 行 2: 风险描述
		if f.Description != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "风险描述:").FontSize(11).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, f.Description).FontSize(11).TextColor(t.Text).Grow(1)
			})
		}

		// 行 3: 建议修复
		if f.Suggestion != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "建议修复:").FontSize(11).TextColor(pillFg).Shrink(0)
				ui.Text(c, f.Suggestion).FontSize(11).TextColor(t.Text).Grow(1)
			})
		}
	})
}
