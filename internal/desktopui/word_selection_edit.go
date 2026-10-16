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
	a.launchManagedBackground(func() {
		var deleted document.WordTextSelectionDeleteResult
		blocks, err := runWordMutation(state, func() error {
			var err error
			deleted, err = editor.DeleteTextSelection(intent.start.runID, intent.start.runeOffset, intent.end.runID, intent.end.runeOffset)
			return err
		})
		a.publishWordCommand(wordCommandResult{state: state, operationToken: token, generation: generation, operation: "删除选区文字", blocks: blocks, err: err, selectionDelete: true, selectionDeleteResult: deleted})
	})
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
