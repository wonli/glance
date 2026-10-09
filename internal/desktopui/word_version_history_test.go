package desktopui

import (
	"archive/zip"
	"bytes"

	"database/sql"

	"io"
	"os"
	"path/filepath"

	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/workspace"
)

func TestWordVersionOpenAddsAndDeduplicatesBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.docx")
	writeWordFile(t, path, "first disk version")
	store := openWordHistoryTestStore(t)
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store

	first := newAppWithRuntime(nil, runtime)
	if err := first.open(path); err != nil {
		t.Fatal(err)
	}
	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil || len(versions) != 1 || versions[0].Source != workspace.WordVersionOpened {
		t.Fatalf("first open history = %+v, %v; want one opened baseline", versions, err)
	}
	replaceWordTextForHistoryTest(t, first.word, "unsaved in-memory edit")
	if err := first.open(path); err != nil {
		t.Fatal(err)
	}
	versions, err = store.ListWordVersions(canonicalSessionPath(path))
	if err != nil || len(versions) != 1 {
		t.Fatalf("reopening identical package history = %+v, %v; want deduplicated baseline", versions, err)
	}

	writeWordFile(t, path, "second disk version")
	second := newAppWithRuntime(nil, newDesktopRuntime())
	second.desktopRuntime.workspaceStore = store
	if err := second.open(path); err != nil {
		t.Fatal(err)
	}
	versions, err = store.ListWordVersions(canonicalSessionPath(path))
	if err != nil || len(versions) != 2 || versions[0].Source != workspace.WordVersionOpened {
		t.Fatalf("changed disk package history = %+v, %v; want two opened baselines", versions, err)
	}
}

func TestWordVersionOpenDatabaseFailureDoesNotBlockOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-history.docx")
	writeWordFile(t, path, "still open")
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.open(path); err != nil {
		t.Fatalf("open failed because version storage is unavailable: %v", err)
	}
	if app.word == nil || app.path != path {
		t.Fatalf("document did not open: word=%v path=%q", app.word, app.path)
	}
	if !bytes.Contains([]byte(app.status), []byte("历史版本暂不可用")) {
		t.Fatalf("unavailable history was not reported without blocking open: status=%q", app.status)
	}
}

func TestWordVersionSaveFailureKeepsDirtyWithoutHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.docx")
	writeWordFile(t, path, "source")
	store := openWordHistoryTestStore(t)
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	replaceWordTextForHistoryTest(t, app.word, "unsaved edit")
	failedPath := filepath.Join(t.TempDir(), "missing", "failed.docx")
	app.word.SetSourcePath(failedPath)
	app.documentSessionState.path = failedPath
	app.path = failedPath
	app.startSave()
	result := waitWordSaveResult(t, app)
	if result.err == nil || result.historyErr != nil || !app.word.Dirty() {
		t.Fatalf("failed save result err=%v historyErr=%v dirty=%v", result.err, result.historyErr, app.word.Dirty())
	}
	versions, err := store.ListWordVersions(canonicalSessionPath(failedPath))
	if err != nil || len(versions) != 0 {
		t.Fatalf("failed source write recorded versions: %+v, %v", versions, err)
	}
}

func TestWordVersionHistoryFailureKeepsSuccessfulDiskSaveClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-unavailable.docx")
	writeWordFile(t, path, "source")
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	replaceWordTextForHistoryTest(t, app.word, "saved without history")
	app.startSave()
	result := waitWordSaveResult(t, app)
	if result.err != nil || result.historyErr == nil || app.word.Dirty() {
		t.Fatalf("successful disk save result err=%v historyErr=%v dirty=%v", result.err, result.historyErr, app.word.Dirty())
	}
	if app.word.Dirty() || !bytes.Contains([]byte(app.status), []byte("文件已保存，但历史版本未记录")) {
		t.Fatalf("history failure UI state dirty=%v status=%q", app.word.Dirty(), app.status)
	}
}

func TestWordVersionHistoryFailureAndLaterEditStatusIncludesBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "later-edit.docx")
	writeWordFile(t, path, "opened")
	store := openWordHistoryTestStore(t)
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	replaceWordTextForHistoryTest(t, app.word, "written before later edit")
	written, err := app.word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runtime.workspaceDBMu.Lock()
	app.startSave()
	waitForFileBytes(t, path, written)
	revision := app.documentSessionState.wordRevisionAtomic.Load()
	replaceWordTextForHistoryTest(t, app.word, "later in-memory edit")
	runtime.publishSessionRevision(app.documentSessionState.documentID, revision+1)
	runtime.workspaceStore = nil
	runtime.workspaceDBMu.Unlock()
	result := waitWordSaveResult(t, app)
	if result.err != nil || result.historyErr == nil || !app.word.Dirty() {
		t.Fatalf("save result err=%v historyErr=%v dirty=%v", result.err, result.historyErr, app.word.Dirty())
	}
	if !bytes.Contains([]byte(app.status), []byte("文件已保存，但历史版本未记录")) || !bytes.Contains([]byte(app.status), []byte("保存后文档又有修改")) {
		t.Fatalf("status does not report both conditions: %q", app.status)
	}
}

func TestWordVersionSaveCapturesWrittenRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "captured-revision.docx")
	writeWordFile(t, path, "opened")
	store := openWordHistoryTestStore(t)
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	replaceWordTextForHistoryTest(t, app.word, "written revision")
	written, err := app.word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runtime.workspaceDBMu.Lock()
	app.startSave()
	waitForFileBytes(t, path, written)
	replaceWordTextForHistoryTest(t, app.word, "later in-memory edit")
	later, err := app.word.Snapshot()
	if err != nil {
		runtime.workspaceDBMu.Unlock()
		t.Fatal(err)
	}
	runtime.workspaceDBMu.Unlock()
	result := waitWordSaveResult(t, app)
	if result.err != nil || result.historyErr != nil {
		t.Fatalf("save result err=%v historyErr=%v", result.err, result.historyErr)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil {
		t.Fatal(err)
	}
	var saveVersion *workspace.WordVersion
	for i := range versions {
		if versions[i].Source == workspace.WordVersionManualSave {
			saveVersion = &versions[i]
			break
		}
	}
	if saveVersion == nil {
		t.Fatalf("manual save version missing from %+v", versions)
	}
	_, payload, err := store.LoadWordVersion(canonicalSessionPath(path), saveVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, disk) || !bytes.Equal(payload, written) || bytes.Equal(payload, later) {
		t.Fatalf("history captured the wrong revision: disk=%v written=%v later=%v", bytes.Equal(payload, disk), bytes.Equal(payload, written), bytes.Equal(payload, later))
	}
}

func TestWordVersionSaveAsUsesTargetPathTimeline(t *testing.T) {
	for _, seedTarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "new target", true: "existing target"}[seedTarget], func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.docx")
			target := filepath.Join(dir, "target.docx")
			writeWordFile(t, source, "source document")
			store := openWordHistoryTestStore(t)
			runtime := newDesktopRuntime()
			runtime.workspaceStore = store
			app := newAppWithRuntime(nil, runtime)
			if err := app.open(source); err != nil {
				t.Fatal(err)
			}
			if seedTarget {
				writeWordFile(t, target, "older target revision")
				payload, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := store.AddWordVersion(workspace.WordVersionInput{
					PathKey: canonicalSessionPath(target), DisplayPath: target,
					Source: workspace.WordVersionOpened, CapturedAt: time.Now().UTC(), Snapshot: payload,
				}); err != nil {
					t.Fatal(err)
				}
			}
			app.startSaveAs(target)
			result := waitWordSaveResult(t, app)
			if result.err != nil || result.historyErr != nil {
				t.Fatalf("Save As result err=%v historyErr=%v", result.err, result.historyErr)
			}
			targetVersions, err := store.ListWordVersions(canonicalSessionPath(target))
			if err != nil {
				t.Fatal(err)
			}
			wantTargetCount := 1
			if seedTarget {
				wantTargetCount = 2
			}
			if len(targetVersions) != wantTargetCount || targetVersions[0].Source != workspace.WordVersionManualSave {
				t.Fatalf("target timeline = %+v; want %d versions headed by Save As", targetVersions, wantTargetCount)
			}
			sourceVersions, err := store.ListWordVersions(canonicalSessionPath(source))
			if err != nil || len(sourceVersions) != 1 || sourceVersions[0].Source != workspace.WordVersionOpened {
				t.Fatalf("Save As changed source timeline: %+v, %v", sourceVersions, err)
			}
		})
	}
}

