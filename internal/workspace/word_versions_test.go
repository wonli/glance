package workspace

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWordVersionMigrateV1PreservesWorkspace(t *testing.T) {
	assertLegacyWorkspaceMigratesToV5(t, 1)
}

func TestWordVersionMigrateV2PreservesWorkspace(t *testing.T) {
	assertLegacyWorkspaceMigratesToV5(t, 2)
}

func TestWordVersionMigrateV3PreservesWorkspace(t *testing.T) {
	assertLegacyWorkspaceMigratesToV5(t, 3)
}

func assertLegacyWorkspaceMigratesToV5(t *testing.T, version int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyWorkspaceDatabase(t, path, version)

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after v%d migration: %v", version, err)
	}
	defer store.Close()

	var migratedVersion int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&migratedVersion); err != nil {
		t.Fatal(err)
	}
	if migratedVersion != 6 {
		t.Fatalf("user_version=%d; want 6", migratedVersion)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantTime := time.Unix(123, 0).UTC()
	if got.ActiveTabID != "doc-1" ||
		len(got.Tabs) != 1 ||
		got.Tabs[0] != (Tab{ID: "doc-1", Kind: "document", Title: "legacy.docx", DocumentID: "doc-1", SourcePath: "/tmp/legacy.docx", Position: 0}) ||
		len(got.Recents) != 1 ||
		got.Recents[0] != (Recent{Path: "/tmp/legacy.docx", Kind: "docx", OpenedAt: wantTime}) {
		t.Fatalf("legacy workspace changed during migration: %+v", got)
	}
	doc, ok, err := store.LoadDocument("doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || doc.Format != "docx" || string(doc.Payload) != "legacy-payload" || doc.Dirty || !doc.UpdatedAt.Equal(wantTime) {
		t.Fatalf("legacy document changed during migration: %+v (found=%v)", doc, ok)
	}
	if version == 3 {
		favorites, err := store.LoadFavorites()
		if err != nil {
			t.Fatal(err)
		}
		if len(favorites) != 1 || favorites[0] != "/tmp/legacy.docx" {
			t.Fatalf("legacy favorites changed during migration: %v", favorites)
		}
	}
}

func createLegacyWorkspaceDatabase(t *testing.T, path string, version int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	queries := []string{
		"CREATE TABLE documents (id TEXT PRIMARY KEY, format TEXT NOT NULL, payload BLOB NOT NULL, dirty INTEGER NOT NULL, updated_at INTEGER NOT NULL)",
		"CREATE TABLE tabs (id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL, document_id TEXT NOT NULL DEFAULT '', source_path TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL UNIQUE)",
		"CREATE TABLE recents (path TEXT PRIMARY KEY, kind TEXT NOT NULL, opened_at INTEGER NOT NULL)",
		"CREATE TABLE workspace_settings (singleton INTEGER PRIMARY KEY CHECK(singleton = 1), active_tab_id TEXT NOT NULL)",
		"INSERT INTO workspace_settings(singleton, active_tab_id) VALUES (1, 'doc-1')",
		"INSERT INTO documents(id, format, payload, dirty, updated_at) VALUES ('doc-1', 'docx', X'6c65676163792d7061796c6f6164', 0, 123000000000)",
		"INSERT INTO tabs(id, kind, title, document_id, source_path, position) VALUES ('doc-1', 'document', 'legacy.docx', 'doc-1', '/tmp/legacy.docx', 0)",
		"INSERT INTO recents(path, kind, opened_at) VALUES ('/tmp/legacy.docx', 'docx', 123000000000)",
	}
	if version >= 2 {
		queries = append(queries, "ALTER TABLE documents ADD COLUMN view_state BLOB NOT NULL DEFAULT X''")
	}
	if version >= 3 {
		queries = append(queries, "CREATE TABLE favorites (path TEXT PRIMARY KEY)")
		queries = append(queries, "INSERT INTO favorites(path) VALUES ('/tmp/legacy.docx')")
	}
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("create v%d fixture: %v", version, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		t.Fatal(err)
	}
}

func TestWordVersionAddDeduplicatesByPathAndHash(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "versions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	capturedAt := time.Unix(100, 0).UTC()
	input := WordVersionInput{
		PathKey:     "/docs/report.docx",
		DisplayPath: "/docs/report.docx",
		Source:      WordVersionOpened,
		CapturedAt:  capturedAt,
		Snapshot:    []byte("same package"),
	}

	first, added, err := store.AddWordVersion(input)
	if err != nil || !added {
		t.Fatalf("first AddWordVersion() = (%+v, %v, %v); want inserted", first, added, err)
	}
	duplicate := input
	duplicate.Source = WordVersionManualSave
	duplicate.CapturedAt = capturedAt.Add(time.Hour)
	again, added, err := store.AddWordVersion(duplicate)
	if err != nil || added || again.ID != first.ID || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("duplicate AddWordVersion() = (%+v, %v, %v); want original record", again, added, err)
	}
	otherPath := input
	otherPath.PathKey = "/docs/copy.docx"
	otherPath.DisplayPath = "/docs/copy.docx"
	other, added, err := store.AddWordVersion(otherPath)
	if err != nil || !added || other.ID == first.ID {
		t.Fatalf("same package at another path = (%+v, %v, %v); want an independent version", other, added, err)
	}
	if first.SHA256 != fmt.Sprintf("%x", sha256.Sum256(input.Snapshot)) || first.SizeBytes != int64(len(input.Snapshot)) {
		t.Fatalf("stored digest/size = %s/%d", first.SHA256, first.SizeBytes)
	}
	loaded, payload, err := store.LoadWordVersion(input.PathKey, first.ID)
	if err != nil || loaded.ID != first.ID || !bytes.Equal(payload, input.Snapshot) {
		t.Fatalf("LoadWordVersion() = (%+v, %q, %v)", loaded, payload, err)
	}
	versions, err := store.ListWordVersions(input.PathKey)
	if err != nil || len(versions) != 1 || versions[0].ID != first.ID {
		t.Fatalf("ListWordVersions() = (%+v, %v); want one deduplicated item", versions, err)
	}
}

