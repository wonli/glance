package desktopui

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
)

type wordImageInteraction struct {
	moveDrag          gesture.Drag
	resizeDrags       [8]gesture.Drag
	rotationDrag      gesture.Drag
	preview           wordImageGesture
	committingPreview wordImagePreview
	committingMode    wordImageGestureMode
	commitPending     bool
	moveStart         f32.Point
	resizeStart       f32.Point
	resizeStartFrame  image.Point
	resizeStartEMU    int64
	rotationStart     f32.Point
	rotationCenter    f32.Point
	rotationStartDeg  float64
}

func wordContentOrder(blocks []document.WordBlock) []string {
	var order []string
	appendParagraph := func(paragraph document.WordParagraph) {
		for _, inline := range paragraph.Inlines {
			switch {
			case inline.Text != nil:
				order = append(order, inline.Text.ID)
			case inline.Image != nil:
				order = append(order, inline.Image.ID)
			case inline.Unsupported != nil:
				order = append(order, inline.Unsupported.ID)
			}
		}
	}
	for _, block := range blocks {
		switch block.Kind {
		case document.WordParagraphBlock:
			if block.Paragraph != nil {
				appendParagraph(*block.Paragraph)
			}
		case document.WordTableBlock:
			if block.Table == nil {
				continue
			}
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						appendParagraph(paragraph)
					}
				}
			}
		case document.WordUnsupportedBlock:
			if block.Unsupported != nil {
				order = append(order, block.Unsupported.ID)
			}
		}
	}
	return order
}

func wordBlocksForState(state *documentSessionState) []document.WordBlock {
	if state == nil {
		return nil
	}
	state.wordBlocksMu.RLock()
	if state.wordBlocks != nil {
		blocks := cloneWordBlockSnapshot(state.wordBlocks)
		state.wordBlocksMu.RUnlock()
		return blocks
	}
	state.wordBlocksMu.RUnlock()
	if state.word != nil {
		unlock := lockSessionAdapterRead(state)
		blocks := cloneWordBlockSnapshot(state.word.WordBlocks())
		unlock()
		state.wordBlocksMu.Lock()
		if state.wordBlocks == nil {
			state.wordBlocks = blocks
		}
		blocks = cloneWordBlockSnapshot(state.wordBlocks)
		state.wordBlocksMu.Unlock()
		return blocks
	}
	return nil
}

func selectWordRun(state *documentSessionState, paragraphID, runID string) bool {
	if state == nil || state.word == nil {
		return false
	}
	blocks := wordBlocksForState(state)
	paragraph, ok := selectedWordParagraph(blocks, paragraphID)
	if !ok {
		return false
	}
	run, ok := selectedWordRun(blocks, runID)
	if !ok || !run.Editable {
		return false
	}
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraph.ID
	state.wordPreferredVerticalXSet = false
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordEditor.SetText(run.Text)
	editor := wordRunEditor(state, run)
	if draft, exists := state.wordDraft(run.ID); exists {
		editor.SetText(draft)
	} else {
		committed, exists := state.committedWordText(run.ID)
		if !exists || committed != run.Text {
			state.setCommittedWordText(run.ID, run.Text)
		}
		editor.SetText(run.Text)
	}
	return true
}

func wordRunEditor(state *documentSessionState, run document.WordTextRun) *widget.Editor {
	if state.wordRunEditors == nil {
		state.wordRunEditors = make(map[string]*widget.Editor)
	}
	editor := state.wordRunEditors[run.ID]
	if editor == nil {
		editor = new(widget.Editor)
		state.wordRunEditors[run.ID] = editor
		if draft, exists := state.wordDraft(run.ID); exists {
			editor.SetText(draft)
		} else {
			editor.SetText(run.Text)
			state.setCommittedWordText(run.ID, run.Text)
		}
	}
	editor.SingleLine = false
	editor.Submit = true
	return editor
}

func selectWordImage(state *documentSessionState, imageID string) bool {
	if state == nil || state.word == nil {
		return false
	}
	blocks := wordBlocksForState(state)
	if findWordImage(blocks, imageID) == nil {
		return false
	}
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordFlushAllDrafts.Store(true)
	}
	state.selectedWordRunID = ""
	state.selectedWordImageID = imageID
	state.selectedBlock = -1
	state.selectedBlockEditable = false
	state.wordToolbarTab = wordTabPicture
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == imageID {
				state.selectedWordParagraphID = paragraph.ID
				return true
			}
		}
	}
	return false
}

func clearWordImageSelection(state *documentSessionState) {
	if state == nil {
		return
	}
	state.selectedWordImageID = ""
	state.selectedWordRunID = ""
	state.selectedWordParagraphID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = false
	if state.wordToolbarTab == wordTabPicture {
		state.wordToolbarTab = wordTabHome
	}
}

func wordObjectButton(state *documentSessionState, id string) *widget.Clickable {
	if state.wordObjectButtons == nil {
		state.wordObjectButtons = make(map[string]*widget.Clickable)
	}
	button := state.wordObjectButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		state.wordObjectButtons[id] = button
	}
	return button
}

func wordImageInteractionFor(state *documentSessionState, id string) *wordImageInteraction {
	if state.wordImageInteractions == nil {
		state.wordImageInteractions = make(map[string]*wordImageInteraction)
	}
	interaction := state.wordImageInteractions[id]
	if interaction == nil {
		interaction = &wordImageInteraction{}
		state.wordImageInteractions[id] = interaction
	}
	return interaction
}

func wordImageLayoutPreview(interaction *wordImageInteraction) (wordImagePreview, wordImageGestureMode) {
	if interaction == nil {
		return wordImagePreview{}, wordImageGestureNone
	}
	preview := interaction.preview.Preview()
	mode := interaction.preview.mode
	if !preview.Active && interaction.commitPending {
		return interaction.committingPreview, interaction.committingMode
	}
	return preview, mode
}

