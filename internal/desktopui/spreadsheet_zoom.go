package desktopui

import (
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

// spreadsheetZoomFraction converts a zoom level in [0.5, 2.0] to a track fraction in [0, 1].
// The midpoint fraction 0.5 corresponds exactly to zoom 1.0 (100%).
func spreadsheetZoomFraction(zoom float64) float64 {
	zoom = spreadsheetZoomClamp(zoom)
	if zoom <= 1.0 {
		return (zoom - spreadsheetZoomMinimum) / (1.0 - spreadsheetZoomMinimum) * 0.5
	}
	return 0.5 + (zoom - 1.0) / (spreadsheetZoomMaximum - 1.0) * 0.5
}

// spreadsheetZoomFromFraction converts a track fraction in [0, 1] back to a zoom level in [0.5, 2.0].
// The midpoint fraction 0.5 corresponds exactly to zoom 1.0 (100%).
func spreadsheetZoomFromFraction(frac float64) float64 {
	frac = math.Max(0, math.Min(1, frac))
	if frac <= 0.5 {
		return spreadsheetZoomMinimum + (frac / 0.5) * (1.0 - spreadsheetZoomMinimum)
	}
	return 1.0 + ((frac - 0.5) / 0.5) * (spreadsheetZoomMaximum - 1.0)
}
