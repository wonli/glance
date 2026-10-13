package desktopui

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"glance/internal/workspace"
)

func TestHomeFolderRuntimeSharesSnapshotAcrossWindows(t *testing.T) {
	runtime, _ := newHomeFolderRuntimeForTest(t)
	first := &App{desktopRuntime: runtime, homeFolderWriteResults: make(chan homeFolderWriteResult, 1)}
	second := &App{desktopRuntime: runtime, homeFolderWriteResults: make(chan homeFolderWriteResult, 1)}

	snapshot, _ := first.homeFolderSnapshot()
	next, err := snapshot.AddFolder(workspace.HomeFolder{ID: "shared", Name: "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.updateHomeFolderSnapshot(next, first.homeFolderWriteResults); err != nil {
		t.Fatal(err)
	}
	if result := awaitHomeFolderWrite(t, first.homeFolderWriteResults); result.err != nil {
		t.Fatal(result.err)
	}

	firstSnapshot, firstRevision := first.homeFolderSnapshot()
	secondSnapshot, secondRevision := second.homeFolderSnapshot()
	if firstRevision != secondRevision || !reflect.DeepEqual(firstSnapshot, secondSnapshot) {
		t.Fatalf("window snapshots differ: first=%+v@%d second=%+v@%d", firstSnapshot, firstRevision, secondSnapshot, secondRevision)
	}
	firstSnapshot.Folders[0].Name = "Mutated copy"
	secondSnapshot, _ = second.homeFolderSnapshot()
	if secondSnapshot.Folders[0].Name != "Shared" {
		t.Fatalf("homeFolderSnapshot() exposed mutable runtime state: %+v", secondSnapshot)
	}
}

func TestHomeFolderRuntimeConcurrentMutationsRebaseAcrossWindows(t *testing.T) {
	runtime, _ := newHomeFolderRuntimeForTest(t)
	firstResults := make(chan homeFolderWriteResult, 1)
	secondResults := make(chan homeFolderWriteResult, 1)
	start := make(chan struct{})
	mutation := func(id, name string) func(workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		return func(snapshot workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
			next, err := snapshot.AddFolder(workspace.HomeFolder{ID: id, Name: name})
			return next, err == nil, err
		}
	}
	results := make(chan error, 2)
	go func() {
		<-start
		_, _, _, err := runtime.mutateHomeFolderSnapshot(mutation("from-first", "第一窗口"), firstResults)
		results <- err
	}()
	go func() {
		<-start
		_, _, _, err := runtime.mutateHomeFolderSnapshot(mutation("from-second", "第二窗口"), secondResults)
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if result := awaitHomeFolderWrite(t, firstResults); result.err != nil {
		t.Fatal(result.err)
	}
	if result := awaitHomeFolderWrite(t, secondResults); result.err != nil {
		t.Fatal(result.err)
	}
	snapshot, _ := runtime.homeFolderSnapshot()
	if len(snapshot.Folders) != 2 {
		t.Fatalf("concurrent mutations left %d folders, want both windows' changes: %+v", len(snapshot.Folders), snapshot.Folders)
	}
	if !hasHomeFolder(snapshot, "from-first") {
		t.Fatalf("first window's folder was lost: %+v", snapshot.Folders)
	}
	if !hasHomeFolder(snapshot, "from-second") {
		t.Fatalf("second window's folder was lost: %+v", snapshot.Folders)
	}
}

func TestHomeFolderWriterCommitsSnapshotAsynchronously(t *testing.T) {
	runtime, _ := newHomeFolderRuntimeForTest(t)
	resultChannel := make(chan homeFolderWriteResult, 1)
	snapshot := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "queued", Name: "Queued", SortOrder: 0}}}
	runtime.workspaceDBMu.Lock()
	updateDone := make(chan error, 1)
	go func() {
		_, err := runtime.updateHomeFolderSnapshot(snapshot, resultChannel)
		updateDone <- err
	}()
	select {
	case err := <-updateDone:
		runtime.workspaceDBMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		runtime.workspaceDBMu.Unlock()
		t.Fatal("updateHomeFolderSnapshot blocked on workspace database I/O")
	}

	result := awaitHomeFolderWrite(t, resultChannel)
	if result.err != nil {
		t.Fatal(result.err)
	}
	got, err := runtime.workspaceStore.LoadHomeFolderSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("persisted folder snapshot=%+v; want %+v", got, snapshot)
	}
}

