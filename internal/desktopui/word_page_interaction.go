package desktopui

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

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
				if state.selectedWordImageID != "" {
					clearWordImageSelection(state)
					state.selectedWordRunID = ""
					state.selectedWordParagraphID = ""
					state.wordPageSelectionDrag = nil
					if a.window != nil {
						a.window.Invalidate()
					}
					continue
				}
				if isWordPageBottomBlank(page, pagePoint) && !state.wordReadingMode && !state.wordProtected && !state.wordInsertionRunning && !state.formatRunning.Load() && !a.isSaving() {
					state.selectedWordRunID = ""
					state.selectedWordParagraphID = ""
					clearWordImageSelection(state)
					state.wordPageSelectionDrag = nil
					a.handleWordBottomBlankClick(state, page, pageIndex)
					continue
				}
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
