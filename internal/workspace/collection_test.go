package workspace

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCollectionEmptyMarkdownSnapshotAndMissingSnapshotGate(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"markdown", "md", "docx", "xlsx", "json"} {
		missing := Session{Tab: Tab{ID: "missing-" + format, Kind: "draft"}, Format: format}
		if c.AddDocument(missing, true) == nil {
			t.Fatalf("missing %s snapshot accepted", format)
		}
		missing.Payload = []byte{}
		if format == "md" || format == "markdown" {
			if err = c.AddDocument(missing, true); err != nil {
				t.Fatal(err)
			}
			doc, found, err := s.LoadDocument(missing.Tab.ID)
			if err != nil || !found || doc.Format != format || len(doc.Payload) != 0 {
				t.Fatalf("empty snapshot not durable: %+v %v %v", doc, found, err)
			}
		} else if c.AddDocument(missing, true) == nil {
			t.Fatalf("empty structured %s snapshot accepted", format)
		}
	}
}

func TestCollectionStageRecentsSnapshotReplacesAndRestoresOrder(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	opened := time.Date(2026, 10, 13, 10, 20, 30, 0, time.UTC)
	original := []Recent{
		{Path: "/docs/first.xlsx", Kind: "xlsx", OpenedAt: opened},
		{Path: "/docs/second.docx", Kind: "docx", OpenedAt: opened.Add(time.Minute)},
	}
	collection.StageRecentsSnapshot(original)
	collection.StageRecentsSnapshot(original[1:])
	if got := collection.Recents(); len(got) != 1 || got[0].Path != original[1].Path {
		t.Fatalf("staged removal recents=%+v; want only %q", got, original[1].Path)
	}
	collection.StageRecentsSnapshot(original)
	if got := collection.Recents(); len(got) != len(original) || got[0] != original[0] || got[1] != original[1] {
		t.Fatalf("staged undo recents=%+v; want original order and timestamps %+v", got, original)
	}
}

func TestCollectionSwitchAndCloseKeepSessionsIndependent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put(Session{Tab: Tab{ID: "a", Kind: "document", Title: "a.xlsx", DocumentID: "a"}, Format: "xlsx", Payload: []byte("A"), ViewState: []byte("cell=A1"), Dirty: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(Session{Tab: Tab{ID: "b", Kind: "document", Title: "b.docx", DocumentID: "b"}, Format: "docx", Payload: []byte("B"), ViewState: []byte("block=2")}); err != nil {
		t.Fatal(err)
	}
	if err := c.Activate("a"); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseTab("b"); err != nil {
		t.Fatal(err)
	}
	if got := c.Active().Tab.ID; got != "a" {
		t.Fatalf("active = %q", got)
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("closing b removed a")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("closed tab b remains")
	}
	doc, ok, err := s.LoadDocument("a")
	if err != nil || !ok {
		t.Fatalf("load a: found=%v err=%v", ok, err)
	}
	if string(doc.Payload) != "A" || string(doc.ViewState) != "cell=A1" || !doc.Dirty {
		t.Fatalf("persisted a=%+v", doc)
	}
}

func TestCollectionAddTabInactivePreservesActiveTab(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.AddTab(Tab{ID: "primary", Title: "primary.docx"}, "docx"); err != nil {
		t.Fatal(err)
	}
	if err := collection.AddTabInactive(Tab{ID: "secondary", Title: "secondary.docx"}, "docx"); err != nil {
		t.Fatal(err)
	}
	if got := collection.ActiveTabID(); got != "primary" {
		t.Fatalf("active tab after inactive add = %q, want primary", got)
	}
	if _, ok := collection.Get("secondary"); !ok {
		t.Fatal("inactive tab was not added to the shared catalog")
	}
}

func TestCollectionConcurrentWindowReadsAndActivation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if err := collection.AddTab(Tab{ID: id, Title: id}, "docx"); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 25; iteration++ {
				if err := collection.Activate([]string{"first", "second"}[(worker+iteration)%2]); err != nil {
					t.Errorf("activate workspace tab: %v", err)
					return
				}
				_ = collection.ActiveTabID()
				_ = collection.Tabs()
				_ = collection.Recents()
				collection.StageRecents([]Recent{{Path: "/tmp/concurrent.docx", Kind: "docx"}})
			}
		}()
	}
	wg.Wait()
}

func TestCollectionRestoreKeepsSnapshotWhenSourceIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put(Session{Tab: Tab{ID: "lost", Kind: "document", Title: "lost.xlsx", DocumentID: "lost", SourcePath: filepath.Join(t.TempDir(), "gone.xlsx")}, Format: "xlsx", Payload: []byte("recoverable"), Dirty: true}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	session, ok := restored.Get("lost")
	if !ok || session.Available || string(session.Payload) != "recoverable" || !session.Dirty {
		t.Fatalf("recovered session = %+v found=%v", session, ok)
	}
}

func TestCollectionPersistsActiveTabAndViewStateOnRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC()
	if err := c.Put(Session{Tab: Tab{ID: "doc", Kind: "draft", Title: "New.xlsx", DocumentID: "doc"}, Format: "xlsx", Payload: []byte("bytes"), ViewState: []byte("sheet=Sheet2;cell=F8"), UpdatedAt: stamp}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ActiveTabID() != "doc" {
		t.Fatalf("active = %q", restored.ActiveTabID())
	}
	doc, ok := restored.Get("doc")
	if !ok || string(doc.ViewState) != "sheet=Sheet2;cell=F8" || string(doc.Payload) != "bytes" {
		t.Fatalf("restored session=%+v found=%v", doc, ok)
	}
}

func TestRemoveRecentPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddRecent(Recent{Path: "/tmp/missing.xlsx", Kind: "xlsx"}); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveRecent("/tmp/missing.xlsx"); err != nil {
		t.Fatal(err)
	}
	if len(c.Recents()) != 0 {
		t.Fatalf("recent entries remain: %+v", c.Recents())
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Recents) != 0 {
		t.Fatalf("recent entries restored: %+v", w.Recents)
	}
}

func TestStageRecentsAndPersistThemAsOneBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	items := []Recent{{Path: "/tmp/a.xlsx", Kind: "xlsx"}, {Path: "/tmp/b.docx", Kind: "docx"}}
	collection.StageRecents(items)
	if len(collection.Recents()) != 2 {
		t.Fatalf("staged recents=%+v", collection.Recents())
	}
	if err := store.SaveRecents(collection.Recents()); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	w, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Recents) != 2 {
		t.Fatalf("persisted recents=%+v", w.Recents)
	}
}
