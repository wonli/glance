package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestDataValidation_ListAndNumber(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// 1. Add dropdown list validation
	err := wb.AddDataValidation(document.SpreadsheetDataValidationSpec{
		Sheet:    sheet,
		Range:    document.CellRange{StartColumn: 1, StartRow: 1, EndColumn: 1, EndRow: 5},
		Type:     document.DataValidationTypeList,
		DropList: []string{"Apple", "Banana", "Orange"},
	})
	if err != nil {
		t.Fatalf("AddDataValidation list failed: %v", err)
	}

	dvs, err := wb.file.GetDataValidations(sheet)
	if err != nil {
		t.Fatalf("GetDataValidations failed: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1 data validation, got %d", len(dvs))
	}
	if dvs[0].Sqref != "A1:A5" {
		t.Errorf("expected Sqref A1:A5, got %s", dvs[0].Sqref)
	}

	// 2. Add number validation on B1:B10
	minVal := 10.0
	maxVal := 100.0
	err = wb.AddDataValidation(document.SpreadsheetDataValidationSpec{
		Sheet:     sheet,
		Range:     document.CellRange{StartColumn: 2, StartRow: 1, EndColumn: 2, EndRow: 10},
		Type:      document.DataValidationTypeNumber,
		MinNumber: &minVal,
		MaxNumber: &maxVal,
	})
	if err != nil {
		t.Fatalf("AddDataValidation number failed: %v", err)
	}

	dvs, err = wb.file.GetDataValidations(sheet)
	if err != nil {
		t.Fatalf("GetDataValidations failed: %v", err)
	}
	if len(dvs) != 2 {
		t.Fatalf("expected 2 data validations, got %d", len(dvs))
	}

	// 3. Test Undo
	if _, err := wb.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	dvs, err = wb.file.GetDataValidations(sheet)
	if err != nil {
		t.Fatalf("GetDataValidations after undo failed: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1 data validation after undo, got %d", len(dvs))
	}

	// 4. Test Redo
	if _, err := wb.Redo(); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	dvs, err = wb.file.GetDataValidations(sheet)
	if err != nil {
		t.Fatalf("GetDataValidations after redo failed: %v", err)
	}
	if len(dvs) != 2 {
		t.Fatalf("expected 2 data validations after redo, got %d", len(dvs))
	}

	// 5. Delete data validation
	err = wb.DeleteDataValidation(sheet, document.CellRange{StartColumn: 1, StartRow: 1, EndColumn: 1, EndRow: 5})
	if err != nil {
		t.Fatalf("DeleteDataValidation failed: %v", err)
	}
	dvs, err = wb.file.GetDataValidations(sheet)
	if err != nil {
		t.Fatalf("GetDataValidations failed: %v", err)
	}
	if len(dvs) != 1 {
		t.Fatalf("expected 1 data validation after delete, got %d", len(dvs))
	}
}
