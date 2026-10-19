package desktopui

import (
	"errors"
	"glance/internal/i18n"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
)

type wordSelectionEditIntent struct {
	start, end      wordPageCaretPosition
	replacement     string
	replaces        bool
	formatPatch     *document.WordTextFormatPatch
	inputRunID      string
	inputText       string
	inputCaretStart int
	inputCaretEnd   int
}

type wordSelectionInputDraft struct {
	runID      string
	text       string
	caretStart int
	caretEnd   int
	start      wordPageCaretPosition
	end        wordPageCaretPosition
}

func beginWordSelectionInputDraft(state *documentSessionState, runID string, editor *widget.Editor, focused bool) bool {
	if state == nil || editor == nil || !focused || !wordPageTextSelectionActive(state.wordPageTextSelection) || state.wordSelectionEditIntent != nil {
		return state != nil && state.wordSelectionEditIntent != nil
	}
	if _, ok := any(state.word).(document.WordTextSelectionReplacer); !ok {
		return false
	}
	start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
	if !ok {
		return false
	}
	if existing := state.wordSelectionInputDraft; existing != nil && existing.runID == runID && existing.start == start && existing.end == end {
		return false
	}
	caretStart, caretEnd := editor.Selection()
	state.wordSelectionInputDraft = &wordSelectionInputDraft{
		runID: runID, text: editor.Text(), caretStart: caretStart, caretEnd: caretEnd,
		start: start, end: end,
	}
	return false
}

func (a *App) finishWordSelectionInput(state *documentSessionState, runID string, editor *widget.Editor) bool {
	if state == nil || editor == nil {
		return false
	}
	if state.wordSelectionEditIntent != nil {
		return true
	}
	draft := state.wordSelectionInputDraft
	if draft == nil || draft.runID != runID {
		return false
	}
	if !wordPageTextSelectionActive(state.wordPageTextSelection) {
		state.wordSelectionInputDraft = nil
		return false
	}
	if state.wordParagraphSplitIntent != nil {
		state.wordSelectionInputDraft = nil
		return false
	}
	current := editor.Text()
	if current == draft.text {
		return editorHasActiveComposition(editor)
	}
	if editorHasActiveComposition(editor) {
		return true
	}
	replacement := wordTextReplacementDiff(draft.text, current)
	if strings.Contains(replacement, "\n") {
		state.wordSelectionInputDraft = nil
		return false
	}
	if !a.queueWordSelectionReplacement(state, editor, *draft, replacement) {
		state.wordSelectionInputDraft = nil
		return false
	}
	return true
}

func wordTextReplacementDiff(before, after string) string {
	oldRunes, newRunes := []rune(before), []rune(after)
	prefix := 0
	for prefix < len(oldRunes) && prefix < len(newRunes) && oldRunes[prefix] == newRunes[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldRunes)-prefix && suffix < len(newRunes)-prefix && oldRunes[len(oldRunes)-suffix-1] == newRunes[len(newRunes)-suffix-1] {
		suffix++
	}
	return string(newRunes[prefix : len(newRunes)-suffix])
}

func (a *App) queueWordSelectionReplacement(state *documentSessionState, editor *widget.Editor, draft wordSelectionInputDraft, replacement string) bool {
	if state == nil || editor == nil {
		return false
	}
	if _, ok := any(state.word).(document.WordTextSelectionReplacer); !ok {
		return false
	}
	// Gio applies EditEvent to the focused run first. Restore that transient edit
	// so the adapter receives only the document-wide replacement intent.
	editor.SetText(draft.text)
	editor.SetCaret(draft.caretStart, draft.caretEnd)
	state.wordSelectionInputDraft = nil
	state.wordSelectionEditIntent = &wordSelectionEditIntent{
		start: draft.start, end: draft.end, replacement: replacement, replaces: true,
		inputRunID: draft.runID, inputText: draft.text, inputCaretStart: draft.caretStart, inputCaretEnd: draft.caretEnd,
	}
	editor.ReadOnly = true
	for id, current := range state.wordRunEditors {
		captureWordEditorDraft(state, id, current.Text())
	}
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommitFor(state)
	}
	return true
}

