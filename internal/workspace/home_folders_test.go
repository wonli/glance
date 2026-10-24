package workspace

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestHomeFoldersMigrateFromV6AndPreserveExistingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	openedAt := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	wantWorkspace := Snapshot{
		ActiveTabID: "document-1",
		Tabs:        []Tab{{ID: "document-1", Kind: "document", Title: "report.docx", DocumentID: "document-1", SourcePath: "/tmp/report.docx", Position: 0}},
		Recents:     []Recent{{Path: "/tmp/report.docx", Kind: "docx", OpenedAt: openedAt}},
	}
	if err := store.ReplaceWorkspace(wantWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDocument(Document{ID: "document-1", Format: "docx", Payload: []byte("existing payload"), UpdatedAt: openedAt}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFavorite("/tmp/report.docx", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE IF EXISTS home_document_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE IF EXISTS home_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA user_version = 6`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("Open() after v6 migration: %v", err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Fatalf("user_version=%d; want 8", version)
	}
	gotWorkspace, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotWorkspace, wantWorkspace) {
		t.Fatalf("existing workspace changed during migration:\n got: %+v\nwant: %+v", gotWorkspace, wantWorkspace)
	}
	doc, found, err := store.LoadDocument("document-1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || doc.Format != "docx" || string(doc.Payload) != "existing payload" || !doc.UpdatedAt.Equal(openedAt) {
		t.Fatalf("existing document changed during migration: %+v (found=%v)", doc, found)
	}
	favorites, err := store.LoadFavorites()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(favorites, []string{"/tmp/report.docx"}) {
		t.Fatalf("favorites changed during migration: %v", favorites)
	}
	folders, err := store.LoadHomeFolderSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(folders.Folders) != 0 || len(folders.Assignments) != 0 {
		t.Fatalf("new folder snapshot is not empty after migration: %+v", folders)
	}
}

func TestHomeFolderSnapshotRoundTripsAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := HomeFolderSnapshot{
		Folders: []HomeFolder{
			{ID: "projects", Name: "Projects", SortOrder: 0},
			{ID: "active", ParentID: "projects", Name: "Active", SortOrder: 0},
			{ID: "imported", Name: "Imported", SortOrder: 1, SourcePath: "/tmp/imported"},
		},
		Assignments: []HomeFolderAssignment{
			{Path: "/tmp/data.xlsx", FolderID: "imported"},
			{Path: "/tmp/plan.docx", FolderID: "active"},
		},
	}
	if err := store.SaveHomeFolderSnapshot(want); err != nil {
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
	got, err := store.LoadHomeFolderSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded folder snapshot mismatch:\n got: %+v\nwant: %+v", got, want)
	}
	renamed, err := got.RenameFolder("imported", "Imported Archive")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Folders[2].SourcePath != "/tmp/imported" {
		t.Fatalf("renaming an imported folder changed its source path: %+v", renamed.Folders[2])
	}
	replacement := HomeFolderSnapshot{Folders: []HomeFolder{{ID: "archive", Name: "Archive", SortOrder: 0}}}
	if err := store.SaveHomeFolderSnapshot(replacement); err != nil {
		t.Fatal(err)
	}
	got, err = store.LoadHomeFolderSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, replacement) {
		t.Fatalf("replaced folder snapshot mismatch:\n got: %+v\nwant: %+v", got, replacement)
	}
}

func TestAssignHomeDocumentReplacesExistingFolder(t *testing.T) {
	snapshot := HomeFolderSnapshot{
		Folders:     []HomeFolder{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}},
		Assignments: []HomeFolderAssignment{{Path: "/tmp/report.docx", FolderID: "one"}},
	}
	got, err := snapshot.AssignDocument("/tmp/report.docx", "two")
	if err != nil {
		t.Fatal(err)
	}
	want := HomeFolderSnapshot{
		Folders:     snapshot.Folders,
		Assignments: []HomeFolderAssignment{{Path: "/tmp/report.docx", FolderID: "two"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AssignDocument() = %+v; want %+v", got, want)
	}
	got, err = got.AssignDocument("/tmp/report.docx", "two")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assigning to the current folder changed the snapshot: %+v", got)
	}
}

func TestHomeFolderSnapshotRejectsInvalidTreesAndNames(t *testing.T) {
	root := HomeFolder{ID: "root", Name: "Root"}
	base := HomeFolderSnapshot{Folders: []HomeFolder{root}}
	created, err := base.AddFolder(HomeFolder{ID: "alpha", ParentID: "root", Name: "  Alpha  "})
	if err != nil {
		t.Fatal(err)
	}
	if created.Folders[1].Name != "Alpha" {
		t.Fatalf("AddFolder() name=%q; want trimmed Alpha", created.Folders[1].Name)
	}
	withSibling, err := created.AddFolder(HomeFolder{ID: "beta", ParentID: "root", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withSibling.AddFolder(HomeFolder{ID: "duplicate", ParentID: "root", Name: "alpha"}); err == nil {
		t.Fatal("AddFolder() accepted a case-insensitive sibling duplicate")
	}
	renamed, err := withSibling.RenameFolder("alpha", "  Renamed  ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Folders[1].Name != "Renamed" {
		t.Fatalf("RenameFolder() name=%q; want trimmed Renamed", renamed.Folders[1].Name)
	}
	if _, err := withSibling.RenameFolder("alpha", "BETA"); err == nil {
		t.Fatal("RenameFolder() accepted a case-insensitive sibling duplicate")
	}
	withOtherRoot, err := created.AddFolder(HomeFolder{ID: "other-root", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withOtherRoot.AddFolder(HomeFolder{ID: "other-alpha", ParentID: "other-root", Name: "alpha"}); err != nil {
		t.Fatalf("AddFolder() rejected a same name under a different parent: %v", err)
	}
	for _, test := range []struct{ label, name string }{
		{label: "empty", name: ""},
		{label: "whitespace", name: " \t "},
		{label: "slash", name: "bad/name"},
		{label: "backslash", name: `bad\name`},
		{label: "control", name: "bad\nname"},
	} {
		t.Run("invalid_name_"+test.label, func(t *testing.T) {
			if _, err := base.AddFolder(HomeFolder{ID: "invalid", ParentID: "root", Name: test.name}); err == nil {
				t.Fatalf("AddFolder() accepted invalid name %q", test.name)
			}
		})
	}

	valid := HomeFolderSnapshot{
		Folders:     []HomeFolder{{ID: "root", Name: "Root"}, {ID: "child", ParentID: "root", Name: "Child"}},
		Assignments: []HomeFolderAssignment{{Path: "/tmp/report.docx", FolderID: "child"}},
	}
	tests := []struct {
		name     string
		snapshot HomeFolderSnapshot
	}{
		{name: "duplicate_id", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "x", Name: "One"}, {ID: "x", Name: "Two"}}}},
		{name: "missing_parent", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "x", ParentID: "missing", Name: "Child"}}}},
		{name: "parent_cycle", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "a", ParentID: "b", Name: "A"}, {ID: "b", ParentID: "a", Name: "B"}}}},
		{name: "case_insensitive_sibling_duplicate", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "root", Name: "Root"}, {ID: "one", ParentID: "root", Name: "Work"}, {ID: "two", ParentID: "root", Name: "work"}}}},
		{name: "duplicate_source_path", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "one", Name: "One", SourcePath: "/tmp/source"}, {ID: "two", Name: "Two", SourcePath: "/tmp/source"}}}},
		{name: "child_source_path", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "root", Name: "Root"}, {ID: "child", ParentID: "root", Name: "Child", SourcePath: "/tmp/source"}}}},
		{name: "duplicate_document_path", snapshot: HomeFolderSnapshot{Folders: []HomeFolder{{ID: "root", Name: "Root"}}, Assignments: []HomeFolderAssignment{{Path: "/tmp/a", FolderID: "root"}, {Path: "/tmp/a", FolderID: "root"}}}},
		{name: "assignment_to_missing_folder", snapshot: HomeFolderSnapshot{Assignments: []HomeFolderAssignment{{Path: "/tmp/a", FolderID: "missing"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateHomeFolderSnapshot(test.snapshot); err == nil {
				t.Fatalf("ValidateHomeFolderSnapshot(%+v) returned nil; want validation error", test.snapshot)
			}
		})
	}
	if err := ValidateHomeFolderSnapshot(valid); err != nil {
		t.Fatalf("ValidateHomeFolderSnapshot(valid) = %v", err)
	}
}

func TestLoadHomeFolderSnapshotRejectsCorruptRows(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO home_folders(id, parent_id, name, sort_order, source_path) VALUES ('a', NULL, 'A', 0, NULL), ('b', NULL, 'B', 1, NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE home_folders SET parent_id='b' WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE home_folders SET parent_id='a' WHERE id='b'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadHomeFolderSnapshot(); err == nil {
		t.Fatal("LoadHomeFolderSnapshot() accepted a cyclic persisted tree")
	}
}

func TestRemoveHomeFolderPromotesChildrenAndUngroupsDirectDocuments(t *testing.T) {
	snapshot := HomeFolderSnapshot{
		Folders: []HomeFolder{
			{ID: "root", Name: "Root", SortOrder: 0},
			{ID: "sibling", ParentID: "root", Name: "Sibling", SortOrder: 3},
			{ID: "remove", ParentID: "root", Name: "Remove", SortOrder: 4},
			{ID: "keep-b", ParentID: "remove", Name: "Keep B", SortOrder: 1},
			{ID: "keep-a", ParentID: "remove", Name: "Keep A", SortOrder: 0},
		},
		Assignments: []HomeFolderAssignment{
			{Path: "/tmp/direct.docx", FolderID: "remove"},
			{Path: "/tmp/kept-b.docx", FolderID: "keep-b"},
			{Path: "/tmp/kept-a.docx", FolderID: "keep-a"},
		},
	}
	got, err := snapshot.RemoveFolder("remove")
	if err != nil {
		t.Fatal(err)
	}
	want := HomeFolderSnapshot{
		Folders: []HomeFolder{
			{ID: "root", Name: "Root", SortOrder: 0},
			{ID: "sibling", ParentID: "root", Name: "Sibling", SortOrder: 3},
			{ID: "keep-b", ParentID: "root", Name: "Keep B", SortOrder: 5},
			{ID: "keep-a", ParentID: "root", Name: "Keep A", SortOrder: 4},
		},
		Assignments: []HomeFolderAssignment{
			{Path: "/tmp/kept-b.docx", FolderID: "keep-b"},
			{Path: "/tmp/kept-a.docx", FolderID: "keep-a"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RemoveFolder() = %+v; want %+v", got, want)
	}
}

func TestHomeFolderSnapshotSaveIsTransactional(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	initial := HomeFolderSnapshot{Folders: []HomeFolder{{ID: "initial", Name: "Initial", SortOrder: 0}}}
	if err := store.SaveHomeFolderSnapshot(initial); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_home_folder_insert BEFORE INSERT ON home_folders BEGIN SELECT RAISE(ABORT, 'forced folder insert failure'); END`); err != nil {
		t.Fatal(err)
	}
	failed := HomeFolderSnapshot{Folders: []HomeFolder{{ID: "replacement", Name: "Replacement", SortOrder: 0}}}
	if err := store.SaveHomeFolderSnapshot(failed); err == nil {
		t.Fatal("SaveHomeFolderSnapshot() succeeded despite the abort trigger")
	}
	got, err := store.LoadHomeFolderSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, initial) {
		t.Fatalf("failed save changed stored snapshot:\n got: %+v\nwant: %+v", got, initial)
	}
}
