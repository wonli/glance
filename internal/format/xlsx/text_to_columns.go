package xlsx

import (
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.SpreadsheetTextToColumnsEditor = (*Workbook)(nil)

// SplitTextToColumns splits cell values in keyColumn for each row within bounds by delimiter,
// and writes the resulting tokens into keyColumn, keyColumn+1, ...
func (w *Workbook) SplitTextToColumns(sheet string, bounds document.CellRange, keyColumn int, delimiter string) (document.SpreadsheetTextToColumnsResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetTextToColumnsResult{}, err
	}
	if delimiter == "" {
		return document.SpreadsheetTextToColumnsResult{}, errors.New("delimiter cannot be empty")
	}
	if keyColumn <= 0 {
		keyColumn = bounds.StartColumn
	}
	if bounds.StartRow <= 0 || bounds.EndRow < bounds.StartRow {
		return document.SpreadsheetTextToColumnsResult{}, fmt.Errorf("invalid row bounds %d..%d", bounds.StartRow, bounds.EndRow)
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return document.SpreadsheetTextToColumnsResult{}, err
	}

	rowsSplit := 0
	maxCols := 1

	for r := bounds.StartRow; r <= bounds.EndRow; r++ {
		cell := CellName(keyColumn, r)
		val, _ := w.file.GetCellValue(sheet, cell)
		if val == "" || !strings.Contains(val, delimiter) {
			continue
		}
		parts := strings.Split(val, delimiter)
		rowsSplit++
		if len(parts) > maxCols {
			maxCols = len(parts)
		}
		for i, part := range parts {
			col := keyColumn + i
			targetCell := CellName(col, r)
			if err := w.file.SetCellValue(sheet, targetCell, part); err != nil {
				_ = w.restoreSnapshotLocked(beforeSnapshot)
				return document.SpreadsheetTextToColumnsResult{}, fmt.Errorf("set cell %s: %w", targetCell, err)
			}
		}
	}

	if rowsSplit == 0 {
		return document.SpreadsheetTextToColumnsResult{
			RowsSplit: 0,
			MaxCols:   1,
		}, nil
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return document.SpreadsheetTextToColumnsResult{}, err
	}
	w.dirty = true

	return document.SpreadsheetTextToColumnsResult{
		RowsSplit: rowsSplit,
		MaxCols:   maxCols,
	}, nil
}
