package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
	"image"
	"io"
	"strings"
	"testing"
	"time"
)

func TestProofreadingTabsShareWindowAndKeepIndependentState(t *testing.T) {
	host := newAppWithRuntime(nil, newDesktopRuntime())
	host.proofreadingModule = true
	defer func() { host.closeAllProofreadingTabs(); host.stopAndWaitManagedBackground() }()
	open := func(id string) *proofreadingTab {
		t.Helper()
		if err := host.openProofreadingTab(proofreadingWindowRequest{resume: &workspace.ProofreadingProject{ID: id}, sourceName: id + ".pdf", sourceSnapshot: io.NopCloser(strings.NewReader("invalid test PDF"))}); err != nil {
			t.Fatal(err)
		}
		return host.proofreadingTab(host.selectedProofreadingTabID)
	}
	first := open("first")
	second := open("second")
	if len(host.desktopRuntime.windows) != 0 {
		t.Fatal("opening a manuscript registered another native window")
	}
	if first.editor.window != host.window || second.editor.window != host.window {
		t.Fatal("manuscripts did not share the host window")
	}
	first.editor.pdfView = &pdfViewState{}
	second.editor.pdfView = &pdfViewState{}
	first.editor.proofreading.page = 6
	second.editor.proofreading.page = 500
	first.editor.ensurePDFView().canvas.Position.Offset = 36
	second.editor.ensurePDFView().canvas.Position.Offset = 72
	host.activateRecent()
	if host.workspaceSelectedTabID() != workspace.RecentTabID || len(host.proofreadingTabs) != 2 {
		t.Fatal("workbench selection removed manuscript tabs")
	}
	host.activateWorkspaceTab(first.id)
	if host.workspaceSelectedTabID() != first.id || first.editor.proofreading.page != 6 || first.editor.ensurePDFView().canvas.Position.Offset != 36 {
		t.Fatal("first manuscript lost its selection or scroll")
	}
	host.activateWorkspaceTab(second.id)
	if host.workspaceSelectedTabID() != second.id || second.editor.proofreading.page != 500 || second.editor.ensurePDFView().canvas.Position.Offset != 72 {
		t.Fatal("second manuscript lost its independent state")
	}
	// Continuing an open project focuses its tab without making another model.
	open("first")
	host.processProofreadingTabs()
	if len(host.proofreadingTabs) != 2 || host.selectedProofreadingTabID != first.id {
		t.Fatal("continuing an open task duplicated its editor")
	}
	host.closeWorkspaceTab(first.id)
	if len(host.proofreadingTabs) != 1 || host.workspaceSelectedTabID() != workspace.RecentTabID {
		t.Fatal("closing a manuscript did not leave workbench and other manuscript intact")
	}
	if second.editor.proofreading.page != 500 {
		t.Fatal("closing one manuscript changed another")
	}
}

func TestOpeningPDFSourceInWorkbenchDoesNotOpenRegularPDFDocumentTab(t *testing.T) {
	host := newAppWithRuntime(nil, newDesktopRuntime())
	host.proofreadingModule = true
	defer func() { host.closeAllProofreadingTabs(); host.stopAndWaitManagedBackground() }()

	if err := host.openProofreadingTab(proofreadingWindowRequest{
		sourcePath:     "/books/book.pdf",
		sourceName:     "book.pdf",
		sourceSnapshot: io.NopCloser(strings.NewReader("invalid test PDF")),
	}); err != nil {
		t.Fatal(err)
	}
	if len(host.proofreadingTabs) != 1 || host.proofreadingTabs[0].id != host.selectedProofreadingTabID {
		t.Fatalf("proofreading tabs=%d selected=%q; want one selected proofreading task", len(host.proofreadingTabs), host.selectedProofreadingTabID)
	}
	if host.pdfDoc != nil || len(host.sessions) != 0 || len(host.tabMeta) != 0 {
		t.Fatalf("opening the PDF source created a regular PDF document session: pdf=%v sessions=%d metadata=%d", host.pdfDoc != nil, len(host.sessions), len(host.tabMeta))
	}
	ids := host.workspaceTabIDs()
	if len(ids) != 1 || ids[0] != host.selectedProofreadingTabID {
		t.Fatalf("workbench tabs=%v; want only the proofreading task, without a duplicate PDF document tab", ids)
	}
}

