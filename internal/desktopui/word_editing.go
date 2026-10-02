package desktopui

import (
	"sort"

	"glance/internal/document"
)

type wordParagraphSplitIntent struct {
	runID          string
	value          string
	selectionStart int
	selectionEnd   int
}

type wordParagraphJoinIntent struct {
	runID     string
	direction document.WordParagraphJoinDirection
}

func (a *App) startWordBodyParagraphInsert() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.wordInsertionRunning || state.formatRunning || a.saving {
		return
	}
	editor, ok := any(state.word).(document.WordBodyEditor)
	if !ok {
		a.status = "当前 DOCX 适配器不支持新增正文段落"
		return
	}
	state.wordInsertionRunning = true
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		paragraphID, err := editor.InsertBodyParagraph()
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "插入段落", err: err, blocks: blocks, insertedParagraphID: paragraphID, bodyParagraphInsert: true})
	}()
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
			wordRunEditor(state, *inline.Text)
			state.wordListState.ScrollTo(max(0, len(state.wordBlocks)-1))
			a.status = "已插入正文段落，可直接输入"
			return
		}
	}
	a.status = "新段落已插入，但没有可编辑的文字运行"
}

func wordHasPendingTextDrafts(state *documentSessionState) bool {
	return wordHasPendingTextDraftsExcept(state, "")
}

func wordHasPendingTextDraftsExcept(state *documentSessionState, exceptRunID string) bool {
	if state == nil {
		return false
	}
	for runID, value := range state.wordRunDrafts {
		if runID == exceptRunID {
			continue
		}
		if value != state.wordRunCommitted[runID] {
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
		a.status = "无法按当前光标位置拆分段落，请重试"
		return
	}
	if _, ok := any(state.word).(document.WordParagraphEditor); !ok {
		a.status = "当前 DOCX 适配器不支持按光标拆分段落"
		return
	}
	state.wordParagraphSplitIntent = &wordParagraphSplitIntent{
		runID:          runID,
		value:          value,
		selectionStart: selectionStart,
		selectionEnd:   selectionEnd,
	}
	captureWordTextDraft(state, runID, value)
	state.wordForceDraftFlush = true
	state.wordFlushRequested = true
	state.wordFlushAllDrafts = true
	if !state.formatRunning {
		a.startNextWordTextCommit()
	}
}

func (a *App) queueWordParagraphJoin(runID string, value string, direction document.WordParagraphJoinDirection) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.wordParagraphJoinIntent != nil || state.wordParagraphSplitIntent != nil || state.formatRunning || a.saving {
		return
	}
	if _, ok := any(state.word).(document.WordParagraphJoiner); !ok {
		a.status = "当前 DOCX 适配器不支持合并段落"
		return
	}
	state.wordParagraphJoinIntent = &wordParagraphJoinIntent{runID: runID, direction: direction}
	captureWordTextDraft(state, runID, value)
	state.wordForceDraftFlush = true
	state.wordFlushRequested = true
	state.wordFlushAllDrafts = true
	if !state.formatRunning {
		a.startNextWordTextCommit()
	}
}

func captureWordTextDraft(state *documentSessionState, runID, value string) {
	state.wordExportCommitMu.Lock()
	defer state.wordExportCommitMu.Unlock()
	if state.wordRunDrafts == nil {
		state.wordRunDrafts = make(map[string]string)
	}
	if value == state.wordRunCommitted[runID] {
		if _, exists := state.wordRunDrafts[runID]; exists {
			delete(state.wordRunDrafts, runID)
			state.wordDraftEpoch.Add(1)
		}
		return
	}
	if previous, exists := state.wordRunDrafts[runID]; exists && previous == value {
		return
	}
	state.wordRunDrafts[runID] = value
	state.wordDraftEpoch.Add(1)
}

func (a *App) requestWordTextFlush(all bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	state.wordFlushRequested = true
	state.wordFlushAllDrafts = state.wordFlushAllDrafts || all
	if !state.formatRunning {
		a.startNextWordTextCommit()
	}
}

func (a *App) startNextWordTextCommit() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || !state.wordFlushRequested {
		return
	}
	var runIDs []string
	for runID, value := range state.wordRunDrafts {
		if value == state.wordRunCommitted[runID] {
			delete(state.wordRunDrafts, runID)
			continue
		}
		if state.wordParagraphSplitIntent != nil && runID == state.wordParagraphSplitIntent.runID {
			continue
		}
		if state.wordPageBreakIntent != nil && runID == state.wordPageBreakIntent.runID {
			continue
		}
		if !state.wordFlushAllDrafts && runID == state.wordFocusedRunID {
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
			if wordHasPendingTextDraftsExcept(state, intent.runID) {
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
		if state.wordFlushAllDrafts && wordHasPendingTextDrafts(state) {
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
	value := state.wordRunDrafts[runID]
	editor, ok := any(state.word).(document.WordTextEditor)
	if !ok {
		state.wordFlushRequested = false
		state.wordParagraphJoinIntent = nil
		a.status = "当前 DOCX 适配器不支持原位文字编辑"
		return
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		err := editor.ReplaceTextRun(runID, value)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "编辑文字", err: err, blocks: blocks, textDraft: true, runID: runID, runText: value})
	}()
}

func (a *App) finishWordTextFlush(state *documentSessionState) {
	if state == nil {
		return
	}
	state.wordFlushRequested = false
	state.wordFlushAllDrafts = false
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
		a.startSave()
		return
	}
	if state.wordInsertAfterDraftFlush {
		state.wordInsertAfterDraftFlush = false
		a.startWordBodyParagraphInsert()
		return
	}
	if state.wordPendingPasteRunID != "" {
		runID, value := state.wordPendingPasteRunID, state.wordPendingPasteText
		state.wordPendingPasteRunID, state.wordPendingPasteText = "", ""
		a.runWordCommand("粘贴文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, value) })
		return
	}
	if action := state.wordDeferredToolbarAction; action != "" {
		state.wordDeferredToolbarAction = ""
		a.executeWordToolbarAction(action)
	}
}

func (a *App) startWordParagraphJoin(state *documentSessionState) {
	if state == nil || state.word == nil || state.formatRunning || a.saving || state.wordParagraphJoinIntent == nil {
		return
	}
	joiner, ok := any(state.word).(document.WordParagraphJoiner)
	if !ok {
		state.wordParagraphJoinIntent = nil
		state.wordFlushRequested = false
		state.wordFlushAllDrafts = false
		a.status = "当前 DOCX 适配器不支持合并段落"
		return
	}
	intent := *state.wordParagraphJoinIntent
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		joined, err := joiner.JoinParagraphAtBoundary(intent.runID, intent.direction)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
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
	}()
}

