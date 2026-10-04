package desktopui

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"glance/internal/workspace"
)

// desktopRuntime is the process-level owner of workspace persistence, shared
// document sessions, and the set of live windows.
type desktopRuntime struct {
	mu                      sync.Mutex
	workspaceQueueMu        sync.Mutex
	workspaceInitMu         sync.Mutex
	workspaceInitialized    bool
	workspaceDBMu           sync.Mutex
	workspaceStore          *workspace.Store
	collection              *workspace.Collection
	workspaceWriteJobs      chan workspaceWriteJob
	workspaceWriteResults   chan error
	workspaceWriterDone     chan struct{}
	workspaceWriterStopOnce sync.Once
	workspaceWriterStopped  bool
	shutdownOnce            sync.Once
	completion              chan struct{}
	completionOnce          sync.Once
	windowLauncher          windowLauncher
	sessions                map[string]*sharedDocumentSession
	sessionsByCanonicalPath map[string]string
	openingPaths            map[string]chan struct{}
	windows                 map[uint64]*App
	windowAttachmentOrder   map[uint64]uint64
	primaryWindowID         uint64
	nextWindowID            uint64
	nextWindowAttachment    uint64
}

func newDesktopRuntime() *desktopRuntime {
	return &desktopRuntime{
		sessions:                make(map[string]*sharedDocumentSession),
		sessionsByCanonicalPath: make(map[string]string),
		openingPaths:            make(map[string]chan struct{}),
		windows:                 make(map[uint64]*App),
		windowAttachmentOrder:   make(map[uint64]uint64),
	}
}

func (r *desktopRuntime) allocateWindowID() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextWindowID++
	return r.nextWindowID
}

func (a *App) ensureDesktopRuntime() *desktopRuntime {
	if a.desktopRuntime == nil {
		a.desktopRuntime = newDesktopRuntime()
	}
	return a.desktopRuntime
}

func (r *desktopRuntime) session(documentID string) (*sharedDocumentSession, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	session, ok := r.sessions[documentID]
	r.mu.Unlock()
	return session, ok
}

func (r *desktopRuntime) registerSession(session *sharedDocumentSession) *sharedDocumentSession {
	if r == nil || session == nil {
		return session
	}
	r.mu.Lock()
	if r.sessions == nil {
		r.sessions = make(map[string]*sharedDocumentSession)
	}
	if r.sessionsByCanonicalPath == nil {
		r.sessionsByCanonicalPath = make(map[string]string)
	}
	if session.documentID == "" {
		if session.path != "" {
			session.documentID = workspaceSessionID(session.path)
		} else {
			session.documentID = newDraftSessionID("document")
		}
	}
	existing := r.findSessionByPathLocked(session.path)
	if existing == nil {
		existing = r.sessions[session.documentID]
	}
	if existing == nil {
		r.sessions[session.documentID] = session
		session.ownerRuntime = r
		if key := canonicalSessionPath(session.path); key != "" {
			r.sessionsByCanonicalPath[key] = session.documentID
		}
		existing = session
	}
	r.mu.Unlock()
	if existing != session {
		closeUnregisteredSessionAdapters(session, existing)
	}
	return existing
}

func (r *desktopRuntime) findSessionByPathLocked(path string) *sharedDocumentSession {
	if path == "" {
		return nil
	}
	key := canonicalSessionPath(path)
	if documentID := r.sessionsByCanonicalPath[key]; documentID != "" {
		if session := r.sessions[documentID]; session != nil {
			return session
		}
		delete(r.sessionsByCanonicalPath, key)
	}
	for _, session := range r.sessions {
		if session != nil && sameDocumentPath(session.path, path) {
			return session
		}
	}
	return nil
}

func canonicalSessionPath(path string) string {
	if path == "" {
		return ""
	}
	return canonicalDocumentPath(path)
}

