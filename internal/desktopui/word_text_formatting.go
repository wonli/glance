package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
	"math"
	"strconv"
)

func (a *App) toggleWordTextStyle(property string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if start, end, selected := wordTextFormattingSelection(state); selected {
		if _, ok := any(state.word).(document.WordTextSelectionFormatter); !ok {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.formatting_selected_text_is_not_da448e"}, feedbackWarning, notificationToast)
			return
		}
		run, ok := firstSelectedWordTextRun(state, start, end)
		if !ok || !run.Editable {
			return
		}
		patch := document.WordTextFormatPatch{}
		switch property {
		case "bold":
			value := !run.Style.Bold
			patch.Bold = &value
		case "italic":
			value := !run.Style.Italic
			patch.Italic = &value
		case "underline":
			value := !run.Style.Underline
			patch.Underline = &value
		default:
			return
		}
		a.queueWordSelectionStyle(state, start, end, patch)
		return
	}
	run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
	if !ok {
		return
	}
	runID := run.ID
	patch := document.WordTextFormatPatch{}
	switch property {
	case "bold":
		value := !run.Style.Bold
		patch.Bold = &value
	case "italic":
		value := !run.Style.Italic
		patch.Italic = &value
	case "underline":
		value := !run.Style.Underline
		patch.Underline = &value
	default:
		return
	}
	a.runWordCommand("文字格式", func(editor document.WordTextEditor) error { return editor.SetTextRunStyle(runID, patch) })
}

// wordTextFormattingSelection unifies the page-scene range with the native
// Gio editor range used in Web Layout. The latter remains on the editor after
// clicking a ribbon button, even though it is not represented by
// wordPageTextSelection.
func wordTextFormattingSelection(state *documentSessionState) (wordPageCaretPosition, wordPageCaretPosition, bool) {
	if state == nil {
		return wordPageCaretPosition{}, wordPageCaretPosition{}, false
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		ensureWordPageRunOrder(state, state.wordPageTextSelection)
		return wordPageOrderedSelection(state.wordPageTextSelection)
	}
	runID := state.selectedWordRunID
	if runID == "" {
		return wordPageCaretPosition{}, wordPageCaretPosition{}, false
	}
	editor := state.wordRunEditors[runID]
	if editor == nil {
		return wordPageCaretPosition{}, wordPageCaretPosition{}, false
	}
	start, end := editor.Selection()
	if start == end {
		return wordPageCaretPosition{}, wordPageCaretPosition{}, false
	}
	if start > end {
		start, end = end, start
	}
	return wordPageCaretPosition{runID: runID, runeOffset: start}, wordPageCaretPosition{runID: runID, runeOffset: end}, true
}

func (a *App) queueWordSelectionStyle(state *documentSessionState, start, end wordPageCaretPosition, patch document.WordTextFormatPatch) bool {
	if state == nil || state.word == nil || state.wordSelectionEditIntent != nil || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil {
		return false
	}
	if _, ok := any(state.word).(document.WordTextSelectionFormatter); !ok {
		return false
	}
	if draft := state.wordSelectionInputDraft; draft != nil {
		if editor := state.wordRunEditors[draft.runID]; editor != nil {
			if editorHasActiveComposition(editor) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_fb60d3"}, feedbackWarning, notificationToast)
				return false
			}
			if editor.Text() != draft.text {
				a.finishWordSelectionInput(state, draft.runID, editor)
				return false
			}
		}
		state.wordSelectionInputDraft = nil
	}
	patchCopy := patch
	state.wordSelectionEditIntent = &wordSelectionEditIntent{start: start, end: end, formatPatch: &patchCopy}
	for id, editor := range state.wordRunEditors {
		if editorHasActiveComposition(editor) {
			state.wordSelectionEditIntent = nil
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_fb60d3"}, feedbackWarning, notificationToast)
			return false
		}
		captureWordEditorDraft(state, id, editor.Text())
	}
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommitFor(state)
	}
	return true
}

