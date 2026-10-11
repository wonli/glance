package xlsx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"

	"glance/internal/document"
)

var _ document.SpreadsheetFlashFillEditor = (*Workbook)(nil)

// FlashFill extrapolates series or repeats values into empty cells in the selection.
func (w *Workbook) FlashFill(sheet string, selection document.CellRange) (document.SpreadsheetFlashFillResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetFlashFillResult{}, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return document.SpreadsheetFlashFillResult{}, fmt.Errorf("worksheet %q does not exist", sheet)
	}

	minCol := min(selection.StartColumn, selection.EndColumn)
	maxCol := max(selection.StartColumn, selection.EndColumn)
	minRow := min(selection.StartRow, selection.EndRow)
	maxRow := max(selection.StartRow, selection.EndRow)

	if minCol < 1 || minRow < 1 || maxCol > maxXLSXColumns || maxRow > maxXLSXRows {
		return document.SpreadsheetFlashFillResult{}, errors.New("selection bounds exceed worksheet limits")
	}

	type cellFill struct {
		cell  string
		value interface{}
	}
	var fills []cellFill
	hasSourcePattern := false

	for col := minCol; col <= maxCol; col++ {
		type rowVal struct {
			row int
			val string
		}
		var present []rowVal
		for r := minRow; r <= maxRow; r++ {
			cellName, err := excelize.CoordinatesToCellName(col, r)
			if err != nil {
				continue
			}
			val, _ := w.file.GetCellValue(sheet, cellName)
			if strings.TrimSpace(val) != "" {
				present = append(present, rowVal{row: r, val: strings.TrimSpace(val)})
			}
		}

		if len(present) == 0 && minRow > 1 {
			aboveRow := minRow - 1
			cellName, _ := excelize.CoordinatesToCellName(col, aboveRow)
			val, _ := w.file.GetCellValue(sheet, cellName)
			if strings.TrimSpace(val) != "" {
				present = append(present, rowVal{row: aboveRow, val: strings.TrimSpace(val)})
				if aboveRow > 1 {
					aboveRow2 := aboveRow - 1
					cellName2, _ := excelize.CoordinatesToCellName(col, aboveRow2)
					val2, _ := w.file.GetCellValue(sheet, cellName2)
					if strings.TrimSpace(val2) != "" {
						present = append([]rowVal{{row: aboveRow2, val: strings.TrimSpace(val2)}}, present...)
					}
				}
			}
		}

		if len(present) == 0 {
			continue
		}
		hasSourcePattern = true

		firstRow := present[0].row
		firstVal := present[0].val

		if len(present) >= 2 && present[1].row == firstRow+1 {
			f1, err1 := strconv.ParseFloat(firstVal, 64)
			f2, err2 := strconv.ParseFloat(present[1].val, 64)
			if err1 == nil && err2 == nil {
				step := f2 - f1
				startRow := present[1].row + 1
				for r := startRow; r <= maxRow; r++ {
					if r < minRow {
						continue
					}
					cellName, _ := excelize.CoordinatesToCellName(col, r)
					existing, _ := w.file.GetCellValue(sheet, cellName)
					if strings.TrimSpace(existing) == "" {
						idx := float64(r - present[1].row)
						nextVal := f2 + step*idx
						fills = append(fills, cellFill{cell: cellName, value: formatNumeric(nextVal, step)})
					}
				}
				continue
			}
		}

		if f1, err := strconv.ParseFloat(firstVal, 64); err == nil {
			startRow := firstRow + 1
			for r := startRow; r <= maxRow; r++ {
				if r < minRow {
					continue
				}
				cellName, _ := excelize.CoordinatesToCellName(col, r)
				existing, _ := w.file.GetCellValue(sheet, cellName)
				if strings.TrimSpace(existing) == "" {
					idx := float64(r - firstRow)
					nextVal := f1 + idx
					fills = append(fills, cellFill{cell: cellName, value: formatNumeric(nextVal, 1)})
				}
			}
			continue
		}

		if prefix, num, digits, ok := parseTrailingDigits(firstVal); ok {
			startRow := firstRow + 1
			for r := startRow; r <= maxRow; r++ {
				if r < minRow {
					continue
				}
				cellName, _ := excelize.CoordinatesToCellName(col, r)
				existing, _ := w.file.GetCellValue(sheet, cellName)
				if strings.TrimSpace(existing) == "" {
					idx := int64(r - firstRow)
					nextNum := num + idx
					formatStr := fmt.Sprintf("%%s%%0%dd", digits)
					nextVal := fmt.Sprintf(formatStr, prefix, nextNum)
					fills = append(fills, cellFill{cell: cellName, value: nextVal})
				}
			}
			continue
		}

		startRow := firstRow + 1
		for r := startRow; r <= maxRow; r++ {
			if r < minRow {
				continue
			}
			cellName, _ := excelize.CoordinatesToCellName(col, r)
			existing, _ := w.file.GetCellValue(sheet, cellName)
			if strings.TrimSpace(existing) == "" {
				fills = append(fills, cellFill{cell: cellName, value: firstVal})
			}
		}
	}

	if len(fills) == 0 {
		if hasSourcePattern {
			return document.SpreadsheetFlashFillResult{RowsFilled: 0}, nil
		}
		return document.SpreadsheetFlashFillResult{RowsFilled: 0}, errors.New("no pattern or source data found to fill")
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return document.SpreadsheetFlashFillResult{}, fmt.Errorf("capture flash fill history: %w", err)
	}

	for _, item := range fills {
		if err := w.file.SetCellValue(sheet, item.cell, item.value); err != nil {
			_ = w.restoreSnapshotLocked(beforeSnapshot)
			return document.SpreadsheetFlashFillResult{}, err
		}
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return document.SpreadsheetFlashFillResult{}, err
	}

	w.dirty = true
	return document.SpreadsheetFlashFillResult{RowsFilled: len(fills)}, nil
}

func formatNumeric(val float64, step float64) interface{} {
	if val == float64(int64(val)) && step == float64(int64(step)) {
		return int64(val)
	}
	return val
}

func parseTrailingDigits(s string) (prefix string, num int64, digits int, ok bool) {
	if s == "" {
		return "", 0, 0, false
	}
	runes := []rune(s)
	end := len(runes)
	for end > 0 && unicode.IsDigit(runes[end-1]) {
		end--
	}
	if end == len(runes) {
		return "", 0, 0, false
	}
	digitStr := string(runes[end:])
	n, err := strconv.ParseInt(digitStr, 10, 64)
	if err != nil {
		return "", 0, 0, false
	}
	return string(runes[:end]), n, len(digitStr), true
}
