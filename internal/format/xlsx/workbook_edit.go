package xlsx

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (w *Workbook) SetRowHeight(sheet string, row int, heightPoints float64) error {
	return w.SetDimensionSizes(sheet, document.SpreadsheetDimensionRow, []document.SpreadsheetDimensionSize{{Index: row, Value: heightPoints}})
}

func (w *Workbook) SetColumnWidth(sheet string, column int, widthChars float64) error {
	return w.SetDimensionSizes(sheet, document.SpreadsheetDimensionColumn, []document.SpreadsheetDimensionSize{{Index: column, Value: widthChars}})
}

// SetDimensionSizes applies an entire set of row heights or column widths as
// one undoable edit. All input is validated before the workbook is changed.
func (w *Workbook) SetDimensionSizes(sheet string, axis document.SpreadsheetDimensionAxis, sizes []document.SpreadsheetDimensionSize) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	maxIndex := 0
	switch axis {
	case document.SpreadsheetDimensionRow:
		maxIndex = maxXLSXRows
	case document.SpreadsheetDimensionColumn:
		maxIndex = maxXLSXColumns
	default:
		return fmt.Errorf("unknown dimension axis %d", axis)
	}
	seen := make(map[int]struct{}, len(sizes))
	changed := make([]document.SpreadsheetDimensionSize, 0, len(sizes))
	for _, size := range sizes {
		if size.Index < 1 || size.Index > maxIndex {
			if axis == document.SpreadsheetDimensionRow {
				return fmt.Errorf("row %d outside [1,%d]", size.Index, maxIndex)
			}
			return fmt.Errorf("column %d outside [1,%d]", size.Index, maxIndex)
		}
		if _, duplicate := seen[size.Index]; duplicate {
			return fmt.Errorf("dimension index %d appears more than once", size.Index)
		}
		seen[size.Index] = struct{}{}
		limit := 409.5
		name := "row height"
		if axis == document.SpreadsheetDimensionColumn {
			limit = 255
			name = "column width"
		}
		if !finiteNumber(size.Value) || size.Value <= 0 || size.Value > limit {
			return fmt.Errorf("%s must be finite and within (0,%v]: %v", name, limit, size.Value)
		}
		var current float64
		var err error
		if axis == document.SpreadsheetDimensionRow {
			current, err = w.file.GetRowHeight(sheet, size.Index)
		} else {
			columnName, nameErr := excelize.ColumnNumberToName(size.Index)
			if nameErr != nil {
				return fmt.Errorf("resolve column %d: %w", size.Index, nameErr)
			}
			current, err = w.file.GetColWidth(sheet, columnName)
		}
		if err != nil {
			return fmt.Errorf("read dimension %d size: %w", size.Index, err)
		}
		if current != size.Value {
			changed = append(changed, size)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture dimension history: %w", err)
	}
	historyState := w.historyState
	for _, size := range changed {
		if axis == document.SpreadsheetDimensionRow {
			err = w.file.SetRowHeight(sheet, size.Index, size.Value)
		} else {
			columnName, nameErr := excelize.ColumnNumberToName(size.Index)
			if nameErr != nil {
				return w.rollbackDimensionChangeLocked(before, nameErr)
			}
			err = w.file.SetColWidth(sheet, columnName, columnName, size.Value)
		}
		if err != nil {
			return w.rollbackDimensionChangeLocked(before, err)
		}
	}
	if err := w.recordHistoryLocked(before); err != nil {
		return w.rollbackDimensionChangeLocked(before, err)
	}
	if w.historyState != historyState {
		w.dirty = true
	}
	return nil
}

func (w *Workbook) rollbackDimensionChangeLocked(before []byte, cause error) error {
	if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("restore workbook after dimension change: %w", rollbackErr))
	}
	return cause
}

