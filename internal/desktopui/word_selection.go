package desktopui

import "glance/internal/document"

func (a *App) selectAllWordText() {
	state := a.documentSessionState
	if state == nil || len(state.wordBlocks) == 0 {
		return
	}
	paragraphs := allWordParagraphs(wordBlocksForState(state))
	var firstRunID, lastRunID string
	var lastRunLen int

	for _, p := range paragraphs {
		for _, in := range p.Inlines {
			if in.Kind == document.WordTextInline && in.Text != nil && in.Text.Editable {
				if firstRunID == "" {
					firstRunID = in.Text.ID
				}
				lastRunID = in.Text.ID
				text := in.Text.Text
				if ed := state.wordRunEditors[in.Text.ID]; ed != nil {
					text = ed.Text()
				}
				lastRunLen = len([]rune(text))
			}
		}
	}
	if firstRunID == "" {
		a.setStatus("当前文档无文字内容")
		return
	}
	order := make(map[string]int)
	for _, p := range paragraphs {
		for _, in := range p.Inlines {
			if in.Kind == document.WordTextInline && in.Text != nil && in.Text.Editable {
				if _, exists := order[in.Text.ID]; !exists {
					order[in.Text.ID] = len(order)
				}
			}
		}
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		anchor:   wordPageCaretPosition{runID: firstRunID, runeOffset: 0},
		focus:    wordPageCaretPosition{runID: lastRunID, runeOffset: lastRunLen},
		runOrder: order,
	}
	populateWordPageRunOrder(state.wordPageTextSelection, state.wordLayoutResult)
	if state.selectedWordRunID != "" {
		if ed := state.wordRunEditors[state.selectedWordRunID]; ed != nil {
			ed.SetCaret(0, len([]rune(ed.Text())))
		}
	}
	a.setStatus("已全选文档正文")
	if a.window != nil {
		a.window.Invalidate()
	}
}
