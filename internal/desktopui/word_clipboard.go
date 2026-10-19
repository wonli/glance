package desktopui

import (
	"glance/internal/i18n"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
)

type wordClipboardSelectionTarget struct {
	start      wordPageCaretPosition
	end        wordPageCaretPosition
	localRunID string
	localText  string
	localStart int
	localEnd   int
}

type wordPendingSelectionPaste struct {
	target wordClipboardSelectionTarget
	text   string
}

func (a *App) requestWordClipboardRead(gtx layout.Context, state *documentSessionState) {
	if state == nil {
		return
	}
	if state.wordClipboardRunID != "" || state.wordClipboardSelection != nil {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.a_paste_is_already_reading_403883"}, feedbackSuccess, notificationSuppressed)
		return
	}
	state.wordClipboardRunID = state.selectedWordRunID
	state.wordClipboardSelection = nil
	if start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection); ok {
		state.wordClipboardSelection = &wordClipboardSelectionTarget{start: start, end: end}
	} else {
		runID := state.wordFocusedRunID
		run, ok := selectedWordRun(wordBlocksForState(state), runID)
		if !ok || !run.Editable {
			runID = state.selectedWordRunID
			run, ok = selectedWordRun(wordBlocksForState(state), runID)
		}
		if ok && run.Editable {
			editor := wordRunEditor(state, run)
			start, end := editor.Selection()
			state.wordClipboardRunID = runID
			state.wordClipboardSelection = &wordClipboardSelectionTarget{
				localRunID: runID, localText: editor.Text(), localStart: start, localEnd: end,
			}
		}
	}
	if state.wordClipboardSelection == nil {
		state.wordClipboardRunID = ""
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.place_the_caret_in_the_f286e3"}, feedbackWarning, notificationToast)
		return
	}
	gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
}

func (a *App) pasteWordLocalEditorSelection(gtx layout.Context, state *documentSessionState, target wordClipboardSelectionTarget, text string) {
	if state == nil || state.wordReadingMode || state.formatRunning.Load() || a.isSaving() {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_word_document_is_e4d6b3"}, feedbackInfo, notificationSuppressed)
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), target.localRunID)
	if !ok || !run.Editable {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_word_paste_target_expired_1a3179"}, feedbackWarning, notificationToast)
		return
	}
	editor := wordRunEditor(state, run)
	if editorHasActiveComposition(editor) {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_e3b0d1"}, feedbackWarning, notificationToast)
		return
	}
	current := []rune(editor.Text())
	if editor.Text() != target.localText {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_body_changed_before_clipboard_61c34e"}, feedbackWarning, notificationToast)
		return
	}
	start, end := target.localStart, target.localEnd
	if start < 0 || end < start || end > len(current) {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_word_paste_caret_expired_16cf74"}, feedbackWarning, notificationToast)
		return
	}
	editor.SetCaret(start, end)
	inserted := editor.Insert(text)
	value := editor.Text()
	caret := start + inserted
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = wordParagraphIDForRun(wordBlocksForState(state), run.ID)
	state.selectedWordImageID = ""
	state.wordFocusedRunID = run.ID
	state.wordLayoutFocusedRunID = run.ID
	state.wordPendingCaretRunID = run.ID
	state.wordPendingCaretOffset = caret
	state.wordPendingCaretByX = false
	state.wordPendingFocusRunID = run.ID
	captureWordTextDraft(state, run.ID, value)
	gtx.Execute(key.FocusCmd{Tag: editor})
	a.setStatusMessage(i18n.Message{ID: "word.status.inline.editing_word_document_text_556c9c"}, feedbackInfo, notificationSuppressed)
}

func (a *App) cutSelectedWordText(gtx layout.Context, state *documentSessionState) bool {
	if state == nil || !wordPageTextSelectionActive(state.wordPageTextSelection) {
		return false
	}
	text, selected := wordPageSelectedText(state)
	start, end, ordered := wordPageOrderedSelection(state.wordPageTextSelection)
	if !selected || !ordered {
		return true
	}
	if !a.queueWordSelectionDeletion(state, start, end) {
		return true
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	return true
}

func (a *App) handleWordClipboardEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &state.wordClipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		data, ok := ev.(transfer.DataEvent)
		if !ok {
			continue
		}
		reader := data.Open()
		value, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			state.wordClipboardRunID = ""
			state.wordClipboardSelection = nil
			a.setWordCommandStatus(err)
			continue
		}
		state.wordClipboardRunID = ""
		selection := state.wordClipboardSelection
		state.wordClipboardSelection = nil
		if selection == nil {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_word_paste_target_expired_1a3179"}, feedbackWarning, notificationToast)
			continue
		}
		if selection.localRunID != "" {
			a.pasteWordLocalEditorSelection(gtx, state, *selection, string(value))
			continue
		}
		if state.wordPendingPasteSelection != nil {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.a_word_paste_is_already_57b9fb"}, feedbackSuccess, notificationSuppressed)
			continue
		}
		state.wordPendingPasteSelection = &wordPendingSelectionPaste{target: *selection, text: string(value)}
		if draft := state.wordSelectionInputDraft; draft != nil {
			if editor := state.wordRunEditors[draft.runID]; editor != nil {
				if editorHasActiveComposition(editor) {
					state.wordPendingPasteSelection = nil
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_297909"}, feedbackWarning, notificationToast)
					continue
				}
				if editor.Text() != draft.text {
					// Commit the user's already typed replacement first. The pending
					// paste will be rebased to the resulting caret on completion.
					a.finishWordSelectionInput(state, draft.runID, editor)
					continue
				}
				state.wordSelectionInputDraft = nil
			}
		}
		composing := false
		for _, editor := range state.wordRunEditors {
			if editorHasActiveComposition(editor) {
				composing = true
				break
			}
		}
		if composing {
			state.wordPendingPasteSelection = nil
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_chinese_input_composition_before_297909"}, feedbackWarning, notificationToast)
			continue
		}
		if wordSelectionPasteTargetHasPendingDraft(state, *selection) {
			state.wordPendingPasteSelection = nil
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_selection_has_uncommitted_text_33ffa5"}, feedbackWarning, notificationSuppressed)
			continue
		}
		for id, editor := range state.wordRunEditors {
			captureWordEditorDraft(state, id, editor.Text())
		}
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		if !state.formatRunning.Load() {
			a.startNextWordTextCommitFor(state)
		}
		continue
	}
}
