package csv

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"glance/internal/document"
)

func parseCellCoordinate(cell string) (row, col int, ok bool) {
	c := strings.ToUpper(strings.TrimSpace(cell))
	i := 0
	for i < len(c) && c[i] >= 'A' && c[i] <= 'Z' {
		col = col*26 + int(c[i]-'A'+1)
		i++
	}
	if i == 0 || i == len(c) {
		return 0, 0, false
	}
	r, err := strconv.Atoi(c[i:])
	if err != nil || r < 1 || col < 1 {
		return 0, 0, false
	}
	return r, col, true
}

func (d *Document) setCellInternal(row, col int, value string) error {
	oldVal := d.getCellValueLocked(row, col)
	if oldVal == value {
		return nil
	}

	d.edits[cellKey{row: row, col: col}] = value
	if row > d.maxRow {
		d.maxRow = row
	}
	if col > d.maxCol {
		d.maxCol = col
	}

	d.undoStack = append(d.undoStack, editBatchAction{
		edits: []editAction{{row: row, col: col, oldVal: oldVal, newVal: value}},
	})
	d.redoStack = nil
	d.dirty = true
	return nil
}

// SetCellText writes text to one cell and records history.
func (d *Document) SetCellText(sheet, cell, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("document is closed")
	}

	row, col, ok := parseCellCoordinate(cell)
	if !ok {
		return fmt.Errorf("invalid cell reference: %q", cell)
	}

	return d.setCellInternal(row, col, value)
}

// SetCellValueChanged writes a typed value to one cell and reports whether content changed.
func (d *Document) SetCellValueChanged(sheet, cell string, value document.CellValue) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false, errors.New("document is closed")
	}

	row, col, ok := parseCellCoordinate(cell)
	if !ok {
		return false, fmt.Errorf("invalid cell reference: %q", cell)
	}

	text := value.Text
	switch value.Kind {
	case document.CellValueNumber:
		text = strconv.FormatFloat(value.Number, 'f', -1, 64)
	case document.CellValueBoolean:
		if value.Boolean {
			text = "TRUE"
		} else {
			text = "FALSE"
		}
	}

	oldVal := d.getCellValueLocked(row, col)
	if oldVal == text {
		return false, nil
	}

	if err := d.setCellInternal(row, col, text); err != nil {
		return false, err
	}
	return true, nil
}

// SetCellsChanged writes a batch of cell edits as one undoable action.
func (d *Document) SetCellsChanged(sheet string, edits []document.CellEdit) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false, errors.New("document is closed")
	}

	var batch []editAction
	for _, e := range edits {
		oldVal := d.getCellValueLocked(e.Row, e.Column)
		if oldVal != e.Value {
			d.edits[cellKey{row: e.Row, col: e.Column}] = e.Value
			if e.Row > d.maxRow {
				d.maxRow = e.Row
			}
			if e.Column > d.maxCol {
				d.maxCol = e.Column
			}
			batch = append(batch, editAction{
				row:    e.Row,
				col:    e.Column,
				oldVal: oldVal,
				newVal: e.Value,
			})
		}
	}

	if len(batch) == 0 {
		return false, nil
	}

	d.undoStack = append(d.undoStack, editBatchAction{edits: batch})
	d.redoStack = nil
	d.dirty = true
	return true, nil
}

// ClearCellRange clears values across a rectangular cell range.
func (d *Document) ClearCellRange(sheet string, bounds document.CellRange) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false, errors.New("document is closed")
	}

	var batch []editAction
	for r := bounds.StartRow; r <= bounds.EndRow; r++ {
		for c := bounds.StartColumn; c <= bounds.EndColumn; c++ {
			oldVal := d.getCellValueLocked(r, c)
			if oldVal != "" {
				d.edits[cellKey{row: r, col: c}] = ""
				batch = append(batch, editAction{
					row:    r,
					col:    c,
					oldVal: oldVal,
					newVal: "",
				})
			}
		}
	}

	if len(batch) == 0 {
		return false, nil
	}

	d.undoStack = append(d.undoStack, editBatchAction{edits: batch})
	d.redoStack = nil
	d.dirty = true
	return true, nil
}

// TransferCellRangeChanged pastes cells from source to target as one undoable edit.
func (d *Document) TransferCellRangeChanged(sheet string, source document.CellRange, targetRow, targetColumn int, cells [][]document.Cell) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false, errors.New("document is closed")
	}

	var batch []editAction
	for rIdx, row := range cells {
		for cIdx, cell := range row {
			destR := targetRow + rIdx
			destC := targetColumn + cIdx
			oldVal := d.getCellValueLocked(destR, destC)
			if oldVal != cell.Value {
				d.edits[cellKey{row: destR, col: destC}] = cell.Value
				if destR > d.maxRow {
					d.maxRow = destR
				}
				if destC > d.maxCol {
					d.maxCol = destC
				}
				batch = append(batch, editAction{
					row:    destR,
					col:    destC,
					oldVal: oldVal,
					newVal: cell.Value,
				})
			}
		}
	}

	if len(batch) == 0 {
		return false, nil
	}

	d.undoStack = append(d.undoStack, editBatchAction{edits: batch})
	d.redoStack = nil
	d.dirty = true
	return true, nil
}

