package desktopui

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
)

type wordSelectionDeleteIntent struct{ start, end wordPageCaretPosition }

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
		a.setStatus("当前文档不支持删除此文字选区")
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
				captureWordTextDraft(state, id, editor.Text())
				break
			}
		}
		return true
	}
	start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection)
	if !ok {
		return true
	}
	state.wordSelectionDeleteIntent = &wordSelectionDeleteIntent{start: start, end: end}
	// The selection can include drafts in several formatted runs. Commit those
	// before the adapter resolves the rune offsets against the document.
	for id, e := range state.wordRunEditors {
		captureWordTextDraft(state, id, e.Text())
	}
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	if !state.formatRunning.Load() {
		a.startNextWordTextCommitFor(state)
	}
	return true
}

func (a *App) startWordSelectionDelete(state *documentSessionState) {
	if state == nil || state.wordSelectionDeleteIntent == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := any(state.word).(document.WordTextSelectionEditor)
	if !ok {
		return
	}
	intent := *state.wordSelectionDeleteIntent
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation := a.documentGeneration
	started := a.launchManagedBackground(func() {
		var deleted document.WordTextSelectionDeleteResult
		blocks, err := runWordMutation(state, func() error {
			var err error
			deleted, err = editor.DeleteTextSelection(intent.start.runID, intent.start.runeOffset, intent.end.runID, intent.end.runeOffset)
			return err
		})
		a.publishWordCommand(wordCommandResult{state: state, operationToken: token, generation: generation, operation: "删除选区文字", blocks: blocks, err: err, selectionDelete: true, selectionDeleteResult: deleted})
	})
	if !started {
		state.finishOperation(token)
		cancelWordSelectionDelete(state)
	}
}

func (a *App) completeWordSelectionDelete(result wordCommandResult, active bool) {
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
	state.wordSelectionDeleteIntent = nil
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.wordForceDraftFlush.Store(false)
	if result.err != nil {
		if active {
			a.setStatus(wordCommandError(result.operation, result.err))
		}
		return
	}
	state.setWordBlocksAndTextState(result.blocks, true)
	state.wordRunEditors = make(map[string]*widget.Editor)
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
	caret := result.selectionDeleteResult
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
		a.setStatus("已删除选区文字")
		a.persistActiveSession()
	}
}

// A failed flush keeps the user's draft, but must release the pending intent
// that makes its editors read-only. This also handles rejected worker dispatch.
func cancelWordSelectionDelete(state *documentSessionState) {
	state.wordSelectionDeleteIntent = nil
	state.wordFlushRequested.Store(false)
	state.wordFlushAllDrafts.Store(false)
	state.wordForceDraftFlush.Store(false)
}

// Only continue the deletion owned by this inactive view. Generic flush
// completion may dispatch actions (such as Save) against the active App view.
func (a *App) completeInactiveWordSelectionDeleteDraft(result wordCommandResult) {
	state := result.state
	if result.err != nil {
		cancelWordSelectionDelete(state)
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
	a.startWordSelectionDelete(state)
}
