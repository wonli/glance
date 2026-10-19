package desktopui

import (
	"glance/internal/document"
	"glance/internal/wordlayout"
)

// The ruler keeps paragraph context independently of text focus/selection.
// Its initial context is the first visible paragraph, without focusing an editor.
type wordRulerContext struct {
	paragraphID string
	runID       string
	offset      int
}

func wordRulerContextForState(state *documentSessionState) (wordRulerContext, document.WordParagraph, bool) {
	blocks := wordBlocksForState(state)
	if p, ok := selectedWordParagraph(blocks, state.selectedWordParagraphID); ok {
		context := wordRulerContext{paragraphID: p.ID, runID: state.selectedWordRunID}
		if context.runID != "" {
			context.offset = wordPageCaretOffset(state, context.runID)
		} else if state.wordRulerContext.paragraphID == p.ID {
			context = state.wordRulerContext
		}
		return context, p, true
	}
	if p, ok := selectedWordParagraph(blocks, state.wordRulerContext.paragraphID); ok {
		return state.wordRulerContext, p, true
	}
	if result := state.wordLayoutResult; result != nil && len(result.Pages) > 0 {
		columns := 1
		if state.wordViewMode == wordViewPrintLayout && state.wordMultiplePagesMode {
			columns = 2
		}
		index := max(0, min(state.wordPageListState.Position.First*columns, len(result.Pages)-1))
		for _, fragment := range result.Pages[index].Text {
			if p, ok := selectedWordParagraphForRun(blocks, fragment.RunID); ok {
				return wordRulerContext{paragraphID: p.ID, runID: fragment.RunID, offset: fragment.RuneStart}, p, true
			}
		}
	}
	for _, p := range allWordParagraphs(blocks) {
		return wordRulerContext{paragraphID: p.ID}, p, true
	}
	return wordRulerContext{}, document.WordParagraph{}, false
}

func wordRulerParagraphForState(state *documentSessionState) (document.WordParagraph, bool) {
	_, p, ok := wordRulerContextForState(state)
	return p, ok
}

func wordRulerSourcePosition(state *documentSessionState) wordlayout.SourcePosition {
	if state.selectedWordRunID != "" {
		return wordlayout.SourcePosition{RunID: state.selectedWordRunID, RuneOffset: wordPageCaretOffset(state, state.selectedWordRunID)}
	}
	context, _, _ := wordRulerContextForState(state)
	return wordlayout.SourcePosition{RunID: context.runID, RuneOffset: context.offset}
}