func finishWordImagePreviewCommand(state *documentSessionState, imageID string) {
	if state == nil || imageID == "" {
		return
	}
	interaction := wordImageInteractionFor(state, imageID)
	interaction.committingPreview = wordImagePreview{}
	interaction.committingMode = wordImageGestureNone
	interaction.commitPending = false
}

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
					return a.layoutWordBodyArea(gtx, state, blocks)
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
										drawWordGridlines(gtx, image.Rectangle{Max: gtx.Constraints.Max})
									}
									canvasClip := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
									if webMode {
										semantic.DescriptionOp("Word Web 文档区域").Add(gtx.Ops)
									} else {
										semantic.DescriptionOp("Word 文档画布").Add(gtx.Ops)
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
					semantic.DescriptionOp("Word 文档滚动条").Add(gtx.Ops)
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
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if state.wordRulerHidden || state.wordReadingMode || webMode {
						return layout.Dimensions{}
					}
					rulerWidth := gtx.Dp(unit.Dp(wordRulerWidthDP))
					left := max(0, canvasLeftPx-rulerWidth-gtx.Dp(unit.Dp(wordRulerGapDP)))
					leftDP := float32(left) / max32(0.1, gtx.Metric.PxPerDp)
					topDP := float32(min(gtx.Dp(unit.Dp(wordVerticalRulerTopDP)), gtx.Constraints.Max.Y)) / max32(0.1, gtx.Metric.PxPerDp)
					bottomDP := float32(min(gtx.Dp(unit.Dp(wordVerticalRulerBottomDP)), max(0, gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(wordVerticalRulerTopDP))))) / max32(0.1, gtx.Metric.PxPerDp)
					return layout.Inset{Top: unit.Dp(topDP), Bottom: unit.Dp(bottomDP), Left: unit.Dp(leftDP)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutWordVerticalRuler(gtx, a.theme)
					})
				}),
			)
		},
	)
	for event, ok := state.wordCanvasBlankClick.Update(gtx.Source); ok; event, ok = state.wordCanvasBlankClick.Update(gtx.Source) {
		if event.Kind != gesture.KindClick || state.selectedWordImageID == "" || state.wordCanvasContentClickHandled {
			continue
		}
		clearWordImageSelection(state)
		gtx.Execute(key.FocusCmd{Tag: &state.wordCanvasBlankClick})
		if a.window != nil {
			a.window.Invalidate()
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

func (a *App) layoutWordList(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	state.wordListState.List.Axis = layout.Vertical
	return state.wordListState.List.Layout(gtx, len(blocks)+1, func(gtx layout.Context, index int) layout.Dimensions {
		if index == len(blocks) {
			return a.layoutWordAppendTarget(gtx, state)
		}
		if blocks[index].Kind == document.WordParagraphBlock && blocks[index].Paragraph != nil {
			border, hasBorder := wordContinuousParagraphBottomBorder(blocks, index)
			return a.layoutWordParagraphWithBottomBorder(gtx, *blocks[index].Paragraph, false, border, hasBorder)
		}
		return a.layoutWordBlock(gtx, blocks[index])
	})
}

func wordContinuousParagraphBottomBorder(blocks []document.WordBlock, index int) (document.WordParagraphBorder, bool) {
	if index < 0 || index >= len(blocks) || blocks[index].Paragraph == nil {
		return document.WordParagraphBorder{}, false
	}
	paragraph := blocks[index].Paragraph
	currentSet := paragraph.Layout.Set&document.WordParagraphBottomBorderSet != 0
	if currentSet && paragraphBottomBorderVisible(paragraph.Layout.BottomBorder) {
		if index+1 < len(blocks) && blocks[index+1].Kind == document.WordParagraphBlock && blocks[index+1].Paragraph != nil && blocks[index+1].Paragraph.Layout.Set&document.WordParagraphBottomBorderSet == 0 {
			return document.WordParagraphBorder{}, false
		}
		return paragraph.Layout.BottomBorder, true
	}
	if !currentSet && index > 0 && blocks[index-1].Kind == document.WordParagraphBlock && blocks[index-1].Paragraph != nil {
		previous := blocks[index-1].Paragraph
		if previous.Layout.Set&document.WordParagraphBottomBorderSet != 0 && paragraphBottomBorderVisible(previous.Layout.BottomBorder) {
			return previous.Layout.BottomBorder, true
		}
	}
	return document.WordParagraphBorder{}, false
}

func paragraphBottomBorderVisible(border document.WordParagraphBorder) bool {
	return border.Style != "none" && border.Style != "nil" && border.SizePT > 0
}

func wordListViewportRange(position layout.Position, elements, viewportSize int) (start, end float32) {
	if position.Length <= 0 || elements <= 0 || viewportSize <= 0 {
		return 0, 1
	}
	length := float32(position.Length)
	elementLength := length / float32(elements)
	start = clampWordFraction((float32(position.First)*elementLength + float32(position.Offset)) / length)
	end = clampWordFraction((float32(position.First+position.Count)*elementLength + float32(position.OffsetLast)) / length)
	viewportFraction := end - start
	visibleFraction := min(1, float32(viewportSize)/length)
	err := visibleFraction - viewportFraction
	if viewportFraction < 1 {
		startShare := start / (1 - viewportFraction)
		endShare := (1 - end) / (1 - viewportFraction)
		start -= startShare * err
		end += endShare * err
	}
	return clampWordFraction(start), clampWordFraction(end)
}

func clampWordFraction(value float32) float32 {
	return max(0, min(1, value))
}

func (a *App) layoutWordAppendTarget(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	if state.wordReadingMode {
		height := gtx.Dp(unit.Dp(560))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if state.wordAppendTarget.Clicked(gtx) {
		if state.selectedWordImageID != "" {
			clearWordImageSelection(state)
			gtx.Execute(key.FocusCmd{Tag: &state.wordAppendTarget})
		} else if !state.wordInsertionRunning {
			state.wordForceDraftFlush.Store(true)
			state.wordInsertAfterDraftFlush = true
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
		}
	}
	return state.wordAppendTarget.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(560))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("单击此处继续输入").Add(gtx.Ops)
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				if !state.wordAppendTarget.Hovered() {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				}
				return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Caption(a.theme, "单击此处继续输入").Layout(gtx)
				})
			},
		)
	})
}

func (a *App) layoutWordBlock(gtx layout.Context, block document.WordBlock) layout.Dimensions {
	switch block.Kind {
	case document.WordParagraphBlock:
		if block.Paragraph != nil {
			return a.layoutWordParagraph(gtx, *block.Paragraph, false)
		}
	case document.WordTableBlock:
		if block.Table != nil {
			return a.layoutWordTable(gtx, *block.Table)
		}
	case document.WordUnsupportedBlock:
		if block.Unsupported != nil {
			return wordReadOnlyPlaceholder(gtx, a.theme, block.Unsupported.Description)
		}
	}
	return layout.Dimensions{}
}

func (a *App) layoutWordTable(gtx layout.Context, table document.WordTable) layout.Dimensions {
	columnWidths := wordTableColumnWidths(table)
	rows := make([]layout.FlexChild, 0, len(table.Rows))
	for _, row := range table.Rows {
		row := row
		cells := make([]layout.FlexChild, 0, len(row.Cells))
		for cellIndex, cell := range row.Cells {
			cell := cell
			weight := 1.0
			if cellIndex < len(columnWidths) {
				weight = columnWidths[cellIndex]
			}
			cells = append(cells, layout.Flexed(float32(weight), func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordTableCell(gtx, cell)
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, cells...)
			})
		}))
	}
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return wordMeasuredBackground(gtx, gridLine, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		})
	})
}

