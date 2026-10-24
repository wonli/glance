package desktopui

import (
	"context"
	"fmt"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/application"
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"image"
	"image/color"
	"strings"
	"unicode/utf8"
)

var (
	proofreadHistoryBackground = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	proofreadHistoryBorder     = color.NRGBA{R: 223, G: 229, B: 234, A: 255}
	proofreadHistorySelected   = color.NRGBA{R: 238, G: 244, B: 248, A: 255}
	proofreadHistoryAdded      = color.NRGBA{R: 35, G: 122, B: 76, A: 255}
	proofreadHistoryDeleted    = color.NRGBA{R: 181, G: 74, B: 74, A: 255}
	proofreadHistoryChanged    = color.NRGBA{R: 123, G: 99, B: 160, A: 255}
)

func (a *App) layoutProofreadingHistoryPage(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, proofreadHistoryBackground, clip.Rect{Max: size}.Op())
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutProofreadingTabChrome),
		layout.Rigid(a.layoutProofreadingHistoryBackBar),
		layout.Flexed(1, a.layoutProofreadingHistoryBody),
	)
}

func (a *App) layoutProofreadingHistoryBackBar(gtx layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Min: image.Pt(0, size.Y-max(1, gtx.Dp(1))), Max: size}.Op())
	button := w.button("history-back")
	if button.Clicked(gtx) {
		a.closeProofreadingHistory(gtx)
	}
	return layout.Inset{Left: 64, Right: 64}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutProofreadingIcon(gtx, "home", "arrow-left", 17, color.NRGBA{R: 66, G: 109, B: 145, A: 255})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Label(a.theme, 13, a.textID("proofread.history.back"))
						label.Color = color.NRGBA{R: 66, G: 109, B: 145, A: 255}
						label.Font.Weight = font.SemiBold
						return layoutProofreadingOpticallyCenteredLabel(gtx, label)
					})
				}),
			)
		})
	})
}

func (a *App) closeProofreadingHistory(gtx layout.Context) {
	w := a.ensureProofreadingWorkbench()
	w.historyRequest++
	w.historyOpen = false
	gtx.Execute(key.FocusCmd{Tag: w.button("workspace-history")})
	gtx.Execute(op.InvalidateCmd{})
}

func (a *App) layoutProofreadingHistoryBody(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutProofreadingHistoryHeading),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 64, Right: 64, Bottom: 63}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(0.302, func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingHistoryPages(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(24), 0)}
					}),
					layout.Flexed(0.698, func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingHistoryTimeline(gtx)
					}),
				)
			})
		}),
	)
}

func (a *App) layoutProofreadingHistoryHeading(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(153))
	gtx.Constraints = layout.Exact(size)
	proofreadingAt(gtx, 64, 38, 480, 40, func(gtx layout.Context) layout.Dimensions {
		return a.proofreadingText(gtx, a.textID("proofread.history.title"), 28, proofreadInk, true)
	})
	proofreadingAt(gtx, 64, 88, 900, 28, func(gtx layout.Context) layout.Dimensions {
		return a.proofreadingText(gtx, a.textID("proofread.history.subtitle"), 13, color.NRGBA{R: 112, G: 128, B: 141, A: 255}, false)
	})
	proofreadingAt(gtx, unit.Dp(float32(size.X)/gtx.Metric.PxPerDp)-323, 46, 259, 40, a.layoutProofreadingHistoryOpenGitDirectory)
	return layout.Dimensions{Size: size}
}

