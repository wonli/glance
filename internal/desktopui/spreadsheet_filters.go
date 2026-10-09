package desktopui

import (
	"fmt"
	"glance/internal/document"
)

func (a *App) toggleValueFilter() {
	state := a.documentSessionState
	if state == nil || state.formatRunning.Load() || state.book == nil {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
		return
	}
	if !state.selectionSet {
		return
	}
	bounds := state.selection.Bounds()
	pos := state.selection.Active
	reader, ok := state.book.(document.SpreadsheetRangeReader)
	if !ok || pos.Row+1 <= bounds.StartRow {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	column := pos.Column + 1
	go func() {
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
	}()
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
	for row := maxInt(f.Bounds.StartRow, v.WindowRange.StartRow); row <= minInt(f.Bounds.EndRow, v.WindowRange.EndRow); row++ {
		index := row - 1
		if row == f.Bounds.StartRow || f.VisibleRows[row] {
			continue
		}
		_ = v.Rows.SetSize(index, 0)
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
