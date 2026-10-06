package docx

import (
	"math"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordPageOrientationToggleAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	layout := doc.WordLayoutData()
	if len(layout.Sections) == 0 {
		t.Fatal("expected at least 1 section in blank doc")
	}
	initialSetup := layout.Sections[0].PageSetup
	if initialSetup.Orientation != "portrait" {
		t.Fatalf("expected initial orientation portrait, got %q", initialSetup.Orientation)
	}
	if initialSetup.WidthPT >= initialSetup.HeightPT {
		t.Fatalf("expected portrait width < height, got %f >= %f", initialSetup.WidthPT, initialSetup.HeightPT)
	}

	editor, ok := any(doc).(document.WordPageSetupEditor)
	if !ok {
		t.Fatal("doc does not implement WordPageSetupEditor")
	}

	// Switch to landscape
	if err := editor.SetPageOrientation("landscape"); err != nil {
		t.Fatalf("SetPageOrientation landscape failed: %v", err)
	}
	layout = doc.WordLayoutData()
	landscapeSetup := layout.Sections[0].PageSetup
	if landscapeSetup.Orientation != "landscape" {
		t.Fatalf("expected landscape orientation, got %q", landscapeSetup.Orientation)
	}
	if landscapeSetup.WidthPT <= landscapeSetup.HeightPT {
		t.Fatalf("expected landscape width > height, got %f <= %f", landscapeSetup.WidthPT, landscapeSetup.HeightPT)
	}

	// Verify XML
	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:orient="landscape"`) {
		t.Fatalf("XML missing w:orient=\"landscape\": %s", xml)
	}

	// Test Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after orientation change")
	}
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout = doc.WordLayoutData()
	if layout.Sections[0].PageSetup.Orientation != "portrait" {
		t.Fatalf("expected portrait after undo, got %q", layout.Sections[0].PageSetup.Orientation)
	}

	// Test Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}
	layout = doc.WordLayoutData()
	if layout.Sections[0].PageSetup.Orientation != "landscape" {
		t.Fatalf("expected landscape after redo, got %q", layout.Sections[0].PageSetup.Orientation)
	}

	// Error case: invalid orientation
	if err := editor.SetPageOrientation("diagonal"); err == nil {
		t.Fatal("expected error for invalid orientation")
	}
}

func TestWordPageMarginsAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor := any(doc).(document.WordPageSetupEditor)
	// Set custom margins: top=36 (half inch), right=54 (0.75 in), bottom=36, left=54
	if err := editor.SetPageMargins(36, 54, 36, 54); err != nil {
		t.Fatalf("SetPageMargins failed: %v", err)
	}

	layout := doc.WordLayoutData()
	setup := layout.Sections[0].PageSetup
	if setup.MarginTopPT != 36 || setup.MarginRightPT != 54 || setup.MarginBottomPT != 36 || setup.MarginLeftPT != 54 {
		t.Fatalf("unexpected margins: top=%f right=%f bottom=%f left=%f", setup.MarginTopPT, setup.MarginRightPT, setup.MarginBottomPT, setup.MarginLeftPT)
	}

	// Verify XML twips (36 pt = 720 twips, 54 pt = 1080 twips)
	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:top="720"`) || !strings.Contains(xml, `w:right="1080"`) ||
		!strings.Contains(xml, `w:bottom="720"`) || !strings.Contains(xml, `w:left="1080"`) {
		t.Fatalf("XML margins incorrect: %s", xml)
	}

	// Undo
	history := any(doc).(document.WordHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout = doc.WordLayoutData()
	if layout.Sections[0].PageSetup.MarginTopPT != 72 {
		t.Fatalf("expected restored 72pt top margin after undo, got %f", layout.Sections[0].PageSetup.MarginTopPT)
	}

	// Error case: negative margin
	if err := editor.SetPageMargins(-10, 54, 36, 54); err == nil {
		t.Fatal("expected error for negative margin")
	}
}

func TestWordPageSizeAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor := any(doc).(document.WordPageSetupEditor)
	// Set Letter size: 612 x 792 pt
	if err := editor.SetPageSize(612, 792); err != nil {
		t.Fatalf("SetPageSize failed: %v", err)
	}

	layout := doc.WordLayoutData()
	setup := layout.Sections[0].PageSetup
	if math.Abs(setup.WidthPT-612) > 0.1 || math.Abs(setup.HeightPT-792) > 0.1 {
		t.Fatalf("unexpected page size: width=%f height=%f", setup.WidthPT, setup.HeightPT)
	}

	// Verify XML twips (612 * 20 = 12240, 792 * 20 = 15840)
	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:w="12240"`) || !strings.Contains(xml, `w:h="15840"`) {
		t.Fatalf("XML page size incorrect: %s", xml)
	}

	// Undo
	history := any(doc).(document.WordHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout = doc.WordLayoutData()
	if math.Abs(layout.Sections[0].PageSetup.WidthPT-595.3) > 1.0 {
		t.Fatalf("expected restored A4 width (~595.3), got %f", layout.Sections[0].PageSetup.WidthPT)
	}

	// Error case: zero or negative dimensions
	if err := editor.SetPageSize(0, 100); err == nil {
		t.Fatal("expected error for zero width")
	}
}

func TestWordPageColumnsAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordPageSetupEditor)
	if !ok {
		t.Fatal("doc does not implement WordPageSetupEditor")
	}

	// Set 2 columns
	if err := editor.SetPageColumns(2); err != nil {
		t.Fatalf("SetPageColumns 2 failed: %v", err)
	}
	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:cols`) || !strings.Contains(xml, `w:num="2"`) {
		t.Fatalf("XML missing 2 columns: %s", xml)
	}

	// Undo
	history := any(doc).(document.WordHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	xml = string(doc.parts[doc.documentIdx].data)
	if strings.Contains(xml, `w:num="2"`) {
		t.Fatalf("expected 2 columns undone: %s", xml)
	}

	// Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}
	xml = string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:num="2"`) {
		t.Fatalf("expected 2 columns restored after redo: %s", xml)
	}

	// Invalid column counts
	if err := editor.SetPageColumns(0); err == nil {
		t.Fatal("expected error for 0 columns")
	}
	if err := editor.SetPageColumns(5); err == nil {
		t.Fatal("expected error for 5 columns")
	}
}

