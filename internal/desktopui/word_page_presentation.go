package desktopui

import (
	"fmt"
	"image"
	"math"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/internal/wordlayout"
)

type wordPagePresentation uint8

const (
	wordPageContinuous wordPagePresentation = iota
	wordPagePrint
)

const (
	wordPageBottomGapDP   = 24
	wordPageTopGapDP      = 16
	wordPageContinuousGap = 4
	wordMultiplePageGapDP = 24
)

// layoutWordPages paints the canonical page scenes in either presentation.
// Content geometry always comes from wordlayout; only surfaces and gaps vary.
func (a *App) layoutWordPages(gtx layout.Context, state *documentSessionState, presentation wordPagePresentation) layout.Dimensions {
	beginWordPageInteractions(state)
	defer a.finishWordPageInteractions(state)
	state.wordCanvasContentClickHandled = false
	state.wordPageListState.List.Axis = layout.Vertical
	paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	result := state.wordLayoutResult
	if result == nil && state.wordLayoutError != "" {
		if state.wordGridlinesVisible && !state.wordReadingMode {
			drawWordGridlines(gtx, a, image.Rectangle{Max: gtx.Constraints.Max})
		}
		return layoutWordPageMessage(gtx, a.theme, "分页失败："+state.wordLayoutError, wordPageErrorSurface)
	}
	if result == nil {
		message := "正在计算页面布局…"
		surface := wordPageWarningSurface
		if state.wordLayoutPendingKey != nil {
			surface = wordWorkspaceBackground
		} else {
			message = "页面数尚未计算"
		}
		if state.wordGridlinesVisible && !state.wordReadingMode {
			drawWordGridlines(gtx, a, image.Rectangle{Max: gtx.Constraints.Max})
		}
		return layoutWordPageMessage(gtx, a.theme, message, surface)
	}
	if !result.CanPreview {
		message := "此文档包含暂不支持的版式，无法保证页面与打印结果一致。"
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Severity == wordlayout.DiagnosticError {
				message = diagnostic.Detail
				break
			}
		}
		return layoutWordPageMessage(gtx, a.theme, message, wordPageErrorSurface)
	}
	if len(result.Pages) == 0 {
		return layoutWordPageMessage(gtx, a.theme, "文档没有可显示的页面。", wordPageWarningSurface)
	}
	columns, columnGap := 1, 0
	if presentation == wordPagePrint && state.wordMultiplePagesMode {
		columns = 2
		columnGap = gtx.Dp(unit.Dp(wordMultiplePageGapDP))
	}
	if pendingRunID := state.wordPagePendingScrollRunID; pendingRunID != "" {
		if page, ok := wordPageForCaret(*result, pendingRunID, wordPageCaretOffset(state, pendingRunID)); ok {
			firstRow := page
			if columns == 2 {
				firstRow /= 2
			}
			setWordPageListFirst(state, firstRow)
		}
		state.wordPagePendingScrollRunID = ""
	}
	pageIndex := state.wordPageListState.Position.First * columns
	pageIndex = max(0, min(pageIndex, len(result.Pages)-1))
	if state.wordPageFitMode {
		verticalGap := wordPageTopGapDP + wordPageBottomGapDP
		if presentation == wordPageContinuous {
			verticalGap = 0
		}
		availableWidth := float64(max(1, gtx.Constraints.Max.X-gtx.Dp(unit.Dp(48))-columnGap))
		availableHeight := float64(max(1, gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(verticalGap))))
		fitKey := ""
		if state.wordLayoutAcceptedKey != nil {
			fitKey = state.wordLayoutAcceptedKey.PageSetupKey
		}
		page := result.Pages[pageIndex]
		fitKey += fmt.Sprintf("|%.3f|%.3f", page.Clip.WidthPT, page.Clip.HeightPT)
		if state.wordPageFitWidth != int(availableWidth) || state.wordPageFitHeight != int(availableHeight) || state.wordPageFitColumns != columns || state.wordPageFitPageSetupKey != fitKey {
			state.wordLayoutZoom = wordPageFitZoomForColumns(page, availableWidth, availableHeight, float64(gtx.Metric.PxPerDp), columns)
			state.wordPageFitWidth = int(availableWidth)
			state.wordPageFitHeight = int(availableHeight)
			state.wordPageFitColumns = columns
			state.wordPageFitPageSetupKey = fitKey
			state.wordPageScrollX = 0
		}
	}

	rowCount := len(result.Pages)
	if columns == 2 {
		rowCount = (rowCount + 1) / 2
	}
	dimensions := state.wordPageListState.Layout(gtx, rowCount, func(gtx layout.Context, rowIndex int) layout.Dimensions {
		firstPageIndex := rowIndex * columns
		first := result.Pages[firstPageIndex]
		scale := wordScenePixelsPerPoint(gtx, first, state.wordLayoutZoom)
		firstWidth := max(1, int(math.Ceil(first.Clip.WidthPT*scale)))
		firstHeight := max(1, int(math.Ceil(first.Clip.HeightPT*scale)))
		secondIndex := firstPageIndex + 1
		hasSecond := columns == 2 && secondIndex < len(result.Pages)
		secondWidth, secondHeight := 0, 0
		if hasSecond {
			second := result.Pages[secondIndex]
			secondScale := wordScenePixelsPerPoint(gtx, second, state.wordLayoutZoom)
			secondWidth = max(1, int(math.Ceil(second.Clip.WidthPT*secondScale)))
			secondHeight = max(1, int(math.Ceil(second.Clip.HeightPT*secondScale)))
		}

		topGap, bottomGap := 0, 0
		separator := gtx.Dp(unit.Dp(wordPageContinuousGap))
		if presentation == wordPagePrint {
			topGap = gtx.Dp(unit.Dp(wordPageTopGapDP))
			bottomGap = gtx.Dp(unit.Dp(wordPageBottomGapDP))
			separator = 0
		}
		rowHeight := topGap + max(firstHeight, secondHeight) + bottomGap + separator
		rowGtx := gtx
		rowGtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, rowHeight)
		rowGtx.Constraints.Max = rowGtx.Constraints.Min
		paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: rowGtx.Constraints.Max}.Op())
		totalWidth := firstWidth
		if hasSecond {
			totalWidth += columnGap + secondWidth
		}
		horizontalOverflow := max(0, totalWidth-rowGtx.Constraints.Max.X)
		state.wordPageScrollX = math.Max(0, math.Min(float64(horizontalOverflow), state.wordPageScrollX))
		left := max(0, (rowGtx.Constraints.Max.X-totalWidth)/2) - int(math.Round(state.wordPageScrollX))
		firstRect := image.Rect(left, topGap, left+firstWidth, topGap+firstHeight)
		a.layoutWordPageInteraction(gtx, state, first, firstPageIndex, firstRect, scale, horizontalOverflow)
		a.drawWordPageSurface(gtx, state, first, firstPageIndex, len(result.Pages), firstRect, scale, presentation)
		if hasSecond {
			second := result.Pages[secondIndex]
			secondScale := wordScenePixelsPerPoint(gtx, second, state.wordLayoutZoom)
			secondRect := image.Rect(firstRect.Max.X+columnGap, topGap, firstRect.Max.X+columnGap+secondWidth, topGap+secondHeight)
			a.layoutWordPageInteraction(gtx, state, second, secondIndex, secondRect, secondScale, horizontalOverflow)
			a.drawWordPageSurface(gtx, state, second, secondIndex, len(result.Pages), secondRect, secondScale, presentation)
		}
		if separator > 0 {
			lineY := topGap + max(firstHeight, secondHeight) + max(0, separator/2)
			paint.FillShape(gtx.Ops, wordPageTableLine, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(rowGtx.Constraints.Max.X, lineY+1)}.Op())
		}
		return layout.Dimensions{Size: rowGtx.Constraints.Max}
	})
	start, end := wordListViewportRange(state.wordPageListState.Position, rowCount, gtx.Constraints.Max.Y)
	if rowCount > 1 && end-start < 1 {
		style := material.Scrollbar(a.theme, &state.wordPageListState.Scrollbar)
		trackWidth := gtx.Dp(style.Width())
		trackClip := clip.Rect{Min: image.Pt(max(0, gtx.Constraints.Max.X-trackWidth), 0), Max: gtx.Constraints.Max}.Push(gtx.Ops)
		semantic.DescriptionOp(a.textID("word.accessibility.page_scrollbar")).Add(gtx.Ops)
		trackClip.Pop()
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			dimensions := style.Layout(gtx, layout.Vertical, start, end)
			if delta := state.wordPageListState.ScrollDistance(); delta != 0 {
				state.wordTargetRestorePage = -1
				state.wordPageListState.List.ScrollBy(delta * float32(rowCount))
			}
			return dimensions
		})
	}
	a.syncWordVisiblePageInteractions(gtx, state, result, rowCount, columns, columnGap, presentation)
	state.wordLayoutActivePage = state.wordPageListState.Position.First * columns
	return dimensions
}

