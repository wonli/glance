//go:build darwin

package desktopui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
	"glance/internal/workspace"
)

func legacyDOCFixture(t *testing.T) string {
	t.Helper()
	converter, err := exec.LookPath("textutil")
	if err != nil {
		t.Fatalf("macOS textutil is required for DOC import: %v", err)
	}
	input := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "sample.docx")
	output := filepath.Join(t.TempDir(), "legacy.doc")
	if result, err := exec.Command(converter, "-convert", "doc", "-output", output, input).CombinedOutput(); err != nil {
		t.Fatalf("create legacy DOC fixture: %v\n%s", err, result)
	}
	return output
}

func hasWordText(doc *docx.Document) bool {
	for _, block := range doc.Blocks() {
		if strings.TrimSpace(block.Text) != "" {
			return true
		}
		for _, row := range block.Rows {
			for _, cell := range row {
				if strings.TrimSpace(cell) != "" {
					return true
				}
			}
		}
	}
	return false
}

func TestLoadLegacyDOCThroughDOCXAdapter(t *testing.T) {
	path := legacyDOCFixture(t)
	sourceBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("load legacy DOC: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })
	if loaded.word == nil || loaded.book != nil {
		t.Fatalf("loaded adapters: word=%T workbook=%T; want DOCX word adapter", loaded.word, loaded.book)
	}
	if loaded.path != path || loaded.sourceFormat != application.FormatDOC || !loaded.saveAsRequired {
		t.Fatalf("source metadata: path=%q format=%q saveAsRequired=%v", loaded.path, loaded.sourceFormat, loaded.saveAsRequired)
	}
	if !hasWordText(loaded.word) {
		t.Fatal("converted DOCX adapter contains no document text")
	}
	if err := loaded.word.Save(); err == nil {
		t.Fatal("imported DOC unexpectedly has a writable source path")
	}
	sourceAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceBefore) != string(sourceAfter) {
		t.Fatal("DOC import modified the source file")
	}
}

func TestLegacyDOCImportKeepsTextOnEveryPaginatedPage(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 80; i++ {
		text := fmt.Sprintf("Paragraph %02d: %s", i, "This imported document contains enough text to flow across page boundaries.")
		if _, _, err := word.AppendBodyTextParagraph(text); err != nil {
			_ = word.Close()
			t.Fatalf("append synthetic paragraph: %v", err)
		}
	}
	packageBytes, err := word.Snapshot()
	_ = word.Close()
	if err != nil {
		t.Fatalf("snapshot synthetic DOCX: %v", err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "multipage.docx")
	legacyPath := filepath.Join(directory, "multipage.doc")
	if err := os.WriteFile(source, packageBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	converter, err := exec.LookPath("textutil")
	if err != nil {
		t.Fatalf("macOS textutil is required for DOC import: %v", err)
	}
	if output, err := exec.Command(converter, "-convert", "doc", "-output", legacyPath, source).CombinedOutput(); err != nil {
		t.Fatalf("create multipage legacy DOC fixture: %v\n%s", err, output)
	}
	loaded, err := loadDocument(legacyPath)
	if err != nil {
		t.Fatalf("load multipage legacy DOC: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })
	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	desktop := newDesktopRuntime()
	app := newAppWithRuntime(nil, desktop)
	app.wordLayoutEngine = wordlayout.NewEngine(shaper)
	t.Cleanup(func() {
		app.stopAndWaitManagedBackground()
		close(app.shutdown)
		desktop.shutdown()
	})
	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("install imported legacy DOC: %v", err)
	}
	var layoutResult wordLayoutResult
	select {
	case layoutResult = <-app.wordLayoutResults:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for imported legacy DOC pagination")
	}
	if layoutResult.err != nil {
		t.Fatalf("paginate imported legacy DOC: %v", layoutResult.err)
	}
	app.applyWordLayoutResult(layoutResult)
	if app.documentSessionState.wordLayoutResult == nil {
		t.Fatal("desktop session did not accept the imported legacy DOC page scenes")
	}
	pages := app.documentSessionState.wordLayoutResult.Pages
	if len(pages) < 2 {
		t.Fatalf("imported legacy DOC produced %d pages; want multiple pages", len(pages))
	}
	for index, page := range pages {
		if len(page.Text) == 0 {
			t.Errorf("page %d has no text fragments after DOC import", index+1)
		}
	}
}

func TestLegacyDOCSessionRequiresDOCXSaveAsAndReopens(t *testing.T) {
	ctx := context.Background()
	path := legacyDOCFixture(t)
	sourceBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)

	shared, openedFromDisk, err := desktop.openLiveDocument(path)
	if err != nil {
		t.Fatalf("open legacy DOC through application runtime: %v", err)
	}
	if !openedFromDisk || shared.path != path || shared.sourceFormat != application.FormatDOC || !shared.saveAsRequired || shared.word == nil {
		t.Fatalf("shared session: openedFromDisk=%v path=%q source=%q saveAsRequired=%v word=%v", openedFromDisk, shared.path, shared.sourceFormat, shared.saveAsRequired, shared.word != nil)
	}
	adapter, err := newDesktopSessionAdapter(desktop, shared)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Format() != application.FormatDocx || adapter.SourceFormat() != application.FormatDOC || adapter.ReadOnly() {
		t.Fatalf("adapter format=%q source=%q readOnly=%v", adapter.Format(), adapter.SourceFormat(), adapter.ReadOnly())
	}
	if err := adapter.Save(ctx); err == nil || !strings.Contains(err.Error(), "另存为 .docx") {
		t.Fatalf("Save error = %v; want DOCX Save As instruction", err)
	}
	wronlyDoc := filepath.Join(t.TempDir(), "wrong.doc")
	if _, err := desktop.applicationRuntime.SaveDocumentAs(ctx, shared.documentID, wronlyDoc); err == nil {
		t.Fatal("Save As accepted a .doc destination for the converted Word package")
	}
	if _, err := os.Stat(wronlyDoc); !os.IsNotExist(err) {
		t.Fatalf("rejected .doc Save As created a destination: stat err=%v", err)
	}
	if shared.path != path || shared.sourceFormat != application.FormatDOC || !shared.saveAsRequired {
		t.Fatalf("rejected Save As changed session path=%q source=%q saveAsRequired=%v", shared.path, shared.sourceFormat, shared.saveAsRequired)
	}
	output := filepath.Join(t.TempDir(), "edited.docx")
	info, err := desktop.applicationRuntime.SaveDocumentAs(ctx, shared.documentID, output)
	if err != nil {
		t.Fatalf("Save As DOCX: %v", err)
	}
	if info.Format != application.FormatDocx || shared.path != canonicalSessionPath(output) || shared.sourceFormat != application.FormatDocx || shared.saveAsRequired {
		t.Fatalf("post-Save-As format=%q path=%q source=%q saveAsRequired=%v", info.Format, shared.path, shared.sourceFormat, shared.saveAsRequired)
	}
	reopened, err := docx.Open(output)
	if err != nil {
		t.Fatalf("reopen DOCX output: %v", err)
	}
	defer reopened.Close()
	if !hasWordText(reopened) {
		t.Fatal("saved DOCX contains no document text")
	}
	sourceAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceBefore) != string(sourceAfter) {
		t.Fatal("DOC import or Save As modified the source file")
	}
}

func TestInstallLegacyDOCStoresDOCXRecoverySnapshotAndDOCRecentKind(t *testing.T) {
	path := legacyDOCFixture(t)
	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("load legacy DOC: %v", err)
	}
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		closeLoadedDocument(loaded)
		t.Fatalf("open workspace store: %v", err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		closeLoadedDocument(loaded)
		_ = store.Close()
		t.Fatalf("open workspace collection: %v", err)
	}
	desktop := newDesktopRuntime()
	app := newAppWithRuntime(nil, desktop)
	app.collection = collection
	t.Cleanup(func() {
		app.stopAndWaitManagedBackground()
		close(app.shutdown)
		desktop.shutdown()
		_ = store.Close()
	})
	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("install legacy DOC: %v", err)
	}
	record, ok := collection.Get(app.activeTabID)
	if !ok || record.Format != "docx" || record.Tab.SourcePath != path || len(record.Payload) == 0 {
		t.Fatalf("workspace record: found=%v format=%q path=%q payload=%d", ok, record.Format, record.Tab.SourcePath, len(record.Payload))
	}
	if recents := collection.Recents(); len(recents) != 1 || recents[0].Kind != "doc" || recents[0].Path != path {
		t.Fatalf("recent DOC entries = %#v; want one .doc recent", recents)
	}
	if !strings.Contains(app.status, "另存为 .docx") {
		t.Fatalf("legacy DOC status = %q; want Save As instruction", app.status)
	}
}

