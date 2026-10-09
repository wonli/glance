package desktopui

import (
	"bytes"

	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/workspace"
)

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