func (a *App) completeWordParagraphJoin(result wordCommandResult, active bool) {
	state := result.state
	if state == nil {
		return
	}
	state.wordParagraphJoinIntent = nil
	state.wordFlushRequested = false
	state.wordFlushAllDrafts = false
	if result.err != nil {
		if active {
			a.status = wordCommandError(result.operation, result.err)
			a.finishWordTextFlush(state)
		}
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
	}
	reconcileWordSelection(state, false)
	run, ok := selectedWordRun(state.wordBlocks, result.joinRunID)
	if !ok || !run.Editable {
		if active {
			a.status = "段落已合并，但无法定位接缝文字"
			a.finishWordTextFlush(state)
		}
		return
	}
	delete(state.wordRunDrafts, run.ID)
	state.wordRunCommitted[run.ID] = run.Text
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
		a.status = "正在编辑 Word 正文"
		a.persistActiveSession()
		a.finishWordTextFlush(state)
	} else {
		state.wordPendingFocusRunID = ""
	}
}

func (a *App) startWordParagraphSplit(state *documentSessionState) {
	if state == nil || state.word == nil || state.formatRunning || a.saving || state.wordParagraphSplitIntent == nil {
		return
	}
	splitter, ok := any(state.word).(document.WordParagraphEditor)
	if !ok {
		state.wordParagraphSplitIntent = nil
		a.status = "当前 DOCX 适配器不支持按光标拆分段落"
		return
	}
	intent := *state.wordParagraphSplitIntent
	prefix, suffix, valid := wordTextAroundSelection(intent.value, intent.selectionStart, intent.selectionEnd)
	if !valid {
		state.wordParagraphSplitIntent = nil
		a.status = "无法按当前光标位置拆分段落，请重试"
		return
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		paragraphID, runID, err := splitter.SplitParagraphAt(intent.runID, intent.value, intent.selectionStart, intent.selectionEnd)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		a.publishWordCommand(wordCommandResult{
			state:            state,
			generation:       generation,
			operation:        "拆分段落",
			err:              err,
			blocks:           blocks,
			paragraphSplit:   true,
			splitSourceRunID: intent.runID,
			splitPrefix:      prefix,
			splitParagraphID: paragraphID,
			splitRunID:       runID,
			splitRunText:     suffix,
		})
	}()
}

func wordTextAroundSelection(value string, selectionStart, selectionEnd int) (prefix, suffix string, valid bool) {
	runes := []rune(value)
	if selectionStart < 0 || selectionEnd < selectionStart || selectionEnd > len(runes) {
		return "", "", false
	}
	return string(runes[:selectionStart]), string(runes[selectionEnd:]), true
}

func (a *App) completeWordParagraphSplit(result wordCommandResult) {
	state := result.state
	if state == nil {
		return
	}
	intent := state.wordParagraphSplitIntent
	state.wordParagraphSplitIntent = nil
	if result.err != nil {
		state.wordFlushRequested = false
		state.wordFlushAllDrafts = false
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
		a.status = wordCommandError(result.operation, result.err)
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
	}
	if sourceEditor := state.wordRunEditors[result.splitSourceRunID]; sourceEditor != nil {
		sourceEditor.SetText(result.splitPrefix)
	}
	state.wordRunCommitted[result.splitSourceRunID] = result.splitPrefix
	delete(state.wordRunDrafts, result.splitSourceRunID)
	reconcileWordSelection(state, false)

	newRun, ok := selectedWordRun(state.wordBlocks, result.splitRunID)
	if !ok {
		a.status = "段落已拆分，但无法定位后续文字"
		a.persistActiveSession()
		a.finishWordTextFlush(state)
		return
	}
	delete(state.wordRunDrafts, result.splitRunID)
	state.wordRunCommitted[result.splitRunID] = result.splitRunText
	newEditor := wordRunEditor(state, newRun)
	newEditor.SetText(result.splitRunText)
	newEditor.SetCaret(0, 0)
	state.selectedWordRunID = result.splitRunID
	state.selectedWordParagraphID = result.splitParagraphID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordPendingFocusRunID = result.splitRunID
	a.status = "正在编辑 Word 正文"
	a.persistActiveSession()
	a.finishWordTextFlush(state)
}
