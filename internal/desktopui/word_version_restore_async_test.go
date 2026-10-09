package desktopui

import (
	"bytes"

	"testing"
	"time"

	"glance/internal/document"
)

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
