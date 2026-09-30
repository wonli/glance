package docx

import (
	"path/filepath"
	"strconv"
	"testing"

	"glance/internal/document"
)

func TestWordHistoryUndoRedoTracksSavedAndUnsavedStates(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	editor := requireWordTextEditor(t, doc)
	history := requireWordHistory(t, doc)
	firstRun, _ := findRunByText(t, doc.WordBlocks(), "bold")
	if err := editor.ReplaceTextRun(firstRun, "saved text"); err != nil {
		t.Fatal(err)
	}
	savedPath := filepath.Join(t.TempDir(), "saved.docx")
	if err := doc.SaveAsDocument(savedPath); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() {
		t.Fatal("successful SaveAsDocument should establish the saved history state")
	}
	italicRun, _ := findRunByText(t, doc.WordBlocks(), " normal")
	no := false
	if err := editor.SetTextRunStyle(italicRun, document.WordTextFormatPatch{Italic: &no}); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() || !history.CanUndo() || history.CanRedo() {
		t.Fatalf("after style edit: dirty=%v canUndo=%v canRedo=%v", doc.Dirty(), history.CanUndo(), history.CanRedo())
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo style edit = %v, %v", applied, err)
	}
	if doc.Dirty() {
		t.Fatal("undo to the last saved state should clear dirty state")
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[1].Text.Style.Italic; !got {
		t.Fatal("undo did not restore the saved italic style")
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo style edit = %v, %v", applied, err)
	}
	if !doc.Dirty() {
		t.Fatal("redo away from saved state should mark document dirty")
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() {
		t.Fatal("successful save should mark the current history state clean")
	}
	firstRun, _ = findRunByText(t, doc.WordBlocks(), "saved text")
	if err := editor.ReplaceTextRun(firstRun, "after save"); err != nil {
		t.Fatal(err)
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo post-save text edit = %v, %v", applied, err)
	}
	if doc.Dirty() {
		t.Fatal("undo to the saved state after a later edit should clear dirty state")
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "saved text" {
		t.Fatalf("undo restored text %q; want saved text", got)
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo before save = %v, %v", applied, err)
	}
	if !doc.Dirty() {
		t.Fatal("undoing before the saved state should mark document dirty")
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "saved text" {
		t.Fatalf("undoing the style edit changed saved text to %q", got)
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo style edit to saved state = %v, %v", applied, err)
	}
	if doc.Dirty() {
		t.Fatal("redo to the saved state should clear dirty state")
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo saved style edit = %v, %v", applied, err)
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo pre-save text edit = %v, %v", applied, err)
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "bold" {
		t.Fatalf("undo pre-save text edit restored %q; want original text", got)
	}
	if !doc.Dirty() {
		t.Fatal("undo before the saved state should mark document dirty")
	}
}

func TestWordHistoryRetainsOnlyTheLatest100Actions(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	editor := requireWordTextEditor(t, doc)
	history := requireWordHistory(t, doc)
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	for value := 1; value <= 101; value++ {
		if err := editor.ReplaceTextRun(runID, strconv.Itoa(value)); err != nil {
			t.Fatalf("edit %d: %v", value, err)
		}
	}
	for action := 0; action < 100; action++ {
		applied, err := history.Undo()
		if err != nil || !applied {
			t.Fatalf("undo %d = %v, %v", action, applied, err)
		}
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "1" {
		t.Fatalf("value after 100 undos = %q; want first retained action value 1", got)
	}
	if applied, err := history.Undo(); err != nil || applied {
		t.Fatalf("101st undo = %v, %v; want false, nil", applied, err)
	}
	for action := 0; action < 100; action++ {
		applied, err := history.Redo()
		if err != nil || !applied {
			t.Fatalf("redo %d = %v, %v", action, applied, err)
		}
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "101" {
		t.Fatalf("value after 100 redos = %q; want 101", got)
	}
}

func TestNewWordEditAfterUndoClearsRedoBranch(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	editor := requireWordTextEditor(t, doc)
	history := requireWordHistory(t, doc)
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	if err := editor.ReplaceTextRun(runID, "first"); err != nil {
		t.Fatal(err)
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo = %v, %v", applied, err)
	}
	if err := editor.ReplaceTextRun(runID, "branch"); err != nil {
		t.Fatal(err)
	}
	if history.CanRedo() {
		t.Fatal("new edit after undo left a stale redo branch")
	}
	if applied, err := history.Redo(); err != nil || applied {
		t.Fatalf("redo after branching = %v, %v; want false, nil", applied, err)
	}
}
