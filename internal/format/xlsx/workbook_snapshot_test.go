package xlsx

import (
	"bytes"
	"testing"
)

func TestSnapshotRestoresEditedWorkbook(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	if err := book.SetCellText("Sheet1", "A1", "draft value"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := book.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	value, err := restored.Value("Sheet1", "A1")
	if err != nil || value != "draft value" {
		t.Fatalf("restored A1 = %q, %v", value, err)
	}
}

func TestOpenSnapshotRejectsMalformedPayload(t *testing.T) {
	if _, err := OpenSnapshot(bytes.Repeat([]byte("x"), 32)); err == nil {
		t.Fatal("expected malformed snapshot error")
	}
}
