package desktopui

import "testing"

func TestSessionOperationGateSerializesSaveAndMutation(t *testing.T) {
	session := newSharedDocumentSession("")
	session.documentID = "draft-test"

	save, ok := session.beginOperation(sessionOperationSave)
	if !ok {
		t.Fatal("first save operation was rejected")
	}
	if _, ok := session.beginOperation(sessionOperationMutation); ok {
		t.Fatal("mutation overlapped an active save")
	}
	if !session.finishOperation(save) {
		t.Fatal("current save operation did not finish")
	}
	if session.finishOperation(save) {
		t.Fatal("finished operation token was accepted twice")
	}

	mutation, ok := session.beginOperation(sessionOperationMutation)
	if !ok {
		t.Fatal("mutation was not admitted after save completed")
	}
	if mutation.generation <= save.generation {
		t.Fatalf("operation generation did not advance: save=%d mutation=%d", save.generation, mutation.generation)
	}
	if !session.finishOperation(mutation) {
		t.Fatal("current mutation operation did not finish")
	}
}

func TestSessionPublicationInvalidatesEveryAttachedView(t *testing.T) {
	runtime := newDesktopRuntime()
	shared := newSharedDocumentSession("/tmp/shared-publication.docx")
	shared.documentID = "doc-publication"
	runtime.registerSession(shared)

	firstState := newDocumentSessionView(shared, 1)
	secondState := newDocumentSessionView(shared, 2)
	first := &App{desktopRuntime: runtime, documentSessionState: firstState}
	second := &App{desktopRuntime: runtime, documentSessionState: secondState}
	runtime.attachWindow(1, first)
	runtime.attachWindow(2, second)

	initial := shared.wordRevisionAtomic.Load()
	runtime.publishSessionRevision(shared.documentID, initial+1)
	if shared.wordRevisionAtomic.Load() != initial+1 {
		t.Fatalf("published revision = %d; want %d", shared.wordRevisionAtomic.Load(), initial+1)
	}
	for index, state := range []*documentSessionState{firstState, secondState} {
		if got := state.observedSessionRevision.Load(); got != initial+1 {
			t.Errorf("window %d observed revision = %d; want %d", index+1, got, initial+1)
		}
	}

	runtime.publishSessionRevision(shared.documentID, initial+1)
	if shared.wordRevisionAtomic.Load() != initial+1 {
		t.Fatalf("duplicate publication advanced revision to %d", shared.wordRevisionAtomic.Load())
	}
}
