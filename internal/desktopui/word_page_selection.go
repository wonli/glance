package desktopui

import (
	"glance/internal/document"
	"glance/internal/wordlayout"
	"image"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
)

func (a *App) focusWordPageRun(gtx layout.Context, state *documentSessionState, runID string, start, end int) {
	if !requestWordRunFocus(state, runID, start, end) {
		return
	}
	gtx.Execute(op.InvalidateCmd{})
}

func requestWordRunFocus(state *documentSessionState, runID string, start, end int) bool {
	run, paragraph, ok := findEditableWordRunInState(state, runID)
	if !ok || !run.Editable {
		return false
	}
	state.wordPageTextSelection = nil
	state.wordPageCaretBlinkStart = time.Time{}
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
	if state.wordFormatPainterActive && runID != state.wordFormatPainterSourceRunID {
		state.wordFormatPainterPendingParagraphID = paragraphID
		state.wordFormatPainterPendingRunID = runID
	}
	if state.wordToolbarTab == wordTabPicture {
		state.wordToolbarTab = wordTabHome
	}
	return true
}

func (a *App) focusLastEditableWordRun(gtx layout.Context, state *documentSessionState) bool {
	if state == nil || state.word == nil || state.wordReadingMode || state.wordProtected || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	runs := wordNavigationRuns(wordBlocksForState(state))
	if len(runs) == 0 {
		return false
	}
	target := runs[len(runs)-1]
	editor := wordRunEditor(state, target.run)
	if editor == nil || !requestWordRunFocus(state, target.run.ID, editor.Len(), editor.Len()) {
		return false
	}
	state.wordPreferredVerticalXSet = false
	state.wordPagePreferredVerticalXSet = false
	if state.wordViewMode == wordViewPrintLayout {
		requestWordPageScrollForKeyboardCaret(state, target.run.ID, editor.Len())
	} else {
		state.wordListState.List.ScrollTo(target.blockIndex)
	}
	gtx.Execute(op.InvalidateCmd{})
	return true
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
	return &wordPageTextSelection{scope: "range", anchor: anchor, focus: anchor}
}

func populateWordPageRunOrder(selection *wordPageTextSelection, result *wordlayout.Result) {
	if selection == nil || result == nil {
		return
	}
	if selection.runOrder == nil {
		selection.runOrder = make(map[string]int)
	}
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

func ensureWordPageRunOrder(state *documentSessionState, selection *wordPageTextSelection) {
	if selection == nil || state == nil {
		return
	}
	if selection.runOrder == nil {
		selection.runOrder = make(map[string]int)
	}
	if state.wordLayoutResult != nil {
		populateWordPageRunOrder(selection, state.wordLayoutResult)
	}
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable || inline.Text.ID == "" {
				continue
			}
			if _, exists := selection.runOrder[inline.Text.ID]; !exists {
				selection.runOrder[inline.Text.ID] = len(selection.runOrder)
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
	if anchor.runID == focus.runID {
		if anchor.runeOffset <= focus.runeOffset {
			return anchor, focus, true
		}
		return focus, anchor, true
	}
	anchorOrder, anchorOK := selection.runOrder[anchor.runID]
	focusOrder, focusOK := selection.runOrder[focus.runID]
	if anchorOK && focusOK {
		if anchorOrder <= focusOrder {
			return anchor, focus, true
		}
		return focus, anchor, true
	}
	return anchor, focus, true
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
	if selection == nil {
		return "", false
	}
	ensureWordPageRunOrder(state, selection)
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
