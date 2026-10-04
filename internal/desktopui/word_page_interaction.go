package desktopui

import (
	"image"
	"image/color"
	"io"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

type wordPageTransform struct {
	Origin         image.Point
	PixelsPerPoint float64
}

func (transform wordPageTransform) PageToScreen(point f32.Point) f32.Point {
	scale := float32(transform.PixelsPerPoint)
	return f32.Pt(float32(transform.Origin.X)+point.X*scale, float32(transform.Origin.Y)+point.Y*scale)
}

func (transform wordPageTransform) ScreenToPage(point f32.Point) f32.Point {
	if transform.PixelsPerPoint <= 0 || math.IsNaN(transform.PixelsPerPoint) || math.IsInf(transform.PixelsPerPoint, 0) {
		return f32.Point{}
	}
	scale := float32(transform.PixelsPerPoint)
	return f32.Pt((point.X-float32(transform.Origin.X))/scale, (point.Y-float32(transform.Origin.Y))/scale)
}

type wordVisiblePageInteraction struct {
	PageIndex int
	PageRect  image.Rectangle
	Transform wordPageTransform
}

type wordPageInteractionTag struct{ identity byte }

type wordPageCaretPosition struct {
	runID      string
	runeOffset int
}

type wordPageTextSelection struct {
	anchor   wordPageCaretPosition
	focus    wordPageCaretPosition
	runOrder map[string]int
}

type wordPageSelectionDrag struct {
	pageIndex   int
	anchor      wordPageCaretPosition
	pointerID   pointer.ID
	tag         *wordPageInteractionTag
	panning     bool
	startX      float32
	startScroll float64
	scrollLimit int
}

type wordPageHit struct {
	runID      string
	runeOffset int
	imageID    string
	found      bool
}

func beginWordPageInteractions(state *documentSessionState) {
	if state.wordPageVisibleInteractionsNext == nil {
		state.wordPageVisibleInteractionsNext = make([]wordVisiblePageInteraction, 0, 4)
	} else {
		state.wordPageVisibleInteractionsNext = state.wordPageVisibleInteractionsNext[:0]
	}
	if state.wordPageInteractionTagsNext == nil {
		state.wordPageInteractionTagsNext = make(map[int]*wordPageInteractionTag, 4)
	} else {
		clear(state.wordPageInteractionTagsNext)
	}
}

func (a *App) finishWordPageInteractions(state *documentSessionState) {
	if state == nil {
		return
	}
	state.wordPageVisibleInteractions, state.wordPageVisibleInteractionsNext = state.wordPageVisibleInteractionsNext, state.wordPageVisibleInteractions[:0]
	state.wordPageInteractionTags, state.wordPageInteractionTagsNext = state.wordPageInteractionTagsNext, state.wordPageInteractionTags
	clear(state.wordPageInteractionTagsNext)
	if drag := state.wordPageSelectionDrag; drag != nil && drag.tag != nil {
		state.wordPageInteractionTags[drag.pageIndex] = drag.tag
	}
	visibleRuns := make(map[string]struct{})
	visibleImages := make(map[string]struct{})
	if result := state.wordLayoutResult; result != nil {
		for _, visible := range state.wordPageVisibleInteractions {
			if visible.PageIndex < 0 || visible.PageIndex >= len(result.Pages) {
				continue
			}
			for _, fragment := range result.Pages[visible.PageIndex].Text {
				if fragment.Editable && fragment.RunID != "" {
					visibleRuns[fragment.RunID] = struct{}{}
				}
			}
			for _, placement := range result.Pages[visible.PageIndex].Images {
				visibleImages[placement.ImageID] = struct{}{}
			}
		}
	}
	drafts := state.wordDraftSnapshot()
	for runID, editor := range state.wordRunEditors {
		_, visible := visibleRuns[runID]
		_, drafted := drafts[runID]
		if visible || drafted || runID == state.selectedWordRunID || runID == state.wordFocusedRunID || runID == state.wordPendingFocusRunID {
			continue
		}
		captureWordTextDraft(state, runID, editor.Text())
		if _, remainsDrafted := state.wordDraft(runID); remainsDrafted {
			continue
		}
		delete(state.wordRunEditors, runID)
	}
	for imageID, interaction := range state.wordImageInteractions {
		_, visible := visibleImages[imageID]
		if visible || imageID == state.selectedWordImageID || interaction.preview.mode != wordImageGestureNone || interaction.commitPending {
			continue
		}
		delete(state.wordImageInteractions, imageID)
	}
	for imageID := range state.wordObjectButtons {
		if _, visible := visibleImages[imageID]; visible || imageID == state.selectedWordImageID {
			continue
		}
		delete(state.wordObjectButtons, imageID)
	}
}

func (a *App) layoutWordPageInteraction(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, pageIndex int, pageRect image.Rectangle, pageScale float64, horizontalOverflow int) {
	tag := state.wordPageInteractionTags[pageIndex]
	if tag == nil {
		tag = state.wordPageInteractionTagsNext[pageIndex]
	}
	if tag == nil {
		tag = new(wordPageInteractionTag)
	}
	state.wordPageInteractionTagsNext[pageIndex] = tag
	interaction := wordVisiblePageInteraction{
		PageIndex: pageIndex, PageRect: pageRect,
		Transform: wordPageTransform{Origin: pageRect.Min, PixelsPerPoint: pageScale},
	}
	if previous, ok := visibleWordPageInteraction(state.wordPageVisibleInteractions, pageIndex); ok {
		interaction = previous
	}

	for eventValue, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel}); ok; eventValue, ok = gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel}) {
		event, ok := eventValue.(pointer.Event)
		if !ok {
			continue
		}
		// Gio pointer positions are pixels local to the page tag. Convert them
		// to viewport pixels first, then invert the exact paint transform to
		// obtain page points for hit testing.
		viewportPoint := f32.Pt(
			float32(interaction.PageRect.Min.X)+event.Position.X,
			float32(interaction.PageRect.Min.Y)+event.Position.Y,
		)
		pagePoint := interaction.Transform.ScreenToPage(viewportPoint)
		switch event.Kind {
		case pointer.Press:
			if event.Buttons&pointer.ButtonPrimary == 0 {
				continue
			}
			state.wordCanvasContentClickHandled = true
			state.wordPageTextSelection = nil
			hit := wordPageHitTestWithTolerance(page, pagePoint, wordPageDevicePixelTolerance(interaction.Transform.PixelsPerPoint))
			if hit.imageID != "" {
				clearWordImageSelection(state)
				selectWordImage(state, hit.imageID)
				state.wordImageFocusRequestID = ""
				state.wordPageSelectionDrag = nil
				if a.window != nil {
					a.window.Invalidate()
				}
				continue
			}
			if hit.runID == "" {
				if horizontalOverflow > 0 {
					state.wordPageSelectionDrag = &wordPageSelectionDrag{
						pageIndex: pageIndex, pointerID: event.PointerID, tag: tag,
						panning: true, startX: viewportPoint.X, startScroll: state.wordPageScrollX, scrollLimit: horizontalOverflow,
					}
					gtx.Execute(pointer.GrabCmd{Tag: tag, ID: event.PointerID})
					continue
				}
				state.selectedWordRunID = ""
				state.selectedWordParagraphID = ""
				clearWordImageSelection(state)
				state.wordPageSelectionDrag = nil
				continue
			}
			a.focusWordPageRun(gtx, state, hit.runID, hit.runeOffset, hit.runeOffset)
			position := wordPageCaretPosition{runID: hit.runID, runeOffset: hit.runeOffset}
			state.wordPageTextSelection = newWordPageTextSelection(state.wordLayoutResult, position)
			state.wordPageSelectionDrag = &wordPageSelectionDrag{pageIndex: pageIndex, anchor: position, pointerID: event.PointerID, tag: tag}
			state.wordPagePreferredVerticalXSet = false
			gtx.Execute(pointer.GrabCmd{Tag: tag, ID: event.PointerID})
		case pointer.Drag:
			drag := state.wordPageSelectionDrag
			if drag == nil || drag.pointerID != event.PointerID {
				continue
			}
			if drag.panning {
				state.wordPageScrollX = math.Max(0, math.Min(float64(drag.scrollLimit), drag.startScroll-float64(viewportPoint.X-drag.startX)))
				continue
			}
			hit, hitPage := hitVisibleWordPage(state, viewportPoint)
			if !hit.found && (hitPage == pageIndex || hitPage < 0) {
				hit = wordPageHitTestNearestWithTolerance(page, pagePoint, wordPageDevicePixelTolerance(interaction.Transform.PixelsPerPoint))
			}
			if hit.found {
				position := wordPageCaretPosition{runID: hit.runID, runeOffset: hit.runeOffset}
				if selection := state.wordPageTextSelection; selection != nil {
					if position.runID != selection.anchor.runID {
						populateWordPageRunOrder(selection, state.wordLayoutResult)
					}
					selection.focus = position
				}
				if editor := state.wordRunEditors[drag.anchor.runID]; editor != nil {
					if position.runID == drag.anchor.runID {
						editor.SetCaret(drag.anchor.runeOffset, position.runeOffset)
					} else {
						editor.SetCaret(drag.anchor.runeOffset, drag.anchor.runeOffset)
					}
				}
			}
		case pointer.Release, pointer.Cancel:
			if drag := state.wordPageSelectionDrag; drag != nil && drag.pointerID == event.PointerID {
				state.wordPageSelectionDrag = nil
				if !wordPageTextSelectionActive(state.wordPageTextSelection) {
					state.wordPageTextSelection = nil
				}
			}
		}
	}
	a.layoutWordPageFocusedEditor(gtx, state, page, pageIndex, pageRect, pageScale)
	// The focused Gio editor is registered first by layoutWordPageFocusedEditor.
	// Register the page hit target after it so all pointer input remains routed
	// through PageScene hit testing instead of the editor's separate text layout.
	pageOffset := op.Offset(pageRect.Min).Push(gtx.Ops)
	pageClip := clip.Rect{Max: pageRect.Size()}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	pointer.CursorText.Add(gtx.Ops)
	pageClip.Pop()
	pageOffset.Pop()
}

