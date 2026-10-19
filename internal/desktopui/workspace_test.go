package desktopui

import (
	"path/filepath"

	"testing"
	"time"

	"gioui.org/layout"

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
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
	a.handleCloseConfirmation("discard")
	if _, ok := a.sessions[idB]; ok {
		t.Fatal("closed tab state remains in session collection")
	}
	if _, ok := a.tabMeta[idB]; ok {
		t.Fatal("closed tab remains in rendered tab metadata")
	}
	for _, id := range a.workspaceTabIDs() {
		if id == idB {
			t.Fatal("closed tab remains in the next frame's tab list")
		}
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
	if a.workspaceTabsList.ScrollAnyAxis {
		t.Fatal("document tab list should keep horizontal scrolling separate from the vertical-wheel adapter")
	}
	if a.recentList.Axis != layout.Vertical {
		t.Fatalf("recent list axis=%v, want vertical", a.recentList.Axis)
	}
	if a.createActionsList.Axis != layout.Horizontal {
		t.Fatalf("create action list axis=%v, want horizontally scrollable", a.createActionsList.Axis)
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

func TestUntouchedNewDocumentsRequireDiscardConfirmation(t *testing.T) {
	for _, format := range []string{"xlsx", "docx", "json"} {
		t.Run(format, func(t *testing.T) {
			a := New(nil, "")
			var loaded loadedDocument
			switch format {
			case "xlsx":
				book := xlsx.NewBlank()
				loaded = loadedDocument{book: book, sheets: book.SheetNames()}
			case "docx":
				word, err := docx.NewBlank()
				if err != nil {
					t.Fatal(err)
				}
				loaded.word = word
			case "json":
				loaded.jsonDoc = jsondoc.NewBlank()
			}
			if err := a.installDocument(loaded); err != nil {
				t.Fatal(err)
			}
			id := a.activeTabID
			a.closeWorkspaceTab(id)
			if !a.modal.Visible || a.pendingCloseTabID != id {
				t.Fatalf("untouched new %s document did not ask before discard", format)
			}
			a.handleCloseConfirmation("discard")
			if _, ok := a.sessions[id]; ok {
				t.Fatalf("discarded new %s document remains open", format)
			}
			if a.activeTabID != workspace.RecentTabID {
				t.Fatalf("active tab after closing new %s document=%q, want recent", format, a.activeTabID)
			}
		})
	}
}

func TestClosingDirtyDocumentShowsSaveConfirmation(t *testing.T) {
	a := New(nil, "")
	a.applyLocaleSelection(i18n.SelectionEnglish)
	book := xlsx.NewBlank()
	if err := a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	book.SetDirty(true)
	id := a.activeTabID
	a.closeWorkspaceTab(id)
	if !a.modal.Visible || a.pendingCloseTabID != id {
		t.Fatalf("dirty close did not open confirmation: visible=%v pending=%q", a.modal.Visible, a.pendingCloseTabID)
	}
	if a.modal.DialogID != "workspace.close.confirm" || a.text(a.modal.TitleRef) != "Save new document?" {
		t.Fatalf("close dialog did not use a stable ID and English title: id=%q title=%q", a.modal.DialogID, a.text(a.modal.TitleRef))
	}
	a.applyLocaleSelection(i18n.SelectionChinese)
	if !a.modal.Visible || a.modal.DialogID != "workspace.close.confirm" || a.text(a.modal.TitleRef) != "保存新文档？" {
		t.Fatalf("language switch changed the pending close dialog: visible=%v id=%q title=%q", a.modal.Visible, a.modal.DialogID, a.text(a.modal.TitleRef))
	}
	if _, ok := a.sessions[id]; !ok {
		t.Fatal("dirty document was removed before confirmation")
	}
}

func TestDocumentTabWidgetsCanFinishFrameAfterClose(t *testing.T) {
	a := New(nil, "")
	book := xlsx.NewBlank()
	book.SetDirty(false)
	if err := a.installDocument(loadedDocument{path: filepath.Join(t.TempDir(), "book.xlsx"), book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	id := a.activeTabID
	tabButton, closeButton := a.documentTabWidgets(id)
	a.closeWorkspaceTab(id)
	if tabButton == nil || closeButton == nil {
		t.Fatal("current frame lost document tab widgets after close")
	}
	if a.tabButtons[id] != nil || a.tabCloseButtons[id] != nil {
		t.Fatal("closed document tab widgets remain in app state")
	}
}

func TestCancelingCloseConfirmationKeepsDraftAndWorkspaceRecord(t *testing.T) {
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
	book.SetDirty(true)
	id := a.activeTabID
	a.closeWorkspaceTab(id)
	a.handleCloseConfirmation("cancel")
	if _, ok := a.sessions[id]; !ok {
		t.Fatal("cancel removed the document session")
	}
	if _, ok := collection.Get(id); !ok {
		t.Fatal("cancel removed the workspace record")
	}
	if a.modal.Visible || a.pendingCloseTabID != "" {
		t.Fatalf("cancel left confirmation state open: visible=%v pending=%q", a.modal.Visible, a.pendingCloseTabID)
	}
}

func TestDiscardingCloseConfirmationDeletesDraftAndWorkspaceRecord(t *testing.T) {
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
	book.SetDirty(true)
	id := a.activeTabID
	a.closeWorkspaceTab(id)
	a.handleCloseConfirmation("discard")
	if _, ok := a.sessions[id]; ok {
		t.Fatal("discard left the document session open")
	}
	if _, ok := collection.Get(id); ok {
		t.Fatal("discard left the workspace record")
	}
	if a.activeTabID != workspace.RecentTabID {
		t.Fatalf("active tab after discard=%q, want recent", a.activeTabID)
	}
}

func TestSavingFromCloseConfirmationSavesThenCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.xlsx")
	book := xlsx.NewBlank()
	if err := book.SaveAsDocument(path); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText("Sheet1", "A1", "changed"); err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	if err := a.installDocument(loadedDocument{path: path, book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatal(err)
	}
	id := a.activeTabID
	a.closeWorkspaceTab(id)
	a.handleCloseConfirmation("save")
	select {
	case result := <-a.saveResults:
		a.saveResults <- result
	case <-time.After(5 * time.Second):
		t.Fatal("save operation did not finish")
	}
	a.processAsyncResults()
	if _, ok := a.sessions[id]; ok {
		t.Fatalf("document remains open after save and close: saving=%v dirty=%v", a.saving, book.Dirty())
	}
	if book.Dirty() {
		t.Fatal("document remained dirty after successful save")
	}
}

func TestDiscardRemovesDraftAfterQueuedWorkspaceSnapshots(t *testing.T) {
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
	id := a.activeTabID
	book.SetDirty(true)
	a.persistActiveSession()
	a.closeWorkspaceTab(id)
	a.handleCloseConfirmation("discard")
	a.closeWorkspace()

	reopened, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok, err := reopened.LoadDocument(id); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("queued snapshot recreated explicitly discarded draft")
	}
	restored, err := workspace.OpenCollection(reopened)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := restored.Get(id); ok {
		t.Fatal("explicitly discarded draft tab survived restart")
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