func (a *App) layoutProofreadingHistoryPages(gtx layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(6)).Op(gtx.Ops))
	proofreadRoundedBorder(gtx, size, 6, proofreadHistoryBorder, max(1, gtx.Dp(1)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			headerSize := image.Pt(gtx.Constraints.Max.X, gtx.Dp(65))
			gtx.Constraints = layout.Exact(headerSize)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 230, G: 235, B: 239, A: 255}, clip.Rect{Min: image.Pt(0, headerSize.Y-max(1, gtx.Dp(1))), Max: headerSize}.Op())
			return layout.Inset{Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						label := material.Label(a.theme, 17, a.textID("proofread.history.pages"))
						label.Color = color.NRGBA{R: 44, G: 58, B: 70, A: 255}
						label.Font.Weight = font.SemiBold
						return layoutProofreadingOpticallyCenteredLabel(gtx, label)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Label(a.theme, 11, fmt.Sprintf(a.textID("proofread.history.pages_count"), len(w.historyPages)))
						label.Color = color.NRGBA{R: 128, G: 144, B: 156, A: 255}
						label.MaxLines = 1
						return layoutProofreadingOpticallyCenteredLabel(gtx, label)
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(w.historyPages) == 0 {
				return a.layoutProofreadingHistoryMessage(gtx, a.historyMessageOrDefault("proofread.history.empty"))
			}
			return w.historyPageList.Layout(gtx, len(w.historyPages), func(gtx layout.Context, i int) layout.Dimensions {
				return a.layoutProofreadingHistoryPageRow(gtx, w.historyPages[i])
			})
		}),
	)
}

func (a *App) layoutProofreadingHistoryPageRow(gtx layout.Context, page proofhistory.PageSummary) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	button := w.button(fmt.Sprintf("history-page-%d", page.Page))
	if button.Clicked(gtx) {
		a.selectProofreadingHistoryPage(uint64(page.Page))
		gtx.Execute(op.InvalidateCmd{})
	}
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(85))
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fill := contentSurface
		if uint64(page.Page) == w.historyPage {
			fill = proofreadHistorySelected
		}
		paint.FillShape(gtx.Ops, fill, clip.Rect{Max: size}.Op())
		if uint64(page.Page) == w.historyPage {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 66, G: 109, B: 145, A: 255}, clip.Rect{Max: image.Pt(max(1, gtx.Dp(3)), size.Y)}.Op())
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 236, G: 240, B: 243, A: 255}, clip.Rect{Min: image.Pt(gtx.Dp(24), size.Y-max(1, gtx.Dp(1))), Max: image.Pt(size.X-gtx.Dp(24), size.Y)}.Op())
		proofreadingAt(gtx, 24, 20, 16, 16, func(gtx layout.Context) layout.Dimensions {
			ink := color.NRGBA{R: 138, G: 162, B: 178, A: 255}
			if uint64(page.Page) == w.historyPage {
				ink = color.NRGBA{R: 66, G: 109, B: 145, A: 255}
			}
			return layoutProofreadingIcon(gtx, "home", "file-text", 16, ink)
		})
		pageTitle := fmt.Sprintf(a.textID("proofread.history.page_name"), page.Page)
		proofreadingAt(gtx, 49, 13, unit.Dp(float32(size.X)/gtx.Metric.PxPerDp)-62, 23, func(gtx layout.Context) layout.Dimensions {
			ink := color.NRGBA{R: 54, G: 70, B: 82, A: 255}
			if uint64(page.Page) == w.historyPage {
				ink = color.NRGBA{R: 47, G: 89, B: 118, A: 255}
			}
			label := material.Label(a.theme, 13, pageTitle)
			label.Color, label.MaxLines = ink, 1
			label.Font.Weight = font.SemiBold
			return label.Layout(gtx)
		})
		proofreadingAt(gtx, 49, 40, unit.Dp(min(float32(220), float32(size.X)/gtx.Metric.PxPerDp-170)), 22, func(gtx layout.Context) layout.Dimensions {
			text := fmt.Sprintf(a.textID("proofread.history.latest_commit"), page.Latest.Date)
			label := material.Label(a.theme, 11, text)
			label.Color = color.NRGBA{R: 133, G: 147, B: 158, A: 255}
			label.MaxLines = 1
			return label.Layout(gtx)
		})
		proofreadingAt(gtx, unit.Dp(float32(size.X)/gtx.Metric.PxPerDp)-91, 0, 80, 85, func(gtx layout.Context) layout.Dimensions {
			label := material.Label(a.theme, 10, a.textID("proofread.history.view_history"))
			label.Color = color.NRGBA{R: 66, G: 109, B: 145, A: 255}
			label.MaxLines = 1
			return layoutFlexEnd(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutProofreadingOpticallyCenteredLabel(gtx, label)
			})
		})
		return layout.Dimensions{Size: size}
	})
}