func (a *App) focusWordPageRun(gtx layout.Context, state *documentSessionState, runID string, start, end int) {
	run, paragraph, ok := findEditableWordRunInState(state, runID)
	if !ok || !run.Editable {
		return
	}
	state.wordPageTextSelection = nil
	state.wordPageFocusedMetaRunID = runID
	state.wordPageFocusedMetaRevision = state.wordRevisionAtomic.Load()
	state.wordPageFocusedParagraph = paragraph
	paragraphID := paragraph.ID
	editor := wordRunEditor(state, run)
	if start < 0 {
		start = 0
	}
	if start > editor.Len() {
		start = editor.Len()
	}
	if end < 0 {
		end = 0
	}
	if end > editor.Len() {
		end = editor.Len()
	}
	editor.SetCaret(start, end)
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraphID
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordPendingFocusRunID = runID
	state.wordPendingCaretRunID = runID
	state.wordPendingCaretOffset = end
	state.wordPendingCaretByX = false
	state.wordLayoutFocusedRunID = runID
	state.wordToolbarTab = wordTabHome
	gtx.Execute(op.InvalidateCmd{})
}

func requestWordPageScrollForKeyboardCaret(state *documentSessionState, runID string, runeOffset int) {
	if state == nil || state.wordLayoutResult == nil {
		return
	}
	position, ok := state.wordLayoutResult.Locations[wordlayout.SourcePosition{RunID: runID, RuneOffset: runeOffset}]
	if !ok {
		return
	}
	columns := 1
	if state.wordViewMode == wordViewPrintLayout && state.wordMultiplePagesMode {
		columns = 2
	}
	targetRow := position.PageIndex / columns
	firstRow := max(0, state.wordPageListState.Position.First)
	visibleRows := max(1, state.wordPageListState.Position.Count)
	if targetRow < firstRow || targetRow >= firstRow+visibleRows {
		state.wordPagePendingScrollRunID = runID
	}
}

