package desktopui

import (
	"sort"

	"glance/internal/document"
)

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
			state.wordToolbarTab = wordTabHome
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
	if state == nil {
		return false
	}
	for runID, value := range state.wordRunDrafts {
		if value != state.wordRunCommitted[runID] {
			return true
		}
	}
	return false
}

func captureWordTextDraft(state *documentSessionState, runID, value string) {
	if state.wordRunDrafts == nil {
		state.wordRunDrafts = make(map[string]string)
	}
	if value == state.wordRunCommitted[runID] {
		delete(state.wordRunDrafts, runID)
		return
	}
	state.wordRunDrafts[runID] = value
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
		if !state.wordFlushAllDrafts && runID == state.wordFocusedRunID {
			continue
		}
		runIDs = append(runIDs, runID)
	}
	if len(runIDs) == 0 {
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
