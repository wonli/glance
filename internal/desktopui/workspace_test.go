package desktopui

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
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

func TestPlusMenuDoesNotExposeSaveActions(t *testing.T) {
	a := New(nil, "")
	for _, state := range []string{"empty", "draft", "saved"} {
		if state == "draft" {
			a.documentSessionState = newDocumentSessionState("")
			a.book = xlsx.NewBlank()
			a.book.(*xlsx.Workbook).SetDirty(true)
		} else if state == "saved" {
			a.path = "/tmp/book.xlsx"
		}
		got := a.createActionLabels()
		if containsLabel(got, "保存") || containsLabel(got, "保存…") || containsLabel(got, "另存为…") {
			t.Fatalf("save actions visible for %s document: %v", state, got)
		}
		if !containsLabel(got, "打开本地文件") || !containsLabel(got, "打开文件夹") {
			t.Fatalf("open actions missing for %s document: %v", state, got)
		}
	}
}

func TestPlusTabMatchesPenDocumentTabSpacing(t *testing.T) {
	a := New(nil, "")
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(500, 42)},
	}
	dims := a.layoutPlusTab(gtx)
	if dims.Size != image.Pt(38, int(workspaceTabHeight)) {
		t.Fatalf("plus tab size=%v; want 38x%v (32px button plus 6px left gap)", dims.Size, workspaceTabHeight)
	}
}

func TestPlusMenuAnchorsAtPenClpgmTabPosition(t *testing.T) {
	a := New(nil, "")
	a.clientTitlebar = false
	a.tabMeta["doc-a"] = workspace.Tab{ID: "doc-a", Position: 0}
	a.tabMeta["doc-b"] = workspace.Tab{ID: "doc-b", Position: 1}
	gtx := layout.Context{
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
	}
	if got := a.plusMenuOffset(gtx); got != 526 {
		t.Fatalf("plus menu x=%d, want Pen Clpgm x=526", got)
	}
}

func TestCreateMenuItemsHaveMonochromeIconsAndASectionDivider(t *testing.T) {
	a := New(nil, "")
	items := a.createActionItems()
	if len(items) != 7 || !items[2].separator {
		t.Fatalf("new-document menu items=%+v; want open actions, one divider, and four create actions", items)
	}
	for _, item := range items {
		if item.separator {
			continue
		}
		if item.icon == "" {
			t.Errorf("menu action %q has no icon", item.label)
			continue
		}
		if _, ok := toolbarIconData(item.icon); !ok {
			t.Errorf("menu action %q has no monochrome sprite icon %q", item.label, item.icon)
		}
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
	a.workspaceWriteResults = make(chan error, 1)
	a.workspaceWriteResults <- errors.New("disk full")
	a.processAsyncResults()
	if got := a.visibleStatus(); !strings.Contains(got, "disk full") {
		t.Fatalf("visible status=%q, want persistence failure", got)
	}
	a.status = "已打开另一个文档"
	if got := a.visibleStatus(); !strings.Contains(got, "disk full") {
		t.Fatalf("workspace failure was hidden by another status: %q", got)
	}
	a.workspaceWriteResults <- nil
	a.processAsyncResults()
	if got := a.visibleStatus(); got != a.status {
		t.Fatalf("successful write did not clear persistence error: got=%q status=%q", got, a.status)
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