func (a *App) wordPageEditorMetadata(state *documentSessionState, runID string) (document.WordTextRun, document.WordParagraph, bool) {
	if state == nil || runID == "" {
		return document.WordTextRun{}, document.WordParagraph{}, false
	}
	revision := state.wordRevisionAtomic.Load()
	if state.wordPageFocusedMetaRunID == runID && state.wordPageFocusedMetaRevision == revision {
		return document.WordTextRun{ID: runID, Editable: true}, state.wordPageFocusedParagraph, true
	}
	run, paragraph, ok := findEditableWordRunInState(state, runID)
	if !ok {
		return document.WordTextRun{}, document.WordParagraph{}, false
	}
	state.wordPageFocusedMetaRunID = runID
	state.wordPageFocusedMetaRevision = revision
	state.wordPageFocusedParagraph = paragraph
	return run, paragraph, true
}

func visibleWordPageInteraction(interactions []wordVisiblePageInteraction, pageIndex int) (wordVisiblePageInteraction, bool) {
	for _, interaction := range interactions {
		if interaction.PageIndex == pageIndex {
			return interaction, true
		}
	}
	return wordVisiblePageInteraction{}, false
}

func hitVisibleWordPage(state *documentSessionState, point f32.Point) (wordPageHit, int) {
	if state == nil || state.wordLayoutResult == nil {
		return wordPageHit{}, -1
	}
	for _, interaction := range state.wordPageVisibleInteractions {
		if !pointInWordPageRect(point, interaction.PageRect) || interaction.PageIndex < 0 || interaction.PageIndex >= len(state.wordLayoutResult.Pages) {
			continue
		}
		pagePoint := interaction.Transform.ScreenToPage(point)
		page := state.wordLayoutResult.Pages[interaction.PageIndex]
		hit := wordPageHitTestWithTolerance(page, pagePoint, wordPageDevicePixelTolerance(interaction.Transform.PixelsPerPoint))
		if !hit.found {
			hit = wordPageHitTestNearestWithTolerance(page, pagePoint, wordPageDevicePixelTolerance(interaction.Transform.PixelsPerPoint))
		}
		if hit.found {
			return hit, interaction.PageIndex
		}
	}
	return wordPageHit{}, -1
}

func newWordPageTextSelection(result *wordlayout.Result, anchor wordPageCaretPosition) *wordPageTextSelection {
	if result == nil {
		return nil
	}
	return &wordPageTextSelection{anchor: anchor, focus: anchor}
}

func populateWordPageRunOrder(selection *wordPageTextSelection, result *wordlayout.Result) {
	if selection == nil || result == nil || selection.runOrder != nil {
		return
	}
	selection.runOrder = make(map[string]int)
	for _, page := range result.Pages {
		for _, fragment := range page.Text {
			if !fragment.Editable || fragment.RunID == "" {
				continue
			}
			if _, exists := selection.runOrder[fragment.RunID]; !exists {
				selection.runOrder[fragment.RunID] = len(selection.runOrder)
			}
		}
	}
}

func wordPageTextSelectionActive(selection *wordPageTextSelection) bool {
	return selection != nil && (selection.anchor.runID != selection.focus.runID || selection.anchor.runeOffset != selection.focus.runeOffset)
}

func wordPageOrderedSelection(selection *wordPageTextSelection) (wordPageCaretPosition, wordPageCaretPosition, bool) {
	if !wordPageTextSelectionActive(selection) {
		return wordPageCaretPosition{}, wordPageCaretPosition{}, false
	}
	anchor, focus := selection.anchor, selection.focus
	anchorOrder, anchorOK := selection.runOrder[anchor.runID]
	focusOrder, focusOK := selection.runOrder[focus.runID]
	if anchor.runID == focus.runID {
		if anchor.runeOffset <= focus.runeOffset {
			return anchor, focus, true
		}
		return focus, anchor, true
	}
	if anchorOK && focusOK && anchorOrder <= focusOrder {
		return anchor, focus, true
	}
	return focus, anchor, true
}

func wordPageTextSelectionRange(selection *wordPageTextSelection, fragment wordlayout.TextFragment) (int, int, bool) {
	start, end, active := wordPageOrderedSelection(selection)
	if !active {
		return 0, 0, false
	}
	if start.runID == end.runID {
		if fragment.RunID != start.runID {
			return 0, 0, false
		}
		return max(fragment.RuneStart, start.runeOffset), min(fragment.RuneEnd, end.runeOffset), max(fragment.RuneStart, start.runeOffset) < min(fragment.RuneEnd, end.runeOffset)
	}
	fragmentOrder, exists := selection.runOrder[fragment.RunID]
	startOrder, startExists := selection.runOrder[start.runID]
	endOrder, endExists := selection.runOrder[end.runID]
	if !exists || !startExists || !endExists || fragmentOrder < startOrder || fragmentOrder > endOrder {
		return 0, 0, false
	}
	runeStart, runeEnd := fragment.RuneStart, fragment.RuneEnd
	if fragment.RunID == start.runID {
		runeStart = max(runeStart, start.runeOffset)
	}
	if fragment.RunID == end.runID {
		runeEnd = min(runeEnd, end.runeOffset)
	}
	return runeStart, runeEnd, runeStart < runeEnd
}