func (a *App) layoutWordTableCell(gtx layout.Context, cell document.WordTableCell) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(cell.Paragraphs))
	for _, paragraph := range cell.Paragraphs {
		paragraph := paragraph
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordParagraph(gtx, paragraph, true)
		}))
	}
	fill := contentSurface
	if cellFill, ok := parseWordRunColor(cell.FillColor); ok {
		fill = cellFill
	}
	return wordMeasuredBackground(gtx, fill, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func wordTableColumnWidths(table document.WordTable) []float64 {
	columns := len(table.Layout.GridColumnsEMU)
	for _, row := range table.Rows {
		if len(row.Cells) > columns {
			columns = len(row.Cells)
		}
	}
	if columns == 0 {
		return nil
	}
	widths := make([]float64, columns)
	for index, emu := range table.Layout.GridColumnsEMU {
		if emu > 0 {
			widths[index] = float64(emu)
		}
	}
	for _, row := range table.Rows {
		for index, cell := range row.Cells {
			if widths[index] <= 0 && cell.WidthEMU > 0 {
				widths[index] = float64(cell.WidthEMU)
			}
		}
	}
	knownTotal := 0.0
	knownCount := 0
	for _, width := range widths {
		if width > 0 {
			knownTotal += width
			knownCount++
		}
	}
	if knownCount == 0 {
		for index := range widths {
			widths[index] = 1
		}
		return widths
	}
	average := knownTotal / float64(knownCount)
	for index := range widths {
		if widths[index] <= 0 {
			widths[index] = average
		}
	}
	return widths
}

// wordMeasuredBackground paints a list item's measured bounds without claiming
// the full remaining viewport height. Returning viewport-sized fill dimensions
// here makes a vertical List believe its first paragraph consumes the screen.
func wordMeasuredBackground(gtx layout.Context, fill color.NRGBA, child func(layout.Context) layout.Dimensions) layout.Dimensions {
	recorder := op.Record(gtx.Ops)
	dimensions := child(gtx)
	call := recorder.Stop()
	paint.FillShape(gtx.Ops, fill, clip.Rect{Max: dimensions.Size}.Op())
	call.Add(gtx.Ops)
	return dimensions
}

func (a *App) layoutWordParagraph(gtx layout.Context, paragraph document.WordParagraph, inCell bool) layout.Dimensions {
	return a.layoutWordParagraphWithBottomBorder(gtx, paragraph, inCell, paragraph.Layout.BottomBorder, paragraph.Layout.Set&document.WordParagraphBottomBorderSet != 0)
}

func (a *App) layoutWordParagraphWithBottomBorder(gtx layout.Context, paragraph document.WordParagraph, inCell bool, border document.WordParagraphBorder, hasBorder bool) layout.Dimensions {
	children := make([]wordInlineLayoutChild, 0, len(paragraph.Inlines))
	leftIndentEMU, rightIndentEMU := wordRulerParagraphIndents(a.documentSessionState, paragraph)
	leftIndentDP := max(0, int(math.Round(float64(leftIndentEMU)/float64(wordEMUPerDP))))
	rightIndentDP := max(0, int(math.Round(float64(rightIndentEMU)/float64(wordEMUPerDP))))
	leftIndentPX, rightIndentPX := gtx.Dp(unit.Dp(leftIndentDP)), gtx.Dp(unit.Dp(rightIndentDP))
	maxWidth := max(1, gtx.Constraints.Max.X-leftIndentPX-rightIndentPX)
	for _, inline := range paragraph.Inlines {
		inline := inline
		switch {
		case inline.Text != nil:
			run := *inline.Text
			var width int
			if len(paragraph.Inlines) == 1 {
				width = maxWidth
			} else {
				widthRun := run
				if editor := a.documentSessionState.wordRunEditors[run.ID]; editor != nil {
					widthRun.Text = editor.Text()
				} else if draft, exists := a.documentSessionState.wordDraft(run.ID); exists {
					widthRun.Text = draft
				}
				width = wordRunPreferredWidth(gtx, a.theme, paragraph, widthRun)
			}
			children = append(children, wordInlineLayoutChild{width: width, runID: run.ID, layout: func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordTextRun(gtx, paragraph, *inline.Text)
			}})
		case inline.Image != nil:
			item := *inline.Image
			overlay := &wordInlineLayoutOverlay{}
			children = append(children, wordInlineLayoutChild{width: a.wordImagePreferredWidth(gtx, item, maxWidth), overlay: overlay, layout: func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordImage(gtx, paragraph, *inline.Image, overlay)
			}})
		case inline.PageBreak != nil:
			pageBreak := *inline.PageBreak
			children = append(children, wordInlineLayoutChild{width: gtx.Dp(unit.Dp(68)), layout: func(gtx layout.Context) layout.Dimensions {
				return layoutWordPageBreak(gtx, a.theme, pageBreak)
			}})
		case inline.Unsupported != nil:
			children = append(children, wordInlineLayoutChild{width: min(maxWidth, gtx.Dp(unit.Dp(240))), layout: func(gtx layout.Context) layout.Dimensions {
				return wordReadOnlyPlaceholder(gtx, a.theme, inline.Unsupported.Description)
			}})
		}
	}
	spacing := unit.Dp(6)
	if inCell {
		spacing = unit.Dp(3)
	}
	dimensions := layout.Inset{Top: unit.Dp(5), Bottom: spacing, Left: unit.Dp(leftIndentDP), Right: unit.Dp(rightIndentDP)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		flow := func(gtx layout.Context) layout.Dimensions {
			return layoutWordInlineFlow(gtx, a.documentSessionState, paragraph.Alignment, children)
		}
		if paragraph.ListMarker == "" {
			return flow(gtx)
		}
		markerWidth := unit.Dp(wordListMarkerWidthDP(paragraph))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(a.theme, paragraph.ListMarker)
					gtx.Constraints.Min.X = gtx.Dp(markerWidth) - gtx.Dp(unit.Dp(6))
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					return label.Layout(gtx)
				})
			}),
			layout.Flexed(1, flow),
		)
	})
	if hasBorder && paragraphBottomBorderVisible(border) {
		pixelsPerPoint := float64(gtx.Metric.PxPerDp) * (4.0 / 3.0)
		space := int(math.Round(border.SpacePT * pixelsPerPoint))
		thickness := max(1, int(math.Ceil(border.SizePT*pixelsPerPoint)))
		// Word's border space is measured from the end of the text line toward
		// the border. Keep the editor's bottom inset outside that measurement so
		// this matches the paginated scene's text-line geometry.
		y := dimensions.Size.Y - gtx.Dp(spacing) + space
		width := max(0, gtx.Constraints.Max.X-leftIndentPX-rightIndentPX)
		if width > 0 && y >= 0 {
			ink, ok := parseWordRunColor(border.Color)
			if !ok {
				ink = color.NRGBA{A: 255}
			}
			paint.FillShape(gtx.Ops, ink, clip.Rect{Min: image.Pt(leftIndentPX, y), Max: image.Pt(leftIndentPX+width, y+thickness)}.Op())
		}
	}
	return dimensions
}

func layoutWordPageBreak(gtx layout.Context, theme *material.Theme, pageBreak document.WordPageBreak) layout.Dimensions {
	semantic.DescriptionOp("分页符 " + pageBreak.ID).Add(gtx.Ops)
	bounds := image.Rectangle{Max: gtx.Constraints.Max}
	shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(3)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 246, G: 248, B: 250, A: 255}, shape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, gridLine, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	return layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Caption(theme, "分页符")
		label.Color = color.NRGBA{R: 89, G: 99, B: 111, A: 255}
		label.TextSize = unit.Sp(11)
		return label.Layout(gtx)
	})
}

func wordListMarkerWidthDP(paragraph document.WordParagraph) int {
	width := max(24, len([]rune(paragraph.ListMarker))*9+12)
	return width + max(0, paragraph.ListLevel)*18
}