func firstSelectedWordTextRun(state *documentSessionState, start, end wordPageCaretPosition) (document.WordTextRun, bool) {
	if state == nil {
		return document.WordTextRun{}, false
	}
	if start.runID == end.runID {
		if start.runeOffset < 0 || end.runeOffset <= start.runeOffset {
			return document.WordTextRun{}, false
		}
		run, ok := selectedWordFormattingRun(state, start.runID)
		if !ok {
			return document.WordTextRun{}, false
		}
		length := len([]rune(run.Text))
		if editor := state.wordRunEditors[run.ID]; editor != nil {
			length = editor.Len()
		}
		if end.runeOffset > length {
			return document.WordTextRun{}, false
		}
		return run, true
	}
	if state.wordPageTextSelection != nil {
		ensureWordPageRunOrder(state, state.wordPageTextSelection)
	}
	runOrder := map[string]int(nil)
	if state.wordPageTextSelection != nil {
		runOrder = state.wordPageTextSelection.runOrder
	}
	if runOrder == nil {
		runOrder = make(map[string]int)
		for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
			for _, inline := range paragraph.Inlines {
				if inline.Text != nil && inline.Text.Editable && inline.Text.ID != "" {
					if _, exists := runOrder[inline.Text.ID]; !exists {
						runOrder[inline.Text.ID] = len(runOrder)
					}
				}
			}
		}
	}
	startOrder, startOK := runOrder[start.runID]
	endOrder, endOK := runOrder[end.runID]
	if !startOK || !endOK {
		return document.WordTextRun{}, false
	}
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			order, exists := runOrder[inline.Text.ID]
			if !exists || order < startOrder || order > endOrder {
				continue
			}
			from, to := 0, len([]rune(inline.Text.Text))
			if inline.Text.ID == start.runID {
				from = start.runeOffset
			}
			if inline.Text.ID == end.runID {
				to = end.runeOffset
			}
			if from >= to {
				continue
			}
			run, _ := wordRunWithResolvedStyle(state, *inline.Text)
			return run, true
		}
	}
	return document.WordTextRun{}, false
}

func (a *App) setWordFontColor(runID, value string) {
	state := a.documentSessionState
	if state != nil && wordPageTextSelectionActive(state.wordPageTextSelection) {
		start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
		if ok {
			patch := document.WordTextFormatPatch{FontColor: &value}
			a.queueWordSelectionStyle(state, start, end, patch)
			return
		}
	}
	a.runWordCommand("文字颜色", func(editor document.WordTextEditor) error {
		return editor.SetTextRunStyle(runID, document.WordTextFormatPatch{FontColor: &value})
	})
}

func (a *App) chooseWordHighlightColor(index int) {
	state := a.documentSessionState
	if state == nil || state.word == nil || index < 0 || index >= len(wordHighlightColors) {
		return
	}
	runID := state.selectedWordRunID
	run, ok := selectedWordRun(wordBlocksForState(state), runID)
	if !ok || !run.Editable {
		state.wordToolbarHighlightPicker = false
		return
	}
	value := wordHighlightColors[index].value
	state.wordToolbarHighlightPicker = false
	a.setWordTextHighlight(runID, value)
}

func (a *App) setWordTextHighlight(runID, value string) {
	state := a.documentSessionState
	if state != nil && wordPageTextSelectionActive(state.wordPageTextSelection) {
		start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
		if ok {
			patch := document.WordTextFormatPatch{Highlight: &value}
			a.queueWordSelectionStyle(state, start, end, patch)
			return
		}
	}
	a.runWordCommand("文字高亮", func(editor document.WordTextEditor) error {
		return editor.SetTextRunStyle(runID, document.WordTextFormatPatch{Highlight: &value})
	})
}

func effectiveWordFontSize(points float64) float64 {
	if points <= 0 || math.IsNaN(points) || math.IsInf(points, 0) {
		return wordDefaultFontSizePt
	}
	return points
}

func wordFontSizeDisplayName(points float64) string {
	switch {
	case math.Abs(points-42) < 0.1:
		return "初号"
	case math.Abs(points-36) < 0.1:
		return "小初"
	case math.Abs(points-26) < 0.1:
		return "一号"
	case math.Abs(points-24) < 0.1:
		return "小一"
	case math.Abs(points-22) < 0.1:
		return "二号"
	case math.Abs(points-18) < 0.1:
		return "小二"
	case math.Abs(points-16) < 0.1:
		return "三号"
	case math.Abs(points-15) < 0.1:
		return "小三"
	case math.Abs(points-14) < 0.1:
		return "四号"
	case math.Abs(points-12) < 0.1:
		return "小四"
	case math.Abs(points-10.5) < 0.1:
		return "五号"
	case math.Abs(points-9) < 0.1:
		return "小五"
	case math.Abs(points-7.5) < 0.1:
		return "六号"
	case math.Abs(points-6.5) < 0.1:
		return "小六"
	case math.Abs(points-5.5) < 0.1:
		return "七号"
	case math.Abs(points-5) < 0.1:
		return "八号"
	default:
		if points <= 0 {
			return "五号"
		}
		return strconv.FormatFloat(points, 'f', -1, 64)
	}
}