func wordPageSelectedText(state *documentSessionState) (string, bool) {
	selection := state.wordPageTextSelection
	start, end, active := wordPageOrderedSelection(selection)
	if !active {
		return "", false
	}
	startOrder, endOrder := 0, 0
	if start.runID != end.runID {
		var startExists, endExists bool
		startOrder, startExists = selection.runOrder[start.runID]
		endOrder, endExists = selection.runOrder[end.runID]
		if !startExists || !endExists {
			return "", false
		}
	}
	var value strings.Builder
	previousParagraphID := ""
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			run := inline.Text
			if inline.Kind != document.WordTextInline || run == nil || !run.Editable {
				continue
			}
			if start.runID == end.runID {
				if run.ID != start.runID {
					continue
				}
			} else {
				runOrder, exists := selection.runOrder[run.ID]
				if !exists || runOrder < startOrder || runOrder > endOrder {
					continue
				}
			}
			text := run.Text
			if editor := state.wordRunEditors[run.ID]; editor != nil {
				text = editor.Text()
			} else if draft, exists := state.wordDraft(run.ID); exists {
				text = draft
			}
			runes := []rune(text)
			from, to := 0, len(runes)
			if run.ID == start.runID {
				from = max(0, min(start.runeOffset, len(runes)))
			}
			if run.ID == end.runID {
				to = max(0, min(end.runeOffset, len(runes)))
			}
			if from >= to {
				continue
			}
			if value.Len() > 0 && previousParagraphID != paragraph.ID {
				value.WriteByte('\n')
			}
			value.WriteString(string(runes[from:to]))
			previousParagraphID = paragraph.ID
		}
	}
	if value.Len() == 0 {
		return "", false
	}
	return value.String(), true
}

func pointInWordPageRect(point f32.Point, rect image.Rectangle) bool {
	return point.X >= float32(rect.Min.X) && point.X < float32(rect.Max.X) && point.Y >= float32(rect.Min.Y) && point.Y < float32(rect.Max.Y)
}

func wordParagraphForRun(blocks []document.WordBlock, runID string) string {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return paragraph.ID
			}
		}
	}
	return ""
}

func wordPageHitTest(page wordlayout.PageScene, point f32.Point) wordPageHit {
	return wordPageHitTestWithTolerance(page, point, 0)
}

// wordPageDevicePixelTolerance makes geometrical caret midpoints stable when
// they are rounded to an integer device pixel. Without the half-pixel band,
// repeated CJK glyph centers can all round just to the trailing side and place
// the caret one rune after the character under the pointer.
func wordPageDevicePixelTolerance(pixelsPerPoint float64) float64 {
	if pixelsPerPoint <= 0 || math.IsNaN(pixelsPerPoint) || math.IsInf(pixelsPerPoint, 0) {
		return 0
	}
	return 0.5 / pixelsPerPoint
}

func wordPageHitTestWithTolerance(page wordlayout.PageScene, point f32.Point, midpointTolerancePT float64) wordPageHit {
	for index := len(page.Images) - 1; index >= 0; index-- {
		placement := page.Images[index]
		bounds := placement.Bounds
		if float64(point.X) >= bounds.XPT && float64(point.X) <= bounds.XPT+bounds.WidthPT && float64(point.Y) >= bounds.YPT && float64(point.Y) <= bounds.YPT+bounds.HeightPT {
			return wordPageHit{imageID: placement.ImageID, found: true}
		}
	}
	best := wordPageHit{}
	bestScore := math.MaxFloat64
	for _, fragment := range page.Text {
		if !fragment.Editable || fragment.RunID == "" {
			continue
		}
		top := fragment.BaselineYPT - math.Max(1, fragment.Style.FontSize*0.9)
		bottom := fragment.BaselineYPT + math.Max(1, fragment.Style.FontSize*0.3)
		y := float64(point.Y)
		if y < top-3 || y > bottom+3 {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			distanceX := math.Abs(float64(point.X) - fragment.XPT)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score < bestScore {
				best = wordPageHit{runID: fragment.RunID, runeOffset: fragment.RuneStart, found: true}
				bestScore = score
			}
			continue
		}
		for _, glyph := range fragment.Glyphs {
			left := glyph.XPT
			right := glyph.XPT + glyph.AdvanceXPT
			if right < left {
				left, right = right, left
			}
			x := math.Max(left, math.Min(right, float64(point.X)))
			distanceX := math.Abs(float64(point.X) - x)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score >= bestScore {
				continue
			}
			offset := glyph.RuneStart
			if float64(point.X) > (left+right)/2+midpointTolerancePT {
				offset = glyph.RuneEnd
			}
			best = wordPageHit{runID: fragment.RunID, runeOffset: offset, found: true}
			bestScore = score
		}
	}
	return best
}

// wordPageHitTestNearest extends an active drag to the nearest editable caret
// even when the pointer drifts outside the narrow text-line hit band. Clicks
// still use wordPageHitTest so blank page clicks do not unexpectedly focus text.
func wordPageHitTestNearest(page wordlayout.PageScene, point f32.Point) wordPageHit {
	return wordPageHitTestNearestWithTolerance(page, point, 0)
}

func wordPageHitTestNearestWithTolerance(page wordlayout.PageScene, point f32.Point, midpointTolerancePT float64) wordPageHit {
	best := wordPageHit{}
	bestScore := math.MaxFloat64
	y := float64(point.Y)
	for _, fragment := range page.Text {
		if !fragment.Editable || fragment.RunID == "" {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			distanceX := math.Abs(float64(point.X) - fragment.XPT)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score < bestScore {
				best = wordPageHit{runID: fragment.RunID, runeOffset: fragment.RuneStart, found: true}
				bestScore = score
			}
			continue
		}
		for _, glyph := range fragment.Glyphs {
			left, right := glyph.XPT, glyph.XPT+glyph.AdvanceXPT
			if right < left {
				left, right = right, left
			}
			x := math.Max(left, math.Min(right, float64(point.X)))
			distanceX := math.Abs(float64(point.X) - x)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score >= bestScore {
				continue
			}
			offset := glyph.RuneStart
			if float64(point.X) > (left+right)/2+midpointTolerancePT {
				offset = glyph.RuneEnd
			}
			best = wordPageHit{runID: fragment.RunID, runeOffset: offset, found: true}
			bestScore = score
		}
	}
	return best
}

