package desktopui

import (
	"fmt"
	"strings"
	"unicode"

	"glance/internal/document"
)

type wordTextMatch struct {
	ParagraphID string
	RunID       string
	Start       int
	End         int
	EndRunID    string
	EndOffset   int
}

type wordSearchTextRange struct {
	Start int
	End   int
}

type wordSearchResults struct {
	matches []wordTextMatch
	ranges  map[string][]wordSearchTextRange
}

func findWordTextMatch(blocks []document.WordBlock, query, currentRunID string, currentEnd int, drafts map[string]string) (wordTextMatch, bool) {
	results := findWordSearchResults(blocks, query, drafts)
	return findWordTextMatchInResults(blocks, results.matches, currentRunID, currentEnd)
}

func findWordTextMatchInResults(blocks []document.WordBlock, matches []wordTextMatch, currentRunID string, currentEnd int) (wordTextMatch, bool) {
	if len(matches) == 0 {
		return wordTextMatch{}, false
	}
	if currentRunID == "" {
		return matches[0], true
	}
	runOrder := make(map[string]int)
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if _, exists := runOrder[inline.Text.ID]; !exists {
				runOrder[inline.Text.ID] = len(runOrder)
			}
		}
	}
	currentOrder, exists := runOrder[currentRunID]
	if !exists {
		return matches[0], true
	}
	currentEnd = max(0, currentEnd)
	for _, match := range matches {
		matchOrder, exists := runOrder[match.RunID]
		if exists && (matchOrder > currentOrder || (matchOrder == currentOrder && match.Start >= currentEnd)) {
			return match, true
		}
	}
	return matches[0], true
}

func wordTextMatchEndRunID(match wordTextMatch) string {
	if match.EndRunID != "" {
		return match.EndRunID
	}
	return match.RunID
}

func wordTextMatchEndOffset(match wordTextMatch) int {
	if match.EndRunID != "" {
		return match.EndOffset
	}
	return match.End
}

type wordSearchSegment struct {
	run   document.WordTextRun
	text  []rune
	start int
}

type wordSearchChunk struct {
	paragraphID string
	segments    []wordSearchSegment
	text        []rune
}

func wordSearchChunks(blocks []document.WordBlock, drafts map[string]string) []wordSearchChunk {
	var chunks []wordSearchChunk
	var chunk wordSearchChunk
	flush := func() {
		if len(chunk.segments) > 0 {
			chunks = append(chunks, chunk)
		}
		chunk = wordSearchChunk{}
	}
	for _, paragraph := range allWordParagraphs(blocks) {
		flush()
		for _, inline := range paragraph.Inlines {
			run := inline.Text
			if inline.Kind != document.WordTextInline || run == nil || !run.Editable {
				flush()
				continue
			}
			if len(chunk.segments) == 0 {
				chunk.paragraphID = paragraph.ID
			}
			value := run.Text
			if draft, ok := drafts[run.ID]; ok {
				value = draft
			}
			runes := []rune(value)
			chunk.segments = append(chunk.segments, wordSearchSegment{run: *run, text: runes, start: len(chunk.text)})
			chunk.text = append(chunk.text, runes...)
		}
		flush()
	}
	return chunks
}

