package desktopui

import (
	"glance/internal/i18n"
	"sort"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/document"
)

type wordParagraphSplitIntent struct {
	runID          string
	value          string
	selectionStart int
	selectionEnd   int
	boundaryInsert bool
	insertBefore   bool
}

type wordParagraphJoinIntent struct {
	runID     string
	direction document.WordParagraphJoinDirection
}

// Word text remains in the per-view draft while a save serializes the shared
// adapter. Other operations can change the run structure or formatting, so
// keep editors read-only for those operations.
func wordEditorOperationPending(state *documentSessionState) bool {
	return state == nil || (state.formatRunning.Load() && !state.saveRunning.Load())
}

func wordHasPendingTextDrafts(state *documentSessionState) bool {
	return wordHasPendingTextDraftsExcept(state, "")
}

func wordHasPendingTextDraftsExcept(state *documentSessionState, exceptRunID string) bool {
	if state == nil {
		return false
	}
	committed, drafts := state.wordTextStateSnapshot()
	for runID, value := range drafts {
		if runID == exceptRunID {
			continue
		}
		if value != committed[runID] {
			return true
		}
	}
	return false
}

func (a *App) queueWordParagraphSplit(runID, value string, selectionStart, selectionEnd int) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.wordParagraphSplitIntent != nil {
		return
	}
	if selectionStart > selectionEnd {
		selectionStart, selectionEnd = selectionEnd, selectionStart
	}
	if _, _, valid := wordTextAroundSelection(value, selectionStart, selectionEnd); !valid {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_split_the_paragraph_c61e5a"}, feedbackWarning, notificationToast)
		return
	}
	boundaryInsert, insertBefore := wordParagraphBoundaryForEnter(state, runID, value, selectionStart, selectionEnd)
	if boundaryInsert {
		if _, ok := any(state.word).(document.WordParagraphBoundaryEditor); !ok {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserting_a_paragraph_at_a_550718"}, feedbackWarning, notificationToast)
			return
		}
	} else if _, ok := any(state.word).(document.WordParagraphEditor); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.splitting_a_paragraph_at_the_8c89b9"}, feedbackWarning, notificationToast)
		return
	}
	state.wordParagraphSplitIntent = &wordParagraphSplitIntent{
		runID:          runID,
		value:          value,
		selectionStart: selectionStart,
		selectionEnd:   selectionEnd,
		boundaryInsert: boundaryInsert,
		insertBefore:   insertBefore,
	}
	logWordDebug("paragraph_enter_queued revision=%d run=%q selection=%d:%d length=%d boundary=%v insert_before=%v pending_drafts=%v before=%s",
		state.wordRevisionAtomic.Load(), runID, selectionStart, selectionEnd, len([]rune(value)), boundaryInsert, insertBefore, wordHasPendingTextDrafts(state),
		wordDebugParagraphNeighborhood(wordBlocksForState(state), runID))
	captureWordTextDraft(state, runID, value)
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommit()
	}
}

func (a *App) queueWordParagraphJoin(runID string, value string, direction document.WordParagraphJoinDirection) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.wordParagraphJoinIntent != nil || state.wordParagraphSplitIntent != nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	if _, ok := any(state.word).(document.WordParagraphJoiner); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.paragraph_merging_is_not_supported_9365e5"}, feedbackWarning, notificationToast)
		return
	}
	state.wordParagraphJoinIntent = &wordParagraphJoinIntent{runID: runID, direction: direction}
	captureWordTextDraft(state, runID, value)
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommit()
	}
}

func captureWordTextDraft(state *documentSessionState, runID, value string) {
	state.wordExportCommitMu.Lock()
	defer state.wordExportCommitMu.Unlock()
	state.captureWordTextDraft(runID, value)
}

func captureWordEditorDraft(state *documentSessionState, runID, value string) {
	state.wordExportCommitMu.Lock()
	defer state.wordExportCommitMu.Unlock()
	state.captureWordEditorDraft(runID, value, state.appliedWordTextResetEpoch.Load())
}

func (a *App) requestWordTextFlush(all bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(state.wordFlushAllDrafts.Load() || all)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommit()
	}
}