func restoreWordSelectionReplacementDraft(state *documentSessionState, intent wordSelectionEditIntent) {
	if state == nil || intent.inputRunID == "" {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), intent.inputRunID)
	if !ok {
		return
	}
	editor := wordRunEditor(state, run)
	start, end := min(intent.inputCaretStart, intent.inputCaretEnd), max(intent.inputCaretStart, intent.inputCaretEnd)
	runes := []rune(intent.inputText)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	replacement := []rune(intent.replacement)
	value := string(runes[:start]) + string(replacement) + string(runes[end:])
	editor.SetText(value)
	caret := start + len(replacement)
	editor.SetCaret(caret, caret)
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
	state.selectedWordRunID = intent.inputRunID
	state.selectedWordParagraphID = wordParagraphIDForRun(wordBlocksForState(state), intent.inputRunID)
	state.wordFocusedRunID = intent.inputRunID
	state.wordLayoutFocusedRunID = intent.inputRunID
	state.wordPendingCaretRunID = intent.inputRunID
	state.wordPendingCaretOffset = caret
	state.wordPendingCaretByX = false
	state.wordPendingFocusRunID = intent.inputRunID
	captureWordTextDraft(state, intent.inputRunID, value)
}

func (a *App) handleWordSelectionDeleteKey(gtx layout.Context, state *documentSessionState, editor *widget.Editor) bool {
	if !wordPageTextSelectionActive(state.wordPageTextSelection) {
		return false
	}
	value, ok := gtx.Event(key.Filter{Focus: editor, Name: key.NameDeleteBackward}, key.Filter{Focus: editor, Name: key.NameDeleteForward})
	if !ok {
		return false
	}
	pressed, ok := value.(key.Event)
	if !ok || pressed.State != key.Press {
		return true
	}
	if _, ok := any(state.word).(document.WordTextSelectionEditor); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_750fa0"}, feedbackWarning, notificationToast)
		return true
	}
	// A targeted key filter can pull Delete ahead of earlier edit/selection
	// events. Apply those first, as history shortcuts do, before capturing drafts.
	beforeText := editor.Text()
	beforeStart, beforeEnd := editor.Selection()
	for pending, ok := editor.Update(gtx); ok; pending, ok = editor.Update(gtx) {
		if _, submit := pending.(widget.SubmitEvent); submit {
			start, end := editor.Selection()
			a.queueWordParagraphSplit(state.selectedWordRunID, editor.Text(), start, end)
			state.wordPageTextSelection = nil
			return true
		}
	}
	afterStart, afterEnd := editor.Selection()
	if editor.Text() != beforeText || beforeStart != afterStart || beforeEnd != afterEnd {
		// The rendered cross-run endpoints predate this input. Delete using Gio's
		// current native caret/selection, including its grapheme-aware behavior.
		state.wordPageTextSelection = nil
		state.wordPageSelectionDrag = nil
		direction := 1
		if pressed.Name == key.NameDeleteBackward {
			direction = -1
		}
		editor.Delete(direction)
		for id, current := range state.wordRunEditors {
			if current == editor {
				captureWordEditorDraft(state, id, editor.Text())
				break
			}
		}
		return true
	}
	start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
	if !ok {
		return true
	}
	a.queueWordSelectionDeletion(state, start, end)
	return true
}