func (a *App) layoutProofreadingHistoryTimeline(gtx layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(6)).Op(gtx.Ops))
	proofreadRoundedBorder(gtx, size, 6, proofreadHistoryBorder, max(1, gtx.Dp(1)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			headerSize := image.Pt(gtx.Constraints.Max.X, gtx.Dp(83))
			gtx.Constraints = layout.Exact(headerSize)
			textWidth := float32(headerSize.X)/gtx.Metric.PxPerDp - 270
			if a.canRestoreProofreadingHistory() {
				textWidth -= 150
			}
			proofreadingAt(gtx, 28, 20, unit.Dp(textWidth), 28, func(gtx layout.Context) layout.Dimensions {
				text := a.textID("proofread.history.select_page")
				if w.historyPage != 0 {
					text = fmt.Sprintf(a.textID("proofread.history.timeline_title"), w.historyPage)
				}
				label := material.Label(a.theme, 21, text)
				label.Color = color.NRGBA{R: 38, G: 53, B: 65, A: 255}
				label.Font.Weight = font.SemiBold
				label.MaxLines = 1
				return label.Layout(gtx)
			})
			proofreadingAt(gtx, 28, 54, unit.Dp(textWidth), 22, func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 11, a.textID("proofread.history.timeline_subtitle"))
				label.Color = color.NRGBA{R: 113, G: 129, B: 141, A: 255}
				label.MaxLines = 1
				return label.Layout(gtx)
			})
			if len(w.history) > 0 {
				proofreadingAt(gtx, unit.Dp(float32(headerSize.X)/gtx.Metric.PxPerDp)-138, 29, 110, 25, func(gtx layout.Context) layout.Dimensions {
					return a.historySecondaryText(gtx, fmt.Sprintf(a.textID("proofread.history.commit_count"), len(w.history)))
				})
				if a.canRestoreProofreadingHistory() {
					proofreadingAt(gtx, unit.Dp(float32(headerSize.X)/gtx.Metric.PxPerDp)-310, 18, 150, 34, a.layoutProofreadingHistoryRestoreButton)
				}
			}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 230, G: 235, B: 239, A: 255}, clip.Rect{Min: image.Pt(gtx.Dp(28), headerSize.Y-max(1, gtx.Dp(1))), Max: image.Pt(headerSize.X-gtx.Dp(28), headerSize.Y)}.Op())
			return layout.Dimensions{Size: headerSize}
		}),
		layout.Rigid(a.layoutProofreadingHistoryTimelineLegend),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if w.historyLoading {
				return a.layoutProofreadingHistoryMessage(gtx, a.textID("proofread.history.loading"))
			}
			if w.historyMessage != "" {
				return a.layoutProofreadingHistoryMessage(gtx, w.historyMessage)
			}
			if w.historyPage == 0 || len(w.history) == 0 {
				return a.layoutProofreadingHistoryMessage(gtx, a.textID("proofread.history.page_empty"))
			}
			return layout.Inset{Left: 28, Right: 28, Top: 3, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.historyTimelineList.Layout(gtx, len(w.history), func(gtx layout.Context, i int) layout.Dimensions {
					return a.layoutProofreadingHistoryCommit(gtx, w.history[i])
				})
			})
		}),
	)
}