func wordPageHitTestRun(page wordlayout.PageScene, point f32.Point, runID string) wordPageHit {
	best := wordPageHit{}
	bestScore := math.MaxFloat64
	for _, fragment := range page.Text {
		if fragment.RunID != runID {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			if !best.found {
				best = wordPageHit{runID: runID, runeOffset: fragment.RuneStart, found: true}
			}
			continue
		}
		for _, glyph := range fragment.Glyphs {
			left, right := glyph.XPT, glyph.XPT+glyph.AdvanceXPT
			if right < left {
				left, right = right, left
			}
			x := math.Max(left, math.Min(right, float64(point.X)))
			yDistance := math.Abs(float64(point.Y) - fragment.BaselineYPT)
			score := yDistance*2 + math.Abs(float64(point.X)-x)
			if score >= bestScore {
				continue
			}
			offset := glyph.RuneStart
			if float64(point.X) > (left+right)/2 {
				offset = glyph.RuneEnd
			}
			best, bestScore = wordPageHit{runID: runID, runeOffset: offset, found: true}, score
		}
	}
	return best
}

func (a *App) layoutWordPageFocusedEditor(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, pageIndex int, pageRect image.Rectangle, pageScale float64) {
	runID := state.selectedWordRunID
	editor := state.wordRunEditors[runID]
	if runID == "" || editor == nil || !wordPageHasEditableRun(page, runID) {
		return
	}
	run, paragraph, ok := a.wordPageEditorMetadata(state, runID)
	if !ok || !run.Editable {
		return
	}
	_, currentCaret := editor.Selection()
	if position, exists := state.wordLayoutResult.Locations[wordlayout.SourcePosition{RunID: runID, RuneOffset: currentCaret}]; exists {
		if position.PageIndex != pageIndex {
			return
		}
	} else {
		for _, visible := range state.wordPageVisibleInteractions {
			if visible.PageIndex == pageIndex {
				break
			}
			if visible.PageIndex >= 0 && visible.PageIndex < len(state.wordLayoutResult.Pages) && wordPageHasEditableRun(state.wordLayoutResult.Pages[visible.PageIndex], runID) {
				return
			}
		}
	}
	caretStartBeforeUpdate, caretEndBeforeUpdate := editor.Selection()
	editor.SingleLine = false
	editor.Submit = true
	editor.ReadOnly = state.wordReadingMode || !run.Editable || state.formatRunning.Load() || a.isSaving()
	paragraphJoinPending := state.wordParagraphJoinIntent != nil
	paragraphSplitPending := state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == runID
	focusTargetPending := state.wordPendingFocusRunID != "" && state.wordPendingFocusRunID != runID
	editor.ReadOnly = editor.ReadOnly || paragraphJoinPending || paragraphSplitPending || focusTargetPending
	pageOps := op.Offset(pageRect.Min).Push(gtx.Ops)
	pageClip := clip.Rect{Max: pageRect.Size()}.Push(gtx.Ops)
	event.Op(gtx.Ops, editor)
	key.InputHintOp{Tag: editor, Hint: editor.InputHint}.Add(gtx.Ops)
	pageClip.Pop()
	pageOps.Pop()
	if state.wordPendingFocusRunID == runID {
		applyWordPagePendingCaret(state, editor, runID, page, pageScale)
		gtx.Execute(key.FocusCmd{Tag: editor})
		state.wordPendingFocusRunID = ""
	}
	if !editor.ReadOnly && a.handleWordParagraphJoinKey(gtx, paragraph, run, editor) {
		editor.ReadOnly = true
	}
	if a.handleWordPageArrowNavigation(gtx, state, editor) {
		state.wordPagePreferredVerticalXSet = false
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		for event, ok := gtx.Event(key.Filter{Focus: editor, Name: "C", Required: key.ModShortcut}); ok; event, ok = gtx.Event(key.Filter{Focus: editor, Name: "C", Required: key.ModShortcut}) {
			keyEvent, ok := event.(key.Event)
			if !ok || keyEvent.State != key.Press {
				continue
			}
			if text, selected := wordPageSelectedText(state); selected {
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
			}
		}
	}
	for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
		if _, submit := event.(widget.SubmitEvent); submit {
			start, end := editor.Selection()
			a.queueWordParagraphSplit(runID, editor.Text(), start, end)
			editor.ReadOnly = true
		}
	}
	label := wordRunLabel(a.theme, paragraph, run)
	label.TextSize = unit.Sp(float32(label.TextSize) * float32(wordPageZoomFromScale(gtx, pageScale)))
	editor.Alignment = wordParagraphAlignment(paragraph.Alignment)
	style := material.Editor(a.theme, editor, "")
	style.Font, style.TextSize = label.Font, label.TextSize
	style.Color = label.Color
	editorGtx := gtx
	editorGtx.Constraints = layout.Constraints{Max: image.Pt(
		max(1, int(math.Round(page.Content.WidthPT*pageScale))),
		max(1, int(math.Round(page.Content.HeightPT*pageScale))),
	)}
	recorder := op.Record(editorGtx.Ops)
	style.Layout(editorGtx)
	_ = recorder.Stop()
	captureWordTextDraft(state, runID, editor.Text())
	selectionStart, selectionEnd := editor.Selection()
	if state.wordLayoutResult != nil {
		if position, exists := state.wordLayoutResult.Locations[wordlayout.SourcePosition{RunID: runID, RuneOffset: selectionEnd}]; exists && position.PageIndex == pageIndex {
			caret := wordPageSceneCaret(page, runID, position.XPT, position.YPT, pageScale)
			start, end := selectionStart, selectionEnd
			if start > end {
				start, end = end, start
			}
			gtx.Execute(key.SelectionCmd{Tag: editor, Range: key.Range{Start: start, End: end}, Caret: caret})
		}
	}
	if gtx.Focused(editor) && (selectionStart != caretStartBeforeUpdate || selectionEnd != caretEndBeforeUpdate) {
		state.wordPagePreferredVerticalXSet = false
		if state.wordPageSelectionDrag == nil {
			state.wordPageTextSelection = nil
		}
	}
	if gtx.Focused(editor) && (state.wordPendingFocusRunID == "" || state.wordPendingFocusRunID == runID) {
		state.wordFocusedRunID = runID
		state.wordLayoutFocusedRunID = runID
		state.selectedWordRunID = runID
		state.selectedWordParagraphID = paragraph.ID
		if state.wordFormatPainterActive && runID != state.wordFormatPainterSourceRunID {
			state.wordFormatPainterPendingParagraphID = paragraph.ID
			state.wordFormatPainterPendingRunID = runID
		}
	}
}

