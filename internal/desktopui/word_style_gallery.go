package desktopui

import (
	"strings"

	"glance/internal/document"
)

func wordStyleGalleryActions() []ribbonAction {
	return []ribbonAction{
		{id: "style-body", label: "正文", group: "样式"},
		{id: "style-heading", label: "标题 1", group: "样式"},
		{id: "style-heading-2", label: "标题 2", group: "样式"},
	}
}

func wordParagraphStyleID(actionID string) string {
	want := map[string]string{
		"style-body":      "Normal",
		"style-heading":   "Heading1",
		"style-heading-2": "Heading2",
	}[actionID]
	return want
}

func wordParagraphIDsForSelection(state *documentSessionState) []string {
	if state == nil || state.word == nil {
		return nil
	}
	blocks := wordBlocksForState(state)
	if selection := state.wordPageTextSelection; wordPageTextSelectionActive(selection) {
		start, end, ok := wordPageOrderedSelection(selection)
		startOrder, startOK := selection.runOrder[start.runID]
		endOrder, endOK := selection.runOrder[end.runID]
		if ok && startOK && endOK {
			var selected []string
			for _, paragraph := range allWordParagraphs(blocks) {
				include := false
				for _, inline := range paragraph.Inlines {
					run := inline.Text
					if run == nil || !run.Editable {
						continue
					}
					order, found := selection.runOrder[run.ID]
					if !found || order < startOrder || order > endOrder {
						continue
					}
					from, to := 0, len([]rune(run.Text))
					if run.ID == start.runID {
						from = start.runeOffset
					}
					if run.ID == end.runID {
						to = end.runeOffset
					}
					if from < to {
						include = true
						break
					}
				}
				if include {
					selected = append(selected, paragraph.ID)
				}
			}
			if len(selected) > 0 {
				return selected
			}
		}
	}
	if _, ok := selectedWordParagraph(blocks, state.selectedWordParagraphID); ok {
		return []string{state.selectedWordParagraphID}
	}
	if paragraph, ok := selectedWordParagraphForRun(blocks, state.selectedWordRunID); ok {
		return []string{paragraph.ID}
	}
	return nil
}

func wordParagraphStyleActionActive(state *documentSessionState, actionID string) bool {
	paragraphIDs := wordParagraphIDsForSelection(state)
	if len(paragraphIDs) == 0 {
		return false
	}
	want := wordParagraphStyleID(actionID)
	if want == "" {
		return false
	}
	blocks := wordBlocksForState(state)
	for _, paragraphID := range paragraphIDs {
		paragraph, ok := selectedWordParagraph(blocks, paragraphID)
		if !ok {
			return false
		}
		styleID := strings.TrimSpace(paragraph.Style)
		if styleID == "" {
			styleID = "Normal"
		}
		if !strings.EqualFold(styleID, want) {
			return false
		}
	}
	return true
}

func (a *App) applyWordParagraphStyleAction(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraphIDs := wordParagraphIDsForSelection(state)
	if len(paragraphIDs) == 0 {
		a.setStatus("请先将光标放在可编辑的 Word 段落中")
		return
	}
	styleID := wordParagraphStyleID(actionID)
	if styleID == "" {
		return
	}
	if wordHasPendingTextDrafts(state) {
		if state.wordDeferredToolbarAction == "" {
			state.wordDeferredToolbarAction = actionID
		}
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.startNextWordTextCommitFor(state)
		return
	}
	a.runWordParagraphStyleCommand(paragraphIDs, styleID)
}

func (a *App) runWordParagraphStyleCommand(paragraphIDs []string, styleID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordParagraphStyleEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	paragraphIDs = append([]string(nil), paragraphIDs...)
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error {
			return editor.SetParagraphStyles(paragraphIDs, styleID)
		})
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "段落样式", err: err, blocks: blocks})
	})
}