func wordPageCaretOffset(state *documentSessionState, runID string) int {
	if state != nil {
		if editor := state.wordRunEditors[runID]; editor != nil {
			_, caret := editor.Selection()
			return caret
		}
	}
	return 0
}

func (a *App) syncWordVisiblePageInteractions(gtx layout.Context, state *documentSessionState, result *wordlayout.Result, rowCount, columns, columnGap int, presentation wordPagePresentation) {
	if state == nil || result == nil {
		return
	}
	state.wordPageVisibleInteractionsNext = state.wordPageVisibleInteractionsNext[:0]
	firstRow := max(0, state.wordPageListState.Position.First)
	lastRow := min(rowCount, firstRow+max(0, state.wordPageListState.Position.Count))
	rowY := -state.wordPageListState.Position.Offset
	for rowIndex := firstRow; rowIndex < lastRow; rowIndex++ {
		firstPageIndex := rowIndex * columns
		if firstPageIndex < 0 || firstPageIndex >= len(result.Pages) {
			break
		}
		first := result.Pages[firstPageIndex]
		firstScale := wordScenePixelsPerPoint(gtx, first, state.wordLayoutZoom)
		firstWidth := max(1, int(math.Ceil(first.Clip.WidthPT*firstScale)))
		firstHeight := max(1, int(math.Ceil(first.Clip.HeightPT*firstScale)))
		secondIndex := firstPageIndex + 1
		hasSecond := columns == 2 && secondIndex < len(result.Pages)
		secondWidth, secondHeight := 0, 0
		if hasSecond {
			second := result.Pages[secondIndex]
			secondScale := wordScenePixelsPerPoint(gtx, second, state.wordLayoutZoom)
			secondWidth = max(1, int(math.Ceil(second.Clip.WidthPT*secondScale)))
			secondHeight = max(1, int(math.Ceil(second.Clip.HeightPT*secondScale)))
		}
		topGap, bottomGap := 0, 0
		separator := gtx.Dp(unit.Dp(wordPageContinuousGap))
		if presentation == wordPagePrint {
			topGap = gtx.Dp(unit.Dp(wordPageTopGapDP))
			bottomGap = gtx.Dp(unit.Dp(wordPageBottomGapDP))
			separator = 0
		}
		rowHeight := topGap + max(firstHeight, secondHeight) + bottomGap + separator
		totalWidth := firstWidth
		if hasSecond {
			totalWidth += columnGap + secondWidth
		}
		left := max(0, (gtx.Constraints.Max.X-totalWidth)/2) - int(math.Round(state.wordPageScrollX))
		firstRect := image.Rect(left, rowY+topGap, left+firstWidth, rowY+topGap+firstHeight)
		a.appendWordVisibleInteraction(state, firstPageIndex, firstRect, firstScale)
		if hasSecond {
			secondRect := image.Rect(firstRect.Max.X+columnGap, rowY+topGap, firstRect.Max.X+columnGap+secondWidth, rowY+topGap+secondHeight)
			a.appendWordVisibleInteraction(state, secondIndex, secondRect, wordScenePixelsPerPoint(gtx, result.Pages[secondIndex], state.wordLayoutZoom))
		}
		rowY += rowHeight
	}
}