func (a *App) queueWordSelectionDeletion(state *documentSessionState, start, end wordPageCaretPosition) bool {
	if state == nil || state.word == nil || state.wordSelectionEditIntent != nil || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil {
		return false
	}
	if _, ok := any(state.word).(document.WordTextSelectionEditor); !ok {
		return false
	}
	if draft := state.wordSelectionInputDraft; draft != nil {
		if editor := state.wordRunEditors[draft.runID]; editor != nil {
			if editorHasActiveComposition(editor) {
				return false
			}
			if editor.Text() != draft.text {
				a.finishWordSelectionInput(state, draft.runID, editor)
				return false
			}
		}
		state.wordSelectionInputDraft = nil
	}
	for _, editor := range state.wordRunEditors {
		if editorHasActiveComposition(editor) {
			return false
		}
	}
	state.wordSelectionEditIntent = &wordSelectionEditIntent{start: start, end: end}
	// The selection can include drafts in several formatted runs. Commit those
	// before the adapter resolves the rune offsets against the document.
	for id, editor := range state.wordRunEditors {
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

func (a *App) startPendingWordSelectionPaste(state *documentSessionState) {
	if state == nil || state.word == nil || state.wordPendingPasteSelection == nil || state.formatRunning.Load() || state.wordSelectionEditIntent != nil {
		return
	}
	pending := *state.wordPendingPasteSelection
	if _, ok := any(state.word).(document.WordTextSelectionReplacer); !ok {
		state.wordPendingPasteSelection = nil
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.replacing_a_text_selection_is_1c3fcf"}, feedbackWarning, notificationToast)
		return
	}
	if draft := state.wordSelectionInputDraft; draft != nil {
		if editor := state.wordRunEditors[draft.runID]; editor != nil {
			if editorHasActiveComposition(editor) {
				state.wordPendingPasteSelection = nil
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_297909"}, feedbackWarning, notificationToast)
				return
			}
			if editor.Text() != draft.text {
				a.finishWordSelectionInput(state, draft.runID, editor)
				return
			}
		}
		state.wordSelectionInputDraft = nil
	}
	if !wordSelectionPasteTargetValid(state, pending.target) {
		state.wordPendingPasteSelection = nil
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_word_paste_selection_expired_9e3286"}, feedbackWarning, notificationToast)
		return
	}
	for _, editor := range state.wordRunEditors {
		if editorHasActiveComposition(editor) {
			state.wordPendingPasteSelection = nil
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_297909"}, feedbackWarning, notificationToast)
			return
		}
	}
	state.wordSelectionEditIntent = &wordSelectionEditIntent{
		start: pending.target.start, end: pending.target.end,
		replacement: pending.text, replaces: true,
	}
	state.wordPendingPasteSelection = nil
	for id, editor := range state.wordRunEditors {
		captureWordEditorDraft(state, id, editor.Text())
	}
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommitFor(state)
	}
}

func wordSelectionPasteTargetValid(state *documentSessionState, target wordClipboardSelectionTarget) bool {
	if state == nil || state.word == nil {
		return false
	}
	startRun, startOK := selectedWordRun(wordBlocksForState(state), target.start.runID)
	endRun, endOK := selectedWordRun(wordBlocksForState(state), target.end.runID)
	if !startOK || !endOK || target.start.runeOffset < 0 || target.end.runeOffset < 0 {
		return false
	}
	if target.start.runeOffset > len([]rune(startRun.Text)) || target.end.runeOffset > len([]rune(endRun.Text)) {
		return false
	}
	if target.start.runID == target.end.runID {
		return target.start.runeOffset <= target.end.runeOffset
	}
	order := make(map[string]int)
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				if _, exists := order[inline.Text.ID]; !exists {
					order[inline.Text.ID] = len(order)
				}
			}
		}
	}
	startOrder, startFound := order[target.start.runID]
	endOrder, endFound := order[target.end.runID]
	return startFound && endFound && startOrder < endOrder
}

func wordSelectionPasteTargetHasPendingDraft(state *documentSessionState, target wordClipboardSelectionTarget) bool {
	if state == nil {
		return false
	}
	committed, drafts := state.wordTextStateSnapshot()
	inside := false
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if inline.Text.ID == target.start.runID {
				inside = true
			}
			if inside {
				if draft, exists := drafts[inline.Text.ID]; exists && draft != committed[inline.Text.ID] {
					return true
				}
			}
			if inline.Text.ID == target.end.runID {
				return false
			}
		}
	}
	return false
}

