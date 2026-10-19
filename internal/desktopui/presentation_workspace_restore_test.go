package desktopui

import (
	"path/filepath"
	"testing"

	"glance/internal/format/pptx"
	"glance/internal/workspace"
)

func TestPPTXNewDraftSurvivesTabSwitch(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	defer runtime.shutdown()
	app := newAppWithRuntime(nil, runtime)
	app.workspaceStore, app.collection = store, collection
	doc, err := pptx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.installDocument(loadedDocument{pptxDoc: doc}); err != nil {
		t.Fatal(err)
	}
	id := app.activeTabID
	wantSlides := doc.Snapshot().SlideCount
	app.activateRecent()
	if app.sessions[id] != nil {
		t.Fatal("inactive presentation was not evicted; test must exercise recovery")
	}
	state, err := app.viewForTabWithError(id)
	if err != nil {
		t.Fatalf("switch back to untouched PPTX: %v", err)
	}
	app.activateSession(id, state)
	if app.pptxDoc == nil || app.pptxDoc.Snapshot().SlideCount != wantSlides {
		t.Fatal("switching tabs lost the new presentation")
	}
	if app.pptxDoc.Dirty() || app.pptxDoc.Path() != "" {
		t.Fatal("untouched draft acquired changes or a disk path")
	}
}

func TestPPTXWorkspaceRestoreSources(t *testing.T) {
	doc, err := pptx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	path := filepath.Join(t.TempDir(), "source.pptx")
	if err := doc.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddSlide(); err != nil {
		t.Fatal(err)
	}
	payload, err := doc.SnapshotBytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		record workspace.Session
		slides int
	}{
		{"empty draft", workspace.Session{Tab: workspace.Tab{Kind: "draft"}, Format: "pptx"}, 1},
		{"source file", workspace.Session{Tab: workspace.Tab{SourcePath: path}, Format: "pptx"}, 1},
		{"edited snapshot", workspace.Session{Tab: workspace.Tab{SourcePath: path}, Format: "pptx", Payload: payload, Dirty: true}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := New(nil, "")
			defer close(app.shutdown)
			state, err := app.restoreSessionState(tc.record)
			if err != nil {
				t.Fatal(err)
			}
			defer state.pptxDoc.Close()
			if got := state.pptxDoc.Snapshot().SlideCount; got != tc.slides {
				t.Fatalf("slides=%d, want %d", got, tc.slides)
			}
			if state.pptxDoc.Path() != tc.record.Tab.SourcePath || state.pptxDoc.Dirty() != tc.record.Dirty {
				t.Fatal("source path or dirty state changed")
			}
			if tc.record.Dirty {
				if err := state.pptxDoc.Save(); err != nil {
					t.Fatalf("save recovered changes to original destination: %v", err)
				}
				reopened, err := pptx.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				if reopened.Snapshot().SlideCount != tc.slides {
					t.Fatal("recovered changes were not saved")
				}
			}
		})
	}
}

func TestPPTXWorkspaceRestoreRejectsInvalidSnapshot(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	_, err := app.restoreSessionState(workspace.Session{
		Tab: workspace.Tab{Kind: "draft"}, Format: "pptx", Payload: []byte("invalid PPTX"), Dirty: true,
	})
	if err == nil {
		t.Fatal("corrupt snapshot must not silently become a blank presentation")
	}
}
