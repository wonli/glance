package xlsx

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (w *Workbook) SetRowHeight(sheet string, row int, heightPoints float64) error {
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
	if err := w.file.SetRowHeight(sheet, row, heightPoints); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

func (w *Workbook) SetColumnWidth(sheet string, column int, widthChars float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if column < 1 || column > maxXLSXColumns {
		return fmt.Errorf("column %d outside [1,%d]", column, maxXLSXColumns)
	}
	if !finiteNumber(widthChars) || widthChars <= 0 || widthChars > 255 {
		return fmt.Errorf("column width must be finite and within (0,255]: %v", widthChars)
	}
	columnName, err := excelize.ColumnNumberToName(column)
	if err != nil {
		return err
	}
	current, err := w.file.GetColWidth(sheet, columnName)
	if err != nil {
		return err
	}
	if current == widthChars {
		return nil
	}
	if err := w.file.SetColWidth(sheet, columnName, columnName, widthChars); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// SetCellText edits a cell as a literal string. Formula entry is deliberately
// deferred until calculation and round-trip behavior are defined.
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
	_ = w.file.UpdateLinkedValue()
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
