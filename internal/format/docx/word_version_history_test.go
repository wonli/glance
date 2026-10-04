package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveWithSnapshotReturnsExactWrittenPackage(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	path := filepath.Join(t.TempDir(), "saved.docx")
	if err := doc.SaveAsDocument(path); err != nil {
		t.Fatal(err)
	}
	replaceTestWordText(t, doc, "bold", "written revision")

	snapshot, err := doc.SaveWithSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot, onDisk) {
		t.Fatal("SaveWithSnapshot returned bytes different from the package written to disk")
	}
	if doc.Dirty() {
		t.Fatal("successful SaveWithSnapshot left the document dirty")
	}
}

func TestSaveAsDocumentWithSnapshotAdoptsPathOnlyAfterSuccess(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	sourcePath := filepath.Join(t.TempDir(), "source.docx")
	if err := doc.SaveAsDocument(sourcePath); err != nil {
		t.Fatal(err)
	}
	replaceTestWordText(t, doc, "bold", "unsaved revision")

	failedPath := filepath.Join(t.TempDir(), "missing", "target.docx")
	if _, err := doc.SaveAsDocumentWithSnapshot(failedPath); err == nil {
		t.Fatal("SaveAsDocumentWithSnapshot succeeded with a missing parent directory")
	}
	if doc.path != sourcePath || !doc.Dirty() {
		t.Fatalf("failed Save As changed path=%q dirty=%v; want source path and dirty state preserved", doc.path, doc.Dirty())
	}

	targetPath := filepath.Join(t.TempDir(), "target.docx")
	snapshot, err := doc.SaveAsDocumentWithSnapshot(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot, onDisk) || doc.path != targetPath || doc.Dirty() {
		t.Fatalf("successful Save As: bytesMatch=%v path=%q dirty=%v", bytes.Equal(snapshot, onDisk), doc.path, doc.Dirty())
	}
}

func TestRestoreSnapshotPreservesUnknownParts(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	addUnknownWordPackageParts(doc)

	target := textEditingFixture(t)
	defer target.Close()
	replaceTestWordText(t, target, "bold", "restored content")
	addUnknownWordPackageParts(target)
	target.parts[packagePartIndex(target.parts, "custom/media/opaque.bin")].data = []byte{0, 255, 1, 43}
	targetSnapshot, err := target.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	if err := doc.RestoreSnapshot(targetSnapshot); err != nil {
		t.Fatal(err)
	}
	restored, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	parts := readZipParts(t, restored)
	for name, want := range readZipParts(t, targetSnapshot) {
		if name == "word/document.xml" {
			continue
		}
		if !bytes.Equal(parts[name], want) {
			t.Errorf("restored package part %q differs from selected snapshot", name)
		}
	}
	if applied, err := doc.Undo(); err != nil || !applied {
		t.Fatalf("Undo() = %v, %v; want original opaque parts", applied, err)
	}
	undone, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := readZipParts(t, undone)["custom/media/opaque.bin"]; !bytes.Equal(got, []byte{0, 255, 1, 42}) {
		t.Fatalf("undo did not restore original opaque part: %v", got)
	}
	if applied, err := doc.Redo(); err != nil || !applied {
		t.Fatalf("Redo() = %v, %v; want selected opaque parts", applied, err)
	}
	redone, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := readZipParts(t, redone)["custom/media/opaque.bin"]; !bytes.Equal(got, []byte{0, 255, 1, 43}) {
		t.Fatalf("redo did not restore selected opaque part: %v", got)
	}
}