func (r *desktopRuntime) addWordSnapshot(path string, source workspace.WordVersionSource, snapshot []byte) error {
	if r == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	pathKey := canonicalSessionPath(path)
	if pathKey == "" {
		return fmt.Errorf("Word 文档没有可用于版本历史的路径")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	_, _, err := r.workspaceStore.AddWordVersion(workspace.WordVersionInput{
		PathKey: pathKey, DisplayPath: path, Source: source,
		CapturedAt: time.Now().UTC(), Snapshot: snapshot,
	})
	return err
}

func (r *desktopRuntime) loadWordVersion(pathKey string, versionID int64) (workspace.WordVersion, []byte, error) {
	if r == nil {
		return workspace.WordVersion{}, nil, fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore == nil {
		return workspace.WordVersion{}, nil, fmt.Errorf("工作区数据库不可用")
	}
	return r.workspaceStore.LoadWordVersion(pathKey, versionID)
}

func (r *desktopRuntime) listWordVersions(pathKey string) ([]workspace.WordVersion, error) {
	if r == nil {
		return nil, fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore == nil {
		return nil, fmt.Errorf("工作区数据库不可用")
	}
	return r.workspaceStore.ListWordVersions(pathKey)
}

func (r *desktopRuntime) recordOpenedWord(shared *sharedDocumentSession) error {
	if shared == nil || shared.word == nil {
		return nil
	}
	shared.adapterMu.RLock()
	path := shared.path
	if path == "" {
		shared.adapterMu.RUnlock()
		return nil
	}
	snapshot, err := shared.word.Snapshot()
	shared.adapterMu.RUnlock()
	if err != nil {
		return err
	}
	return r.addWordSnapshot(path, workspace.WordVersionOpened, snapshot)
}

func closeUnregisteredSessionAdapters(candidate, existing *sharedDocumentSession) {
	if candidate == nil || existing == nil {
		return
	}
	if candidate.book != nil && !sameAdapterInstance(candidate.book, existing.book) {
		candidate.adapterMu.Lock()
		_ = candidate.book.Close()
		candidate.adapterMu.Unlock()
	}
	if candidate.word != nil && candidate.word != existing.word {
		candidate.adapterMu.Lock()
		_ = candidate.word.Close()
		candidate.adapterMu.Unlock()
	}
	if candidate.jsonDoc != nil && candidate.jsonDoc != existing.jsonDoc {
		candidate.adapterMu.Lock()
		_ = candidate.jsonDoc.Close()
		candidate.adapterMu.Unlock()
	}
	candidate.finalized.Store(true)
}

func sameAdapterInstance(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || leftValue.Kind() != reflect.Pointer {
		return false
	}
	return leftValue.Pointer() == rightValue.Pointer()
}

func (r *desktopRuntime) openDocumentSession(path string) (*sharedDocumentSession, error) {
	session, _, err := r.openDocumentSessionWithStatus(path)
	return session, err
}

func (r *desktopRuntime) openDocumentSessionWithStatus(path string) (*sharedDocumentSession, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("文档运行时不可用")
	}
	path = stringsTrimSpace(path)
	key := canonicalSessionPath(path)
	for {
		r.mu.Lock()
		if r.sessions == nil {
			r.sessions = make(map[string]*sharedDocumentSession)
		}
		if r.sessionsByCanonicalPath == nil {
			r.sessionsByCanonicalPath = make(map[string]string)
		}
		if r.openingPaths == nil {
			r.openingPaths = make(map[string]chan struct{})
		}
		if existing := r.findSessionByPathLocked(path); existing != nil {
			r.mu.Unlock()
			return existing, false, nil
		}
		if wait := r.openingPaths[key]; wait != nil {
			r.mu.Unlock()
			<-wait
			continue
		}
		if r.openingPaths == nil {
			r.openingPaths = make(map[string]chan struct{})
		}
		opened := make(chan struct{})
		r.openingPaths[key] = opened
		r.mu.Unlock()

		loaded, err := loadDocument(path)
		var candidate *sharedDocumentSession
		if err == nil {
			candidate = sharedSessionFromLoadedDocument(loaded)
		}

		r.mu.Lock()
		existing := r.findSessionByPathLocked(path)
		var result *sharedDocumentSession
		openedFromDisk := false
		if err != nil {
			result = nil
		} else if existing != nil {
			result = existing
		} else {
			result = r.registerSessionLocked(candidate)
			openedFromDisk = result == candidate
		}
		delete(r.openingPaths, key)
		close(opened)
		r.mu.Unlock()
		if candidate != nil && result != candidate {
			closeUnregisteredSessionAdapters(candidate, result)
		}
		return result, openedFromDisk, err
	}
}

func sharedSessionFromLoadedDocument(loaded loadedDocument) *sharedDocumentSession {
	if loaded.shared != nil {
		return loaded.shared
	}
	session := newSharedDocumentSession(loaded.path)
	if loaded.path == "" {
		format := "document"
		switch {
		case loaded.book != nil:
			format = "xlsx"
		case loaded.word != nil:
			format = "docx"
		case loaded.jsonDoc != nil:
			format = "json"
		}
		session.documentID = newDraftSessionID(format)
	}
	session.book = loaded.book
	session.word = loaded.word
	session.wordBlocks = cloneWordBlockSnapshot(loaded.wordBlocks)
	if loaded.word != nil && loaded.wordBlocks == nil {
		session.wordBlocks = cloneWordBlockSnapshot(loaded.word.WordBlocks())
	}
	session.jsonDoc = loaded.jsonDoc
	session.sheets = append([]string(nil), loaded.sheets...)
	return session
}

func loadedDocumentFromSharedSession(session *sharedDocumentSession) loadedDocument {
	if session == nil {
		return loadedDocument{}
	}
	return loadedDocument{
		shared:     session,
		path:       session.path,
		book:       session.book,
		word:       session.word,
		wordBlocks: cloneWordBlockSnapshot(session.wordBlocks),
		jsonDoc:    session.jsonDoc,
		sheets:     append([]string(nil), session.sheets...),
	}
}

func (r *desktopRuntime) registerSessionLocked(session *sharedDocumentSession) *sharedDocumentSession {
	if session.documentID == "" {
		if session.path != "" {
			session.documentID = workspaceSessionID(session.path)
		} else {
			session.documentID = newDraftSessionID("document")
		}
	}
	if existing := r.findSessionByPathLocked(session.path); existing != nil {
		return existing
	}
	if existing := r.sessions[session.documentID]; existing != nil {
		return existing
	}
	r.sessions[session.documentID] = session
	session.ownerRuntime = r
	if key := canonicalSessionPath(session.path); key != "" {
		r.sessionsByCanonicalPath[key] = session.documentID
	}
	return session
}

func (r *desktopRuntime) invalidateSessionViews(documentID string) {
	if r == nil || documentID == "" {
		return
	}
	r.mu.Lock()
	apps := make([]*App, 0, len(r.windows))
	for _, app := range r.windows {
		if app != nil && app.activeSessionID() == documentID {
			apps = append(apps, app)
		}
	}
	r.mu.Unlock()
	for _, app := range apps {
		if app.window != nil {
			app.window.Invalidate()
		}
	}
}

func (r *desktopRuntime) updateSessionPath(session *sharedDocumentSession, path string) {
	if r == nil || session == nil {
		return
	}
	r.mu.Lock()
	if r.sessions[session.documentID] == session {
		session.adapterMu.Lock()
		if oldKey := canonicalSessionPath(session.path); oldKey != "" && r.sessionsByCanonicalPath[oldKey] == session.documentID {
			delete(r.sessionsByCanonicalPath, oldKey)
		}
		session.path = path
		if key := canonicalSessionPath(path); key != "" {
			r.sessionsByCanonicalPath[key] = session.documentID
		}
		session.adapterMu.Unlock()
	}
	r.mu.Unlock()
}

func (r *desktopRuntime) removeSession(documentID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	session := r.sessions[documentID]
	delete(r.sessions, documentID)
	if session != nil {
		if key := canonicalSessionPath(session.path); key != "" && r.sessionsByCanonicalPath[key] == documentID {
			delete(r.sessionsByCanonicalPath, key)
		}
	}
	r.mu.Unlock()
	if session != nil {
		session.viewsMu.RLock()
		views := make([]*documentSessionState, 0, len(session.views))
		for _, view := range session.views {
			views = append(views, view)
		}
		session.viewsMu.RUnlock()
		for _, view := range views {
			if view != nil {
				view.closed.Store(true)
			}
		}
		r.mu.Lock()
		apps := make([]*App, 0, len(r.windows))
		for _, app := range r.windows {
			apps = append(apps, app)
		}
		r.mu.Unlock()
		for _, app := range apps {
			if app != nil && app.activeSessionID() == documentID {
				app.setActiveSessionID("")
			}
		}
	}
	closeSharedDocumentSession(session)
	r.invalidateWindows()
}

func closeSharedDocumentSession(session *sharedDocumentSession) {
	if session == nil {
		return
	}
	for {
		session.operationMu.Lock()
		session.finalized.Store(true)
		if !session.operationActive {
			session.operationMu.Unlock()
			break
		}
		changed := session.operationChanged
		session.operationMu.Unlock()
		<-changed
	}
	if !session.adaptersClosed.CompareAndSwap(false, true) {
		return
	}
	session.adapterMu.Lock()
	defer session.adapterMu.Unlock()
	if session.book != nil {
		_ = session.book.Close()
	}
	if session.word != nil {
		_ = session.word.Close()
	}
	if session.jsonDoc != nil {
		_ = session.jsonDoc.Close()
	}
}

func (r *desktopRuntime) attachWindow(windowID uint64, app *App) {
	if r == nil || app == nil {
		return
	}
	r.mu.Lock()
	if r.windows == nil {
		r.windows = make(map[uint64]*App)
	}
	if r.windowAttachmentOrder == nil {
		r.windowAttachmentOrder = make(map[uint64]uint64)
	}
	if windowID == 0 {
		r.nextWindowID++
		windowID = r.nextWindowID
	} else if windowID > r.nextWindowID {
		r.nextWindowID = windowID
	}
	r.windows[windowID] = app
	if _, attached := r.windowAttachmentOrder[windowID]; !attached {
		r.nextWindowAttachment++
		r.windowAttachmentOrder[windowID] = r.nextWindowAttachment
	}
	if r.primaryWindowID == 0 {
		r.primaryWindowID = windowID
	}
	r.mu.Unlock()
	app.desktopRuntime = r
	app.windowID = windowID
	if app.documentSessionState != nil {
		app.documentSessionState.windowID = windowID
		app.setActiveSessionID(app.documentSessionState.documentID)
		app.documentSessionState.sharedDocumentSession.attachView(windowID, app.documentSessionState)
	}
}

func (r *desktopRuntime) detachWindow(windowID uint64) {
	if r == nil || windowID == 0 {
		return
	}
	r.mu.Lock()
	app := r.windows[windowID]
	if app == nil {
		r.mu.Unlock()
		return
	}
	delete(r.windows, windowID)
	delete(r.windowAttachmentOrder, windowID)
	wasPrimary := r.primaryWindowID == windowID
	if wasPrimary {
		r.primaryWindowID = 0
	}
	sessions := make([]*sharedDocumentSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	for _, session := range sessions {
		session.viewsMu.Lock()
		view := session.views[windowID]
		delete(session.views, windowID)
		session.viewsMu.Unlock()
		if view != nil {
			view.closed.Store(true)
		}
	}
	for _, view := range app.sessions {
		if view != nil {
			view.closed.Store(true)
		}
	}
	if app.documentSessionState != nil {
		app.documentSessionState.closed.Store(true)
	}
	if wasPrimary {
		r.promotePrimaryWindow()
	}
}

// promotePrimaryWindow assigns the oldest attached window and asks that
// window's own Gio loop to persist its independent workspace view state.
func (r *desktopRuntime) promotePrimaryWindow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.primaryWindowID != 0 && r.windows[r.primaryWindowID] != nil {
		r.mu.Unlock()
		return
	}
	var promotedID, oldestOrder uint64
	var promoted *App
	for windowID, app := range r.windows {
		order := r.windowAttachmentOrder[windowID]
		if app == nil || (promotedID != 0 && order >= oldestOrder) {
			continue
		}
		promotedID, promoted, oldestOrder = windowID, app, order
	}
	r.primaryWindowID = promotedID
	r.mu.Unlock()
	if promoted != nil {
		promoted.primaryPromotionPending.Store(true)
		if promoted.window != nil {
			promoted.window.Invalidate()
		}
	}
}

func (r *desktopRuntime) isPrimaryWindow(windowID uint64) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	primary := r.primaryWindowID == windowID && (windowID != 0 || len(r.windows) == 0)
	r.mu.Unlock()
	return primary
}

func (r *desktopRuntime) hasWindows() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	hasWindows := len(r.windows) > 0
	r.mu.Unlock()
	return hasWindows
}

