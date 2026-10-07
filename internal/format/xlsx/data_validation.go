package xlsx

import (
	"fmt"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetDataValidationEditor = (*Workbook)(nil)

// AddDataValidation applies data validation rules to the specified cell range.
func (w *Workbook) AddDataValidation(spec document.SpreadsheetDataValidationSpec) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	sheet := spec.Sheet
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}
	if spec.Range.StartColumn <= 0 || spec.Range.StartRow <= 0 {
		return fmt.Errorf("invalid range for data validation: %v", spec.Range)
	}

	startCell := CellName(spec.Range.StartColumn, spec.Range.StartRow)
	endCell := CellName(spec.Range.EndColumn, spec.Range.EndRow)
	if startCell == "" || endCell == "" {
		return fmt.Errorf("invalid range bounds: col %d..%d row %d..%d",
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
			return fmt.Errorf("drop list cannot be empty for list validation")
		}
		if err := dv.SetDropList(spec.DropList); err != nil {
			return fmt.Errorf("set drop list: %w", err)
		}
	case document.DataValidationTypeNumber:
		if spec.MinNumber != nil && spec.MaxNumber != nil {
			if err := dv.SetRange(*spec.MinNumber, *spec.MaxNumber, excelize.DataValidationTypeDecimal, excelize.DataValidationOperatorBetween); err != nil {
				return fmt.Errorf("set number range: %w", err)
			}
		} else if spec.MinNumber != nil {
			if err := dv.SetRange(*spec.MinNumber, *spec.MinNumber, excelize.DataValidationTypeDecimal, excelize.DataValidationOperatorGreaterThanOrEqual); err != nil {
				return fmt.Errorf("set min number: %w", err)
			}
		} else if spec.MaxNumber != nil {
			if err := dv.SetRange(*spec.MaxNumber, *spec.MaxNumber, excelize.DataValidationTypeDecimal, excelize.DataValidationOperatorLessThanOrEqual); err != nil {
				return fmt.Errorf("set max number: %w", err)
			}
		} else {
			return fmt.Errorf("at least one of MinNumber or MaxNumber must be specified")
		}
	default:
		return fmt.Errorf("unsupported data validation type %q", spec.Type)
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	// Delete previous validation on the range if any to prevent conflict
	_ = w.file.DeleteDataValidation(sheet, sqref)

	if err := w.file.AddDataValidation(sheet, dv); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("add data validation: %w", err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// DeleteDataValidation removes data validation rules from the specified bounds or sheet.
func (w *Workbook) DeleteDataValidation(sheet string, bounds document.CellRange) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	var errDel error
	if bounds.StartColumn > 0 && bounds.StartRow > 0 {
		startCell := CellName(bounds.StartColumn, bounds.StartRow)
		endCell := CellName(bounds.EndColumn, bounds.EndRow)
		var sqref string
		if startCell == endCell {
			sqref = startCell
		} else {
			sqref = fmt.Sprintf("%s:%s", startCell, endCell)
		}
		errDel = w.file.DeleteDataValidation(sheet, sqref)
	} else {
		errDel = w.file.DeleteDataValidation(sheet)
	}

	if errDel != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("delete data validation: %w", errDel)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
