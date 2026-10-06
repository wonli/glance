package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/format/markdowndoc"
	"glance/internal/workspace"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownCreateEntrypointsWithDurableWorkspace(t *testing.T) {
	for _, entry := range []string{"home", "plus"} {
		t.Run(entry, func(t *testing.T) {
			store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			a := New(nil, "")
			defer close(a.shutdown)
			a.desktopRuntime.workspaceStore = store
			a.initializeWorkspace()
			if entry == "home" {
				a.homeActionButtons["md"] = new(widget.Clickable)
				a.homeActionButtons["md"].Click()
			} else {
				a.showNewMenu = true
				a.actionNewMarkdown.Click()
			}
			for i := 0; i < 2; i++ {
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1440, 900))}
				a.layout(gtx)
			}
			if a.activeTabID == workspace.RecentTabID || a.markdownDoc == nil {
				t.Fatalf("creation returned to Recent: tab=%q status=%q", a.activeTabID, a.status)
			}
			record, ok := a.collection.Get(a.activeTabID)
			if !ok || record.Format != "markdown" {
				t.Fatalf("missing Markdown catalog record: %+v", record)
			}
			doc, found, err := store.LoadDocument(record.Tab.DocumentID)
			if err != nil || !found || doc.Format != "markdown" || len(doc.Payload) != 0 {
				t.Fatalf("empty draft not stored: %+v %v %v", doc, found, err)
			}
			reopened, err := workspace.OpenCollection(store)
			if err != nil {
				t.Fatal(err)
			}
			restored, ok := reopened.Get(record.Tab.ID)
			if !ok || !hasRestorableSnapshot(*restored) {
				t.Fatal("empty draft unavailable after reopen")
			}
			state, err := a.restoreSessionState(*restored)
			if err != nil || state.markdownDoc == nil || state.markdownDoc.Text() != "" {
				t.Fatalf("empty recovery: %v", err)
			}
		})
	}
}

func TestMarkdownEmptySavedSourceIsRestorable(t *testing.T) {
	record := workspace.Session{Tab: workspace.Tab{ID: "notes", Kind: "document", SourcePath: "/tmp/notes.md"}, Format: "markdown", Payload: []byte{}, Dirty: true}
	if !hasRestorableSnapshot(record) {
		t.Fatal("empty edited source omitted from recovery")
	}
}

func TestMarkdownWorkspaceRecovery(t *testing.T) {
	for _, ext := range []string{".md", ".MARKDOWN"} {
		path := filepath.Join(t.TempDir(), "notes"+ext)
		if err := os.WriteFile(path, []byte("# 中文\r\n"), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadDocument(path)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.markdownDoc == nil {
			t.Fatal("missing Markdown adapter")
		}
		shared := sharedSessionFromLoadedDocument(loaded)
		state := newDocumentSessionState(path)
		state.sharedDocumentSession = shared
		if err := shared.markdownDoc.SetText(""); err != nil {
			t.Fatal(err)
		}
		snapshot, err := snapshotState(state)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot) != 0 || !sessionDirty(state) {
			t.Fatal("empty edited source not recorded")
		}
		a := &App{}
		restored, err := a.restoreSessionState(workspace.Session{Tab: workspace.Tab{SourcePath: path}, Format: "markdown", Payload: snapshot, Dirty: true})
		if err != nil {
			t.Fatal(err)
		}
		if restored.markdownDoc.Text() != "" || !sessionDirty(restored) {
			t.Fatal("empty draft recovery failed")
		}
		closeLoadedDocument(loaded)
	}
}

func TestMarkdownDraftIdentityAndSaveAs(t *testing.T) {
	shared := sharedSessionFromLoadedDocument(loadedDocument{markdownDoc: markdowndoc.NewBlank()})
	state := newDocumentSessionView(shared, 0)
	if got := sessionFormat(workspace.Tab{Kind: "draft"}, state); got != "markdown" {
		t.Fatalf("format %q", got)
	}
	for _, name := range []string{"a.md", "a.MARKDOWN"} {
		if err := validateSaveAsExtension("markdown", name); err != nil {
			t.Fatal(err)
		}
	}
	if validateSaveAsExtension("markdown", "a.json") == nil {
		t.Fatal("wrong extension accepted")
	}
}
