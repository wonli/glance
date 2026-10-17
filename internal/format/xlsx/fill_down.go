package xlsx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetFillDownEditor = (*Workbook)(nil)
var _ document.SpreadsheetFillRightEditor = (*Workbook)(nil)

type fillCell struct {
	ref     string
	formula string
	value   document.Cell
}

// FillDownChanged repeats the first row of bounds through the selected rows.
// Formula references are translated relative to their source cell. All cells
// are prepared before mutation, so unsupported structures and invalid formula
// translations leave the workbook untouched.
func (w *Workbook) FillDownChanged(sheet string, bounds document.CellRange) (bool, error) {
	return w.fillFromEdgeChanged(sheet, bounds, true)
}

// FillRightChanged repeats the first column of bounds through the selected
// columns, translating formula references relative to each source cell.
func (w *Workbook) FillRightChanged(sheet string, bounds document.CellRange) (bool, error) {
	return w.fillFromEdgeChanged(sheet, bounds, false)
}

func (w *Workbook) fillFromEdgeChanged(sheet string, bounds document.CellRange, down bool) (bool, error) {
	direction := "fill right"
	if down {
		direction = "fill down"
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if err := validateRange(sheet, bounds, w.file.GetSheetList()); err != nil {
		return false, err
	}
	rows := bounds.EndRow - bounds.StartRow + 1
	columns := bounds.EndColumn - bounds.StartColumn + 1
	targets, sourceCount := columns, rows
	if down {
		targets, sourceCount = rows, columns
	}
	if targets < 2 {
		return false, nil
	}
	if sourceCount > maxClipboardCells/targets {
		return false, fmt.Errorf("%s exceeds %d cells", direction, maxClipboardCells)
	}

	merges, err := w.file.GetMergeCells(sheet)
	if err != nil {
		return false, fmt.Errorf("read merged ranges: %w", err)
	}
	for _, merge := range merges {
		mergeBounds, err := formulaReferenceBounds(merge.GetStartAxis() + ":" + merge.GetEndAxis())
		if err != nil {
			return false, fmt.Errorf("identify merged range: %w", err)
		}
		if rangesIntersect(mergeBounds, bounds) {
			return false, fmt.Errorf("%s does not support merged cells in %s:%s", direction, merge.GetStartAxis(), merge.GetEndAxis())
		}
	}

	groups, err := w.clipboardFormulaGroupsLocked(sheet)
	if err != nil {
		return false, fmt.Errorf("inspect formula groups: %w", err)
	}
	for _, group := range groups {
		if rangesIntersect(group.bounds, bounds) {
			return false, fmt.Errorf("%s would modify part of a %s formula range %s", direction, group.kind, group.ref)
		}
	}

	definedNames := make(map[string]bool)
	for _, name := range w.file.GetDefinedName() {
		definedNames[strings.ToLower(name.Name)] = true
	}
	source := make([]document.Cell, sourceCount)
	for sourceOffset := 0; sourceOffset < sourceCount; sourceOffset++ {
		row, column := bounds.StartRow, bounds.StartColumn
		if down {
			column += sourceOffset
		} else {
			row += sourceOffset
		}
		ref, err := excelize.CoordinatesToCellName(column, row)
		if err != nil {
			return false, err
		}
		cell, err := readFillCell(w.file, sheet, ref)
		if err != nil {
			return false, fmt.Errorf("read %s source %s: %w", direction, ref, err)
		}
		if cell.Formula == "" && cell.TypedValue == nil && cell.Value != "" {
			return false, fmt.Errorf("%s source %s contains a value type that cannot be preserved safely", direction, ref)
		}
		source[sourceOffset] = cell
	}

	prepared := make([]fillCell, 0, rows*columns)
	changed := false
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			sourceOffset, rowOffset, columnOffset := column-bounds.StartColumn, row-bounds.StartRow, 0
			if !down {
				sourceOffset, rowOffset, columnOffset = row-bounds.StartRow, 0, column-bounds.StartColumn
			}
			sourceCell := source[sourceOffset]
			ref, err := excelize.CoordinatesToCellName(column, row)
			if err != nil {
				return false, err
			}
			cell := sourceCell
			if sourceCell.Formula != "" {
				cell.Formula, err = translateClipboardFormula(sourceCell.Formula, rowOffset, columnOffset, definedNames)
				if err != nil {
					return false, fmt.Errorf("adjust %s formula for %s: %w", direction, ref, err)
				}
				cell.Formula = normalizeFormulaFunctionNames(cell.Formula)
				if !withinFormulaUTF16Limit(cell.Formula) {
					return false, fmt.Errorf("%s formula for %s exceeds 8192 UTF-16 units", direction, ref)
				}
			}
			prepared = append(prepared, fillCell{ref: ref, formula: cell.Formula, value: cell})
			current, err := readFillCell(w.file, sheet, ref)
			if err != nil {
				return false, fmt.Errorf("compare %s target %s: %w", direction, ref, err)
			}
			if !sameFillCell(current, cell) {
				changed = true
			}
		}
	}
	if !changed {
		return false, nil
	}

	before, err := w.snapshotLocked()
	if err != nil {
		return false, fmt.Errorf("capture %s history: %w", direction, err)
	}
	rollback := func(cause error) (bool, error) {
		if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
			return false, fmt.Errorf("%w; restoring workbook after failed %s: %v", cause, direction, rollbackErr)
		}
		return false, cause
	}
	for _, edit := range prepared {
		if err := w.file.SetCellFormula(sheet, edit.ref, ""); err != nil {
			return rollback(fmt.Errorf("clear %s target formula %s: %w", direction, edit.ref, err))
		}
		if edit.formula != "" {
			if err := w.file.SetCellFormula(sheet, edit.ref, edit.formula); err != nil {
				return rollback(fmt.Errorf("%s formula %s: %w", direction, edit.ref, err))
			}
		} else {
			value, err := clipboardCellValue(edit.value)
			if err != nil {
				return rollback(fmt.Errorf("%s value %s: %w", direction, edit.ref, err))
			}
			if err := w.file.SetCellValue(sheet, edit.ref, value); err != nil {
				return rollback(fmt.Errorf("%s value %s: %w", direction, edit.ref, err))
			}
		}
		if err := w.file.SetCellStyle(sheet, edit.ref, edit.ref, edit.value.StyleID); err != nil {
			return rollback(fmt.Errorf("%s style %s: %w", direction, edit.ref, err))
		}
	}
	if err := w.file.UpdateLinkedValue(); err != nil {
		return rollback(fmt.Errorf("update %s formula results: %w", direction, err))
	}
	if err := w.recordHistoryLocked(before); err != nil {
		return rollback(fmt.Errorf("record %s history: %w", direction, err))
	}
	w.dirty = true
	return true, nil
}

