package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/explain"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// prDialogState 保存查看 GitHub PR 列表对话框的状态。
type prDialogState struct {
	on      bool
	loading bool
	err     string
	items   []gitrepo.PRItem
	scroll  ui.ScrollState
}

// prReviewState 保存检视 PR 代码差异并进行 AI Review 的状态。
type prReviewState struct {
	on        bool
	pr        gitrepo.PRItem
	diffFiles []diff.FileDiff
	state     loadState
	err       string

	// AI Review 评论与生成
	reviewModal bool
	reviewText  string
	editMode    bool // false 为 Markdown 渲染预览，true 为编辑原文
	loading     bool
	mail        *explainMail
	cancel      context.CancelFunc
	posting     bool
	postErr     string
	postSuccess bool
	scroll      ui.ScrollState
}

// openPRDialog 打开 GitHub PR 列表对话框并加载当前仓库的开放 PR。
func (a *app) openPRDialog() {
	if !a.repoReady() {
		return
	}
	a.prDlg.on = true
	a.prDlg.loading = true
	a.prDlg.err = ""
	a.prDlg.items = nil

	dir := a.repoPath
	a.run(func() {
		ctx := context.Background()
		items, err := gitrepo.ListPRs(ctx, dir)
		a.dispatch(func() {
			if !a.prDlg.on {
				return
			}
			a.prDlg.loading = false
			if err != nil {
				a.prDlg.err = err.Error()
				return
			}
			a.prDlg.items = items
		})
	})
}

// openPRReview 进入指定 PR 的代码差异检视模式。
func (a *app) openPRReview(pr gitrepo.PRItem) {
	if !a.repoReady() {
		return
	}
	if a.tab == 0 && a.workFiles == nil {
		a.workFiles = a.allFiles
	}
	a.tab = 0

	a.cancelAIReview()
	a.prReview.on = true
	a.prReview.pr = pr
	a.prReview.diffFiles = nil
	a.prReview.state = loadLoading
	a.prReview.err = ""
	a.prReview.reviewModal = false
	a.prReview.reviewText = ""
	a.prReview.postSuccess = false
	a.prReview.postErr = ""

	dir := a.repoPath
	prNum := pr.Number
	a.run(func() {
		ctx := context.Background()
		diffs, err := gitrepo.GetPRDiff(ctx, dir, prNum)
		a.dispatch(func() {
			if !a.prReview.on || a.prReview.pr.Number != prNum {
				return
			}
			if err != nil {
				a.prReview.state = loadFailed
				a.prReview.err = err.Error()
				return
			}
			a.prReview.state = loadReady
			a.prReview.diffFiles = diffs
			wrapped := gitrepo.FilesFromDiff(diffs)
			id := fmt.Sprintf("pr:%d", prNum)
			a.showFiles(wrapped, id)
		})
	})
}

// exitPRReview 退出 PR 检视模式并恢复工作区文件列表。
func (a *app) exitPRReview() {
	if !a.prReview.on {
		return
	}
	a.cancelAIReview()
	a.prReview.on = false
	a.prReview.state = loadIdle
	a.prReview.diffFiles = nil
	a.prReview.reviewModal = false

	if a.tab == 0 {
		if a.workFiles != nil {
			a.showFiles(a.workFiles, workListID(a.repoPath, a.workBranch()))
		} else {
			a.allFiles = nil
			a.applyFilter()
		}
	}
}

// openPRReviewModal 打开 AI Review 评审及评论生成模态框。
func (a *app) openPRReviewModal() {
	if !a.prReview.on {
		return
	}
	a.prReview.reviewModal = true
	a.prReview.postSuccess = false
	a.prReview.postErr = ""
	if a.prReview.reviewText == "" && !a.prReview.loading {
		a.startAIReview()
	}
}