// Undo reverts the latest edit batch.
func (d *Document) Undo() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false, errors.New("document is closed")
	}

	if len(d.undoStack) == 0 {
		return false, nil
	}

	action := d.undoStack[len(d.undoStack)-1]
	d.undoStack = d.undoStack[:len(d.undoStack)-1]

	for i := len(action.edits) - 1; i >= 0; i-- {
		e := action.edits[i]
		d.edits[cellKey{row: e.row, col: e.col}] = e.oldVal
	}

	d.redoStack = append(d.redoStack, action)
	d.dirty = true
	return true, nil
}

// Redo reapplies the latest undone edit batch.
func (d *Document) Redo() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false, errors.New("document is closed")
	}

	if len(d.redoStack) == 0 {
		return false, nil
	}

	action := d.redoStack[len(d.redoStack)-1]
	d.redoStack = d.redoStack[:len(d.redoStack)-1]

	for _, e := range action.edits {
		d.edits[cellKey{row: e.row, col: e.col}] = e.newVal
	}

	d.undoStack = append(d.undoStack, action)
	d.dirty = true
	return true, nil
}

// RebaseHistory clears undo and redo stacks.
func (d *Document) RebaseHistory() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.undoStack = nil
	d.redoStack = nil
}

// Dimension reports the current populated extents (maxColumn, maxRow), both 1-based.
func (d *Document) Dimension(sheet string) (maxColumn, maxRow int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.maxCol, d.maxRow, nil
}

