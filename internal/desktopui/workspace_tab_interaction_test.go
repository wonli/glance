package desktopui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/format/docx"
	"glance/internal/workspace"
)

func TestInactiveDocumentTabCanBeSelectedWithClientTitlebar(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.clientTitlebar = true
	app.desktopRuntime = newDesktopRuntime()
	app.windowID = 1
	app.sessions = map[string]*documentSessionState{
		"first":  newDocumentSessionView(newSharedDocumentSession("first"), app.windowID),
		"second": newDocumentSessionView(newSharedDocumentSession("second"), app.windowID),
	}
	app.documentSessionState = app.sessions["first"]
	app.activeTabID = "first"
	app.tabMeta = map[string]workspace.Tab{
		"first":  {ID: "first", DocumentID: "first", Kind: "docx", Title: "First", Position: 0},
		"second": {ID: "second", DocumentID: "second", Kind: "docx", Title: "Second", Position: 1},
	}

	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(220, 42), Max: image.Pt(220, 42)},
		}
		app.layoutWorkspaceTabListItem(gtx, []string{"second"}, 1)
		router.Frame(&ops)
	}
	draw()
	point := f32.Pt(100, 20)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: point, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	draw()
	if app.activeTabID != "second" {
		t.Fatalf("inactive document tab click selected %q; want second", app.activeTabID)
	}
}

func TestSnapshotlessDocumentTabReopensItsSourceWhenSelected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := word.SaveAs(path); err != nil {
		_ = word.Close()
		t.Fatal(err)
	}
	if err := word.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	app := New(nil, "")
	defer close(app.shutdown)
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	runtime.collection = collection
	app.desktopRuntime = runtime
	app.collection = collection
	app.clientTitlebar = true
	app.windowID = 1
	id := workspaceSessionID(path)
	meta := workspace.Tab{ID: id, DocumentID: id, Kind: "document", Title: "source.docx", SourcePath: path, Position: 0}
	if err := collection.AddTab(meta, "docx"); err != nil {
		t.Fatal(err)
	}
	app.tabMeta[id] = meta

	router := new(input.Router)
	var ops op.Ops
	draw := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(220, 42), Max: image.Pt(220, 42)},
		}
		app.layoutWorkspaceTabListItem(gtx, []string{id}, 1)
		router.Frame(&ops)
	}
	draw()
	point := f32.Pt(100, 20)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 9, Position: point, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 9, Position: point})
	draw()
	if !app.opening {
		t.Fatal("selecting a snapshotless tab with an existing source did not start an asynchronous reopen")
	}

	deadline := time.Now().Add(3 * time.Second)
	for app.opening && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(time.Millisecond)
	}
	app.processAsyncResults()
	if app.activeTabID != id || app.word == nil {
		t.Fatalf("reopened tab state = active %q / word loaded %v; want active %q with a loaded DOCX", app.activeTabID, app.word != nil, id)
	}
	app.closeWorkspaceTabNow(id)
}

func TestCreateMenuDismissOnBlankClick(t *testing.T) {
	app := New(nil, "")
	app.clientTitlebar = false
	app.showNewMenu = true

	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(1000, 700), Max: image.Pt(1000, 700)},
		}
		app.layoutWorkspaceBody(gtx)
		router.Frame(&ops)
	}

	draw()
	if !app.showNewMenu {
		t.Fatal("expected showNewMenu to be true before click")
	}

	// 1. Click inside the menu card (e.g. at (200, 20), on the header area of the card):
	cardPoint := f32.Pt(200, 20)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: cardPoint, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: cardPoint})
	draw()

	if !app.showNewMenu {
		t.Fatal("clicking inside create menu card unexpectedly dismissed the menu")
	}

	// 2. Click outside the menu in the blank area (e.g. at (700, 500) in workspace body):
	blankPoint := f32.Pt(700, 500)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 2, Position: blankPoint, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 2, Position: blankPoint})
	draw()

	if app.showNewMenu {
		t.Fatal("clicking outside create menu in blank area did not dismiss the menu")
	}
}

func TestCreateMenuDismissOnTabClick(t *testing.T) {
	app := New(nil, "")
	app.clientTitlebar = false
	app.activeTabID = "doc-a"
	app.tabMeta["doc-a"] = workspace.Tab{ID: "doc-a", Title: "doc-a.docx", Position: 0}
	app.tabMeta["doc-b"] = workspace.Tab{ID: "doc-b", Title: "doc-b.docx", Position: 1}
	app.showNewMenu = true

	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(800, 42), Max: image.Pt(800, 42)},
		}
		app.layoutWorkspaceTabs(gtx)
		router.Frame(&ops)
	}

	draw()
	// Click on Recent tab ("⌂ Glance") at (40, 20)
	point := f32.Pt(40, 20)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: point, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	draw()

	if app.showNewMenu {
		t.Fatal("clicking recent tab did not dismiss the create menu")
	}
}

func TestWorkspaceTabsFullStripClickableWithClientTitlebar(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.clientTitlebar = true
	app.desktopRuntime = newDesktopRuntime()
	app.windowID = 1
	app.sessions = map[string]*documentSessionState{
		"first":  newDocumentSessionView(newSharedDocumentSession("first"), app.windowID),
		"second": newDocumentSessionView(newSharedDocumentSession("second"), app.windowID),
	}
	app.documentSessionState = app.sessions["first"]
	app.activeTabID = "first"
	app.tabMeta = map[string]workspace.Tab{
		"first":  {ID: "first", DocumentID: "first", Kind: "docx", Title: "First", Position: 0},
		"second": {ID: "second", DocumentID: "second", Kind: "docx", Title: "Second", Position: 1},
	}

	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(900, 42), Max: image.Pt(900, 42)},
		}
		app.layoutWorkspaceTabs(gtx)
		router.Frame(&ops)
	}

	clickAt := func(pt f32.Point) {
		draw()
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: pt, Buttons: pointer.ButtonPrimary})
		draw()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pt})
		draw()
	}

	draw()

	base := float32(16)
	if isDarwin() {
		base = 100
	}
	// Click second doc tab (inactive)
	secondDocPt := f32.Pt(base+112+196+95, 20)
	clickAt(secondDocPt)
	if app.activeTabID != "second" {
		t.Fatalf("clicking inactive tab in layoutWorkspaceTabs selected %q; want second", app.activeTabID)
	}

	// Click Recent tab
	recentPt := f32.Pt(base+56, 20)
	clickAt(recentPt)
	if app.activeTabID != workspace.RecentTabID {
		t.Fatalf("clicking recent tab in layoutWorkspaceTabs selected %q; want %q", app.activeTabID, workspace.RecentTabID)
	}

	// Click plus tab
	plusPt := f32.Pt(base+112+196+196+16, 20)
	clickAt(plusPt)
	if !app.showNewMenu {
		t.Fatal("clicking plus button in layoutWorkspaceTabs did not toggle showNewMenu")
	}
}

func isDarwin() bool {
	return filepath.Separator == '/' && len(filepath.VolumeName("/")) == 0 && os.Getenv("GOOS") != "linux" && os.Getenv("GOOS") != "windows"
}