func (a *App) layoutProofreadingHistoryTimelineLegend(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(38))
	gtx.Constraints = layout.Exact(size)
	return layout.Inset{Left: 28, Right: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 13, a.textID("proofread.history.timeline"))
				label.Color = color.NRGBA{R: 52, G: 70, B: 84, A: 255}
				label.Font.Weight = font.SemiBold
				return layout.Inset{Top: 15}.Layout(gtx, label.Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.historyLegendText(gtx, "proofread.history.legend_added", "+", proofreadHistoryAdded)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.historyLegendText(gtx, "proofread.history.legend_deleted", "−", proofreadHistoryDeleted)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.historyLegendText(gtx, "proofread.history.legend_changed", "±", proofreadHistoryChanged)
			}),
		)
	})
}

func (a *App) historyLegendText(gtx layout.Context, key, prefix string, ink color.NRGBA) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(83), gtx.Constraints.Max.Y))
	label := material.Label(a.theme, 11, prefix+" "+a.textID(key))
	label.Color = ink
	label.Font.Weight = font.SemiBold
	label.MaxLines = 1
	return layout.Inset{Top: 16, Right: 8}.Layout(gtx, label.Layout)
}

func (a *App) layoutProofreadingHistoryCommit(gtx layout.Context, change proofhistory.PageChange) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	buttonID := "history-commit-" + change.Entry.Hash[:min(7, len(change.Entry.Hash))]
	button := w.button(buttonID)
	if button.Clicked(gtx) {
		w.selectedCommit = change.Entry.Hash
		gtx.Execute(op.InvalidateCmd{})
	}
	width := unit.Dp(float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp)
	return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Measure the complete patch before drawing the card so the outer list
		// can scroll through every line, including wrapped long lines.
		diffContext := gtx
		diffContext.Constraints.Min = image.Point{}
		diffContext.Constraints.Max.X = max(0, gtx.Constraints.Max.X-gtx.Dp(32))
		record := op.Record(gtx.Ops)
		diffDims := a.layoutProofreadingHistoryContent(diffContext, change)
		diffCall := record.Stop()
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(101)+diffDims.Size.Y)
		gtx.Constraints = layout.Exact(size)
		return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			fill := color.NRGBA{R: 251, G: 252, B: 253, A: 255}
			border := proofreadHistoryBorder
			if w.selectedCommit == change.Entry.Hash {
				fill = color.NRGBA{R: 247, G: 250, B: 252, A: 255}
				border = color.NRGBA{R: 189, G: 209, B: 222, A: 255}
			}
			paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(5)).Op(gtx.Ops))
			proofreadRoundedBorder(gtx, size, 5, border, max(1, gtx.Dp(1)))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 66, G: 109, B: 145, A: 255}, clip.Rect{Max: image.Pt(max(1, gtx.Dp(3)), size.Y)}.Op())
			proofreadingAt(gtx, 16, 11, width-418, 22, func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 14, change.Entry.Message)
				label.Color = color.NRGBA{R: 38, G: 63, B: 80, A: 255}
				label.Font.Weight = font.SemiBold
				label.MaxLines = 1
				return label.Layout(gtx)
			})
			proofreadingAt(gtx, width-302, 9, 82, 22, func(gtx layout.Context) layout.Dimensions {
				return a.layoutProofreadingHistoryStatusBadge(gtx, change.Status)
			})
			proofreadingAt(gtx, width-96, 13, 78, 20, func(gtx layout.Context) layout.Dimensions {
				return a.historyMonoText(gtx, shortProofreadingCommit(change.Entry.Hash), color.NRGBA{R: 87, G: 127, B: 152, A: 255})
			})
			proofreadingAt(gtx, 16, 37, width-32, 18, func(gtx layout.Context) layout.Dimensions {
				return a.historySecondaryText(gtx, fmt.Sprintf(a.textID("proofread.history.author_date"), change.Entry.Author, change.Entry.Date))
			})
			paint.FillShape(gtx.Ops, color.NRGBA{R: 230, G: 235, B: 239, A: 255}, clip.Rect{Min: image.Pt(gtx.Dp(16), gtx.Dp(62)), Max: image.Pt(size.X-gtx.Dp(16), gtx.Dp(63))}.Op())
			proofreadingAt(gtx, 16, 71, width-382, 20, func(gtx layout.Context) layout.Dimensions {
				return a.historyMonoText(gtx, strings.Join(change.Paths, ", "), color.NRGBA{R: 64, G: 85, B: 98, A: 255})
			})
			proofreadingAt(gtx, width-322, 71, 304, 20, func(gtx layout.Context) layout.Dimensions {
				return a.layoutProofreadingHistoryChangeStats(gtx, change)
			})
			offset := op.Offset(image.Pt(gtx.Dp(16), gtx.Dp(91))).Push(gtx.Ops)
			diffCall.Add(gtx.Ops)
			offset.Pop()
			return layout.Dimensions{Size: size}
		})
	})
}