func readFillCell(file *excelize.File, sheet, ref string) (document.Cell, error) {
	formula, err := file.GetCellFormula(sheet, ref)
	if err != nil {
		return document.Cell{}, err
	}
	styleID, err := file.GetCellStyle(sheet, ref)
	if err != nil {
		return document.Cell{}, err
	}
	cell := document.Cell{Formula: formula, StyleID: styleID}
	if formula != "" {
		return cell, nil
	}
	cellType, err := file.GetCellType(sheet, ref)
	if err != nil {
		return document.Cell{}, err
	}
	cell.Value, err = file.GetCellValue(sheet, ref)
	if err != nil {
		return document.Cell{}, err
	}
	if cellType == excelize.CellTypeDate {
		raw, err := file.GetCellValue(sheet, ref, excelize.Options{RawCellValue: true})
		if err != nil {
			return document.Cell{}, err
		}
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil || !finiteNumber(number) {
			return document.Cell{}, fmt.Errorf("invalid date serial %q", raw)
		}
		cell.TypedValue = &document.CellValue{Kind: document.CellValueNumber, Number: number}
		return cell, nil
	}
	cell.TypedValue, err = readClipboardTypedValue(file, sheet, ref)
	if err != nil {
		return document.Cell{}, err
	}
	return cell, nil
}

func sameFillCell(a, b document.Cell) bool {
	if a.Formula != b.Formula || a.StyleID != b.StyleID {
		return false
	}
	if a.Formula != "" {
		return true
	}
	return a.Value == b.Value && sameClipboardValue(a.TypedValue, b.TypedValue)
}
