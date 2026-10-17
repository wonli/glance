package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session is a format-neutral live document and its serializable UI state.
type Session struct {
	Tab       Tab
	Format    string
	Payload   []byte
	ViewState []byte
	Dirty     bool
	UpdatedAt time.Time
	Available bool
}

// Collection owns the durable ordering and activation state for document sessions.
type Collection struct {
	mu      sync.RWMutex
	version uint64
	store   *Store
	tabs    []*Session
	active  string
	recents []Recent
}

func OpenCollection(store *Store) (*Collection, error) {
	if store == nil {
		return nil, errors.New("workspace store is required")
	}
	w, err := store.Load()
	if err != nil {
		return nil, err
	}
	c := &Collection{store: store, active: w.ActiveTabID, recents: w.Recents, version: 1}
	if c.active == "" {
		c.active = RecentTabID
	}
	for _, tab := range w.Tabs {
		if tab.DocumentID == "" {
			tab.DocumentID = tab.ID
		}
		doc, found, err := store.LoadDocumentMetadata(tab.DocumentID)
		if err != nil {
			return nil, err
		}
		session := &Session{Tab: tab, Available: found || tab.SourcePath == ""}
		if found {
			session.Format, session.ViewState, session.Dirty, session.UpdatedAt = doc.Format, doc.ViewState, doc.Dirty, doc.UpdatedAt
		} else {
			session.Format = inferDraftFormat(tab)
			if session.Format == "" && tab.SourcePath != "" {
				ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(tab.SourcePath)), ".")
				if ext == "md" {
					ext = "markdown"
				}
				session.Format = ext
			}
		}
		if tab.SourcePath != "" {
			if _, err := os.Stat(tab.SourcePath); err != nil {
				session.Available = false
			} else {
				session.Available = true
			}
		}
		c.tabs = append(c.tabs, session)
	}
	positionsChanged := false
	for position, session := range c.tabs {
		if session.Tab.Position != position {
			session.Tab.Position = position
			positionsChanged = true
		}
	}
	if positionsChanged {
		if err := c.persist(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func inferDraftFormat(tab Tab) string {
	if tab.Kind != "draft" || tab.SourcePath != "" {
		return ""
	}
	id := tab.DocumentID
	if id == "" {
		id = tab.ID
	}
	for _, format := range []string{"docx", "xlsx", "json"} {
		if strings.HasPrefix(id, "draft-"+format+"-") {
			return format
		}
	}
	return ""
}

func (c *Collection) ActiveTabID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active
}

func (c *Collection) ensurePayloadLocked(session *Session) {
	if session != nil && session.Payload == nil && c.store != nil {
		if payload, err := c.store.LoadDocumentPayload(session.Tab.DocumentID); err == nil && len(payload) > 0 {
			session.Payload = payload
		}
	}
}

func (c *Collection) Active() *Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == RecentTabID {
		return nil
	}
	session := c.getLocked(c.active)
	c.ensurePayloadLocked(session)
	return cloneSession(session)
}

func (c *Collection) Get(id string) (*Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	session := c.getLocked(id)
	if session == nil {
		return nil, false
	}
	c.ensurePayloadLocked(session)
	return cloneSession(session), true
}

func (c *Collection) Version() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

func (c *Collection) TabCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.tabs)
}

func (c *Collection) TabMetadata() []Tab {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Tab, len(c.tabs))
	for i, s := range c.tabs {
		out[i] = s.Tab
	}
	return out
}

func (c *Collection) Tabs() []Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Session, len(c.tabs))
	for i, s := range c.tabs {
		c.ensurePayloadLocked(s)
		out[i] = *cloneSession(s)
	}
	return out
}

// ReleaseInactivePayloads drops in-memory cached payloads for all sessions except
// the specified active document. The payloads remain safe in SQLite.
func (c *Collection) ReleaseInactivePayloads(activeDocumentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.tabs {
		if s.Tab.DocumentID != activeDocumentID && s.Tab.ID != activeDocumentID {
			s.Payload = nil
		}
	}
}

func (c *Collection) Recents() []Recent {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Recent(nil), c.recents...)
}

func (c *Collection) Activate(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id != RecentTabID {
		if c.getLocked(id) == nil {
			return fmt.Errorf("workspace tab %q does not exist", id)
		}
	}
	old := c.active
	c.active = id
	if err := c.persist(); err != nil {
		c.active = old
		return err
	}
	return nil
}

func (c *Collection) Put(session Session) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.putLocked(session, true)
}

func (c *Collection) Refresh(session Session) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.putLocked(session, false)
}

func (c *Collection) AddTab(tab Tab, format string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.putLocked(Session{Tab: tab, Format: format, Available: true}, true)
}

// AddTabInactive adds a shared tab without changing the collection's durable
// active tab. Secondary windows use it to keep activation local to each view.
func (c *Collection) AddTabInactive(tab Tab, format string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.putLocked(Session{Tab: tab, Format: format, Available: true}, false)
}