func (r *desktopRuntime) viewsForSession(documentID string) []*App {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	apps := make([]*App, 0, len(r.windows))
	for _, app := range r.windows {
		if app != nil {
			apps = append(apps, app)
		}
	}
	r.mu.Unlock()
	views := make([]*App, 0, len(apps))
	for _, app := range apps {
		if app.activeSessionID() == documentID {
			views = append(views, app)
		}
	}
	return views
}

func (r *desktopRuntime) windowCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	count := len(r.windows)
	r.mu.Unlock()
	return count
}

func (r *desktopRuntime) hasWindow(windowID uint64) bool {
	if r == nil || windowID == 0 {
		return false
	}
	r.mu.Lock()
	_, exists := r.windows[windowID]
	r.mu.Unlock()
	return exists
}

func (r *desktopRuntime) startWorkspaceWriter() {
	if r == nil {
		return
	}
	r.workspaceQueueMu.Lock()
	r.mu.Lock()
	if r.workspaceStore == nil || r.workspaceWriteJobs != nil || r.workspaceWriterStopped {
		r.mu.Unlock()
		r.workspaceQueueMu.Unlock()
		return
	}
	r.workspaceWriteJobs = make(chan workspaceWriteJob, 128)
	r.workspaceWriteResults = make(chan error, 1)
	r.workspaceWriterDone = make(chan struct{})
	jobs, done := r.workspaceWriteJobs, r.workspaceWriterDone
	r.mu.Unlock()
	r.workspaceQueueMu.Unlock()
	go func() {
		defer close(done)
		for job := range jobs {
			if job.barrier != nil {
				close(job.barrier)
				continue
			}
			var err error
			if job.recents != nil {
				err = r.workspaceStore.SaveRecents(job.recents)
			} else if job.saveFavorite {
				err = r.workspaceStore.SaveFavorite(job.favoritePath, job.favoriteValue)
			} else {
				err = r.writeWorkspaceSnapshot(job)
			}
			r.publishWorkspaceWriteResult(err)
			r.invalidateWindows()
		}
	}()
}