func wordSearchMatchAt(chunk wordSearchChunk, start, end int) (wordTextMatch, bool) {
	var startSegment, endSegment *wordSearchSegment
	for index := range chunk.segments {
		segment := &chunk.segments[index]
		segmentEnd := segment.start + len(segment.text)
		if start >= segment.start && start < segmentEnd {
			startSegment = segment
		}
		if end > segment.start && end <= segmentEnd {
			endSegment = segment
		}
	}
	if startSegment == nil || endSegment == nil {
		return wordTextMatch{}, false
	}
	startOffset := start - startSegment.start
	endOffset := end - endSegment.start
	return wordTextMatch{
		ParagraphID: chunk.paragraphID,
		RunID:       startSegment.run.ID,
		Start:       startOffset,
		End:         endOffset,
		EndRunID:    endSegment.run.ID,
		EndOffset:   endOffset,
	}, true
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
	query := wordFindInitialQuery(state)
	actions := []modalDialogActionSpec{{ID: "find-next", Label: "查找下一个", Style: modalDialogActionPrimary}}
	if canReplace {
		actions = []modalDialogActionSpec{
			{ID: "replace", Label: "替换", Style: modalDialogActionSecondary, WidthDp: 76, TextSizeSp: 12},
		}
		if _, canReplaceAll := any(state.word).(document.WordTextBatchEditor); canReplaceAll {
			actions = append(actions, modalDialogActionSpec{ID: "replace-all", Label: "全部替换", Style: modalDialogActionSecondary, WidthDp: 92, TextSizeSp: 11})
		}
		actions = append(actions, modalDialogActionSpec{ID: "find-next", Label: "查找下一个", Style: modalDialogActionPrimary, WidthDp: 130, TextSizeSp: 12})
	}
	onAction := func(action string) {
		if action == "close" {
			a.hideWordSearchReplaceDialog(state)
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
		if action == "replace-all" {
			a.replaceAllWordText(query, replacement)
			a.modal.FocusInputRequested = true
			return
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
			a.modal.FocusInputRequested = true
			return
		}
		if action == "confirm" {
			a.findNextWordText(query)
			a.hideWordSearchReplaceDialog(state)
			return
		}
		if action == "find-next" {
			a.findNextWordText(query)
			a.modal.FocusInputRequested = true
			return
		}
		if action == "find-previous" {
			a.findPreviousWordText(query)
			a.modal.FocusInputRequested = true
		}
	}
	if canReplace {
		a.modal.ShowDualInput("查找和替换", "", query, "查找内容", state.wordReplaceText, "替换为", onAction, actions...)
	} else {
		a.modal.ShowInput("查找和替换", "", query, "查找内容", onAction, actions...)
	}
	a.modal.Input.SetCaret(0, len([]rune(query)))
	a.modal.SearchReplace = true
	a.modal.SearchReplaceHasReplace = canReplace
	a.modal.SearchReplaceReplaceTab = replaceTab
	a.modal.SearchReplaceSummary = "输入查找内容后，点击“查找下一个”"
	a.modal.CardWidthDp = 560
	a.modal.FocusInputRequested = true
}

func (a *App) hideWordSearchReplaceDialog(state *documentSessionState) {
	a.modal.Hide()
	if state != nil {
		state.clearWordSearchHighlights()
	}
	if state == nil || state.word == nil || state.selectedWordRunID == "" {
		return
	}
	run, _, ok := findEditableWordRunInState(state, state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	wordRunEditor(state, run)
	state.wordPendingFocusRunID = run.ID
	state.wordPendingCaretRunID = ""
	state.wordPendingCaretByX = false
}

func wordFindInitialQuery(state *documentSessionState) string {
	if state == nil {
		return ""
	}
	if selected, ok := wordPageSelectedText(state); ok && !strings.ContainsAny(selected, "\r\n") {
		return selected
	}
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		if selected := editor.SelectedText(); selected != "" && !strings.ContainsAny(selected, "\r\n") {
			return selected
		}
	}
	return state.wordFindQuery
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
	results := findWordSearchResults(blocks, query, drafts)
	state.setWordSearchHighlights(query, results.ranges)
	match, found := findWordTextMatchInResults(blocks, results.matches, state.selectedWordRunID, currentEnd)
	if !found {
		a.setStatus("未找到：" + query)
		a.modal.SearchReplaceSummary = "未找到匹配项"
		return
	}
	a.focusWordTextMatch(state, blocks, match)
	a.updateWordSearchSummary(state, results.matches, match)
	a.setStatus("已定位：" + query)
}

func (a *App) findPreviousWordText(query string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || query == "" {
		return
	}
	blocks := wordBlocksForState(state)
	drafts := state.wordDraftSnapshot()
	for runID, editor := range state.wordRunEditors {
		drafts[runID] = editor.Text()
	}
	currentRunID, currentStart := state.selectedWordRunID, 0
	if start, _, active := wordPageOrderedSelection(state.wordPageTextSelection); active {
		currentRunID, currentStart = start.runID, start.runeOffset
	} else if editor := state.wordRunEditors[currentRunID]; editor != nil {
		currentStart, _ = editor.Selection()
	}
	results := findWordSearchResults(blocks, query, drafts)
	state.setWordSearchHighlights(query, results.ranges)
	match, found := findPreviousWordTextMatchInResults(blocks, results.matches, currentRunID, currentStart)
	if !found {
		a.setStatus("未找到：" + query)
		a.modal.SearchReplaceSummary = "未找到匹配项"
		return
	}
	a.focusWordTextMatch(state, blocks, match)
	a.updateWordSearchSummary(state, results.matches, match)
	a.setStatus("已定位：" + query)
}

func findPreviousWordTextMatch(blocks []document.WordBlock, query, currentRunID string, currentStart int, drafts map[string]string) (wordTextMatch, bool) {
	results := findWordSearchResults(blocks, query, drafts)
	return findPreviousWordTextMatchInResults(blocks, results.matches, currentRunID, currentStart)
}

func findPreviousWordTextMatchInResults(blocks []document.WordBlock, matches []wordTextMatch, currentRunID string, currentStart int) (wordTextMatch, bool) {
	if len(matches) == 0 {
		return wordTextMatch{}, false
	}
	if currentRunID == "" {
		return matches[len(matches)-1], true
	}
	runOrder := make(map[string]int)
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if _, exists := runOrder[inline.Text.ID]; !exists {
				runOrder[inline.Text.ID] = len(runOrder)
			}
		}
	}
	currentOrder, exists := runOrder[currentRunID]
	if !exists {
		return matches[len(matches)-1], true
	}
	currentStart = max(0, currentStart)
	for index := len(matches) - 1; index >= 0; index-- {
		match := matches[index]
		matchOrder, exists := runOrder[match.RunID]
		if exists && (matchOrder < currentOrder || (matchOrder == currentOrder && match.Start < currentStart)) {
			return match, true
		}
	}
	return matches[len(matches)-1], true
}

