package xlsx

import (
	"testing"
)

func TestWorkbookProtectionAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// 1. Initial unprotected
	protected, err := wb.GetSheetProtection(sheet)
	if err != nil {
		t.Fatalf("GetSheetProtection failed: %v", err)
	}
	if protected {
		t.Fatal("expected sheet initially unprotected")
	}

	// 2. Protect sheet without password
	if err := wb.ProtectSheet(sheet, ""); err != nil {
		t.Fatalf("ProtectSheet failed: %v", err)
	}
	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || !protected {
		t.Fatalf("expected sheet protected, got protected=%v (err=%v)", protected, err)
	}

	// 3. Unprotect sheet
	if err := wb.UnprotectSheet(sheet, ""); err != nil {
		t.Fatalf("UnprotectSheet failed: %v", err)
	}
	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || protected {
		t.Fatalf("expected sheet unprotected, got protected=%v (err=%v)", protected, err)
	}

	// 4. Test Undo unprotect -> restores protected
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || !protected {
		t.Fatalf("expected sheet protected after undo, got protected=%v (err=%v)", protected, err)
	}

	// 5. Test Undo protect -> restores unprotected
	applied, err = wb.Undo()
	if err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || protected {
		t.Fatalf("expected sheet unprotected after second undo, got protected=%v (err=%v)", protected, err)
	}
}

func TestWorkbookStructureProtectionAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	// 1. Initial unprotected
	protected, err := wb.GetWorkbookProtection()
	if err != nil {
		t.Fatalf("GetWorkbookProtection failed: %v", err)
	}
	if protected {
		t.Fatal("expected workbook initially unprotected")
	}

	// 2. Protect workbook with password
	if err := wb.ProtectWorkbook("myPassword123"); err != nil {
		t.Fatalf("ProtectWorkbook failed: %v", err)
	}
	protected, err = wb.GetWorkbookProtection()
	if err != nil || !protected {
		t.Fatalf("expected workbook protected, got protected=%v (err=%v)", protected, err)
	}

	// 3. Unprotect with wrong password should fail
	if err := wb.UnprotectWorkbook("wrongPassword"); err == nil {
		t.Fatal("expected error unprotecting with wrong password, got nil")
	}

	// 4. Unprotect with correct password
	if err := wb.UnprotectWorkbook("myPassword123"); err != nil {
		t.Fatalf("UnprotectWorkbook failed: %v", err)
	}
	protected, err = wb.GetWorkbookProtection()
	if err != nil || protected {
		t.Fatalf("expected workbook unprotected, got protected=%v (err=%v)", protected, err)
	}

	// 5. Test Undo unprotect -> restores protected
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetWorkbookProtection()
	if err != nil || !protected {
		t.Fatalf("expected workbook protected after undo, got protected=%v (err=%v)", protected, err)
	}

	// 6. Test Undo protect -> restores unprotected
	applied, err = wb.Undo()
	if err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetWorkbookProtection()
	if err != nil || protected {
		t.Fatalf("expected workbook unprotected after second undo, got protected=%v (err=%v)", protected, err)
	}
}

