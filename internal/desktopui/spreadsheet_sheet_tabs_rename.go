package desktopui

import (
	"fmt"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"glance/internal/document"
)

func (a *App) beginSpreadsheetSheetRename(gtx layout.Context, state *documentSessionState, sheet string) {
	if state == nil || state.sheetNameEditing || state.formatRunning.Load() || state.book == nil || !state.book.Editable() {
		return
	}
	if _, ok := state.book.(document.SpreadsheetStructureEditor); !ok {
		return
	}
	state.sheetNameEditing = true
	state.sheetNameEditTarget = sheet
	state.sheetNameRenamePending = false
	state.sheetNameEditor.ReadOnly = false
	state.sheetNameEditor.SetText(sheet)
	state.sheetNameEditor.SetCaret(0, state.sheetNameEditor.Len())
	gtx.Execute(key.FocusCmd{Tag: &state.sheetNameEditor})
	gtx.Execute(op.InvalidateCmd{})
}

func (a *App) handleSpreadsheetSheetRenameEvents(gtx layout.Context, state *documentSessionState) {
	if state == nil || !state.sheetNameEditing || state.sheetNameRenamePending {
		return
	}
	for event, ok := state.sheetNameEditor.Update(gtx); ok; event, ok = state.sheetNameEditor.Update(gtx) {
		if _, submit := event.(widget.SubmitEvent); submit {
			a.commitSpreadsheetSheetRename(gtx, state)
			return
		}
	}
	if editorHasActiveComposition(&state.sheetNameEditor) {
		return
	}
	for event, ok := gtx.Event(key.Filter{Focus: &state.sheetNameEditor, Name: key.NameEscape}); ok; event, ok = gtx.Event(key.Filter{Focus: &state.sheetNameEditor, Name: key.NameEscape}) {
		if pressed, ok := event.(key.Event); ok && pressed.State == key.Press {
			a.cancelSpreadsheetSheetRename(gtx, state)
			return
		}
	}
}

func (a *App) commitSpreadsheetSheetRename(gtx layout.Context, state *documentSessionState) {
	if state == nil || state != a.documentSessionState || !state.sheetNameEditing || state.sheetNameRenamePending {
		return
	}
	name := strings.TrimSpace(state.sheetNameEditor.Text())
	if name == "" {
		name = nextSpreadsheetSheetName(state.sheets, state.sheetNameEditTarget)
	}
	if name == state.sheetNameEditTarget {
		a.cancelSpreadsheetSheetRename(gtx, state)
		return
	}
	if state.formatRunning.Load() || state.book == nil || !state.book.Editable() {
		return
	}
	if _, ok := state.book.(document.SpreadsheetStructureEditor); !ok {
		return
	}
	state.sheetNameRenamePending = true
	state.sheetNameEditor.ReadOnly = true
	a.applyStructure(document.StructureEdit{Kind: document.StructureRenameSheet, Sheet: state.sheetNameEditTarget, NewName: name})
	gtx.Execute(op.InvalidateCmd{})
}

func (a *App) cancelSpreadsheetSheetRename(gtx layout.Context, state *documentSessionState) {
	if state == nil {
		return
	}
	target := state.sheetNameEditTarget
	state.sheetNameEditing = false
	state.sheetNameEditTarget = ""
	state.sheetNameRenamePending = false
	state.sheetNameEditor.ReadOnly = false
	state.sheetNameEditor.SetText("")
	if target != "" {
		gtx.Execute(key.FocusCmd{Tag: a.sheetButton(target)})
	}
	gtx.Execute(op.InvalidateCmd{})
}

func nextSpreadsheetSheetName(sheets []string, replacing string) string {
	for index := 1; ; index++ {
		candidate := fmt.Sprintf("Sheet%d", index)
		used := false
		for _, sheet := range sheets {
			if strings.EqualFold(sheet, replacing) {
				continue
			}
			if strings.EqualFold(sheet, candidate) {
				used = true
				break
			}
		}
		if !used {
			return candidate
		}
	}
}
