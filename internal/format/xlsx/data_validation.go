package xlsx

import (
	"fmt"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetDataValidationEditor = (*Workbook)(nil)

// AddDataValidation applies data validation rules to the specified cell range.
func (w *Workbook) AddDataValidation(spec document.SpreadsheetDataValidationSpec) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	sheet := spec.Sheet
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}
	if spec.Range.StartColumn <= 0 || spec.Range.StartRow <= 0 {
		return false, fmt.Errorf("invalid range for data validation: %v", spec.Range)
	}

	startCell := CellName(spec.Range.StartColumn, spec.Range.StartRow)
	endCell := CellName(spec.Range.EndColumn, spec.Range.EndRow)
	if startCell == "" || endCell == "" {
		return false, fmt.Errorf("invalid range bounds: col %d..%d row %d..%d",
			spec.Range.StartColumn, spec.Range.EndColumn, spec.Range.StartRow, spec.Range.EndRow)
	}
	var sqref string
	if startCell == endCell {
		sqref = startCell
	} else {
		sqref = fmt.Sprintf("%s:%s", startCell, endCell)
	}

	dv := excelize.NewDataValidation(true)
	dv.Sqref = sqref

	switch spec.Type {
	case document.DataValidationTypeList:
		if len(spec.DropList) == 0 {
			return false, fmt.Errorf("drop list cannot be empty for list validation")
		}
		if err := dv.SetDropList(spec.DropList); err != nil {
			return false, fmt.Errorf("set drop list: %w", err)
		}
	case document.DataValidationTypeNumber:
		if spec.MinNumber != nil && spec.MaxNumber != nil {
			if err := dv.SetRange(*spec.MinNumber, *spec.MaxNumber, excelize.DataValidationTypeDecimal, excelize.DataValidationOperatorBetween); err != nil {
				return false, fmt.Errorf("set number range: %w", err)
			}
		} else if spec.MinNumber != nil {
			if err := dv.SetRange(*spec.MinNumber, *spec.MinNumber, excelize.DataValidationTypeDecimal, excelize.DataValidationOperatorGreaterThanOrEqual); err != nil {
				return false, fmt.Errorf("set min number: %w", err)
			}
		} else if spec.MaxNumber != nil {
			if err := dv.SetRange(*spec.MaxNumber, *spec.MaxNumber, excelize.DataValidationTypeDecimal, excelize.DataValidationOperatorLessThanOrEqual); err != nil {
				return false, fmt.Errorf("set max number: %w", err)
			}
		} else {
			return false, fmt.Errorf("at least one of MinNumber or MaxNumber must be specified")
		}
	default:
		return false, fmt.Errorf("unsupported data validation type %q", spec.Type)
	}

	validations, err := w.file.GetDataValidations(sheet)
	if err != nil {
		return false, fmt.Errorf("get data validations: %w", err)
	}
	for _, current := range validations {
		if current.Sqref == sqref && sameDataValidation(current, dv) {
			return false, nil
		}
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}

	// Delete previous validation on the range if any to prevent conflict
	_ = w.file.DeleteDataValidation(sheet, sqref)

	if err := w.file.AddDataValidation(sheet, dv); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return false, fmt.Errorf("add data validation: %w", err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return false, err
	}
	w.dirty = true
	return true, nil
}

// DeleteDataValidation removes data validation rules from the specified bounds or sheet.
func (w *Workbook) DeleteDataValidation(sheet string, bounds document.CellRange) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}

	var sqref string
	if bounds.StartColumn > 0 && bounds.StartRow > 0 {
		startCell := CellName(bounds.StartColumn, bounds.StartRow)
		endCell := CellName(bounds.EndColumn, bounds.EndRow)
		if startCell == endCell {
			sqref = startCell
		} else {
			sqref = fmt.Sprintf("%s:%s", startCell, endCell)
		}
	}

	validations, err := w.file.GetDataValidations(sheet)
	if err != nil {
		return false, fmt.Errorf("get data validations: %w", err)
	}
	found := false
	for _, current := range validations {
		if sqref == "" || current.Sqref == sqref {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	if errDel := w.file.DeleteDataValidation(sheet, sqref); errDel != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return false, fmt.Errorf("delete data validation: %w", errDel)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return false, err
	}
	w.dirty = true
	return true, nil
}

func sameDataValidation(current, expected *excelize.DataValidation) bool {
	if current == nil || expected == nil {
		return current == expected
	}
	return current.Sqref == expected.Sqref &&
		current.Type == expected.Type &&
		current.Operator == expected.Operator &&
		current.Formula1 == expected.Formula1 &&
		current.Formula2 == expected.Formula2 &&
		current.AllowBlank == expected.AllowBlank &&
		current.ShowDropDown == expected.ShowDropDown &&
		current.ShowErrorMessage == expected.ShowErrorMessage &&
		current.ShowInputMessage == expected.ShowInputMessage &&
		equalOptionalString(current.Error, expected.Error) &&
		equalOptionalString(current.ErrorStyle, expected.ErrorStyle) &&
		equalOptionalString(current.ErrorTitle, expected.ErrorTitle) &&
		equalOptionalString(current.Prompt, expected.Prompt) &&
		equalOptionalString(current.PromptTitle, expected.PromptTitle)
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
