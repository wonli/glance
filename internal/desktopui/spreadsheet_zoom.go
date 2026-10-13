package desktopui

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	spreadsheetZoomMinimum = 0.5
	spreadsheetZoomMaximum = 2.0
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
	if a.documentSessionState.spreadsheetZoom == zoom {
		return
	}
	a.documentSessionState.spreadsheetZoom = zoom
	if a.virtualGrid != nil {
		a.virtualGrid.CellPaintCache = nil
		a.virtualGrid.CellPaintFIFO = nil
		a.virtualGrid.GeometryChanged = true
	}
	a.persistViewState()
	if a.window != nil {
		a.window.Invalidate()
	}
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

// spreadsheetZoomFraction converts a zoom level in [0.5, 2.0] to a track fraction in [0, 1].
// The midpoint fraction 0.5 corresponds exactly to zoom 1.0 (100%).
func spreadsheetZoomFraction(zoom float64) float64 {
	zoom = spreadsheetZoomClamp(zoom)
	if zoom <= 1.0 {
		return (zoom - spreadsheetZoomMinimum) / (1.0 - spreadsheetZoomMinimum) * 0.5
	}
	return 0.5 + (zoom-1.0)/(spreadsheetZoomMaximum-1.0)*0.5
}

// spreadsheetZoomFromFraction converts a track fraction in [0, 1] back to a zoom level in [0.5, 2.0].
// The midpoint fraction 0.5 corresponds exactly to zoom 1.0 (100%).
func spreadsheetZoomFromFraction(frac float64) float64 {
	frac = math.Max(0, math.Min(1, frac))
	if frac <= 0.5 {
		return spreadsheetZoomMinimum + (frac/0.5)*(1.0-spreadsheetZoomMinimum)
	}
	return 1.0 + ((frac-0.5)/0.5)*(spreadsheetZoomMaximum-1.0)
}
