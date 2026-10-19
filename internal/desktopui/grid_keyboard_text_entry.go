package desktopui

import (
	"glance/internal/i18n"
	"runtime"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
)

// handleSpreadsheetGridFocusEvent turns text input sent to the focused grid
// into a normal cell-editor draft. The grid remains the input target while an
// IME composition is active so that changing focus cannot interrupt it.
func (a *App) handleSpreadsheetGridFocusEvent(gtx layout.Context, v *gridViewport, ev event.Event) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	switch ev := ev.(type) {
	case key.CompositionEvent:
		state.gridInputComposing = ev.Start >= 0 && ev.End >= 0
		if !state.gridInputComposing && state.cellEdit.Editing {
			gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		}
	case key.EditEvent:
		starting := !state.cellEdit.Editing
		if starting {
			if !state.cellEdit.begin() {
				return
			}
			a.prepareSpreadsheetCellEditorForDirectEntry(v)
			a.cellEditor.SetText("")
			state.cellEdit.setDraft("")
		}
		applySpreadsheetEditorEditEvent(&a.cellEditor, ev, starting)
		a.syncSpreadsheetGridTextDraft()
		a.updateSpreadsheetGridInputSnippet(gtx, v)
		if !state.gridInputComposing {
			gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	case key.SelectionEvent:
		if state.cellEdit.Editing {
			start, end := clampEditorRange(ev.Start, ev.End, a.cellEditor.Len())
			a.cellEditor.SetCaret(start, end)
		}
	}
}

func (a *App) updateSpreadsheetGridInputSnippet(gtx layout.Context, v *gridViewport) {
	if v == nil {
		return
	}
	gtx.Execute(key.SnippetCmd{Tag: &v.PointerTag, Snippet: key.Snippet{
		Range: key.Range{Start: 0, End: a.cellEditor.Len()},
		Text:  a.cellEditor.Text(),
	}})
}

func (a *App) prepareSpreadsheetCellEditorForDirectEntry(v *gridViewport) {
	if v == nil || a.documentSessionState == nil {
		return
	}
	row, column, ok := parseCellAddress(a.documentSessionState.cellEdit.Address)
	if !ok || v.WindowSheet != a.sheet || !v.cellLoaded(row, column) {
		return
	}
	a.cellEditor.SingleLine = !v.cell(row, column).Style.WrapText
}

func (a *App) beginSpreadsheetCellEditFromF2(gtx layout.Context, v *gridViewport) {
	if !a.cellEdit.begin() {
		return
	}
	a.prepareSpreadsheetCellEditorForDirectEntry(v)
	a.cellEditor.SetText(a.cellEdit.Original)
	caret := a.cellEditor.Len()
	a.cellEditor.SetCaret(caret, caret)
	a.formulaEditor.SetText(a.cellEdit.Original)
	a.formulaEditor.SetCaret(caret, caret)
	gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
	if a.window != nil {
		a.window.Invalidate()
	}
}

func applySpreadsheetEditorEditEvent(editor *widget.Editor, edit key.EditEvent, replaceAll bool) {
	text := []rune(editor.Text())
	start, end := edit.Range.Start, edit.Range.End
	if replaceAll {
		start, end = 0, len(text)
	}
	start, end = clampEditorRange(start, end, len(text))
	if start > end {
		start, end = end, start
	}
	replacement := []rune(edit.Text)
	result := make([]rune, 0, len(text)-(end-start)+len(replacement))
	result = append(result, text[:start]...)
	result = append(result, replacement...)
	result = append(result, text[end:]...)
	editor.SetText(string(result))
	caret := start + len(replacement)
	editor.SetCaret(caret, caret)
}

func clampEditorRange(start, end, length int) (int, int) {
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

func (a *App) syncSpreadsheetGridTextDraft() {
	state := a.documentSessionState
	if state == nil {
		return
	}
	text := a.cellEditor.Text()
	state.cellEdit.setDraft(text)
	a.formulaEditor.SetText(text)
	start, end := a.cellEditor.Selection()
	a.formulaEditor.SetCaret(start, end)
}

func (a *App) commitSpreadsheetGridTextEntry(gtx layout.Context, v *gridViewport, direction key.Name) {
	if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if state := a.documentSessionState; state != nil {
		state.gridInputComposing = false
	}
	a.moveSpreadsheetSelectionAfterEntry(v, direction)
	if v != nil {
		gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) beginSpreadsheetCellBackspaceEdit(gtx layout.Context, v *gridViewport) bool {
	state := a.documentSessionState
	if runtime.GOOS != "darwin" || state == nil || !state.selectionSet || state.selection.Scope != selectionCells || state.selection.Anchor != state.selection.Active {
		return false
	}
	if !state.cellEdit.begin() {
		return false
	}
	a.prepareSpreadsheetCellEditorForDirectEntry(v)
	a.cellEditor.SetText(state.cellEdit.Original)
	end := a.cellEditor.Len()
	a.cellEditor.SetCaret(end, end)
	a.cellEditor.Delete(-1)
	a.syncSpreadsheetGridTextDraft()
	state.gridInputComposing = false
	gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}