func (a *App) startWordSelectionEdit(state *documentSessionState) {
	if state == nil || state.wordSelectionEditIntent == nil || state.formatRunning.Load() {
		return
	}
	intent := *state.wordSelectionEditIntent
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation := a.documentGeneration
	started := a.launchManagedBackground(func() {
		var edited document.WordTextSelectionEditResult
		var formatted document.WordTextSelectionFormatResult
		blocks, err := runWordMutation(state, func() error {
			if intent.formatPatch != nil {
				formatter, ok := any(state.word).(document.WordTextSelectionFormatter)
				if !ok {
					return errors.New("Word selection formatting is unsupported")
				}
				var formatErr error
				formatted, formatErr = formatter.SetTextSelectionStyle(intent.start.runID, intent.start.runeOffset, intent.end.runID, intent.end.runeOffset, *intent.formatPatch)
				return formatErr
			}
			if intent.replaces {
				editor, ok := any(state.word).(document.WordTextSelectionReplacer)
				if !ok {
					return errors.New("Word text selection replacement is unsupported")
				}
				var err error
				edited, err = editor.ReplaceTextSelection(intent.start.runID, intent.start.runeOffset, intent.end.runID, intent.end.runeOffset, intent.replacement)
				return err
			}
			editor, ok := any(state.word).(document.WordTextSelectionEditor)
			if !ok {
				return errors.New("Word text selection deletion is unsupported")
			}
			var err error
			edited, err = editor.DeleteTextSelection(intent.start.runID, intent.start.runeOffset, intent.end.runID, intent.end.runeOffset)
			return err
		})
		operation := "删除选区文字"
		if intent.replaces {
			operation = "替换选区文字"
		} else if intent.formatPatch != nil {
			operation = "格式化选中文字"
		}
		a.publishWordCommand(wordCommandResult{state: state, operationToken: token, generation: generation, operation: operation, blocks: blocks, err: err, selectionEdit: true, selectionEditResult: edited, selectionFormatResult: formatted})
	})
	if !started {
		state.finishOperation(token)
		cancelWordSelectionEdit(state)
	}
}

func (a *App) completeWordSelectionEdit(result wordCommandResult, active bool) {
	state := result.state
	if state == nil {
		return
	}
	intent := state.wordSelectionEditIntent
	if result.err == nil && intent != nil {
		rebase := func(target *wordClipboardSelectionTarget) {
			if target == nil {
				return
			}
			if intent.formatPatch != nil {
				formatted := result.selectionFormatResult
				*target = wordClipboardSelectionTarget{
					start: wordPageCaretPosition{runID: formatted.StartRunID, runeOffset: formatted.StartOffset},
					end:   wordPageCaretPosition{runID: formatted.EndRunID, runeOffset: formatted.EndOffset},
				}
				return
			}
			caret := result.selectionEditResult
			position := wordPageCaretPosition{runID: caret.RunID, runeOffset: caret.Caret}
			*target = wordClipboardSelectionTarget{start: position, end: position}
		}
		rebase(state.wordClipboardSelection)
		if pending := state.wordPendingPasteSelection; pending != nil {
			rebase(&pending.target)
		}
	} else if state.wordPendingPasteSelection != nil && result.err != nil {
		state.wordPendingPasteSelection = nil
	}
	defer func() {
		state.finishOperation(result.operationToken)
		if active && result.err == nil {
			a.finishWordTextFlush(state)
		}
	}()
	state.wordSelectionEditIntent = nil
	state.wordSelectionInputDraft = nil
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.wordForceDraftFlush.Store(false)
	if result.err != nil {
		if intent != nil && intent.replaces {
			restoreWordSelectionReplacementDraft(state, *intent)
		}
		if active {
			a.setWordCommandStatus(result.err)
		}
		return
	}
	state.setWordBlocksAndTextState(result.blocks, true)
	if intent != nil && intent.formatPatch == nil {
		if editor := state.wordRunEditors[intent.end.runID]; editor != nil {
			_, caret := editor.Selection()
			editor.SetCaret(caret, caret)
		}
	}
	state.wordRunEditors = make(map[string]*widget.Editor)
	if intent != nil && intent.formatPatch != nil {
		a.completeWordSelectionFormat(state, result.selectionFormatResult, active)
		return
	}
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
	caret := result.selectionEditResult
	run, ok := selectedWordRun(wordBlocksForState(state), caret.RunID)
	if !ok {
		return
	}
	editor := wordRunEditor(state, run)
	editor.SetCaret(caret.Caret, caret.Caret)
	state.selectedWordRunID = caret.RunID
	state.selectedWordParagraphID = caret.ParagraphID
	state.selectedWordImageID = ""
	state.wordFocusedRunID = caret.RunID
	state.wordPendingCaretRunID = caret.RunID
	state.wordPendingCaretOffset = caret.Caret
	state.wordPendingCaretByX = false
	state.wordPendingFocusRunID = ""
	if active {
		state.wordPendingFocusRunID = caret.RunID
		messageID := "word.selection.status.deleted"
		if intent != nil && intent.replaces {
			messageID = "word.selection.status.replaced"
		}
		a.setStatusMessage(i18n.Message{ID: messageID}, feedbackSuccess, notificationToast)
		a.persistActiveSession()
	}
}

