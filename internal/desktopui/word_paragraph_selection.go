package desktopui

import (
	"unicode"

	"gioui.org/layout"
	"glance/internal/document"
)

type wordParagraphSelectionRun struct {
	run   document.WordTextRun
	start int
	text  []rune
}

// selectWordPageTextSegment selects the text around a double-click position.
// Formatting runs remain transparent to the selection; punctuation and line
// breaks split the selected segment within an OOXML paragraph.
func (a *App) selectWordPageTextSegment(gtx layout.Context, state *documentSessionState, runID string, runeOffset int) bool {
	if state == nil {
		return false
	}
	paragraph, ok := selectedWordParagraphForRun(wordBlocksForState(state), runID)
	if !ok {
		return false
	}
	runs, text := wordParagraphSelectionRuns(state, paragraph)
	if len(text) == 0 {
		return false
	}

	globalOffset, found := 0, false
	for _, run := range runs {
		if run.run.ID != runID {
			continue
		}
		globalOffset = run.start + max(0, min(runeOffset, len(run.text)))
		found = true
		break
	}
	if !found {
		return false
	}
	start, end, ok := wordDoubleClickTextRange(text, globalOffset)
	if !ok {
		return false
	}
	return a.selectWordParagraphRange(gtx, state, paragraph, runs, start, end)
}

func wordParagraphSelectionRuns(state *documentSessionState, paragraph document.WordParagraph) ([]wordParagraphSelectionRun, []rune) {
	runs := make([]wordParagraphSelectionRun, 0, len(paragraph.Inlines))
	var text []rune
	for _, inline := range paragraph.Inlines {
		run := inline.Text
		if inline.Kind != document.WordTextInline || run == nil || !run.Editable {
			continue
		}
		value := run.Text
		if editor := state.wordRunEditors[run.ID]; editor != nil {
			value = editor.Text()
		} else if draft, exists := state.wordDraft(run.ID); exists {
			value = draft
		}
		runes := []rune(value)
		runs = append(runs, wordParagraphSelectionRun{run: *run, start: len(text), text: runes})
		text = append(text, runes...)
	}
	return runs, text
}

func wordDoubleClickTextRange(text []rune, caret int) (int, int, bool) {
	if len(text) == 0 {
		return 0, 0, false
	}
	caret = max(0, min(caret, len(text)))
	boundary := func(r rune) bool { return r == '\n' || unicode.IsPunct(r) }
	if caret < len(text) && boundary(text[caret]) {
		return caret, caret + 1, true
	}
	start, end := caret, caret
	for start > 0 && !boundary(text[start-1]) {
		start--
	}
	for end < len(text) && !boundary(text[end]) {
		end++
	}
	if start == end {
		// A caret at the end of a paragraph after punctuation still represents
		// a double-click on its final visible punctuation mark.
		if caret > 0 {
			return caret - 1, caret, true
		}
		return 0, 0, false
	}
	return start, end, true
}

func (a *App) selectWordParagraphRange(gtx layout.Context, state *documentSessionState, paragraph document.WordParagraph, runs []wordParagraphSelectionRun, start, end int) bool {
	if len(runs) == 0 || start < 0 || end <= start {
		return false
	}
	startPosition := wordParagraphSelectionPosition(runs, start, false)
	endPosition := wordParagraphSelectionPosition(runs, end, true)
	if startPosition.runID == "" || endPosition.runID == "" {
		return false
	}

	a.focusWordPageRun(gtx, state, startPosition.runID, startPosition.runeOffset, startPosition.runeOffset)
	runOrder := make(map[string]int, len(runs))
	for index, run := range runs {
		runOrder[run.run.ID] = index
		from := max(start, run.start)
		to := min(end, run.start+len(run.text))
		if from >= to {
			continue
		}
		wordRunEditor(state, run.run).SetCaret(from-run.start, to-run.start)
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "range",
		anchor:   startPosition,
		focus:    endPosition,
		runOrder: runOrder,
	}
	state.selectedWordParagraphID = paragraph.ID
	state.wordPageSelectionDrag = nil
	return true
}

func wordParagraphSelectionPosition(runs []wordParagraphSelectionRun, offset int, preferPrevious bool) wordPageCaretPosition {
	var previous *wordParagraphSelectionRun
	for index := range runs {
		run := &runs[index]
		runEnd := run.start + len(run.text)
		if len(run.text) == 0 {
			continue
		}
		if offset < run.start {
			break
		}
		if offset == run.start && preferPrevious && previous != nil {
			return wordPageCaretPosition{runID: previous.run.ID, runeOffset: len(previous.text)}
		}
		if offset < runEnd {
			return wordPageCaretPosition{runID: run.run.ID, runeOffset: offset - run.start}
		}
		previous = run
	}
	if previous != nil {
		return wordPageCaretPosition{runID: previous.run.ID, runeOffset: len(previous.text)}
	}
	return wordPageCaretPosition{}
}