// startAIReview 触发 LLM 流式生成 PR 代码评审建议。
func (a *app) startAIReview() {
	if !a.prReview.on {
		return
	}
	if a.prReview.loading {
		a.cancelAIReview()
		return
	}
	if !a.store.Provider().Ready() {
		a.openSettings()
		return
	}

	a.cancelAIReview()

	ctx, cancel := context.WithCancel(context.Background())
	a.prReview.cancel = cancel
	a.prReview.loading = true
	a.prReview.postSuccess = false
	a.prReview.postErr = ""
	a.prReview.reviewText = ""

	mail := &explainMail{last: time.Now()}
	a.prReview.mail = mail

	system, user := explain.BuildPRReviewPrompt(a.prReview.pr, a.prReview.diffFiles)
	prov := a.store.Provider()

	onDelta := func(chunk string) {
		if mail.fill(chunk) {
			a.dispatch(func() { a.flushAIReview(mail) })
		}
	}

	a.run(func() {
		text, err := llm.StreamExplanation(ctx, prov, system, user, onDelta)
		a.dispatch(func() {
			if a.prReview.mail != mail {
				return
			}
			a.flushAIReview(mail)
			a.prReview.loading = false
			a.prReview.cancel = nil
			if err != nil {
				if ctx.Err() == nil {
					a.prReview.postErr = "生成评审失败: " + err.Error()
				}
				return
			}
			a.prReview.reviewText = strings.TrimSpace(text)
		})
	})
}

func (a *app) flushAIReview(mail *explainMail) {
	if mail != a.prReview.mail {
		return
	}
	pending := mail.take()
	if pending == "" {
		return
	}
	a.prReview.reviewText += pending
}

func (a *app) cancelAIReview() {
	if a.prReview.cancel != nil {
		a.prReview.cancel()
		a.prReview.cancel = nil
	}
	a.prReview.loading = false
	a.prReview.mail = nil
}

// postPRReviewComment 调用 gh CLI 把 Review 评论提交到 GitHub PR。
func (a *app) postPRReviewComment() {
	if !a.prReview.on || a.prReview.posting {
		return
	}
	body := strings.TrimSpace(a.prReview.reviewText)
	if body == "" {
		return
	}
	a.prReview.posting = true
	a.prReview.postErr = ""
	a.prReview.postSuccess = false

	dir := a.repoPath
	prNum := a.prReview.pr.Number
	a.run(func() {
		ctx := context.Background()
		err := gitrepo.PostPRComment(ctx, dir, prNum, body)
		a.dispatch(func() {
			if !a.prReview.on || a.prReview.pr.Number != prNum {
				return
			}
			a.prReview.posting = false
			if err != nil {
				a.prReview.postErr = err.Error()
				return
			}
			a.prReview.postSuccess = true
		})
	})
}

// prDialog 渲染 GitHub PR 列表模态窗口。
func (a *app) prDialog(c *ui.Context, pal palette) {
	if !a.prDlg.on {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.prDialogBody(c, pal) })
	if !open {
		a.prDlg.on = false
	}
}

