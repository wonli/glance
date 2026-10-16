package desktopui

import (
	"runtime"

	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/format/xlsx"
)

func (a *App) handleSpreadsheetSheetNavigationKey(gtx layout.Context, event key.Event) bool {
	switch {
	case event.Modifiers == key.ModCtrl && event.Name == key.NamePageUp:
		return a.moveSpreadsheetSheet(gtx, -1)
	case event.Modifiers == key.ModCtrl && event.Name == key.NamePageDown:
		return a.moveSpreadsheetSheet(gtx, 1)
	case runtime.GOOS == "darwin" && event.Modifiers == key.ModAlt && event.Name == key.NameLeftArrow:
		return a.moveSpreadsheetSheet(gtx, -1)
	case runtime.GOOS == "darwin" && event.Modifiers == key.ModAlt && event.Name == key.NameRightArrow:
		return a.moveSpreadsheetSheet(gtx, 1)
	default:
		return false
	}
}

func (a *App) moveSpreadsheetSheet(gtx layout.Context, direction int) bool {
	state := a.documentSessionState
	if state == nil || len(state.sheets) < 2 || direction == 0 {
		return false
	}
	current := -1
	for index, name := range state.sheets {
		if name == state.sheet {
			current = index
			break
		}
	}
	if current < 0 {
		return false
	}
	next := current + direction
	if next < 0 || next >= len(state.sheets) {
		return true
	}
	return a.activateSpreadsheetSheet(gtx, state.sheets[next])
}

func (a *App) activateSpreadsheetSheet(gtx layout.Context, sheetName string) bool {
	state := a.documentSessionState
	if state == nil || sheetName == "" {
		return false
	}
	if sheetName == state.sheet {
		return true
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	formulaReferenceActive := state.formulaReference.Active
	if state.cellEdit.Editing && !formulaReferenceActive {
		if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
			a.setStatus("修改失败：" + err.Error())
			if a.window != nil {
				a.window.Invalidate()
			}
			return false
		}
	}
	if !formulaReferenceActive {
		if state.sheetActiveCells == nil {
			state.sheetActiveCells = make(map[string]string)
		}
		if a.selectedCell != "" {
			state.sheetActiveCells[state.sheet] = a.selectedCell
		}
	}
	state.gridNavigationSequence++
	state.gridEndMode = false
	state.gridPictureGesture.cancel()
	setSelectedGridPicture(state, "")

	state.sheet = sheetName
	a.sheet = sheetName
	setSpreadsheetZoomView(state, state.spreadsheetSheetZoom(sheetName))
	if state.virtualGrid != nil {
		state.virtualGrid.resetSheetWindow()
		state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = 1, 1
		state.virtualGrid.PendingSelectionLoad = true
	}
	state.selection = CellSelection{}
	if formulaReferenceActive {
		state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
	} else {
		cell := state.sheetActiveCells[sheetName]
		if cell == "" {
			cell = "A1"
		}
		position := CellPosition{}
		if row, column, ok := parseCellAddress(cell); ok {
			position = clampCellPosition(CellPosition{Row: row - 1, Column: column - 1})
			cell = spreadsheetCellName(position)
		} else {
			cell = "A1"
		}
		state.selectedCell = cell
		state.selection = CellSelection{Anchor: position, Active: position}
		if state.virtualGrid != nil {
			state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = position.Row+1, position.Column+1
			state.virtualGrid.PendingSelectionLoad = true
			revealGridCell(state.virtualGrid, position, a.lastGridWidth, a.lastGridHeight)
		}
		state.cellEdit.selectCell(cell, "")
		state.nameBox.SetText(cell)
		state.formulaEditor.SetText("")
	}
	state.selectionSet = true
	state.selectionDragging = false
	if formulaReferenceActive {
		if editor := a.formulaReferenceEditor(); editor != nil {
			gtx.Execute(key.FocusCmd{Tag: editor})
		}
	} else {
		a.persistViewState()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}

func spreadsheetCellName(position CellPosition) string {
	return xlsx.CellName(position.Column+1, position.Row+1)
}
