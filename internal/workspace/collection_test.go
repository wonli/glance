package workspace

import (
	"path/filepath"
	"testing"
	"time"
)

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