// AddDocument adds a tab together with its initial durable snapshot. New
// workspace drafts must use this instead of AddTab so a restart can always
// restore the document even if the asynchronous editor snapshot has not run.
func (c *Collection) AddDocument(session Session, activate bool) error {
	// Empty Markdown source is a complete snapshot. A nil payload still means
	// the caller did not supply a snapshot; structured formats need bytes.
	emptySource := strings.EqualFold(session.Format, "markdown") || strings.EqualFold(session.Format, "md")
	if session.Payload == nil || (len(session.Payload) == 0 && !emptySource) {
		return errors.New("workspace document snapshot is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.putLocked(session, activate)
}

func (c *Collection) UpdateTabMeta(tab Tab) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.tabs {
		if s.Tab.ID == tab.ID {
			s.Tab = tab
			c.version++
			return c.persist()
		}
	}
	return fmt.Errorf("workspace tab %q does not exist", tab.ID)
}

func (c *Collection) putLocked(session Session, activate bool) error {
	if session.Tab.ID == "" {
		return errors.New("workspace session tab id is required")
	}
	if session.Tab.DocumentID == "" {
		session.Tab.DocumentID = session.Tab.ID
	}
	if session.Format == "" {
		return errors.New("workspace session format is required")
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	}
	session.Available = true
	c.version++
	if old := c.getLocked(session.Tab.ID); old != nil {
		*old = session
	} else {
		session.Tab.Position = len(c.tabs)
		c.tabs = append(c.tabs, &session)
	}
	oldActive := c.active
	if activate {
		c.active = session.Tab.ID
	}
	if session.Payload != nil {
		if err := c.store.SaveDocument(Document{ID: session.Tab.DocumentID, Format: session.Format, Payload: session.Payload, ViewState: session.ViewState, Dirty: session.Dirty, UpdatedAt: session.UpdatedAt}); err != nil {
			return err
		}
	}
	if err := c.persist(); err != nil {
		c.active = oldActive
		return err
	}
	return nil
}

func (c *Collection) Update(id string, payload, viewState []byte, dirty bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.getLocked(id)
	if s == nil {
		return fmt.Errorf("workspace tab %q does not exist", id)
	}
	doc := Document{ID: s.Tab.DocumentID, Format: s.Format, Payload: append([]byte(nil), payload...), ViewState: append([]byte(nil), viewState...), Dirty: dirty, UpdatedAt: time.Now().UTC()}
	if err := c.store.SaveDocument(doc); err != nil {
		return err
	}
	s.Payload = doc.Payload
	s.ViewState = doc.ViewState
	s.Dirty = dirty
	s.UpdatedAt = doc.UpdatedAt
	return nil
}

func (c *Collection) SaveViewState(id string, viewState []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.getLocked(id)
	if s == nil {
		return fmt.Errorf("workspace tab %q does not exist", id)
	}
	if err := c.store.SaveViewState(s.Tab.DocumentID, viewState); err != nil {
		return err
	}
	s.ViewState = append(s.ViewState[:0], viewState...)
	return nil
}

func (c *Collection) AddRecent(recent Recent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if recent.Path == "" {
		return errors.New("recent path is required")
	}
	if recent.OpenedAt.IsZero() {
		recent.OpenedAt = time.Now().UTC()
	}
	for i := range c.recents {
		if c.recents[i].Path == recent.Path {
			c.recents[i] = recent
			return c.persist()
		}
	}
	c.recents = append(c.recents, recent)
	return c.persist()
}

// StageRecents updates the in-memory recent list without touching SQLite.
// Callers that need background persistence can enqueue the list to Store.SaveRecents.
func (c *Collection) StageRecents(recents []Recent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, recent := range recents {
		if recent.Path == "" {
			continue
		}
		if recent.OpenedAt.IsZero() {
			recent.OpenedAt = time.Now().UTC()
		}
		updated := false
		for i := range c.recents {
			if c.recents[i].Path == recent.Path {
				c.recents[i] = recent
				updated = true
				break
			}
		}
		if !updated {
			c.recents = append(c.recents, recent)
		}
	}
}

// StageRecentsSnapshot replaces the in-memory recent list without touching
// SQLite. Callers can persist that snapshot through a background writer.
func (c *Collection) StageRecentsSnapshot(recents []Recent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recents = append(make([]Recent, 0, len(recents)), recents...)
}

func (c *Collection) RemoveRecent(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, recent := range c.recents {
		if recent.Path == path {
			c.recents = append(c.recents[:i], c.recents[i+1:]...)
			break
		}
	}
	return c.store.DeleteRecent(path)
}

func (c *Collection) CloseTab(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	index := -1
	for i, s := range c.tabs {
		if s.Tab.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("workspace tab %q does not exist", id)
	}
	if c.active == id {
		c.active = RecentTabID
		if index > 0 {
			c.active = c.tabs[index-1].Tab.ID
		} else if len(c.tabs) > 1 {
			c.active = c.tabs[1].Tab.ID
		}
	}
	removed := c.tabs[index]
	copy(c.tabs[index:], c.tabs[index+1:])
	c.tabs[len(c.tabs)-1] = nil
	c.tabs = c.tabs[:len(c.tabs)-1]
	for i, s := range c.tabs {
		s.Tab.Position = i
	}
	c.version++
	if err := c.persist(); err != nil {
		c.tabs = append(c.tabs, nil)
		copy(c.tabs[index+1:], c.tabs[index:])
		c.tabs[index] = removed
		return err
	}
	if err := c.store.DeleteDocument(removed.Tab.DocumentID); err != nil {
		return err
	}
	return nil
}

func (c *Collection) persist() error {
	w := Snapshot{ActiveTabID: c.active, Recents: c.recents, Tabs: make([]Tab, len(c.tabs))}
	for i, s := range c.tabs {
		s.Tab.Position = i
		w.Tabs[i] = s.Tab
	}
	return c.store.ReplaceWorkspace(w)
}

func (c *Collection) getLocked(id string) *Session {
	for _, session := range c.tabs {
		if session.Tab.ID == id {
			return session
		}
	}
	return nil
}

func cloneSession(session *Session) *Session {
	if session == nil {
		return nil
	}
	copy := *session
	return &copy
}
