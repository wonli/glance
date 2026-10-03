package desktopui

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPrimaryWindowPromotionAndFinalShutdown(t *testing.T) {
	fixture := newWindowViewFixture(t)
	first, second := fixture.apps[0], fixture.apps[1]
	first.activateSession(fixture.ids[0], fixture.states[0])

	secondView := newDocumentViewFromSnapshot(fixture.shared[1], second.windowID, windowViewSnapshot{})
	secondView.documentID = fixture.ids[1]
	second.sessions[fixture.ids[1]] = secondView
	second.activateSession(fixture.ids[1], secondView)
	run := firstEditableRunForWindowTest(t, secondView)
	secondView.wordViewMode = wordViewWebLayout
	secondView.wordLayoutZoom = 1.35
	secondView.wordNavigationVisible = true
	secondView.wordNavigationSearchQuery = "独立窗口视图"
	secondView.wordNavigationSearchEditor.SetText(secondView.wordNavigationSearchQuery)
	secondView.wordToolbarTab = wordTabView
	secondView.selectedWordRunID = run.ID
	secondView.selectedWordParagraphID = allWordParagraphs(wordBlocksForState(secondView))[0].ID
	secondView.wordPageListState.Position.First = 3
	wordRunEditor(secondView, run).SetCaret(0, 0)

	runtime := fixture.runtime
	runtime.completion = make(chan struct{})
	if runtime.primaryWindowID != first.windowID {
		t.Fatalf("initial primary window = %d, want %d", runtime.primaryWindowID, first.windowID)
	}
	first.closeWorkspace()
	if runtime.primaryWindowID != second.windowID {
		t.Fatalf("primary window after detach = %d, want promoted window %d", runtime.primaryWindowID, second.windowID)
	}
	if !first.documentSessionState.closed.Load() || secondView.closed.Load() {
		t.Fatalf("detach closed flags = first %v, survivor %v", first.documentSessionState.closed.Load(), secondView.closed.Load())
	}
	if !runtime.hasWindow(second.windowID) || runtime.windowCount() != 1 {
		t.Fatalf("surviving window registry = %v / %d; want only window %d", runtime.hasWindow(second.windowID), runtime.windowCount(), second.windowID)
	}
	for index, shared := range fixture.shared {
		if shared.finalized.Load() || shared.adaptersClosed.Load() {
			t.Errorf("session %s closed while a window survived", fixture.ids[index])
		}
	}
	select {
	case <-runtime.completion:
		t.Fatal("runtime signaled completion while a window survived")
	default:
	}

	second.processAsyncResults()
	if !runtime.isPrimaryWindow(second.windowID) || runtime.collection.ActiveTabID() != fixture.ids[1] {
		t.Fatalf("promotion did not persist survivor active tab: primary=%v active=%q", runtime.isPrimaryWindow(second.windowID), runtime.collection.ActiveTabID())
	}
	record, ok := runtime.collection.Get(fixture.ids[1])
	if !ok {
		t.Fatal("surviving document tab missing from collection")
	}
	var persisted sessionViewSnapshot
	if err := json.Unmarshal(record.ViewState, &persisted); err != nil {
		t.Fatalf("decode promoted view state: %v", err)
	}
	if persisted.WordViewMode != wordViewWebLayout || persisted.WordLayoutZoom != 1.35 || !persisted.WordNavigationVisible || persisted.WordNavigationQuery != "独立窗口视图" || persisted.WordToolbarTab != wordTabView || persisted.SelectedWordRunID != run.ID || persisted.WordPageFirst != 3 {
		t.Fatalf("persisted promoted Word view = %+v", persisted)
	}
	payload, err := snapshotState(secondView)
	if err != nil {
		t.Fatalf("snapshot promoted Word document: %v", err)
	}
	if err := runtime.collection.Update(fixture.ids[1], payload, record.ViewState, false); err != nil {
		t.Fatalf("save promoted Word snapshot for restore: %v", err)
	}
	restoredRecord, ok := runtime.collection.Get(fixture.ids[1])
	if !ok {
		t.Fatal("promoted document disappeared before restore")
	}
	restored, err := second.restoreSessionState(*restoredRecord)
	if err != nil {
		t.Fatalf("restore promoted Word view state: %v", err)
	}
	defer func() { _ = restored.word.Close() }()
	if restored.wordViewMode != wordViewWebLayout || restored.wordLayoutZoom != 1.35 || !restored.wordNavigationVisible || restored.wordNavigationSearchQuery != "独立窗口视图" || restored.wordToolbarTab != wordTabView || restored.selectedWordRunID != run.ID || restored.wordPageListState.Position.First != 3 {
		t.Fatalf("reopened Word view = mode %v zoom %.2f navigation %v/%q tab %v run %q page %d", restored.wordViewMode, restored.wordLayoutZoom, restored.wordNavigationVisible, restored.wordNavigationSearchQuery, restored.wordToolbarTab, restored.selectedWordRunID, restored.wordPageListState.Position.First)
	}

	// Final shutdown must wait for an in-flight shared operation before closing
	// either adapter, and then signal completion after the store is closed.
	token, ok := fixture.shared[1].beginOperation(sessionOperationMutation)
	if !ok {
		t.Fatal("could not start operation before final shutdown")
	}
	closed := make(chan struct{})
	go func() {
		second.closeWorkspace()
		close(closed)
	}()
	deadline := time.After(2 * time.Second)
	for !fixture.shared[1].finalized.Load() {
		select {
		case <-deadline:
			t.Fatal("final shutdown did not start finalizing the last session")
		case <-time.After(time.Millisecond):
		}
	}
	if fixture.shared[1].adaptersClosed.Load() {
		t.Fatal("adapter closed before the active operation drained")
	}
	fixture.shared[1].finishOperation(token)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("final shutdown did not finish after the operation drained")
	}
	if runtime.windowCount() != 0 || len(runtime.sessions) != 0 || runtime.workspaceStore != nil || runtime.collection != nil {
		t.Fatalf("runtime resources remain after final shutdown: windows=%d sessions=%d store=%p collection=%p", runtime.windowCount(), len(runtime.sessions), runtime.workspaceStore, runtime.collection)
	}
	for index, shared := range fixture.shared {
		if !shared.finalized.Load() || !shared.adaptersClosed.Load() {
			t.Errorf("session %s was not finalized and closed", fixture.ids[index])
		}
	}
	select {
	case <-runtime.completion:
	default:
		t.Fatal("completion channel was not closed after final cleanup")
	}
	if err := fixture.store.SaveRecents(nil); err == nil {
		t.Fatal("workspace store still accepts writes after final shutdown")
	}
	runtime.shutdown() // repeated shutdown must be harmless and must not reopen resources.
}

