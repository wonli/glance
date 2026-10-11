package desktopui

import "glance/internal/document"

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
		a.setStatus("当前 DOCX 适配器不支持插入分隔符")
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
		a.setStatus("无法按当前光标位置插入分隔符，请重试")
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
		a.setStatus("当前 DOCX 适配器不支持插入分隔符")
		return
	}
	intent := *state.wordPageBreakIntent
	prefix, _, valid := wordTextAroundSelection(intent.value, intent.selectionStart, intent.selectionEnd)
	if !valid {
		state.wordPageBreakIntent = nil
		state.wordFlushRequested.Store(false)
		state.wordFlushAllDrafts.Store(false)
		a.setStatus("无法按当前光标位置插入分隔符，请重试")
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
			a.setStatus(wordCommandError(result.operation, result.err))
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
		a.setStatus("分隔符已插入，但无法定位插入点")
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
	msg := "已插入分页符"
	if intent != nil && intent.blankPage {
		msg = "已插入空白页"
	} else if intent != nil && intent.breakType == document.WordBreakColumn {
		msg = "已插入分栏符"
	}
	a.setStatus(msg)
	a.persistActiveSession()
}
