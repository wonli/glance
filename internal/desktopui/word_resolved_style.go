package desktopui

import (
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
	"unicode"
)

// The accepted scene includes paragraph/default/character inheritance. Apply
// newer direct properties over it while an edited scene is being repaginated.
func wordRunWithResolvedStyle(state *documentSessionState, run document.WordTextRun) (document.WordTextRun, bool) {
	if state == nil || state.wordLayoutResult == nil {
		return run, false
	}
	for _, page := range state.wordLayoutResult.Pages {
		for _, fragment := range page.Text {
			if fragment.RunID == run.ID {
				run.Style = wordlayout.MergeTextStyle(fragment.Style, run.Style, run.StyleSet)
				return run, true
			}
		}
	}
	return run, false
}

func selectedWordFormattingRun(state *documentSessionState, runID string) (document.WordTextRun, bool) {
	if state == nil {
		return document.WordTextRun{}, false
	}
	state.wordBlocksMu.RLock()
	run, ok := selectedWordRun(state.wordBlocks, runID)
	state.wordBlocksMu.RUnlock()
	if !ok {
		run, ok = selectedWordRun(wordBlocksForState(state), runID)
	}
	if ok {
		run, _ = wordRunWithResolvedStyle(state, run)
	}
	return run, ok
}

func wordRunLabelForState(theme *material.Theme, state *documentSessionState, paragraph document.WordParagraph, run document.WordTextRun) material.LabelStyle {
	if resolved, ok := wordRunWithResolvedStyle(state, run); ok {
		run = resolved
		run.Style.FontFamily = wordRunDisplayFamily(run)
		// Resolved scene typography already includes the heading style.
		paragraph.Style = ""
	}
	return wordRunLabel(theme, paragraph, run)
}

func wordRunDisplayFamily(run document.WordTextRun) string {
	for _, r := range run.Text {
		if !unicode.IsSpace(r) {
			return wordfont.FamilyForRune(run.Style, r)
		}
	}
	return run.Style.FontFamily
}