func (a *App) appendWordVisibleInteraction(state *documentSessionState, pageIndex int, pageRect image.Rectangle, pageScale float64) {
	if pageIndex < 0 {
		return
	}
	tag := state.wordPageInteractionTagsNext[pageIndex]
	if tag == nil {
		tag = state.wordPageInteractionTags[pageIndex]
	}
	if tag == nil {
		tag = new(wordPageInteractionTag)
	}
	state.wordPageInteractionTagsNext[pageIndex] = tag
	state.wordPageVisibleInteractionsNext = append(state.wordPageVisibleInteractionsNext, wordVisiblePageInteraction{
		PageIndex: pageIndex,
		PageRect:  pageRect,
		Transform: wordPageTransform{Origin: pageRect.Min, PixelsPerPoint: pageScale},
	})
}

func (a *App) drawWordPageSurface(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, index, total int, pageRect image.Rectangle, pageScale float64, presentation wordPagePresentation) {
	if presentation == wordPagePrint {
		shadowRect := pageRect.Add(image.Pt(gtx.Dp(unit.Dp(2)), gtx.Dp(unit.Dp(3))))
		paint.FillShape(gtx.Ops, wordPageShadow, clip.Rect{Min: shadowRect.Min, Max: shadowRect.Max}.Op())
	}
	surfaceColor := wordPageSurface
	if state.wordBackgroundColor != nil {
		surfaceColor = *state.wordBackgroundColor
	}
	paint.FillShape(gtx.Ops, surfaceColor, clip.Rect{Min: pageRect.Min, Max: pageRect.Max}.Op())
	if state.wordGridlinesVisible && !state.wordReadingMode {
		drawWordGridlines(gtx, a, pageRect)
	}
	pageClip := clip.Rect{Min: pageRect.Min, Max: pageRect.Max}.Push(gtx.Ops)
	semantic.DescriptionOp(a.textArgs("word.accessibility.page_number", i18n.Args{"Current": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(index+1)), "Total": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(total))})).Add(gtx.Ops)
	a.drawWordPageScene(gtx, state, page, pageRect.Min, pageScale)
	pageClip.Pop()
}
