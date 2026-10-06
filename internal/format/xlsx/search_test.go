package xlsx

import (
	"testing"
)

func TestWorkbookSearchSheet(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	if err := wb.SetCellText("Sheet1", "A1", "Apple"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCellText("Sheet1", "B2", "pineapple"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCellText("Sheet1", "C3", "Banana"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCellText("Sheet1", "D4", "Cost $10.50"); err != nil {
		t.Fatal(err)
	}

	// Case-insensitive search
	matches, err := wb.SearchSheet("Sheet1", "apple")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches for 'apple', got %d: %+v", len(matches), matches)
	}
	if matches[0].Row != 1 || matches[0].Column != 1 || matches[0].Value != "Apple" {
		t.Errorf("match 0 mismatch: %+v", matches[0])
	}
	if matches[1].Row != 2 || matches[1].Column != 2 || matches[1].Value != "pineapple" {
		t.Errorf("match 1 mismatch: %+v", matches[1])
	}

	// Regex metacharacter search
	matches, err = wb.SearchSheet("Sheet1", "$10.50")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(matches) != 1 || matches[0].Row != 4 || matches[0].Column != 4 {
		t.Fatalf("expected 1 match for '$10.50', got %+v", matches)
	}

	// Empty query
	matches, err = wb.SearchSheet("Sheet1", "")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty query, got %d", len(matches))
	}

	// No match
	matches, err = wb.SearchSheet("Sheet1", "NotFound")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}

	// Invalid sheet
	if _, err := wb.SearchSheet("NoSuchSheet", "apple"); err == nil {
		t.Error("expected error for non-existent sheet")
	}
}