func (a *app) prDialogBody(c *ui.Context, pal palette) {
	t := c.Theme()
	s := &a.prDlg

	ui.Column(c).Width(580).Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Box(c).Size(18, 18).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Icon(iconGitPullRequest, r, t.Accent)
			})
			ui.Text(c, "GitHub Pull Requests").Bold().FontSize(15).TextColor(t.Text)
		})
		ui.Text(c, "查看当前仓库中开放的 Pull Request，检视代码差异并生成 AI Review 评论。").
			FontSize(11.5).TextColor(t.TextMuted)

		switch {
		case s.loading:
			ui.Row(c).Height(120).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在从 GitHub 获取 Pull Requests…").FontSize(12).TextColor(t.TextMuted)
			})
		case s.err != "":
			ui.Column(c).Gap(10).Children(func() {
				ui.Box(c).Padding(10, 12).Radius(6).Background(t.Danger.Alpha(0.08)).
					Border(1, t.Danger.Alpha(0.25)).Children(func() {
					ui.Text(c, "获取 PR 列表失败").Bold().FontSize(12).TextColor(t.Danger)
					ui.Text(c, s.err).FontSize(11).TextColor(t.TextMuted).Margin(4, 0, 0, 0)
				})
				ui.Row(c).Justify(ui.End).Children(func() {
					if ui.Button(c, "重试").Clicked() {
						a.openPRDialog()
					}
				})
			})
		case len(s.items) == 0:
			ui.Column(c).Height(100).Center().Gap(6).Children(func() {
				ui.Text(c, "暂无开放的 Pull Request").FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, "所有 PR 均已合并或关闭，或者当前仓库未包含打开的 PR。").
					FontSize(11).TextColor(t.TextMuted.Alpha(0.6))
			})
		default:
			ui.Scroll(c).TrackScroll(&s.scroll).MaxHeight(320).FillWidth().Children(func() {
				ui.Column(c).Gap(6).FillWidth().Children(func() {
					for _, item := range s.items {
						pr := item
						row := ui.ButtonBase(c).FillWidth().Padding(10, 12).Radius(6).
							Background(t.Background).Border(1, t.Border).Gap(6).
							Cursor(ui.CursorPointer).Label(fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title))
						if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						fadeColors(row)
						row.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
								ui.Box(c).Padding(2, 6).Radius(4).Background(t.Accent.Alpha(0.12)).Children(func() {
									ui.Textf(c, "#%d", pr.Number).Bold().FontSize(11.5).TextColor(t.Accent)
								})
								ui.Text(c, pr.Title).Bold().FontSize(12.5).TextColor(t.Text).Grow(1).SingleLine()
								ui.Text(c, "检视 Diff →").FontSize(11).TextColor(t.TextMuted)
							})
							ui.Row(c).AlignItems(ui.Center).Gap(10).Margin(4, 0, 0, 0).Children(func() {
								if pr.Author != "" {
									ui.Text(c, "@"+pr.Author).FontSize(11).TextColor(t.TextMuted)
									ui.Text(c, "·").FontSize(11).TextColor(t.TextMuted.Alpha(0.5))
								}
								ui.Textf(c, "%s → %s", pr.HeadBranch, pr.BaseBranch).
									Font("monospace").FontSize(10.5).TextColor(t.TextMuted)
							})
						})
						if row.Clicked() {
							a.prDlg.on = false
							a.openPRReview(pr)
						}
					}
				})
			})
		}

		ui.Row(c).Justify(ui.End).Children(func() {
			if ui.Button(c, "关闭").Clicked() {
				a.prDlg.on = false
			}
		})
	})
}

