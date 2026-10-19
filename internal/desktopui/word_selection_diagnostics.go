package desktopui

import (
	"glance/internal/diagnostics"
)

func recordWordSelectionDiagnostic(state *documentSessionState, event diagnostics.WordSelectionEvent) {
	if !diagnostics.WordSelectionDiagnosticsEnabled() {
		return
	}
	event.ViewMode = int(state.wordViewMode)
	event.ReadingMode = state.wordReadingMode
	event.Dragging = state.wordPageSelectionDrag != nil
	if selection := state.wordPageTextSelection; selection != nil {
		event.AnchorRun, event.AnchorOffset = selection.anchor.runID, selection.anchor.runeOffset
		event.FocusRun, event.FocusOffset = selection.focus.runID, selection.focus.runeOffset
	}
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		event.EditorStart, event.EditorEnd = editor.Selection()
	}
	diagnostics.RecordWordSelection(event)
}
