package csv

import (
	"errors"

	"glance/internal/document"
	"glance/internal/textintern"
)

const (
	defaultRowHeight   = 15.0
	defaultColumnWidth = 10.0
)

// getCellValueLocked returns the cell value at 1-based (row, col). Caller must hold d.mu.
func (d *Document) getCellValueLocked(row, col int) string {
	if row < 1 || col < 1 {
		return ""
	}
	if val, ok := d.edits[cellKey{row: row, col: col}]; ok {
		return val
	}
	if d.file != nil && d.index != nil {
		if row <= d.index.totalRows {
			records, err := d.index.readRowRange(d.file, row-1, 1)
			if err == nil && len(records) > 0 && col <= len(records[0]) {
				return records[0][col-1]
			}
		}
		return ""
	}
	if d.records != nil {
		if row <= len(d.records) && col <= len(d.records[row-1]) {
			return d.records[row-1][col-1]
		}
	}
	return ""
}

// Window returns a bounded viewport of cells and dimensions for the virtual grid.
func (d *Document) Window(sheet string, startRow, startColumn, rowCount, columnCount int) (document.SheetWindow, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return document.SheetWindow{}, errors.New("document is closed")
	}

	if startRow < 1 {
		startRow = 1
	}
	if startColumn < 1 {
		startColumn = 1
	}
	if rowCount < 0 {
		rowCount = 0
	}
	if columnCount < 0 {
		columnCount = 0
	}

	cells := make([][]document.Cell, rowCount)
	rowHeights := make([]float64, rowCount)
	columnWidths := make([]float64, columnCount)

	for c := 0; c < columnCount; c++ {
		colIdx := startColumn + c
		if w, ok := d.colWidths[colIdx]; ok {
			columnWidths[c] = w
		} else {
			columnWidths[c] = defaultColumnWidth
		}
	}

	// Pre-fetch rows from disk index if available
	var diskRows [][]string
	if d.file != nil && d.index != nil && rowCount > 0 {
		diskRows, _ = d.index.readRowRange(d.file, startRow-1, rowCount)
	}

	defaultStyle := document.CellStyle{
		FontFamily: textintern.String("Aptos Narrow"),
		FontSize:   11,
	}

	for r := 0; r < rowCount; r++ {
		rowIdx := startRow + r
		if h, ok := d.rowHeights[rowIdx]; ok {
			rowHeights[r] = h
		} else {
			rowHeights[r] = defaultRowHeight
		}

		cells[r] = make([]document.Cell, columnCount)
		for c := 0; c < columnCount; c++ {
			colIdx := startColumn + c
			val := ""

			if editedVal, ok := d.edits[cellKey{row: rowIdx, col: colIdx}]; ok {
				val = editedVal
			} else if diskRows != nil {
				if r < len(diskRows) && colIdx <= len(diskRows[r]) {
					val = diskRows[r][colIdx-1]
				}
			} else if d.records != nil {
				if rowIdx <= len(d.records) && colIdx <= len(d.records[rowIdx-1]) {
					val = d.records[rowIdx-1][colIdx-1]
				}
			}

			cells[r][c] = document.Cell{
				Value: textintern.String(val),
				Style: defaultStyle,
			}
		}
	}

	return document.SheetWindow{
		StartRow:          startRow,
		StartColumn:       startColumn,
		Cells:             cells,
		RowHeightsPoints:  rowHeights,
		ColumnWidthsChars: columnWidths,
	}, nil
}

// SetRowHeight sets the point height for one row.
func (d *Document) SetRowHeight(sheet string, row int, heightPoints float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	d.rowHeights[row] = heightPoints
	return nil
}

// SetColumnWidth sets the character width for one column.
func (d *Document) SetColumnWidth(sheet string, column int, widthChars float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	d.colWidths[column] = widthChars
	return nil
}

// Range returns the top-left cell matrix up to maxRows and maxColumns.
func (d *Document) Range(sheet string, maxRows, maxColumns int) ([][]document.Cell, error) {
	window, err := d.Window(sheet, 1, 1, maxRows, maxColumns)
	if err != nil {
		return nil, err
	}
	return window.Cells, nil
}

// ReadRange reads a bounded cell range.
func (d *Document) ReadRange(sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	rowCount := bounds.EndRow - bounds.StartRow + 1
	colCount := bounds.EndColumn - bounds.StartColumn + 1
	window, err := d.Window(sheet, bounds.StartRow, bounds.StartColumn, rowCount, colCount)
	if err != nil {
		return nil, err
	}
	return window.Cells, nil
}