func findAllWordTextMatches(blocks []document.WordBlock, query string, drafts map[string]string) []wordTextMatch {
	return findWordSearchResults(blocks, query, drafts).matches
}

func wordSearchHighlightRanges(blocks []document.WordBlock, query string, drafts map[string]string) map[string][]wordSearchTextRange {
	return findWordSearchResults(blocks, query, drafts).ranges
}

func (state *documentSessionState) setWordSearchHighlights(query string, ranges map[string][]wordSearchTextRange) {
	if state == nil {
		return
	}
	state.wordSearchHighlightQuery = query
	state.wordSearchHighlightRanges = ranges
	state.wordSearchHighlightRevision = state.wordRevisionAtomic.Load()
	state.wordSearchHighlightDraftEpoch = state.wordDraftEpoch.Load()
}

func (state *documentSessionState) clearWordSearchHighlights() {
	if state == nil {
		return
	}
	state.wordSearchHighlightQuery = ""
	state.wordSearchHighlightRanges = nil
	state.wordSearchHighlightRevision = 0
	state.wordSearchHighlightDraftEpoch = 0
}

func (state *documentSessionState) wordSearchHighlightRangesFor(query string) map[string][]wordSearchTextRange {
	if state == nil || query == "" || query != state.wordSearchHighlightQuery {
		return nil
	}
	if state.wordSearchHighlightRevision != state.wordRevisionAtomic.Load() || state.wordSearchHighlightDraftEpoch != state.wordDraftEpoch.Load() {
		state.clearWordSearchHighlights()
		return nil
	}
	return state.wordSearchHighlightRanges
}

func findWordSearchResults(blocks []document.WordBlock, query string, drafts map[string]string) wordSearchResults {
	needle := lowerWordRunes([]rune(query))
	if len(needle) == 0 {
		return wordSearchResults{}
	}
	results := wordSearchResults{ranges: make(map[string][]wordSearchTextRange)}
	for _, chunk := range wordSearchChunks(blocks, drafts) {
		lowered := lowerWordRunes(chunk.text)
		for from := 0; from < len(lowered); {
			start := findWordRunes(lowered, needle, from)
			if start < 0 {
				break
			}
			if match, ok := wordSearchMatchAt(chunk, start, start+len(needle)); ok {
				results.matches = append(results.matches, match)
				appendWordSearchHighlightRanges(results.ranges, chunk, match)
			}
			from = start + len(needle)
		}
	}
	return results
}