// prReviewBar 渲染在 PR 检视模式下主区顶部的状态及操作条。
func (a *app) prReviewBar(c *ui.Context, pal palette) {
	t := c.Theme()
	pr := a.prReview.pr

	ui.Row(c).Height(38).Shrink(0).FillWidth().
		Padding(0, 16).Gap(8).AlignItems(ui.Center).
		Background(t.Surface).Border(1, pal.border).Children(func() {

		ui.Box(c).Size(16, 16).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Icon(iconGitPullRequest, r, t.Accent)
		})
		ui.Text(c, "PR 检视").Bold().FontSize(12).TextColor(t.Text)

		ui.Box(c).Padding(1, 5).Radius(3).Background(t.Accent.Alpha(0.12)).Children(func() {
			ui.Textf(c, "#%d", pr.Number).Bold().FontSize(11).TextColor(t.Accent)
		})

		ui.Text(c, pr.Title).FontSize(12).Bold().TextColor(t.Text).
			Tooltip(fmt.Sprintf("#%d: %s (%s → %s)", pr.Number, pr.Title, pr.HeadBranch, pr.BaseBranch))

		ui.Box(c).Padding(1, 6).Radius(3).Background(t.Background).Border(1, t.Border).Children(func() {
			ui.Textf(c, "%s → %s", pr.HeadBranch, pr.BaseBranch).
				Font("monospace").FontSize(10.5).TextColor(t.TextMuted)
		})

		if pr.Author != "" {
			ui.Text(c, "@"+pr.Author).FontSize(11).TextColor(t.TextMuted)
		}

		toolbarDivider(c)

		switch a.prReview.state {
		case loadLoading:
			ui.Spinner(c)
			ui.Text(c, "正在读取 PR Diff…").FontSize(11).TextColor(t.TextMuted)
		case loadFailed:
			ui.Text(c, "Diff 读取失败: "+a.prReview.err).FontSize(11).TextColor(t.Danger)
			if ui.ButtonBase(c).Height(22).Padding(0, 6).Radius(3).Cursor(ui.CursorPointer).Label("重试").
				Children(func() { ui.Text(c, "重试").FontSize(11) }).Clicked() {
				a.openPRReview(pr)
			}
		default:
			ui.Textf(c, "%d 个改动文件", len(a.allFiles)).FontSize(11.5).TextColor(t.TextMuted)
		}

		// ✨ 生成并发布 AI Review 评论 按钮
		reviewBtn := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(5).Gap(4).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Background(t.Accent.Alpha(0.12)).Border(1, t.Accent.Alpha(0.3)).
			Label("✨ 生成并发布 AI Review 评论").Tooltip("AI 智能分析 PR 改动并提交 Review 评论")
		if reviewBtn.Hovered() {
			reviewBtn.Background(t.Accent.Alpha(0.2))
		}
		fadeColors(reviewBtn)
		reviewBtn.Children(func() {
			sparkleIcon(c, sparkleSize, t.Accent)
			ui.Text(c, "✨ 生成并发布 AI Review 评论").FontSize(11.5).Bold().TextColor(t.Accent)
		})
		if reviewBtn.Clicked() {
			a.openPRReviewModal()
		}

		// 切换 PR 按钮
		switchBtn := ui.ButtonBase(c).Height(26).Padding(0, 8).Radius(5).
			Background(t.Background).Border(1, t.Border).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Tooltip("查看其他 PR 列表").Label("切换 PR")
		if switchBtn.Hovered() {
			switchBtn.Background(t.SurfaceHover)
		}
		fadeColors(switchBtn)
		switchBtn.Children(func() {
			ui.Text(c, "切换 PR…").FontSize(11.5).TextColor(t.TextMuted)
		})
		if switchBtn.Clicked() {
			a.openPRDialog()
		}

		ui.Spacer(c)

		// 退出 PR 检视
		exitBtn := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(5).Gap(4).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Background(ui.Transparent).Border(1, t.Border).
			Label("退出 PR 检视").Tooltip("退出 PR 检视并恢复工作区 (Esc)")
		if exitBtn.Hovered() {
			exitBtn.Background(t.SurfaceHover)
		}
		fadeColors(exitBtn)
		exitBtn.Children(func() {
			ui.Text(c, "✕  退出检视").FontSize(11.5).TextColor(t.Text)
		})
		if exitBtn.Clicked() {
			a.exitPRReview()
		}
	})
}

// prReviewModal 渲染生成、预览并回写 GitHub PR Review 评论的模态对话框。
func (a *app) prReviewModal(c *ui.Context, pal palette) {
	if !a.prReview.reviewModal {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.prReviewModalBody(c, pal) })
	if !open {
		a.prReview.reviewModal = false
	}
}

