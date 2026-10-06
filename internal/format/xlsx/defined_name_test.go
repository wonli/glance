package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookDefinedNamesAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	// Add defined name
	nameSpec := document.SpreadsheetDefinedName{
		Name:    "MySales",
		Ref:     "Sheet1!$A$1:$B$10",
		Comment: "Sales range",
	}
	if err := wb.SetDefinedName(nameSpec); err != nil {
		t.Fatalf("SetDefinedName: %v", err)
	}

	names, err := wb.GetDefinedNames()
	if err != nil {
		t.Fatalf("GetDefinedNames: %v", err)
	}
	found := false
	for _, n := range names {
		if n.Name == "MySales" && n.Ref == "Sheet1!$A$1:$B$10" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MySales defined name, found %v", names)
	}

	// Test Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}

	namesAfterUndo, err := wb.GetDefinedNames()
	if err != nil {
		t.Fatalf("GetDefinedNames after undo: %v", err)
	}
	for _, n := range namesAfterUndo {
		if n.Name == "MySales" {
			t.Fatalf("expected MySales to be reverted by Undo, but still present")
		}
	}

	// Test Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	namesAfterRedo, _ := wb.GetDefinedNames()
	foundRedo := false
	for _, n := range namesAfterRedo {
		if n.Name == "MySales" {
			foundRedo = true
			break
		}
	}
	if !foundRedo {
		t.Fatal("expected MySales to be restored after Redo")
	}

	// Delete defined name
	if err := wb.DeleteDefinedName("MySales", ""); err != nil {
		t.Fatalf("DeleteDefinedName: %v", err)
	}
	namesAfterDelete, _ := wb.GetDefinedNames()
	for _, n := range namesAfterDelete {
		if n.Name == "MySales" {
			t.Fatal("expected MySales to be deleted")
		}
	}
}
