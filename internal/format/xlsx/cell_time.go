package xlsx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetCellTimeEditor = (*Workbook)(nil)

// SetCellTimeChanged writes a static, minute-precision time value to one cell
// as a single undoable edit. A cell's existing non-General style is retained.
func (w *Workbook) SetCellTimeChanged(sheet, cell string, value time.Time) (bool, error) {
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return false, fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if column < 1 || row < 1 || column > maxXLSXColumns || row > maxXLSXRows {
		return false, fmt.Errorf("cell reference %q is outside XLSX worksheet limits", cell)
	}
	minutes := value.Hour()*60 + value.Minute()
	serial := float64(minutes) / float64(24*60)

	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return false, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	formula, err := w.file.GetCellFormula(sheet, cell)
	if err != nil {
		return false, fmt.Errorf("read cell formula: %w", err)
	}
	styleID, err := w.file.GetCellStyle(sheet, cell)
	if err != nil {
		return false, fmt.Errorf("read cell style: %w", err)
	}
	generalFormat := styleID == 0
	var style *excelize.Style
	if styleID != 0 {
		style, err = w.file.GetStyle(styleID)
		if err != nil {
			return false, fmt.Errorf("read cell style details: %w", err)
		}
		generalFormat = style.NumFmt == 0 && (style.CustomNumFmt == nil || strings.TrimSpace(*style.CustomNumFmt) == "")
	}
	if !generalFormat && formula == "" {
		cellType, typeErr := w.file.GetCellType(sheet, cell)
		if typeErr != nil {
			return false, fmt.Errorf("read cell value type: %w", typeErr)
		}
		if cellType == excelize.CellTypeUnset || cellType == excelize.CellTypeNumber || cellType == excelize.CellTypeDate {
			raw, rawErr := w.file.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
			if rawErr != nil {
				return false, fmt.Errorf("read cell value: %w", rawErr)
			}
			if current, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil && !math.IsNaN(current) && !math.IsInf(current, 0) && math.Abs(current-serial) < 1e-12 {
				return false, nil
			}
		}
	}

	before, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	if err := w.file.SetCellValue(sheet, cell, serial); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return false, fmt.Errorf("write static time: %w", err)
	}
	if generalFormat {
		if style == nil {
			style = &excelize.Style{}
		} else {
			style.NumFmt = 0
			style.CustomNumFmt = nil
		}
		style.NumFmt = 20 // Excel's built-in h:mm time format.
		newStyleID, styleErr := w.file.NewStyle(style)
		if styleErr != nil {
			_ = w.restoreSnapshotLocked(before)
			return false, fmt.Errorf("create default time style: %w", styleErr)
		}
		if styleErr = w.file.SetCellStyle(sheet, cell, cell, newStyleID); styleErr != nil {
			_ = w.restoreSnapshotLocked(before)
			return false, fmt.Errorf("apply default time style: %w", styleErr)
		}
	} else if err := w.file.SetCellStyle(sheet, cell, cell, styleID); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return false, fmt.Errorf("preserve cell style: %w", err)
	}
	if err := w.recordHistoryLocked(before); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return false, err
	}
	w.dirty = true
	return true, nil
}
