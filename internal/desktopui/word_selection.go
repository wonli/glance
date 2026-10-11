package desktopui

import (
	"errors"

	"glance/internal/application"
	"glance/internal/document"
)

type wordTextSelectionRequest struct {
	Scope       string  `json:"scope"`
	StartRunID  *string `json:"start_run_id"`
	StartOffset *int    `json:"start_offset"`
	EndRunID    *string `json:"end_run_id"`
	EndOffset   *int    `json:"end_offset"`
}

func (a *App) setWordTextSelection(state *documentSessionState, request wordTextSelectionRequest) (bool, error) {
	if state == nil || state.word == nil {
		return false, application.ErrCapabilityUnsupported
	}
	paragraphs := allWordParagraphs(wordBlocksForState(state))
	type editableRun struct {
		paragraphID string
		run         document.WordTextRun
		text        string
	}
	runs := make([]editableRun, 0)
	runOrder := make(map[string]int)
	for _, paragraph := range paragraphs {
		for _, inline := range paragraph.Inlines {
			if inline.Kind != document.WordTextInline || inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if _, exists := runOrder[inline.Text.ID]; exists {
				continue
			}
			text := inline.Text.Text
			if editor := state.wordRunEditors[inline.Text.ID]; editor != nil {
				text = editor.Text()
			} else if draft, exists := state.wordDraft(inline.Text.ID); exists {
				text = draft
			}
			runOrder[inline.Text.ID] = len(runs)
			runs = append(runs, editableRun{paragraphID: paragraph.ID, run: *inline.Text, text: text})
		}
	}
	if request.Scope == "clear" {
		changed := state.wordPageTextSelection != nil
		state.wordPageTextSelection = nil
		return changed, nil
	}
	var startRunID, endRunID string
	var startOffset, endOffset int
	switch request.Scope {
	case "document":
		if len(runs) == 0 {
			return false, errors.New("当前文档没有可选择的正文文字")
		}
		startRunID, endRunID = runs[0].run.ID, runs[len(runs)-1].run.ID
		endOffset = len([]rune(runs[len(runs)-1].text))
	case "range":
		if request.StartRunID == nil || request.StartOffset == nil || request.EndRunID == nil || request.EndOffset == nil || *request.StartRunID == "" || *request.EndRunID == "" || *request.StartOffset < 0 || *request.EndOffset < 0 {
			return false, application.ErrInvalidCommandArguments
		}
		startRunID, startOffset = *request.StartRunID, *request.StartOffset
		endRunID, endOffset = *request.EndRunID, *request.EndOffset
	default:
		return false, application.ErrInvalidCommandArguments
	}
	startIndex, startExists := runOrder[startRunID]
	endIndex, endExists := runOrder[endRunID]
	if !startExists || !endExists || startIndex > endIndex || startOffset > len([]rune(runs[startIndex].text)) || endOffset > len([]rune(runs[endIndex].text)) {
		return false, application.ErrInvalidCommandArguments
	}
	selection := &wordPageTextSelection{
		scope:    request.Scope,
		anchor:   wordPageCaretPosition{runID: startRunID, runeOffset: startOffset},
		focus:    wordPageCaretPosition{runID: endRunID, runeOffset: endOffset},
		runOrder: runOrder,
	}
	previous := wordSelectionSnapshot(state)
	previousRunID, previousParagraphID := state.selectedWordRunID, state.selectedWordParagraphID
	state.wordPageTextSelection = selection
	state.selectedWordRunID = endRunID
	state.selectedWordParagraphID = runs[endIndex].paragraphID
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.wordPendingFocusRunID = endRunID
	state.wordPendingCaretRunID = endRunID
	state.wordPendingCaretOffset = endOffset
	state.wordPendingCaretByX = false
	state.wordLayoutFocusedRunID = endRunID
	if editor := state.wordRunEditors[endRunID]; editor != nil {
		if startRunID == endRunID {
			editor.SetCaret(startOffset, endOffset)
		} else {
			editor.SetCaret(endOffset, endOffset)
		}
	}
	current := wordSelectionSnapshot(state)
	return previous != current || previousRunID != state.selectedWordRunID || previousParagraphID != state.selectedWordParagraphID, nil
}

type wordSelectionSnapshotData struct {
	Scope         string `json:"scope"`
	StartRunID    string `json:"start_run_id"`
	StartOffset   int    `json:"start_offset"`
	EndRunID      string `json:"end_run_id"`
	EndOffset     int    `json:"end_offset"`
	SelectedRunes int    `json:"selected_runes"`
}

func wordSelectionSnapshot(state *documentSessionState) wordSelectionSnapshotData {
	snapshot := wordSelectionSnapshotData{Scope: "none"}
	if state == nil || state.wordPageTextSelection == nil {
		return snapshot
	}
	selection := state.wordPageTextSelection
	start, end, active := wordPageOrderedSelection(selection)
	if !active {
		start, end = selection.anchor, selection.focus
	}
	snapshot.Scope = selection.scope
	if snapshot.Scope == "" {
		snapshot.Scope = "range"
	}
	snapshot.StartRunID, snapshot.StartOffset = start.runID, start.runeOffset
	snapshot.EndRunID, snapshot.EndOffset = end.runID, end.runeOffset
	if selected, ok := wordPageSelectedText(state); ok {
		snapshot.SelectedRunes = len([]rune(selected))
	}
	return snapshot
}

func (a *App) selectAllWordText() {
	state := a.documentSessionState
	if state == nil || len(state.wordBlocks) == 0 {
		return
	}
	changed, err := a.setWordTextSelection(state, wordTextSelectionRequest{Scope: "document"})
	if err != nil {
		a.setStatus("当前文档无文字内容")
		return
	}
	a.setStatus("已全选文档正文")
	if changed && a.window != nil {
		a.window.Invalidate()
	}
}