func applyWordPagePendingCaret(state *documentSessionState, editor *widget.Editor, runID string, page wordlayout.PageScene, pageScale float64) {
	if state == nil || editor == nil || state.wordPendingCaretRunID != runID {
		return
	}
	caret := max(0, min(editor.Len(), state.wordPendingCaretOffset))
	if state.wordPendingCaretByX {
		caret = wordClosestPageCaretOffset(page, runID, float64(state.wordPendingCaretX), pageScale, editor.Len())
	}
	// A press and its drag/release may be drained in the same frame before the
	// editor's first focus layout. In that case the pointer path has already
	// installed the drag selection; replaying the original click caret here
	// would collapse it after release.
	if !wordPageTextSelectionActive(state.wordPageTextSelection) {
		editor.SetCaret(caret, caret)
	}
	state.wordPendingCaretRunID = ""
	state.wordPendingCaretByX = false
}

func wordPageHasEditableRun(page wordlayout.PageScene, runID string) bool {
	for _, fragment := range page.Text {
		if fragment.Editable && fragment.RunID == runID {
			return true
		}
	}
	return false
}

func wordPageZoomFromScale(gtx layout.Context, pageScale float64) float64 {
	base := float64(gtx.Metric.PxPerDp) * (4.0 / 3.0)
	if base <= 0 || pageScale <= 0 {
		return 1
	}
	return pageScale / base
}

func wordPageSceneCaret(page wordlayout.PageScene, runID string, xPT, yPT, pageScale float64) key.Caret {
	fontSize := 11.0
	for _, fragment := range page.Text {
		if fragment.RunID == runID && math.Abs(fragment.BaselineYPT-yPT) <= math.Max(1, fragment.Style.FontSize*0.8) {
			fontSize = math.Max(1, fragment.Style.FontSize)
			break
		}
	}
	return key.Caret{
		Pos:     f32.Pt(float32(xPT*pageScale), float32(yPT*pageScale)),
		Ascent:  float32(fontSize * pageScale * 0.82),
		Descent: float32(fontSize * pageScale * 0.28),
	}
}

func wordClosestPageCaretOffset(page wordlayout.PageScene, runID string, x float64, pageScale float64, length int) int {
	bestOffset, bestDistance := 0, math.MaxFloat64
	for _, fragment := range page.Text {
		if fragment.RunID != runID {
			continue
		}
		for _, glyph := range fragment.Glyphs {
			for _, candidate := range []struct {
				offset int
				x      float64
			}{{glyph.RuneStart, glyph.XPT * pageScale}, {glyph.RuneEnd, (glyph.XPT + glyph.AdvanceXPT) * pageScale}} {
				distance := math.Abs(candidate.x - x)
				if distance < bestDistance {
					bestOffset, bestDistance = candidate.offset, distance
				}
			}
		}
	}
	return max(0, min(length, bestOffset))
}

