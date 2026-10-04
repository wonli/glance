package desktopui

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestWordVersionRestorePersistsProtectionBeforeApply(t *testing.T) {
	app, store, currentBefore, selectedID, path, _ := prepareWordRestoreTest(t)
	startWordRestoreAndWait(t, app, selectedID)
	if got, err := app.word.Snapshot(); err != nil || bytes.Equal(got, currentBefore) {
		t.Fatalf("restore did not apply selected package: changed=%v err=%v", !bytes.Equal(got, currentBefore), err)
	}
	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil {
		t.Fatal(err)
	}
	var protection *workspace.WordVersion
	for i := range versions {
		if versions[i].Source == workspace.WordVersionRestoreBefore {
			protection = &versions[i]
			break
		}
	}
	if protection == nil {
		t.Fatalf("restore-before protection missing from history: %+v", versions)
	}
	_, payload, err := store.LoadWordVersion(canonicalSessionPath(path), protection.ID)
	if err != nil || !bytes.Equal(payload, currentBefore) {
		t.Fatalf("restore-before payload matches current pre-restore package=%v err=%v", bytes.Equal(payload, currentBefore), err)
	}
}

func TestWordVersionRestoreIsSingleUndoRedoOperation(t *testing.T) {
	app, store, currentBefore, selectedID, path, _ := prepareWordRestoreTest(t)
	_, selectedPayload, err := store.LoadWordVersion(canonicalSessionPath(path), selectedID)
	if err != nil {
		t.Fatal(err)
	}
	selectedDoc, err := docx.OpenSnapshot(selectedPayload)
	if err != nil {
		t.Fatal(err)
	}
	selectedPayload, err = selectedDoc.Snapshot()
	_ = selectedDoc.Close()
	if err != nil {
		t.Fatal(err)
	}
	startWordRestoreAndWait(t, app, selectedID)
	if !app.word.Dirty() || !app.word.CanUndo() || app.word.CanRedo() {
		t.Fatalf("restored state dirty=%v undo=%v redo=%v", app.word.Dirty(), app.word.CanUndo(), app.word.CanRedo())
	}
	if applied, err := app.word.Undo(); err != nil || !applied {
		t.Fatalf("Undo() = %v, %v; want one restore operation", applied, err)
	}
	undone, err := app.word.Snapshot()
	if err != nil || !bytes.Equal(undone, currentBefore) || !app.word.Dirty() {
		t.Fatalf("undo restored pre-restore package=%v dirty=%v err=%v", bytes.Equal(undone, currentBefore), app.word.Dirty(), err)
	}
	if applied, err := app.word.Redo(); err != nil || !applied {
		t.Fatalf("Redo() = %v, %v; want one restore operation", applied, err)
	}
	redone, err := app.word.Snapshot()
	if err != nil || !bytes.Equal(redone, selectedPayload) || !app.word.Dirty() {
		t.Fatalf("redo restored selected package=%v dirty=%v err=%v", bytes.Equal(redone, selectedPayload), app.word.Dirty(), err)
	}
}

func TestWordVersionRestoreFlushesPendingTextDraftBeforeProtectionSnapshot(t *testing.T) {
	app, store, _, selectedID, path, _ := prepareWordRestoreTest(t)
	state := app.documentSessionState
	var run document.WordTextRun
	var paragraphID string
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				run = *inline.Text
				paragraphID = paragraph.ID
				break
			}
		}
		if run.ID != "" {
			break
		}
	}
	if run.ID == "" {
		t.Fatal("document has no editable text run")
	}
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = paragraphID
	editor := wordRunEditor(state, run)
	pendingText := "pending draft before restore"
	editor.SetText(pendingText)
	captureWordTextDraft(state, run.ID, pendingText)

	if !app.startRestoreWordVersion(state, selectedID) {
		t.Fatal("restore request with a pending text draft was rejected")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if !state.formatRunning.Load() && wordRunHasText(wordBlocksForState(state), "selected historical revision") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.formatRunning.Load() || !wordRunHasText(wordBlocksForState(state), "selected historical revision") {
		t.Fatal("timed out waiting for draft flush followed by history restore")
	}
	app.processAsyncResults()

	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil {
		t.Fatal(err)
	}
	var protection *workspace.WordVersion
	for i := range versions {
		if versions[i].Source == workspace.WordVersionRestoreBefore {
			protection = &versions[i]
			break
		}
	}
	if protection == nil {
		t.Fatalf("restore-before protection missing from history: %+v", versions)
	}
	_, payload, err := store.LoadWordVersion(canonicalSessionPath(path), protection.ID)
	if err != nil {
		t.Fatal(err)
	}
	protectedDoc, err := docx.OpenSnapshot(payload)
	if err != nil {
		t.Fatal(err)
	}
	protectedHasDraft := wordRunHasText(protectedDoc.WordBlocks(), pendingText)
	_ = protectedDoc.Close()
	if !protectedHasDraft {
		t.Fatal("restore-before snapshot omitted text that was pending in the editor")
	}
	if _, hasDraft := state.wordDraft(run.ID); hasDraft {
		t.Fatal("flushed text draft remained attached after restoring the historical version")
	}
	if editor.Text() != "selected historical revision" {
		t.Fatalf("focused editor after restore = %q; want restored text", editor.Text())
	}
	app.applyWordHistory(true)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if !state.formatRunning.Load() && wordRunHasText(wordBlocksForState(state), pendingText) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.formatRunning.Load() || !wordRunHasText(wordBlocksForState(state), pendingText) {
		t.Fatal("Undo after restore did not return to the pending draft content")
	}
	if editor.Text() != pendingText {
		t.Fatalf("editor after Undo = %q; want restored pre-restore draft", editor.Text())
	}
}

