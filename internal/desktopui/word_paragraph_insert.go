package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
)

func (a *App) startWordBodyParagraphInsert() {
	a.startWordParagraphInsertAfter("")
}

func (a *App) startWordParagraphInsertAfter(afterParagraphID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordBodyEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.adding_body_paragraphs_is_not_8bdb55"}, feedbackWarning, notificationToast)
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	state.wordInsertionRunning = true
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		var paragraphID string
		blocks, err := runWordMutation(state, func() error {
			var insertErr error
			if afterParagraphID != "" {
				paragraphID, insertErr = editor.InsertParagraphAfter(afterParagraphID)
			} else {
				paragraphID, insertErr = editor.InsertBodyParagraph()
			}
			return insertErr
		})
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "插入段落", err: err, blocks: blocks, insertedParagraphID: paragraphID, bodyParagraphInsert: true})
	})
}

func (a *App) queueWordParagraphInsert(state *documentSessionState, afterParagraphID string) {
	if state == nil || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() {
		return
	}
	state.wordInsertAfterParagraphID = afterParagraphID
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordInsertAfterDraftFlush = true
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		return
	}
	a.startWordParagraphInsertAfter(afterParagraphID)
}

func (a *App) focusInsertedWordParagraph(state *documentSessionState, paragraphID string) {
	if state == nil || paragraphID == "" {
		return
	}
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		if paragraph.ID != paragraphID {
			continue
		}
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			state.selectedWordRunID = inline.Text.ID
			state.selectedWordParagraphID = paragraph.ID
			state.selectedWordImageID = ""
			state.selectedBlock = -1
			state.selectedBlockEditable = true
			state.wordPendingFocusRunID = inline.Text.ID
			state.wordPendingCaretRunID = inline.Text.ID
			state.wordPendingCaretOffset = 0
			state.wordPendingCaretByX = false
			state.wordLayoutFocusedRunID = inline.Text.ID
			state.wordPageFocusedMetaRunID = inline.Text.ID
			state.wordPageFocusedMetaRevision = state.wordRevisionAtomic.Load()
			state.wordPageFocusedParagraph = paragraph
			state.wordPagePendingScrollRunID = inline.Text.ID
			editor := wordRunEditor(state, *inline.Text)
			editor.SetCaret(0, 0)
			state.wordListState.ScrollTo(max(0, len(wordBlocksForState(state))-1))
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
	}
	a.setStatusMessage(i18n.Message{ID: "word.status.inline.a_new_paragraph_was_inserted_2371f7"}, feedbackWarning, notificationToast)
}
