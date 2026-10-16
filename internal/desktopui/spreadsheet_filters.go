package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/document"
)

type toggleValueFilterSetFromRowArguments struct {
	Operation string             `json:"operation"`
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	Column    int                `json:"column"`
	SourceRow int                `json:"source_row"`
}

type toggleValueFilterArguments struct {
	Operation string `json:"operation"`
	Sheet     string `json:"sheet"`
}

func (a *App) toggleValueFilter() {
	state := a.documentSessionState
	if state == nil || state.formatRunning.Load() || state.book == nil {
		return
	}
	if state.valueFilter != nil {
		if a.applicationSessionRegistered(state.documentID) {
			a.runSpreadsheetFilterIntent(toggleValueFilterArguments{Operation: "clear", Sheet: state.sheet})
		} else {
			a.clearValueFilter(state)
		}
		return
	}
	if !state.selectionSet {
		return
	}
	bounds := state.selection.Bounds()
	pos := state.selection.Active
	sheet := state.sheet
	if pos.Row+1 <= bounds.StartRow {
		return
	}
	if a.applicationSessionRegistered(state.documentID) {
		a.runSpreadsheetFilterIntent(toggleValueFilterSetFromRowArguments{Operation: "set_from_row", Sheet: sheet, Bounds: bounds, Column: pos.Column + 1, SourceRow: pos.Row + 1})
		return
	}
	reader, ok := state.book.(document.SpreadsheetRangeReader)
	if !ok {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	generation, window := a.documentGeneration, state.virtualGrid
	column := pos.Column + 1
	a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(state)
		cells, err := reader.ReadRange(sheet, bounds)
		var filter *ValueFilter
		if err == nil {
			targetRow, targetCol := pos.Row+1-bounds.StartRow, column-bounds.StartColumn
			if targetRow < 0 || targetRow >= len(cells) || targetCol < 0 || targetCol >= len(cells[targetRow]) {
				err = fmt.Errorf("筛选列不在选择范围内")
			} else {
				value := cells[targetRow][targetCol].Value
				visible := filterMatchingRows(cells, bounds.StartRow, targetCol, value)
				filter = &ValueFilter{Sheet: sheet, Bounds: bounds, Column: column, Value: value, VisibleRows: visible}
				if grid, ok := state.book.(document.GridReader); ok {
					rows := bounds.EndRow - bounds.StartRow + 1
					if full, readErr := grid.Window(sheet, bounds.StartRow, bounds.StartColumn, rows, 1); readErr == nil {
						filter.RowHeightsPoints = full.RowHeightsPoints
					}
				}
			}
		}
		unlock()
		a.completeSessionOperation(state, token, err, false)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "筛选", filter: filter}:
		case <-a.shutdown:
		}
	})
}

func (a *App) runSpreadsheetFilterIntent(arguments any) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.filter.apply", "attached_window", fmt.Sprint(a.windowID), sheet, arguments)
		var changed bool
		if err == nil {
			var outcome struct {
				Changed bool `json:"changed"`
			}
			err = json.Unmarshal(result.Data, &outcome)
			changed = outcome.Changed
		}
		a.completeSessionOperation(state, token, err, false)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "筛选", applied: changed}:
		case <-a.shutdown:
		}
	})
}

func filterMatchingRows(cells [][]document.Cell, firstRow, column int, value string) map[int]bool {
	visible := make(map[int]bool)
	for offset := 1; offset < len(cells); offset++ {
		if column >= 0 && column < len(cells[offset]) && cells[offset][column].Value == value {
			visible[firstRow+offset] = true
		}
	}
	return visible
}

func (a *App) clearValueFilter(state *documentSessionState) {
	filter := state.valueFilter
	if filter == nil {
		return
	}
	if state.virtualGrid != nil {
		for row, size := range filter.OriginalHeights {
			_ = state.virtualGrid.Rows.SetSize(row, size)
		}
		state.virtualGrid.invalidateWindow()
	}
	state.valueFilter = nil
	if state == a.documentSessionState && a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) applyValueFilterWindow(state *documentSessionState, v *gridViewport) {
	f := state.valueFilter
	if f == nil || v == nil || f.Sheet != v.WindowSheet {
		return
	}
	for _, loaded := range []document.CellRange{v.WindowRange, v.TopRange, v.LeftRange, v.CornerRange} {
		for row := maxInt(f.Bounds.StartRow, loaded.StartRow); row <= minInt(f.Bounds.EndRow, loaded.EndRow); row++ {
			if row == f.Bounds.StartRow || f.VisibleRows[row] {
				continue
			}
			_ = v.Rows.SetSize(row-1, 0)
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