func TestWordVersionRestoreStopsWhenPendingTextDraftCannotCommit(t *testing.T) {
	app, store, before, selectedID, path, _ := prepareWordRestoreTest(t)
	state := app.documentSessionState
	const missingRunID = "missing-run-for-restore-test"
	captureWordTextDraft(state, missingRunID, "draft with an invalid run")
	if !app.startRestoreWordVersion(state, selectedID) {
		t.Fatal("restore request with a pending draft was rejected before attempting its commit")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if !state.formatRunning.Load() && state.pendingWordRestoreAfterDrafts() == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.formatRunning.Load() || state.pendingWordRestoreAfterDrafts() != 0 {
		t.Fatal("timed out waiting for failed draft commit to cancel the restore")
	}
	after, err := app.word.Snapshot()
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("failed draft commit changed document=%v err=%v", !bytes.Equal(after, before), err)
	}
	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if version.Source == workspace.WordVersionRestoreBefore {
			t.Fatal("failed draft commit recorded a restore-before snapshot")
		}
	}
}

func TestWordVersionRestoreDraftFailureSurvivesTabSwitch(t *testing.T) {
	app, _, _, selectedID, _, _ := prepareWordRestoreTest(t)
	original := app.documentSessionState
	captureWordTextDraft(original, "missing-run-for-tab-switch-test", "draft that cannot commit")
	if !app.startRestoreWordVersion(original, selectedID) {
		t.Fatal("restore request with a pending draft was rejected")
	}

	var result wordCommandResult
	select {
	case result = <-app.wordCommandResults:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the pending text commit result")
	}
	if result.err == nil {
		t.Fatal("missing-run draft unexpectedly committed")
	}
	runtime := app.ensureDesktopRuntime()
	runtime.attachWindow(11, app)
	otherApp := newAppWithRuntime(nil, runtime)
	runtime.attachWindow(22, otherApp)
	otherViewOfOriginal := newDocumentSessionView(original.sharedDocumentSession, otherApp.windowID)
	otherViewOfOriginal.documentID = original.documentID
	otherApp.sessions[original.documentID] = otherViewOfOriginal
	otherApp.activateSession(original.documentID, otherViewOfOriginal)

	otherWord, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer otherWord.Close()
	otherSession := newSharedDocumentSession("other.docx")
	otherSession.documentID = "other-document"
	otherSession.word = otherWord
	otherSession.wordBlocks = cloneWordBlockSnapshot(otherWord.WordBlocks())
	runtime.registerSession(otherSession)
	otherView := newDocumentSessionView(otherSession, app.windowID)
	otherView.documentID = otherSession.documentID
	app.sessions[otherSession.documentID] = otherView
	app.tabMeta[otherSession.documentID] = workspace.Tab{ID: otherSession.documentID, Kind: "document", Title: "other", DocumentID: otherSession.documentID}
	app.activateSession(otherSession.documentID, otherView)
	app.wordCommandResults <- result
	app.processAsyncResults()
	message, _ := original.wordRestoreNotice()
	if original.pendingWordRestoreAfterDrafts() != 0 || message == "" {
		t.Fatalf("inactive restore failure was not retained: pendingID=%d error=%q", original.pendingWordRestoreAfterDrafts(), message)
	}
	if strings.Contains(app.status, "恢复历史版本已取消") {
		t.Fatalf("failure was shown on the unrelated active tab: %q", app.status)
	}
	otherApp.processAsyncResults()
	if !strings.Contains(otherApp.status, "恢复历史版本已取消") {
		t.Fatalf("other window displaying the document did not receive its restore failure: %q", otherApp.status)
	}

	app.activateSession(original.documentID, original)
	if !strings.Contains(app.status, "恢复历史版本已取消") {
		t.Fatalf("returning to the document did not surface its restore failure: status=%q", app.status)
	}
}