func selectedWordFontSize(state *documentSessionState) float64 {
	if state == nil {
		return wordDefaultFontSizePt
	}
	runID := state.selectedWordRunID
	if runID == "" && wordPageTextSelectionActive(state.wordPageTextSelection) {
		start, _, ok := wordPageOrderedSelection(state.wordPageTextSelection)
		if ok {
			runID = start.runID
		}
	}
	if run, ok := selectedWordFormattingRun(state, runID); ok && run.Style.FontSize > 0 {
		return run.Style.FontSize
	}
	return wordDefaultFontSizePt
}

func selectedWordFontSizeDisplay(state *documentSessionState) string {
	if state == nil {
		return "五号"
	}
	size := selectedWordFontSize(state)
	return wordFontSizeDisplayName(size)
}

func (a *App) localizedSelectedWordFontSizeDisplay(state *documentSessionState) string {
	size := selectedWordFontSize(state)
	if a.localeSnapshot().Resolved == i18n.LocaleChinese {
		return wordFontSizeDisplayName(size)
	}
	return strconv.FormatFloat(size, 'f', -1, 64)
}

func (a *App) setWordFontSize(points float64) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	points = math.Max(wordMinFontSizePt, math.Min(wordMaxFontSizePt, points))
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
		if ok {
			patch := document.WordTextFormatPatch{FontSize: &points}
			a.queueWordSelectionStyle(state, start, end, patch)
			return
		}
	}
	run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontSizeEditor); !ok {
		return
	}
	runID := run.ID
	a.runWordCommand("字号", func(editor document.WordTextEditor) error {
		fontSizeEditor, ok := any(editor).(document.WordTextFontSizeEditor)
		if !ok {
			return fmt.Errorf("DOCX adapter does not support font-size editing")
		}
		return fontSizeEditor.SetTextRunFontSize(runID, points)
	})
}

func (a *App) adjustWordFontSize(delta float64) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
		if ok {
			current := wordDefaultFontSizePt
			if run, found := selectedWordFormattingRun(state, start.runID); found {
				current = effectiveWordFontSize(run.Style.FontSize)
			}
			next := math.Max(wordMinFontSizePt, math.Min(wordMaxFontSizePt, current+delta))
			patch := document.WordTextFormatPatch{FontSize: &next}
			a.queueWordSelectionStyle(state, start, end, patch)
			return
		}
	}
	run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontSizeEditor); !ok {
		return
	}
	next := math.Max(wordMinFontSizePt, math.Min(wordMaxFontSizePt, effectiveWordFontSize(run.Style.FontSize)+delta))
	runID := run.ID
	a.runWordCommand("字号", func(editor document.WordTextEditor) error {
		fontSizeEditor, ok := any(editor).(document.WordTextFontSizeEditor)
		if !ok {
			return fmt.Errorf("DOCX adapter does not support font-size editing")
		}
		return fontSizeEditor.SetTextRunFontSize(runID, next)
	})
}

func (a *App) showWordFontFamilyDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontFamilyEditor); !ok {
		return
	}
	a.openFontPicker(fontPickerTarget{state: state, runID: run.ID, revision: state.wordRevisionAtomic.Load(), current: run.Style.FontFamily, word: true})
}

func (a *App) toggleWordFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if state.wordFormatPainterActive {
		state.wordFormatPainterActive = false
		state.wordFormatPainterSourceRunID = ""
		clearWordFormatPainterTarget(state)
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.format_painter_canceled_cab564"}, feedbackInfo, notificationToast)
		return
	}
	run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordFormatPainter); !ok {
		return
	}
	state.wordFormatPainterSourceRunID = run.ID
	state.wordFormatPainterActive = true
	clearWordFormatPainterTarget(state)
	a.setStatusMessage(i18n.Message{ID: "word.status.inline.format_painter_is_active_click_987771"}, feedbackSuccess, notificationToast)
}