func (a *App) handleWordPageArrowNavigation(gtx layout.Context, state *documentSessionState, editor *widget.Editor) bool {
	if state == nil || editor == nil || !gtx.Focused(editor) || editor.ReadOnly {
		return false
	}
	event, ok := gtx.Event(
		key.Filter{Focus: editor, Name: key.NameUpArrow},
		key.Filter{Focus: editor, Name: key.NameDownArrow},
		key.Filter{Focus: editor, Name: key.NameLeftArrow},
		key.Filter{Focus: editor, Name: key.NameRightArrow},
		key.Filter{Focus: editor, Name: key.NameHome},
		key.Filter{Focus: editor, Name: key.NameEnd},
		key.Filter{Focus: editor, Name: key.NamePageUp},
		key.Filter{Focus: editor, Name: key.NamePageDown},
	)
	if !ok {
		return false
	}
	keyEvent, ok := event.(key.Event)
	if !ok || keyEvent.State != key.Press || keyEvent.Modifiers != 0 {
		return false
	}
	if keyEvent.Name == key.NameLeftArrow || keyEvent.Name == key.NameRightArrow {
		start, end := editor.Selection()
		runID := state.selectedWordRunID
		if start != end {
			if keyEvent.Name == key.NameLeftArrow {
				editor.SetCaret(start, start)
			} else {
				editor.SetCaret(end, end)
			}
			return true
		}
		direction := 1
		if gtx.Locale.Direction.Progression() == system.TowardOrigin {
			direction = -1
		}
		if keyEvent.Name == key.NameLeftArrow {
			direction = -direction
		}
		runs := wordNavigationRuns(wordBlocksForState(state))
		index := -1
		for i := range runs {
			if runs[i].run.ID == runID {
				index = i
				break
			}
		}
		if index < 0 {
			return false
		}
		if direction < 0 && start == 0 && index > 0 {
			target := runs[index-1].run
			targetEditor := wordRunEditor(state, target)
			a.focusWordPageRun(gtx, state, target.ID, targetEditor.Len(), targetEditor.Len())
			requestWordPageScrollForKeyboardCaret(state, target.ID, targetEditor.Len())
			return true
		}
		if direction > 0 && end == editor.Len() && index+1 < len(runs) {
			target := runs[index+1].run
			a.focusWordPageRun(gtx, state, target.ID, 0, 0)
			requestWordPageScrollForKeyboardCaret(state, target.ID, 0)
			return true
		}
		caret := max(0, min(editor.Len(), end+direction))
		editor.SetCaret(caret, caret)
		return true
	}
	if keyEvent.Name == key.NameHome || keyEvent.Name == key.NameEnd {
		start, end := editor.Selection()
		if start != end {
			return false
		}
		result := state.wordLayoutResult
		if result == nil {
			return false
		}
		position, exists := result.Locations[wordlayout.SourcePosition{RunID: state.selectedWordRunID, RuneOffset: end}]
		if !exists || position.PageIndex < 0 || position.PageIndex >= len(result.Pages) {
			return false
		}
		lineStart, lineEnd, found := wordPageRunLineCaretBounds(result.Pages[position.PageIndex], state.selectedWordRunID, position.YPT)
		if !found {
			return false
		}
		offset := lineStart
		if keyEvent.Name == key.NameEnd {
			offset = lineEnd
		}
		editor.SetCaret(offset, offset)
		return true
	}
	if keyEvent.Name == key.NamePageUp || keyEvent.Name == key.NamePageDown {
		start, end := editor.Selection()
		if start != end {
			return false
		}
		direction := -1
		if keyEvent.Name == key.NamePageDown {
			direction = 1
		}
		if a.moveWordPageCaretByPage(gtx, state, direction) {
			return true
		}
		return false
	}
	direction := -1
	if keyEvent.Name == key.NameDownArrow {
		direction = 1
	}
	start, end := editor.Selection()
	offset := end
	if direction < 0 {
		offset = start
	}
	result := state.wordLayoutResult
	position, ok := result.Locations[wordlayout.SourcePosition{RunID: state.selectedWordRunID, RuneOffset: offset}]
	if !ok {
		return false
	}
	if !state.wordPagePreferredVerticalXSet {
		state.wordPagePreferredVerticalX = position.XPT
		state.wordPagePreferredVerticalXSet = true
	}
	fontSize := 11.0
	for _, page := range result.Pages[max(0, position.PageIndex-1):min(len(result.Pages), position.PageIndex+2)] {
		for _, fragment := range page.Text {
			if fragment.RunID == state.selectedWordRunID {
				fontSize = math.Max(1, fragment.Style.FontSize)
			}
		}
	}
	currentY := float64(position.PageIndex)*10000 + position.YPT
	targetY := currentY + float64(direction)*fontSize*1.2
	bestScore := math.MaxFloat64
	best := wordPageHit{}
	for pageIndex := max(0, position.PageIndex-1); pageIndex < min(len(result.Pages), position.PageIndex+2); pageIndex++ {
		for _, fragment := range result.Pages[pageIndex].Text {
			if !fragment.Editable || fragment.RunID == "" {
				continue
			}
			for _, glyph := range fragment.Glyphs {
				for _, candidate := range []struct {
					offset int
					x      float64
				}{{glyph.RuneStart, glyph.XPT}, {glyph.RuneEnd, glyph.XPT + glyph.AdvanceXPT}} {
					candidatePosition, exists := result.Locations[wordlayout.SourcePosition{RunID: fragment.RunID, RuneOffset: candidate.offset}]
					if !exists {
						continue
					}
					candidateY := float64(pageIndex)*10000 + candidatePosition.YPT
					deltaY := candidateY - currentY
					if float64(direction)*deltaY < fontSize*0.45 {
						continue
					}
					score := math.Abs(candidateY-targetY)*3 + math.Abs(candidate.x-state.wordPagePreferredVerticalX)
					if score < bestScore {
						best, bestScore = wordPageHit{runID: fragment.RunID, runeOffset: candidate.offset, found: true}, score
					}
				}
			}
		}
	}
	if !best.found {
		return true
	}
	a.focusWordPageRun(gtx, state, best.runID, best.runeOffset, best.runeOffset)
	requestWordPageScrollForKeyboardCaret(state, best.runID, best.runeOffset)
	return true
}

func wordPageRunLineCaretBounds(page wordlayout.PageScene, runID string, yPT float64) (int, int, bool) {
	lineY, bestDistance := 0.0, math.MaxFloat64
	lineFound := false
	for _, fragment := range page.Text {
		if fragment.RunID != runID {
			continue
		}
		distance := math.Abs(fragment.BaselineYPT - yPT)
		if distance < bestDistance {
			lineY, bestDistance, lineFound = fragment.BaselineYPT, distance, true
		}
	}
	if !lineFound {
		return 0, 0, false
	}
	lineStart, lineEnd := int(^uint(0)>>1), 0
	for _, fragment := range page.Text {
		if fragment.RunID != runID || math.Abs(fragment.BaselineYPT-lineY) > math.Max(1, fragment.Style.FontSize*0.25) {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			lineStart = min(lineStart, fragment.RuneStart)
			lineEnd = max(lineEnd, fragment.RuneEnd)
		}
		for _, glyph := range fragment.Glyphs {
			lineStart = min(lineStart, glyph.RuneStart)
			lineEnd = max(lineEnd, glyph.RuneEnd)
		}
	}
	if lineStart == int(^uint(0)>>1) {
		return 0, 0, false
	}
	return lineStart, lineEnd, true
}

