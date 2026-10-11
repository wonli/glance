package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/application"
	"glance/internal/workspace"
)

func TestApplicationDocumentOpenedEventAddsMCPDocumentToDesktopWorkspace(t *testing.T) {
	ctx := context.Background()
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtime.workspaceStore, runtime.collection = store, collection
	app := newAppWithRuntime(nil, runtime)
	runtime.attachWindow(1, app)

	created, err := runtime.applicationRuntime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("create document through application runtime: %v", err)
	}
	app.processAgentApplicationEvents()
	if meta, ok := app.tabMeta[created.DocumentID]; !ok || meta.DocumentID != created.DocumentID {
		t.Fatalf("desktop tab for application document = %+v, found=%v", meta, ok)
	}
	if app.activeTabID != created.DocumentID || app.documentSessionState.documentID != created.DocumentID {
		t.Fatalf("active desktop document = %q / %q; want %q", app.activeTabID, app.documentSessionState.documentID, created.DocumentID)
	}
	if _, ok := runtime.session(created.DocumentID); !ok {
		t.Fatal("application document has no live desktop session")
	}

	target := filepath.Join(t.TempDir(), "agent-report.docx")
	if _, err := runtime.applicationRuntime.SaveDocumentAs(ctx, created.DocumentID, target); err != nil {
		t.Fatalf("save MCP-created document: %v", err)
	}
	app.processAgentApplicationEvents()
	meta := app.tabMeta[created.DocumentID]
	if meta.Title != "agent-report.docx" || canonicalDocumentPath(meta.SourcePath) != canonicalDocumentPath(target) || meta.Kind != "document" {
		t.Fatalf("desktop tab metadata after save = %+v", meta)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("saved Word document is missing: %v", err)
	}
}