func TestProofreadingCommonTabStripSwitchesBothDirections(t *testing.T) {
	host := newAppWithRuntime(nil, newDesktopRuntime())
	host.proofreadingModule = true
	editor := newAppWithRuntime(nil, host.desktopRuntime)
	editor.proofreading = newProofreadingWorkspace(proofreadingWindowRequest{sourceName: "书稿.pdf"})
	editor.proofreadingTabHost = host
	tab := &proofreadingTab{id: "proofreading:test", editor: editor}
	host.proofreadingTabs = []*proofreadingTab{tab}
	host.selectedProofreadingTabID = tab.id
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1280, 42))}
		host.layoutWorkspaceTabs(g)
		router.Frame(&ops)
	}
	click := func(x float32) {
		frame()
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, 20)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, 20)})
		frame()
		frame()
	}
	click(60)
	if host.workspaceSelectedTabID() != workspace.RecentTabID || len(host.workspaceTabIDs()) != 1 {
		t.Fatal("workbench tab hid the open manuscript")
	}
	click(220)
	if host.workspaceSelectedTabID() != tab.id {
		t.Fatal("cannot switch back to manuscript using common tab strip")
	}
	click(60)
	click(220)
	if host.workspaceSelectedTabID() != tab.id {
		t.Fatal("repeated tab switch failed")
	}
}

func TestProofreadingTabSwitchFlushesLatestChineseDraft(t *testing.T) {
	host := newAppWithRuntime(nil, newDesktopRuntime())
	host.proofreadingModule = true
	host.managedBackgroundClosed = true
	host.ensureProofreadingWorkbench().loaded = true
	editor, state := proofreadingModelFixture(t)
	editor.proofreadingTabHost = host
	editor.pdfView = &pdfViewState{}
	state.aDocument = &proofreadingObject{Name: "书稿.pdf"}
	state.sourceLoading = true
	host.proofreadingTabs = []*proofreadingTab{{id: "proofreading:book", editor: editor}}
	host.selectedProofreadingTabID = "proofreading:book"
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1280, 820))}
		host.layout(g)
		router.Frame(&ops)
	}
	frame()
	state.xPageEditors[0].SetText("校对修订")
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, 20)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, 20)})
	frame()
	frame()
	if host.workspaceSelectedTabID() != workspace.RecentTabID {
		t.Fatal("did not switch to workbench")
	}
	if got := state.controller.Model.Text(1); got != "校对修订" {
		t.Fatalf("switch lost latest Chinese draft: %q", got)
	}
	if got := state.controller.Model.Text(2); got != "efgh" {
		t.Fatalf("switch changed another page: %q", got)
	}
}

func TestProofreadingModuleDoesNotImportSharedDocumentTabs(t *testing.T) {
	fixture := newWindowViewFixture(t)
	host := newAppWithRuntime(nil, fixture.runtime)
	host.proofreadingModule = true
	defer func() { host.closeAllProofreadingTabs(); host.stopAndWaitManagedBackground() }()
	// The real runtime already contains the source PDF (and may contain other
	// documents) when the separate proofreading window is constructed.
	meta := workspace.Tab{ID: "source-pdf", Kind: "document", Title: "book.pdf", DocumentID: "source-pdf", SourcePath: "/books/book.pdf", Position: 2}
	if err := fixture.runtime.collection.AddTab(meta, "pdf"); err != nil {
		t.Fatal(err)
	}
	if err := host.openProofreadingTab(proofreadingWindowRequest{sourceName: "book.pdf", sourceSnapshot: io.NopCloser(strings.NewReader("invalid test PDF"))}); err != nil {
		t.Fatal(err)
	}
	host.reconcileWorkspaceTabs()
	ids := host.workspaceTabIDs()
	if len(ids) != 1 || ids[0] != host.selectedProofreadingTabID {
		t.Fatalf("module imported ordinary document tabs: %v", ids)
	}
	if len(host.tabMeta) != 0 || len(host.sessions) != 0 {
		t.Fatal("module imported regular document metadata or views")
	}
	// Isolating the module must preserve the source window's open documents.
	if _, ok := fixture.runtime.collection.GetMetadata(meta.ID); !ok {
		t.Fatal("source PDF was removed from the shared collection")
	}
	for _, source := range fixture.apps {
		found := false
		for _, id := range source.workspaceTabIDs() {
			if id == meta.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("source window lost its PDF tab")
		}
	}
}
