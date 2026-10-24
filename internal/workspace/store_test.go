package workspace

import (
	"bytes"
	"errors"
	"os"
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

func TestFavoritesSurviveRestartAndMigrateVersionTwoDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE favorites`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE word_versions`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE agent_conversations`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE preferences`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE home_document_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE home_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFavorite("/tmp/budget.xlsx", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	favorites, err := store.LoadFavorites()
	if err != nil {
		t.Fatal(err)
	}
	if len(favorites) != 1 || favorites[0] != "/tmp/budget.xlsx" {
		t.Fatalf("persisted favorites=%v; want [/tmp/budget.xlsx]", favorites)
	}
	if err := store.SaveFavorite("/tmp/budget.xlsx", false); err != nil {
		t.Fatal(err)
	}
	favorites, err = store.LoadFavorites()
	if err != nil {
		t.Fatal(err)
	}
	if len(favorites) != 0 {
		t.Fatalf("favorites after removal=%v; want empty", favorites)
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

func TestStoreAutoMaintainSpace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert large payload rows directly into SQLite to allocate pages for freelist reclamation test
	largePayload := make([]byte, 2*1024*1024)
	id := filepath.Base(path) + "-doc"
	if _, err := s.db.Exec(`INSERT INTO documents(id,format,payload,dirty,updated_at,view_state) VALUES(?, 'docx', ?, 0, ?, X'')`, id, largePayload, time.Now().UTC().UnixNano()); err != nil {
		t.Fatal(err)
	}

	// Delete document in SQLite to create dead pages in freelist
	if err := s.DeleteDocument(id); err != nil {
		t.Fatal(err)
	}

	var freelist int64
	if err := s.db.QueryRow(`PRAGMA freelist_count`).Scan(&freelist); err != nil {
		t.Fatal(err)
	}
	if freelist == 0 {
		t.Fatal("expected non-zero freelist after delete")
	}

	maintained, err := s.AutoMaintainSpace()
	if err != nil {
		t.Fatalf("AutoMaintainSpace failed: %v", err)
	}
	if !maintained {
		t.Fatal("expected AutoMaintainSpace to return true and vacuum freelist")
	}

	if err := s.db.QueryRow(`PRAGMA freelist_count`).Scan(&freelist); err != nil {
		t.Fatal(err)
	}
	if freelist != 0 {
		t.Fatalf("expected 0 freelist pages after AutoMaintainSpace, got %d", freelist)
	}
}

func TestStoreFileSessionPayloadLifecycleAndOrphanCleanup(t *testing.T) {
	dbDir := t.TempDir()
	path := filepath.Join(dbDir, "workspace.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	payload := []byte("hello-session-file-storage")
	docID := "tab-doc-test"
	if err := s.SaveDocument(Document{
		ID:        docID,
		Format:    "docx",
		Payload:   payload,
		Dirty:     true,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	// Verify SQLite documents.payload column is empty (0 bytes)
	var sqlitePayload []byte
	if err := s.db.QueryRow(`SELECT payload FROM documents WHERE id=?`, docID).Scan(&sqlitePayload); err != nil {
		t.Fatalf("query SQLite payload: %v", err)
	}
	if len(sqlitePayload) != 0 {
		t.Fatalf("expected SQLite payload to be empty, got %d bytes", len(sqlitePayload))
	}

	// Verify session file exists on disk
	datPath := s.payloadPath(docID)
	if fi, err := os.Stat(datPath); err != nil || fi.Size() != int64(len(payload)) {
		t.Fatalf("session file %q stat error=%v size=%d, want %d", datPath, err, fi.Size(), len(payload))
	}

	// Verify LoadDocument returns the exact content
	loaded, found, err := s.LoadDocument(docID)
	if err != nil || !found {
		t.Fatalf("LoadDocument found=%v err=%v", found, err)
	}
	if !bytes.Equal(loaded.Payload, payload) || loaded.PayloadSize != int64(len(payload)) {
		t.Fatalf("LoadDocument payload mismatch: got %q, want %q", loaded.Payload, payload)
	}

	// Create an artificial orphan .dat file and a leftover .tmp-session- file
	orphanDat := filepath.Join(s.sessionsDir, "orphan-9999999999999999.dat")
	if err := os.WriteFile(orphanDat, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	leftoverTmp := filepath.Join(s.sessionsDir, ".tmp-session-temp123")
	if err := os.WriteFile(leftoverTmp, []byte("tmp"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Call CleanupOrphanedSessions
	if err := s.CleanupOrphanedSessions(); err != nil {
		t.Fatalf("CleanupOrphanedSessions: %v", err)
	}

	// Verify orphan files were deleted
	if _, err := os.Stat(orphanDat); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected orphan .dat to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(leftoverTmp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected leftover .tmp to be deleted, got err=%v", err)
	}

	// Verify valid session file still exists
	if _, err := os.Stat(datPath); err != nil {
		t.Fatalf("valid session file was accidentally deleted: %v", err)
	}

	// Delete valid document
	if err := s.DeleteDocument(docID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	// Verify valid session file is now deleted
	if _, err := os.Stat(datPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected session file to be removed after DeleteDocument, got err=%v", err)
	}
}
