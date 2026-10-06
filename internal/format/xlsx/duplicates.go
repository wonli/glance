package xlsx

import (
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.SpreadsheetDuplicatesEditor = (*Workbook)(nil)

// RemoveDuplicates finds and deletes duplicate rows within bounds.
// If keyColumn > 0, uniqueness is determined by that column; otherwise by all columns in bounds.
// Rows are checked from StartRow to EndRow; earlier occurrences are preserved.
func (w *Workbook) RemoveDuplicates(sheet string, bounds document.CellRange, keyColumn int) (document.SpreadsheetDuplicatesResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetDuplicatesResult{}, err
	}
	if bounds.StartRow <= 0 || bounds.EndRow < bounds.StartRow {
		return document.SpreadsheetDuplicatesResult{}, fmt.Errorf("invalid row bounds %d..%d", bounds.StartRow, bounds.EndRow)
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return document.SpreadsheetDuplicatesResult{}, err
	}

	seen := make(map[string]bool)
	var duplicateRows []int

	for r := bounds.StartRow; r <= bounds.EndRow; r++ {
		var key string
		if keyColumn > 0 {
			cell := CellName(keyColumn, r)
			val, _ := w.file.GetCellValue(sheet, cell)
			key = val
		} else {
			var b strings.Builder
			for c := bounds.StartColumn; c <= bounds.EndColumn; c++ {
				cell := CellName(c, r)
				val, _ := w.file.GetCellValue(sheet, cell)
				b.WriteString(val)
				b.WriteByte(0)
			}
			key = b.String()
		}

		if seen[key] {
			duplicateRows = append(duplicateRows, r)
		} else {
			seen[key] = true
		}
	}

	if len(duplicateRows) == 0 {
		totalRows := bounds.EndRow - bounds.StartRow + 1
		return document.SpreadsheetDuplicatesResult{
			RemovedCount:   0,
			RemainingCount: totalRows,
		}, nil
	}

	// Remove duplicate rows in descending order so row removals do not shift preceding row indices
	for i := len(duplicateRows) - 1; i >= 0; i-- {
		rowToRemove := duplicateRows[i]
		if err := w.file.RemoveRow(sheet, rowToRemove); err != nil {
			_ = w.restoreSnapshotLocked(beforeSnapshot)
			return document.SpreadsheetDuplicatesResult{}, fmt.Errorf("remove row %d from sheet %q: %w", rowToRemove, sheet, err)
		}
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return document.SpreadsheetDuplicatesResult{}, err
	}
	w.dirty = true

	totalRows := bounds.EndRow - bounds.StartRow + 1
	removed := len(duplicateRows)
	return document.SpreadsheetDuplicatesResult{
		RemovedCount:   removed,
		RemainingCount: totalRows - removed,
	}, nil
}
