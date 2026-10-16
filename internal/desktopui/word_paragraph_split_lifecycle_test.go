package desktopui

import (
	"testing"
	"time"

	"gioui.org/io/key"
)

func TestWordParagraphSplitCompletesAfterTabGenerationChanges(t *testing.T) {
	word, state, app, router, frame := newWordJoinInteraction(t, "已有文字", "下一段")
	defer word.Close()
	runID := state.wordBlocks[0].Paragraph.Inlines[0].Text.ID
	editor := focusWordRunForInteraction(t, state, router, frame, runID)
	editor.SetCaret(editor.Len(), editor.Len())
	frame()
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	frame()
	// The user leaves and returns before the UI consumes the background result.
	app.documentGeneration++
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(3 * time.Second):
		t.Fatal("paragraph split did not finish")
	}
	app.processAsyncResults()
	if state.wordParagraphSplitIntent != nil {
		t.Fatal("completed Enter remains pending after a tab switch, blocking later Enter events")
	}
	if len(word.WordBlocks()) != 3 {
		t.Fatalf("paragraph count = %d; want 3", len(word.WordBlocks()))
	}
	frame()
	frame()
	// Returning to the document must permit another Enter without replaying the old intent.
	newRunID := state.wordBlocks[1].Paragraph.Inlines[0].Text.ID
	newEditor := focusWordRunForInteraction(t, state, router, frame, newRunID)
	router.Queue(key.EditEvent{Text: "继续输入"})
	frame()
	if newEditor.Text() != "继续输入" {
		t.Fatalf("typing after returning = %q", newEditor.Text())
	}
	newEditor.SetCaret(newEditor.Len(), newEditor.Len())
	frame()
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	frame()
	waitWordInteraction(t, app, func() bool { return state.wordParagraphSplitIntent == nil && !state.formatRunning.Load() })
	if len(word.WordBlocks()) != 4 {
		t.Fatalf("second Enter paragraph count=%d; want 4", len(word.WordBlocks()))
	}
}

func TestWordParagraphSplitKeepsOperationUntilViewAcknowledgesResult(t *testing.T) {
	word, state, app, _, _ := newWordJoinInteraction(t, "文字", "后文")
	defer word.Close()
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	wordRunEditor(state, *run)
	app.queueWordParagraphSplit(run.ID, run.Text, 2, 2)
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(3 * time.Second):
		t.Fatal("paragraph split did not finish")
	}
	if token, ok := state.beginOperation(sessionOperationMutation); ok {
		state.finishOperation(token)
		t.Fatal("another mutation can supersede Enter before its editor state is reconciled")
	}
	app.processAsyncResults()
	if state.wordParagraphSplitIntent != nil || state.formatRunning.Load() {
		t.Fatal("Enter was not acknowledged and released")
	}
}

func TestWordParagraphSplitQueuedAtWindowCloseReleasesOperation(t *testing.T) {
	word, state, app, _, _ := newWordJoinInteraction(t, "文字", "后文")
	defer word.Close()
	app.shutdown = make(chan struct{})
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	wordRunEditor(state, *run)
	app.queueWordParagraphSplit(run.ID, run.Text, 2, 2)
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(3 * time.Second):
		t.Fatal("paragraph split did not finish")
	}
	close(app.shutdown)
	app.stopAndWaitManagedBackground()
	app.closeWorkspace()
	if state.formatRunning.Load() {
		t.Fatal("window close left a queued Enter holding the session operation")
	}
}