func (a *App) layoutProofreadingHistoryChangeStats(gtx layout.Context, change proofhistory.PageChange) layout.Dimensions {
	statsKey := "proofread.history.change_stats_many"
	if change.FilesChanged == 1 {
		statsKey = "proofread.history.change_stats_one"
	}
	stats := fmt.Sprintf(a.textID(statsKey), change.Additions, change.Deletions, change.FilesChanged)
	if change.TextAvailable {
		added, deleted := 0, 0
		for _, span := range change.TextChanges {
			if span.Kind == proofhistory.TextAdded {
				added += utf8.RuneCountInString(span.Text)
			}
			if span.Kind == proofhistory.TextDeleted {
				deleted += utf8.RuneCountInString(span.Text)
			}
		}
		stats = fmt.Sprintf(a.textID("proofread.history.text_stats"), added, deleted)
	}
	ink := proofreadHistoryChanged
	switch change.Status {
	case "added":
		ink = proofreadHistoryAdded
	case "deleted":
		ink = proofreadHistoryDeleted
	}
	label := material.Label(a.theme, 10, stats)
	label.Color = ink
	label.Font.Weight = font.SemiBold
	label.MaxLines = 1
	return layoutFlexEnd(gtx, label.Layout)
}

func (a *App) layoutProofreadingHistoryStatusBadge(gtx layout.Context, status string) layout.Dimensions {
	ink, fill, key := proofreadHistoryChanged, color.NRGBA{R: 240, G: 236, B: 248, A: 255}, "proofread.history.status_modified"
	switch status {
	case "added":
		ink, fill, key = proofreadHistoryAdded, color.NRGBA{R: 228, G: 244, B: 233, A: 255}, "proofread.history.status_added"
	case "deleted":
		ink, fill, key = proofreadHistoryDeleted, color.NRGBA{R: 251, G: 234, B: 234, A: 255}, "proofread.history.status_deleted"
	}
	size := image.Pt(gtx.Dp(82), gtx.Dp(22))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(3)).Op(gtx.Ops))
	label := material.Label(a.theme, 10, a.textID(key))
	label.Color = ink
	label.MaxLines = 1
	return layout.Center.Layout(gtx, label.Layout)
}

func (a *App) layoutProofreadingHistoryDiff(gtx layout.Context, diff string) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	lines := strings.Split(diff, "\n")
	if diff == "" {
		lines = []string{a.textID("proofread.history.diff_empty")}
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, color.NRGBA{R: 247, G: 249, B: 250, A: 255}, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(3)).Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 8, Right: 8, Top: 3, Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(lines))
				for _, line := range lines {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingHistoryDiffLine(gtx, line)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		},
	)
}

func (a *App) layoutProofreadingHistoryDiffLine(gtx layout.Context, text string) layout.Dimensions {
	ink := proofreadMuted
	if strings.HasPrefix(text, "+") {
		ink = proofreadHistoryAdded
	} else if strings.HasPrefix(text, "-") {
		ink = proofreadHistoryDeleted
	}
	label := material.Label(a.theme, 10, text)
	label.Color = ink
	label.Font.Typeface = jsonMonoFace
	return label.Layout(gtx)
}