func (a *App) layoutWordTextRun(gtx layout.Context, paragraph document.WordParagraph, run document.WordTextRun) layout.Dimensions {
	state := a.documentSessionState
	editor := wordRunEditor(state, run)
	label := wordRunLabel(a.theme, paragraph, run)
	focusTargetPending := state.wordPendingFocusRunID != "" && state.wordPendingFocusRunID != run.ID
	paragraphSplitPending := state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID
	paragraphJoinPending := state.wordParagraphJoinIntent != nil
	editor.ReadOnly = state.wordReadingMode || !run.Editable || state.formatRunning.Load() || a.isSaving() || paragraphSplitPending || paragraphJoinPending || focusTargetPending
	selectionStartBeforeUpdate, selectionEndBeforeUpdate := editor.Selection()
	if !editor.ReadOnly {
		if a.handleWordParagraphJoinKey(gtx, paragraph, run, editor) {
			editor.ReadOnly = true
		} else {
			a.handleWordArrowNavigation(gtx, paragraph, run, editor)
			for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
				if _, submit := event.(widget.SubmitEvent); submit {
					selectionStart, selectionEnd := editor.Selection()
					a.queueWordParagraphSplit(run.ID, editor.Text(), selectionStart, selectionEnd)
					if state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID {
						editor.ReadOnly = true
					}
				}
			}
			selectionStart, selectionEnd := editor.Selection()
			if gtx.Focused(editor) && state.wordFocusedRunID == run.ID && (selectionStart != selectionStartBeforeUpdate || selectionEnd != selectionEndBeforeUpdate) && state.wordNavigationVerticalHandledRunID != run.ID {
				state.wordPreferredVerticalXSet = false
			}
		}
	}
	editor.Alignment = wordParagraphAlignment(paragraph.Alignment)
	style := material.Editor(a.theme, editor, "")
	style.Font, style.TextSize, style.Color = label.Font, label.TextSize, label.Color
	background := contentSurface
	dimensions := wordMeasuredBackground(gtx, background, func(gtx layout.Context) layout.Dimensions {
		if highlightColor, ok := wordHighlightDisplayColor(run.Style.Highlight); ok {
			for _, rect := range wordTextHighlightRects(gtx, a.theme.Shaper, label, editor.Text()) {
				paint.FillShape(gtx.Ops, highlightColor, clip.Rect(rect).Op())
			}
		}
		dims := layoutWordRunEditor(gtx, style, run.ID)
		if run.Style.Underline && dims.Size.X > 0 && dims.Size.Y > 0 {
			y := max(0, dims.Baseline)
			thickness := max(1, gtx.Dp(unit.Dp(1)))
			paint.FillShape(gtx.Ops, label.Color, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(dims.Size.X, y+thickness)}.Op())
		}
		return dims
	})
	pendingImageFocus := state.selectedWordImageID != "" && state.wordImageFocusRequestID == state.selectedWordImageID
	if gtx.Focused(editor) && run.Editable && !focusTargetPending && !pendingImageFocus {
		state.wordCanvasContentClickHandled = true
		if state.selectedWordRunID != "" && state.selectedWordRunID != run.ID && state.wordPendingFocusRunID != run.ID {
			state.wordPreferredVerticalXSet = false
		}
		state.selectedWordRunID = run.ID
		state.selectedWordParagraphID = paragraph.ID
		state.selectedWordImageID = ""
		state.wordImageFocusRequestID = ""
		state.selectedBlock = -1
		state.selectedBlockEditable = true
		state.wordLayoutFocusedRunID = run.ID
		if state.wordFormatPainterActive && run.ID != state.wordFormatPainterSourceRunID {
			state.wordFormatPainterPendingParagraphID = paragraph.ID
			state.wordFormatPainterPendingRunID = run.ID
		}
	}
	if run.Editable {
		current := editor.Text()
		captureWordTextDraft(state, run.ID, current)
		committed, _ := state.committedWordText(run.ID)
		if current != committed && state.selectedWordImageID == "" {
			state.selectedWordRunID = run.ID
			state.selectedWordParagraphID = paragraph.ID
		}
	}
	if state.wordPendingFocusRunID == run.ID {
		if state.wordPendingCaretRunID == run.ID && !state.wordPendingCaretByX {
			editor.SetCaret(state.wordPendingCaretOffset, state.wordPendingCaretOffset)
			state.wordPendingCaretRunID = ""
		}
		gtx.Execute(key.FocusCmd{Tag: editor})
		state.wordPendingFocusRunID = ""
	}
	return dimensions
}

func (a *App) handleWordParagraphJoinKey(gtx layout.Context, paragraph document.WordParagraph, run document.WordTextRun, editor *widget.Editor) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || !run.Editable || !gtx.Focused(editor) || state.formatRunning.Load() || state.wordParagraphJoinIntent != nil || state.wordParagraphSplitIntent != nil || a.isSaving() {
		return false
	}
	start, end := editor.Selection()
	if start != end {
		return false
	}
	var filters []key.Filter
	if start == 0 && wordTextRunAtBoundary(paragraph, run.ID, true) && wordHasAdjacentParagraphInState(state, paragraph.ID, document.WordJoinPrevious) {
		filters = append(filters, key.Filter{Focus: editor, Name: key.NameDeleteBackward})
	}
	if start == editor.Len() && wordTextRunAtBoundary(paragraph, run.ID, false) && wordHasAdjacentParagraphInState(state, paragraph.ID, document.WordJoinNext) {
		filters = append(filters, key.Filter{Focus: editor, Name: key.NameDeleteForward})
	}
	for _, filter := range filters {
		event, ok := gtx.Event(filter)
		if !ok {
			continue
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		// The targeted Gio filter can find a later delete key while earlier
		// navigation or text events are still queued. Let the editor consume
		// those first, then decide whether this delete is still at a paragraph
		// boundary.
		for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
			if _, submit := event.(widget.SubmitEvent); submit {
				selectionStart, selectionEnd := editor.Selection()
				a.queueWordParagraphSplit(run.ID, editor.Text(), selectionStart, selectionEnd)
				if state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID {
					return true
				}
			}
		}
		start, end = editor.Selection()
		atBoundary := start == end
		direction := document.WordJoinPrevious
		if keyEvent.Name == key.NameDeleteForward {
			direction = document.WordJoinNext
		}
		joinBoundary := (direction == document.WordJoinPrevious && atBoundary && start == 0 && wordTextRunAtBoundary(paragraph, run.ID, true)) ||
			(direction == document.WordJoinNext && atBoundary && start == editor.Len() && wordTextRunAtBoundary(paragraph, run.ID, false))
		if joinBoundary && wordHasAdjacentParagraphInState(state, paragraph.ID, direction) {
			a.queueWordParagraphJoin(run.ID, editor.Text(), direction)
			return true
		}
		if direction == document.WordJoinPrevious {
			editor.Delete(-1)
		} else {
			editor.Delete(1)
		}
		return true
	}
	return false
}

func wordHasAdjacentParagraphInState(state *documentSessionState, paragraphID string, direction document.WordParagraphJoinDirection) bool {
	if state == nil {
		return false
	}
	state.wordBlocksMu.RLock()
	if state.wordBlocks != nil {
		found := wordHasAdjacentParagraph(state.wordBlocks, paragraphID, direction)
		state.wordBlocksMu.RUnlock()
		return found
	}
	state.wordBlocksMu.RUnlock()
	return wordHasAdjacentParagraph(wordBlocksForState(state), paragraphID, direction)
}

func wordTextRunAtBoundary(paragraph document.WordParagraph, runID string, first bool) bool {
	if first {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				return inline.Text.ID == runID
			}
		}
		return false
	}
	for index := len(paragraph.Inlines) - 1; index >= 0; index-- {
		inline := paragraph.Inlines[index]
		if inline.Text != nil && inline.Text.Editable {
			return inline.Text.ID == runID
		}
	}
	return false
}

func wordHasAdjacentParagraph(blocks []document.WordBlock, paragraphID string, direction document.WordParagraphJoinDirection) bool {
	for blockIndex := range blocks {
		block := &blocks[blockIndex]
		if block.Paragraph != nil && block.Paragraph.ID == paragraphID {
			adjacentIndex := blockIndex - 1
			if direction == document.WordJoinNext {
				adjacentIndex = blockIndex + 1
			}
			if adjacentIndex < 0 || adjacentIndex >= len(blocks) || blocks[adjacentIndex].Kind != document.WordParagraphBlock || blocks[adjacentIndex].Paragraph == nil {
				return false
			}
			return true
		}
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for paragraphIndex := range cell.Paragraphs {
					if cell.Paragraphs[paragraphIndex].ID != paragraphID {
						continue
					}
					adjacentIndex := paragraphIndex - 1
					if direction == document.WordJoinNext {
						adjacentIndex = paragraphIndex + 1
					}
					return adjacentIndex >= 0 && adjacentIndex < len(cell.Paragraphs)
				}
			}
		}
	}
	return false
}