// handleWordHistoryShortcut routes Word undo and redo through the document
// history. Gio's Editor records programmatic SetText calls in its private
// undo stack, so its local Ctrl+Z can undo a paragraph split or a model sync
// and leave the document draft inconsistent with the DOCX model.
func (a *App) handleWordHistoryShortcut(gtx layout.Context, runID string, editor *widget.Editor) bool {
	if editor == nil {
		return false
	}
	event, ok := gtx.Event(
		key.Filter{
			Focus:    editor,
			Name:     key.Name("Z"),
			Required: key.ModShortcut,
			Optional: key.ModShift,
		},
		key.Filter{Focus: editor, Name: key.Name("Y"), Required: key.ModShortcut},
	)
	if !ok {
		return false
	}
	keyEvent, ok := event.(key.Event)
	if !ok || keyEvent.State != key.Press {
		return true
	}

	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return true
	}
	// A targeted key filter may pull the shortcut ahead of earlier text or
	// selection events already queued for the editor. Let Gio apply those first
	// so the snapshot we flush includes the user's latest input.
	for pending, ok := editor.Update(gtx); ok; pending, ok = editor.Update(gtx) {
		if _, submit := pending.(widget.SubmitEvent); submit {
			start, end := editor.Selection()
			a.queueWordParagraphSplit(runID, editor.Text(), start, end)
			if state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == runID {
				return true
			}
		}
	}
	action := "undo"
	if keyEvent.Name == key.Name("Y") || keyEvent.Modifiers.Contain(key.ModShift) {
		action = "redo"
	}

	// The editor's value may have changed in this frame. Capture it before
	// either the flush or the document history operation begins.
	captureWordEditorDraft(state, runID, editor.Text())
	if wordHasPendingTextDrafts(state) {
		if state.wordDeferredToolbarAction != "" {
			return true
		}
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		state.wordDeferredToolbarAction = action
		a.startNextWordTextCommitFor(state)
		return true
	}

	a.applyWordHistory(action == "undo")
	return true
}

func (a *App) startNextWordTextCommit() {
	a.startNextWordTextCommitFor(a.documentSessionState)
}

func (a *App) startNextWordTextCommitFor(state *documentSessionState) {
	if state == nil || state.word == nil || state.formatRunning.Load() || !state.wordFlushRequested.Load() {
		return
	}
	committed, drafts := state.wordTextStateSnapshot()
	var runIDs []string
	for runID, value := range drafts {
		if value == committed[runID] {
			state.discardWordDraftIfCommitted(runID, value)
			continue
		}
		if state.wordPageBreakIntent != nil && runID == state.wordPageBreakIntent.runID {
			continue
		}
		if !state.wordFlushAllDrafts.Load() && runID == state.wordFocusedRunID {
			continue
		}
		runIDs = append(runIDs, runID)
	}
	if len(runIDs) == 0 {
		if state.wordParagraphJoinIntent != nil {
			if wordHasPendingTextDrafts(state) {
				return
			}
			a.finishWordTextFlush(state)
			return
		}
		if intent := state.wordParagraphSplitIntent; intent != nil {
			if wordHasPendingTextDrafts(state) {
				return
			}
			a.finishWordTextFlush(state)
			return
		}
		if intent := state.wordPageBreakIntent; intent != nil {
			if wordHasPendingTextDraftsExcept(state, intent.runID) {
				return
			}
			a.finishWordTextFlush(state)
			return
		}
		if state.wordFlushAllDrafts.Load() && wordHasPendingTextDrafts(state) {
			return
		}
		if wordHasPendingTextDrafts(state) {
			return
		}
		a.finishWordTextFlush(state)
		return
	}
	sort.Strings(runIDs)
	runID := runIDs[0]
	value, _ := state.wordDraft(runID)
	_, ok := any(state.word).(document.WordTextEditor)
	if !ok {
		state.wordFlushRequested.Store(false)
		state.wordFlushAllDrafts.Store(false)
		state.clearPendingWordRestore()
		state.wordParagraphJoinIntent = nil
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.in_place_text_editing_is_712956"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation := a.documentGeneration
	applicationCommand := a.applicationSessionRegistered(token.documentID)
	a.launchManagedBackground(func() {
		if !applicationCommand {
			blocks, err := runWordMutation(state, func() error { return state.word.ReplaceTextRun(runID, value) })
			a.publishWordCommand(wordCommandResult{
				state: state, operationToken: token, generation: generation, operation: "编辑文字",
				err: err, blocks: blocks, textDraft: true, runID: runID, runText: value,
			})
			return
		}
		result, err := a.executeRibbonApplicationCommand(token, "word.text_run.replace", "word.text_run", runID, "", map[string]string{
			"run_id": runID, "value": value,
		})
		revision := uint64(0)
		if err == nil {
			revision = result.Revision
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, revision: revision, applicationCommand: true,
			generation: generation, operation: "编辑文字", err: err, textDraft: true, runID: runID, runText: value,
		})
	})
}

