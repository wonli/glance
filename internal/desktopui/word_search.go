package desktopui

import (
	"unicode"

	"glance/internal/document"
)

type wordTextMatch struct {
	ParagraphID string
	RunID       string
	Start       int
	End         int
}

func findWordTextMatch(blocks []document.WordBlock, query, currentRunID string, currentEnd int, drafts map[string]string) (wordTextMatch, bool) {
	needle := []rune(query)
	if len(needle) == 0 {
		return wordTextMatch{}, false
	}
	needle = lowerWordRunes(needle)
	type candidate struct {
		paragraphID string
		run         document.WordTextRun
	}
	var candidates []candidate
	currentIndex := -1
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if inline.Text.ID == currentRunID {
				currentIndex = len(candidates)
			}
			candidates = append(candidates, candidate{paragraphID: paragraph.ID, run: *inline.Text})
		}
	}
	if len(candidates) == 0 {
		return wordTextMatch{}, false
	}
	search := func(index, from int) (wordTextMatch, bool) {
		item := candidates[index]
		value := item.run.Text
		if draft, ok := drafts[item.run.ID]; ok {
			value = draft
		}
		start := findWordRunes(lowerWordRunes([]rune(value)), needle, from)
		if start < 0 {
			return wordTextMatch{}, false
		}
		return wordTextMatch{ParagraphID: item.paragraphID, RunID: item.run.ID, Start: start, End: start + len(needle)}, true
	}
	if currentIndex < 0 {
		for index := range candidates {
			if match, ok := search(index, 0); ok {
				return match, true
			}
		}
		return wordTextMatch{}, false
	}
	if currentEnd < 0 {
		currentEnd = 0
	}
	if match, ok := search(currentIndex, currentEnd); ok {
		return match, true
	}
	for offset := 1; offset < len(candidates); offset++ {
		index := (currentIndex + offset) % len(candidates)
		if match, ok := search(index, 0); ok {
			return match, true
		}
	}
	return search(currentIndex, 0)
}

func lowerWordRunes(value []rune) []rune {
	lowered := make([]rune, len(value))
	for index, r := range value {
		lowered[index] = unicode.ToLower(r)
	}
	return lowered
}

func findWordRunes(value, needle []rune, from int) int {
	if len(needle) == 0 || from < 0 {
		return -1
	}
	for start := from; start+len(needle) <= len(value); start++ {
		matched := true
		for offset, want := range needle {
			if value[start+offset] != want {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func (a *App) showWordFindDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		return
	}
	a.modal.ShowInput("查找", "在当前文档中查找文字", state.wordFindQuery, "查找内容", func(action string) {
		query := a.modal.Input.Text()
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if query == "" {
			a.setStatus("请输入要查找的文字")
			return
		}
		state.wordFindQuery = query
		blocks := wordBlocksForState(state)
		drafts := state.wordDraftSnapshot()
		for runID, editor := range state.wordRunEditors {
			drafts[runID] = editor.Text()
		}
		currentEnd := 0
		if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
			_, currentEnd = editor.Selection()
		}
		match, found := findWordTextMatch(blocks, query, state.selectedWordRunID, currentEnd, drafts)
		if !found {
			a.setStatus("未找到：" + query)
			return
		}
		a.focusWordTextMatch(state, blocks, match)
		a.setStatus("已定位：" + query)
	})
}

func (a *App) showWordReplaceDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		return
	}
	if _, ok := any(state.word).(document.WordTextRangeEditor); !ok {
		return
	}
	a.modal.ShowDualInput("替换", "在当前文档中查找和替换文字", state.wordFindQuery, "查找内容", "", "替换为", func(action string) {
		if action == "close" {
			a.modal.Hide()
			return
		}
		query := a.modal.Input.Text()
		replacement := a.modal.SecondInput.Text()
		if query == "" {
			a.setStatus("请输入要查找的文字")
			return
		}
		state.wordFindQuery = query
		if action == "confirm" {
			a.findNextWordText(query)
			return
		}
		if action != "replace" {
			a.findNextWordText(query)
			return
		}
		match, found := selectedWordTextQueryMatch(state, query)
		if !found {
			a.findNextWordText(query)
			return
		}
		if !a.runWordRangeReplacement(match, query, replacement) {
			a.setStatus("当前 DOCX 暂时无法替换文字")
		}
	},
		modalDialogActionSpec{ID: "find-next", Label: "查找下一个"},
		modalDialogActionSpec{ID: "replace", Label: "替换"},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) findNextWordText(query string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || query == "" {
		return
	}
	blocks := wordBlocksForState(state)
	drafts := state.wordDraftSnapshot()
	for runID, editor := range state.wordRunEditors {
		drafts[runID] = editor.Text()
	}
	currentEnd := 0
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		_, currentEnd = editor.Selection()
	}
	match, found := findWordTextMatch(blocks, query, state.selectedWordRunID, currentEnd, drafts)
	if !found {
		a.setStatus("未找到：" + query)
		return
	}
	a.focusWordTextMatch(state, blocks, match)
	a.setStatus("已定位：" + query)
}