func layoutWordRunEditor(gtx layout.Context, style material.EditorStyle, runID string) layout.Dimensions {
	recorder := op.Record(gtx.Ops)
	dimensions := style.Layout(gtx)
	call := recorder.Stop()
	clip := clip.Rect{Max: dimensions.Size}.Push(gtx.Ops)
	semantic.DescriptionOp("正文文字 " + runID).Add(gtx.Ops)
	call.Add(gtx.Ops)
	clip.Pop()
	return dimensions
}

type wordInlineLayoutChild struct {
	width   int
	runID   string
	overlay *wordInlineLayoutOverlay
	layout  layout.Widget
}

type wordInlineLayoutOverlay struct {
	call  op.CallOp
	ready bool
}

func wordInlineGeometry(maxWidth int, sizes []image.Point, alignment document.WordAlignment) ([]image.Point, int) {
	if len(sizes) == 0 || maxWidth <= 0 {
		return make([]image.Point, len(sizes)), 0
	}
	positions := make([]image.Point, len(sizes))
	lineWidths := []int{0}
	lineHeights := []int{0}
	lineForItem := make([]int, len(sizes))
	line := 0
	for index, size := range sizes {
		width := min(maxWidth, max(0, size.X))
		if lineWidths[line] > 0 && lineWidths[line]+width > maxWidth {
			line++
			lineWidths = append(lineWidths, 0)
			lineHeights = append(lineHeights, 0)
		}
		positions[index].X = lineWidths[line]
		lineWidths[line] += width
		lineHeights[line] = max(lineHeights[line], max(0, size.Y))
		lineForItem[index] = line
	}
	lineTops := make([]int, len(lineHeights))
	for index := 1; index < len(lineTops); index++ {
		lineTops[index] = lineTops[index-1] + lineHeights[index-1]
	}
	for index, line := range lineForItem {
		positions[index].Y = lineTops[line]
		alignOffset := 0
		switch alignment {
		case document.WordAlignCenter:
			alignOffset = (maxWidth - lineWidths[line]) / 2
		case document.WordAlignRight:
			alignOffset = maxWidth - lineWidths[line]
		}
		positions[index].X += max(0, alignOffset)
	}
	totalHeight := lineTops[len(lineTops)-1] + lineHeights[len(lineHeights)-1]
	return positions, totalHeight
}

func layoutWordInlineFlow(gtx layout.Context, state *documentSessionState, alignment document.WordAlignment, children []wordInlineLayoutChild) layout.Dimensions {
	maxWidth := max(1, gtx.Constraints.Max.X)
	type measuredChild struct {
		call  op.CallOp
		size  image.Point
		runID string
	}
	measured := make([]measuredChild, len(children))
	sizes := make([]image.Point, len(children))
	for index, child := range children {
		width := min(maxWidth, max(1, child.width))
		childGtx := gtx
		childGtx.Constraints.Min.X = width
		childGtx.Constraints.Max.X = width
		recorder := op.Record(gtx.Ops)
		dims := child.layout(childGtx)
		call := recorder.Stop()
		measured[index] = measuredChild{call: call, size: dims.Size, runID: child.runID}
		sizes[index] = dims.Size
	}
	positions, height := wordInlineGeometry(maxWidth, sizes, alignment)
	for index, child := range measured {
		if child.runID != "" {
			if state.wordNavLayoutsNext == nil {
				state.wordNavLayoutsNext = make(map[string]wordNavigationGeometry)
			}
			state.wordNavLayoutsNext[child.runID] = wordNavigationGeometry{x: positions[index].X, y: positions[index].Y}
			if state.wordPendingCaretRunID == child.runID && state.wordPendingCaretByX {
				if editor := state.wordRunEditors[child.runID]; editor != nil {
					offset := wordClosestCaretOffsetToX(editor, state.wordPendingCaretX-positions[index].X)
					editor.SetCaret(offset, offset)
				}
				state.wordPendingCaretRunID = ""
				state.wordPendingCaretByX = false
			}
		}
		transform := op.Offset(positions[index]).Push(gtx.Ops)
		child.call.Add(gtx.Ops)
		transform.Pop()
	}
	for index := range measured {
		if children[index].overlay == nil || !children[index].overlay.ready {
			continue
		}
		transform := op.Offset(positions[index]).Push(gtx.Ops)
		children[index].overlay.call.Add(gtx.Ops)
		transform.Pop()
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, height))}
}

func wordRunPreferredWidth(gtx layout.Context, theme *material.Theme, paragraph document.WordParagraph, run document.WordTextRun) int {
	if run.Text == "" {
		return max(1, gtx.Dp(unit.Dp(12)))
	}
	label := wordRunLabel(theme, paragraph, run)
	theme.Shaper.LayoutString(text.Parameters{Font: label.Font, PxPerEm: fixed.I(gtx.Sp(label.TextSize)), MaxWidth: 1 << 20, Locale: gtx.Locale}, run.Text)
	var width fixed.Int26_6
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		right := glyph.X + glyph.Advance
		if right > width {
			width = right
		}
	}
	pixels := (int(width) + 63) / 64
	return max(1, max(pixels, wordRunFallbackWidth(gtx, label.TextSize, run.Text))+gtx.Dp(unit.Dp(4)))
}

func wordRunFallbackWidth(gtx layout.Context, size unit.Sp, value string) int {
	em := max(1, gtx.Sp(size))
	width := 0
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '\n' || r == '\r':
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r), unicode.Is(unicode.Bopomofo, r),
			r >= 0x3000 && r <= 0x303F, r >= 0xFF01 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6, r >= 0x1F300 && r <= 0x1FAFF:
			width += em
		default:
			width += max(1, int(math.Round(float64(em)*0.55)))
		}
	}
	return width
}

func (a *App) wordImagePreferredWidth(gtx layout.Context, item document.WordImage, maxWidth int) int {
	preview, previewMode := wordImageLayoutPreview(wordImageInteractionFor(a.documentSessionState, item.ID))
	if preview.Active && preview.ImageID == item.ID {
		switch previewMode {
		case wordImageGestureResize:
			item.WidthEMU, item.HeightEMU = preview.WidthEMU, preview.HeightEMU
		case wordImageGestureRotate:
			item.RotationDegrees = preview.RotationDegrees
		}
	}
	if decoded, ok := a.documentSessionState.wordImageCache.Get(item.ID); ok {
		width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
		if item.WidthEMU > 0 && item.HeightEMU > 0 {
			width = wordImageEMUToPixels(item.WidthEMU, gtx.Metric.PxPerDp)
			height = wordImageEMUToPixels(item.HeightEMU, gtx.Metric.PxPerDp)
		}
		source := fitRotatedWordImageSize(width, height, maxWidth, gtx.Dp(unit.Dp(360)), item.RotationDegrees)
		return max(1, rotatedWordImageBounds(source, item.RotationDegrees).X)
	}
	width, height := wordImageEMUToPixels(item.WidthEMU, gtx.Metric.PxPerDp), wordImageEMUToPixels(item.HeightEMU, gtx.Metric.PxPerDp)
	if width <= 0 || height <= 0 {
		return min(maxWidth, gtx.Dp(unit.Dp(240)))
	}
	source := fitRotatedWordImageSize(width, height, maxWidth, gtx.Dp(unit.Dp(360)), item.RotationDegrees)
	return max(1, rotatedWordImageBounds(source, item.RotationDegrees).X)
}

func wordRunLabel(theme *material.Theme, paragraph document.WordParagraph, run document.WordTextRun) material.LabelStyle {
	label := material.Body1(theme, run.Text)
	label.Alignment = wordParagraphAlignment(paragraph.Alignment)
	label.Font.Typeface = wordRunTypeface(run.Style.FontFamily)
	if run.Style.Bold {
		label.Font.Weight = font.Bold
	}
	if run.Style.Italic {
		label.Font.Style |= font.Italic
	}
	if run.Style.FontSize > 0 {
		label.TextSize = unit.Sp(run.Style.FontSize)
	}
	if fontColor, ok := parseWordRunColor(run.Style.FontColor); ok {
		label.Color = fontColor
	}
	switch strings.ToLower(paragraph.Style) {
	case "heading1":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(20)
	case "heading2":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(18)
	case "heading3":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(16)
	}
	return label
}