func (a *App) finishWordTextFlush(state *documentSessionState) {
	if state == nil {
		return
	}
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.wordForceDraftFlush.Store(false)
	if versionID, handled := state.claimWordRestoreAfterDrafts(); handled {
		if versionID == 0 {
			return
		}
		if !a.startRestoreWordVersionAfterDrafts(state, versionID) {
			state.releaseWordRestoreClaim()
			a.publishWordRestoreNoticeMessage(state, i18n.Message{ID: "word.history.error.restore_start_failed"})
		}
		return
	}
	if state.wordTableSelectionDeleteIntent != nil {
		a.startWordTableSelectionDelete(state)
		return
	}
	if state.wordSelectionEditIntent != nil {
		a.startWordSelectionEdit(state)
		return
	}
	if state.wordParagraphSplitIntent != nil {
		a.startWordParagraphSplit(state)
		return
	}
	if state.wordParagraphJoinIntent != nil {
		a.startWordParagraphJoin(state)
		return
	}
	if state.wordPageBreakIntent != nil {
		a.startWordPageBreak(state)
		return
	}
	if state.wordPDFExportAfterDrafts {
		state.wordPDFExportAfterDrafts = false
		a.beginWordPDFExport(state)
		return
	}
	if state.wordPrintAfterDrafts {
		state.wordPrintAfterDrafts = false
		a.beginWordPrint(state)
		return
	}
	if state.wordSaveAfterDrafts {
		state.wordSaveAfterDrafts = false
		reason := state.wordSaveAfterDraftsReason
		state.wordSaveAfterDraftsReason = wordSaveManual
		a.startSaveForReason(reason)
		return
	}
	if state.wordInsertAfterDraftFlush {
		state.wordInsertAfterDraftFlush = false
		afterID := state.wordInsertAfterParagraphID
		state.wordInsertAfterParagraphID = ""
		a.startWordParagraphInsertAfter(afterID)
		return
	}
	if state.wordPendingPasteSelection != nil {
		a.startPendingWordSelectionPaste(state)
		return
	}
	if action := state.wordDeferredToolbarAction; action != "" {
		state.wordDeferredToolbarAction = ""
		a.executeWordToolbarAction(action)
	}
}

func (a *App) startWordParagraphJoin(state *documentSessionState) {
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || state.wordParagraphJoinIntent == nil {
		return
	}
	joiner, ok := any(state.word).(document.WordParagraphJoiner)
	if !ok {
		state.wordParagraphJoinIntent = nil
		state.wordFlushRequested.Store(false)
		state.wordFlushAllDrafts.Store(false)
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.paragraph_merging_is_not_supported_9365e5"}, feedbackWarning, notificationToast)
		return
	}
	intent := *state.wordParagraphJoinIntent
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		var joined document.WordParagraphJoinResult
		blocks, err := runWordMutation(state, func() error {
			var joinErr error
			joined, joinErr = joiner.JoinParagraphAtBoundary(intent.runID, intent.direction)
			return joinErr
		})
		a.publishWordCommand(wordCommandResult{
			state:           state,
			generation:      generation,
			operation:       "合并段落",
			err:             err,
			blocks:          blocks,
			paragraphJoin:   true,
			joinParagraphID: joined.ParagraphID,
			joinRunID:       joined.RunID,
			joinCaret:       joined.Caret,
		})
	})
}

