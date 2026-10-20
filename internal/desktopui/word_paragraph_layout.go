package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"glance/internal/wordlayout"
	"image"
	"image/color"
	"math"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

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
				return layoutWordPageBreak(gtx, a, pageBreak)
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

func layoutWordPageBreak(gtx layout.Context, app *App, pageBreak document.WordPageBreak) layout.Dimensions {
	semantic.DescriptionOp(app.text(i18n.Message{ID: "word.accessibility.paging_break", Args: i18n.Args{"ID": pageBreak.ID}})).Add(gtx.Ops)
	bounds := image.Rectangle{Max: gtx.Constraints.Max}
	shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(3)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 246, G: 248, B: 250, A: 255}, shape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, gridLine, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	return layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Caption(app.theme, app.textID("word.paragraph.page_break"))
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
	selectionStart, selectionEnd, selectedText := wordPageTextSelectionRange(state.wordPageTextSelection, wordlayout.TextFragment{RunID: run.ID, RuneEnd: editor.Len()})
	if selectedText {
		// The document selection spans independently edited runs. Mirror this
		// run's slice into its Editor so Web Layout can paint the same selection
		// as the page-scene renderer does in print and continuous layouts.
		editor.SetCaret(selectionStart, selectionEnd)
	}
	label := wordRunLabel(a.theme, paragraph, run)
	focusTargetPending := state.wordPendingFocusRunID != "" && state.wordPendingFocusRunID != run.ID
	paragraphSplitPending := state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID
	paragraphJoinPending := state.wordParagraphJoinIntent != nil
	editor.ReadOnly = state.wordReadingMode || !run.Editable || wordEditorOperationPending(state) || paragraphSplitPending || paragraphJoinPending || focusTargetPending || state.wordSelectionEditIntent != nil
	selectionStartBeforeUpdate, selectionEndBeforeUpdate := editor.Selection()
	documentSelectionHandled := false
	selectionInputPending := false
	if !editor.ReadOnly {
		if a.handleWordSelectionDeleteKey(gtx, state, editor) {
			editor.ReadOnly = true
		} else if a.handleWordSelectAllShortcut(gtx, state, run.ID, editor) {
			documentSelectionHandled = true
			editor.ReadOnly = true
		} else if a.handleWordHistoryShortcut(gtx, run.ID, editor) {
			editor.ReadOnly = true
		} else if a.handleWordParagraphJoinKey(gtx, paragraph, run, editor) {
			editor.ReadOnly = true
		} else {
			if a.handleWordArrowNavigation(gtx, paragraph, run, editor) {
				// Navigation can set a different pending focus target after this
				// run's initial layout checks. Refresh the snapshot before the
				// focused-run bookkeeping below, or the source run overwrites the
				// destination in the same frame.
				focusTargetPending = state.wordPendingFocusRunID != "" && state.wordPendingFocusRunID != run.ID
			}
			selectionInputPending = beginWordSelectionInputDraft(state, run.ID, editor, gtx.Focused(editor))
			for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
				switch event.(type) {
				case widget.SelectEvent:
					selectionStart, selectionEnd := editor.Selection()
					logWordDebug("flow_selection_change revision=%d paragraph=%q run=%q selection=%d:%d length=%d focused=%v",
						state.wordRevisionAtomic.Load(), paragraph.ID, run.ID, selectionStart, selectionEnd, editor.Len(), gtx.Focused(editor))
				case widget.SubmitEvent:
					selectionStart, selectionEnd := editor.Selection()
					logWordDebug("flow_submit_event revision=%d run=%q selection=%d:%d length=%d",
						state.wordRevisionAtomic.Load(), run.ID, selectionStart, selectionEnd, editor.Len())
					state.wordPageTextSelection = nil
					state.wordPageSelectionDrag = nil
					state.wordSelectionInputDraft = nil
					a.queueWordParagraphSplit(run.ID, editor.Text(), selectionStart, selectionEnd)
					if state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID {
						editor.ReadOnly = true
					}
				}
			}
			if a.finishWordSelectionInput(state, run.ID, editor) {
				selectionInputPending = true
			}
			selectionStart, selectionEnd := editor.Selection()
			if gtx.Focused(editor) && !selectionInputPending && state.wordFocusedRunID == run.ID && (selectionStart != selectionStartBeforeUpdate || selectionEnd != selectionEndBeforeUpdate) && state.wordNavigationVerticalHandledRunID != run.ID {
				state.wordPreferredVerticalXSet = false
			}
		}
	}
	editor.Alignment = wordParagraphAlignment(paragraph.Alignment)
	style := material.Editor(a.theme, editor, "")
	style.Font, style.TextSize, style.Color = label.Font, label.TextSize, label.Color
	background := contentSurface
	paragraphSelected := false
	editor.PaintSelectionWhenUnfocused = selectedText
	dimensions := wordMeasuredBackground(gtx, background, func(gtx layout.Context) layout.Dimensions {
		if highlightColor, ok := wordHighlightDisplayColor(run.Style.Highlight); ok {
			for _, rect := range wordTextHighlightRects(gtx, a.theme.Shaper, label, editor.Text()) {
				paint.FillShape(gtx.Ops, highlightColor, clip.Rect(rect).Op())
			}
		}
		if a.modal.Visible && a.modal.SearchReplace {
			for _, rect := range wordTextRangeHighlightRects(gtx, a.theme.Shaper, label, editor.Text(), state.wordSearchHighlightRangesFor(a.modal.Input.Text())[run.ID]) {
				paint.FillShape(gtx.Ops, wordPageSearchHighlightColor, clip.Rect(rect).Op())
			}
		}

		if state.wordParagraphSelections == nil {
			state.wordParagraphSelections = make(map[string]*textEditorParagraphSelection)
		}
		selection := state.wordParagraphSelections[paragraph.ID]
		if selection == nil {
			selection = new(textEditorParagraphSelection)
			state.wordParagraphSelections[paragraph.ID] = selection
		}
		dims := selection.layoutWithDoubleClickAtTextSize(gtx, editor, func(gtx layout.Context) layout.Dimensions { return layoutWordRunEditor(gtx, a, style, run.ID) }, func(caret int) bool {
			return a.selectWordPageTextSegment(gtx, state, run.ID, caret)
		}, label.TextSize)
		if selection.clicked && selection.clickedOnText {
			state.wordCanvasContentClickHandled = true
		} else if selection.clicked && !state.wordCanvasContentClickHandled {
			if a.focusLastEditableWordRun(gtx, state) {
				state.wordCanvasContentClickHandled = true
			}
		}
		if selection.paragraphSelected {
			paragraphSelected = true
			gtx.Execute(op.InvalidateCmd{})
		}
		if run.Style.Underline && dims.Size.X > 0 && dims.Size.Y > 0 {
			y := max(0, dims.Baseline)
			thickness := max(1, gtx.Dp(unit.Dp(1)))
			paint.FillShape(gtx.Ops, label.Color, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(dims.Size.X, y+thickness)}.Op())
		}
		return dims
	})
	if !paragraphSelected && !documentSelectionHandled && gtx.Focused(editor) {
		start, end := editor.Selection()
		if start != selectionStartBeforeUpdate || end != selectionEndBeforeUpdate {
			state.wordPageTextSelection = nil
		}
	}
	pendingImageFocus := state.selectedWordImageID != "" && state.wordImageFocusRequestID == state.selectedWordImageID
	if gtx.Focused(editor) && run.Editable && !focusTargetPending && !pendingImageFocus && !documentSelectionHandled && !wordPageTextSelectionActive(state.wordPageTextSelection) {
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
		if !selectionInputPending {
			captureWordEditorDraft(state, run.ID, current)
		}
		committed, _ := state.committedWordText(run.ID)
		if current != committed && state.selectedWordImageID == "" && !wordPageTextSelectionActive(state.wordPageTextSelection) {
			state.selectedWordRunID = run.ID
			state.selectedWordParagraphID = paragraph.ID
		}
	}
	if state.wordPendingFocusRunID == run.ID {
		if state.wordPendingCaretRunID == run.ID && !state.wordPendingCaretByX {
			if !wordPageTextSelectionActive(state.wordPageTextSelection) {
				editor.SetCaret(state.wordPendingCaretOffset, state.wordPendingCaretOffset)
			}
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

func layoutWordRunEditor(gtx layout.Context, a *App, style material.EditorStyle, runID string) layout.Dimensions {
	recorder := op.Record(gtx.Ops)
	dimensions := style.Layout(gtx)
	call := recorder.Stop()
	clip := clip.Rect{Max: dimensions.Size}.Push(gtx.Ops)
	semantic.DescriptionOp(a.textArgs("word.accessibility.body_text_run", i18n.Args{"ID": runID})).Add(gtx.Ops)
	call.Add(gtx.Ops)
	clip.Pop()
	return dimensions
}