func (a *App) moveWordPageCaretByPage(gtx layout.Context, state *documentSessionState, direction int) bool {
	if state == nil || state.wordLayoutResult == nil || state.selectedWordRunID == "" {
		return false
	}
	editor := state.wordRunEditors[state.selectedWordRunID]
	if editor == nil {
		return false
	}
	_, offset := editor.Selection()
	position, ok := state.wordLayoutResult.Locations[wordlayout.SourcePosition{RunID: state.selectedWordRunID, RuneOffset: offset}]
	if !ok {
		return false
	}
	targetPage := position.PageIndex + direction
	if targetPage < 0 || targetPage >= len(state.wordLayoutResult.Pages) {
		return false
	}
	bestScore := math.MaxFloat64
	best := wordPageHit{}
	for _, fragment := range state.wordLayoutResult.Pages[targetPage].Text {
		if !fragment.Editable || fragment.RunID == "" {
			continue
		}
		for _, glyph := range fragment.Glyphs {
			for _, candidate := range []struct {
				offset int
				x      float64
			}{{glyph.RuneStart, glyph.XPT}, {glyph.RuneEnd, glyph.XPT + glyph.AdvanceXPT}} {
				candidatePosition, exists := state.wordLayoutResult.Locations[wordlayout.SourcePosition{RunID: fragment.RunID, RuneOffset: candidate.offset}]
				if !exists || candidatePosition.PageIndex != targetPage {
					continue
				}
				score := math.Abs(candidatePosition.YPT-position.YPT)*3 + math.Abs(candidate.x-position.XPT)
				if score < bestScore {
					bestScore = score
					best = wordPageHit{runID: fragment.RunID, runeOffset: candidate.offset, found: true}
				}
			}
		}
	}
	if !best.found {
		return false
	}
	a.focusWordPageRun(gtx, state, best.runID, best.runeOffset, best.runeOffset)
	requestWordPageScrollForKeyboardCaret(state, best.runID, best.runeOffset)
	state.wordPagePreferredVerticalXSet = false
	return true
}

func drawWordPageSelection(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, origin image.Point, pixelsPerPoint float64) {
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		for _, fragment := range page.Text {
			start, end, selected := wordPageTextSelectionRange(state.wordPageTextSelection, fragment)
			if selected {
				drawWordPageFragmentSelection(gtx, fragment, start, end, origin, pixelsPerPoint)
			}
		}
		return
	}
	editor := state.wordRunEditors[state.selectedWordRunID]
	if editor == nil {
		return
	}
	start, end := editor.Selection()
	if start > end {
		start, end = end, start
	}
	if start == end {
		return
	}
	for _, fragment := range page.Text {
		if fragment.RunID == state.selectedWordRunID {
			drawWordPageFragmentSelection(gtx, fragment, start, end, origin, pixelsPerPoint)
		}
	}
}

func drawWordPageFragmentSelection(gtx layout.Context, fragment wordlayout.TextFragment, start, end int, origin image.Point, pixelsPerPoint float64) {
	if start >= end {
		return
	}
	ink := wordPageSelectionColor
	top := origin.Y + int(math.Round((fragment.BaselineYPT-fragment.Style.FontSize*0.88)*pixelsPerPoint))
	height := max(1, int(math.Ceil(fragment.Style.FontSize*1.18*pixelsPerPoint)))
	for _, glyph := range fragment.Glyphs {
		if glyph.RuneEnd <= start || glyph.RuneStart >= end {
			continue
		}
		left := origin.X + int(math.Round(glyph.XPT*pixelsPerPoint))
		right := origin.X + int(math.Ceil((glyph.XPT+glyph.AdvanceXPT)*pixelsPerPoint))
		if right <= left {
			right = left + 1
		}
		paint.FillShape(gtx.Ops, ink, clip.Rect{Min: image.Pt(left, top), Max: image.Pt(right, top+height)}.Op())
	}
}

func drawWordPageCaret(gtx layout.Context, state *documentSessionState, result *wordlayout.Result, pageIndex int, origin image.Point, pixelsPerPoint float64) {
	if state.selectedWordRunID == "" || result == nil || pageIndex < 0 || pageIndex >= len(result.Pages) {
		return
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		return
	}
	editor := state.wordRunEditors[state.selectedWordRunID]
	if editor == nil || !gtx.Focused(editor) {
		return
	}
	start, end := editor.Selection()
	if start != end {
		return
	}
	position, ok := result.Locations[wordlayout.SourcePosition{RunID: state.selectedWordRunID, RuneOffset: end}]
	if !ok || position.PageIndex != pageIndex {
		return
	}
	fontSize := 11.0
	for _, fragment := range result.Pages[pageIndex].Text {
		if fragment.RunID == state.selectedWordRunID {
			fontSize = math.Max(1, fragment.Style.FontSize)
			break
		}
	}
	x := origin.X + int(math.Round(position.XPT*pixelsPerPoint))
	top := origin.Y + int(math.Round((position.YPT-fontSize*0.86)*pixelsPerPoint))
	bottom := origin.Y + int(math.Round((position.YPT+fontSize*0.22)*pixelsPerPoint))
	paint.FillShape(gtx.Ops, wordPageCaretColor, clip.Rect{Min: image.Pt(x, top), Max: image.Pt(x+max(1, int(math.Ceil(pixelsPerPoint))), max(top+1, bottom))}.Op())
}

var (
	wordPageSelectionColor = color.NRGBA{R: 76, G: 143, B: 222, A: 96}
	wordPageCaretColor     = color.NRGBA{R: 32, G: 104, B: 195, A: 255}
)
