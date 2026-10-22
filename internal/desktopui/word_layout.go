package desktopui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

func (a *App) layoutWordFlow(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return layout.Dimensions{}
	}
	a.startWordPagination(state)
	blocks := wordBlocksForState(state)
	state.wordListState.List.Axis = layout.Vertical
	state.wordToolbarTabList.List.Axis = layout.Horizontal
	if state.wordCanvasWidthDP <= 0 {
		state.wordCanvasWidthDP = wordPaperMaxWidthDP
	}
	state.wordLayoutFocusedRunID = ""
	state.wordNavigationVerticalHandledRunID = ""
	state.wordNavLayoutsNext = make(map[string]wordNavigationGeometry)
	dimensions := layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.wordReadingMode {
						return a.layoutWordReadingBar(gtx, state)
					}
					return a.layoutWordRibbon(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutWordBodyArea(gtx, state, blocks)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordStatusBar(gtx, state)
				}),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if state.wordReadingMode {
				return layout.Dimensions{}
			}
			return a.layoutWordToolbarColorPicker(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if state.wordReadingMode {
				return layout.Dimensions{}
			}
			return a.layoutOfficeRibbonDropdown(gtx)
		}),
	)
	state.wordNavLayouts = state.wordNavLayoutsNext
	state.wordNavLayoutsNext = nil
	if focused := state.wordLayoutFocusedRunID; focused != "" {
		if wordHasPendingTextDrafts(state) && (focused != state.wordFocusedRunID || state.wordForceDraftFlush.Load()) {
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(state.wordFlushAllDrafts.Load() || state.wordForceDraftFlush.Load())
		}
		state.wordFocusedRunID = focused
	} else {
		if wordHasPendingTextDrafts(state) {
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
		}
		state.wordFocusedRunID = ""
	}
	state.wordForceDraftFlush.Store(false)
	if state.wordFlushRequested.Load() && !state.formatRunning.Load() {
		a.startNextWordTextCommit()
	}
	return dimensions
}

const (
	wordPaperMaxWidthDP      = 900
	wordPaperHeightDP        = 600
	wordPaperTopInsetDP      = 16
	wordPaperBottomDP        = 35
	wordWebContentMaxWidthDP = 1200
)
const wordCanvasMinWidthDP = 480

func wordPageGeometry(availableWidth, maxWidth int) (pageWidth, left int) {
	pageWidth = min(max(0, availableWidth), max(0, maxWidth))
	left = max(0, (availableWidth-pageWidth)/2)
	return pageWidth, left
}

func wordWebContentInset(canvasWidthDP int) int {
	inset := min(48, max(16, (canvasWidthDP-480)/2))
	contentWidth := max(0, canvasWidthDP-2*inset)
	return inset + max(0, (contentWidth-wordWebContentMaxWidthDP)/2)
}

func wordPageHorizontalMargin(pageWidth, preferred int) int {
	return min(max(0, preferred), max(0, (pageWidth-160)/2))
}