func (r *desktopRuntime) drainWorkspaceWriter() {
	if r == nil {
		return
	}
	r.workspaceQueueMu.Lock()
	if r.workspaceWriterStopped {
		r.workspaceQueueMu.Unlock()
		return
	}
	r.mu.Lock()
	jobs, done := r.workspaceWriteJobs, r.workspaceWriterDone
	r.mu.Unlock()
	if jobs == nil || done == nil {
		r.workspaceQueueMu.Unlock()
		return
	}
	barrier := make(chan struct{})
	jobs <- workspaceWriteJob{barrier: barrier}
	r.workspaceQueueMu.Unlock()
	<-barrier
}

func (r *desktopRuntime) publishWorkspaceWriteResult(err error) {
	if r == nil || r.workspaceWriteResults == nil {
		return
	}
	select {
	case r.workspaceWriteResults <- err:
		return
	default:
	}
	select {
	case <-r.workspaceWriteResults:
	default:
	}
	select {
	case r.workspaceWriteResults <- err:
	default:
	}
}

func (r *desktopRuntime) writeWorkspaceSnapshot(job workspaceWriteJob) error {
	state := job.state
	if state == nil || state.closed.Load() {
		return nil
	}
	token, ok := state.beginOperationWait(sessionOperationSnapshot)
	if !ok {
		return nil
	}
	defer state.finishOperation(token)
	payload, err := snapshotState(state)
	if err != nil {
		return err
	}
	if r.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if state.closed.Load() {
		return nil
	}
	return r.workspaceStore.SaveDocument(workspace.Document{ID: job.documentID, Format: job.format, Payload: payload, ViewState: job.viewState, Dirty: sessionDirty(state), UpdatedAt: time.Now().UTC()})
}

