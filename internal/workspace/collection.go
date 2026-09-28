package workspace

import (
	"errors"
	"fmt"
	"os"
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
	c := &Collection{store: store, active: w.ActiveTabID, recents: w.Recents}
	if c.active == "" {
		c.active = RecentTabID
	}
	for _, tab := range w.Tabs {
		if tab.DocumentID == "" {
			tab.DocumentID = tab.ID
		}
		doc, found, err := store.LoadDocument(tab.DocumentID)
		if err != nil {
			return nil, err
		}
		session := &Session{Tab: tab, Available: found || tab.SourcePath == ""}
		if found {
			session.Format, session.Payload, session.ViewState, session.Dirty, session.UpdatedAt = doc.Format, doc.Payload, doc.ViewState, doc.Dirty, doc.UpdatedAt
		}
		if tab.SourcePath != "" {
			if _, err := os.Stat(tab.SourcePath); err != nil {
				session.Available = false
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

func (c *Collection) ActiveTabID() string { return c.active }

func (c *Collection) Active() *Session {
	if c.active == RecentTabID {
		return nil
	}
	s, _ := c.Get(c.active)
	return s
}

func (c *Collection) Get(id string) (*Session, bool) {
	for _, s := range c.tabs {
		if s.Tab.ID == id {
			return s, true
		}
	}
	return nil, false
}

func (c *Collection) Tabs() []Session {
	out := make([]Session, len(c.tabs))
	for i, s := range c.tabs {
		out[i] = *s
	}
	return out
}

func (c *Collection) Recents() []Recent { return append([]Recent(nil), c.recents...) }

func (c *Collection) Activate(id string) error {
	if id != RecentTabID {
		if _, ok := c.Get(id); !ok {
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
	return c.put(session, true)
}

func (c *Collection) Refresh(session Session) error { return c.put(session, false) }

func (c *Collection) AddTab(tab Tab, format string) error {
	return c.put(Session{Tab: tab, Format: format, Available: true}, true)
}

func (c *Collection) UpdateTabMeta(tab Tab) error {
	for _, s := range c.tabs {
		if s.Tab.ID == tab.ID {
			s.Tab = tab
			return c.persist()
		}
	}
	return fmt.Errorf("workspace tab %q does not exist", tab.ID)
}

func (c *Collection) put(session Session, activate bool) error {
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
	if old, ok := c.Get(session.Tab.ID); ok {
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
	s, ok := c.Get(id)
	if !ok {
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
	s, ok := c.Get(id)
	if !ok {
		return fmt.Errorf("workspace tab %q does not exist", id)
	}
	if err := c.store.SaveViewState(s.Tab.DocumentID, viewState); err != nil {
		return err
	}
	s.ViewState = append(s.ViewState[:0], viewState...)
	return nil
}

func (c *Collection) AddRecent(recent Recent) error {
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

func (c *Collection) RemoveRecent(path string) error {
	for i, recent := range c.recents {
		if recent.Path == path {
			c.recents = append(c.recents[:i], c.recents[i+1:]...)
			break
		}
	}
	return c.store.DeleteRecent(path)
}

func (c *Collection) CloseTab(id string) error {
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
	c.tabs = append(c.tabs[:index], c.tabs[index+1:]...)
	for i, s := range c.tabs {
		s.Tab.Position = i
	}
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
