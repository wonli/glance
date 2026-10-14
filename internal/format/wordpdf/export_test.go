package wordpdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/format/docx"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
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

func TestExportSnapshotUsesSameParagraphGeometryAsDesktopPages(t *testing.T) {
	d, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.SetPageSize(200, 200); err != nil {
		t.Fatal(err)
	}
	if err = d.SetPageMargins(20, 20, 20, 20); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, _, err = d.AppendBodyTextParagraph("body"); err != nil {
			t.Fatal(err)
		}
	}
	s, err := wordfont.NewSystem()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := wordlayout.NewEngine(s).Layout(context.Background(), wordlayout.Input{DocumentID: "same-pages", Blocks: d.WordBlocks(), Layout: d.WordLayoutData(), DefaultPageSetup: wordlayout.DefaultPageSetup()})
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical.Pages) != 1 {
		t.Fatalf("fixture pages=%d; want a single canonical page", len(canonical.Pages))
	}
	snapshot, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExportSnapshot(context.Background(), filepath.Join(t.TempDir(), "same-pages.pdf"), snapshot, "same-pages", 1, "document", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.PageCount != len(canonical.Pages) {
		t.Fatalf("PDF pages=%d; desktop scenes=%d; editor padding changed document pagination", result.PageCount, len(canonical.Pages))
	}
}