func (a *App) layoutProofreadingHistoryRestoreButton(gtx layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	size := image.Pt(gtx.Dp(150), gtx.Dp(34))
	gtx.Constraints = layout.Exact(size)
	button := w.button("restore-history")
	if button.Clicked(gtx) && !w.busy {
		a.restoreProofreadingHistory()
		gtx.Execute(op.InvalidateCmd{})
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, proofreadingDesignBlue, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(5)).Op(gtx.Ops))
		label := material.Label(a.theme, 11, a.textID("proofread.history.restore"))
		label.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		label.MaxLines = 1
		return layout.Center.Layout(gtx, label.Layout)
	})
}

func (a *App) canRestoreProofreadingHistory() bool {
	w := a.ensureProofreadingWorkbench()
	s := a.proofreading
	return w.selectedCommit != "" && w.historyPage > 0 && s != nil && s.controller != nil && s.projectDocumentID == w.historyProject.ID
}

func (a *App) layoutProofreadingHistoryMessage(gtx layout.Context, message string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Body2(a.theme, message)
		label.Color = proofreadMuted
		return label.Layout(gtx)
	})
}

func (a *App) historyMessageOrDefault(key string) string {
	w := a.ensureProofreadingWorkbench()
	if w.historyMessage != "" {
		return w.historyMessage
	}
	if w.historyLoading {
		return a.textID("proofread.history.loading")
	}
	return a.textID(key)
}

func (a *App) historySecondaryText(gtx layout.Context, text string) layout.Dimensions {
	label := material.Label(a.theme, 11, text)
	label.Color = color.NRGBA{R: 128, G: 144, B: 156, A: 255}
	label.MaxLines = 1
	return label.Layout(gtx)
}

func (a *App) historyMonoText(gtx layout.Context, text string, ink color.NRGBA) layout.Dimensions {
	label := material.Label(a.theme, 11, text)
	label.Color = ink
	label.Font.Typeface = jsonMonoFace
	label.Font.Weight = font.SemiBold
	label.MaxLines = 1
	return label.Layout(gtx)
}

func layoutFlexEnd(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
		}),
		layout.Rigid(child),
	)
}

func shortProofreadingCommit(hash string) string {
	return hash[:min(7, len(hash))]
}

func (a *App) restoreProofreadingHistory() {
	w := a.ensureProofreadingWorkbench()
	s := a.proofreading
	if s == nil || s.controller == nil || s.projectDocumentID != w.historyProject.ID || w.historyPage == 0 {
		w.historyMessage = a.textID("proofread.history.open_page")
		return
	}
	for i := range s.xPageEditors {
		if editorHasActiveComposition(&s.xPageEditors[i]) {
			w.historyMessage = a.textID("proofread.history.ime")
			return
		}
	}
	p, page, commit := w.historyProject, proofreading.PageID(w.historyPage), w.selectedCommit
	captured := s.controller.Model.Capture()
	revision := s.controller.Model.Revision()
	taskID, sourceHash := s.projectDocumentID, s.aSourceHash
	a.proofreadingWorkbenchJob(func() func() {
		old, err := (proofhistory.Repository{Directory: p.Directory}).ReadPage(context.Background(), commit, page)
		var model *proofreading.Model
		if err == nil && (old.ProjectID != taskID || old.SourceHash != sourceHash) {
			err = fmt.Errorf("source mismatch")
		}
		if err == nil {
			var candidate proofreading.Snapshot
			candidate, err = application.RestoreProofreadingPage(captured.Snapshot(), old)
			if err == nil {
				model, err = proofreading.Restore(candidate)
			}
		}
		return func() {
			if err != nil {
				w.historyMessage = a.textID("proofread.history.conflict")
				return
			}
			if err = s.controller.Model.AcceptRestored(model, revision); err != nil {
				w.historyMessage = a.textID("proofread.history.conflict")
				return
			}
			a.proofreadingModelChanged(false)
			w.active = false
			w.historyRequest++
			w.historyOpen = false
		}
	})
}
