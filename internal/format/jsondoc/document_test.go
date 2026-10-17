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

func TestCompactSnapshotForLargeDiskJSON(t *testing.T) {
	tmpDir := t.TempDir()
	largePath := filepath.Join(tmpDir, "large.json")
	// Create ~2.5MB valid JSON
	data := make([]byte, 2500000)
	for i := range data {
		data[i] = ' '
	}
	data[0] = '['
	copy(data[1:], []byte(`{"id":1}`))
	data[len(data)-1] = ']'
	if err := os.WriteFile(largePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	doc, err := Open(largePath)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) > 500 {
		t.Fatalf("expected compact snapshot (<500 bytes), got %d bytes", len(snap))
	}

	restored, err := OpenSnapshot(snap)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Text()) != len(data) {
		t.Fatalf("restored length=%d, want %d", len(restored.Text()), len(data))
	}
}
