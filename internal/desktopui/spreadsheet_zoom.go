package desktopui

import (
	"encoding/json"
	"fmt"
	"math"

	"glance/internal/document"
)

const (
	spreadsheetZoomMinimum = 0.1
	spreadsheetZoomMaximum = 4.0
)

func spreadsheetZoomClamp(zoom float64) float64 {
	if math.IsNaN(zoom) || math.IsInf(zoom, 0) || zoom <= 0 {
		return 1.0
	}
	return math.Max(spreadsheetZoomMinimum, math.Min(spreadsheetZoomMaximum, zoom))
}

func (a *App) spreadsheetZoom() float64 {
	if a == nil || a.documentSessionState == nil || a.documentSessionState.spreadsheetZoom <= 0 {
		return 1.0
	}
	return spreadsheetZoomClamp(a.documentSessionState.spreadsheetZoom)
}

func (a *App) setSpreadsheetZoom(zoom float64) {
	if a == nil || a.documentSessionState == nil {
		return
	}
	zoom = spreadsheetZoomClamp(zoom)
	state := a.documentSessionState
	mapChanged := state.setSpreadsheetSheetZoom(state.sheet, int(math.Round(zoom*100)))
	viewChanged := setSpreadsheetZoomView(state, zoom)
	if !viewChanged && !mapChanged {
		return
	}
	a.persistViewState()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func setSpreadsheetZoomView(state *documentSessionState, zoom float64) bool {
	if state == nil {
		return false
	}
	zoom = spreadsheetZoomClamp(zoom)
	if state.spreadsheetZoom == zoom {
		return false
	}
	state.spreadsheetZoom = zoom
	if state.virtualGrid != nil {
		state.virtualGrid.CellPaintCache = nil
		state.virtualGrid.CellPaintFIFO = nil
		state.virtualGrid.GeometryChanged = true
	}
	return true
}

func spreadsheetZoomPercent(percent int) int {
	if percent < 10 || percent > 400 {
		return 100
	}
	return percent
}

func (state *documentSessionState) spreadsheetSheetZoomPercent(sheet string) int {
	if state == nil || state.sharedDocumentSession == nil || sheet == "" {
		return 100
	}
	state.spreadsheetZoomMu.RLock()
	percent := state.spreadsheetZooms[sheet]
	state.spreadsheetZoomMu.RUnlock()
	return spreadsheetZoomPercent(percent)
}

func (state *documentSessionState) spreadsheetSheetZoom(sheet string) float64 {
	return spreadsheetZoomClamp(float64(state.spreadsheetSheetZoomPercent(sheet)) / 100)
}

func (state *documentSessionState) setSpreadsheetSheetZoom(sheet string, percent int) bool {
	if state == nil || state.sharedDocumentSession == nil || sheet == "" {
		return false
	}
	percent = spreadsheetZoomPercent(percent)
	state.spreadsheetZoomMu.Lock()
	defer state.spreadsheetZoomMu.Unlock()
	if state.spreadsheetZooms == nil {
		state.spreadsheetZooms = make(map[string]int)
	}
	if state.spreadsheetZooms[sheet] == percent {
		return false
	}
	state.spreadsheetZooms[sheet] = percent
	state.spreadsheetZoomRevision.Add(1)
	return true
}

func (state *documentSessionState) spreadsheetZoomNeedsSave() bool {
	return state != nil && state.sharedDocumentSession != nil &&
		state.spreadsheetZoomRevision.Load() > state.spreadsheetZoomSavedRevision.Load()
}

func (state *documentSessionState) markSpreadsheetZoomSaved(revision uint64) {
	if state == nil || state.sharedDocumentSession == nil {
		return
	}
	for saved := state.spreadsheetZoomSavedRevision.Load(); revision > saved; saved = state.spreadsheetZoomSavedRevision.Load() {
		if state.spreadsheetZoomSavedRevision.CompareAndSwap(saved, revision) {
			return
		}
	}
}

func (state *documentSessionState) restoreSpreadsheetZoomPending() {
	if state != nil && state.sharedDocumentSession != nil && !state.spreadsheetZoomNeedsSave() {
		state.spreadsheetZoomRevision.Add(1)
	}
}

func (state *documentSessionState) spreadsheetSheetZoomSnapshot() map[string]int {
	if state == nil || state.sharedDocumentSession == nil {
		return nil
	}
	state.spreadsheetZoomMu.RLock()
	defer state.spreadsheetZoomMu.RUnlock()
	if len(state.spreadsheetZooms) == 0 {
		return nil
	}
	result := make(map[string]int, len(state.spreadsheetZooms))
	for sheet, percent := range state.spreadsheetZooms {
		result[sheet] = spreadsheetZoomPercent(percent)
	}
	return result
}

func (state *documentSessionState) restoreSpreadsheetSheetZooms(zooms map[string]int) {
	if state == nil || state.sharedDocumentSession == nil || len(zooms) == 0 {
		return
	}
	state.spreadsheetZoomMu.Lock()
	defer state.spreadsheetZoomMu.Unlock()
	if state.spreadsheetZooms == nil {
		state.spreadsheetZooms = make(map[string]int)
	}
	for sheet, percent := range zooms {
		if sheet != "" && (len(state.sheets) == 0 || containsString(state.sheets, sheet)) {
			percent = spreadsheetZoomPercent(percent)
			if state.spreadsheetZooms[sheet] != percent {
				state.spreadsheetZooms[sheet] = percent
				state.spreadsheetZoomRevision.Add(1)
			}
		}
	}
}

func initializeSpreadsheetSheetZooms(shared *sharedDocumentSession, book document.Spreadsheet, sheets []string) {
	if shared == nil || book == nil || len(sheets) == 0 {
		return
	}
	reader, ok := book.(document.SpreadsheetViewsEditor)
	if !ok {
		return
	}
	for _, sheet := range sheets {
		view, err := reader.GetSheetView(sheet)
		if err == nil {
			shared.spreadsheetZoomMu.Lock()
			if shared.spreadsheetZooms == nil {
				shared.spreadsheetZooms = make(map[string]int)
			}
			if _, exists := shared.spreadsheetZooms[sheet]; !exists {
				shared.spreadsheetZooms[sheet] = spreadsheetZoomPercent(view.ZoomPercent)
			}
			shared.spreadsheetZoomMu.Unlock()
		}
	}
}

// applySpreadsheetSheetZooms writes saved per-sheet presentation state into
// the XLSX package before snapshotting or saving it. Callers hold the session
// adapter write lock and run outside the Gio layout loop.
func applySpreadsheetSheetZooms(state *documentSessionState) error {
	if state == nil || state.book == nil {
		return nil
	}
	editor, ok := state.book.(document.SpreadsheetViewsEditor)
	if !ok {
		return nil
	}
	zooms := state.spreadsheetSheetZoomSnapshot()
	for sheet, percent := range zooms {
		if len(state.sheets) != 0 && !containsString(state.sheets, sheet) {
			continue
		}
		current, err := editor.GetSheetView(sheet)
		if err != nil {
			return err
		}
		percent = spreadsheetZoomPercent(percent)
		if current.ZoomPercent == percent {
			continue
		}
		if err := editor.SetSheetZoom(sheet, percent); err != nil {
			return err
		}
	}
	return nil
}

type runSpreadsheetZoomIntentPayload struct {
	Operation string `json:"operation"`
}

type runSpreadsheetZoomIntentPayloadOperation struct {
	Operation   string `json:"operation"`
	ZoomPercent int    `json:"zoom_percent"`
}

func (a *App) runSpreadsheetZoomIntent(operation string, zoomPercent int) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	var arguments any
	if operation == "set" {
		arguments = runSpreadsheetZoomIntentPayloadOperation{Operation: operation, ZoomPercent: zoomPercent}
	} else {
		arguments = runSpreadsheetZoomIntentPayload{Operation: operation}
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.applySpreadsheetZoomLocally(operation, zoomPercent)
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.zoom.apply", "attached_window", fmt.Sprint(a.windowID), sheet, arguments)
		var changed bool
		if err == nil {
			var outcome struct {
				Changed bool `json:"changed"`
			}
			err = json.Unmarshal(result.Data, &outcome)
			changed = outcome.Changed
		}
		a.completeSessionOperation(state, token, err, false)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "缩放", applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) applySpreadsheetZoomLocally(operation string, zoomPercent int) {
	current := a.spreadsheetZoom()
	target := current
	switch operation {
	case "zoom_in":
		target = math.Min(spreadsheetZoomMaximum, current+0.1)
	case "reset":
		target = 1
	case "set":
		if zoomPercent < 50 || zoomPercent > 200 {
			return
		}
		target = float64(zoomPercent) / 100
	default:
		return
	}
	a.setSpreadsheetZoom(target)
	if operation == "reset" {
		a.setStatus("缩放比例：100%")
	} else {
		a.setStatus(fmt.Sprintf("缩放比例：%.0f%%", a.spreadsheetZoom()*100))
	}
}

// spreadsheetZoomFraction converts a zoom level in [0.1, 4.0] to a track fraction in [0, 1].
// The midpoint fraction 0.5 corresponds exactly to zoom 1.0 (100%).
func spreadsheetZoomFraction(zoom float64) float64 {
	zoom = spreadsheetZoomClamp(zoom)
	if zoom <= 1.0 {
		return (zoom - spreadsheetZoomMinimum) / (1.0 - spreadsheetZoomMinimum) * 0.5
	}
	return 0.5 + (zoom-1.0)/(spreadsheetZoomMaximum-1.0)*0.5
}

// spreadsheetZoomFromFraction converts a track fraction in [0, 1] back to a zoom level in [0.1, 4.0].
// The midpoint fraction 0.5 corresponds exactly to zoom 1.0 (100%).
func spreadsheetZoomFromFraction(frac float64) float64 {
	frac = math.Max(0, math.Min(1, frac))
	if frac <= 0.5 {
		return spreadsheetZoomMinimum + (frac/0.5)*(1.0-spreadsheetZoomMinimum)
	}
	return 1.0 + ((frac-0.5)/0.5)*(spreadsheetZoomMaximum-1.0)
}
