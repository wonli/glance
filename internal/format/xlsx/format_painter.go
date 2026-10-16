package xlsx

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetCellFormatPainter = (*Workbook)(nil)

func (w *Workbook) PaintCellFormat(sheet, sourceCell string, targetBounds document.CellRange) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if err := validateRange(sheet, targetBounds, w.file.GetSheetList()); err != nil {
		return false, err
	}
	rows := targetBounds.EndRow - targetBounds.StartRow + 1
	columns := targetBounds.EndColumn - targetBounds.StartColumn + 1
	if rows*columns > maxClipboardCells {
		return false, fmt.Errorf("format paint range exceeds %d cells", maxClipboardCells)
	}
	sourceCell = strings.ToUpper(strings.TrimSpace(sourceCell))
	column, row, err := excelize.CellNameToCoordinates(sourceCell)
	if err != nil || row < 1 || column < 1 || row > maxXLSXRows || column > maxXLSXColumns {
		return false, fmt.Errorf("invalid source cell %q", sourceCell)
	}
	sourceStyle, err := w.file.GetCellStyle(sheet, sourceCell)
	if err != nil {
		return false, fmt.Errorf("read source style for %s: %w", sourceCell, err)
	}
	changed := false
	for targetRow := targetBounds.StartRow; targetRow <= targetBounds.EndRow && !changed; targetRow++ {
		for targetColumn := targetBounds.StartColumn; targetColumn <= targetBounds.EndColumn; targetColumn++ {
			cell := CellName(targetColumn, targetRow)
			style, err := w.file.GetCellStyle(sheet, cell)
			if err != nil {
				return false, fmt.Errorf("read target style for %s: %w", cell, err)
			}
			if style != sourceStyle {
				changed = true
				break
			}
		}
	}
	if !changed {
		return false, nil
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, fmt.Errorf("capture format paint history: %w", err)
	}
	first := CellName(targetBounds.StartColumn, targetBounds.StartRow)
	last := CellName(targetBounds.EndColumn, targetBounds.EndRow)
	if err := w.file.SetCellStyle(sheet, first, last, sourceStyle); err != nil {
		if restoreErr := w.restoreSnapshotLocked(before); restoreErr != nil {
			return false, fmt.Errorf("set format paint range: %v (rollback failed: %w)", err, restoreErr)
		}
		return false, fmt.Errorf("set format paint range: %w", err)
	}
	if err := w.recordHistoryLocked(before); err != nil {
		if restoreErr := w.restoreSnapshotLocked(before); restoreErr != nil {
			return false, fmt.Errorf("record format paint history: %v (rollback failed: %w)", err, restoreErr)
		}
		return false, fmt.Errorf("record format paint history: %w", err)
	}
	w.dirty = true
	return true, nil
}