func TestWordRestoreFailureNoticeSurvivesOriginWindowShutdown(t *testing.T) {
	origin, _, _, selectedID, _, dbPath := prepareWordRestoreTest(t)
	installRestoreBeforeFailureTrigger(t, dbPath)
	state := origin.documentSessionState
	runtime := origin.ensureDesktopRuntime()
	runtime.attachWindow(41, origin)

	survivor := newAppWithRuntime(nil, runtime)
	runtime.attachWindow(42, survivor)
	survivorView := newDocumentSessionView(state.sharedDocumentSession, survivor.windowID)
	survivorView.documentID = state.documentID
	survivor.sessions[state.documentID] = survivorView
	survivor.tabMeta[state.documentID] = workspace.Tab{
		ID: state.documentID, Kind: "document", Title: "restore", DocumentID: state.documentID,
	}
	survivor.activateSession(state.documentID, survivorView)

	if !origin.startRestoreWordVersion(state, selectedID) {
		t.Fatal("restore request was rejected")
	}
	// Simulate the initiating window closing before it consumes its private result.
	origin.shutdownOnce.Do(func() { close(origin.shutdown) })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && state.formatRunning.Load() {
		time.Sleep(time.Millisecond)
	}
	if state.formatRunning.Load() || state.wordRestoreLaunchInProgress() {
		t.Fatal("restore worker did not finish after the origin window closed")
	}

	message, epoch := state.wordRestoreNotice()
	if epoch == 0 || !strings.Contains(message, "恢复历史版本失败") {
		t.Fatalf("restore worker did not persist its failure before delivering the result: epoch=%d message=%q", epoch, message)
	}
	// Do not read or process origin.wordVersionRestoreResults: the surviving
	// window must be able to surface the shared notice on its own.
	survivor.processAsyncResults()
	if !strings.Contains(survivor.status, "恢复历史版本失败") {
		t.Fatalf("surviving window did not surface the shared restore failure: %q", survivor.status)
	}
}

func TestWordVersionRestoreAfterDraftFlushHasSingleWindowOwner(t *testing.T) {
	first, store, _, selectedID, path, _ := prepareWordRestoreTest(t)
	state := first.documentSessionState
	runtime := first.ensureDesktopRuntime()
	runtime.attachWindow(31, first)
	second := newAppWithRuntime(nil, runtime)
	runtime.attachWindow(32, second)
	secondView := newDocumentSessionView(state.sharedDocumentSession, second.windowID)
	secondView.documentID = state.documentID
	second.sessions[state.documentID] = secondView
	second.activateSession(state.documentID, secondView)

	if !state.queueWordRestoreAfterDrafts(selectedID) {
		t.Fatal("could not queue restore behind the shared text flush")
	}
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	start := make(chan struct{})
	var finish sync.WaitGroup
	finish.Add(2)
	for _, app := range []*App{first, second} {
		go func(app *App) {
			defer finish.Done()
			<-start
			app.finishWordTextFlush(state)
		}(app)
	}
	close(start)
	finish.Wait()

	var origin *App
	var result wordVersionRestoreResult
	deadline := time.After(5 * time.Second)
	for origin == nil {
		select {
		case result = <-first.wordVersionRestoreResults:
			origin = first
		case result = <-second.wordVersionRestoreResults:
			origin = second
		case <-deadline:
			t.Fatal("timed out waiting for the single restore result")
		}
	}
	if result.err != nil || !result.applied {
		t.Fatalf("claimed restore result err=%v applied=%v", result.err, result.applied)
	}
	if state.wordRestoreLaunchInProgress() {
		t.Fatal("restore claim stayed held while the result waited in its owner's private channel")
	}
	origin.wordVersionRestoreResults <- result
	origin.processAsyncResults()

	versions, err := store.ListWordVersions(canonicalSessionPath(path))
	if err != nil {
		t.Fatal(err)
	}
	protectionCount := 0
	for _, version := range versions {
		if version.Source == workspace.WordVersionRestoreBefore {
			protectionCount++
		}
	}
	if protectionCount != 1 {
		t.Fatalf("simultaneous flush completion wrote %d restore-before versions; want exactly one", protectionCount)
	}
}