func TestSessionResultFansOutAfterOriginWindowCloses(t *testing.T) {
	fixture := newWindowViewFixture(t)
	first, second, shared := fixture.apps[0], fixture.apps[1], fixture.shared[0]
	first.activateSession(fixture.ids[0], fixture.states[0])
	second.activateSession(fixture.ids[0], fixture.states[1])
	firstRevision := fixture.states[0].observedSessionRevision.Load()
	secondRevision := fixture.states[1].observedSessionRevision.Load()

	first.closeWorkspace()
	if !fixture.states[0].closed.Load() || fixture.states[1].closed.Load() {
		t.Fatalf("origin/survivor closed flags after detach = %v/%v", fixture.states[0].closed.Load(), fixture.states[1].closed.Load())
	}
	if _, ok := shared.views[second.windowID]; !ok {
		t.Fatal("surviving window view was removed from shared invalidation targets")
	}
	if _, ok := shared.views[first.windowID]; ok {
		t.Fatal("detached window remains in shared invalidation targets")
	}

	revision := shared.wordRevisionAtomic.Load() + 1
	fixture.runtime.publishSessionRevision(fixture.ids[0], revision)
	if got := fixture.states[1].observedSessionRevision.Load(); got != revision || got <= secondRevision {
		t.Fatalf("surviving view observed revision = %d, previous %d; want %d", got, secondRevision, revision)
	}
	if got := fixture.states[0].observedSessionRevision.Load(); got != firstRevision {
		t.Fatalf("detached view was touched by late result: revision %d -> %d", firstRevision, got)
	}
	if session, ok := fixture.runtime.session(fixture.ids[0]); !ok || session != shared || shared.finalized.Load() {
		t.Fatal("late result closed or replaced the surviving shared session")
	}
}
