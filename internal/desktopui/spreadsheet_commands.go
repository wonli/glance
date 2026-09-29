package desktopui

import (
	"fmt"
	"glance/internal/document"
)

type spreadsheetFormatResult struct {
	state       *documentSessionState
	generation  uint64
	sheet       string
	window      *gridViewport
	err         error
	history     bool
	applied     bool
	undo        bool
	operation   string
	structure   *document.StructureEdit
	sort        *document.SortRequest
	filter      *ValueFilter
	clearFilter bool
}

func (a *App) applyHistory(undo bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning {
		return
	}
	history, ok := state.book.(document.SpreadsheetHistory)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	state.formatRunning = true
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		var applied bool
		var err error
		if undo {
			applied, err = history.Undo()
		} else {
			applied, err = history.Redo()
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, generation: generation, sheet: sheet, window: window, err: err, history: true, applied: applied, undo: undo}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) applyCellFormat(patch document.CellFormatPatch) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning {
		return
	}
	formatter, ok := state.book.(document.SpreadsheetFormatter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	selection := state.selection.Bounds()
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	state.formatRunning = true
	go func() {
		err := formatter.SetCellFormats(sheet, selection, patch)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, generation: generation, sheet: sheet, window: window, err: err}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) applyBatchEdits(edits []document.CellEdit, operation string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning {
		return fmt.Errorf("表格正在处理其他操作")
	}
	editor, ok := state.book.(document.SpreadsheetBatchEditor)
	if !ok {
		return fmt.Errorf("当前表格不支持批量编辑")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	state.formatRunning = true
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		err := editor.SetCells(sheet, edits)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
	return nil
}

func (a *App) applyStructure(edit document.StructureEdit) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning {
		return
	}
	editor, ok := state.book.(document.SpreadsheetStructureEditor)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	state.formatRunning = true
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		err := editor.EditStructure(edit)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, generation: generation, sheet: sheet, window: window, err: err, operation: "结构操作", structure: &edit}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) applySort(request document.SortRequest) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning {
		return
	}
	sorter, ok := state.book.(document.SpreadsheetSorter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	state.formatRunning = true
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		err := sorter.SortRange(request)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, generation: generation, sheet: sheet, window: window, err: err, operation: "排序", sort: &request}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) toggleValueFilter() {
	state := a.documentSessionState
	if state == nil || state.formatRunning || state.book == nil {
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
	state.formatRunning = true
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	column := pos.Column + 1
	go func() {
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
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, generation: generation, sheet: sheet, window: window, err: err, operation: "筛选", filter: filter}:
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

func (a *App) clearSelectionAsync() {
	state := a.documentSessionState
	if state == nil || !state.selectionSet {
		return
	}
	b := state.selection.Bounds()
	count := (b.EndRow - b.StartRow + 1) * (b.EndColumn - b.StartColumn + 1)
	if count > maxClipboardCellsUI {
		a.status = fmt.Sprintf("剪切选区超过 %d 个单元格", maxClipboardCellsUI)
		return
	}
	edits := make([]document.CellEdit, 0, count)
	for r := b.StartRow; r <= b.EndRow; r++ {
		for c := b.StartColumn; c <= b.EndColumn; c++ {
			edits = append(edits, document.CellEdit{Row: r, Column: c, Value: ""})
		}
	}
	if err := a.applyBatchEdits(edits, "剪切"); err != nil {
		a.status = "剪切失败：" + err.Error()
	}
}
