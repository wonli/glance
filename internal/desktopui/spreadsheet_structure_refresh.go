package desktopui

import (
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) refreshAfterStructure(state *documentSessionState, edit document.StructureEdit) {
	unlock := lockSessionAdapterRead(state)
	oldSheet := state.sheet
	state.sheets = state.book.SheetNames()
	unlock()
	switch edit.Kind {
	case document.StructureCreateSheet:
		if len(state.sheets) > 0 {
			state.sheet = state.sheets[len(state.sheets)-1]
		}
	case document.StructureRenameSheet:
		if strings.EqualFold(oldSheet, edit.Sheet) {
			state.sheet = strings.TrimSpace(edit.NewName)
		}
	case document.StructureDeleteSheet:
		if strings.EqualFold(oldSheet, edit.Sheet) && len(state.sheets) > 0 {
			state.sheet = state.sheets[0]
		}
	}
	if edit.Kind == document.StructureInsertRows || edit.Kind == document.StructureDeleteRows || edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
		state.selection.Anchor = rebasePosition(state.selection.Anchor, edit)
		state.selection.Active = rebasePosition(state.selection.Active, edit)
	} else {
		state.selection = CellSelection{}
	}
	state.selectionSet = true
	active := state.selection.Active
	state.selectedCell = xlsx.CellName(active.Column+1, active.Row+1)
	state.cellEdit.selectCell(state.selectedCell, "")
	state.nameBox.SetText(state.selectedCell)
	state.formulaEditor.SetText("")
	if state.virtualGrid != nil {
		state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = active.Row+1, active.Column+1
		state.virtualGrid.PendingSelectionLoad = true
		state.virtualGrid.resetSheetWindow()
	}
	if state == a.documentSessionState {
		a.sheet = state.sheet
		a.sheets = state.sheets
	}
	if edit.Kind == document.StructureRenameSheet && a.sheetButtons != nil {
		delete(a.sheetButtons, edit.Sheet)
	}
	state.sheetNameEditing = false
	state.sheetNameEditTarget = ""
	state.sheetNameRenamePending = false
	state.sheetNameEditor.ReadOnly = false
	state.sheetNameEditor.SetText("")
}

func rebasePosition(position CellPosition, edit document.StructureEdit) CellPosition {
	coordinate := position.Row
	if edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
		coordinate = position.Column
	}
	index := edit.Index - 1
	switch edit.Kind {
	case document.StructureInsertRows, document.StructureInsertColumns:
		if coordinate >= index {
			coordinate += edit.Count
		}
	case document.StructureDeleteRows, document.StructureDeleteColumns:
		if coordinate >= index+edit.Count {
			coordinate -= edit.Count
		} else if coordinate >= index {
			coordinate = index
		}
	}
	if coordinate < 0 {
		coordinate = 0
	}
	if edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
		position.Column = coordinate
	} else {
		position.Row = coordinate
	}
	return position
}