func (a *app) prReviewModalBody(c *ui.Context, pal palette) {
	t := c.Theme()
	r := &a.prReview
	pr := r.pr

	ui.Column(c).Width(640).Gap(12).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			sparkleIcon(c, 16, t.Accent)
			ui.Textf(c, "AI Review 评论: #%d %s", pr.Number, pr.Title).
				Bold().FontSize(14).TextColor(t.Text).Grow(1).SingleLine()
		})
		ui.Text(c, "由 AI 模型结合变更 Diff 自动生成评审建议，支持编辑修改后一键回写至 GitHub PR。").
			FontSize(11.5).TextColor(t.TextMuted)

		if r.postSuccess {
			ui.Box(c).Padding(8, 12).Radius(5).Background(t.Success.Alpha(0.1)).
				Border(1, t.Success.Alpha(0.3)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Text(c, "✓").Bold().TextColor(t.Success)
					ui.Textf(c, "已成功将 Review 评论发布至 GitHub PR #%d！", pr.Number).
						FontSize(12).TextColor(t.Success)
				})
			})
		}
		if r.postErr != "" {
			ui.Box(c).Padding(8, 12).Radius(5).Background(t.Danger.Alpha(0.08)).
				Border(1, t.Danger.Alpha(0.25)).Children(func() {
				ui.Text(c, r.postErr).FontSize(11.5).TextColor(t.Danger)
			})
		}

		ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			modeIdx := 0
			if r.editMode {
				modeIdx = 1
			}
			toolbarSegmentedText(c, &modeIdx, false, "Markdown 预览", "编辑原文")
			r.editMode = (modeIdx == 1)

			if r.loading {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Spinner(c)
					ui.Text(c, "正在 AI 生成 Review 评论…").FontSize(11.5).TextColor(t.Accent)
				})
			}
		})

		if r.editMode {
			ui.TextArea(c, &r.reviewText).
				FillWidth().MinHeight(260).MaxHeight(320).
				Placeholder("AI 生成的 Review 评论内容将显示在此，您可以直接进行修改…").
				Label("Review 评论正文")
		} else {
			ui.Box(c).FillWidth().MinHeight(260).MaxHeight(320).
				Padding(12, 14).Radius(6).Background(t.Background).Border(1, t.Border).
				Children(func() {
					if r.reviewText == "" && !r.loading {
						ui.Text(c, "暂无评论内容，点击下方「开始生成」启动 AI 评审。").
							FontSize(12).TextColor(t.TextMuted)
					} else {
						ui.Scroll(c).TrackScroll(&r.scroll).FillWidth().Children(func() {
							ui.Column(c).FillWidth().Children(func() {
								st := mdStyleFor(t, pal)
								blocks := mdBlocks(r.reviewText, st)
								for i, b := range blocks {
									a.mdBlock(c, st, b, mdBlockGap(blocks, i))
								}
								if r.loading {
									ui.Row(c).Margin(8, 0, 0, 0).AlignItems(ui.Center).Gap(6).Children(func() {
										ui.Spinner(c)
										ui.Text(c, "生成中…").FontSize(11).TextColor(t.TextMuted)
									})
								}
							})
						})
					}
				})
		}

		ui.Row(c).AlignItems(ui.Center).FillWidth().Children(func() {
			if r.loading {
				if ui.Button(c, "停止生成").Clicked() {
					a.cancelAIReview()
				}
			} else {
				regenLabel := "重新生成"
				if r.reviewText == "" {
					regenLabel = "开始生成"
				}
				if ui.Button(c, regenLabel).Clicked() {
					a.startAIReview()
				}
			}

			ui.Spacer(c)

			if ui.Button(c, "关闭").Clicked() {
				r.reviewModal = false
			}

			publishDisabled := r.loading || r.posting || strings.TrimSpace(r.reviewText) == ""
			publishBtn := ui.PrimaryButton(c, "发布到 GitHub").
				Tooltip("通过 gh CLI 将评论回写至 GitHub PR").
				Disabled(publishDisabled)
			if r.posting {
				publishBtn.Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Spinner(c)
						ui.Text(c, "正在发布…").FontSize(12).TextColor(t.AccentText)
					})
				})
			}
			if publishBtn.Clicked() && !publishDisabled {
				a.postPRReviewComment()
			}
		})
	})
}
