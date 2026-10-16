package desktopui

import (
	"fmt"
	"sort"

	"glance/internal/document"
)

const wordReplaceAllMaxMatches = 10000
const wordReplaceAllMaxEditedRuns = 10000
const wordReplaceAllMaxExpandedBytes = 64 << 20

type wordReplaceAllPlan struct {
	replacements []document.WordTextRunReplacement
	runIDs       []string
	inputTexts   map[string]string
	matches      int
	outputBytes  int
	truncated    bool
}

type wordReplaceAllResult struct {
	matches    int
	truncated  bool
	runIDs     []string
	inputTexts map[string]string
}

func planWordTextReplaceAll(blocks []document.WordBlock, drafts map[string]string, query, replacement string) wordReplaceAllPlan {
	plan := wordReplaceAllPlan{}
	for _, chunk := range wordSearchChunks(blocks, drafts) {
		remaining := wordReplaceAllMaxMatches - plan.matches
		matches, exceeded := wordMatchesInChunk(chunk, query, remaining)
		if exceeded {
			return wordReplaceAllPlan{matches: plan.matches + len(matches), truncated: true}
		}
		if len(matches) == 0 {
			continue
		}
		updated, outputBytes, truncated := replaceWordTextMatchesInChunk(chunk, matches, replacement, wordReplaceAllMaxExpandedBytes-plan.outputBytes)
		if truncated {
			return wordReplaceAllPlan{matches: plan.matches + len(matches), truncated: true}
		}
		plan.matches += len(matches)
		plan.outputBytes += outputBytes
		appendWordTextRunUpdates(&plan, chunk, updated)
		if len(plan.replacements) > wordReplaceAllMaxEditedRuns {
			return wordReplaceAllPlan{matches: plan.matches, truncated: true}
		}
	}
	return plan
}

func planWordTextMatchReplacement(blocks []document.WordBlock, drafts map[string]string, match wordTextMatch, query, replacement string) wordReplaceAllPlan {
	needle := lowerWordRunes([]rune(query))
	if len(needle) == 0 {
		return wordReplaceAllPlan{}
	}
	endRunID, endOffset := wordTextMatchEndRunID(match), wordTextMatchEndOffset(match)
	for _, chunk := range wordSearchChunks(blocks, drafts) {
		if chunk.paragraphID != match.ParagraphID {
			continue
		}
		start, startOK := wordSearchChunkOffset(chunk, match.RunID, match.Start)
		end, endOK := wordSearchChunkOffset(chunk, endRunID, endOffset)
		if !startOK || !endOK || start >= end || end-start != len(needle) || !equalWordRunes(lowerWordRunes(chunk.text[start:end]), needle) {
			continue
		}
		plan := wordReplaceAllPlan{matches: 1}
		updated, outputBytes, truncated := replaceWordTextMatchesInChunk(chunk, []wordTextMatch{match}, replacement, wordReplaceAllMaxExpandedBytes)
		if truncated {
			plan.truncated = true
			return plan
		}
		if len(updated) == 0 {
			updated = make(map[string]string)
			startIndex, endIndex := -1, -1
			for index, segment := range chunk.segments {
				if segment.run.ID == match.RunID {
					startIndex = index
				}
				if segment.run.ID == endRunID {
					endIndex = index
				}
			}
			for index := startIndex; startIndex >= 0 && index <= endIndex; index++ {
				segment := chunk.segments[index]
				updated[segment.run.ID] = string(segment.text)
			}
		}
		plan.outputBytes = outputBytes
		appendWordTextRunUpdates(&plan, chunk, updated)
		if len(plan.replacements) > wordReplaceAllMaxEditedRuns {
			plan.truncated = true
		}
		return plan
	}
	return wordReplaceAllPlan{}
}

func appendWordTextRunUpdates(plan *wordReplaceAllPlan, chunk wordSearchChunk, updated map[string]string) {
	for _, segment := range chunk.segments {
		value, changed := updated[segment.run.ID]
		if !changed {
			continue
		}
		if plan.inputTexts == nil {
			plan.inputTexts = make(map[string]string)
		}
		plan.replacements = append(plan.replacements, document.WordTextRunReplacement{
			RunID: segment.run.ID, ExpectedText: segment.run.Text, Replacement: value,
		})
		plan.runIDs = append(plan.runIDs, segment.run.ID)
		plan.inputTexts[segment.run.ID] = string(segment.text)
	}
}

