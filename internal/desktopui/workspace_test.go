package desktopui

import (
	"path/filepath"
	"testing"

	"gioui.org/layout"

	"glance/internal/format/docx"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

func TestDocumentTabsKeepIndependentSessionState(t *testing.T) {
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
	book := xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	idA := a.activeTabID
	stateA := a.documentSessionState
	stateA.sheet = "Sheet1"
	stateA.selectedCell = "F8"
	stateA.virtualGrid.SelectedRow, stateA.virtualGrid.SelectedColumn = 8, 6
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatal(err)
	}
	idB := a.activeTabID
	if idA == idB || len(a.tabMeta) != 2 {
		t.Fatalf("tabs=%d ids=%q,%q", len(a.tabMeta), idA, idB)
	}
	a.activateSession(idA, stateA)
	if a.activeTabID != idA || a.selectedCell != "F8" || a.virtualGrid.SelectedRow != 8 || a.virtualGrid.SelectedColumn != 6 {
		t.Fatalf("restored tab state id=%q cell=%q viewport=%+v", a.activeTabID, a.selectedCell, a.virtualGrid)
	}
	stateA.book.(*xlsx.Workbook).SetDirty(false)
	a.sessions[idB].word.SetDirty(false)
	a.closeWorkspaceTab(idB)
	if _, ok := a.sessions[idB]; ok {
		t.Fatal("closed tab state remains in session collection")
	}
	if _, ok := a.sessions[idA]; !ok {
		t.Fatal("closing inactive tab removed active document")
	}
}

func TestWorkspaceTabsUseHorizontalGioList(t *testing.T) {
	a := New(nil, "")
	if a.workspaceTabsList.Axis != layout.Horizontal {
		t.Fatalf("document tab list axis=%v, want horizontal", a.workspaceTabsList.Axis)
	}
	if a.recentList.Axis != layout.Vertical {
		t.Fatalf("recent list axis=%v, want vertical", a.recentList.Axis)
	}
}

func TestSaveAsRequiresMatchingFormatExtension(t *testing.T) {
	for _, tc := range []struct {
		format, path string
		wantError    bool
	}{{"xlsx", "report.xlsx", false}, {"docx", "report.docx", false}, {"json", "report.json", false}, {"xlsx", "report.docx", true}, {"docx", "report.json", true}} {
		err := validateSaveAsExtension(tc.format, tc.path)
		if (err != nil) != tc.wantError {
			t.Errorf("validate(%q,%q) error=%v", tc.format, tc.path, err)
		}
	}
}

func TestRecentTabCannotBeClosed(t *testing.T) {
	a := New(nil, "")
	a.activeTabID = "recent"
	a.closeWorkspaceTab("recent")
	if a.activeTabID != "recent" {
		t.Fatalf("active tab became %q", a.activeTabID)
	}
}

func TestSaveActionsLiveInPlusMenuForActiveDocument(t *testing.T) {
	a := New(nil, "")
	if got := a.createActionLabels(); containsLabel(got, "保存") || containsLabel(got, "另存为…") {
		t.Fatalf("save actions visible without a document: %v", got)
	}
	a.documentSessionState = newDocumentSessionState("")
	a.book = xlsx.NewBlank()
	a.book.(*xlsx.Workbook).SetDirty(true)
	got := a.createActionLabels()
	if !containsLabel(got, "保存…") || !containsLabel(got, "另存为…") {
		t.Fatalf("draft save actions missing from plus menu: %v", got)
	}
	a.path = "/tmp/book.xlsx"
	got = a.createActionLabels()
	if !containsLabel(got, "保存") || !containsLabel(got, "另存为…") {
		t.Fatalf("document save actions missing from plus menu: %v", got)
	}
}

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
	a.saveAsWith(func(ext string) (string, error) {
		if ext != "xlsx" {
			t.Fatalf("save picker extension=%q, want xlsx", ext)
		}
		return "", nil
	})
	if a.saving || a.path != "" || !a.book.Dirty() {
		t.Fatalf("canceled Save As changed draft: saving=%v path=%q dirty=%v", a.saving, a.path, a.book.Dirty())
	}
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
