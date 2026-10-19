package desktopui

import (
	"bytes"
	"crypto/sha256"

	"encoding/hex"
	"fmt"

	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"glance/internal/format/docx"
	"glance/internal/workspace"
)

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
	if original.pendingWordRestoreAfterDrafts() != 0 || message != "word.history.error.restore_cancelled" {
		t.Fatalf("inactive restore failure was not retained: pendingID=%d error=%q", original.pendingWordRestoreAfterDrafts(), message)
	}
	if app.statusRef.ID == "word.history.error.restore_cancelled" {
		t.Fatalf("failure was shown on the unrelated active tab: %q", app.statusText())
	}
	otherApp.processAsyncResults()
	if otherApp.statusRef.ID != "word.history.error.restore_cancelled" {
		t.Fatalf("other window displaying the document did not receive its restore failure: %q", otherApp.statusText())
	}

	app.activateSession(original.documentID, original)
	if app.statusRef.ID != "word.history.error.restore_cancelled" {
		t.Fatalf("returning to the document did not surface its restore failure: status=%q", app.statusText())
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
	if epoch == 0 || message != "word.history.error.restore" {
		t.Fatalf("restore worker did not persist its failure before delivering the result: epoch=%d message=%q", epoch, message)
	}
	// Do not read or process origin.wordVersionRestoreResults: the surviving
	// window must be able to surface the shared notice on its own.
	survivor.processAsyncResults()
	if survivor.statusRef.ID != "word.history.error.restore" {
		t.Fatalf("surviving window did not surface the shared restore failure: %q", survivor.statusText())
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
	if app.statusRef.ID != "word.history.error.restore" {
		t.Fatalf("protection write failure status=%q", app.statusText())
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