func TestWordVersionListOmitsPayload(t *testing.T) {
	versionType := reflect.TypeOf(WordVersion{})
	for index := 0; index < versionType.NumField(); index++ {
		field := versionType.Field(index)
		if strings.Contains(strings.ToLower(field.Name), "payload") || strings.Contains(strings.ToLower(field.Name), "snapshot") {
			t.Fatalf("metadata WordVersion includes package bytes in field %q", field.Name)
		}
	}
}

func TestWordVersionAutoSaveThrottle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "versions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := "/docs/autosave.docx"
	start := time.Unix(1_700_000_000, 0).UTC()
	put := func(source WordVersionSource, at time.Time, content string) (bool, error) {
		_, added, err := store.AddWordVersion(WordVersionInput{
			PathKey: path, DisplayPath: path, Source: source, CapturedAt: at, Snapshot: []byte(content),
		})
		return added, err
	}
	if added, err := put(WordVersionAutoSave, start, "auto 1"); err != nil || !added {
		t.Fatalf("first auto-save added=%v err=%v", added, err)
	}
	if added, err := put(WordVersionAutoSave, start.Add(4*time.Minute), "auto 2"); err != nil || added {
		t.Fatalf("auto-save inside five-minute window added=%v err=%v; want skipped", added, err)
	}
	if added, err := put(WordVersionManualSave, start.Add(4*time.Minute), "manual"); err != nil || !added {
		t.Fatalf("manual save inside auto window added=%v err=%v; want recorded", added, err)
	}
	if added, err := put(WordVersionAutoSave, start.Add(5*time.Minute), "auto 3"); err != nil || !added {
		t.Fatalf("auto-save at five-minute boundary added=%v err=%v; want recorded", added, err)
	}
	if added, err := put(WordVersionAutoSave, start.Add(6*time.Minute), "auto 4"); err != nil || added {
		t.Fatalf("auto-save one minute after prior auto version added=%v err=%v; want skipped", added, err)
	}
}

func TestWordVersionRetentionPrunesOldestAndProtectsRestoreBefore(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "versions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.wordVersionPolicy = wordVersionPolicy{
		maxVersions: 3, maxBytes: 8, maxSnapshotBytes: 8, autoSaveInterval: 5 * time.Minute,
	}
	path := "/docs/retention.docx"
	add := func(id string, source WordVersionSource, at int64) WordVersion {
		t.Helper()
		version, added, err := store.AddWordVersion(WordVersionInput{
			PathKey: path, DisplayPath: path, Source: source,
			CapturedAt: time.Unix(at, 0).UTC(), Snapshot: []byte(id + "!!"),
		})
		if err != nil || !added {
			t.Fatalf("AddWordVersion(%q) = (%+v, %v, %v)", id, version, added, err)
		}
		return version
	}
	add("a", WordVersionOpened, 1)
	add("b", WordVersionManualSave, 2)
	add("c", WordVersionManualSave, 3)
	restore := add("d", WordVersionRestoreBefore, 4)
	versions, err := store.ListWordVersions(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].ID != restore.ID || versions[0].Source != WordVersionRestoreBefore {
		t.Fatalf("retained versions = %+v; want newest restore-before and one recent version", versions)
	}
	var totalBytes int64
	for _, version := range versions {
		totalBytes += version.SizeBytes
	}
	if totalBytes > store.wordVersionPolicy.maxBytes {
		t.Fatalf("retained %d bytes; limit is %d", totalBytes, store.wordVersionPolicy.maxBytes)
	}
}

func TestWordVersionSizeBoundary(t *testing.T) {
	if err := validateWordVersionSize(512 << 20); err != nil {
		t.Fatalf("512 MiB snapshot rejected: %v", err)
	}
	if err := validateWordVersionSize((512 << 20) + 1); err == nil {
		t.Fatal("snapshot larger than 512 MiB was accepted")
	}
}

func TestWordVersionAddRollsBackOnFailure(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "versions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.wordVersionPolicy = wordVersionPolicy{
		maxVersions: 1, maxBytes: 1024, maxSnapshotBytes: 1024, autoSaveInterval: 5 * time.Minute,
	}
	path := "/docs/atomic.docx"
	first, added, err := store.AddWordVersion(WordVersionInput{
		PathKey: path, DisplayPath: path, Source: WordVersionOpened,
		CapturedAt: time.Unix(1, 0).UTC(), Snapshot: []byte("first"),
	})
	if err != nil || !added {
		t.Fatalf("initial version = (%+v, %v, %v)", first, added, err)
	}
	if _, err := store.db.Exec("CREATE TRIGGER deny_word_version_delete BEFORE DELETE ON word_versions BEGIN SELECT RAISE(ABORT, 'deny prune'); END"); err != nil {
		t.Fatal(err)
	}
	if _, added, err := store.AddWordVersion(WordVersionInput{
		PathKey: path, DisplayPath: path, Source: WordVersionManualSave,
		CapturedAt: time.Unix(2, 0).UTC(), Snapshot: []byte("second"),
	}); err == nil || added {
		t.Fatalf("version write with failed prune = added %v, err %v; want transaction error", added, err)
	}
	versions, err := store.ListWordVersions(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != first.ID {
		t.Fatalf("failed transaction left versions %+v; want original only", versions)
	}
}
