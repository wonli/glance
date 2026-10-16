package desktopui

import (
	"gioui.org/layout"
	"glance/internal/document"
)

// selectWordPageParagraph selects all editable text, including formatted runs
// and wrapped visual lines, while leaving adjacent paragraphs out of the range.
func (a *App) selectWordPageParagraph(gtx layout.Context, state *documentSessionState, runID string) bool {
	paragraph, ok := selectedWordParagraphForRun(wordBlocksForState(state), runID)
	if !ok {
		return false
	}
	var runs []document.WordTextRun
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.Editable {
			runs = append(runs, *inline.Text)
		}
	}
	if len(runs) == 0 {
		return false
	}
	first, last := runs[0], runs[len(runs)-1]
	end := wordRunEditor(state, last).Len()
	a.focusWordPageRun(gtx, state, first.ID, 0, 0)
	state.wordPageTextSelection = &wordPageTextSelection{scope: "range", anchor: wordPageCaretPosition{runID: first.ID}, focus: wordPageCaretPosition{runID: last.ID, runeOffset: end}}
	for _, run := range runs {
		editor := wordRunEditor(state, run)
		editor.SetCaret(editor.Len(), 0)
	}
	if first.ID != last.ID {
		state.wordPageTextSelection.runOrder = make(map[string]int, len(runs))
		for i, r := range runs {
			state.wordPageTextSelection.runOrder[r.ID] = i
		}
	}
	state.wordPageSelectionDrag = nil
	return true
}
