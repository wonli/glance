package desktopui

import (
	"errors"

	"os"
	"path/filepath"
	"testing"

	"glance/internal/document"

	"glance/internal/format/xlsx"
	"glance/internal/i18n"
	"glance/internal/workspace"
)

func TestWorkspaceWriterDrainsDraftSnapshotOnClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	a.workspaceStore = store
	a.collection = collection
	a.startWorkspaceWriter()
	book := xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	createdID := a.activeTabID
	a.closeWorkspace()

	reopened, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := workspace.OpenCollection(reopened)
	if err != nil {
		t.Fatal(err)
	}
	session, ok := restored.Get(createdID)
	if !ok || len(session.Payload) == 0 {
		t.Fatalf("closed workspace did not retain draft snapshot: found=%v payload=%d", ok, len(session.Payload))
	}
}

func TestSaveAsDialogCancellationKeepsDraftDirty(t *testing.T) {
	a := New(nil, "")
	a.documentSessionState = newDocumentSessionState("")
	a.book = xlsx.NewBlank()
	a.book.(*xlsx.Workbook).SetDirty(true)
	a.activeTabID = "draft-xlsx"
	a.sessions[a.activeTabID] = a.documentSessionState
	a.tabMeta[a.activeTabID] = workspace.Tab{ID: a.activeTabID, DocumentID: a.activeTabID, Kind: "draft", Title: "新建文档"}
	a.fileCommandRunning = true
	a.fileCommandResults <- fileCommandResult{kind: commandSaveAs}
	a.processAsyncResults()
	if a.saving || a.path != "" || !a.book.Dirty() {
		t.Fatalf("canceled Save As changed draft: saving=%v path=%q dirty=%v", a.saving, a.path, a.book.Dirty())
	}
}

func TestDraftIDsRemainUniqueAcrossProcessRestarts(t *testing.T) {
	draftCounter.Store(0)
	first := newDraftSessionID("xlsx")
	draftCounter.Store(0) // Simulate a fresh process after restoring the first draft.
	second := newDraftSessionID("xlsx")
	if first == second {
		t.Fatalf("draft ID reused after restart: %q", first)
	}
}

func TestInstallingSameCanonicalPathReusesSessionAndCaseIsPreserved(t *testing.T) {
	a := New(nil, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "book.xlsx")
	first := xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{path: path, book: first, sheets: first.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	id := a.activeTabID
	alias := dir + string(filepath.Separator) + "folder" + string(filepath.Separator) + ".." + string(filepath.Separator) + "book.xlsx"
	second := xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{path: alias, book: second, sheets: second.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	if len(a.tabMeta) != 1 || a.activeTabID != id || a.sessions[id].book != first {
		t.Fatalf("canonical path opened as duplicate session: tabs=%d active=%q", len(a.tabMeta), a.activeTabID)
	}
	upper, lower := xlsx.NewBlank(), xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{path: filepath.Join(dir, "A.xlsx"), book: upper, sheets: upper.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	upperID := a.activeTabID
	if err := a.installDocument(loadedDocument{path: filepath.Join(dir, "a.xlsx"), book: lower, sheets: lower.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	if a.activeTabID == upperID || len(a.tabMeta) != 3 {
		t.Fatalf("case-distinct paths collided: tabs=%d id=%q", len(a.tabMeta), a.activeTabID)
	}
}

func TestPollGridLoadClearsInactiveViewportLoading(t *testing.T) {
	a := New(nil, "")
	inactive, active := newGridViewport(), newGridViewport()
	inactive.Loading = true
	inactive.RequestedSheet = "Data"
	inactive.RequestedRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}
	a.gridLoadResults <- gridLoadResult{viewport: inactive, sheet: "Data", rangeOf: inactive.RequestedRange}
	a.pollGridLoad(active)
	if inactive.Loading {
		t.Fatal("inactive viewport remained permanently loading after its result was drained")
	}
}

func TestEmptyJSONSnapshotIsRestorable(t *testing.T) {
	record := workspace.Session{Format: "json", Payload: []byte{}}
	if !hasRestorableSnapshot(record) {
		t.Fatal("empty JSON text snapshot was treated as missing")
	}
	state, err := New(nil, "").restoreSessionState(record)
	if err != nil {
		t.Fatal(err)
	}
	if state.jsonDoc == nil || state.jsonDoc.Text() != "" {
		t.Fatalf("restored JSON draft text=%q, want empty document", state.jsonDoc.Text())
	}
}

func TestClosingAndAddingTabsKeepsUniqueStablePositions(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	a.collection = collection
	ids := make([]string, 0, 4)
	for i := 0; i < 3; i++ {
		book := xlsx.NewBlank()
		if err := a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
			t.Fatal(err)
		}
		book.SetDirty(false)
		ids = append(ids, a.activeTabID)
	}
	a.closeWorkspaceTab(ids[0])
	book := xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	positions := make(map[int]string)
	for id, tab := range a.tabMeta {
		if previous, ok := positions[tab.Position]; ok {
			t.Fatalf("tabs %q and %q share position %d", previous, id, tab.Position)
		}
		positions[tab.Position] = id
	}
}

func TestScanOfficeFolderIsShallowAndFiltersUnsupportedFiles(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.xlsx", "b.DOCX", "c.json", "ignored.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(nested, "hidden.xlsx"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	recents, err := scanOfficeFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recents) != 3 {
		t.Fatalf("folder scan returned %+v", recents)
	}
}

func TestWorkspaceWriteFailureStaysVisibleUntilNextSuccessfulWrite(t *testing.T) {
	a := New(nil, "")
	a.applyLocaleSelection(i18n.SelectionChinese)
	a.workspaceWriteResults = make(chan error, 1)
	a.workspaceWriteResults <- errors.New("disk full")
	a.processAsyncResults()
	if got := a.visibleStatus(); got != "工作区保存失败。" || a.feedback.detail != "disk full" {
		t.Fatalf("visible status=%q, want persistence failure", got)
	}
	a.setStatusMessage(i18n.Message{ID: "workspace.status.opened_another_document"}, feedbackInfo, notificationSuppressed)
	if got := a.visibleStatus(); got != "工作区保存失败。" {
		t.Fatalf("workspace failure was hidden by another status: %q", got)
	}
	a.workspaceWriteResults <- nil
	a.processAsyncResults()
	if got := a.visibleStatus(); got != a.statusText() {
		t.Fatalf("successful write did not clear persistence error: got=%q status=%q", got, a.statusText())
	}
}