func (a *App) layoutWordCanvas(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	state.wordListState.List.Axis = layout.Vertical
	state.wordCanvasContentClickHandled = false
	webMode := state.wordViewMode == wordViewWebLayout
	if state.wordCanvasWidthDP <= 0 {
		state.wordCanvasWidthDP = wordPaperMaxWidthDP
	}
	availableWidth := gtx.Constraints.Max.X
	preferredWidth := gtx.Dp(unit.Dp(state.wordCanvasWidthDP))
	canvasWidthPx, canvasLeftPx := wordPageGeometry(availableWidth, preferredWidth)
	if webMode {
		canvasWidthPx, canvasLeftPx = wordPageGeometry(availableWidth, availableWidth)
	} else {
		a.updateWordCanvasResize(gtx, state, canvasLeftPx, canvasLeftPx+canvasWidthPx)
	}
	pageMarginLeftDP, pageMarginRightDP, pageMarginTopDP := wordCanvasPageMarginsDP(gtx, state)
	paperTop, paperBottom := 0, 0
	pageHeight := gtx.Constraints.Max.Y
	if !webMode {
		paperTop = min(gtx.Dp(unit.Dp(wordPaperTopInsetDP)), gtx.Constraints.Max.Y)
		paperBottom = min(gtx.Dp(unit.Dp(wordPaperBottomDP)), max(0, gtx.Constraints.Max.Y-paperTop))
		pageHeight = min(max(0, gtx.Constraints.Max.Y-paperTop-paperBottom), gtx.Dp(unit.Dp(wordPaperHeightDP)))
	}
	listViewportHeight := max(0, pageHeight-gtx.Dp(unit.Dp(56)))
	const scrollLimit = 1 << 30
	if delta := state.wordBodyScroll.Update(
		gtx.Metric,
		gtx.Source,
		gtx.Now,
		gesture.Vertical,
		pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	); delta != 0 {
		state.wordListState.Position.Offset += delta
	}
	dimensions := layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.NW}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(float32(paperTop) / max32(0.1, gtx.Metric.PxPerDp)), Bottom: unit.Dp(float32(paperBottom) / max32(0.1, gtx.Metric.PxPerDp))}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						leftDP := float32(canvasLeftPx) / max32(0.1, gtx.Metric.PxPerDp)
						return layout.Inset{Left: unit.Dp(leftDP)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							layoutWidthPx := min(gtx.Constraints.Max.X, canvasWidthPx)
							pageHeight = min(gtx.Constraints.Max.Y, pageHeight)
							layoutWidthPx = max(0, layoutWidthPx)
							gtx.Constraints.Min.X, gtx.Constraints.Max.X = layoutWidthPx, layoutWidthPx
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = pageHeight, pageHeight
							marginLeft, marginRight := gtx.Dp(unit.Dp(pageMarginLeftDP)), gtx.Dp(unit.Dp(pageMarginRightDP))
							marginTopDP := pageMarginTopDP
							if webMode {
								canvasWidthDP := int(float32(layoutWidthPx) / max32(0.1, gtx.Metric.PxPerDp))
								webMargin := gtx.Dp(unit.Dp(wordWebContentInset(canvasWidthDP)))
								marginLeft, marginRight = webMargin, webMargin
								marginTopDP = 0
							}
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
									if state.wordGridlinesVisible && !state.wordReadingMode {
										drawWordGridlines(gtx, a, image.Rectangle{Max: gtx.Constraints.Max})
									}
									canvasClip := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
									if webMode {
										semantic.DescriptionOp(a.textID("word.accessibility.web_region")).Add(gtx.Ops)
									} else {
										semantic.DescriptionOp(a.textID("word.accessibility.canvas")).Add(gtx.Ops)
									}
									canvasClip.Pop()
									return layout.Dimensions{Size: gtx.Constraints.Max}
								},
								func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(float32(marginTopDP)), Bottom: unit.Dp(28), Left: unit.Dp(float32(marginLeft) / max32(0.1, gtx.Metric.PxPerDp)), Right: unit.Dp(float32(marginRight) / max32(0.1, gtx.Metric.PxPerDp))}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return a.layoutWordList(gtx, state, blocks)
									})
								},
							)
						})
					})
				}),
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
					pass := pointer.PassOp{}.Push(gtx.Ops)
					state.wordCanvasBlankClick.Add(gtx.Ops)
					state.wordBodyScroll.Add(gtx.Ops)
					pass.Pop()
					area.Pop()
					return layout.Dimensions{Size: gtx.Constraints.Max}
				}),
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					if state.wordReadingMode || webMode {
						return layout.Dimensions{Size: gtx.Constraints.Max}
					}
					hitWidth := max(1, gtx.Dp(unit.Dp(8)))
					pageBottom := paperTop + pageHeight
					for _, edge := range []int{canvasLeftPx, canvasLeftPx + canvasWidthPx} {
						area := clip.Rect{Min: image.Pt(max(0, edge-hitWidth), paperTop), Max: image.Pt(min(gtx.Constraints.Max.X, edge+hitWidth), pageBottom)}.Push(gtx.Ops)
						pointer.CursorColResize.Add(gtx.Ops)
						state.wordCanvasResizeDrag.Add(gtx.Ops)
						area.Pop()
					}
					return layout.Dimensions{Size: gtx.Constraints.Max}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					start, end := wordListViewportRange(state.wordListState.Position, len(blocks)+1, listViewportHeight)
					trackWidth := gtx.Dp(material.Scrollbar(a.theme, &state.wordListState.Scrollbar).Width())
					trackClip := clip.Rect{Min: image.Pt(max(0, gtx.Constraints.Max.X-trackWidth), 0), Max: gtx.Constraints.Max}.Push(gtx.Ops)
					semantic.DescriptionOp(a.textID("word.accessibility.scrollbar")).Add(gtx.Ops)
					trackClip.Pop()
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						dimensions := material.Scrollbar(a.theme, &state.wordListState.Scrollbar).Layout(gtx, layout.Vertical, start, end)
						if delta := state.wordListState.ScrollDistance(); delta != 0 {
							state.wordListState.List.ScrollBy(delta * float32(len(blocks)+1))
						}
						return dimensions
					})
				}),
			)
		},
	)
	for event, ok := state.wordCanvasBlankClick.Update(gtx.Source); ok; event, ok = state.wordCanvasBlankClick.Update(gtx.Source) {
		if event.Kind != gesture.KindClick || state.wordCanvasContentClickHandled || state.wordReadingMode || state.wordProtected || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() {
			continue
		}
		state.wordTableSelection = nil
		state.wordTableSelectionDrag = nil
		if state.selectedWordImageID != "" {
			clearWordImageSelection(state)
		}
		if !a.focusLastEditableWordRun(gtx, state) {
			gtx.Execute(key.FocusCmd{Tag: &state.wordCanvasBlankClick})
		}
	}
	return dimensions
}