// VisitDimensionCells visits populated cells in one row or column.
func (d *Document) VisitDimensionCells(sheet string, axis document.SpreadsheetDimensionAxis, index int, visit func(document.SpreadsheetDimensionCell) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if axis == document.SpreadsheetDimensionRow {
		for c := 1; c <= d.maxCol; c++ {
			val := d.getCellValueLocked(index, c)
			if val != "" {
				err := visit(document.SpreadsheetDimensionCell{
					DimensionIndex: index,
					DimensionSize:  defaultRowHeight,
					Index:          c,
					Cell:           document.Cell{Value: val},
				})
				if err != nil {
					return err
				}
			}
		}
	} else if axis == document.SpreadsheetDimensionColumn {
		for r := 1; r <= d.maxRow; r++ {
			val := d.getCellValueLocked(r, index)
			if val != "" {
				err := visit(document.SpreadsheetDimensionCell{
					DimensionIndex: index,
					DimensionSize:  defaultColumnWidth,
					Index:          r,
					Cell:           document.Cell{Value: val},
				})
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// VisitDimensionRangeCells visits populated cells across a range of rows or columns.
func (d *Document) VisitDimensionRangeCells(sheet string, axis document.SpreadsheetDimensionAxis, start, end int, visit func(document.SpreadsheetDimensionCell) error) error {
	for idx := start; idx <= end; idx++ {
		if err := d.VisitDimensionCells(sheet, axis, idx, visit); err != nil {
			return err
		}
	}
	return nil
}

// SetDimensionSizes applies row heights or column widths.
func (d *Document) SetDimensionSizes(sheet string, axis document.SpreadsheetDimensionAxis, sizes []document.SpreadsheetDimensionSize) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range sizes {
		if axis == document.SpreadsheetDimensionRow {
			d.rowHeights[s.Index] = s.Value
		} else {
			d.colWidths[s.Index] = s.Value
		}
	}
	return nil
}

// SetRowHeightAsPartOfLatestEdit updates the row height for the latest edit.
func (d *Document) SetRowHeightAsPartOfLatestEdit(sheet string, row int, heightPoints float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rowHeights[row] = heightPoints
	return nil
}

// DataRegionEdge finds the edge of populated cells in direction.
func (d *Document) DataRegionEdge(sheet string, current document.CellCoordinate, direction document.SpreadsheetDirection) (document.CellCoordinate, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	r, c := current.Row, current.Column
	switch direction {
	case document.SpreadsheetDirectionDown:
		for r+1 <= d.maxRow && d.getCellValueLocked(r+1, c) != "" {
			r++
		}
	case document.SpreadsheetDirectionUp:
		for r-1 >= 1 && d.getCellValueLocked(r-1, c) != "" {
			r--
		}
	case document.SpreadsheetDirectionRight:
		for c+1 <= d.maxCol && d.getCellValueLocked(r, c+1) != "" {
			c++
		}
	case document.SpreadsheetDirectionLeft:
		for c-1 >= 1 && d.getCellValueLocked(r, c-1) != "" {
			c--
		}
	}
	return document.CellCoordinate{Row: r, Column: c}, nil
}

// CurrentDataRegion returns the bounded region around current cell.
func (d *Document) CurrentDataRegion(sheet string, current document.CellCoordinate) (document.CellRange, document.CellCoordinate, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	bounds := document.CellRange{
		StartRow:    1,
		StartColumn: 1,
		EndRow:      d.maxRow,
		EndColumn:   d.maxCol,
	}
	return bounds, current, true, nil
}

// NextNonEmptyCell finds the next occupied cell in direction.
func (d *Document) NextNonEmptyCell(sheet string, current document.CellCoordinate, direction document.SpreadsheetDirection) (document.CellCoordinate, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	r, c := current.Row, current.Column
	switch direction {
	case document.SpreadsheetDirectionDown:
		for r = r + 1; r <= d.maxRow; r++ {
			if d.getCellValueLocked(r, c) != "" {
				return document.CellCoordinate{Row: r, Column: c}, nil
			}
		}
		return document.CellCoordinate{Row: d.maxRow, Column: c}, nil
	case document.SpreadsheetDirectionUp:
		for r = r - 1; r >= 1; r-- {
			if d.getCellValueLocked(r, c) != "" {
				return document.CellCoordinate{Row: r, Column: c}, nil
			}
		}
		return document.CellCoordinate{Row: 1, Column: c}, nil
	case document.SpreadsheetDirectionRight:
		for c = c + 1; c <= d.maxCol; c++ {
			if d.getCellValueLocked(r, c) != "" {
				return document.CellCoordinate{Row: r, Column: c}, nil
			}
		}
		return document.CellCoordinate{Row: r, Column: d.maxCol}, nil
	case document.SpreadsheetDirectionLeft:
		for c = c - 1; c >= 1; c-- {
			if d.getCellValueLocked(r, c) != "" {
				return document.CellCoordinate{Row: r, Column: c}, nil
			}
		}
		return document.CellCoordinate{Row: r, Column: 1}, nil
	}
	return current, nil
}

// SearchSheet searches cell values for query.
func (d *Document) SearchSheet(sheet, query string) ([]document.CellMatch, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if query == "" {
		return nil, nil
	}

	lowerQuery := strings.ToLower(query)
	var matches []document.CellMatch

	limitRows := d.maxRow
	if limitRows > 50000 {
		limitRows = 50000 // Cap search on extremely large files to prevent UI stall
	}

	for r := 1; r <= limitRows; r++ {
		for c := 1; c <= d.maxCol; c++ {
			val := d.getCellValueLocked(r, c)
			if strings.Contains(strings.ToLower(val), lowerQuery) {
				matches = append(matches, document.CellMatch{
					Row:    r,
					Column: c,
					Value:  val,
				})
				if len(matches) >= 500 {
					return matches, nil
				}
			}
		}
	}
	return matches, nil
}

// SortRangeChanged sorts rows in bounds by key column and reports whether content changed.
func (d *Document) SortRangeChanged(req document.SortRequest) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	b := req.Bounds
	rowCount := b.EndRow - b.StartRow + 1
	if rowCount <= 1 {
		return false, nil
	}
	if rowCount > 100000 {
		return false, fmt.Errorf("当前仅支持排序不超过 100,000 行的数据区域")
	}

	type sortRow struct {
		origIndex int
		keyVal    string
		fields    []string
	}

	rows := make([]sortRow, rowCount)
	for r := 0; r < rowCount; r++ {
		rowIdx := b.StartRow + r
		fields := make([]string, b.EndColumn-b.StartColumn+1)
		for c := 0; c < len(fields); c++ {
			fields[c] = d.getCellValueLocked(rowIdx, b.StartColumn+c)
		}
		keyVal := d.getCellValueLocked(rowIdx, req.KeyColumn)
		rows[r] = sortRow{
			origIndex: rowIdx,
			keyVal:    keyVal,
			fields:    fields,
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		cmp := strings.Compare(rows[i].keyVal, rows[j].keyVal)
		if req.Descending {
			return cmp > 0
		}
		return cmp < 0
	})

	var batch []editAction
	for r := 0; r < rowCount; r++ {
		targetRow := b.StartRow + r
		for c := 0; c < len(rows[r].fields); c++ {
			colIdx := b.StartColumn + c
			newVal := rows[r].fields[c]
			oldVal := d.getCellValueLocked(targetRow, colIdx)
			if oldVal != newVal {
				d.edits[cellKey{row: targetRow, col: colIdx}] = newVal
				batch = append(batch, editAction{
					row:    targetRow,
					col:    colIdx,
					oldVal: oldVal,
					newVal: newVal,
				})
			}
		}
	}

	if len(batch) == 0 {
		return false, nil
	}

	d.undoStack = append(d.undoStack, editBatchAction{edits: batch})
	d.redoStack = nil
	d.dirty = true
	return true, nil
}

// SortRange implements document.SpreadsheetSorter.
func (d *Document) SortRange(req document.SortRequest) error {
	_, err := d.SortRangeChanged(req)
	return err
}