func TestWordRestoreClaimReleasesWhenOriginWindowStopsBeforeResultHandling(t *testing.T) {
	first, _, _, selectedID, _, _ := prepareWordRestoreTest(t)
	state := first.documentSessionState
	runtime := first.ensureDesktopRuntime()
	runtime.attachWindow(41, first)
	second := newAppWithRuntime(nil, runtime)
	runtime.attachWindow(42, second)
	secondView := newDocumentSessionView(state.sharedDocumentSession, second.windowID)
	secondView.documentID = state.documentID
	second.sessions[state.documentID] = secondView
	second.activateSession(state.documentID, secondView)

	if !state.queueWordRestoreAfterDrafts(selectedID) {
		t.Fatal("could not queue restore")
	}
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	first.shutdownOnce.Do(func() { close(first.shutdown) })
	first.finishWordTextFlush(state)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (state.formatRunning.Load() || state.wordRestoreLaunchInProgress()) {
		time.Sleep(time.Millisecond)
	}
	if state.formatRunning.Load() || state.wordRestoreLaunchInProgress() {
		t.Fatal("origin shutdown left the restore operation or claim active")
	}

	if !second.startRestoreWordVersion(state, selectedID) {
		t.Fatal("surviving window could not start a restore after the origin stopped")
	}
	startWordRestoreAndWait(t, second, selectedID)
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

func TestWordVersionRestoreRejectsCorruptPayload(t *testing.T) {
	for _, corruptMode := range []string{"hash", "docx"} {
		t.Run(corruptMode, func(t *testing.T) {
			app, store, _, selectedID, path, dbPath := prepareWordRestoreTest(t)
			before, err := app.word.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if corruptMode == "hash" {
				updateWordVersionPayloadForTest(t, dbPath, selectedID, []byte("corrupt bytes"), "")
			} else {
				corruptDOCX := malformedWordPackageForTest(t)
				digest := sha256.Sum256(corruptDOCX)
				updateWordVersionPayloadForTest(t, dbPath, selectedID, corruptDOCX, hex.EncodeToString(digest[:]))
			}
			startWordRestoreAndWait(t, app, selectedID)
			after, err := app.word.Snapshot()
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("%s failure changed current package=%v err=%v", corruptMode, !bytes.Equal(after, before), err)
			}
			versions, err := store.ListWordVersions(canonicalSessionPath(path))
			if err != nil {
				t.Fatal(err)
			}
			for _, version := range versions {
				if version.Source == workspace.WordVersionRestoreBefore {
					t.Fatal("corrupt selected version was applied far enough to add a restore-before record")
				}
			}
		})
	}
}

func TestWordVersionRestoreProtectionWriteFailureRejects(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	installRestoreBeforeFailureTrigger(t, dbPath)
	path := filepath.Join(dir, "protected.docx")
	writeWordFile(t, path, "opened")
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	replaceWordTextForHistoryTest(t, app.word, "current unique revision")
	before, err := app.word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	selectedPath := filepath.Join(dir, "selected.docx")
	writeWordFile(t, selectedPath, "selected")
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
	startWordRestoreAndWait(t, app, selected.ID)
	if got, err := app.word.Snapshot(); err != nil || !bytes.Equal(got, before) {
		t.Fatalf("protection write failure applied target: unchanged=%v err=%v", bytes.Equal(got, before), err)
	}
	if !strings.Contains(app.status, "恢复历史版本失败") {
		t.Fatalf("protection write failure status=%q", app.status)
	}
}

