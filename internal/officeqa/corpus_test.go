package officeqa

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type writeManifestFilePayload struct {
	Version int               `json:"version"`
	Cases   []json.RawMessage `json:"cases"`
}

func writeManifestFile(t *testing.T, root string, cases ...json.RawMessage) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(writeManifestFilePayload{Version: 1, Cases: cases})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

type manifestCasePayload struct {
	Id       string   `json:"id"`
	Path     string   `json:"path"`
	Format   string   `json:"format"`
	Features []string `json:"features"`
	Expected string   `json:"expected"`
}

func manifestCase(id, path, format string) json.RawMessage {
	data, _ := json.Marshal(manifestCasePayload{Id: id, Path: path, Format: format, Features: []string{"basic"}, Expected: "rendered"})
	return data
}

func manifestFixtureJSON(t *testing.T, fixture any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode manifest fixture: %v", err)
	}
	return data
}

func setManifestField(t *testing.T, entry json.RawMessage, name string, value any) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(entry, &fields); err != nil {
		t.Fatalf("decode manifest fixture: %v", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode manifest field %q: %v", name, err)
	}
	fields[name] = encoded
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode manifest fixture: %v", err)
	}
	return data
}

func touchCorpusFile(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadManifestRejectsDuplicateIDsAndPaths(t *testing.T) {
	t.Run("duplicate ID", func(t *testing.T) {
		root := t.TempDir()
		touchCorpusFile(t, root, "xlsx/one.xlsx")
		touchCorpusFile(t, root, "xlsx/two.xlsx")
		first := manifestCase("duplicate", "xlsx/one.xlsx", "xlsx")
		second := manifestCase("duplicate", "xlsx/two.xlsx", "xlsx")
		writeManifestFile(t, root, first, second)

		if _, err := LoadManifest(root); err == nil {
			t.Fatal("LoadManifest accepted duplicate case IDs")
		}
	})

	t.Run("duplicate path", func(t *testing.T) {
		root := t.TempDir()
		touchCorpusFile(t, root, "xlsx/one.xlsx")
		first := manifestCase("first", "xlsx/one.xlsx", "xlsx")
		second := manifestCase("second", "xlsx/one.xlsx", "xlsx")
		writeManifestFile(t, root, first, second)

		if _, err := LoadManifest(root); err == nil {
			t.Fatal("LoadManifest accepted duplicate case paths")
		}
	})
}

func TestLoadManifestRejectsUnsupportedFormatsAndPathTraversal(t *testing.T) {
	t.Run("unsupported extension", func(t *testing.T) {
		root := t.TempDir()
		touchCorpusFile(t, root, "xls/legacy.xls")
		writeManifestFile(t, root, manifestCase("legacy", "xls/legacy.xls", "xls"))

		if _, err := LoadManifest(root); err == nil {
			t.Fatal("LoadManifest accepted unsupported .xls format")
		}
	})

	t.Run("document path traversal", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(filepath.Dir(root), "outside.docx")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(outside) })
		writeManifestFile(t, root, manifestCase("escape", "../outside.docx", "docx"))

		if _, err := LoadManifest(root); err == nil {
			t.Fatal("LoadManifest accepted a path outside the corpus root")
		}
	})

	t.Run("baseline path traversal", func(t *testing.T) {
		root := t.TempDir()
		touchCorpusFile(t, root, "xlsx/one.xlsx")
		entry := manifestCase("baseline-escape", "xlsx/one.xlsx", "xlsx")
		entry = setManifestField(t, entry, "baselines", map[string]string{"target-001": "../outside.png"})
		writeManifestFile(t, root, entry)

		if _, err := LoadManifest(root); err == nil {
			t.Fatal("LoadManifest accepted a baseline path outside the corpus root")
		}
	})

	t.Run("non-PNG baseline", func(t *testing.T) {
		root := t.TempDir()
		touchCorpusFile(t, root, "xlsx/one.xlsx")
		entry := manifestCase("wrong-baseline", "xlsx/one.xlsx", "xlsx")
		entry = setManifestField(t, entry, "baselines", map[string]string{"target-001": "baselines/one.jpg"})
		writeManifestFile(t, root, entry)

		if _, err := LoadManifest(root); err == nil {
			t.Fatal("LoadManifest accepted a non-PNG baseline")
		}
	})
}

func TestLoadManifestRequiresExistingCaseFiles(t *testing.T) {
	root := t.TempDir()
	writeManifestFile(t, root, manifestCase("missing", "docx/missing.docx", "docx"))

	if _, err := LoadManifest(root); err == nil {
		t.Fatal("LoadManifest accepted a manifest entry with no document file")
	}
}

func TestLoadManifestRejectsOversizedManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(strings.Repeat(" ", maxManifestBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(root); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized manifest error = %v, want size-limit error", err)
	}
}

func TestLoadManifestRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.docx")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docx", "linked.docx")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeManifestFile(t, root, manifestCase("symlink-escape", "docx/linked.docx", "docx"))

	if _, err := LoadManifest(root); err == nil {
		t.Fatal("LoadManifest accepted a symlink pointing outside the corpus root")
	}
}

func TestLoadManifestRejectsInRootDocumentSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docx", "actual.docx")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("docx"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "docx", "linked.docx")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeManifestFile(t, root, manifestCase("in-root-link", "docx/linked.docx", "docx"))

	if _, err := LoadManifest(root); err == nil {
		t.Fatal("LoadManifest accepted an in-root document symlink that discovery would skip")
	}
}

func TestLoadManifestRejectsBaselineSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	touchCorpusFile(t, root, "xlsx/one.xlsx")
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "baseline.png"), []byte("not a real png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "baselines")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	entry := manifestCase("baseline-symlink", "xlsx/one.xlsx", "xlsx")
	entry = setManifestField(t, entry, "baselines", map[string]string{"worksheet-1": "baselines/baseline.png"})
	writeManifestFile(t, root, entry)

	if _, err := LoadManifest(root); err == nil {
		t.Fatal("LoadManifest accepted a baseline symlink pointing outside the corpus root")
	}
}

func TestLoadManifestLoadsCuratedCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "office-corpus")
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != manifestVersion {
		t.Fatalf("manifest version = %d, want %d", manifest.Version, manifestVersion)
	}
	if len(manifest.Cases) < 3 {
		t.Fatalf("curated corpus has %d cases, want at least one per supported format", len(manifest.Cases))
	}
	formats := map[string]bool{}
	for _, caseEntry := range manifest.Cases {
		formats[caseEntry.Format] = true
	}
	for _, format := range []string{"xlsx", "docx", "pptx"} {
		if !formats[format] {
			t.Errorf("curated corpus has no %s cases", format)
		}
	}
}
