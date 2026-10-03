package desktopui

import (
	"unicode/utf8"

	"gioui.org/layout"
	"glance/internal/document"
)

// windowViewSnapshot contains presentation state copied into another window.
// Gio widgets and transient interactions are deliberately not transferable.
type windowViewSnapshot struct {
	activeTabID                string
	wordViewMode               wordViewMode
	wordPageFitMode            bool
	wordMultiplePagesMode      bool
	wordLayoutZoom             float64
	wordRulerHidden            bool
	wordGridlinesVisible       bool
	wordNavigationVisible      bool
	wordNavigationQuery        string
	wordToolbarTab             wordToolbarTab
	selectedWordRunID          string
	selectedWordParagraphID    string
	selectedWordImageID        string
	selectedWordSelectionStart int
	selectedWordSelectionEnd   int
	wordListFirst              int
	wordPageFirst              int
}

func captureWindowView(a *App) windowViewSnapshot {
	if a == nil || a.documentSessionState == nil {
		return windowViewSnapshot{}
	}
	state := a.documentSessionState
	snapshot := windowViewSnapshot{
		activeTabID:             a.activeTabID,
		wordViewMode:            state.wordViewMode,
		wordPageFitMode:         state.wordPageFitMode,
		wordMultiplePagesMode:   state.wordMultiplePagesMode,
		wordLayoutZoom:          state.wordLayoutZoom,
		wordRulerHidden:         state.wordRulerHidden,
		wordGridlinesVisible:    state.wordGridlinesVisible,
		wordNavigationVisible:   state.wordNavigationVisible,
		wordNavigationQuery:     state.wordNavigationSearchQuery,
		wordToolbarTab:          state.wordToolbarTab,
		selectedWordRunID:       state.selectedWordRunID,
		selectedWordParagraphID: state.selectedWordParagraphID,
		selectedWordImageID:     state.selectedWordImageID,
		wordListFirst:           state.wordListState.Position.First,
		wordPageFirst:           state.wordPageListState.Position.First,
	}
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		snapshot.selectedWordSelectionStart, snapshot.selectedWordSelectionEnd = editor.Selection()
	}
	return snapshot
}

func newDocumentViewFromSnapshot(shared *sharedDocumentSession, windowID uint64, snapshot windowViewSnapshot) *documentSessionState {
	view := newDocumentSessionView(shared, windowID)
	view.wordViewMode = snapshot.wordViewMode
	view.wordPageFitMode = snapshot.wordPageFitMode
	view.wordMultiplePagesMode = snapshot.wordMultiplePagesMode
	if snapshot.wordLayoutZoom > 0 {
		view.wordLayoutZoom = snapshot.wordLayoutZoom
	}
	view.wordRulerHidden = snapshot.wordRulerHidden
	view.wordGridlinesVisible = snapshot.wordGridlinesVisible
	view.wordNavigationVisible = snapshot.wordNavigationVisible
	view.wordNavigationSearchQuery = snapshot.wordNavigationQuery
	view.wordNavigationSearchEditor.SetText(snapshot.wordNavigationQuery)
	view.wordToolbarTab = snapshot.wordToolbarTab
	view.wordListState.Position.First = max(0, snapshot.wordListFirst)
	view.wordPageListState.Position.First = max(0, snapshot.wordPageFirst)

	blocks := wordBlocksForState(view)
	view.wordListState.Position.First = min(view.wordListState.Position.First, max(0, len(blocks)-1))
	view.wordPageListState.Position.First = min(view.wordPageListState.Position.First, max(0, int(shared.wordLayoutPageCount.Load())-1))
	if snapshot.selectedWordRunID != "" {
		if run, paragraph, ok := findEditableWordRun(blocks, snapshot.selectedWordRunID); ok {
			view.selectedWordRunID = run.ID
			view.selectedWordParagraphID = paragraph.ID
			view.selectedWordImageID = ""
			view.selectedBlock = -1
			view.selectedBlockEditable = true
			editor := wordRunEditor(view, run)
			start, end := clampEditorSelection(snapshot.selectedWordSelectionStart, snapshot.selectedWordSelectionEnd, editor.Text())
			editor.SetCaret(start, end)
		}
	} else if snapshot.selectedWordImageID != "" && findWordImage(blocks, snapshot.selectedWordImageID) != nil {
		view.selectedWordImageID = snapshot.selectedWordImageID
		view.selectedWordRunID = ""
		view.selectedWordParagraphID = snapshot.selectedWordParagraphID
	} else if snapshot.selectedWordParagraphID != "" {
		if paragraph, ok := selectedWordParagraph(blocks, snapshot.selectedWordParagraphID); ok {
			view.selectedWordParagraphID = paragraph.ID
		}
	}
	_, drafts := shared.wordTextStateSnapshot()
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			draft, hasDraft := drafts[inline.Text.ID]
			if !hasDraft && inline.Text.ID != view.selectedWordRunID {
				continue
			}
			editor := wordRunEditor(view, *inline.Text)
			if hasDraft && editor.Text() != draft {
				editor.SetText(draft)
			}
		}
	}
	syncWindowViewFromSharedSession(layout.Context{}, view)
	return view
}

func syncWindowViewFromSharedSession(gtx layout.Context, view *documentSessionState) {
	if view == nil || view.sharedDocumentSession == nil || view.word == nil {
		return
	}
	revision := view.wordRevisionAtomic.Load()
	draftEpoch := view.wordDraftEpoch.Load()
	if view.appliedSessionRevision.Load() == revision && view.appliedWordDraftEpoch.Load() == draftEpoch {
		return
	}
	blocks := wordBlocksForState(view)
	committed, drafts := view.wordTextStateSnapshot()
	runs := make(map[string]document.WordTextRun)
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				runs[inline.Text.ID] = *inline.Text
			}
		}
	}
	for runID, editor := range view.wordRunEditors {
		run, exists := runs[runID]
		if !exists || editor == nil {
			continue
		}
		value := run.Text
		if committedValue, ok := committed[runID]; ok {
			value = committedValue
		}
		if draft, ok := drafts[runID]; ok {
			value = draft
		}
		if editor.Text() == value || gtx.Focused(editor) {
			continue
		}
		start, end := editor.Selection()
		editor.SetText(value)
		start, end = clampEditorSelection(start, end, value)
		editor.SetCaret(start, end)
	}
	view.appliedSessionRevision.Store(revision)
	view.appliedWordDraftEpoch.Store(draftEpoch)
}

func findEditableWordRun(blocks []document.WordBlock, runID string) (document.WordTextRun, document.WordParagraph, bool) {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID && inline.Text.Editable {
				return *inline.Text, paragraph, true
			}
		}
	}
	return document.WordTextRun{}, document.WordParagraph{}, false
}

func clampEditorSelection(start, end int, value string) (int, int) {
	length := utf8.RuneCountInString(value)
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > length {
		start = length
	}
	if end > length {
		end = length
	}
	return start, end
}
