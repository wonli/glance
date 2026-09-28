package jsondoc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRestoresEditedText(t *testing.T) {
	doc := NewBlank()
	if err := doc.SetText("{\"ok\":true}"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Text() != "{\"ok\":true}" || !restored.Dirty() {
		t.Fatalf("restored=%q dirty=%v", restored.Text(), restored.Dirty())
	}
}

func TestSaveAsFailurePreservesDirtyDraft(t *testing.T) {
	doc := NewBlank()
	_ = doc.SetText("{\"draft\":1}")
	if err := doc.SaveAs(filepath.Join(t.TempDir(), "missing", "draft.json")); err == nil {
		t.Fatal("expected save failure")
	}
	if !doc.Dirty() || doc.Text() != "{\"draft\":1}" {
		t.Fatal("failed save lost draft state")
	}
}

func TestSaveAsRoundTrip(t *testing.T) {
	doc := NewBlank()
	_ = doc.SetText("{\"value\":1}")
	path := filepath.Join(t.TempDir(), "draft.json")
	if err := doc.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() {
		t.Fatal("successful save left document dirty")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\"value\":1}" {
		t.Fatalf("saved %q, %v", data, err)
	}
}
