package fileutil

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAtomicWriteAndReadExact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "test.txt")

	payload := []byte("Hello, Glance unified file read/write package!")
	if err := AtomicWriteBytes(target, payload, WithAutoCreateDir(true)); err != nil {
		t.Fatalf("AtomicWriteBytes failed: %v", err)
	}

	readBack, err := ReadExact(target)
	if err != nil {
		t.Fatalf("ReadExact failed: %v", err)
	}
	if !bytes.Equal(readBack, payload) {
		t.Fatalf("got %q, want %q", readBack, payload)
	}

	// Overwrite atomically
	updated := []byte("Updated content with atomic swap")
	if err := AtomicWriteString(target, string(updated)); err != nil {
		t.Fatalf("AtomicWriteString failed: %v", err)
	}

	readBack2, err := ReadExact(target)
	if err != nil {
		t.Fatalf("ReadExact after overwrite failed: %v", err)
	}
	if !bytes.Equal(readBack2, updated) {
		t.Fatalf("got %q, want %q", readBack2, updated)
	}
}

func TestAtomicWriteCommitGuard(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "guarded.txt")

	initial := []byte("original pristine content")
	if err := AtomicWriteBytes(target, initial); err != nil {
		t.Fatal(err)
	}

	// 1. Guard that rejects
	guardErr := errors.New("guard verification failed")
	err := AtomicWrite(target, func(w io.Writer) error {
		_, e := w.Write([]byte("bad corrupted data"))
		return e
	}, WithCommitGuard(func() (func(), error) {
		return nil, guardErr
	}))

	if err == nil || !strings.Contains(err.Error(), "guard verification failed") {
		t.Fatalf("expected guard rejection error, got %v", err)
	}

	// Verify original file is pristine and untouched
	got, err := ReadExact(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, initial) {
		t.Fatalf("target was modified despite guard failure: %q", got)
	}

	// 2. Guard that succeeds with release callback
	releaseCalled := false
	err = AtomicWrite(target, func(w io.Writer) error {
		_, e := w.Write([]byte("valid new data"))
		return e
	}, WithCommitGuard(func() (func(), error) {
		return func() {
			releaseCalled = true
		}, nil
	}))

	if err != nil {
		t.Fatalf("AtomicWrite with successful guard failed: %v", err)
	}
	if !releaseCalled {
		t.Fatal("expected guard release callback to be called")
	}

	got2, err := ReadExact(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != "valid new data" {
		t.Fatalf("got %q, want 'valid new data'", got2)
	}
}

func TestAtomicWriteReaderAndCopy(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "stream.bin")

	srcData := bytes.Repeat([]byte("0123456789abcdef"), 10000) // 160KB
	src := bytes.NewReader(srcData)

	n, err := AtomicWriteReader(target, src)
	if err != nil {
		t.Fatalf("AtomicWriteReader failed: %v", err)
	}
	if n != int64(len(srcData)) {
		t.Fatalf("written bytes %d != %d", n, len(srcData))
	}

	// Test OpenStream
	file, size, err := OpenStream(target)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer file.Close()
	if size != int64(len(srcData)) {
		t.Fatalf("OpenStream size %d != %d", size, len(srcData))
	}

	// Test CopyWithProgress
	var dest bytes.Buffer
	var progressRecords []int64
	copied, err := CopyWithProgress(&dest, file, func(w int64) {
		progressRecords = append(progressRecords, w)
	})
	if err != nil {
		t.Fatalf("CopyWithProgress failed: %v", err)
	}
	if copied != int64(len(srcData)) {
		t.Fatalf("copied %d != %d", copied, len(srcData))
	}
	if len(progressRecords) < 2 {
		t.Fatalf("expected progress callbacks across multiple 64KB chunks, got %d", len(progressRecords))
	}
	if !bytes.Equal(dest.Bytes(), srcData) {
		t.Fatal("stream copied content mismatch")
	}
}