func wordMatchesInChunk(chunk wordSearchChunk, query string, limit int) ([]wordTextMatch, bool) {
	needle := lowerWordRunes([]rune(query))
	if len(needle) == 0 {
		return nil, false
	}
	lowered := lowerWordRunes(chunk.text)
	var matches []wordTextMatch
	for from := 0; from < len(lowered); {
		start := findWordRunes(lowered, needle, from)
		if start < 0 {
			break
		}
		if match, ok := wordSearchMatchAt(chunk, start, start+len(needle)); ok {
			matches = append(matches, match)
			if limit >= 0 && len(matches) > limit {
				return matches, true
			}
		}
		from = start + len(needle)
	}
	return matches, false
}

type wordTextRunEdit struct {
	start  int
	end    int
	insert []rune
}

func wordSearchChunkOffset(chunk wordSearchChunk, runID string, offset int) (int, bool) {
	for _, segment := range chunk.segments {
		if segment.run.ID != runID {
			continue
		}
		if offset < 0 || offset > len(segment.text) {
			return 0, false
		}
		return segment.start + offset, true
	}
	return 0, false
}

func replaceWordTextMatchesInChunk(chunk wordSearchChunk, matches []wordTextMatch, replacement string, maxOutputBytes int) (map[string]string, int, bool) {
	segments := make(map[string]wordSearchSegment, len(chunk.segments))
	segmentIndexes := make(map[string]int, len(chunk.segments))
	for index, segment := range chunk.segments {
		segments[segment.run.ID] = segment
		segmentIndexes[segment.run.ID] = index
	}
	editByRun := make(map[string][]wordTextRunEdit)
	replacementRunes := []rune(replacement)
	for _, match := range matches {
		endRunID := wordTextMatchEndRunID(match)
		endOffset := wordTextMatchEndOffset(match)
		startSegment, startExists := segments[match.RunID]
		endSegment, endExists := segments[endRunID]
		startIndex, startIndexExists := segmentIndexes[match.RunID]
		endIndex, endIndexExists := segmentIndexes[endRunID]
		if !startExists || !endExists || !startIndexExists || !endIndexExists || startIndex > endIndex || match.Start < 0 || match.Start > len(startSegment.text) || endOffset < 0 || endOffset > len(endSegment.text) {
			continue
		}
		startGlobal := startSegment.start + match.Start
		endGlobal := endSegment.start + endOffset
		if startGlobal >= endGlobal || endGlobal > len(chunk.text) || string(chunk.text[startGlobal:endGlobal]) == replacement {
			continue
		}
		if startIndex == endIndex {
			editByRun[match.RunID] = append(editByRun[match.RunID], wordTextRunEdit{start: match.Start, end: endOffset, insert: replacementRunes})
			continue
		}
		editByRun[match.RunID] = append(editByRun[match.RunID], wordTextRunEdit{start: match.Start, end: len(startSegment.text), insert: replacementRunes})
		for index := startIndex + 1; index < endIndex; index++ {
			segment := chunk.segments[index]
			editByRun[segment.run.ID] = append(editByRun[segment.run.ID], wordTextRunEdit{start: 0, end: len(segment.text)})
		}
		editByRun[endRunID] = append(editByRun[endRunID], wordTextRunEdit{start: 0, end: endOffset})
	}
	if len(editByRun) == 0 {
		return nil, 0, false
	}
	updated := make(map[string]string, len(editByRun))
	outputBytes := 0
	for _, segment := range chunk.segments {
		edits := editByRun[segment.run.ID]
		if len(edits) == 0 {
			continue
		}
		sort.Slice(edits, func(i, j int) bool {
			if edits[i].start == edits[j].start {
				return edits[i].end > edits[j].end
			}
			return edits[i].start > edits[j].start
		})
		value := append([]rune(nil), segment.text...)
		for _, edit := range edits {
			if edit.start < 0 || edit.end < edit.start || edit.end > len(value) {
				return nil, outputBytes, true
			}
			next := make([]rune, 0, len(value)-(edit.end-edit.start)+len(edit.insert))
			next = append(next, value[:edit.start]...)
			next = append(next, edit.insert...)
			next = append(next, value[edit.end:]...)
			value = next
		}
		text := string(value)
		if text == string(segment.text) {
			continue
		}
		if len(text) > maxOutputBytes-outputBytes {
			return nil, outputBytes, true
		}
		outputBytes += len(text)
		updated[segment.run.ID] = text
	}
	return updated, outputBytes, false
}