func wordRunTypeface(documentFamily string) font.Typeface {
	families := append(splitWordFontFamilies(documentFamily), wordFontFallbackFamilies...)
	seen := make(map[string]struct{}, len(families))
	ordered := make([]string, 0, len(families))
	for _, family := range families {
		key := strings.ToLower(strings.TrimSpace(family))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ordered = append(ordered, family)
	}
	return font.Typeface(strings.Join(ordered, ", "))
}

func wordTextHighlightRects(gtx layout.Context, shaper *text.Shaper, label material.LabelStyle, value string) []image.Rectangle {
	width := gtx.Constraints.Max.X
	if shaper == nil || value == "" || width <= 0 {
		return nil
	}
	shaper.LayoutString(text.Parameters{
		Font:             label.Font,
		Alignment:        label.Alignment,
		PxPerEm:          fixed.I(gtx.Sp(label.TextSize)),
		MaxWidth:         width,
		MinWidth:         gtx.Constraints.Min.X,
		WrapPolicy:       label.WrapPolicy,
		LineHeight:       fixed.I(gtx.Sp(label.LineHeight)),
		LineHeightScale:  label.LineHeightScale,
		Locale:           gtx.Locale,
		DisableSpaceTrim: true,
	}, value)
	var rects []image.Rectangle
	lineBounds := image.Rectangle{}
	lineHasGlyph := false
	flushLine := func() {
		if lineHasGlyph && !lineBounds.Empty() {
			rects = append(rects, lineBounds)
		}
		lineBounds = image.Rectangle{}
		lineHasGlyph = false
	}
	for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
		if glyph.Flags&text.FlagParagraphBreak == 0 {
			bounds := image.Rect(glyph.X.Floor(), int(glyph.Y)-glyph.Ascent.Ceil(), (glyph.X + glyph.Advance).Ceil(), int(glyph.Y)+glyph.Descent.Ceil())
			if !bounds.Empty() {
				if !lineHasGlyph {
					lineBounds = bounds
					lineHasGlyph = true
				} else {
					lineBounds = lineBounds.Union(bounds)
				}
			}
		}
		if glyph.Flags&text.FlagLineBreak != 0 || glyph.Flags&text.FlagParagraphBreak != 0 {
			flushLine()
		}
	}
	flushLine()
	return rects
}

func wordHighlightDisplayColor(value string) (color.NRGBA, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "black":
		return color.NRGBA{A: 255}, true
	case "blue":
		return color.NRGBA{B: 255, A: 255}, true
	case "cyan":
		return color.NRGBA{G: 255, B: 255, A: 255}, true
	case "darkblue":
		return color.NRGBA{B: 128, A: 255}, true
	case "darkcyan":
		return color.NRGBA{G: 128, B: 128, A: 255}, true
	case "darkgray":
		return color.NRGBA{R: 128, G: 128, B: 128, A: 255}, true
	case "darkgreen":
		return color.NRGBA{G: 128, A: 255}, true
	case "darkmagenta":
		return color.NRGBA{R: 128, B: 128, A: 255}, true
	case "darkred":
		return color.NRGBA{R: 128, A: 255}, true
	case "darkyellow":
		return color.NRGBA{R: 128, G: 128, A: 255}, true
	case "green":
		return color.NRGBA{G: 255, A: 255}, true
	case "lightgray":
		return color.NRGBA{R: 192, G: 192, B: 192, A: 255}, true
	case "magenta":
		return color.NRGBA{R: 255, B: 255, A: 255}, true
	case "red":
		return color.NRGBA{R: 255, A: 255}, true
	case "white":
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}, true
	case "yellow":
		return color.NRGBA{R: 255, G: 255, A: 255}, true
	default:
		return color.NRGBA{}, false
	}
}

func wordParagraphAlignment(alignment document.WordAlignment) text.Alignment {
	switch alignment {
	case document.WordAlignCenter:
		return text.Middle
	case document.WordAlignRight:
		return text.End
	case document.WordAlignJustified:
		return text.Start
	default:
		return text.Start
	}
}

func parseWordRunColor(value string) (color.NRGBA, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return color.NRGBA{}, false
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: data[0], G: data[1], B: data[2], A: 255}, true
}