func (a *App) completeWordSelectionFormat(state *documentSessionState, formatted document.WordTextSelectionFormatResult, active bool) {
	if state == nil {
		return
	}
	order := make(map[string]int)
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if _, exists := order[inline.Text.ID]; !exists {
				order[inline.Text.ID] = len(order)
			}
		}
	}
	if _, exists := order[formatted.StartRunID]; !exists {
		return
	}
	if _, exists := order[formatted.EndRunID]; !exists {
		return
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "range",
		anchor:   wordPageCaretPosition{runID: formatted.StartRunID, runeOffset: formatted.StartOffset},
		focus:    wordPageCaretPosition{runID: formatted.EndRunID, runeOffset: formatted.EndOffset},
		runOrder: order,
	}
	state.wordPageSelectionDrag = nil
	state.selectedWordRunID = formatted.EndRunID
	state.selectedWordParagraphID = wordParagraphIDForRun(wordBlocksForState(state), formatted.EndRunID)
	state.selectedWordImageID = ""
	state.wordFocusedRunID = formatted.EndRunID
	state.wordLayoutFocusedRunID = formatted.EndRunID
	state.wordPendingCaretRunID = formatted.EndRunID
	state.wordPendingCaretOffset = formatted.EndOffset
	state.wordPendingCaretByX = false
	state.wordPendingFocusRunID = ""
	if active {
		state.wordPendingFocusRunID = formatted.EndRunID
	}
	if run, ok := selectedWordRun(wordBlocksForState(state), formatted.EndRunID); ok {
		editor := wordRunEditor(state, run)
		if formatted.StartRunID == formatted.EndRunID {
			editor.SetCaret(formatted.StartOffset, formatted.EndOffset)
		} else {
			editor.SetCaret(formatted.EndOffset, formatted.EndOffset)
		}
	}
	if active {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.formatted_the_selected_text_605de5"}, feedbackSuccess, notificationToast)
		a.persistActiveSession()
	}
}

// A failed flush keeps the user's text, but must release the pending intent
// that makes its editors read-only. This also handles rejected worker dispatch.
func cancelWordSelectionEdit(state *documentSessionState) {
	if state == nil {
		return
	}
	if intent := state.wordSelectionEditIntent; intent != nil && intent.replaces {
		restoreWordSelectionReplacementDraft(state, *intent)
	}
	state.wordSelectionEditIntent = nil
	state.wordSelectionInputDraft = nil
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.wordForceDraftFlush.Store(false)
}

// Only continue the selection edit owned by this inactive view. Generic flush
// completion may dispatch actions (such as Save) against the active App view.
func (a *App) completeInactiveWordSelectionEditDraft(result wordCommandResult) {
	state := result.state
	if result.err != nil {
		cancelWordSelectionEdit(state)
		return
	}
	state.setCommittedWordTextAndClearDraft(result.runID, result.runText)
	if result.blocks != nil && result.revision == 0 {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	if wordHasPendingTextDrafts(state) {
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.startNextWordTextCommitFor(state)
		return
	}
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.wordForceDraftFlush.Store(false)
	a.startWordSelectionEdit(state)
}
