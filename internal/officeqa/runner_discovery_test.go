package officeqa

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSingleFile(t *testing.T) {
	root := t.TempDir()
	path := writeInput(t, root, "one.XLSX", []byte("xlsx"))
	discovery, err := discoverInput(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Files) != 1 {
		t.Fatalf("discovered %d files, want 1", len(discovery.Files))
	}
	if discovery.Files[0].RelativePath != "one.XLSX" || discovery.Files[0].Format != "xlsx" {
		t.Fatalf("discovered file = %+v", discovery.Files[0])
	}
}

func TestDiscoverDirectoryOrderingAndExtensionCase(t *testing.T) {
	root := t.TempDir()
	writeInput(t, root, "z/last.DOCX", []byte("docx"))
	writeInput(t, root, "a/first.XLSX", []byte("xlsx"))
	writeInput(t, root, "middle.pptx", []byte("pptx"))
	writeInput(t, root, "ignored.txt", []byte("text"))

	discovery, err := discoverInput(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a/first.XLSX", "middle.pptx", "z/last.DOCX"}
	if len(discovery.Files) != len(want) {
		t.Fatalf("discovered %d files, want %d: %+v", len(discovery.Files), len(want), discovery.Files)
	}
	for index, path := range want {
		if discovery.Files[index].RelativePath != path {
			t.Errorf("file %d = %q, want %q", index, discovery.Files[index].RelativePath, path)
		}
	}
}

func TestDiscoverSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	writeInput(t, root, "inside.xlsx", []byte("xlsx"))
	outside := filepath.Join(t.TempDir(), "outside.docx")
	if err := os.WriteFile(outside, []byte("docx"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.docx")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	discovery, err := discoverInput(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Files) != 1 || discovery.Files[0].RelativePath != "inside.xlsx" {
		t.Fatalf("symlink was followed or supported file omitted: %+v", discovery.Files)
	}
}

func TestCorpusFingerprintStableAndChangesWithBytes(t *testing.T) {
	root := t.TempDir()
	path := writeInput(t, root, "nested/book.xlsx", []byte("original"))
	discovery, err := discoverInput(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := corpusFingerprint(discovery.Files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := corpusFingerprint(discovery.Files)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("fingerprint is empty or unstable: %q then %q", first, second)
	}
	if err := os.WriteFile(path, []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedDiscovery, err := discoverInput(root)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := corpusFingerprint(changedDiscovery.Files)
	if err != nil {
		t.Fatal(err)
	}
	if first == changed {
		t.Fatal("fingerprint did not change when document bytes changed")
	}
}

func TestCorpusFingerprintBoundsOversizedFilesToMetadata(t *testing.T) {
	root := t.TempDir()
	path := writeInput(t, root, "oversized.xlsx", []byte("first"))
	discovery, err := discoverInput(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := corpusFingerprintWithLimit(discovery.Files, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedDiscovery, err := discoverInput(root)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := corpusFingerprintWithLimit(changedDiscovery.Files, 4)
	if err != nil {
		t.Fatal(err)
	}
	if first != changed {
		t.Fatal("fingerprint read oversized file content instead of using bounded metadata")
	}
}