func (a *App) queueWordFormatPainterTarget(paragraphID, runID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || !state.wordFormatPainterActive {
		return
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, runID)
	paragraph, paragraphOK := selectedWordParagraph(blocks, paragraphID)
	if !ok || !run.Editable || !paragraphOK || !wordParagraphContainsRun(paragraph, runID) {
		return
	}
	startRunID, startOffset, endRunID, endOffset, selectedText, hasSelection := wordFormatPainterTargetSelection(state, runID)
	if !hasSelection && runID == state.wordFormatPainterSourceRunID {
		return
	}
	if hasSelection {
		if _, ok := any(state.word).(document.WordTextSelectionFormatPainter); !ok {
			return
		}
	} else if _, ok := any(state.word).(document.WordFormatPainter); !ok {
		return
	}
	state.wordFormatPainterActive = false
	state.selectedWordParagraphID = paragraphID
	state.selectedWordRunID = runID
	state.wordFormatPainterTargetParagraphID = paragraphID
	state.wordFormatPainterTargetRunID = runID
	state.wordFormatPainterTargetSelection = hasSelection
	state.wordFormatPainterStartRunID = startRunID
	state.wordFormatPainterStartOffset = startOffset
	state.wordFormatPainterEndRunID = endRunID
	state.wordFormatPainterEndOffset = endOffset
	state.wordFormatPainterSelectionText = selectedText
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		state.wordDeferredToolbarAction = "apply-format-painter"
		return
	}
	a.startWordFormatPainter()
}

func (a *App) processWordFormatPainterTargetIntent() {
	state := a.documentSessionState
	if state == nil || state.wordFormatPainterPendingRunID == "" {
		return
	}
	paragraphID, runID := state.wordFormatPainterPendingParagraphID, state.wordFormatPainterPendingRunID
	if state.wordPageSelectionDrag != nil {
		return
	}
	state.wordFormatPainterPendingParagraphID = ""
	state.wordFormatPainterPendingRunID = ""
	a.queueWordFormatPainterTarget(paragraphID, runID)
}

func clearWordFormatPainterTarget(state *documentSessionState) {
	if state == nil {
		return
	}
	state.wordFormatPainterTargetRunID = ""
	state.wordFormatPainterTargetParagraphID = ""
	state.wordFormatPainterTargetSelection = false
	state.wordFormatPainterStartRunID = ""
	state.wordFormatPainterStartOffset = 0
	state.wordFormatPainterEndRunID = ""
	state.wordFormatPainterEndOffset = 0
	state.wordFormatPainterSelectionText = ""
}

func wordFormatPainterTargetSelection(state *documentSessionState, focusedRunID string) (startRunID string, startOffset int, endRunID string, endOffset int, selectedText string, ok bool) {
	if state == nil || focusedRunID == "" {
		return "", 0, "", 0, "", false
	}
	selection := state.wordPageTextSelection
	if start, end, active := wordPageOrderedSelection(selection); active && wordPageSelectionContainsRun(selection, focusedRunID) {
		selectedText, ok = wordPageSelectedText(state)
		if ok && selectedText != "" {
			return start.runID, start.runeOffset, end.runID, end.runeOffset, selectedText, true
		}
	}
	editor := state.wordRunEditors[focusedRunID]
	if editor == nil {
		return "", 0, "", 0, "", false
	}
	startOffset, endOffset = editor.Selection()
	if endOffset < startOffset {
		startOffset, endOffset = endOffset, startOffset
	}
	if startOffset < 0 || endOffset <= startOffset || endOffset > len([]rune(editor.Text())) {
		return "", 0, "", 0, "", false
	}
	selectedText = string([]rune(editor.Text())[startOffset:endOffset])
	if selectedText == "" {
		return "", 0, "", 0, "", false
	}
	return focusedRunID, startOffset, focusedRunID, endOffset, selectedText, true
}

func wordPageSelectionContainsRun(selection *wordPageTextSelection, runID string) bool {
	start, end, active := wordPageOrderedSelection(selection)
	if !active {
		return false
	}
	if start.runID == end.runID {
		return runID == start.runID
	}
	runOrder, exists := selection.runOrder[runID]
	startOrder, startExists := selection.runOrder[start.runID]
	endOrder, endExists := selection.runOrder[end.runID]
	return exists && startExists && endExists && runOrder >= startOrder && runOrder <= endOrder
}

func wordParagraphContainsRun(paragraph document.WordParagraph, runID string) bool {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.ID == runID {
			return true
		}
	}
	return false
}