func TestWordVersionRestoreBeforeSurvivesPruning(t *testing.T) {
	app, store, _, selectedID, path, _ := prepareWordRestoreTest(t)
	pathKey := canonicalSessionPath(path)
	for i := 0; i < 50; i++ {
		if _, _, err := store.AddWordVersion(workspace.WordVersionInput{
			PathKey: pathKey, DisplayPath: path, Source: workspace.WordVersionManualSave,
			CapturedAt: time.Unix(int64(i+1), 0).UTC(), Snapshot: []byte(fmt.Sprintf("retention-%02d", i)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := store.LoadWordVersion(pathKey, selectedID); err != nil {
		t.Fatalf("selected version was pruned before restore: %v", err)
	}
	startWordRestoreAndWait(t, app, selectedID)
	versions, err := store.ListWordVersions(pathKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if version.Source == workspace.WordVersionRestoreBefore {
			return
		}
	}
	t.Fatalf("restore-before was pruned from the %d retained versions", len(versions))
}

func TestWordVersionHistoryActionEnabledOnlyForSavedDOCX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.docx")
	writeWordFile(t, path, "saved document")
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	action := ribbonAction{id: "version-history"}
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("version history should be enabled for a saved DOCX")
	}
	app.documentSessionState.path = ""
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("version history should be disabled for an unsaved DOCX")
	}
}

func TestWordVersionHistorySemanticDescriptionRequiresSave(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.installDocument(loadedDocument{word: func() *docx.Document {
		doc, err := docx.NewBlank()
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}()}); err != nil {
		t.Fatal(err)
	}
	description := app.wordToolbarActionDescription(ribbonAction{id: "version-history", label: "版本历史"})
	if !strings.Contains(description, "请先保存文档") {
		t.Fatalf("unsaved history action description = %q; want save guidance", description)
	}
}

func TestWordVersionHistoryListShowsMetadataAndOrder(t *testing.T) {
	store := openWordHistoryTestStore(t)
	pathKey := filepath.Join(t.TempDir(), "history.docx")
	firstAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	for _, item := range []struct {
		at      time.Time
		source  workspace.WordVersionSource
		payload []byte
	}{
		{at: firstAt, source: workspace.WordVersionOpened, payload: []byte("baseline bytes")},
		{at: secondAt, source: workspace.WordVersionManualSave, payload: []byte("manual bytes, larger")},
	} {
		if _, _, err := store.AddWordVersion(workspace.WordVersionInput{
			PathKey: pathKey, DisplayPath: pathKey, Source: item.source,
			CapturedAt: item.at, Snapshot: item.payload,
		}); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := store.ListWordVersions(pathKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || !versions[0].CreatedAt.After(versions[1].CreatedAt) {
		t.Fatalf("versions not newest-first: %+v", versions)
	}
	labels := []string{wordVersionHistoryRowText(versions[0]), wordVersionHistoryRowText(versions[1])}
	if !strings.Contains(labels[0], "手动保存") || !strings.Contains(labels[0], "2026") || !strings.Contains(labels[0], "字节") {
		t.Fatalf("newest row omits source/time/size: %q", labels[0])
	}
	if !strings.Contains(labels[1], "打开文档") {
		t.Fatalf("older row has unexpected metadata: %q", labels[1])
	}
}

func TestWordVersionHistoryLoadFailureIsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-error.docx")
	writeWordFile(t, path, "document")
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	app.openWordVersionHistory()
	select {
	case result := <-app.wordVersionHistoryResults:
		app.wordVersionHistoryResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for history list failure")
	}
	if !app.wordVersionHistoryVisible || app.wordVersionHistoryLoading || app.wordVersionHistoryError == "" {
		t.Fatalf("history failure is not visible: visible=%v loading=%v error=%q", app.wordVersionHistoryVisible, app.wordVersionHistoryLoading, app.wordVersionHistoryError)
	}
}

func TestWordVersionRestoreRunsAsyncAndLocksConflictingOperations(t *testing.T) {
	app, _, before, selectedID, _, _ := prepareWordRestoreTest(t)
	runtime := app.desktopRuntime
	runtime.workspaceDBMu.Lock()
	app.startRestoreWordVersion(app.documentSessionState, selectedID)
	if !app.documentSessionState.formatRunning.Load() {
		runtime.workspaceDBMu.Unlock()
		t.Fatal("restore should mark the shared document busy before background I/O")
	}
	if app.wordToolbarActionEnabled(ribbonAction{id: "save"}) {
		runtime.workspaceDBMu.Unlock()
		t.Fatal("Save should be disabled while restore is active")
	}
	app.runWordCommand("blocked edit", func(editor document.WordTextEditor) error {
		return editor.ReplaceTextRun(app.documentSessionState.selectedWordRunID, "must not apply")
	})
	during, err := app.word.Snapshot()
	if err != nil || !bytes.Equal(during, before) {
		runtime.workspaceDBMu.Unlock()
		t.Fatalf("document changed while restore was blocked in background I/O: unchanged=%v err=%v", bytes.Equal(during, before), err)
	}
	runtime.workspaceDBMu.Unlock()
	select {
	case result := <-app.wordVersionRestoreResults:
		app.wordVersionRestoreResults <- result
		app.processAsyncResults()
		if result.err != nil || !result.applied {
			t.Fatalf("async restore result = %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async restore")
	}
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
