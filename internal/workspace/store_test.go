package workspace

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveTabID != RecentTabID || len(got.Tabs) != 0 {
		t.Fatalf("unexpected initial workspace: %+v", got)
	}
}

func TestWorkspaceAndDocumentPayloadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(123, 0).UTC()
	want := Snapshot{ActiveTabID: "doc-1", Tabs: []Tab{{ID: "doc-1", Kind: "document", Title: "Budget.xlsx", DocumentID: "doc-1", Position: 0}}, Recents: []Recent{{Path: "/tmp/Budget.xlsx", Kind: "xlsx", OpenedAt: now}}}
	if err := s.ReplaceWorkspace(want); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDocument(Document{ID: "doc-1", Format: "xlsx", Payload: []byte{0, 1, 2, 255}, Dirty: true, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveTabID != want.ActiveTabID || len(got.Tabs) != 1 || got.Tabs[0] != want.Tabs[0] || len(got.Recents) != 1 || !got.Recents[0].OpenedAt.Equal(now) {
		t.Fatalf("workspace mismatch: got %+v want %+v", got, want)
	}
	doc, ok, err := s.LoadDocument("doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || doc.Format != "xlsx" || !doc.Dirty || !bytes.Equal(doc.Payload, []byte{0, 1, 2, 255}) || !doc.UpdatedAt.Equal(now) {
		t.Fatalf("document mismatch: %+v (found=%v)", doc, ok)
	}
}

func TestReplaceWorkspaceIsTransactional(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial := Snapshot{ActiveTabID: "one", Tabs: []Tab{{ID: "one", Kind: "document", Position: 0}}}
	if err := s.ReplaceWorkspace(initial); err != nil {
		t.Fatal(err)
	}
	broken := Snapshot{ActiveTabID: "two", Tabs: []Tab{{ID: "two", Kind: "document", Position: 0}, {ID: "three", Kind: "document", Position: 0}}}
	if err := s.ReplaceWorkspace(broken); err == nil {
		t.Fatal("expected invalid active tab to fail")
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveTabID != "one" || len(got.Tabs) != 1 || got.Tabs[0].ID != "one" {
		t.Fatalf("failed replacement changed persisted workspace: %+v", got)
	}
}
