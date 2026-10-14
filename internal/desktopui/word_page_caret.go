package desktopui

import (
	"glance/internal/wordlayout"
	"image"
	"io"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

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
		switch event.(type) {
		case widget.SelectEvent:
			start, end := editor.Selection()
			logWordDebug("page_selection_change revision=%d run=%q selection=%d:%d length=%d focused=%v",
				state.wordRevisionAtomic.Load(), runID, start, end, editor.Len(), gtx.Focused(editor))
		case widget.SubmitEvent:
			start, end := editor.Selection()
			logWordDebug("page_submit_event revision=%d run=%q selection=%d:%d length=%d selected_run=%q layout_revision=%d",
				state.wordRevisionAtomic.Load(), runID, start, end, editor.Len(), state.selectedWordRunID, wordDebugLayoutRevision(state))
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

func (a *App) handleWordPageArrowNavigation(gtx layout.Context, state *documentSessionState, editor *widget.Editor) (handled bool) {
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
	beforeRun := state.selectedWordRunID
	beforeStart, beforeEnd, beforeLength := wordDebugEditorSelection(editor)
	defer func() {
		afterEditor := state.wordRunEditors[state.selectedWordRunID]
		afterStart, afterEnd, afterLength := wordDebugEditorSelection(afterEditor)
		logWordDebug("page_key_navigation revision=%d layout_revision=%d key=%q handled=%v before_run=%q before_selection=%d:%d before_length=%d after_run=%q after_selection=%d:%d after_length=%d preferred_x=%.2f preferred_x_set=%v",
			state.wordRevisionAtomic.Load(), wordDebugLayoutRevision(state), keyEvent.Name, handled,
			beforeRun, beforeStart, beforeEnd, beforeLength, state.selectedWordRunID, afterStart, afterEnd, afterLength,
			state.wordPagePreferredVerticalX, state.wordPagePreferredVerticalXSet)
	}()
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