func (a *App) requestWordImageDecode(state *documentSessionState, imageID string) {
	if state == nil || state.word == nil || imageID == "" {
		return
	}
	if state.wordImagePending == nil {
		state.wordImagePending = make(map[string]uint64)
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	if state.wordImageErrors == nil {
		state.wordImageErrors = make(map[string]string)
	}
	version := state.wordImageVersions[imageID]
	if pendingVersion, pending := state.wordImagePending[imageID]; pending && pendingVersion == version {
		return
	}
	if state.wordImageDecodeActive.Add(1) > wordImageDecodeConcurrency {
		state.wordImageDecodeActive.Add(-1)
		return
	}
	if a.wordImageResults == nil {
		a.wordImageResults = make(chan wordImageDecodeResult, 64)
	}
	state.wordImagePending[imageID] = version
	shutdown := a.shutdown
	results := a.wordImageResults
	window := a.window
	go func() {
		resultSent := false
		defer func() {
			if !resultSent {
				state.wordImageDecodeActive.Add(-1)
			}
		}()
		unlock := lockSessionAdapterRead(state)
		word := state.word
		var data []byte
		var err error
		if word == nil {
			err = fmt.Errorf("Word 文档已关闭")
		} else {
			data, err = word.WordImageBytes(imageID)
		}
		unlock()
		var decoded image.Image
		if err == nil {
			decoded, err = decodeWordPreview(data)
		}
		if err == nil {
			decoded = compactWordPreview(decoded)
		}
		result := wordImageDecodeResult{state: state, id: imageID, version: version, image: decoded, err: err, decodeSlot: true}
		select {
		case results <- result:
			resultSent = true
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func invalidateWordImagePreview(state *documentSessionState, imageID string) {
	if state == nil || imageID == "" {
		return
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	state.wordImageVersions[imageID]++
	state.wordImageCache.Delete(imageID)
	delete(state.wordImageErrors, imageID)
}

func invalidateAllWordImagePreviews(state *documentSessionState) {
	if state == nil {
		return
	}
	if state.wordImageCache != nil {
		state.wordImageCache.Clear()
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	for id := range state.wordImagePending {
		state.wordImageVersions[id]++
	}
	if state.word != nil {
		for _, block := range wordBlocksForState(state) {
			for _, paragraph := range wordParagraphsInBlock(block) {
				for _, inline := range paragraph.Inlines {
					if inline.Image != nil {
						state.wordImageVersions[inline.Image.ID]++
						delete(state.wordImageErrors, inline.Image.ID)
					}
				}
			}
		}
	}
}

func wordParagraphsInBlock(block document.WordBlock) []document.WordParagraph {
	switch block.Kind {
	case document.WordParagraphBlock:
		if block.Paragraph != nil {
			return []document.WordParagraph{*block.Paragraph}
		}
	case document.WordTableBlock:
		if block.Table != nil {
			var paragraphs []document.WordParagraph
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
			return paragraphs
		}
	}
	return nil
}

func reconcileWordSelection(state *documentSessionState, syncDraft bool) {
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	if state.selectedWordRunID != "" {
		if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok {
			if editor := state.wordRunEditors[run.ID]; editor != nil {
				if _, draft := state.wordDraft(run.ID); !draft {
					if editor.Text() != run.Text {
						editor.SetText(run.Text)
					}
					state.setCommittedWordText(run.ID, run.Text)
				}
			}
			if syncDraft {
				state.wordEditor.SetText(run.Text)
			}
		} else {
			state.selectedWordRunID = ""
			state.selectedWordParagraphID = ""
			state.wordEditor.SetText("")
		}
	}
	if state.selectedWordImageID != "" && findWordImage(blocks, state.selectedWordImageID) == nil {
		state.selectedWordImageID = ""
		state.wordImageFocusRequestID = ""
	}
}

func (a *App) layoutWordImage(gtx layout.Context, paragraph document.WordParagraph, item document.WordImage, overlay *wordInlineLayoutOverlay) layout.Dimensions {
	state := a.documentSessionState
	interaction := wordImageInteractionFor(state, item.ID)
	if !state.wordReadingMode {
		a.updateWordImageMoveGesture(gtx, interaction, item, paragraph.ID)
	}
	button := wordObjectButton(state, item.ID)
	decoded, cached := state.wordImageCache.Get(item.ID)
	if !cached && state.wordImageErrors[item.ID] == "" {
		a.requestWordImageDecode(state, item.ID)
	}
	preview, previewMode := wordImageLayoutPreview(interaction)
	widthEMU, heightEMU := item.WidthEMU, item.HeightEMU
	rotationDegrees := item.RotationDegrees
	if preview.Active && previewMode == wordImageGestureResize {
		widthEMU, heightEMU = preview.WidthEMU, preview.HeightEMU
	}
	if preview.Active && previewMode == wordImageGestureRotate {
		rotationDegrees = preview.RotationDegrees
	}
	maxWidth := max(1, gtx.Constraints.Max.X)
	maxHeight := gtx.Dp(unit.Dp(360))
	var sourceSize, visualSize image.Point
	if decoded != nil {
		if widthEMU <= 0 || heightEMU <= 0 {
			sourceSize = fitRotatedWordImageSize(decoded.Bounds().Dx(), decoded.Bounds().Dy(), maxWidth, maxHeight, rotationDegrees)
		} else {
			visualWidth := wordImageEMUToPixels(widthEMU, gtx.Metric.PxPerDp)
			visualHeight := wordImageEMUToPixels(heightEMU, gtx.Metric.PxPerDp)
			sourceSize = fitRotatedWordImageSize(visualWidth, visualHeight, maxWidth, maxHeight, rotationDegrees)
		}
		visualSize = rotatedWordImageBounds(sourceSize, rotationDegrees)
	}
	if !state.wordReadingMode && button.Clicked(gtx) {
		state.wordCanvasContentClickHandled = true
		selectWordImage(state, item.ID)
		state.wordImageFocusRequestID = item.ID
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if state.selectedWordImageID == item.ID && state.wordImageFocusRequestID == item.ID {
		if gtx.Focused(button) {
			state.wordImageFocusRequestID = ""
		} else {
			gtx.Execute(key.FocusCmd{Tag: button})
		}
	}
	selected := !state.wordReadingMode && state.selectedWordImageID == item.ID
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return wordMeasuredBackground(gtx, contentSurface, func(gtx layout.Context) layout.Dimensions {
			if decoded == nil {
				return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if !state.wordReadingMode {
						interaction.moveDrag.Add(gtx.Ops)
					}
					message := state.wordImageErrors[item.ID]
					if message == "" {
						message = "正在载入图片…"
					} else {
						message = "图片无法预览：" + message
					}
					return wordReadOnlyPlaceholder(gtx, a.theme, message)
				})
			}

			// Reserve the same interaction frame before and after selection so
			// showing handles never changes paragraph wrapping or document scroll.
			const imageHandlePaddingXDP = 10
			const imageRotationPaddingTopDP = 14
			const imageHandlePaddingBottomDP = 10
			paddingX := gtx.Dp(unit.Dp(imageHandlePaddingXDP))
			topPadding := gtx.Dp(unit.Dp(imageRotationPaddingTopDP))
			bottomPadding := gtx.Dp(unit.Dp(imageHandlePaddingBottomDP))
			imageRect := image.Rectangle{Min: image.Pt(paddingX, topPadding), Max: image.Pt(paddingX+visualSize.X, topPadding+visualSize.Y)}
			containerSize := image.Pt(visualSize.X+paddingX*2, visualSize.Y+topPadding+bottomPadding)
			gtx.Constraints = layout.Constraints{Min: containerSize, Max: containerSize}
			dimensions := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if !state.wordReadingMode {
					layoutWordImageMoveDragArea(gtx, interaction, imageRect, selected)
				}
				imageBounds := clip.Rect{Max: containerSize}.Push(gtx.Ops)
				semantic.DescriptionOp("内嵌图片 " + item.ID).Add(gtx.Ops)
				imageBounds.Pop()
				a.drawWordImage(gtx, decoded, sourceSize, visualSize, imageRect, rotationDegrees)
				if selected {
					a.drawWordImageSelection(gtx, imageRect)
				}
				return layout.Dimensions{Size: containerSize}
			})
			if selected {
				// Keep the handles outside the image button's clip and replay them above
				// every inline sibling. This also keeps right-edge controls visible.
				overlayRecorder := op.Record(gtx.Ops)
				a.layoutWordImageResizeHandles(gtx, interaction, item, visualSize, imageRect)
				a.layoutWordImageRotationHandle(gtx, interaction, item, imageRect)
				overlay.call = overlayRecorder.Stop()
				overlay.ready = true
			}
			return dimensions
		})
	})
}

func layoutWordImageMoveDragArea(gtx layout.Context, interaction *wordImageInteraction, imageRect image.Rectangle, selected bool) {
	moveBounds := imageRect
	if selected {
		// Keep the move gesture out of the resize handle hit zones at the image edges.
		inset := gtx.Dp(unit.Dp(12))
		moveBounds = moveBounds.Inset(inset)
		if moveBounds.Empty() {
			return
		}
	}
	area := clip.Rect{Min: moveBounds.Min, Max: moveBounds.Max}.Push(gtx.Ops)
	interaction.moveDrag.Add(gtx.Ops)
	area.Pop()
}

func (a *App) drawWordImage(gtx layout.Context, decoded image.Image, sourceSize, visualSize image.Point, imageRect image.Rectangle, rotationDegrees float64) {
	center := f32.Pt(float32(visualSize.X)/2, float32(visualSize.Y)/2)
	offset := op.Offset(imageRect.Min).Push(gtx.Ops)
	imageOffset := op.Offset(image.Pt((visualSize.X-sourceSize.X)/2, (visualSize.Y-sourceSize.Y)/2)).Push(gtx.Ops)
	rotation := op.Affine(f32.AffineId().Rotate(center, float32(rotationDegrees*math.Pi/180))).Push(gtx.Ops)
	imageGtx := gtx
	imageGtx.Constraints = layout.Constraints{Min: sourceSize, Max: sourceSize}
	widget.Image{Src: paint.NewImageOp(decoded), Fit: widget.Contain, Position: layout.Center, Scale: float32(sourceSize.X) / float32(max(1, decoded.Bounds().Dx())) / max32(1, gtx.Metric.PxPerDp)}.Layout(imageGtx)
	rotation.Pop()
	imageOffset.Pop()
	offset.Pop()
}

func (a *App) drawWordImageSelection(gtx layout.Context, imageRect image.Rectangle) {
	border := max(1, gtx.Dp(unit.Dp(1)))
	blue := color.NRGBA{R: 38, G: 117, B: 214, A: 255}
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: imageRect.Min, Max: image.Pt(imageRect.Max.X, imageRect.Min.Y+border)}.Op())
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: image.Pt(imageRect.Min.X, imageRect.Max.Y-border), Max: imageRect.Max}.Op())
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: imageRect.Min, Max: image.Pt(imageRect.Min.X+border, imageRect.Max.Y)}.Op())
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: image.Pt(imageRect.Max.X-border, imageRect.Min.Y), Max: imageRect.Max}.Op())
}

