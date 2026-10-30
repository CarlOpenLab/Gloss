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
	mail        *aiMail
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

	mail := &aiMail{last: time.Now()}
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

func (a *app) flushAIReview(mail *aiMail) {
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
	// 表单对话框：点背景不关 —— PR 标题和描述打到一半不该被误触丢掉（见 specModalSticky）。
	specModalSticky(c, pal, &open, func() { a.prDialogBody(c, pal) })
	if !open {
		a.prDlg.on = false
	}
}

func (a *app) prDialogBody(c *ui.Context, pal palette) {
	s := &a.prDlg

	ui.Column(c).Width(580).Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Box(c).Size(18, 18).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Icon(iconGitPullRequest, r, pal.inkMuted)
			})
			ui.Text(c, "GitHub Pull Requests").FontSize(textHeadline).
				FontWeight(weightSemibold).TextColor(pal.ink)
		})
		ui.Text(c, "查看当前仓库中开放的 Pull Request，检视代码差异并生成 AI Review 评论。").
			FontSize(textBodySM).TextColor(pal.inkSubtle)

		switch {
		case s.loading:
			ui.Row(c).Height(120).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在从 GitHub 获取 Pull Requests…").
					FontSize(textBodySM).TextColor(pal.inkSubtle)
			})
		case s.err != "":
			ui.Column(c).Gap(10).Children(func() {
				ui.Box(c).Padding(10, 12).Radius(radiusMD).Background(pal.danger.Alpha(0.08)).
					Border(1, pal.danger.Alpha(0.25)).Children(func() {
					ui.Text(c, "获取 PR 列表失败").FontSize(textBodySM).
						FontWeight(weightSemibold).TextColor(pal.danger)
					ui.Text(c, s.err).FontSize(textCaption).TextColor(pal.inkSubtle).Margin(4, 0, 0, 0)
				})
				ui.Row(c).Justify(ui.End).Children(func() {
					if secondaryAction(c, pal, "重试", "", false).Clicked() {
						a.openPRDialog()
					}
				})
			})
		case len(s.items) == 0:
			ui.Column(c).Height(100).Center().Gap(6).Children(func() {
				ui.Text(c, "暂无开放的 Pull Request").FontSize(textBody).TextColor(pal.inkMuted)
				ui.Text(c, "所有 PR 均已合并或关闭，或者当前仓库未包含打开的 PR。").
					FontSize(textCaption).TextColor(pal.inkTertiary)
			})
		default:
			ui.Scroll(c).TrackScroll(&s.scroll).MaxHeight(320).FillWidth().Children(func() {
				ui.Column(c).Gap(6).FillWidth().Children(func() {
					for _, item := range s.items {
						pr := item
						row := ui.ButtonBase(c).FillWidth().Padding(10, 12).Radius(radiusMD).
							Background(pal.surface2).Border(1, pal.hairline).Gap(6).
							Cursor(ui.CursorPointer).Label(fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title))
						if row.Hovered() {
							row.Background(pal.surface3)
						}
						fadeColors(row)
						row.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(8).FillWidth().Children(func() {
								// PR 编号是一枚引用芯片，走 §2.5 的配方。
								specChip(c, pal, fmt.Sprintf("#%d", pr.Number), pal.accentBlue, statusChipH, statusChipText)
								ui.Text(c, pr.Title).FontSize(textBodySM).
									FontWeight(weightMedium).TextColor(pal.ink).Grow(1).SingleLine()
								ui.Text(c, "检视 Diff →").FontSize(textCaption).TextColor(pal.inkSubtle)
							})
							ui.Row(c).AlignItems(ui.Center).Gap(10).Margin(4, 0, 0, 0).Children(func() {
								if pr.Author != "" {
									ui.Text(c, "@"+pr.Author).FontSize(textCaption).TextColor(pal.inkSubtle)
									ui.Text(c, "·").FontSize(textCaption).TextColor(pal.inkTertiary)
								}
								ui.Textf(c, "%s → %s", pr.HeadBranch, pr.BaseBranch).
									Font("monospace").FontSize(textCaption).TextColor(pal.inkSubtle)
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
			if secondaryAction(c, pal, "关闭", "", false).Clicked() {
				a.prDlg.on = false
			}
		})
	})
}

// prReviewBar 渲染在 PR 检视模式下主区顶部的状态及操作条。
func (a *app) prReviewBar(c *ui.Context, pal palette) {
	pr := a.prReview.pr

	// 不铺底：这条长在亚克力工作卡上，surface1 就是卡片色、却是不透明的，
	// 铺上去等于在磨砂上抠出一条实心（和对比栏 compareBar 同一条规矩）。
	ui.Row(c).Height(38).Shrink(0).FillWidth().
		Padding(0, 16).Gap(8).AlignItems(ui.Center).
		Border(1, pal.hairline).Children(func() {

		ui.Box(c).Size(16, 16).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Icon(iconGitPullRequest, r, pal.inkMuted)
		})
		ui.Text(c, "PR 检视").FontSize(textCaption).
			FontWeight(weightSemibold).TextColor(pal.inkSubtle)

		// PR 编号也是「引用类」微件：点和本地分支同一支蓝，不用 Ink ——
		// Ink 是「我在哪」和主干的颜色（§2.4），一枚 PR 号占用它，全屏的
		// 实心强调预算就多花一处。
		specChip(c, pal, fmt.Sprintf("#%d", pr.Number), pal.accentBlue, statusChipH, statusChipText)

		ui.Text(c, pr.Title).FontSize(textBodySM).FontWeight(weightMedium).TextColor(pal.ink).
			Tooltip(fmt.Sprintf("#%d: %s (%s → %s)", pr.Number, pr.Title, pr.HeadBranch, pr.BaseBranch))

		// 分支对（head → base）是一枚等宽信息芯片，不染强调色 ——
		// 它是「这条 PR 从哪儿到哪儿」，读的是那两个名字。
		ui.Box(c).Padding(2, 6).Radius(radiusXS).Background(pal.chip).
			Border(1, pal.hairline).Children(func() {
			ui.Textf(c, "%s → %s", pr.HeadBranch, pr.BaseBranch).
				Font("monospace").FontSize(textCaption).TextColor(pal.inkMuted)
		})

		if pr.Author != "" {
			ui.Text(c, "@"+pr.Author).FontSize(textCaption).TextColor(pal.inkSubtle)
		}

		toolbarDivider(c, pal)

		switch a.prReview.state {
		case loadLoading:
			ui.Spinner(c)
			ui.Text(c, "正在读取 PR Diff…").FontSize(textCaption).TextColor(pal.inkSubtle)
		case loadFailed:
			ui.Text(c, "Diff 读取失败: "+a.prReview.err).FontSize(textCaption).TextColor(pal.danger)
			if ui.ButtonBase(c).Height(22).Padding(0, 6).Radius(radiusSM).Cursor(ui.CursorPointer).Label("重试").
				Children(func() {
					ui.Text(c, "重试").FontSize(textCaption).TextColor(pal.inkMuted)
				}).Clicked() {
				a.openPRReview(pr)
			}
		default:
			ui.Textf(c, "%d 个改动文件", len(a.allFiles)).FontSize(textCaption).TextColor(pal.inkSubtle)
		}

		// ✨ 生成并发布 AI Review 评论：静息态是次级按钮（chip 洗色 + hairline），
		// 悬停才升一档。工具栏上已经有「提交」那种实心主按钮的场合，这里
		// 再来一块蓝底就会打架 —— 而且它并不比「退出检视」更重要。
		reviewBtn := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(radiusMD).Gap(4).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Background(pal.chip).Border(1, pal.hairlineStrong).
			Label("✨ 生成并发布 AI Review 评论").Tooltip("AI 智能分析 PR 改动并提交 Review 评论")
		if reviewBtn.Hovered() {
			reviewBtn.Background(pal.rowSelected)
		}
		fadeColors(reviewBtn)
		reviewBtn.Children(func() {
			sparkleIcon(c, sparkleSize, pal.inkMuted)
			ui.Text(c, "✨ 生成并发布 AI Review 评论").FontSize(textButton).
				FontWeight(weightMedium).TextColor(pal.ink)
		})
		if reviewBtn.Clicked() {
			a.openPRReviewModal()
		}

		// 切换 PR 按钮
		switchBtn := ui.ButtonBase(c).Height(26).Padding(0, 8).Radius(radiusMD).
			Background(ui.Transparent).Border(1, pal.hairline).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Tooltip("查看其他 PR 列表").Label("切换 PR")
		if switchBtn.Hovered() {
			switchBtn.Background(pal.rowHover)
		}
		fadeColors(switchBtn)
		switchBtn.Children(func() {
			ui.Text(c, "切换 PR…").FontSize(textButton).FontWeight(weightMedium).
				TextColor(pal.inkMuted)
		})
		if switchBtn.Clicked() {
			a.openPRDialog()
		}

		ui.Spacer(c)

		// 退出 PR 检视
		exitBtn := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(radiusMD).Gap(4).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Background(ui.Transparent).Border(1, pal.hairline).
			Label("退出 PR 检视").Tooltip("退出 PR 检视并恢复工作区 (Esc)")
		if exitBtn.Hovered() {
			exitBtn.Background(pal.rowHover)
		}
		fadeColors(exitBtn)
		exitBtn.Children(func() {
			ui.Text(c, "✕  退出检视").FontSize(textButton).FontWeight(weightMedium).
				TextColor(pal.inkMuted)
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
	// 表单对话框：点背景不关 —— 评审意见写到一半不该被误触丢掉（见 specModalSticky）。
	specModalSticky(c, pal, &open, func() { a.prReviewModalBody(c, pal) })
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
			sparkleIcon(c, 16, pal.inkMuted)
			ui.Textf(c, "AI Review 评论: #%d %s", pr.Number, pr.Title).
				FontSize(textHeadline).FontWeight(weightSemibold).
				TextColor(pal.ink).Grow(1).SingleLine()
		})
		ui.Text(c, "由 AI 模型结合变更 Diff 自动生成评审建议，支持编辑修改后一键回写至 GitHub PR。").
			FontSize(textBodySM).TextColor(pal.inkSubtle)

		if r.postSuccess {
			// 结果是横幅：语义底色留着（§5 允许语义色），因为这一整条说的
			// 就是「成了」这件事。
			ui.Box(c).Padding(8, 12).Radius(radiusMD).Background(pal.success.Alpha(0.1)).
				Border(1, pal.success.Alpha(0.3)).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Text(c, "✓").FontWeight(weightSemibold).TextColor(pal.success)
					ui.Textf(c, "已成功将 Review 评论发布至 GitHub PR #%d！", pr.Number).
						FontSize(textBodySM).TextColor(pal.success)
				})
			})
		}
		if r.postErr != "" {
			ui.Box(c).Padding(8, 12).Radius(radiusMD).Background(pal.danger.Alpha(0.08)).
				Border(1, pal.danger.Alpha(0.25)).Children(func() {
				ui.Text(c, r.postErr).FontSize(textBodySM).TextColor(pal.danger)
			})
		}

		ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			modeIdx := 0
			if r.editMode {
				modeIdx = 1
			}
			toolbarSegmentedText(c, pal, &modeIdx, false, "Markdown 预览", "编辑原文")
			r.editMode = (modeIdx == 1)

			if r.loading {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Spinner(c)
					ui.Text(c, "正在 AI 生成 Review 评论…").FontSize(textBodySM).TextColor(t.Accent)
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
				Padding(12, 14).Radius(radiusLG).Background(pal.surface2).Border(1, pal.hairline).
				Children(func() {
					if r.reviewText == "" && !r.loading {
						ui.Text(c, "暂无评论内容，点击下方「开始生成」启动 AI 评审。").
							FontSize(textBodySM).TextColor(pal.inkSubtle)
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
										ui.Text(c, "生成中…").FontSize(textCaption).TextColor(pal.inkSubtle)
									})
								}
							})
						})
					}
				})
		}

		ui.Row(c).AlignItems(ui.Center).FillWidth().Children(func() {
			if r.loading {
				if secondaryAction(c, pal, "停止生成", "", false).Clicked() {
					a.cancelAIReview()
				}
			} else {
				regenLabel := "重新生成"
				if r.reviewText == "" {
					regenLabel = "开始生成"
				}
				if secondaryAction(c, pal, regenLabel, "", false).Clicked() {
					a.startAIReview()
				}
			}

			ui.Spacer(c)

			if secondaryAction(c, pal, "关闭", "", false).Clicked() {
				r.reviewModal = false
			}

			publishDisabled := r.loading || r.posting || strings.TrimSpace(r.reviewText) == ""
			publishBtn := primaryAction(c, pal, "发布到 GitHub", "通过 gh CLI 将评论回写至 GitHub PR", publishDisabled)
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
