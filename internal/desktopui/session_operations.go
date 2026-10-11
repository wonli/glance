package desktopui

import "glance/internal/document"

import (
	"context"
)

type sessionOperationKind uint8

const (
	sessionOperationRead sessionOperationKind = iota + 1
	sessionOperationMutation
	sessionOperationSave
	sessionOperationSnapshot
)

type sessionOperationToken struct {
	documentID string
	generation uint64
	revision   uint64
	kind       sessionOperationKind
}

func lockSessionAdapterRead(state *documentSessionState) func() {
	if state == nil || state.sharedDocumentSession == nil {
		return func() {}
	}
	state.adapterMu.RLock()
	return state.adapterMu.RUnlock
}

func lockSessionAdapterWrite(state *documentSessionState) func() {
	if state == nil || state.sharedDocumentSession == nil {
		return func() {}
	}
	state.adapterMu.Lock()
	return state.adapterMu.Unlock
}

func runWordMutation(state *documentSessionState, mutate func() error) (blocks []document.WordBlock, err error) {
	unlock := lockSessionAdapterWrite(state)
	defer unlock()
	err = mutate()
	if err == nil && state != nil && state.word != nil {
		blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
	}
	return blocks, err
}

func (a *App) completeSessionOperation(state *documentSessionState, token sessionOperationToken, err error, publishRevision bool) {
	if state == nil {
		return
	}
	if err == nil && publishRevision {
		a.ensureDesktopRuntime().publishSessionRevision(token.documentID, token.revision+1)
	}
	state.finishOperation(token)
}

func (s *sharedDocumentSession) beginOperation(kind sessionOperationKind) (sessionOperationToken, bool) {
	if s == nil {
		return sessionOperationToken{}, false
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.finalized.Load() || s.operationActive {
		return sessionOperationToken{}, false
	}
	s.operationGeneration++
	token := sessionOperationToken{
		documentID: s.documentID,
		generation: s.operationGeneration,
		revision:   s.wordRevisionAtomic.Load(),
		kind:       kind,
	}
	s.activeOperation = token
	s.operationActive = true
	s.operationChanged = make(chan struct{})
	s.formatRunning.Store(true)
	if kind == sessionOperationSave {
		s.saveRunning.Store(true)
	}
	return token, true
}

func (s *sharedDocumentSession) beginOperationWait(kind sessionOperationKind) (sessionOperationToken, bool) {
	for {
		if token, ok := s.beginOperation(kind); ok {
			return token, true
		}
		if s == nil {
			return sessionOperationToken{}, false
		}
		s.operationMu.Lock()
		if s.finalized.Load() {
			s.operationMu.Unlock()
			return sessionOperationToken{}, false
		}
		if !s.operationActive {
			s.operationMu.Unlock()
			continue
		}
		changed := s.operationChanged
		s.operationMu.Unlock()
		<-changed
	}
}

func (s *sharedDocumentSession) finishOperation(token sessionOperationToken) bool {
	if s == nil {
		return false
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if !s.operationActive || s.activeOperation != token || token.documentID != s.documentID {
		return false
	}
	s.operationActive = false
	s.activeOperation = sessionOperationToken{}
	s.formatRunning.Store(false)
	if token.kind == sessionOperationSave {
		s.saveRunning.Store(false)
	}
	changed := s.operationChanged
	close(changed)
	idle := make(chan struct{})
	close(idle)
	s.operationChanged = idle
	return true
}

func (s *sharedDocumentSession) currentOperationToken() (sessionOperationToken, bool) {
	if s == nil {
		return sessionOperationToken{}, false
	}
	s.operationMu.Lock()
	token, ok := s.activeOperation, s.operationActive
	s.operationMu.Unlock()
	return token, ok
}

func (s *sharedDocumentSession) finalizeIfIdle() bool {
	if s == nil {
		return false
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.operationActive || s.finalized.Load() {
		return false
	}
	s.finalized.Store(true)
	return true
}

// finalizeIfIdleOrSnapshot allows a workspace snapshot to finish harmlessly
// after a user discards its draft. The writer checks closed before committing,
// and workspace DB serialization ensures a commit already in progress is
// deleted before the close operation returns.
func (s *sharedDocumentSession) finalizeIfIdleOrSnapshot() bool {
	if s == nil {
		return false
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.finalized.Load() || (s.operationActive && s.activeOperation.kind != sessionOperationSnapshot) {
		return false
	}
	s.finalized.Store(true)
	return true
}

func (s *sharedDocumentSession) reopenAfterFailedClose() {
	if s == nil {
		return
	}
	s.operationMu.Lock()
	if !s.adaptersClosed.Load() {
		s.finalized.Store(false)
	}
	s.operationMu.Unlock()
}

func (s *sharedDocumentSession) acceptsResult(token sessionOperationToken) bool {
	if s == nil || token.documentID != s.documentID || s.finalized.Load() {
		return false
	}
	s.operationMu.Lock()
	accepted := !s.finalized.Load() && token.generation != 0 && token.generation == s.operationGeneration
	s.operationMu.Unlock()
	return accepted
}

func (s *sharedDocumentSession) advanceRevision(revision uint64) bool {
	if s == nil || s.finalized.Load() {
		return false
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.finalized.Load() || revision <= s.publishedRevision.Load() {
		return false
	}
	if revision > s.wordRevisionAtomic.Load() {
		s.wordRevisionAtomic.Store(revision)
	}
	s.wordRevision = revision
	s.publishedRevision.Store(revision)
	return true
}

func (r *desktopRuntime) publishSessionRevision(documentID string, revision uint64) bool {
	if r == nil {
		return false
	}
	session, ok := r.session(documentID)
	if !ok || !session.advanceRevision(revision) {
		return false
	}
	if r.applicationRuntime != nil {
		_ = r.applicationRuntime.SyncDocumentRevision(context.Background(), documentID, revision)
	}
	for _, app := range r.viewsForSession(documentID) {
		if state := app.documentSessionState; state != nil && state.sharedDocumentSession == session {
			state.observedSessionRevision.Store(revision)
		}
		if app.window != nil {
			app.window.Invalidate()
		}
	}
	return true
}

// publishCommandRevision refreshes views for a revision already committed by
// application.Runtime. CommandBus owns the revision and publishes its event.
func (r *desktopRuntime) publishCommandRevision(documentID string, revision uint64) {
	if r == nil {
		return
	}
	session, ok := r.session(documentID)
	if !ok {
		return
	}
	session.advanceRevision(revision)
	for _, app := range r.viewsForSession(documentID) {
		if state := app.documentSessionState; state != nil && state.sharedDocumentSession == session {
			state.observedSessionRevision.Store(revision)
			if state.virtualGrid != nil {
				state.virtualGrid.invalidateWindow()
			}
		}
		if app.window != nil {
			app.window.Invalidate()
		}
	}
}

func (a *App) activeSessionID() string {
	if a == nil {
		return ""
	}
	value := a.activeSessionDocumentID.Load()
	id, _ := value.(string)
	return id
}

func (a *App) setActiveSessionID(documentID string) {
	if a != nil {
		a.activeSessionDocumentID.Store(documentID)
	}
}

func (a *App) isSaving() bool {
	if a == nil {
		return false
	}
	if a.saving {
		return true
	}
	return a.documentSessionState != nil && a.documentSessionState.saveRunning.Load()
}