func (a *App) replaceAllWordText(query, replacement string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTextBatchEditor)
	if !ok {
		a.setStatus("当前 DOCX 暂不支持安全的全部替换")
		a.modal.SearchReplaceSummary = "当前 DOCX 暂不支持全部替换"
		return
	}
	if a.wordCommandResults == nil {
		a.setStatus("替换服务不可用")
		a.modal.SearchReplaceSummary = "替换服务不可用"
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("文档正在处理其他操作")
		a.modal.SearchReplaceSummary = "文档正在处理其他操作"
		return
	}
	blocks := wordBlocksForState(state)
	drafts := state.wordDraftSnapshot()
	for runID, editor := range state.wordRunEditors {
		drafts[runID] = editor.Text()
	}
	generation := a.documentGeneration
	a.modal.SearchReplaceSummary = "正在扫描并替换当前文档…"
	if !a.launchManagedBackground(func() {
		plan := planWordTextReplaceAll(blocks, drafts, query, replacement)
		var changed bool
		var updatedBlocks []document.WordBlock
		var err error
		if !plan.truncated && len(plan.replacements) > 0 {
			updatedBlocks, err = runWordMutation(state, func() error {
				var replaceErr error
				changed, replaceErr = editor.ReplaceTextRuns(plan.replacements)
				return replaceErr
			})
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, generation: generation, operation: "全部替换",
			err: err, blocks: updatedBlocks, applied: changed,
			wordReplaceAll: &wordReplaceAllResult{
				matches: plan.matches, truncated: plan.truncated, runIDs: plan.runIDs, inputTexts: plan.inputTexts,
			},
		})
	}) {
		state.finishOperation(token)
		const message = "替换任务无法启动"
		a.setStatus(message)
		a.modal.SearchReplaceSummary = message
	}
}

func (a *App) completeWordTextReplaceAll(result wordCommandResult, active bool) {
	state := result.state
	if state == nil || result.wordReplaceAll == nil {
		return
	}
	operation := result.wordReplaceAll
	message := ""
	switch {
	case result.err != nil:
		message = "全部替换失败：" + result.err.Error()
	case operation.truncated:
		message = fmt.Sprintf("匹配或受影响文字运行超过 %d 个，或替换后文字超过 64 MB；未修改，请缩小查找范围后重试", wordReplaceAllMaxMatches)
	case operation.matches == 0:
		message = "未找到匹配项"
	case !result.applied:
		message = "替换结果与当前文本相同"
	default:
		blocks := wordBlocksForState(state)
		for _, runID := range operation.runIDs {
			run, ok := selectedWordRun(blocks, runID)
			if !ok || !run.Editable {
				continue
			}
			expectedText := operation.inputTexts[run.ID]
			state.setCommittedWordTextAndClearExpectedDraft(run.ID, run.Text, expectedText)
			runEditor := wordRunEditor(state, run)
			if runEditor.Text() == expectedText {
				if _, hasNewerDraft := state.wordDraft(run.ID); hasNewerDraft {
					continue
				}
				runEditor.SetText(run.Text)
			}
		}
		reconcileWordSelection(state, false)
		message = fmt.Sprintf("已替换 %d 处", operation.matches)
	}
	if active {
		a.setStatus(message)
		if a.modal.Visible && a.modal.SearchReplace {
			a.modal.SearchReplaceSummary = message
			a.modal.FocusInputRequested = true
		}
		if result.err == nil && result.applied {
			a.persistActiveSession()
			if !state.formatRunning.Load() && !state.wordFlushRequested.Load() {
				a.maybeAutoSaveWord(state)
			}
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