func (a *App) startWordFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || state.wordFormatPainterSourceRunID == "" || state.wordFormatPainterTargetRunID == "" {
		return
	}
	sourceRunID := state.wordFormatPainterSourceRunID
	targetRunID := state.wordFormatPainterTargetRunID
	targetRange := state.wordFormatPainterTargetSelection
	startRunID, startOffset := state.wordFormatPainterStartRunID, state.wordFormatPainterStartOffset
	endRunID, endOffset := state.wordFormatPainterEndRunID, state.wordFormatPainterEndOffset
	selectionText := state.wordFormatPainterSelectionText
	var apply func() error
	if targetRange {
		if !wordFormatPainterSelectionStillMatches(state, startRunID, startOffset, endRunID, endOffset, selectionText) {
			state.wordFormatPainterSourceRunID = ""
			clearWordFormatPainterTarget(state)
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_target_text_changed_select_8a6a5c"}, feedbackWarning, notificationToast)
			return
		}
		if selectedPainter, ok := any(state.word).(document.WordTextSelectionFormatPainter); ok {
			apply = func() error {
				_, err := selectedPainter.ApplyWordFormattingToTextSelection(sourceRunID, startRunID, startOffset, endRunID, endOffset)
				return err
			}
		}
	} else if selectedPainter, ok := any(state.word).(document.WordFormatPainter); ok {
		apply = func() error {
			_, err := selectedPainter.ApplyWordFormatting(sourceRunID, targetRunID)
			return err
		}
	}
	if apply == nil {
		state.wordFormatPainterSourceRunID = ""
		clearWordFormatPainterTarget(state)
		return
	}
	state.wordFormatPainterSourceRunID = ""
	clearWordFormatPainterTarget(state)
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	a.setStatusMessage(i18n.Message{ID: "word.status.inline.applying_format_painter_cb3668"}, feedbackInfo, notificationToast)
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, apply)
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "格式刷", err: err, blocks: blocks})
	})
}

func wordFormatPainterSelectionStillMatches(state *documentSessionState, startRunID string, startOffset int, endRunID string, endOffset int, expected string) bool {
	if selection := state.wordPageTextSelection; wordPageTextSelectionActive(selection) {
		start, end, active := wordPageOrderedSelection(selection)
		if !active || start.runID != startRunID || start.runeOffset != startOffset || end.runID != endRunID || end.runeOffset != endOffset {
			return false
		}
		selected, ok := wordPageSelectedText(state)
		return ok && selected == expected
	}
	if startRunID != endRunID {
		return false
	}
	editor := state.wordRunEditors[startRunID]
	if editor == nil {
		return false
	}
	selectionStart, selectionEnd := editor.Selection()
	if selectionStart > selectionEnd {
		selectionStart, selectionEnd = selectionEnd, selectionStart
	}
	if selectionStart != startOffset || selectionEnd != endOffset {
		return false
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, startRunID)
	if !ok || startOffset < 0 || endOffset < startOffset || endOffset > len([]rune(run.Text)) {
		return false
	}
	return string([]rune(run.Text)[startOffset:endOffset]) == expected
}

func (a *App) setWordAlignment(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraphIDs := wordParagraphIDsForSelection(state)
	if len(paragraphIDs) == 0 {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.place_the_caret_in_an_75475b"}, feedbackWarning, notificationToast)
		return
	}
	alignment := map[string]document.WordAlignment{"align-left": document.WordAlignLeft, "align-center": document.WordAlignCenter, "align-right": document.WordAlignRight, "align-justify": document.WordAlignJustified}[actionID]
	if alignment == "" {
		return
	}
	if wordHasPendingTextDrafts(state) {
		if state.wordDeferredToolbarAction == "" {
			state.wordDeferredToolbarAction = actionID
		}
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.startNextWordTextCommitFor(state)
		return
	}
	paragraphIDs = append([]string(nil), paragraphIDs...)
	a.runWordCommand("段落对齐", func(editor document.WordTextEditor) error {
		if bulk, ok := any(editor).(document.WordParagraphAlignmentEditor); ok {
			return bulk.SetParagraphAlignments(paragraphIDs, alignment)
		}
		if len(paragraphIDs) != 1 {
			return fmt.Errorf("当前 DOCX 适配器不支持批量段落对齐")
		}
		return editor.SetParagraphAlignment(paragraphIDs[0], alignment)
	})
}

func (a *App) setWordList(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	if !ok {
		return
	}
	kind := document.WordListBullet
	if actionID == "numbered-list" {
		kind = document.WordListNumbered
	}
	if paragraph.ListKind == kind {
		kind = document.WordListNone
	}
	paragraphID := paragraph.ID
	a.runWordListCommand("段落列表", func(editor document.WordParagraphListEditor) error {
		return editor.SetParagraphList(paragraphID, kind)
	})
}
