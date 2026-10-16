package xlsx

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetFormulaRangeEntryEditor = (*Workbook)(nil)

func (w *Workbook) SetCellFormulaRangeChanged(sheet string, active document.CellCoordinate, bounds document.CellRange, formula string) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if err := validateRange(sheet, bounds, w.file.GetSheetList()); err != nil {
		return false, err
	}
	if active.Row < bounds.StartRow || active.Row > bounds.EndRow || active.Column < bounds.StartColumn || active.Column > bounds.EndColumn {
		return false, errors.New("active cell must be inside the selected formula range")
	}
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	if rows > maxClipboardCells/columns {
		return false, fmt.Errorf("formula range exceeds %d cells", maxClipboardCells)
	}
	if !utf8.ValidString(formula) || !withinFormulaUTF16Limit(formula) {
		return false, errors.New("formula is invalid UTF-8 or exceeds 8192 UTF-16 units")
	}
	formula = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(formula), "="))
	if formula == "" {
		return false, errors.New("formula cannot be empty")
	}
	definedNames := make(map[string]bool)
	for _, name := range w.file.GetDefinedName() {
		definedNames[strings.ToLower(name.Name)] = true
	}
	groups, err := w.clipboardFormulaGroupsLocked(sheet)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if rangesIntersect(group.bounds, bounds) {
			return false, fmt.Errorf("formula entry would modify part of a %s formula range %s", group.kind, group.ref)
		}
	}
	prepared := make([]document.CellFormulaEdit, 0, rows*columns)
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			translated, err := translateClipboardFormula(formula, row-active.Row, column-active.Column, definedNames)
			if err != nil {
				return false, fmt.Errorf("adjust formula for cell (%d,%d): %w", row, column, err)
			}
			translated = normalizeFormulaFunctionNames(translated)
			if !withinFormulaUTF16Limit(translated) {
				return false, fmt.Errorf("adjusted formula for cell (%d,%d) exceeds 8192 UTF-16 units", row, column)
			}
			prepared = append(prepared, document.CellFormulaEdit{Row: row, Column: column, Formula: translated})
		}
	}
	changed := make([]document.CellFormulaEdit, 0, len(prepared))
	for _, edit := range prepared {
		ref, err := excelize.CoordinatesToCellName(edit.Column, edit.Row)
		if err != nil {
			return false, err
		}
		current, err := w.file.GetCellFormula(sheet, ref)
		if err != nil {
			return false, fmt.Errorf("read formula %s: %w", ref, err)
		}
		if current != edit.Formula {
			changed = append(changed, edit)
		}
	}
	if len(changed) == 0 {
		return false, nil
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, fmt.Errorf("capture formula range history: %w", err)
	}
	for _, edit := range changed {
		ref, _ := excelize.CoordinatesToCellName(edit.Column, edit.Row)
		if err := w.file.SetCellFormula(sheet, ref, edit.Formula); err != nil {
			if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
				return false, errors.Join(fmt.Errorf("write formula %s: %w", ref, err), fmt.Errorf("restore workbook after formula range entry: %w", rollbackErr))
			}
			return false, fmt.Errorf("write formula %s: %w", ref, err)
		}
	}
	_ = w.file.UpdateLinkedValue()
	if err := w.recordHistoryLocked(before); err != nil {
		if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
			return false, errors.Join(fmt.Errorf("record formula range history: %w", err), fmt.Errorf("restore workbook after formula range entry: %w", rollbackErr))
		}
		return false, fmt.Errorf("record formula range history: %w", err)
	}
	w.dirty = true
	return true, nil
}
