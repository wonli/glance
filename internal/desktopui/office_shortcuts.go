package desktopui

import (
	"runtime"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
)

// handleOfficeShortcuts routes the common Office shortcuts through the same
// command methods used by the Ribbon. A nil Focus filter lets the focused
// document editor keep focus while the non-text command is handled here.
func (a *App) handleOfficeShortcuts(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || (state.word == nil && state.book == nil) || a.modal.Visible || a.fileCommandRunning || a.wordVersionHistoryVisible || a.wordSymbolPickerVisible {
		return
	}

	replaceModifier := key.ModShortcut
	if runtime.GOOS == "darwin" {
		// Cmd+H belongs to macOS' Hide Application command. Office for Mac
		// uses Control+H for Replace.
		replaceModifier = key.ModCtrl
	}
	filters := []event.Filter{
		key.Filter{Name: key.Name("S"), Required: key.ModShortcut},
		key.Filter{Name: key.Name("F"), Required: key.ModShortcut},
		key.Filter{Name: key.Name("H"), Required: replaceModifier},
	}
	for event, ok := gtx.Event(filters...); ok; event, ok = gtx.Event(filters...) {
		pressed, ok := event.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		switch pressed.Name {
		case key.Name("S"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
			} else if state.book != nil && !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
				return
			}
			a.startSave()
		case key.Name("F"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				a.showWordFindDialog()
			} else if state.book != nil {
				if !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
					return
				}
				a.showSpreadsheetFindDialog()
			}
		case key.Name("H"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				a.showWordReplaceDialog()
			} else if state.book != nil {
				if !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
					return
				}
				a.showSpreadsheetReplaceDialog()
			}
		}
		// Opening a dialog changes the active key target. Handle at most one
		// shortcut per frame so queued events cannot leak into the new surface.
		if a.modal.Visible {
			return
		}
	}
}

func (a *App) commitSpreadsheetEditorBeforeCommand(gtx layout.Context, state *documentSessionState) bool {
	if state == nil || state.book == nil {
		return true
	}
	var editor *widget.Editor
	switch {
	case gtx.Focused(&a.formulaEditor):
		editor = &a.formulaEditor
	case state.cellEdit.Editing:
		editor = &a.cellEditor
	default:
		return true
	}
	for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
		if _, changed := event.(widget.ChangeEvent); changed {
			state.cellEdit.setDraft(editor.Text())
		}
	}
	value := editor.Text()
	if value == state.cellEdit.Original {
		state.cellEdit.setEditing(false)
		return true
	}
	if err := a.commitGridCell(value); err != nil {
		a.setStatus("修改失败：" + err.Error())
		if a.window != nil {
			a.window.Invalidate()
		}
		return false
	}
	return true
}

func (a *App) updateFocusedWordEditorBeforeCommand(gtx layout.Context, state *documentSessionState) {
	if state == nil || state.word == nil {
		return
	}
	runID := state.wordFocusedRunID
	if runID == "" {
		runID = state.selectedWordRunID
	}
	editor := state.wordRunEditors[runID]
	if editor == nil || !gtx.Focused(editor) {
		return
	}
	for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
		if _, submit := event.(widget.SubmitEvent); submit {
			start, end := editor.Selection()
			a.queueWordParagraphSplit(runID, editor.Text(), start, end)
			break
		}
	}
	captureWordTextDraft(state, runID, editor.Text())
}