// SetRowHeightAsPartOfLatestEdit adds an automatically adjusted row height to
// the latest workbook edit, keeping cell editing, undo, and redo as one step.
func (w *Workbook) SetRowHeightAsPartOfLatestEdit(sheet string, row int, heightPoints float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if row < 1 || row > maxXLSXRows {
		return fmt.Errorf("row %d outside [1,%d]", row, maxXLSXRows)
	}
	if !finiteNumber(heightPoints) || heightPoints <= 0 || heightPoints > 409.5 {
		return fmt.Errorf("row height must be finite and within (0,409.5]: %v", heightPoints)
	}
	current, err := w.file.GetRowHeight(sheet, row)
	if err != nil {
		return err
	}
	if current == heightPoints {
		return nil
	}
	if len(w.undo) == 0 || w.undo[len(w.undo)-1].afterState != w.historyState {
		return errors.New("there is no latest workbook edit to group row height with")
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture grouped row-height change: %w", err)
	}
	latest := &w.undo[len(w.undo)-1]
	if !bytes.Equal(latest.after, before) {
		return errors.New("latest workbook edit no longer matches the current workbook")
	}
	if err := w.file.SetRowHeight(sheet, row, heightPoints); err != nil {
		return w.rollbackDimensionChangeLocked(before, err)
	}
	after, err := w.snapshotLocked()
	if err != nil {
		return w.rollbackDimensionChangeLocked(before, fmt.Errorf("capture grouped row-height result: %w", err))
	}
	if bytes.Equal(before, after) {
		return w.rollbackDimensionChangeLocked(before, nil)
	}
	latest.after = after
	w.dirty = true
	return nil
}

// SetCellText preserves the spreadsheet editor's existing input behavior:
// finite numeric strings and true/false become typed cell values, while other
// strings remain text. Agent commands use SetCellValueChanged when the caller
// supplied an explicit JSON value type.
func (w *Workbook) SetCellText(sheet, cell, value string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if strings.HasPrefix(strings.TrimSpace(value), "=") {
		return errors.New("formula input is not supported")
	}
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if row > maxXLSXRows || column > maxXLSXColumns {
		return fmt.Errorf("cell reference %q exceeds XLSX worksheet limits", cell)
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	current, err := w.file.GetCellValue(sheet, cell)
	if err != nil {
		return err
	}
	if current == value {
		return nil
	}
	var typedValue any = value
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TRUE":
		typedValue = true
	case "FALSE":
		typedValue = false
	default:
		if number, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
			if math.IsInf(number, 0) || math.IsNaN(number) {
				return errors.New("numeric cell values must be finite")
			}
			typedValue = number
		} else if strings.EqualFold(strings.TrimSpace(value), "nan") || strings.EqualFold(strings.TrimSpace(value), "inf") || strings.EqualFold(strings.TrimSpace(value), "+inf") || strings.EqualFold(strings.TrimSpace(value), "-inf") {
			return errors.New("numeric cell values must be finite")
		}
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture cell edit history: %w", err)
	}
	if err := w.file.SetCellValue(sheet, cell, typedValue); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