func appendWordSearchHighlightRanges(ranges map[string][]wordSearchTextRange, chunk wordSearchChunk, match wordTextMatch) {
	startIndex, endIndex := -1, -1
	endRunID := wordTextMatchEndRunID(match)
	for index, segment := range chunk.segments {
		if segment.run.ID == match.RunID {
			startIndex = index
		}
		if segment.run.ID == endRunID {
			endIndex = index
		}
	}
	if startIndex < 0 || endIndex < startIndex {
		return
	}
	for index := startIndex; index <= endIndex; index++ {
		segment := chunk.segments[index]
		start, end := 0, len(segment.text)
		if index == startIndex {
			start = match.Start
		}
		if index == endIndex {
			end = wordTextMatchEndOffset(match)
		}
		start = max(0, min(start, len(segment.text)))
		end = max(start, min(end, len(segment.text)))
		if start < end {
			ranges[segment.run.ID] = append(ranges[segment.run.ID], wordSearchTextRange{Start: start, End: end})
		}
	}
}

func (a *App) updateWordSearchSummary(state *documentSessionState, matches []wordTextMatch, current wordTextMatch) {
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
	if selected, ok := wordPageSelectedText(state); ok {
		needle := lowerWordRunes([]rune(query))
		if equalWordRunes(lowerWordRunes([]rune(selected)), needle) {
			start, end, active := wordPageOrderedSelection(state.wordPageTextSelection)
			if active {
				paragraphID := wordParagraphIDForRun(blocks, start.runID)
				if paragraphID != "" && paragraphID == wordParagraphIDForRun(blocks, end.runID) {
					return wordTextMatch{
						ParagraphID: paragraphID,
						RunID:       start.runID,
						Start:       start.runeOffset,
						End:         end.runeOffset,
						EndRunID:    end.runID,
						EndOffset:   end.runeOffset,
					}, true
				}
			}
		}
	}
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
	return wordTextMatch{ParagraphID: paragraph.ID, RunID: run.ID, Start: start, End: end, EndRunID: run.ID, EndOffset: end}, true
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
	blocks := wordBlocksForState(state)
	drafts := state.wordDraftSnapshot()
	for runID, editor := range state.wordRunEditors {
		drafts[runID] = editor.Text()
	}
	plan := planWordTextMatchReplacement(blocks, drafts, match, query, replacement)
	if plan.matches == 0 || plan.truncated || len(plan.replacements) == 0 {
		return false
	}
	unchanged := true
	for _, run := range plan.replacements {
		if run.Replacement != plan.inputTexts[run.RunID] {
			unchanged = false
			break
		}
	}
	if unchanged {
		a.setStatus("替换内容与当前文字相同")
		a.modal.SearchReplaceSummary = "替换结果与当前文字相同"
		return true
	}
	batchEditor, canBatch := any(state.word).(document.WordTextBatchEditor)
	rangeEditor, canRange := any(state.word).(document.WordTextRangeEditor)
	selectionEditor, canSelect := any(state.word).(document.WordTextSelectionReplacer)
	endRunID, endOffset := wordTextMatchEndRunID(match), wordTextMatchEndOffset(match)
	if !canBatch {
		for runID, text := range plan.inputTexts {
			run, ok := selectedWordRun(blocks, runID)
			if !ok || run.Text != text {
				a.setStatus("当前文档暂不支持替换未提交的文字草稿")
				a.modal.SearchReplaceSummary = "请先完成当前文字输入，再重试替换"
				return true
			}
		}
		if (match.RunID == endRunID && !canRange) || (match.RunID != endRunID && !canSelect) {
			return false
		}
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return false
	}
	generation := a.documentGeneration
	a.modal.SearchReplaceSummary = "正在替换当前匹配项…"
	started := a.launchManagedBackground(func() {
		var changed bool
		updatedBlocks, err := runWordMutation(state, func() error {
			if canBatch {
				var replaceErr error
				changed, replaceErr = batchEditor.ReplaceTextRuns(plan.replacements)
				return replaceErr
			}
			if match.RunID == endRunID {
				changed = true
				return rangeEditor.ReplaceTextRange(match.RunID, match.Start, endOffset, replacement)
			}
			changed = true
			_, replaceErr := selectionEditor.ReplaceTextSelection(match.RunID, match.Start, endRunID, endOffset, replacement)
			return replaceErr
		})
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, generation: generation, operation: "替换文字", err: err, blocks: updatedBlocks, applied: changed,
			wordRangeReplacement: true, wordRangeRunIDs: plan.runIDs, wordRangeInputTexts: plan.inputTexts,
			replaceRunID: match.RunID, replaceParagraphID: match.ParagraphID, replaceStart: match.Start,
			replaceCaret: match.Start + len([]rune(replacement)),
			replaceText:  replacement, replaceQuery: query,
		})
	})
	if !started {
		state.finishOperation(token)
		a.setStatus("替换任务无法启动")
		a.modal.SearchReplaceSummary = "替换任务无法启动"
		return false
	}
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
	if !result.applied {
		a.setStatus("替换结果与当前文字相同")
		if a.modal.Visible && a.modal.SearchReplace {
			a.modal.SearchReplaceSummary = "替换结果与当前文字相同"
		}
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	for _, runID := range result.wordRangeRunIDs {
		run, ok := selectedWordRun(wordBlocksForState(state), runID)
		if !ok || !run.Editable {
			continue
		}
		expected := result.wordRangeInputTexts[runID]
		state.setCommittedWordTextAndClearExpectedDraft(runID, run.Text, expected)
		if newer, exists := state.wordDraft(runID); exists && newer != expected {
			continue
		}
		runEditor := wordRunEditor(state, run)
		if runEditor.Text() == expected {
			runEditor.SetText(run.Text)
		}
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, result.replaceRunID)
	if !ok || !run.Editable {
		a.setStatus("文字已替换，但无法定位编辑位置")
		return
	}
	editor := wordRunEditor(state, run)
	if len(result.wordRangeRunIDs) == 0 {
		state.setCommittedWordTextAndClearDraft(run.ID, run.Text)
		editor.SetText(run.Text)
	}
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
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
	startAfter := result.replaceCaret
	if startAfter == 0 {
		startAfter = result.replaceStart + len([]rune(result.replaceText))
	}
	drafts := state.wordDraftSnapshot()
	for runID, runEditor := range state.wordRunEditors {
		drafts[runID] = runEditor.Text()
	}
	searchResults := findWordSearchResults(blocks, result.replaceQuery, drafts)
	state.setWordSearchHighlights(result.replaceQuery, searchResults.ranges)
	match, found := findWordTextMatchInResults(blocks, searchResults.matches, run.ID, startAfter)
	if found {
		a.focusWordTextMatch(state, blocks, match)
		a.setStatus("已替换，并已定位下一处")
		if a.modal.Visible && a.modal.SearchReplace {
			a.updateWordSearchSummary(state, searchResults.matches, match)
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
	endRunID := wordTextMatchEndRunID(match)
	endOffset := wordTextMatchEndOffset(match)
	focusRunID := run.ID
	if endRunID != run.ID {
		startRunID, start := run.ID, match.Start
		end := endOffset
		if _, err := a.setWordTextSelection(state, wordTextSelectionRequest{
			Scope: "range", StartRunID: &startRunID, StartOffset: &start, EndRunID: &endRunID, EndOffset: &end,
		}); err != nil {
			return
		}
		focusRunID = endRunID
	} else {
		state.wordPageTextSelection = nil
		editor := wordRunEditor(state, run)
		editor.SetCaret(match.Start, endOffset)
	}
	state.selectedWordRunID = focusRunID
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	if state.wordToolbarTab == wordTabPicture {
		state.wordToolbarTab = wordTabHome
	}
	state.wordPendingFocusRunID = focusRunID
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