func wordCanvasPageMarginsDP(gtx layout.Context, state *documentSessionState) (left, right, top int) {
	setup := defaultWordPageSetup
	if state != nil && state.wordLayoutResult != nil && len(state.wordLayoutResult.Pages) > 0 {
		setup = state.wordLayoutResult.Pages[0].PageSetup
	}
	pointsToDP := func(points float64) int {
		return int(math.Round(points * (4.0 / 3.0)))
	}
	return pointsToDP(setup.MarginLeftPT), pointsToDP(setup.MarginRightPT), pointsToDP(setup.MarginTopPT)
}

func (a *App) updateWordCanvasResize(gtx layout.Context, state *documentSessionState, left, right int) {
	for event, ok := state.wordCanvasResizeDrag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = state.wordCanvasResizeDrag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		state.wordCanvasContentClickHandled = true
		switch event.Kind {
		case pointer.Press:
			const edgeHitWidth = 14
			state.wordCanvasResizeSide = 0
			if math.Abs(float64(event.Position.X-float32(left))) <= edgeHitWidth {
				state.wordCanvasResizeSide = -1
			} else if math.Abs(float64(event.Position.X-float32(right))) <= edgeHitWidth {
				state.wordCanvasResizeSide = 1
			}
			if state.wordCanvasResizeSide != 0 {
				state.wordCanvasResizeStartX = event.Position.X
				maxWidthDP := max(1, int(math.Floor(float64(gtx.Constraints.Max.X)/float64(max32(0.1, gtx.Metric.PxPerDp)))))
				state.wordCanvasResizeStartWidthDP = min(state.wordCanvasWidthDP, maxWidthDP)
			}
		case pointer.Drag:
			if state.wordCanvasResizeSide == 0 {
				continue
			}
			pixelsPerDP := max32(0.1, gtx.Metric.PxPerDp)
			delta := int(math.Round(float64(event.Position.X-state.wordCanvasResizeStartX) / float64(pixelsPerDP)))
			width := state.wordCanvasResizeStartWidthDP + state.wordCanvasResizeSide*delta*2
			maxWidth := max(1, int(math.Floor(float64(gtx.Constraints.Max.X)/float64(pixelsPerDP))))
			minWidth := min(wordCanvasMinWidthDP, maxWidth)
			state.wordCanvasWidthDP = max(minWidth, min(maxWidth, width))
		case pointer.Release, pointer.Cancel:
			state.wordCanvasResizeSide = 0
		}
	}
}

var wordWorkspaceBackground = color.NRGBA{R: 238, G: 241, B: 245, A: 255}

func wordReadOnlyPlaceholder(gtx layout.Context, theme *material.Theme, message string) layout.Dimensions {
	label := material.Body2(theme, message)
	label.Color = mutedText
	return wordMeasuredBackground(gtx, gridHeaderFill, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(7)).Layout(gtx, label.Layout)
		})
	})
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
