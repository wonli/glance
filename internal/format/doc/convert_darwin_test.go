//go:build darwin

package doc

import (
	"archive/zip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"glance/internal/fileutil"
)

func makeLegacyDOC(t *testing.T, output string) {
	t.Helper()
	source := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", "sample.docx")
	if result, err := exec.Command(textutilPath, "-convert", "doc", "-output", output, source).CombinedOutput(); err != nil {
		t.Fatalf("create DOC fixture with textutil: %v\n%s", err, result)
	}
}

func TestConvertDOCToPrivateDOCXAndCleanup(t *testing.T) {
	tmpRoot := t.TempDir()
	fileutil.SetDefaultTempDir(tmpRoot)
	defer fileutil.SetDefaultTempDir("")
	source := filepath.Join(t.TempDir(), "source.doc")
	makeLegacyDOC(t, source)

	converted, err := ConvertToTempDOCX(context.Background(), source)
	if err != nil {
		t.Fatalf("ConvertToTempDOCX: %v", err)
	}
	if filepath.Dir(converted.Path) == filepath.Dir(source) || filepath.Dir(converted.Path) == "" {
		t.Fatalf("converted package is not in a private staging directory: %q", converted.Path)
	}
	if _, err := os.Stat(converted.Path); err != nil {
		t.Fatalf("staged DOCX does not exist: %v", err)
	}
	archive, err := zip.OpenReader(converted.Path)
	if err != nil {
		t.Fatalf("open staged DOCX: %v", err)
	}
	_ = archive.Close()
	stagingDirectory := filepath.Dir(converted.Path)
	if err := converted.Cleanup(); err != nil {
		t.Fatalf("cleanup staged DOCX: %v", err)
	}
	if _, err := os.Stat(stagingDirectory); !os.IsNotExist(err) {
		t.Fatalf("staging directory still exists after cleanup: stat err=%v", err)
	}
	if err := converted.Cleanup(); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
}

func TestConvertMalformedDOCRemovesStagingDirectory(t *testing.T) {
	tmpRoot := t.TempDir()
	fileutil.SetDefaultTempDir(tmpRoot)
	defer fileutil.SetDefaultTempDir("")
	source := filepath.Join(tmpRoot, "broken.doc")
	if err := os.WriteFile(source, []byte("not a Word document"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConvertToTempDOCX(context.Background(), source); err == nil {
		t.Fatal("ConvertToTempDOCX accepted malformed DOC")
	}
	entries, err := os.ReadDir(tmpRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(source) {
		t.Fatalf("failed conversion left staging entries: %#v", entries)
	}
}

func TestConvertDOCToTempDOCXHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ConvertToTempDOCX(ctx, "unused.doc"); err != context.Canceled {
		t.Fatalf("ConvertToTempDOCX error = %v; want context canceled", err)
	}
}
