package desktopui

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/fontmanager"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/fontmanager/fontdownload"
	"image"
)

type fontPickerTarget struct {
	state                 *documentSessionState
	runID, sheet, current string
	revision              uint64
	selection             CellSelection
	word                  bool
}
type fontPickerButton struct{ choose, download widget.Clickable }
type fontDocumentFonts struct {
	state    *documentSessionState
	revision uint64
	names    []string
	err      error
}
type fontPickerState struct {
	focusRequested                             bool
	open                                       bool
	target                                     fontPickerTarget
	search                                     widget.Editor
	list                                       widget.List
	close, refresh, dismiss, clear, panelInput widget.Clickable
	buttons                                    map[string]*fontPickerButton
	bounds, anchor                             image.Rectangle
	rows                                       []fontPickerRow
	rowIndexes                                 map[string]int
	visibleRowKeys                             []string
	rowsQuery                                  string
	rowsSnapshotVersion                        uint64
	rowsCacheValid, rowsDirty                  bool
	familyRowCount, sectionRowCount            int
	snapshot                                   fontcatalog.Snapshot
	displayNames                               map[string]string
	displayNamesVersion                        uint64
	resolvedFaceID                             fontcatalog.ResourceID
	resolvedFaceFamily                         string
	resolvedFaceVersion                        uint64
	resolvedFaceCacheValid                     bool
	event                                      fontmanager.Event
	events                                     chan fontmanager.Event
	documents                                  chan fontDocumentFonts
	documentNames                              []string
	entries                                    []fontdownload.Entry
	providerCancel                             func()
	providerGeneration                         uint64
	providerResults                            chan fontProviderResult
	providerErr                                string
	downloads                                  map[string]fontDownloadState
	downloadResults                            chan fontDownloadResult
	renderResults                              chan fontRenderResult
	renderPending                              string
	renderInputKey                             string
	renderAccepted                             string
	renderCache                                *fontCollectionCache
	initialized                                bool
	keyTag                                     struct{}
	active                                     int
}

func (a *App) fontPickerRowsForCurrentState() []fontPickerRow {
	p := &a.fontPicker
	query := p.search.Text()
	if p.rowsCacheValid && !p.rowsDirty && p.rowsQuery == query && p.rowsSnapshotVersion == p.snapshot.Version {
		return p.rows
	}

	p.rows = fontPickerRows(p.snapshot, p.documentNames, p.entries, query)
	p.rowIndexes = make(map[string]int, len(p.rows))
	p.familyRowCount = 0
	p.sectionRowCount = 0
	for i, row := range p.rows {
		key := fontRowKey(row)
		if _, exists := p.rowIndexes[key]; !exists {
			p.rowIndexes[key] = i
		}
		if row.title == "" {
			p.familyRowCount++
		} else {
			p.sectionRowCount++
		}
	}
	p.rowsQuery = query
	p.rowsSnapshotVersion = p.snapshot.Version
	p.rowsDirty = false
	p.rowsCacheValid = true
	return p.rows
}

func (a *App) updateFontPickerResolvedFace() {
	p := &a.fontPicker
	p.resolvedFaceID = ""
	p.resolvedFaceFamily = p.target.current
	p.resolvedFaceVersion = p.snapshot.Version
	p.resolvedFaceCacheValid = true
	if family := p.target.current; family != "" {
		if resolved, err := p.snapshot.Resolve(fontcatalog.Request{Family: family, Weight: 400}); err == nil {
			p.resolvedFaceID = resolved.Face.ID
		}
	}
}

func (a *App) fontPickerResolvedFaceID() fontcatalog.ResourceID {
	p := &a.fontPicker
	if !p.resolvedFaceCacheValid || p.resolvedFaceFamily != p.target.current || p.resolvedFaceVersion != p.snapshot.Version {
		a.updateFontPickerResolvedFace()
	}
	return p.resolvedFaceID
}

type fontDownloadState struct {
	running         bool
	received, total int64
	err             string
	cancel          func()
}
type fontDownloadResult struct {
	id    string
	state fontDownloadState
}

func (a *App) fontPickerTargetValid() bool {
	p := &a.fontPicker
	t := p.target
	s := a.documentSessionState
	if s == nil || s != t.state || s.wordRevisionAtomic.Load() != t.revision {
		return false
	}
	if t.word {
		return s.word != nil && s.selectedWordRunID == t.runID
	}
	return s.book != nil && s.selectionSet && s.sheet == t.sheet && s.selection == t.selection
}
func (a *App) initFontPicker() {
	p := &a.fontPicker
	if p.initialized {
		return
	}
	p.initialized = true
	p.search.SingleLine = true
	p.list.List.Axis = layout.Vertical
	p.buttons = map[string]*fontPickerButton{}
	p.documents = make(chan fontDocumentFonts, 4)
	p.events = make(chan fontmanager.Event, 1)
	p.downloadResults = make(chan fontDownloadResult, 16)
	p.renderResults = make(chan fontRenderResult, 2)
	p.downloads = map[string]fontDownloadState{}
	p.providerResults = make(chan fontProviderResult, 2)
}
