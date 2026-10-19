package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
)

type wordPageBreakIntent struct {
	runID          string
	value          string
	selectionStart int
	selectionEnd   int
	breakType      document.WordBreakType
	blankPage      bool
}

func wordBodyRunEditable(blocks []document.WordBlock, runID string) bool {
	for _, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return inline.Text.Editable
			}
		}
	}
	return false
}

func (a *App) queueWordBlankPage() {
	a.queueWordBreakInternal(document.WordBreakPage, true)
}

func (a *App) queueWordPageBreak() {
	a.queueWordBreak(document.WordBreakPage)
}

func (a *App) queueWordBreak(breakType document.WordBreakType) {
	a.queueWordBreakInternal(breakType, false)
}

func (a *App) queueWordBreakInternal(breakType document.WordBreakType, blankPage bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.wordPageBreakIntent != nil || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordPageBreakEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserting_separators_is_not_supported_66112c"}, feedbackWarning, notificationToast)
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable || !wordBodyRunEditable(wordBlocksForState(state), run.ID) {
		return
	}
	editor := wordRunEditor(state, run)
	value := editor.Text()
	start, end := editor.Selection()
	if start > end {
		start, end = end, start
	}
	if _, _, valid := wordTextAroundSelection(value, start, end); !valid {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_insert_a_separator_9ef226"}, feedbackWarning, notificationToast)
		return
	}
	state.wordPageBreakIntent = &wordPageBreakIntent{runID: run.ID, value: value, selectionStart: start, selectionEnd: end, breakType: breakType, blankPage: blankPage}
	captureWordTextDraft(state, run.ID, value)
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommit()
	}
}

func (a *App) startWordPageBreak(state *documentSessionState) {
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || state.wordPageBreakIntent == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBreakEditor)
	if !ok {
		state.wordPageBreakIntent = nil
		state.wordFlushRequested.Store(false)
		state.wordFlushAllDrafts.Store(false)
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserting_separators_is_not_supported_66112c"}, feedbackWarning, notificationToast)
		return
	}
	intent := *state.wordPageBreakIntent
	prefix, _, valid := wordTextAroundSelection(intent.value, intent.selectionStart, intent.selectionEnd)
	if !valid {
		state.wordPageBreakIntent = nil
		state.wordFlushRequested.Store(false)
		state.wordFlushAllDrafts.Store(false)
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_insert_a_separator_9ef226"}, feedbackWarning, notificationToast)
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	operation := "插入分页符"
	if intent.blankPage {
		operation = "插入空白页"
	} else if intent.breakType == document.WordBreakColumn {
		operation = "插入分栏符"
	}
	a.launchManagedBackground(func() {
		var paragraphID, caretRunID string
		blocks, err := runWordMutation(state, func() error {
			var insertErr error
			if intent.blankPage {
				paragraphID, caretRunID, insertErr = editor.InsertBlankPageAtRange(intent.runID, intent.value, intent.selectionStart, intent.selectionEnd)
			} else {
				paragraphID, caretRunID, insertErr = editor.InsertBreakAtRange(intent.runID, intent.value, intent.selectionStart, intent.selectionEnd, intent.breakType)
			}
			return insertErr
		})
		a.publishWordCommand(wordCommandResult{
			state:                 state,
			generation:            generation,
			operation:             operation,
			err:                   err,
			blocks:                blocks,
			pageBreak:             true,
			pageBreakSourcePrefix: prefix,
			pageBreakParagraphID:  paragraphID,
			pageBreakCaretRunID:   caretRunID,
		})
	})
}

func (a *App) completeWordPageBreak(result wordCommandResult, active bool) {
	state := result.state
	if state == nil {
		return
	}
	intent := state.wordPageBreakIntent
	state.wordPageBreakIntent = nil
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	if result.err != nil {
		if intent != nil {
			captureWordTextDraft(state, intent.runID, intent.value)
			if editor := state.wordRunEditors[intent.runID]; editor != nil {
				if editor.Text() != intent.value {
					editor.SetText(intent.value)
					editor.SetCaret(intent.selectionStart, intent.selectionEnd)
				}
			}
		}
		if active {
			a.setWordCommandStatus(result.err)
		}
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	if intent != nil {
		if result.pageBreakSourcePrefix == "" {
			delete(state.wordRunEditors, intent.runID)
			state.removeWordTextState(intent.runID)
		} else {
			if editor := state.wordRunEditors[intent.runID]; editor != nil {
				editor.SetText(result.pageBreakSourcePrefix)
			}
			state.setCommittedWordTextAndClearDraft(intent.runID, result.pageBreakSourcePrefix)
		}
	}
	if !active {
		return
	}
	newRun, ok := selectedWordRun(wordBlocksForState(state), result.pageBreakCaretRunID)
	if !ok || result.pageBreakParagraphID == "" {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_separator_was_inserted_but_3275d1"}, feedbackWarning, notificationToast)
		a.persistActiveSession()
		return
	}
	state.selectedWordRunID = result.pageBreakCaretRunID
	state.selectedWordParagraphID = result.pageBreakParagraphID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordPendingFocusRunID = result.pageBreakCaretRunID
	state.wordPendingCaretRunID = result.pageBreakCaretRunID
	state.wordPendingCaretOffset = 0
	caretEditor := wordRunEditor(state, newRun)
	caretEditor.SetText(newRun.Text)
	caretEditor.SetCaret(0, 0)
	reconcileWordSelection(state, false)
	messageID := "word.page_break.status.page"
	if intent != nil && intent.blankPage {
		messageID = "word.page_break.status.blank_page"
	} else if intent != nil && intent.breakType == document.WordBreakColumn {
		messageID = "word.page_break.status.column"
	}
	a.setStatusMessage(i18n.Message{ID: messageID}, feedbackSuccess, notificationToast)
	a.persistActiveSession()
}