func TestReadLimit(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "large.dat")

	data := bytes.Repeat([]byte("A"), 1024)
	if err := AtomicWriteBytes(target, data); err != nil {
		t.Fatal(err)
	}

	// Within limit
	read, err := ReadLimit(target, 2048)
	if err != nil {
		t.Fatalf("ReadLimit failed within limit: %v", err)
	}
	if len(read) != 1024 {
		t.Fatalf("got %d bytes, want 1024", len(read))
	}

	// Exceeding limit
	_, err = ReadLimit(target, 512)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestOpenStreamRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, _, err := OpenStream(dir)
	if !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("expected ErrNotRegularFile for directory, got %v", err)
	}
}

func TestMoveExclusive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// First move should succeed
	if err := MoveExclusive(src, dst); err != nil {
		t.Fatalf("MoveExclusive failed: %v", err)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("src should have been removed, got err: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "content" {
		t.Fatalf("dst content mismatch: %q, err: %v", data, err)
	}

	// Try moving another file to the existing destination - should fail with os.ErrExist
	src2 := filepath.Join(dir, "src2.txt")
	if err := os.WriteFile(src2, []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}
	err = MoveExclusive(src2, dst)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}
	// dst content should remain "content"
	data, _ = os.ReadFile(dst)
	if string(data) != "content" {
		t.Fatalf("dst should remain unchanged, got %q", data)
	}
}

func TestCentralizedTempDirDoesNotPolluteTargetDir(t *testing.T) {
	targetDir := t.TempDir()
	glanceTempDir := t.TempDir()

	SetDefaultTempDir(glanceTempDir)
	defer SetDefaultTempDir("")

	targetPath := filepath.Join(targetDir, "document.docx")

	// Save document using AtomicWrite
	err := AtomicWriteBytes(targetPath, []byte("document content"))
	if err != nil {
		t.Fatalf("AtomicWriteBytes failed: %v", err)
	}

	// Verify targetDir has ONLY document.docx, zero temporary files!
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "document.docx" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("targetDir should contain only document.docx, found: %v", names)
	}

	// Verify glanceTempDir has no leftover files
	tempEntries, err := os.ReadDir(glanceTempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tempEntries) != 0 {
		var names []string
		for _, e := range tempEntries {
			names = append(names, e.Name())
		}
		t.Fatalf("glanceTempDir should be clean after commit, found: %v", names)
	}
}

func TestCleanOrphanedTempFiles(t *testing.T) {
	tempRoot := t.TempDir()
	SetDefaultTempDir(tempRoot)
	defer SetDefaultTempDir("")

	oldFile := filepath.Join(tempRoot, "glance-pdf-snapshot-old.pdf")
	if err := os.WriteFile(oldFile, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	recentFile := filepath.Join(tempRoot, "glance-pdf-snapshot-recent.pdf")
	if err := os.WriteFile(recentFile, []byte("recent content"), 0o600); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(tempRoot, "glance-print-stage")
	if err := os.Mkdir(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(subDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// 1. Clean with olderThan = 1 hour: only oldFile and subDir should be removed
	cleaned, err := CleanOrphanedTempFiles(time.Hour)
	if err != nil {
		t.Fatalf("CleanOrphanedTempFiles(1h): %v", err)
	}
	if cleaned != 2 {
		t.Fatalf("cleaned = %d, want 2", cleaned)
	}
	if _, err := os.Stat(recentFile); err != nil {
		t.Fatalf("recent file was unexpectedly removed: %v", err)
	}
	if _, err := os.Stat(oldFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file was not removed: %v", err)
	}

	// 2. Clean with olderThan = 0: removes everything
	cleanedAll, err := CleanOrphanedTempFiles(0)
	if err != nil {
		t.Fatalf("CleanOrphanedTempFiles(0): %v", err)
	}
	if cleanedAll != 1 {
		t.Fatalf("cleanedAll = %d, want 1", cleanedAll)
	}
	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp directory still contains %d items after full clean", len(entries))
	}
}

