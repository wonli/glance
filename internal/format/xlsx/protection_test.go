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
