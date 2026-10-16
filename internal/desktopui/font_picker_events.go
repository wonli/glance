package desktopui

import (
	"context"
	"fmt"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/fontcatalog"
	"glance/internal/fontdownload"
)

func (a *App) openFontPicker(target fontPickerTarget) {
	a.initFontPicker()
	p := &a.fontPicker
	if p.open && p.target.state == target.state && p.target.word == target.word {
		p.open = false
		return
	}
	a.closeOfficeRibbonMenu()
	p.open = true
	p.focusRequested = true
	p.target = target
	p.search.SetText("")
	p.documentNames = nil
	p.rowsDirty = true
	p.visibleRowKeys = p.visibleRowKeys[:0]
	p.active = 0
	p.list.Position = layout.Position{}
	if a.desktopRuntime != nil && a.fonts != nil {
		p.snapshot = a.fonts.Snapshot()
	}
	a.updateFontPickerResolvedFace()
	p.entries = nil
	if p.providerCancel != nil {
		p.providerCancel()
	}
	p.providerGeneration++
	generation := p.providerGeneration
	if a.desktopRuntime != nil && a.fontProvider != nil {
		provider := a.fontProvider
		ctx, cancel := context.WithCancel(context.Background())
		p.providerCancel = cancel
		a.launchManagedBackground(func() {
			select {
			case <-a.shutdown:
				cancel()
			case <-ctx.Done():
			}
		})
		a.launchManagedBackground(func() {
			defer cancel()
			entries, err := provider.List(ctx)
			result := fontProviderResult{entries: entries, err: err, generation: generation}
			select {
			case p.providerResults <- result:
			case <-a.shutdown:
				return
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		})
	}
	state := target.state
	a.launchManagedBackground(func() {
		names, err := documentFontNames(state)
		result := fontDocumentFonts{state: state, revision: target.revision, names: names, err: err}
		select {
		case p.documents <- result:
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	})
}
func (a *App) applyFontChoice(family string) bool {
	family = strings.TrimSpace(family)
	if family == "" || !a.fontPickerTargetValid() {
		a.fontPicker.open = false
		return false
	}
	t := a.fontPicker.target
	a.fontPicker.open = false
	if strings.EqualFold(strings.TrimSpace(t.current), family) && (t.word || t.selection.Anchor == t.selection.Active) {
		return true
	}
	if t.word {
		a.runWordCommand("字体", func(editor document.WordTextEditor) error {
			e, ok := editor.(document.WordTextFontFamilyEditor)
			if !ok {
				return fmt.Errorf("字体修改不可用")
			}
			return e.SetTextRunFontFamily(t.runID, family)
		})
	} else {
		a.applyCellFormat(document.CellFormatPatch{FontFamily: &family})
	}
	return true
}
func (a *App) processFontPickerEvents(gtx layout.Context) {
	p := &a.fontPicker
	if !p.open {
		return
	}
	if !a.fontPickerTargetValid() || a.modal.Visible {
		p.open = false
		return
	}
	rows := a.fontPickerRowsForCurrentState()
	if p.active >= len(rows) {
		p.active = max(0, len(rows)-1)
	}
	if p.close.Clicked(gtx) {
		a.closeFontPickerToAnchor(gtx)
	}
	if p.clear.Clicked(gtx) {
		p.search.SetText("")
		p.list.Position = layout.Position{}
	}
	if p.refresh.Clicked(gtx) && !p.event.Refreshing {
		a.refreshFontCatalog()
	}
	if p.dismiss.Clicked(gtx) {
		history := p.dismiss.History()
		if len(history) > 0 {
			pt := history[len(history)-1].Position
			if !pt.In(p.bounds) && !pt.In(p.anchor) {
				p.open = false
			}
		}
	}
	for _, key := range p.visibleRowKeys {
		i, exists := p.rowIndexes[key]
		if !exists || i < 0 || i >= len(rows) {
			continue
		}
		row := rows[i]
		button := p.buttons[key]
		if button == nil {
			continue
		}
		if button.download.Clicked(gtx) && row.online != nil {
			a.downloadFont(*row.online)
		}
		if button.choose.Clicked(gtx) && row.title == "" && !row.missing && row.online == nil {
			p.active = i
			a.applyFontPickerRow(row)
			if !p.open {
				break
			}
		}
	}
	filters := []event.Filter{
		key.Filter{Focus: &p.search, Name: key.NameUpArrow, Required: key.ModAlt},
		key.Filter{Focus: &p.search, Name: key.NameDownArrow, Required: key.ModAlt},
		key.Filter{Focus: &p.search, Name: key.NameTab},
		key.Filter{Focus: &p.keyTag, Name: key.NameReturn},
		key.Filter{Focus: &p.keyTag, Name: key.NameEnter},
		key.Filter{Focus: &p.keyTag, Name: key.NameEscape},
		key.Filter{Focus: &p.keyTag, Name: key.NameUpArrow},
		key.Filter{Focus: &p.keyTag, Name: key.NameDownArrow},
	}
	if !editorHasActiveComposition(&p.search) {
		filters = append(filters, key.Filter{Focus: &p.search, Name: key.NameEscape})
	}
	for {
		event, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		e, ok := event.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		switch e.Name {
		case key.NameEscape:
			a.closeFontPickerToAnchor(gtx)
		case key.NameDownArrow, key.NameUpArrow, key.NameTab:
			for {
				_, more := p.search.Update(gtx)
				if !more {
					break
				}
			}
			gtx.Execute(key.FocusCmd{Tag: &p.keyTag})
			step := 1
			if e.Name == key.NameUpArrow {
				step = -1
			}
			for n := 0; n < len(p.rows); n++ {
				p.active = (p.active + step + len(p.rows)) % len(p.rows)
				if p.rows[p.active].title == "" {
					break
				}
			}
			p.list.ScrollTo(p.active)
		case key.NameReturn, key.NameEnter:
			if len(rows) > 0 {
				row := rows[p.active]
				if row.title == "" && !row.missing && row.online == nil {
					a.applyFontPickerRow(row)
				}
			}
		}
	}
}
func fontRowKey(row fontPickerRow) string {
	if row.online != nil {
		return "online:" + row.online.ID
	}
	return row.title + ":" + row.family + ":" + string(row.faceID)
}
func (a *App) downloadFont(entry fontdownload.Entry) {
	p := &a.fontPicker
	if a.desktopRuntime == nil || a.fontDownloads == nil || a.fonts == nil {
		return
	}
	if old := p.downloads[entry.ID]; old.running {
		if old.cancel != nil {
			old.cancel()
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.downloads[entry.ID] = fontDownloadState{running: true, cancel: cancel}
	a.launchManagedBackground(func() {
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
	a.launchManagedBackground(func() {
		defer cancel()
		resource, err := a.fontDownloads.Fetch(ctx, entry, func(progress fontdownload.Progress) {
			select {
			case p.downloadResults <- fontDownloadResult{entry.ID, fontDownloadState{running: true, received: progress.Received, total: progress.Total, cancel: cancel}}:
			default:
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		})
		if err == nil {
			err = a.fonts.Register(ctx, resource)
		}
		state := fontDownloadState{}
		if err != nil {
			state.err = err.Error()
		}
		select {
		case p.downloadResults <- fontDownloadResult{entry.ID, state}:
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	})
}

type fontProviderResult struct {
	generation uint64
	entries    []fontdownload.Entry
	err        error
}

func (a *App) applyFontPickerRow(row fontPickerRow) {
	if !a.fontPickerTargetValid() {
		a.fontPicker.open = false
		return
	}
	resolved, resolveErr := a.fontPicker.snapshot.Resolve(fontcatalog.Request{Family: row.family, Weight: 400})
	if row.faceID != "" && (resolveErr != nil || resolved.Face.ID != row.faceID) && a.desktopRuntime != nil && a.fonts != nil {
		id := row.faceID
		a.launchManagedBackground(func() { a.fonts.Prefer(id) })
	}
	a.applyFontChoice(row.family)
}

func (a *App) closeFontPickerToAnchor(gtx layout.Context) {
	p := &a.fontPicker
	p.open = false
	if p.target.state != a.documentSessionState || p.target.state == nil {
		return
	}
	if p.target.word {
		if button := p.target.state.wordToolbarActionButtons["font-family"]; button != nil {
			gtx.Execute(key.FocusCmd{Tag: button})
		}
	} else {
		gtx.Execute(key.FocusCmd{Tag: &p.target.state.toolbarActionButtons[toolbarActionFontFamily]})
	}
}