func (a *App) completeWordParagraphJoin(result wordCommandResult, active bool) {
	state := result.state
	if state == nil {
		return
	}
	state.wordParagraphJoinIntent = nil
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.formatRunning.Store(false)
	if result.err != nil {
		if active {
			a.setWordCommandStatus(result.err)
			a.finishWordTextFlush(state)
		}
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	reconcileWordSelection(state, false)
	run, ok := selectedWordRun(wordBlocksForState(state), result.joinRunID)
	if !ok || !run.Editable {
		if active {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.paragraphs_were_merged_but_the_68d513"}, feedbackWarning, notificationToast)
			a.finishWordTextFlush(state)
		}
		return
	}
	state.setCommittedWordTextAndClearDraft(run.ID, run.Text)
	editor := wordRunEditor(state, run)
	editor.SetText(run.Text)
	caret := max(0, min(result.joinCaret, len([]rune(run.Text))))
	editor.SetCaret(caret, caret)
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = result.joinParagraphID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	if active {
		state.wordPendingFocusRunID = run.ID
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.editing_word_document_text_556c9c"}, feedbackInfo, notificationSuppressed)
		a.persistActiveSession()
	} else {
		state.wordPendingFocusRunID = ""
	}
	a.finishWordTextFlush(state)
}

func (a *App) startWordParagraphSplit(state *documentSessionState) {
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || state.wordParagraphSplitIntent == nil {
		return
	}
	intent := *state.wordParagraphSplitIntent
	prefix, suffix, valid := wordTextAroundSelection(intent.value, intent.selectionStart, intent.selectionEnd)
	if !valid {
		state.wordParagraphSplitIntent = nil
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_split_the_paragraph_c61e5a"}, feedbackWarning, notificationToast)
		return
	}
	var splitter document.WordParagraphEditor
	var boundaryEditor document.WordParagraphBoundaryEditor
	if intent.boundaryInsert {
		boundaryEditor, _ = any(state.word).(document.WordParagraphBoundaryEditor)
		if boundaryEditor == nil {
			state.wordParagraphSplitIntent = nil
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserting_a_paragraph_at_a_550718"}, feedbackWarning, notificationToast)
			return
		}
	} else {
		splitter, _ = any(state.word).(document.WordParagraphEditor)
		if splitter == nil {
			state.wordParagraphSplitIntent = nil
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.splitting_a_paragraph_at_the_8c89b9"}, feedbackWarning, notificationToast)
			return
		}
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	logWordDebug("paragraph_enter_begin revision=%d run=%q selection=%d:%d input_length=%d prefix_length=%d suffix_length=%d boundary=%v insert_before=%v before=%s",
		state.wordRevisionAtomic.Load(), intent.runID, intent.selectionStart, intent.selectionEnd, len([]rune(intent.value)), len([]rune(prefix)), len([]rune(suffix)), intent.boundaryInsert, intent.insertBefore,
		wordDebugParagraphNeighborhood(wordBlocksForState(state), intent.runID))
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		var paragraphID, runID string
		blocks, err := runWordMutation(state, func() error {
			if intent.boundaryInsert {
				var insertErr error
				paragraphID, runID, insertErr = boundaryEditor.InsertParagraphAtBoundary(intent.runID, intent.insertBefore)
				return insertErr
			}
			var splitErr error
			paragraphID, runID, splitErr = splitter.SplitParagraphAt(intent.runID, intent.value, intent.selectionStart, intent.selectionEnd)
			return splitErr
		})
		operation := "拆分段落"
		resultPrefix, resultText := prefix, suffix
		if intent.boundaryInsert {
			operation = "插入段落"
			resultPrefix, resultText = intent.value, ""
		}
		logWordDebug("paragraph_enter_adapter_complete revision=%d source_run=%q new_paragraph=%q new_run=%q prefix_length=%d suffix_length=%d boundary=%v insert_before=%v error=%q source_after=%s new_after=%s",
			state.wordRevisionAtomic.Load(), intent.runID, paragraphID, runID, len([]rune(resultPrefix)), len([]rune(resultText)), intent.boundaryInsert, intent.insertBefore, wordDebugError(err),
			wordDebugParagraphNeighborhood(blocks, intent.runID), wordDebugParagraphNeighborhood(blocks, runID))
		a.publishWordCommand(wordCommandResult{
			state:            state,
			generation:       generation,
			operation:        operation,
			err:              err,
			blocks:           blocks,
			paragraphSplit:   true,
			splitSourceRunID: intent.runID,
			splitPrefix:      resultPrefix,
			splitParagraphID: paragraphID,
			splitRunID:       runID,
			splitRunText:     resultText,
		})
	})
}

func wordTextAroundSelection(value string, selectionStart, selectionEnd int) (prefix, suffix string, valid bool) {
	runes := []rune(value)
	if selectionStart < 0 || selectionEnd < selectionStart || selectionEnd > len(runes) {
		return "", "", false
	}
	return string(runes[:selectionStart]), string(runes[selectionEnd:]), true
}

func wordParagraphBoundaryForEnter(state *documentSessionState, runID, value string, selectionStart, selectionEnd int) (insert bool, before bool) {
	if state == nil || selectionStart != selectionEnd {
		return false, false
	}
	paragraph, ok := selectedWordParagraphForRun(wordBlocksForState(state), runID)
	if !ok {
		return false, false
	}
	runes := []rune(value)
	if len(runes) > 0 && selectionStart == 0 && wordParagraphHasRunAtBoundary(paragraph, runID, true) {
		return true, true
	}
	if selectionStart == len(runes) && wordParagraphHasRunAtBoundary(paragraph, runID, false) {
		return true, false
	}
	return false, false
}

func wordParagraphHasRunAtBoundary(paragraph document.WordParagraph, runID string, first bool) bool {
	if len(paragraph.Inlines) == 0 {
		return false
	}
	inline := paragraph.Inlines[0]
	if !first {
		inline = paragraph.Inlines[len(paragraph.Inlines)-1]
	}
	return inline.Text != nil && inline.Text.ID == runID && inline.Text.Editable
}