func TestWordVersionAutoSaveUsesThrottledSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autosave-history.docx")
	writeWordFile(t, path, "opened")
	store := openWordHistoryTestStore(t)
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}

	replaceWordTextForHistoryTest(t, app.word, "first autosave")
	app.startSaveForReason(wordSaveAuto)
	if result := waitWordSaveResult(t, app); result.err != nil || result.historyErr != nil {
		t.Fatalf("first auto save: err=%v historyErr=%v", result.err, result.historyErr)
	}
	replaceWordTextForHistoryTest(t, app.word, "throttled autosave")
	app.startSaveForReason(wordSaveAuto)
	if result := waitWordSaveResult(t, app); result.err != nil || result.historyErr != nil {
		t.Fatalf("throttled auto save: err=%v historyErr=%v", result.err, result.historyErr)
	}
	replaceWordTextForHistoryTest(t, app.word, "manual save after throttle")
	app.startSave()
	if result := waitWordSaveResult(t, app); result.err != nil || result.historyErr != nil {
		t.Fatalf("manual save after auto throttle: err=%v historyErr=%v", result.err, result.historyErr)
	}
	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 || versions[0].Source != workspace.WordVersionManualSave || versions[1].Source != workspace.WordVersionAutoSave || versions[2].Source != workspace.WordVersionOpened {
		t.Fatalf("save source/throttle timeline = %+v", versions)
	}
}

func wordRunHasText(blocks []document.WordBlock, want string) bool {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Text == want {
				return true
			}
		}
	}
	return false
}

func openWordHistoryTestStore(t *testing.T) *workspace.Store {
	t.Helper()
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func prepareWordRestoreTest(t *testing.T) (*App, *workspace.Store, []byte, int64, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "restore.docx")
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	writeWordFile(t, path, "opened baseline")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	replaceWordTextForHistoryTest(t, app.word, "current unique revision")
	currentBefore, err := app.word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	selectedPath := filepath.Join(t.TempDir(), "selected.docx")
	writeWordFile(t, selectedPath, "selected historical revision")
	selectedBytes, err := os.ReadFile(selectedPath)
	if err != nil {
		t.Fatal(err)
	}
	selected, added, err := store.AddWordVersion(workspace.WordVersionInput{
		PathKey: canonicalSessionPath(path), DisplayPath: path,
		Source: workspace.WordVersionManualSave, CapturedAt: time.Now().UTC(), Snapshot: selectedBytes,
	})
	if err != nil || !added {
		t.Fatalf("add selected version = %+v, %v, %v", selected, added, err)
	}
	return app, store, currentBefore, selected.ID, path, dbPath
}

func startWordRestoreAndWait(t *testing.T, app *App, versionID int64) {
	t.Helper()
	state := app.documentSessionState
	app.startRestoreWordVersion(state, versionID)
	select {
	case result := <-app.wordVersionRestoreResults:
		app.wordVersionRestoreResults <- result
		app.processAsyncResults()
		if result.err == nil && !result.applied {
			t.Fatal("restore result reports success without applying a snapshot")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Word history restore")
	}
}

func updateWordVersionPayloadForTest(t *testing.T, dbPath string, id int64, payload []byte, hash string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if hash == "" {
		if _, err := db.Exec(`UPDATE word_versions SET payload = ? WHERE id = ?`, payload, id); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := db.Exec(`UPDATE word_versions SET payload = ?, sha256 = ? WHERE id = ?`, payload, hash, id); err != nil {
		t.Fatal(err)
	}
}

func installRestoreBeforeFailureTrigger(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_restore_before
		BEFORE INSERT ON word_versions WHEN NEW.source = 'restore-before'
		BEGIN SELECT RAISE(ABORT, 'restore protection write rejected'); END`); err != nil {
		t.Fatal(err)
	}
}

func malformedWordPackageForTest(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	partWriter, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(partWriter, `<w:document><w:body><w:p>`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func writeWordFile(t *testing.T, path, text string) {
	t.Helper()
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if len(doc.WordBlocks()) > 0 && doc.WordBlocks()[0].Paragraph != nil && len(doc.WordBlocks()[0].Paragraph.Inlines) > 0 {
		if run := doc.WordBlocks()[0].Paragraph.Inlines[0].Text; run != nil {
			if err := doc.ReplaceTextRun(run.ID, text); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := doc.SaveAsDocument(path); err != nil {
		t.Fatal(err)
	}
}

func replaceWordTextForHistoryTest(t *testing.T, doc *docx.Document, value string) {
	t.Helper()
	for _, block := range doc.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil {
				if err := doc.ReplaceTextRun(inline.Text.ID, value); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	t.Fatal("DOCX test fixture has no text run")
}

func waitWordSaveResult(t *testing.T, app *App) saveResult {
	t.Helper()
	select {
	case result := <-app.saveResults:
		app.saveResults <- result
		app.processAsyncResults()
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Word save")
		return saveResult{}
	}
}

func waitForFileBytes(t *testing.T, path string, expected []byte) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && bytes.Equal(data, expected) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("file %q never contained the expected saved revision", path)
}
