package pdfdoc

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/document"
	"glance/internal/fileutil"
)

func snapshotTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	fileutil.SetDefaultTempDir(dir)
	t.Cleanup(func() { fileutil.SetDefaultTempDir("") })
	return dir
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary snapshot files remain: %v", entries)
	}
}

func TestOpenSnapshotStreamPreservesSnapshotBytes(t *testing.T) {
	dir := snapshotTempDir(t)
	want := fixture(t)
	doc, err := OpenSnapshotStream(func(w io.Writer) error {
		_, err := w.Write(want)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	got, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("streamed snapshot bytes changed")
	}
	if _, _, _, err := doc.SnapshotWithRevision(); err != nil {
		t.Fatalf("SnapshotWithRevision(): %v", err)
	}
	// The temporary file must be unlinked immediately after mapping;
	// the active document retains no linked temporary file.
	assertNoTempFiles(t, dir)
}

func TestSnapshotReaderWithRevisionRemainsStableAcrossEdits(t *testing.T) {
	doc, err := OpenSnapshot(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	want, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reader, size, revision, dirty, err := doc.SnapshotReaderWithRevision()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if size != int64(len(want)) || revision != 1 || dirty {
		t.Fatalf("captured snapshot metadata = size:%d revision:%d dirty:%v", size, revision, dirty)
	}
	page, err := doc.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Apply(document.PDFCommand{Kind: "note", Page: 0, Revision: page.Revision, Text: "new revision", X: 40, Y: 80}); err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("captured PDF reader changed after the document published a new revision")
	}
	current, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(current, want) {
		t.Fatal("test edit did not create a distinct PDF revision")
	}
}

func TestOpenSnapshotStreamCleansWriterFailure(t *testing.T) {
	dir := snapshotTempDir(t)
	wantErr := errors.New("stream interrupted")
	_, err := OpenSnapshotStream(func(w io.Writer) error {
		if _, writeErr := w.Write([]byte("partial PDF")); writeErr != nil {
			return writeErr
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("OpenSnapshotStream() error = %v, want %v", err, wantErr)
	}
	assertNoTempFiles(t, dir)
}

func TestOpenSnapshotStreamRejectsInvalidPDFAndCleansBacking(t *testing.T) {
	dir := snapshotTempDir(t)
	_, err := OpenSnapshotStream(func(w io.Writer) error {
		_, err := w.Write([]byte("not a PDF"))
		return err
	})
	if err == nil {
		t.Fatal("OpenSnapshotStream() accepted invalid PDF data")
	}
	assertNoTempFiles(t, dir)
}

func TestMappedDocumentCloseReleasesBacking(t *testing.T) {
	snapshotTempDir(t)
	doc, err := OpenSnapshotStream(func(w io.Writer) error {
		_, err := w.Write(fixture(t))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.backing == nil || doc.backing.release == nil {
		t.Fatal("mapped document has no backing release action")
	}
	backing := doc.backing
	release := backing.release
	releases := 0
	backing.release = func() error {
		releases++
		return release()
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	if releases != 1 || len(backing.data) != 0 {
		t.Fatalf("backing release count=%d, remaining bytes=%d", releases, len(backing.data))
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("repeated Close() released backing %d times", releases)
	}
}

func TestSnapshotReaderLeaseDefersMappedBackingRelease(t *testing.T) {
	snapshotTempDir(t)
	want := fixture(t)
	doc, err := OpenSnapshotStream(func(w io.Writer) error {
		_, err := w.Write(want)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	backing := doc.backing
	release := backing.release
	releases := 0
	backing.release = func() error {
		releases++
		return release()
	}
	reader, _, _, _, err := doc.SnapshotReaderWithRevision()
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	if releases != 0 || len(backing.data) == 0 {
		t.Fatalf("backing was released with an active reader: releases=%d bytes=%d", releases, len(backing.data))
	}
	got, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("leased snapshot read differs: bytes=%d err=%v", len(got), err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if releases != 1 || len(backing.data) != 0 {
		t.Fatalf("reader close did not release backing: releases=%d bytes=%d", releases, len(backing.data))
	}
}

func TestOpenCopiesSourceFileSnapshot(t *testing.T) {
	dir := snapshotTempDir(t)
	want := fixture(t)
	path := filepath.Join(dir, "source.pdf")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if err := os.WriteFile(path, []byte("replaced source"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("opened PDF changed after its source file was replaced")
	}
	// The temporary snapshot backing must have been unlinked immediately.
	// Only the source file itself should remain in dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "source.pdf" {
		t.Fatalf("unexpected files in temp dir: %v", entries)
	}
}
