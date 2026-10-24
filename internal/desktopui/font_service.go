package desktopui

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"glance/internal/diagramrender"
	"glance/internal/fontmanager"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/fontmanager/fontdownload"
	"glance/internal/i18n"
)

func (a *App) currentFontSnapshot() fontcatalog.Snapshot {
	if a == nil {
		return fontcatalog.Snapshot{}
	}
	if a.desktopRuntime != nil && a.desktopRuntime.fonts != nil {
		return a.desktopRuntime.fonts.Snapshot()
	}
	// Windowless App values are used by tests and offline utilities. A real
	// desktop window always has a process manager and never uses this fallback.
	if a.window == nil {
		return fontcatalog.Default()
	}
	return fontcatalog.Snapshot{}
}

func (a *App) fontVersion() uint64 { return a.currentFontSnapshot().Version }

func newDesktopFontManager() *fontmanager.Manager {
	cache, cacheErr := os.UserCacheDir()
	if cacheErr != nil {
		cache = ""
	}
	data, dataErr := os.UserConfigDir()
	if dataErr != nil {
		data = ""
	}
	root, objects := "", ""
	if data != "" {
		root = filepath.Join(data, "glance", "fonts")
		objects = filepath.Join(root, "objects")
		_ = os.MkdirAll(objects, 0700)
	}
	var privateDirectories []string
	if objects != "" {
		privateDirectories = []string{objects}
	}
	cachePath := ""
	if cache != "" {
		cachePath = filepath.Join(cache, "glance", "fonts", "catalog-v1.json")
	}
	return fontmanager.New(fontmanager.Options{
		Scanner:     fontcatalog.Scanner{DiscoverSystem: true, PrivateDirectories: privateDirectories},
		CachePath:   cachePath,
		PrivateRoot: root,
		Provider:    fontdownload.DefaultManifest(),
	})
}

func (a *App) startFontService() {
	a.initFontPicker()
	r := a.ensureDesktopRuntime()
	r.fontInit.Do(func() {
		if r.fonts == nil {
			r.fonts = newDesktopFontManager()
		}
		manager := r.fonts
		a.launchManagedBackground(func() {
			_ = manager.CleanStaging()
			if err := manager.Initialize(context.Background()); err != nil && manager.Snapshot().Version == 0 {
				log.Printf("initialize font manager: %v", err)
			}
		})
	})
	if r.fonts == nil {
		return
	}
	events, unsubscribe := r.fonts.Subscribe()
	a.launchManagedBackground(func() {
		defer unsubscribe()
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				select {
				case a.fontPicker.events <- event:
				default:
					select {
					case <-a.fontPicker.events:
					default:
					}
					select {
					case a.fontPicker.events <- event:
					default:
					}
				}
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
				return
			}
		}
	})
}

func (a *App) prepareInitialFontTheme() error {
	if a == nil || a.fonts == nil || a.theme == nil {
		return fmt.Errorf("font manager is unavailable")
	}
	snapshot, err := a.fonts.Ready(context.Background())
	if err != nil {
		return fmt.Errorf("wait for initial font catalog: %w", err)
	}
	cache := newFontCollectionCache()
	collection := fontCollectionForRendering(snapshot, nil, cache)
	if !hasUsableUIFontCollection(collection) {
		return fmt.Errorf("initial font catalog contains no usable Glance UI font faces")
	}
	a.theme.Shaper = newTextShaper(collection)
	a.fontPicker.snapshot = snapshot
	a.fontPicker.event = fontmanager.Event{Snapshot: snapshot}
	a.fontPicker.renderCache = cache
	a.fontPicker.renderAccepted = fmt.Sprintf("%d:", snapshot.Version)

	a.wordLayoutEngine = nil
	a.wordLayoutEngineFontVersion = 0
	a.wordPDFFontSource = nil
	for _, s := range a.sessions {
		if s != nil && s.word != nil {
			s.cancelWordLayout()
			s.wordLayoutAcceptedKey = nil
			s.wordLayoutPendingKey = nil
			s.wordLayoutErrorKey = nil
			s.wordLayoutResult = nil
			s.wordLayoutError = ""
		}
	}
	return nil
}

func (a *App) pollFontResults() {
	p := &a.fontPicker
	if !p.initialized {
		return
	}
	for {
		select {
		case event := <-p.events:
			p.event = event
			if event.Snapshot.Version != p.snapshot.Version {
				p.snapshot = event.Snapshot
				diagramrender.SetFontVersion(p.snapshot.Version)
				for _, state := range a.sessions {
					if state != nil && state.markdownView != nil {
						v := state.markdownView
						if v.parseCancel != nil {
							v.parseCancel()
						}
						v.parseGeneration++
						v.pending = false
						v.images = nil
						v.requested = ""
					}
				}
				p.rowsDirty = true
				p.resolvedFaceCacheValid = false
				p.renderAccepted = ""
				p.renderPending = ""
				p.renderInputKey = ""
				a.wordLayoutEngine = nil
				a.wordLayoutEngineFontVersion = 0
				a.wordPDFFontSource = nil
				for _, s := range a.sessions {
					if s == nil {
						continue
					}
					s.cancelWordLayout()
					s.wordLayoutAcceptedKey = nil
					s.wordLayoutPendingKey = nil
					s.wordLayoutErrorKey = nil
					if s.virtualGrid != nil {
						s.virtualGrid.CellPaintCache = nil
						s.virtualGrid.CellPaintFIFO = nil
					}
				}
			}
		case result := <-p.providerResults:
			if result.generation != p.providerGeneration {
				continue
			}
			p.entries = result.entries
			p.rowsDirty = true
			if result.err != nil {
				p.providerErr = result.err.Error()
			} else {
				p.providerErr = ""
			}
		case result := <-p.documents:
			if result.state == p.target.state && result.revision == p.target.revision {
				p.documentNames = result.names
				p.rowsDirty = true
				if result.err != nil {
					a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, result.err, notificationToast)
				}
			}
		case result := <-p.downloadResults:
			p.downloads[result.id] = result.state
		case result := <-p.renderResults:
			if result.key == p.renderPending {
				p.renderPending = ""
				if result.err != nil {
					a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, result.err, notificationToast)
				} else if result.shaper != nil {
					a.theme.Shaper = result.shaper
					p.renderAccepted = result.key
					if a.virtualGrid != nil {
						a.virtualGrid.CellPaintCache = nil
						a.virtualGrid.CellPaintFIFO = nil
					}
				}
			}
		default:
			a.prepareFontRendering()
			return
		}
	}
}
func (a *App) refreshFontCatalog() {
	if a.desktopRuntime == nil || a.fonts == nil {
		return
	}
	a.launchManagedBackground(func() { a.fonts.Refresh(context.Background(), true) })
}