func TestLoadMalformedLegacyDOCFailsWithoutChangingSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.doc")
	want := []byte("not an OLE Word document")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDocument(path); err == nil {
		t.Fatal("loadDocument accepted malformed DOC")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("failed import modified the source file")
	}
}

func TestRecoveredLegacyDOCSessionKeepsDOCXSaveAsProtection(t *testing.T) {
	path := legacyDOCFixture(t)
	sourceBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("load legacy DOC for recovery fixture: %v", err)
	}
	snapshot, err := loaded.word.Snapshot()
	closeLoadedDocument(loaded)
	if err != nil {
		t.Fatalf("snapshot imported DOC: %v", err)
	}
	record := workspace.Session{
		Tab:    workspace.Tab{ID: "recovered-doc", DocumentID: "recovered-doc", Kind: "document", Title: filepath.Base(path), SourcePath: path},
		Format: "docx", Payload: snapshot, Dirty: true,
	}
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	t.Cleanup(func() { close(app.shutdown) })
	state, err := app.restoreSessionState(record)
	if err != nil {
		t.Fatalf("restore imported DOC workspace snapshot: %v", err)
	}
	if state.sourceFormat != application.FormatDOC || !state.saveAsRequired || state.word == nil {
		t.Fatalf("recovered state source=%q saveAsRequired=%v word=%v", state.sourceFormat, state.saveAsRequired, state.word != nil)
	}
	if got := sessionFormat(record.Tab, state); got != "docx" {
		t.Fatalf("recovered workspace format = %q; want docx", got)
	}
	app.documentSessionState, app.word, app.path = state, state.word, path
	state.wordAutoSave = true
	state.word.SetDirty(true)
	app.maybeAutoSaveWord(state)
	if app.saving {
		t.Fatal("recovered DOC session started AutoSave before Save As")
	}
	if err := state.word.Save(); err == nil {
		t.Fatal("recovered DOCX snapshot unexpectedly has the legacy DOC source path")
	}
	adapter, err := newDesktopSessionAdapter(desktop, state.sharedDocumentSession)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Save(context.Background()); err == nil || !strings.Contains(err.Error(), "另存为 .docx") {
		t.Fatalf("recovered-session Save error = %v; want DOCX Save As instruction", err)
	}
	if err := state.word.Close(); err != nil {
		t.Fatal(err)
	}
	sourceAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceBefore) != string(sourceAfter) {
		t.Fatal("recovered DOC session changed the legacy source")
	}
}
