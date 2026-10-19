package desktopui

import (
	"sync"

	"glance/internal/i18n"
	"glance/internal/systemlocale"
)

type localeSnapshot struct {
	Selection i18n.Selection
	Resolved  i18n.Locale
	Catalog   *i18n.Catalog
}

type localeState struct {
	mu        sync.RWMutex
	Selection i18n.Selection
	Resolved  i18n.Locale
	Catalog   *i18n.Catalog
}

func (s *localeState) initialize() {
	selection := i18n.SelectionSystem
	resolved := i18n.Resolve(selection, systemlocale.PreferredLanguages())
	catalog, err := i18n.NewCatalog(resolved, nil)
	if err != nil {
		resolved = i18n.LocaleEnglish
		catalog, _ = i18n.NewCatalog(resolved, nil)
	}
	s.update(selection, resolved, catalog)
}

func (s *localeState) update(selection i18n.Selection, resolved i18n.Locale, catalog *i18n.Catalog) {
	s.mu.Lock()
	s.Selection = selection
	s.Resolved = resolved
	s.Catalog = catalog
	s.mu.Unlock()
}

func (s *localeState) snapshot() localeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return localeSnapshot{Selection: s.Selection, Resolved: s.Resolved, Catalog: s.Catalog}
}

func (a *App) localeSnapshot() localeSnapshot {
	runtime := a.ensureDesktopRuntime()
	runtime.localeStateOnce.Do(runtime.localeState.initialize)
	return runtime.localeState.snapshot()
}

func (a *App) text(message i18n.Message) string {
	snapshot := a.localeSnapshot()
	if snapshot.Catalog == nil {
		return message.ID
	}
	return snapshot.Catalog.Text(message)
}

func (a *App) textID(id string) string {
	return a.text(i18n.Message{ID: id})
}

func (a *App) textArgs(id string, args i18n.Args) string {
	return a.text(i18n.Message{ID: id, Args: args})
}

func (a *App) plural(message i18n.Message, count int64) string {
	snapshot := a.localeSnapshot()
	if snapshot.Catalog == nil {
		return message.ID
	}
	return snapshot.Catalog.Plural(message, count)
}

func (a *App) messageText(message i18n.Message, fallback string) string {
	if message.ID == "" {
		return fallback
	}
	return a.text(message)
}

func (a *App) statusText() string {
	if a.statusRef.ID != "" {
		return a.text(a.statusRef)
	}
	return a.messageText(a.initialStatus, "")
}
