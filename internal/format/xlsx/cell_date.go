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

var _ document.SpreadsheetCellDateEditor = (*Workbook)(nil)

// SetCellDateChanged writes a static, date-only value to one cell as a single
// undoable edit. A cell's existing non-General style is retained exactly.
func (w *Workbook) SetCellDateChanged(sheet, cell string, date time.Time) (bool, error) {
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return false, fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if column < 1 || row < 1 || column > maxXLSXColumns || row > maxXLSXRows {
		return false, fmt.Errorf("cell reference %q is outside XLSX worksheet limits", cell)
	}
	year, month, day := date.Date()
	date = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if year < 1900 || year > 9999 || date.Year() != year || date.Month() != month || date.Day() != day {
		return false, fmt.Errorf("date is outside XLSX date limits")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return false, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	props, err := w.file.GetWorkbookProps()
	if err != nil {
		return false, fmt.Errorf("read workbook date system: %w", err)
	}
	date1904 := props.Date1904 != nil && *props.Date1904
	if date1904 && date.Before(time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		return false, fmt.Errorf("date is before the workbook's 1904 date system")
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
	if styleID != 0 {
		style, styleErr := w.file.GetStyle(styleID)
		if styleErr != nil {
			return false, fmt.Errorf("read cell style details: %w", styleErr)
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
			if serial, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil && !math.IsNaN(serial) && !math.IsInf(serial, 0) {
				if current, dateErr := excelize.ExcelDateToTime(serial, date1904); dateErr == nil && current.Equal(date) {
					return false, nil
				}
			}
		}
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	if err := w.file.SetCellValue(sheet, cell, date); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return false, fmt.Errorf("write static date: %w", err)
	}
	if !generalFormat {
		if err := w.file.SetCellStyle(sheet, cell, cell, styleID); err != nil {
			_ = w.restoreSnapshotLocked(before)
			return false, fmt.Errorf("preserve cell style: %w", err)
		}
	}
	if err := w.recordHistoryLocked(before); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return false, err
	}
	w.dirty = true
	return true, nil
}