func selectedWordTextQueryMatch(state *documentSessionState, query string) (wordTextMatch, bool) {
	if state == nil || query == "" {
		return wordTextMatch{}, false
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, state.selectedWordRunID)
	if !ok || !run.Editable {
		return wordTextMatch{}, false
	}
	editor := state.wordRunEditors[run.ID]
	if editor == nil {
		return wordTextMatch{}, false
	}
	start, end := editor.Selection()
	value := []rune(editor.Text())
	needle := lowerWordRunes([]rune(query))
	if start < 0 || end <= start || end > len(value) || end-start != len(needle) {
		return wordTextMatch{}, false
	}
	if !equalWordRunes(lowerWordRunes(value[start:end]), needle) {
		return wordTextMatch{}, false
	}
	paragraph, ok := selectedWordParagraph(blocks, state.selectedWordParagraphID)
	if !ok {
		return wordTextMatch{}, false
	}
	return wordTextMatch{ParagraphID: paragraph.ID, RunID: run.ID, Start: start, End: end}, true
}

func equalWordRunes(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (a *App) runWordRangeReplacement(match wordTextMatch, query, replacement string) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	editor, ok := any(state.word).(document.WordTextRangeEditor)
	if !ok {
		return false
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return false
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return editor.ReplaceTextRange(match.RunID, match.Start, match.End, replacement) })
		a.publishWordCommand(wordCommandResult{
			state: state, generation: generation, operation: "替换文字", err: err, blocks: blocks,
			wordRangeReplacement: true, replaceRunID: match.RunID, replaceParagraphID: match.ParagraphID, replaceStart: match.Start,
			replaceText: replacement, replaceQuery: query,
		})
	}()
	return true
}

func (a *App) completeWordRangeReplacement(result wordCommandResult) {
	state := result.state
	if state == nil {
		return
	}
	if result.err != nil {
		a.setStatus(wordCommandError(result.operation, result.err))
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, result.replaceRunID)
	if !ok || !run.Editable {
		a.setStatus("文字已替换，但无法定位编辑位置")
		return
	}
	state.setCommittedWordTextAndClearDraft(run.ID, run.Text)
	editor := wordRunEditor(state, run)
	editor.SetText(run.Text)
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = result.replaceParagraphID
	if state.selectedWordParagraphID == "" {
		for _, paragraph := range allWordParagraphs(blocks) {
			for _, inline := range paragraph.Inlines {
				if inline.Text != nil && inline.Text.ID == run.ID {
					state.selectedWordParagraphID = paragraph.ID
					break
				}
			}
		}
	}
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	startAfter := result.replaceStart + len([]rune(result.replaceText))
	match, found := findWordTextMatch(blocks, result.replaceQuery, run.ID, startAfter, nil)
	if found {
		a.focusWordTextMatch(state, blocks, match)
		a.setStatus("已替换，并已定位下一处")
	} else {
		startAfter = max(0, min(startAfter, len([]rune(run.Text))))
		editor.SetCaret(startAfter, startAfter)
		state.wordPendingFocusRunID = run.ID
		a.setStatus("已替换；没有更多匹配项")
	}
	a.persistActiveSession()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) focusWordTextMatch(state *documentSessionState, blocks []document.WordBlock, match wordTextMatch) {
	run, ok := selectedWordRun(blocks, match.RunID)
	paragraph, paragraphOK := selectedWordParagraph(blocks, match.ParagraphID)
	if !ok || !paragraphOK || !run.Editable {
		return
	}
	editor := wordRunEditor(state, run)
	editor.SetCaret(match.Start, match.End)
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	if state.wordToolbarTab == wordTabPicture {
		state.wordToolbarTab = wordTabHome
	}
	state.wordPendingFocusRunID = run.ID
	state.wordPendingCaretRunID = ""
	state.wordPendingCaretByX = false
	for index, block := range blocks {
		if wordBlockContainsParagraph(block, paragraph.ID) {
			state.wordListState.Position.First = index
			state.wordListState.Position.Offset = 0
			state.wordListState.Position.OffsetLast = 0
			state.wordListState.Position.BeforeEnd = false
			break
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func wordBlockContainsParagraph(block document.WordBlock, paragraphID string) bool {
	if block.Paragraph != nil {
		return block.Paragraph.ID == paragraphID
	}
	if block.Table == nil {
		return false
	}
	for _, row := range block.Table.Rows {
		for _, cell := range row.Cells {
			for _, paragraph := range cell.Paragraphs {
				if paragraph.ID == paragraphID {
					return true
				}
			}
		}
	}
	return false
}