func (a *App) completeWordParagraphSplit(result wordCommandResult, active bool) {
	state := result.state
	if state == nil {
		return
	}
	defer func() {
		state.finishOperation(result.operationToken)
		if active && result.err == nil {
			a.finishWordTextFlush(state)
		}
	}()
	intent := state.wordParagraphSplitIntent
	state.wordParagraphSplitIntent = nil
	if result.err != nil {
		state.wordFlushRequested.Store(false)
		state.wordFlushAllDrafts.Store(false)
		if intent != nil {
			captureWordTextDraft(state, intent.runID, intent.value)
			if editor := state.wordRunEditors[intent.runID]; editor != nil {
				if editor.Text() != intent.value {
					selectionStart, selectionEnd := editor.Selection()
					editor.SetText(intent.value)
					length := len([]rune(intent.value))
					editor.SetCaret(max(0, min(selectionStart, length)), max(0, min(selectionEnd, length)))
				}
			}
		}
		if active {
			a.setWordCommandStatus(result.err)
		}
		return
	}
	if result.blocks != nil {
		if intent != nil && intent.boundaryInsert {
			state.setWordBlocksAndTextState(result.blocks, true)
			state.wordRunEditors = make(map[string]*widget.Editor)
		} else {
			state.setWordBlocks(result.blocks)
		}
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	if intent != nil && intent.boundaryInsert && intent.insertBefore {
		paragraph, ok := wordParagraphAfter(wordBlocksForState(state), result.splitParagraphID)
		if !ok || len(paragraph.Inlines) == 0 || paragraph.Inlines[0].Text == nil || !paragraph.Inlines[0].Text.Editable {
			if active {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.a_paragraph_was_inserted_but_5bf62a"}, feedbackWarning, notificationToast)
				a.persistActiveSession()
			}
			return
		}
		sourceRun := *paragraph.Inlines[0].Text
		state.setCommittedWordTextAndClearDraft(sourceRun.ID, intent.value)
		sourceEditor := wordRunEditor(state, sourceRun)
		if sourceEditor.Text() != intent.value {
			sourceEditor.SetText(intent.value)
		}
		sourceEditor.SetCaret(0, 0)
		state.selectedWordRunID = sourceRun.ID
		state.selectedWordParagraphID = paragraph.ID
		state.selectedWordImageID = ""
		state.selectedBlock = -1
		state.selectedBlockEditable = true
		state.wordPageTextSelection = nil
		state.wordFocusedRunID = sourceRun.ID
		state.wordPendingFocusRunID = sourceRun.ID
		state.wordPendingCaretRunID = sourceRun.ID
		state.wordPendingCaretOffset = 0
		state.wordPendingCaretByX = false
		state.wordLayoutFocusedRunID = sourceRun.ID
		state.wordPageFocusedMetaRunID = sourceRun.ID
		state.wordPageFocusedMetaRevision = state.wordRevisionAtomic.Load()
		state.wordPageFocusedParagraph = paragraph
		if active {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.editing_word_document_text_556c9c"}, feedbackInfo, notificationSuppressed)
			a.persistActiveSession()
		} else {
			state.wordPendingFocusRunID = ""
		}
		return
	}
	if sourceEditor := state.wordRunEditors[result.splitSourceRunID]; sourceEditor != nil {
		sourceEditor.SetText(result.splitPrefix)
	}
	state.setCommittedWordTextAndClearDraft(result.splitSourceRunID, result.splitPrefix)
	reconcileWordSelection(state, false)

	newRun, ok := selectedWordRun(wordBlocksForState(state), result.splitRunID)
	if !ok {
		if active {
			if intent != nil && intent.boundaryInsert {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.a_paragraph_was_inserted_but_3e5154"}, feedbackWarning, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_paragraph_was_split_but_95dfae"}, feedbackWarning, notificationToast)
			}
			a.persistActiveSession()
		}
		return
	}
	state.setCommittedWordTextAndClearDraft(result.splitRunID, result.splitRunText)
	newEditor := wordRunEditor(state, newRun)
	newEditor.SetText(result.splitRunText)
	newEditor.SetCaret(0, 0)
	state.selectedWordRunID = result.splitRunID
	state.selectedWordParagraphID = result.splitParagraphID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordPendingFocusRunID = result.splitRunID
	if active {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.editing_word_document_text_556c9c"}, feedbackInfo, notificationSuppressed)
		a.persistActiveSession()
	} else {
		state.wordPendingFocusRunID = ""
	}
}