func (a *App) updateWordImageMoveGesture(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, paragraphID string) {
	for event, ok := interaction.moveDrag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = interaction.moveDrag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		if a.documentSessionState != nil {
			a.documentSessionState.wordCanvasContentClickHandled = true
		}
		switch event.Kind {
		case pointer.Press:
			interaction.moveStart = event.Position
			interaction.preview.BeginMove(item.ID, paragraphID)
		case pointer.Release:
			if interaction.preview.mode != wordImageGestureMove {
				continue
			}
			interaction.preview.ReleaseMove()
			preview := interaction.preview.Preview()
			if interaction.preview.CommitMove() {
				a.runWordImageCommand("移动图片", item.ID, func(editor document.WordImageEditor) error {
					return editor.MoveInlineImage(item.ID, preview.TargetParagraph, preview.AfterInlineID)
				}, false)
			}
			interaction.preview.Cancel()
		case pointer.Cancel:
			if interaction.preview.mode == wordImageGestureMove {
				interaction.preview.Cancel()
			}
		}
	}
}

func (a *App) layoutWordImageResizeHandles(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, frame image.Point, imageRect image.Rectangle) {
	const hitSizeDP = 18
	hitSize := gtx.Dp(unit.Dp(hitSizeDP))
	dotSize := max(6, gtx.Dp(unit.Dp(8)))
	for index, handle := range wordImageResizeHandles {
		center := image.Pt(imageRect.Min.X+frame.X/2+(handle.x*frame.X)/2, imageRect.Min.Y+frame.Y/2+(handle.y*frame.Y)/2)
		hitRect := image.Rectangle{Min: image.Pt(center.X-hitSize/2, center.Y-hitSize/2), Max: image.Pt(center.X+(hitSize+1)/2, center.Y+(hitSize+1)/2)}
		drag := &interaction.resizeDrags[index]
		area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
		drag.Add(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("拖动调整图片大小：" + handle.name).Add(gtx.Ops)
		area.Pop()
		for event, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
			a.documentSessionState.wordCanvasContentClickHandled = true
			switch event.Kind {
			case pointer.Press:
				interaction.resizeStart = event.Position
				interaction.resizeStartFrame = frame
				interaction.resizeStartEMU = item.WidthEMU
				interaction.preview.BeginResize(item.ID, item.WidthEMU, item.HeightEMU)
			case pointer.Drag:
				width := wordImageResizeWidthForHandle(interaction.resizeStartEMU, interaction.resizeStartFrame, index, event.Position.Sub(interaction.resizeStart))
				interaction.preview.ResizePreview(width)
			case pointer.Release:
				interaction.preview.ReleaseResize()
				preview := interaction.preview.Preview()
				if interaction.preview.CommitResize() {
					interaction.committingPreview = preview
					interaction.committingMode = wordImageGestureResize
					interaction.commitPending = true
					if !a.runWordImageCommand("调整图片", item.ID, func(editor document.WordImageEditor) error {
						return editor.ResizeInlineImage(item.ID, preview.WidthEMU)
					}, false) {
						finishWordImagePreviewCommand(a.documentSessionState, item.ID)
					}
				}
			case pointer.Cancel:
				interaction.preview.Cancel()
			}
		}
		paintWordImageHandle(gtx, center, dotSize)
	}
}

func (a *App) layoutWordImageRotationHandle(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, imageRect image.Rectangle) {
	const handleSizeDP = 18
	size := gtx.Dp(unit.Dp(handleSizeDP))
	gap := gtx.Dp(unit.Dp(5))
	center := f32.Pt(float32(imageRect.Min.X+imageRect.Dx()/2), float32(imageRect.Min.Y+imageRect.Dy()/2))
	rotationPoint := image.Pt(int(center.X), imageRect.Min.Y-gap)
	hitRect := image.Rectangle{Min: image.Pt(rotationPoint.X-size/2, rotationPoint.Y-size/2), Max: image.Pt(rotationPoint.X+(size+1)/2, rotationPoint.Y+(size+1)/2)}
	area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
	interaction.rotationDrag.Add(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("拖动旋转图片").Add(gtx.Ops)
	area.Pop()
	for event, ok := interaction.rotationDrag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = interaction.rotationDrag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		a.documentSessionState.wordCanvasContentClickHandled = true
		switch event.Kind {
		case pointer.Press:
			interaction.rotationStart = event.Position
			interaction.rotationCenter = center
			interaction.rotationStartDeg = item.RotationDegrees
			interaction.preview.BeginRotate(item.ID, item.RotationDegrees)
		case pointer.Drag:
			interaction.preview.RotatePreview(wordImageRotationForPointer(interaction.rotationStartDeg, interaction.rotationCenter, interaction.rotationStart, event.Position))
		case pointer.Release:
			interaction.preview.ReleaseRotate()
			preview := interaction.preview.Preview()
			if interaction.preview.CommitRotate() {
				interaction.committingPreview = preview
				interaction.committingMode = wordImageGestureRotate
				interaction.commitPending = true
				if !a.runWordImageRotationCommand(item.ID, preview.RotationDegrees) {
					finishWordImagePreviewCommand(a.documentSessionState, item.ID)
				}
			}
		case pointer.Cancel:
			interaction.preview.Cancel()
		}
	}
	dotSize := max(6, gtx.Dp(unit.Dp(8)))
	stem := image.Rectangle{Min: image.Pt(rotationPoint.X-1, rotationPoint.Y+dotSize/2), Max: image.Pt(rotationPoint.X+1, imageRect.Min.Y+1)}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 38, G: 117, B: 214, A: 255}, clip.Rect{Min: stem.Min, Max: stem.Max}.Op())
	paintWordImageHandle(gtx, rotationPoint, dotSize)
}

func paintWordImageHandle(gtx layout.Context, center image.Point, size int) {
	outer := image.Rectangle{Min: image.Pt(center.X-size/2, center.Y-size/2), Max: image.Pt(center.X+(size+1)/2, center.Y+(size+1)/2)}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 38, G: 117, B: 214, A: 255}, clip.Ellipse(outer).Op(gtx.Ops))
	innerSize := max(2, size-4)
	inner := image.Rectangle{Min: image.Pt(center.X-innerSize/2, center.Y-innerSize/2), Max: image.Pt(center.X+(innerSize+1)/2, center.Y+(innerSize+1)/2)}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Ellipse(inner).Op(gtx.Ops))
}

func (a *App) runWordImageRotationCommand(imageID string, rotation float64) bool {
	return a.runWordImageCommand("旋转图片", imageID, func(editor document.WordImageEditor) error {
		rotator, ok := any(editor).(document.WordImageRotator)
		if !ok {
			return fmt.Errorf("当前 Word 适配器不支持旋转图片")
		}
		return rotator.SetInlineImageRotation(imageID, rotation)
	}, false)
}

func wordImageEMUToPixels(widthEMU int64, pxPerDp float32) int {
	if widthEMU <= 0 || pxPerDp <= 0 {
		return 0
	}
	pixels := float64(widthEMU) * 96 * float64(pxPerDp) / 914400
	if pixels >= float64(math.MaxInt) {
		return math.MaxInt
	}
	return max(1, int(math.Round(pixels)))
}

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
