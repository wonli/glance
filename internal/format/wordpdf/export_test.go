package wordpdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/format/docx"
)

func TestExportSnapshotCommitGuardRejectsStaleOutput(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create DOCX: %v", err)
	}
	defer word.Close()
	if _, _, err := word.AppendBodyTextParagraph("stale output guard"); err != nil {
		t.Fatalf("append paragraph: %v", err)
	}
	snapshot, err := word.Snapshot()
	if err != nil {
		t.Fatalf("snapshot DOCX: %v", err)
	}
	target := filepath.Join(t.TempDir(), "stale.pdf")
	wantErr := errors.New("revision changed before commit")
	_, err = ExportSnapshot(context.Background(), target, snapshot, "doc-stale", 4, "document", func() (func(), error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("stale export error = %v; want %v", err, wantErr)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale export created target: stat error=%v", err)
	}
}