func TestRestoreSnapshotIsSingleUndoRedoEntry(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	target := textEditingFixture(t)
	defer target.Close()
	replaceTestWordText(t, target, "bold", "selected version")
	targetSnapshot, err := target.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	if err := doc.RestoreSnapshot(targetSnapshot); err != nil {
		t.Fatal(err)
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() || !doc.CanUndo() || doc.CanRedo() {
		t.Fatalf("after restore: dirty=%v canUndo=%v canRedo=%v", doc.Dirty(), doc.CanUndo(), doc.CanRedo())
	}
	if applied, err := doc.Undo(); err != nil || !applied {
		t.Fatalf("Undo() = %v, %v; want one whole-package restore", applied, err)
	}
	undone, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(undone, before) || doc.Dirty() || doc.CanUndo() || !doc.CanRedo() {
		t.Fatalf("after one undo: packageRestored=%v dirty=%v canUndo=%v canRedo=%v", bytes.Equal(undone, before), doc.Dirty(), doc.CanUndo(), doc.CanRedo())
	}
	if applied, err := doc.Redo(); err != nil || !applied {
		t.Fatalf("Redo() = %v, %v; want one whole-package restore", applied, err)
	}
	redone, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(redone, after) || !doc.Dirty() {
		t.Fatalf("redo packageRestored=%v dirty=%v", bytes.Equal(redone, after), doc.Dirty())
	}
}

func TestRestoreSnapshotInvalidPackageLeavesDocumentUntouched(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	replaceTestWordText(t, doc, "bold", "current dirty content")
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	wasDirty, couldUndo, couldRedo := doc.Dirty(), doc.CanUndo(), doc.CanRedo()

	badXML := &bytes.Buffer{}
	writer := zip.NewWriter(badXML)
	partWriter, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(partWriter, `<w:document><w:body><w:p>`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for name, corrupt := range map[string][]byte{
		"invalid ZIP": []byte("not a ZIP package"),
		"invalid XML": badXML.Bytes(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := doc.RestoreSnapshot(corrupt); err == nil {
				t.Fatal("RestoreSnapshot accepted a corrupt package")
			}
		})
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) || doc.Dirty() != wasDirty || doc.CanUndo() != couldUndo || doc.CanRedo() != couldRedo {
		t.Fatalf("failed restore changed document: bytes=%v dirty=%v undo=%v redo=%v", bytes.Equal(after, before), doc.Dirty(), doc.CanUndo(), doc.CanRedo())
	}
}

func TestRestoreSnapshotIdenticalPackageStillCreatesDirtyUndoableEntry(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	path := filepath.Join(t.TempDir(), "saved.docx")
	if err := doc.SaveAsDocument(path); err != nil {
		t.Fatal(err)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	if err := doc.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() || !doc.CanUndo() || doc.CanRedo() {
		t.Fatalf("identical restore: dirty=%v canUndo=%v canRedo=%v", doc.Dirty(), doc.CanUndo(), doc.CanRedo())
	}
	if applied, err := doc.Undo(); err != nil || !applied || doc.Dirty() || doc.CanUndo() || !doc.CanRedo() {
		t.Fatalf("undo identical restore = %v, %v; dirty=%v canUndo=%v canRedo=%v", applied, err, doc.Dirty(), doc.CanUndo(), doc.CanRedo())
	}
	if applied, err := doc.Redo(); err != nil || !applied || !doc.Dirty() {
		t.Fatalf("redo identical restore = %v, %v; dirty=%v", applied, err, doc.Dirty())
	}
}

func replaceTestWordText(t *testing.T, doc *Document, current, replacement string) {
	t.Helper()
	runID, _ := findRunByText(t, doc.WordBlocks(), current)
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(runID, replacement); err != nil {
		t.Fatal(err)
	}
}

func addUnknownWordPackageParts(doc *Document) {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	doc.parts = append(doc.parts,
		part{header: zip.FileHeader{Name: "custom/unknown.xml", Method: zip.Deflate}, data: []byte("<custom><retained/></custom>")},
		part{header: zip.FileHeader{Name: "custom/_rels/unknown.xml.rels", Method: zip.Deflate}, data: []byte("<Relationships><Relationship Id=\"rId1\" Target=\"opaque.bin\"/></Relationships>")},
		part{header: zip.FileHeader{Name: "custom/media/opaque.bin", Method: zip.Store}, data: []byte{0, 255, 1, 42}},
	)
}
