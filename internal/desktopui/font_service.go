package desktopui

import (
	"context"
	"os"
	"path/filepath"

	"glance/internal/application"
	"glance/internal/fontcatalog"
	"glance/internal/fontdownload"
)

func (a *App) startFontService() {
	a.initFontPicker()
	r := a.ensureDesktopRuntime()
	r.fontInit.Do(func() {
		cache, _ := os.UserCacheDir()
		data, _ := os.UserConfigDir()
		root := filepath.Join(data, "glance", "fonts")
		objects := filepath.Join(root, "objects")

		os.MkdirAll(objects, 0700)
		r.fonts = application.NewFontService(application.FontOptions{Publish: fontcatalog.SetDefault, Scanner: fontcatalog.Scanner{DiscoverSystem: true, PrivateDirectories: []string{objects}}, CachePath: filepath.Join(cache, "glance", "fonts", "catalog-v1.json")})
		r.fontDownloads = fontdownload.New(root, nil)
		r.fontProvider = fontdownload.DefaultManifest()
		a.launchManagedBackground(func() { r.fontDownloads.CleanStaging(); r.fonts.Initialize(context.Background()) })
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
				p.rowsDirty = true
				p.resolvedFaceCacheValid = false
				p.renderAccepted = ""
				p.renderPending = ""
				p.renderInputKey = ""
				a.wordLayoutEngine = nil
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
					a.setStatus("读取文档字体失败：" + result.err.Error())
				}
			}
		case result := <-p.downloadResults:
			p.downloads[result.id] = result.state
		case result := <-p.renderResults:
			if result.key == p.renderPending {
				p.renderPending = ""
				if result.shaper != nil {
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