func (r *desktopRuntime) invalidateWindows() {
	if r == nil {
		return
	}
	r.mu.Lock()
	apps := make([]*App, 0, len(r.windows))
	for _, app := range r.windows {
		apps = append(apps, app)
	}
	r.mu.Unlock()
	for _, app := range apps {
		if app != nil && app.window != nil {
			app.window.Invalidate()
		}
	}
}

func (r *desktopRuntime) shutdown() {
	if r == nil {
		return
	}
	r.shutdownOnce.Do(func() {
		r.workspaceWriterStopOnce.Do(func() {
			r.workspaceQueueMu.Lock()
			r.workspaceWriterStopped = true
			r.mu.Lock()
			if r.workspaceWriteJobs != nil {
				close(r.workspaceWriteJobs)
			}
			done := r.workspaceWriterDone
			r.mu.Unlock()
			r.workspaceQueueMu.Unlock()
			if done != nil {
				<-done
			}
		})
		r.mu.Lock()
		sessions := make([]*sharedDocumentSession, 0, len(r.sessions))
		for _, session := range r.sessions {
			sessions = append(sessions, session)
		}
		r.sessions = make(map[string]*sharedDocumentSession)
		r.sessionsByCanonicalPath = make(map[string]string)
		store := r.workspaceStore
		r.workspaceStore = nil
		r.collection = nil
		r.mu.Unlock()
		for _, session := range sessions {
			closeSharedDocumentSession(session)
		}
		if store != nil {
			_ = store.Close()
		}
		r.completionOnce.Do(func() {
			if r.completion != nil {
				close(r.completion)
			}
		})
	})
}

// retain the package's existing whitespace normalization without duplicating
// path-specific policy in the runtime's session key logic.
func stringsTrimSpace(value string) string {
	return strings.TrimSpace(value)
}