// RecalculateFormulas recalculates formulas and updates cached values in the workbook.
func (w *Workbook) RecalculateFormulas() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if err := w.file.UpdateLinkedValue(); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// SetCellFormula sets the formula expression for a cell and updates linked values.
func (w *Workbook) SetCellFormula(sheet, cell, formula string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	cleanFormula := strings.TrimSpace(formula)
	cleanFormula = strings.TrimPrefix(cleanFormula, "=")
	if cleanFormula == "" {
		return errors.New("formula expression cannot be empty")
	}
	cleanFormula = normalizeFormulaFunctionNames(cleanFormula)
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if row > maxXLSXRows || column > maxXLSXColumns {
		return fmt.Errorf("cell reference %q exceeds XLSX worksheet limits", cell)
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	currentFormula, _ := w.file.GetCellFormula(sheet, cell)
	if currentFormula == cleanFormula {
		return nil
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture formula edit history: %w", err)
	}
	if err := w.file.SetCellFormula(sheet, cell, cleanFormula); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	_ = w.file.UpdateLinkedValue()
	w.dirty = true
	return nil
}

// SetCellFormulasChanged writes multiple formulas as one undoable workbook edit.
// The complete batch is validated before a snapshot or mutation is performed.
func (w *Workbook) SetCellFormulasChanged(sheet string, edits []document.CellFormulaEdit) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return false, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if len(edits) == 0 {
		return false, nil
	}
	if len(edits) > maxClipboardCells {
		return false, fmt.Errorf("formula batch exceeds %d cells", maxClipboardCells)
	}
	type formulaEdit struct {
		ref     string
		formula string
	}
	prepared := make([]formulaEdit, len(edits))
	seen := make(map[string]struct{}, len(edits))
	for index, edit := range edits {
		if edit.Row < 1 || edit.Column < 1 || edit.Row > maxXLSXRows || edit.Column > maxXLSXColumns {
			return false, fmt.Errorf("formula destination (%d,%d) exceeds XLSX worksheet limits", edit.Row, edit.Column)
		}
		if !utf8.ValidString(edit.Formula) {
			return false, fmt.Errorf("formula for (%d,%d) is not valid UTF-8", edit.Row, edit.Column)
		}
		if !withinFormulaUTF16Limit(edit.Formula) {
			return false, fmt.Errorf("formula for (%d,%d) exceeds 8192 UTF-16 units", edit.Row, edit.Column)
		}
		formula := strings.TrimSpace(edit.Formula)
		formula = strings.TrimPrefix(formula, "=")
		if strings.TrimSpace(formula) == "" {
			return false, fmt.Errorf("formula for (%d,%d) cannot be empty", edit.Row, edit.Column)
		}
		formula = normalizeFormulaFunctionNames(formula)
		ref, err := excelize.CoordinatesToCellName(edit.Column, edit.Row)
		if err != nil {
			return false, fmt.Errorf("invalid formula destination (%d,%d): %w", edit.Row, edit.Column, err)
		}
		if _, duplicate := seen[ref]; duplicate {
			return false, fmt.Errorf("duplicate formula destination %s", ref)
		}
		seen[ref] = struct{}{}
		prepared[index] = formulaEdit{ref: ref, formula: formula}
	}
	changed := make([]formulaEdit, 0, len(prepared))
	for _, edit := range prepared {
		current, err := w.file.GetCellFormula(sheet, edit.ref)
		if err != nil {
			return false, fmt.Errorf("read formula %s: %w", edit.ref, err)
		}
		if current != edit.formula {
			changed = append(changed, edit)
		}
	}
	if len(changed) == 0 {
		return false, nil
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, fmt.Errorf("capture formula batch history: %w", err)
	}
	for _, edit := range changed {
		if err := w.file.SetCellFormula(sheet, edit.ref, edit.formula); err != nil {
			if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
				return false, errors.Join(fmt.Errorf("write formula %s: %w", edit.ref, err), fmt.Errorf("restore workbook after formula batch: %w", rollbackErr))
			}
			return false, fmt.Errorf("write formula %s: %w", edit.ref, err)
		}
	}
	_ = w.file.UpdateLinkedValue()
	if err := w.recordHistoryLocked(before); err != nil {
		if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
			return false, errors.Join(fmt.Errorf("record formula batch history: %w", err), fmt.Errorf("restore workbook after formula batch: %w", rollbackErr))
		}
		return false, fmt.Errorf("record formula batch history: %w", err)
	}
	w.dirty = true
	return true, nil
}

func withinFormulaUTF16Limit(formula string) bool {
	units := 0
	for _, char := range formula {
		units++
		if char > 0xFFFF {
			units++
		}
		if units > 8192 {
			return false
		}
	}
	return true
}

// GetCellFormula returns the formula expression for a cell.
func (w *Workbook) GetCellFormula(sheet, cell string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return "", err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return "", fmt.Errorf("worksheet %q does not exist", sheet)
	}
	return w.file.GetCellFormula(sheet, cell)
}
