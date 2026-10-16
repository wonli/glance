package desktopui

import (
	"fmt"
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
	a.showWordSearchReplaceDialog(false)
}

func (a *App) showWordReplaceDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		a.setStatus("当前文档暂不支持查找")
		return
	}
	if _, ok := any(state.word).(document.WordTextRangeEditor); !ok {
		a.setStatus("当前 DOCX 暂不支持文字替换")
		return
	}
	a.showWordSearchReplaceDialog(true)
}

func (a *App) showWordSearchReplaceDialog(replaceTab bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		a.setStatus("当前文档暂不支持查找")
		return
	}
	_, canReplace := any(state.word).(document.WordTextRangeEditor)
	if replaceTab && !canReplace {
		a.setStatus("当前 DOCX 暂不支持文字替换")
		return
	}
	actions := []modalDialogActionSpec{{ID: "find-next", Label: "查找下一个", Style: modalDialogActionPrimary}}
	if canReplace {
		actions = []modalDialogActionSpec{
			{ID: "replace", Label: "替换", Style: modalDialogActionSecondary, WidthDp: 76, TextSizeSp: 12},
			{ID: "find-next", Label: "查找下一个", Style: modalDialogActionPrimary, WidthDp: 130, TextSizeSp: 12},
		}
	}
	onAction := func(action string) {
		if action == "close" {
			a.modal.Hide()
			return
		}
		query := a.modal.Input.Text()
		replacement := a.modal.SecondInput.Text()
		if query == "" {
			a.setStatus("请输入要查找的文字")
			a.modal.SearchReplaceSummary = "请输入要查找的文字"
			a.modal.FocusInputRequested = true
			return
		}
		state.wordFindQuery = query
		if canReplace {
			state.wordReplaceText = replacement
		}
		if action == "replace" {
			match, found := selectedWordTextQueryMatch(state, query)
			if !found {
				a.findNextWordText(query)
				a.modal.FocusInputRequested = true
				return
			}
			if !a.runWordRangeReplacement(match, query, replacement) {
				a.setStatus("当前 DOCX 暂时无法替换文字")
				a.modal.SearchReplaceSummary = "当前匹配项暂时无法替换"
				a.modal.FocusInputRequested = true
				return
			}
			a.modal.SearchReplaceSummary = "正在替换当前匹配项…"
			a.modal.FocusInputRequested = true
			return
		}
		if action == "confirm" {
			a.findNextWordText(query)
			a.modal.Hide()
			return
		}
		if action == "find-next" {
			a.findNextWordText(query)
			a.modal.FocusInputRequested = true
		}
	}
	if canReplace {
		a.modal.ShowDualInput("查找和替换", "", state.wordFindQuery, "查找内容", state.wordReplaceText, "替换为", onAction, actions...)
	} else {
		a.modal.ShowInput("查找和替换", "", state.wordFindQuery, "查找内容", onAction, actions...)
	}
	a.modal.SearchReplace = true
	a.modal.SearchReplaceHasReplace = canReplace
	a.modal.SearchReplaceReplaceTab = replaceTab
	a.modal.SearchReplaceSummary = "输入查找内容后，点击“查找下一个”"
	a.modal.CardWidthDp = 560
	a.modal.FocusInputRequested = true
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
		a.modal.SearchReplaceSummary = "未找到匹配项"
		return
	}
	a.focusWordTextMatch(state, blocks, match)
	a.updateWordSearchSummary(state, query, blocks, drafts, match)
	a.setStatus("已定位：" + query)
}

func findAllWordTextMatches(blocks []document.WordBlock, query string, drafts map[string]string) []wordTextMatch {
	needle := lowerWordRunes([]rune(query))
	if len(needle) == 0 {
		return nil
	}
	var matches []wordTextMatch
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			value := inline.Text.Text
			if draft, ok := drafts[inline.Text.ID]; ok {
				value = draft
			}
			runes := lowerWordRunes([]rune(value))
			for from := 0; from < len(runes); {
				start := findWordRunes(runes, needle, from)
				if start < 0 {
					break
				}
				matches = append(matches, wordTextMatch{ParagraphID: paragraph.ID, RunID: inline.Text.ID, Start: start, End: start + len(needle)})
				from = start + len(needle)
			}
		}
	}
	return matches
}

func (a *App) updateWordSearchSummary(state *documentSessionState, query string, blocks []document.WordBlock, drafts map[string]string, current wordTextMatch) {
	matches := findAllWordTextMatches(blocks, query, drafts)
	if len(matches) == 0 {
		a.modal.SearchReplaceSummary = "未找到匹配项"
		return
	}
	index := 0
	for i, match := range matches {
		if match.RunID == current.RunID && match.Start == current.Start {
			index = i + 1
			break
		}
	}
	if index == 0 {
		if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
			start, _ := editor.Selection()
			for i, match := range matches {
				if match.RunID == state.selectedWordRunID && match.Start == start {
					index = i + 1
					break
				}
			}
		}
	}
	if index == 0 {
		index = 1
	}
	a.modal.SearchReplaceSummary = fmt.Sprintf("找到 %d 个匹配项 · 当前第 %d 项", len(matches), index)
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
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error { return editor.ReplaceTextRange(match.RunID, match.Start, match.End, replacement) })
		a.publishWordCommand(wordCommandResult{
			state: state, generation: generation, operation: "替换文字", err: err, blocks: blocks,
			wordRangeReplacement: true, replaceRunID: match.RunID, replaceParagraphID: match.ParagraphID, replaceStart: match.Start,
			replaceText: replacement, replaceQuery: query,
		})
	})
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
		if a.modal.Visible && a.modal.SearchReplace {
			drafts := state.wordDraftSnapshot()
			for runID, editor := range state.wordRunEditors {
				drafts[runID] = editor.Text()
			}
			a.updateWordSearchSummary(state, result.replaceQuery, blocks, drafts, match)
		}
	} else {
		startAfter = max(0, min(startAfter, len([]rune(run.Text))))
		editor.SetCaret(startAfter, startAfter)
		state.wordPendingFocusRunID = run.ID
		a.setStatus("已替换；没有更多匹配项")
		if a.modal.Visible && a.modal.SearchReplace {
			a.modal.SearchReplaceSummary = "已替换当前匹配项；没有更多匹配项"
		}
	}
	a.persistActiveSession()
	if a.modal.Visible && a.modal.SearchReplace {
		a.modal.FocusInputRequested = true
	}
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