func TestHomeFolderWriteFailureRollsBackOnlyLatestRevision(t *testing.T) {
	runtime, storePath := newHomeFolderRuntimeForTest(t)
	committed := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "committed", Name: "Committed", SortOrder: 0}}}
	firstResult := make(chan homeFolderWriteResult, 1)
	if _, err := runtime.updateHomeFolderSnapshot(committed, firstResult); err != nil {
		t.Fatal(err)
	}
	if result := awaitHomeFolderWrite(t, firstResult); result.err != nil {
		t.Fatal(result.err)
	}
	triggerDB, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer triggerDB.Close()
	if _, err := triggerDB.Exec(`CREATE TRIGGER fail_home_folder_test_insert BEFORE INSERT ON home_folders WHEN NEW.name LIKE 'Fail%' BEGIN SELECT RAISE(ABORT, 'forced folder insert failure'); END`); err != nil {
		t.Fatal(err)
	}

	runtime.workspaceDBMu.Lock()
	olderFailure := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "older-failure", Name: "Fail older", SortOrder: 0}}}
	newerSuccess := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "newer-success", Name: "Newest", SortOrder: 0}}}
	olderResult := make(chan homeFolderWriteResult, 1)
	newerResult := make(chan homeFolderWriteResult, 1)
	olderRevision, err := runtime.updateHomeFolderSnapshot(olderFailure, olderResult)
	if err != nil {
		runtime.workspaceDBMu.Unlock()
		t.Fatal(err)
	}
	newerRevision, err := runtime.updateHomeFolderSnapshot(newerSuccess, newerResult)
	if err != nil {
		runtime.workspaceDBMu.Unlock()
		t.Fatal(err)
	}
	runtime.workspaceDBMu.Unlock()
	if olderRevision >= newerRevision {
		t.Fatalf("revisions are not increasing: older=%d newer=%d", olderRevision, newerRevision)
	}
	if result := awaitHomeFolderWrite(t, olderResult); result.err == nil {
		t.Fatal("older folder write unexpectedly succeeded")
	}
	if result := awaitHomeFolderWrite(t, newerResult); result.err != nil {
		t.Fatalf("newer folder write failed: %v", result.err)
	}
	got, revision := runtime.homeFolderSnapshot()
	if revision != newerRevision || !reflect.DeepEqual(got, newerSuccess) {
		t.Fatalf("older failure rolled back newer snapshot: got=%+v@%d want=%+v@%d", got, revision, newerSuccess, newerRevision)
	}

	latestFailure := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "latest-failure", Name: "Fail latest", SortOrder: 0}}}
	latestResult := make(chan homeFolderWriteResult, 1)
	latestRevision, err := runtime.updateHomeFolderSnapshot(latestFailure, latestResult)
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitHomeFolderWrite(t, latestResult); result.err == nil {
		t.Fatal("latest folder write unexpectedly succeeded")
	}
	got, revision = runtime.homeFolderSnapshot()
	if revision != latestRevision || !reflect.DeepEqual(got, newerSuccess) {
		t.Fatalf("latest failure did not roll back to the last committed snapshot: got=%+v@%d want=%+v@%d", got, revision, newerSuccess, latestRevision)
	}
	stored, err := runtime.workspaceStore.LoadHomeFolderSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, newerSuccess) {
		t.Fatalf("latest failed write changed stored snapshot: %+v", stored)
	}
}

func TestHomeFolderLoadFailureBlocksOverwrite(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	t.Cleanup(runtime.shutdown)
	corruptDB, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := corruptDB.Exec(`INSERT INTO home_folders(id, parent_id, name, sort_order, source_path) VALUES ('a', NULL, 'A', 0, NULL), ('b', NULL, 'B', 1, NULL)`); err != nil {
		_ = corruptDB.Close()
		t.Fatal(err)
	}
	if _, err := corruptDB.Exec(`UPDATE home_folders SET parent_id='b' WHERE id='a'`); err != nil {
		_ = corruptDB.Close()
		t.Fatal(err)
	}
	if _, err := corruptDB.Exec(`UPDATE home_folders SET parent_id='a' WHERE id='b'`); err != nil {
		_ = corruptDB.Close()
		t.Fatal(err)
	}
	if err := corruptDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.loadHomeFolderState(); err == nil {
		t.Fatal("loadHomeFolderState() accepted the corrupt persisted tree")
	}
	if runtime.homeFolderStateError() == nil {
		t.Fatal("homeFolderStateError() did not retain the load error")
	}
	if _, err := runtime.updateHomeFolderSnapshot(workspace.HomeFolderSnapshot{}, make(chan homeFolderWriteResult, 1)); err == nil {
		t.Fatal("updateHomeFolderSnapshot() allowed replacing an unreadable persisted tree")
	}
	verifyDB, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer verifyDB.Close()
	var count int
	if err := verifyDB.QueryRow(`SELECT COUNT(*) FROM home_folders`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("corrupt persisted rows changed after failed load: count=%d; want 2", count)
	}
}

func newHomeFolderRuntimeForTest(t *testing.T) (*desktopRuntime, string) {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	if err := runtime.loadHomeFolderState(); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtime.startWorkspaceWriter()
	t.Cleanup(runtime.shutdown)
	return runtime, storePath
}

func awaitHomeFolderWrite(t *testing.T, results <-chan homeFolderWriteResult) homeFolderWriteResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for home folder writer")
		return homeFolderWriteResult{}
	}
}
